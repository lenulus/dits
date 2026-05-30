package workops_test

import (
	"context"
	"testing"

	"github.com/lenulus/pf/internal/domain"
)

// TestAckLifecycle_BothAccept drives the canonical commitment path end to end
// through the workops funnel: file → builder-accept → specifier-accept →
// aligned. This pins the contract every frontend (CLI, MCP, Pilot) shares.
func TestAckLifecycle_BothAccept(t *testing.T) {
	ctx := context.Background()
	w := newTestOps(t)

	created, err := w.CreateWorkItem(ctx, "task", "recurring billing", "", nil)
	if err != nil {
		t.Fatalf("CreateWorkItem: %v", err)
	}
	id := created.WorkItem.ID

	if _, err := w.AckFile(ctx, id, "ship recurring billing v1", "2026 Q3", "GA by Q3", "tests pass"); err != nil {
		t.Fatalf("AckFile: %v", err)
	}
	if _, err := w.AckAccept(ctx, id, domain.AckSideBuilder, "implementable"); err != nil {
		t.Fatalf("AckAccept(builder): %v", err)
	}
	wi, err := w.AckAccept(ctx, id, domain.AckSideSpecifier, "agreed")
	if err != nil {
		t.Fatalf("AckAccept(specifier): %v", err)
	}
	if len(wi.Acks) != 1 {
		t.Fatalf("expected 1 ack, got %d", len(wi.Acks))
	}
	if got := wi.Acks[0].Rollup(); got != domain.RollupAligned {
		t.Fatalf("rollup = %s, want aligned", got)
	}
}

// TestAckLifecycle_ScopeAmendmentClearsBoth pins the auto-clear: once both
// sides have accepted, a scope_change amendment resets both to pending in a
// single AckAmend call (the cleared event is auto-emitted by workops).
func TestAckLifecycle_ScopeAmendmentClearsBoth(t *testing.T) {
	ctx := context.Background()
	w := newTestOps(t)

	created, err := w.CreateWorkItem(ctx, "task", "webhook retry", "", nil)
	if err != nil {
		t.Fatalf("CreateWorkItem: %v", err)
	}
	id := created.WorkItem.ID

	if _, err := w.AckFile(ctx, id, "retry policy", "2026 Q3", "GA", "criteria"); err != nil {
		t.Fatalf("AckFile: %v", err)
	}
	if _, err := w.AckAccept(ctx, id, domain.AckSideBuilder, ""); err != nil {
		t.Fatalf("AckAccept(builder): %v", err)
	}
	if _, err := w.AckAccept(ctx, id, domain.AckSideSpecifier, ""); err != nil {
		t.Fatalf("AckAccept(specifier): %v", err)
	}

	wi, err := w.AckAmend(ctx, id, domain.AmendmentScopeChange, []string{"scope_summary"}, "added retry-storm hardening")
	if err != nil {
		t.Fatalf("AckAmend: %v", err)
	}
	a := wi.Acks[len(wi.Acks)-1]
	if a.Rollup() != domain.RollupBothPending {
		t.Fatalf("rollup after scope amendment = %s, want both_pending", a.Rollup())
	}
	if len(a.Amendments) != 1 || a.Amendments[0].Type != domain.AmendmentScopeChange {
		t.Fatalf("expected one scope_change amendment, got %+v", a.Amendments)
	}

	// An ack_cleared event must be in the log.
	events, err := w.GetEvents(ctx, id)
	if err != nil {
		t.Fatalf("GetEvents: %v", err)
	}
	if countType(events, domain.EventWorkAckCleared) != 1 {
		t.Fatalf("expected exactly 1 ack_cleared event, got %d", countType(events, domain.EventWorkAckCleared))
	}
}

// TestAckLifecycle_ClarificationDoesNotClear pins that a clarification
// amendment leaves an aligned commitment intact (no auto-clear, no cleared
// event).
func TestAckLifecycle_ClarificationDoesNotClear(t *testing.T) {
	ctx := context.Background()
	w := newTestOps(t)

	created, err := w.CreateWorkItem(ctx, "task", "audit log export", "", nil)
	if err != nil {
		t.Fatalf("CreateWorkItem: %v", err)
	}
	id := created.WorkItem.ID

	if _, err := w.AckFile(ctx, id, "export", "2026 Q3", "GA", "criteria"); err != nil {
		t.Fatalf("AckFile: %v", err)
	}
	if _, err := w.AckAccept(ctx, id, domain.AckSideBuilder, ""); err != nil {
		t.Fatalf("AckAccept(builder): %v", err)
	}
	if _, err := w.AckAccept(ctx, id, domain.AckSideSpecifier, ""); err != nil {
		t.Fatalf("AckAccept(specifier): %v", err)
	}

	wi, err := w.AckAmend(ctx, id, domain.AmendmentClarification, []string{"acceptance_criteria"}, "made criteria explicit")
	if err != nil {
		t.Fatalf("AckAmend: %v", err)
	}
	if got := wi.Acks[len(wi.Acks)-1].Rollup(); got != domain.RollupAligned {
		t.Fatalf("rollup after clarification = %s, want aligned (no clear)", got)
	}
	events, err := w.GetEvents(ctx, id)
	if err != nil {
		t.Fatalf("GetEvents: %v", err)
	}
	if n := countType(events, domain.EventWorkAckCleared); n != 0 {
		t.Fatalf("clarification must not emit ack_cleared, got %d", n)
	}
}

// TestAckAmend_TargetChangeRequestsReview pins that a target_change amendment
// both clears (after acceptance) and emits a review request.
func TestAckAmend_TargetChangeRequestsReview(t *testing.T) {
	ctx := context.Background()
	w := newTestOps(t)

	created, err := w.CreateWorkItem(ctx, "task", "pilot independence", "", nil)
	if err != nil {
		t.Fatalf("CreateWorkItem: %v", err)
	}
	id := created.WorkItem.ID

	if _, err := w.AckFile(ctx, id, "enforce independence", "2026 Q4", "GA", "criteria"); err != nil {
		t.Fatalf("AckFile: %v", err)
	}
	if _, err := w.AckAccept(ctx, id, domain.AckSideSpecifier, ""); err != nil {
		t.Fatalf("AckAccept(specifier): %v", err)
	}
	if _, err := w.AckAmend(ctx, id, domain.AmendmentTargetChange, []string{"target_outcome"}, "carve-out moved out"); err != nil {
		t.Fatalf("AckAmend: %v", err)
	}
	events, err := w.GetEvents(ctx, id)
	if err != nil {
		t.Fatalf("GetEvents: %v", err)
	}
	if countType(events, domain.EventWorkAckCleared) != 1 {
		t.Fatalf("target_change after acceptance should clear; got %d cleared events", countType(events, domain.EventWorkAckCleared))
	}
	if countType(events, domain.EventWorkReviewRequested) != 1 {
		t.Fatalf("target_change should request review; got %d review_requested events", countType(events, domain.EventWorkReviewRequested))
	}
}

func countType(events []domain.Event, typ domain.EventType) int {
	n := 0
	for _, e := range events {
		if e.Type == typ {
			n++
		}
	}
	return n
}
