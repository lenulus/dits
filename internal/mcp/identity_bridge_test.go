package mcp

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"

	"github.com/lenulus/pf/internal/crypto"
	"github.com/lenulus/pf/internal/domain"
	"github.com/lenulus/pf/internal/logging"
	"github.com/lenulus/pf/internal/project"
	"github.com/lenulus/pf/internal/workops"
	"github.com/mark3labs/mcp-go/mcp"
)

// TestReviewRequestToolConformance drives dits_review_request through the
// in-process MCP harness and asserts the work.review_requested event lands and
// round-trips with its reviewer_role (Track F.3 acceptance).
func TestReviewRequestToolConformance(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	proj, err := project.Init(root, "REV")
	if err != nil {
		t.Fatalf("project.Init: %v", err)
	}
	w := &workops.WorkOps{Proj: proj}
	w.SetLogger(logging.Discard())
	m, err := w.CreateWorkItem(ctx, "task", "Idle decision", "", nil)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	id := string(m.WorkItem.ID)
	if err := proj.DB.Close(); err != nil {
		t.Fatalf("close DB: %v", err)
	}

	cli := startInProcMCP(t, root)
	mustCallOK(t, cli, "dits_review_request", map[string]any{
		"id": id, "reviewer_role": "leadership", "scope": "decision_idle",
	})

	// The event must be in the work item's log with the role on its payload.
	evRes := mustCallOK(t, cli, "dits_work_events", map[string]any{"id": id, "type": "work.review_requested"})
	var events []domain.Event
	if err := json.Unmarshal([]byte(contentString(evRes)), &events); err != nil {
		t.Fatalf("decode events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("want 1 review_requested event, got %d", len(events))
	}
	var p domain.ReviewRequestedPayload
	if err := json.Unmarshal(events[0].Payload, &p); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if p.ReviewerRole != "leadership" {
		t.Errorf("reviewer_role = %q, want leadership", p.ReviewerRole)
	}
	if p.Scope != "decision_idle" {
		t.Errorf("scope = %q, want decision_idle", p.Scope)
	}
	if p.ReviewID == "" {
		t.Errorf("review_id was not allocated")
	}
}

// TestEventSubmitToolConformance constructs and signs an event as a freshly
// registered actor, submits it through dits_event_submit, and confirms it is
// appended and reduces (Track F.1 acceptance). It also asserts the substrate
// rejects a tampered signature and an unknown actor.
func TestEventSubmitToolConformance(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	proj, err := project.Init(root, "SUB")
	if err != nil {
		t.Fatalf("project.Init: %v", err)
	}
	w := &workops.WorkOps{Proj: proj}
	w.SetLogger(logging.Discard())

	// A work item to comment on, plus the heads to parent the new event.
	m, err := w.CreateWorkItem(ctx, "task", "Custodial signing", "", nil)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	wiID := m.WorkItem.ID
	heads, err := proj.DB.GetHeads(ctx, wiID)
	if err != nil {
		t.Fatalf("heads: %v", err)
	}
	meta, err := w.LoadMeta(ctx)
	if err != nil {
		t.Fatalf("meta: %v", err)
	}
	metaVersion := meta.Version

	// Mint an external actor keypair and register it (the verify path requires
	// a registered public key).
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("genkey: %v", err)
	}
	actor := domain.ActorID("actor_pilot_test")
	if err := proj.DB.RegisterActor(ctx, actor, hex.EncodeToString(pub), ""); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := proj.DB.Close(); err != nil {
		t.Fatalf("close DB: %v", err)
	}

	// Build the event the way Pilot would, and sign it as the external actor.
	build := func() domain.Event {
		return domain.Event{
			ID:             domain.NewEventID(),
			WorkItemID:     wiID,
			Type:           domain.EventWorkCommented,
			ParentEventIDs: heads,
			MetaVersion:    metaVersion,
			ActorID:        actor,
			Timestamp:      time.Now().UTC(),
			Payload:        domain.MustMarshalPayload(domain.CommentPayload{Body: "signed by pilot custodial key"}),
		}
	}
	signed := build()
	if err := crypto.SignEvent(&signed, priv); err != nil {
		t.Fatalf("sign: %v", err)
	}

	cli := startInProcMCP(t, root)

	// Happy path: a correctly-signed event from a registered actor is admitted.
	signedJSON, _ := json.Marshal(signed)
	showRes := mustCallOK(t, cli, "dits_event_submit", map[string]any{"signed_event_json": string(signedJSON)})
	var got domain.WorkItem
	if err := json.Unmarshal([]byte(contentString(showRes)), &got); err != nil {
		t.Fatalf("decode work item: %v", err)
	}

	// Confirm via the event log that the comment was appended attributed to the
	// external actor (NOT the project's dits-mcp actor).
	evRes := mustCallOK(t, cli, "dits_work_events", map[string]any{"id": string(wiID), "type": "work.commented"})
	var events []domain.Event
	if err := json.Unmarshal([]byte(contentString(evRes)), &events); err != nil {
		t.Fatalf("decode events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("want 1 commented event, got %d", len(events))
	}
	if events[0].ActorID != actor {
		t.Errorf("appended event actor = %q, want %q (attribution preserved)", events[0].ActorID, actor)
	}

	// Tampered signature: flip a payload byte after signing → reject.
	tampered := build()
	if err := crypto.SignEvent(&tampered, priv); err != nil {
		t.Fatalf("sign tampered: %v", err)
	}
	tampered.Payload = domain.MustMarshalPayload(domain.CommentPayload{Body: "different body, stale signature"})
	tamperedJSON, _ := json.Marshal(tampered)
	if res := callTool(t, cli, "dits_event_submit", map[string]any{"signed_event_json": string(tamperedJSON)}); !res.IsError {
		t.Errorf("expected error result for tampered signature, got ok")
	}

	// Unknown actor: sign with a fresh, unregistered key → reject.
	_, otherPriv, _ := ed25519.GenerateKey(rand.Reader)
	unknown := build()
	unknown.ActorID = domain.ActorID("actor_unregistered")
	if err := crypto.SignEvent(&unknown, otherPriv); err != nil {
		t.Fatalf("sign unknown: %v", err)
	}
	unknownJSON, _ := json.Marshal(unknown)
	if res := callTool(t, cli, "dits_event_submit", map[string]any{"signed_event_json": string(unknownJSON)}); !res.IsError {
		t.Errorf("expected error result for unknown actor, got ok")
	}
}

// callTool invokes a tool and returns the (possibly error) result without
// failing the test on an error *result* — used for negative assertions.
func callTool(t *testing.T, cli interface {
	CallTool(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)
}, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	req := mcp.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = args
	res, err := cli.CallTool(context.Background(), req)
	if err != nil {
		t.Fatalf("%s transport error: %v", name, err)
	}
	return res
}
