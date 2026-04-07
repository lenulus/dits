package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/lenulus/pf/internal/logging"
	"github.com/lenulus/pf/internal/project"
	mcpgo "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

// TestRedactedArgs_AllowsSafelist asserts the safelisted keys round-trip
// through redactedArgs. The set lives in tools.go; if a future contributor
// adds a tool with a new id-shaped argument, this test should be updated
// to extend the safelist alongside the tool registration.
func TestRedactedArgs_AllowsSafelist(t *testing.T) {
	in := map[string]any{
		"id":     "wrk_abc",
		"kind":   "task",
		"status": "open",
		"title":  "okay to log",
	}
	out := redactedArgs(in)
	for k, v := range in {
		if got, ok := out[k]; !ok || got != v {
			t.Fatalf("safelisted key %q dropped or mangled: got=%v ok=%v", k, got, ok)
		}
	}
}

// TestRedactedArgs_DropsSensitive asserts the high-sensitivity fields
// listed in the redactedArgs doc comment never appear at DEBUG level.
// This is the contract tests/clients rely on; promoting any of these to
// the safelist must be a deliberate change documented in the commit.
func TestRedactedArgs_DropsSensitive(t *testing.T) {
	in := map[string]any{
		"id":           "wrk_abc",
		"body":         "this is a long user-supplied prompt",
		"summary":      "should never be logged at DEBUG",
		"plan":         "leaks stakeholder context",
		"context":      "private",
		"statement":    "claim text",
		"metrics_json": `{"score": 0.5}`,
		"error":        "internal stack trace",
	}
	out := redactedArgs(in)

	if got, ok := out["id"]; !ok || got != "wrk_abc" {
		t.Fatalf("expected id to round-trip, got %v", got)
	}
	for _, key := range []string{
		"body", "summary", "plan", "context",
		"statement", "metrics_json", "error",
	} {
		if _, ok := out[key]; ok {
			t.Fatalf("sensitive key %q leaked into redacted args", key)
		}
	}
}

// TestRequestIDCorrelation drives the same request_id from the MCP
// middleware (where the ULID is minted) through to the workops layer's
// `event appended` line. This is the load-bearing observability
// guarantee: a single grep on request_id reconstructs the call.
//
// The test wires a memory-backed slog logger into the in-process MCP
// client/server pair, calls dits_work_create, and asserts that both the
// `tool start` line (emitted by withOps) and the `event appended` line
// (emitted by workops.AppendAndMaterialize) carry the same request_id.
func TestRequestIDCorrelation(t *testing.T) {
	// Capture all log output for inspection.
	var buf bytes.Buffer
	logger := logging.New(&buf, slog.LevelDebug, "json")

	// Init a temp project; pass the captured logger into Config so both
	// the middleware and workops emit through it.
	root := t.TempDir()
	proj, err := project.Init(root, "CORR")
	if err != nil {
		t.Fatalf("project.Init: %v", err)
	}
	if err := proj.DB.Close(); err != nil {
		t.Fatalf("close project: %v", err)
	}

	srv := NewServer(Config{ProjectRoot: root, Logger: logger})
	cli, err := mcpgo.NewInProcessClient(srv)
	if err != nil {
		t.Fatalf("NewInProcessClient: %v", err)
	}
	defer cli.Close()

	ctx := context.Background()
	if err := cli.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{Name: "corr", Version: "0.0.0"}
	if _, err := cli.Initialize(ctx, initReq); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	createReq := mcp.CallToolRequest{}
	createReq.Params.Name = "dits_work_create"
	createReq.Params.Arguments = map[string]any{
		"title": "correlation",
		"kind":  "task",
		"body":  "this body must NOT appear in any log line at debug",
	}
	res, err := cli.CallTool(ctx, createReq)
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("dits_work_create returned error result")
	}

	// Walk every JSON log line, group by request_id, confirm we see at
	// least one `tool start` and one `event appended` sharing the same id.
	type record struct {
		Msg       string `json:"msg"`
		RequestID string `json:"request_id"`
		Tool      string `json:"tool"`
		Type      string `json:"type"`
		Body      string `json:"body,omitempty"`
	}
	scanner := bytes.NewReader(buf.Bytes())
	dec := json.NewDecoder(scanner)
	idsByMsg := map[string]map[string]struct{}{}
	for {
		var r record
		if err := dec.Decode(&r); err != nil {
			break
		}
		if r.RequestID == "" {
			continue
		}
		if idsByMsg[r.Msg] == nil {
			idsByMsg[r.Msg] = map[string]struct{}{}
		}
		idsByMsg[r.Msg][r.RequestID] = struct{}{}
	}

	if len(idsByMsg["tool start"]) == 0 {
		t.Fatalf("no `tool start` line with request_id captured; log was:\n%s", buf.String())
	}
	if len(idsByMsg["event appended"]) == 0 {
		t.Fatalf("no `event appended` line with request_id captured; log was:\n%s", buf.String())
	}

	// Intersection must be non-empty: at least one ID appears in both.
	var matched bool
	for id := range idsByMsg["tool start"] {
		if _, ok := idsByMsg["event appended"][id]; ok {
			matched = true
			break
		}
	}
	if !matched {
		t.Fatalf("no shared request_id between `tool start` and `event appended`; log was:\n%s", buf.String())
	}

	// Redaction guard: the `body` value MUST NOT appear in any log line.
	// (At DEBUG, redactedArgs is what guards this; at INFO, only
	// arg_keys is logged. TRACE would dump the full payload — we did not
	// enable TRACE here.)
	if strings.Contains(buf.String(), "this body must NOT appear") {
		t.Fatalf("sensitive body value leaked into log output; log was:\n%s", buf.String())
	}
}
