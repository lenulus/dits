package workops_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lenulus/pf/internal/project"
	"github.com/lenulus/pf/internal/store"
	"github.com/lenulus/pf/internal/workops"
)

// newTestOps initializes a fresh DITS project in a temp dir and returns a
// WorkOps bound to it. The caller does not need to defer Shutdown — the
// temp dir is cleaned by t.TempDir.
func newTestOps(t *testing.T) *workops.WorkOps {
	t.Helper()
	root := t.TempDir()
	proj, err := project.Init(root, "WTEST")
	if err != nil {
		t.Fatalf("project.Init: %v", err)
	}
	w := &workops.WorkOps{Proj: proj}
	t.Cleanup(func() { _ = w.Shutdown() })
	return w
}

// TestWorkLoop drives the canonical execution loop directly against
// WorkOps: create → lease → start → checkpoint → complete → release.
// This is the load-bearing path the CLI shims and the MCP tools both
// delegate to; testing it here pins the behavior independently of either
// frontend, so a third frontend (TUI, web) can rely on the same contract.
func TestWorkLoop(t *testing.T) {
	ctx := context.Background()
	w := newTestOps(t)

	// Create.
	created, err := w.CreateWorkItem(ctx, "task", "loop test", "body text", []string{})
	if err != nil {
		t.Fatalf("CreateWorkItem: %v", err)
	}
	if created.WorkItem == nil {
		t.Fatalf("CreateWorkItem returned nil work item")
	}
	if !strings.HasPrefix(string(created.SharedID), "WTEST-") {
		t.Fatalf("expected shared ID prefixed WTEST-, got %q", created.SharedID)
	}
	if created.WorkItem.Title != "loop test" {
		t.Fatalf("title round-trip failed: %q", created.WorkItem.Title)
	}
	id := created.WorkItem.ID

	// Lease.
	leased, err := w.Lease(ctx, id)
	if err != nil {
		t.Fatalf("Lease: %v", err)
	}
	if leased.WorkItem.LeaseHolder == nil {
		t.Fatalf("lease holder not set after Lease")
	}
	if *leased.WorkItem.LeaseHolder != w.ActorID() {
		t.Fatalf("lease holder mismatch: got %s want %s", *leased.WorkItem.LeaseHolder, w.ActorID())
	}

	// Lease conflict — second lease against the same item must error.
	if _, err := w.Lease(ctx, id); err == nil {
		t.Fatalf("expected error on double-lease, got nil")
	}

	// Start.
	started, err := w.Start(ctx, id)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if started.AttemptID == "" {
		t.Fatalf("Start returned empty attempt ID")
	}
	if started.WorkItem.CurrentAttempt == nil || *started.WorkItem.CurrentAttempt != started.AttemptID {
		t.Fatalf("CurrentAttempt not updated after Start")
	}

	// Checkpoint twice.
	if _, err := w.Checkpoint(ctx, id, "halfway", 0.5); err != nil {
		t.Fatalf("Checkpoint(0.5): %v", err)
	}
	cp, err := w.Checkpoint(ctx, id, "almost there", 0.9)
	if err != nil {
		t.Fatalf("Checkpoint(0.9): %v", err)
	}
	if len(cp.Checkpoints) != 2 {
		t.Fatalf("expected 2 checkpoints, got %d", len(cp.Checkpoints))
	}

	// Complete.
	completed, err := w.Complete(ctx, id, "done")
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if len(completed.Attempts) != 1 {
		t.Fatalf("expected 1 attempt recorded, got %d", len(completed.Attempts))
	}

	// Release lease.
	released, err := w.LeaseRelease(ctx, id, "wrapped up")
	if err != nil {
		t.Fatalf("LeaseRelease: %v", err)
	}
	if released.LeaseHolder != nil {
		t.Fatalf("lease holder still set after release")
	}

	// Re-lease must now succeed (nothing holding it).
	if _, err := w.Lease(ctx, id); err != nil {
		t.Fatalf("re-Lease after release: %v", err)
	}

	// Event log records the full sequence.
	events, err := w.GetEvents(ctx, id)
	if err != nil {
		t.Fatalf("GetEvents: %v", err)
	}
	// 1 create + 1 lease + 1 start + 2 checkpoints + 1 complete + 1 release + 1 re-lease = 8.
	if len(events) < 8 {
		t.Fatalf("expected at least 8 events in the log, got %d", len(events))
	}
}

// TestCreateValidation pins the input contracts the CLI used to enforce
// inline. These now live in CreateWorkItem itself.
func TestCreateValidation(t *testing.T) {
	ctx := context.Background()
	w := newTestOps(t)

	if _, err := w.CreateWorkItem(ctx, "task", "", "", nil); err == nil {
		t.Fatalf("expected error for empty title")
	}

	// Empty kind defaults to "task" rather than erroring.
	res, err := w.CreateWorkItem(ctx, "", "kind defaults", "", nil)
	if err != nil {
		t.Fatalf("CreateWorkItem(empty kind): %v", err)
	}
	if res.WorkItem.Kind != "task" {
		t.Fatalf("expected kind to default to task, got %q", res.WorkItem.Kind)
	}
}

// TestResolveAndList exercises the lookup paths used by every CLI/MCP
// command that takes a work-item reference: by full ID, by shared ID,
// and the not-found case.
func TestResolveAndList(t *testing.T) {
	ctx := context.Background()
	w := newTestOps(t)

	a, err := w.CreateWorkItem(ctx, "task", "first", "", nil)
	if err != nil {
		t.Fatalf("create first: %v", err)
	}
	b, err := w.CreateWorkItem(ctx, "task", "second", "", nil)
	if err != nil {
		t.Fatalf("create second: %v", err)
	}

	// Resolve by canonical ID.
	got, err := w.ResolveWorkItem(ctx, string(a.WorkItem.ID))
	if err != nil || got.ID != a.WorkItem.ID {
		t.Fatalf("resolve by ID: got=%v err=%v", got, err)
	}

	// Resolve by shared ID.
	got, err = w.ResolveWorkItem(ctx, string(b.SharedID))
	if err != nil || got.ID != b.WorkItem.ID {
		t.Fatalf("resolve by shared ID: got=%v err=%v", got, err)
	}

	// Not found.
	if _, err := w.ResolveWorkItem(ctx, "wrk_doesnotexist"); err == nil {
		t.Fatalf("expected not-found error for unknown ID")
	}

	// List returns both.
	items, err := w.ListWorkItems(ctx, store.WorkItemFilter{}, false)
	if err != nil {
		t.Fatalf("ListWorkItems: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
}

// TestAttach round-trips a real file through the blob store + artifact
// event so we know the path that backs dits_work_attach actually writes
// content and surfaces it on the work item.
func TestAttach(t *testing.T) {
	ctx := context.Background()
	w := newTestOps(t)

	created, err := w.CreateWorkItem(ctx, "task", "with file", "", nil)
	if err != nil {
		t.Fatalf("CreateWorkItem: %v", err)
	}

	tmp := filepath.Join(t.TempDir(), "evidence.txt")
	const payload = "hello workops"
	if err := os.WriteFile(tmp, []byte(payload), 0o644); err != nil {
		t.Fatalf("write tmp: %v", err)
	}

	att, err := w.Attach(ctx, created.WorkItem.ID, tmp)
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}
	if att.SizeBytes != int64(len(payload)) {
		t.Fatalf("size mismatch: got %d want %d", att.SizeBytes, len(payload))
	}
	if att.Filename != "evidence.txt" {
		t.Fatalf("filename mismatch: %q", att.Filename)
	}
	if len(att.WorkItem.Artifacts) != 1 {
		t.Fatalf("expected 1 artifact on work item, got %d", len(att.WorkItem.Artifacts))
	}

	// Blob store should now hold the content under the returned hash.
	has, err := w.Proj.Blobs.Has(ctx, att.Hash)
	if err != nil {
		t.Fatalf("Blobs.Has: %v", err)
	}
	if !has {
		t.Fatalf("blob %s missing from store after Attach", att.Hash)
	}
}

// TestBlockUnblock pins the block/unblock branch separately from the
// main loop because it gates ready-queue filtering in the work-loop
// skill.
func TestBlockUnblock(t *testing.T) {
	ctx := context.Background()
	w := newTestOps(t)

	created, err := w.CreateWorkItem(ctx, "task", "blockable", "", nil)
	if err != nil {
		t.Fatalf("CreateWorkItem: %v", err)
	}
	id := created.WorkItem.ID

	wi, err := w.Block(ctx, id, "waiting on infra")
	if err != nil {
		t.Fatalf("Block: %v", err)
	}
	if !wi.Blocked {
		t.Fatalf("expected work item to be blocked")
	}

	wi, err = w.Unblock(ctx, id, "infra back")
	if err != nil {
		t.Fatalf("Unblock: %v", err)
	}
	if wi.Blocked {
		t.Fatalf("expected work item to be unblocked")
	}
}
