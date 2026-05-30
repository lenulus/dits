package projections

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/lenulus/pf/internal/pilot/mcp"
)

// --- fixture helpers ---

func ev(workItemID, etype, ts string, payload map[string]any) mcp.Event {
	var raw json.RawMessage
	if payload != nil {
		raw, _ = json.Marshal(payload)
	}
	return mcp.Event{WorkItemID: workItemID, Type: etype, Timestamp: ts, Payload: raw}
}

const day = 24 * time.Hour

// TestDependencyClosureRate: a shipped milestone depending on one shipped and
// one open target → 0.5 closure. A non-shipped milestone's edges are ignored.
func TestDependencyClosureRate(t *testing.T) {
	items := []mcp.WorkItem{
		{ID: "m1", Kind: "milestone", Status: "shipped", Relations: []mcp.Relation{
			{Type: "depends_on", TargetWorkItem: "dep_shipped"},
			{Type: "depends_on", TargetWorkItem: "dep_open"},
			{Type: "relates_to", TargetWorkItem: "dep_shipped"}, // ignored: not depends_on
		}},
		{ID: "dep_shipped", Kind: "milestone", Status: "shipped"},
		{ID: "dep_open", Kind: "milestone", Status: "in_flight"},
		// An unshipped milestone's depends_on edges must not count.
		{ID: "m2", Kind: "milestone", Status: "in_flight", Relations: []mcp.Relation{
			{Type: "depends_on", TargetWorkItem: "dep_shipped"},
		}},
	}
	if got := DependencyClosureRate(items); got != 0.5 {
		t.Fatalf("DependencyClosureRate = %v, want 0.5", got)
	}

	// No depends_on edges on shipped milestones → 0.
	none := []mcp.WorkItem{{ID: "m3", Kind: "milestone", Status: "shipped"}}
	if got := DependencyClosureRate(none); got != 0 {
		t.Fatalf("DependencyClosureRate(no edges) = %v, want 0", got)
	}
}

// TestDependencyClosureRate_SharedID: a depends_on edge can point at a target's
// shared ID; a shipped target registered under both IDs still counts closed.
func TestDependencyClosureRate_SharedID(t *testing.T) {
	items := []mcp.WorkItem{
		{ID: "m1", Kind: "milestone", Status: "shipped", Relations: []mcp.Relation{
			{Type: "depends_on", TargetWorkItem: "PROJ-9"},
		}},
		{ID: "wrk_9", SharedID: "PROJ-9", Kind: "milestone", Status: "shipped"},
	}
	if got := DependencyClosureRate(items); got != 1.0 {
		t.Fatalf("DependencyClosureRate(shared id) = %v, want 1.0", got)
	}
}

// TestScopeChangeVelocity: 2 scope_change amendments over a 4-week window →
// 0.5/week. Non-scope amendments and other event types are ignored.
func TestScopeChangeVelocity(t *testing.T) {
	events := []mcp.Event{
		ev("m1", "work.ack_amended", "2026-05-01T10:00:00Z", map[string]any{"amendment_type": "scope_change"}),
		ev("m2", "work.ack_amended", "2026-05-08T10:00:00Z", map[string]any{"amendment_type": "scope_change"}),
		ev("m1", "work.ack_amended", "2026-05-09T10:00:00Z", map[string]any{"amendment_type": "clarification"}),
		ev("m3", "work.status_set", "2026-05-09T10:00:00Z", map[string]any{"to": "shipped"}),
	}
	if got := ScopeChangeVelocity(events, 4); got != 0.5 {
		t.Fatalf("ScopeChangeVelocity = %v, want 0.5", got)
	}
	// weeks < 1 is treated as 1 (per-window count).
	if got := ScopeChangeVelocity(events, 0); got != 2 {
		t.Fatalf("ScopeChangeVelocity(weeks=0) = %v, want 2", got)
	}
}

// TestDecisionFriction: one RFC takes 4d to approve, a second 2d → median 3d.
// One DecisionBlock takes 5d to resolve → median 5d. Unrelated kinds ignored.
func TestDecisionFriction(t *testing.T) {
	events := []mcp.Event{
		// RFC a: created 05-01, approved 05-05 (4d).
		ev("rfc_a", "work.created", "2026-05-01T00:00:00Z", map[string]any{"kind": "rfc"}),
		ev("rfc_a", "work.status_set", "2026-05-05T00:00:00Z", map[string]any{"to": "approved"}),
		// RFC b: created 05-01, approved 05-03 (2d).
		ev("rfc_b", "work.created", "2026-05-01T00:00:00Z", map[string]any{"kind": "rfc"}),
		ev("rfc_b", "work.status_set", "2026-05-02T00:00:00Z", map[string]any{"to": "leadership_review"}),
		ev("rfc_b", "work.status_set", "2026-05-03T00:00:00Z", map[string]any{"to": "approved"}),
		// DecisionBlock d: created 05-01, escalated 05-06 (5d).
		ev("db_d", "work.created", "2026-05-01T00:00:00Z", map[string]any{"kind": "decision_block"}),
		ev("db_d", "work.status_set", "2026-05-06T00:00:00Z", map[string]any{"to": "escalated"}),
		// A milestone shipping must not be mistaken for an RFC/DB closure.
		ev("m1", "work.created", "2026-05-01T00:00:00Z", map[string]any{"kind": "milestone"}),
		ev("m1", "work.status_set", "2026-05-02T00:00:00Z", map[string]any{"to": "shipped"}),
	}
	rfcMed, dbMed := DecisionFriction(events)
	if rfcMed != 3*day {
		t.Fatalf("rfc median = %v, want %v", rfcMed, 3*day)
	}
	if dbMed != 5*day {
		t.Fatalf("decision-block median = %v, want %v", dbMed, 5*day)
	}
}

// TestDecisionFrictionEmpty: no completed pairs → both zero.
func TestDecisionFrictionEmpty(t *testing.T) {
	events := []mcp.Event{
		ev("rfc_a", "work.created", "2026-05-01T00:00:00Z", map[string]any{"kind": "rfc"}),
		// never approved
	}
	rfcMed, dbMed := DecisionFriction(events)
	if rfcMed != 0 || dbMed != 0 {
		t.Fatalf("expected zero medians, got rfc=%v db=%v", rfcMed, dbMed)
	}
}

// TestAckToStartLatency: builder accepts 05-02, specifier accepts 05-04
// (aligned = later = 05-04), first execution_started 05-06 → 2d. A second
// milestone aligned with no start contributes nothing.
func TestAckToStartLatency(t *testing.T) {
	events := []mcp.Event{
		ev("m1", "work.ack_filed", "2026-05-01T00:00:00Z", nil),
		ev("m1", "work.ack_accepted", "2026-05-02T00:00:00Z", map[string]any{"who": "builder"}),
		ev("m1", "work.ack_accepted", "2026-05-04T00:00:00Z", map[string]any{"who": "specifier"}),
		ev("m1", "work.execution_started", "2026-05-06T00:00:00Z", nil),
		// m2 aligns but never starts → excluded.
		ev("m2", "work.ack_accepted", "2026-05-02T00:00:00Z", map[string]any{"who": "builder"}),
		ev("m2", "work.ack_accepted", "2026-05-03T00:00:00Z", map[string]any{"who": "specifier"}),
	}
	if got := AckToStartLatency(events); got != 2*day {
		t.Fatalf("AckToStartLatency = %v, want %v", got, 2*day)
	}
}

// TestAckToStartLatency_AmendmentResets: a material amendment clears the
// commitment, so alignment is measured from the SECOND time both sides accept,
// and an execution_started before re-alignment does not count.
func TestAckToStartLatency_AmendmentResets(t *testing.T) {
	events := []mcp.Event{
		ev("m1", "work.ack_accepted", "2026-05-01T00:00:00Z", map[string]any{"who": "builder"}),
		ev("m1", "work.ack_accepted", "2026-05-02T00:00:00Z", map[string]any{"who": "specifier"}), // aligned #1
		ev("m1", "work.ack_cleared", "2026-05-03T00:00:00Z", nil),                                 // reset both
		ev("m1", "work.ack_accepted", "2026-05-04T00:00:00Z", map[string]any{"who": "builder"}),
		ev("m1", "work.ack_accepted", "2026-05-05T00:00:00Z", map[string]any{"who": "specifier"}), // aligned #2
		ev("m1", "work.execution_started", "2026-05-07T00:00:00Z", nil),
	}
	// The clear drops alignment, so latency is measured from the SECOND
	// alignment (05-05) to the start (05-07) = 2d, not from the first.
	if got := AckToStartLatency(events); got != 2*day {
		t.Fatalf("AckToStartLatency = %v, want %v (re-alignment 05-05 → start 05-07)", got, 2*day)
	}
}

// TestAckToStartLatency_StartWhileUnaligned: an execution_started that happens
// after a clear but before re-alignment is ignored; only a start while aligned
// counts. Here the only start (05-035) lands during the unaligned window, so
// there is no sample → 0.
func TestAckToStartLatency_StartWhileUnaligned(t *testing.T) {
	events := []mcp.Event{
		ev("m1", "work.ack_accepted", "2026-05-01T00:00:00Z", map[string]any{"who": "builder"}),
		ev("m1", "work.ack_accepted", "2026-05-02T00:00:00Z", map[string]any{"who": "specifier"}), // aligned
		ev("m1", "work.ack_cleared", "2026-05-03T00:00:00Z", nil),                                 // unaligned
		ev("m1", "work.execution_started", "2026-05-03T12:00:00Z", nil),                           // during unaligned window
	}
	if got := AckToStartLatency(events); got != 0 {
		t.Fatalf("AckToStartLatency = %v, want 0 (start while unaligned must not count)", got)
	}
}

// TestMedianEven: even sample count averages the two middle values.
func TestMedianEven(t *testing.T) {
	got := medianDuration([]time.Duration{1 * day, 3 * day, 5 * day, 9 * day})
	if got != 4*day { // (3d+5d)/2
		t.Fatalf("medianDuration even = %v, want %v", got, 4*day)
	}
}

// --- Cache.Rebuild round-trip with a fake client ---

// fakeClient implements mcp.Client returning canned events/items. Only the
// methods Rebuild touches do anything; the rest satisfy the interface.
type fakeClient struct {
	events []mcp.Event
	items  []mcp.WorkItem
	err    error
}

func (f *fakeClient) EventsList(context.Context, string, string, int) ([]mcp.Event, error) {
	return f.events, f.err
}
func (f *fakeClient) WorkList(context.Context, mcp.Filters) ([]mcp.WorkItem, error) {
	return f.items, f.err
}

// remaining interface methods — unused by Rebuild.
func (f *fakeClient) WorkGet(context.Context, string) (mcp.WorkItem, error) {
	return mcp.WorkItem{}, nil
}
func (f *fakeClient) RoleBindingsList(context.Context, string) ([]mcp.RoleBinding, error) {
	return nil, nil
}
func (f *fakeClient) DiagnosticsGet(context.Context, string) ([]mcp.Diagnostic, error) {
	return nil, nil
}
func (f *fakeClient) MetaGet(context.Context) (mcp.Meta, error)            { return mcp.Meta{}, nil }
func (f *fakeClient) ActorList(context.Context) ([]mcp.ActorRecord, error) { return nil, nil }
func (f *fakeClient) WorkCreate(context.Context, string, string, string) (string, error) {
	return "", nil
}
func (f *fakeClient) SetStatus(context.Context, string, string) error          { return nil }
func (f *fakeClient) SetTitle(context.Context, string, string) error            { return nil }
func (f *fakeClient) SetBody(context.Context, string, string) error             { return nil }
func (f *fakeClient) Link(context.Context, string, string, string) error       { return nil }
func (f *fakeClient) Unlink(context.Context, string, string, string) error     { return nil }
func (f *fakeClient) FieldSet(context.Context, string, string, string) error   { return nil }
func (f *fakeClient) ScheduleSet(context.Context, string, []mcp.Stage) error   { return nil }
func (f *fakeClient) Observe(context.Context, string, string, json.RawMessage) error {
	return nil
}
func (f *fakeClient) Classify(context.Context, string, string, string) error   { return nil }
func (f *fakeClient) Declassify(context.Context, string, string, string) error { return nil }
func (f *fakeClient) BindRole(context.Context, string, string, string) error   { return nil }
func (f *fakeClient) UnbindRole(context.Context, string, string, string) error { return nil }
func (f *fakeClient) AckFile(context.Context, string, string, string, string, string) error {
	return nil
}
func (f *fakeClient) AckAccept(context.Context, string, string, string) error { return nil }
func (f *fakeClient) AckReject(context.Context, string, string, string) error { return nil }
func (f *fakeClient) AckAmend(context.Context, string, string, []string, string) error {
	return nil
}
func (f *fakeClient) MetaApply(context.Context, []byte) error { return nil }
func (f *fakeClient) TaxonomyNodeAdd(context.Context, string, string, string, string) error {
	return nil
}
func (f *fakeClient) TaxonomyNodeMove(context.Context, string, string, string) error {
	return nil
}
func (f *fakeClient) TaxonomyNodeRetire(context.Context, string, string) error { return nil }
func (f *fakeClient) TaxonomyNodeSet(context.Context, string, string, json.RawMessage) error {
	return nil
}
func (f *fakeClient) ActorRegister(context.Context, string, string, string) error { return nil }
func (f *fakeClient) ReviewRequest(context.Context, string, string, string) error { return nil }
func (f *fakeClient) EventSubmit(context.Context, []byte) error                    { return nil }
func (f *fakeClient) Close() error                                                 { return nil }

func TestCacheRebuild(t *testing.T) {
	fc := &fakeClient{
		items: []mcp.WorkItem{
			{ID: "m1", Kind: "milestone", Status: "shipped", Relations: []mcp.Relation{
				{Type: "depends_on", TargetWorkItem: "dep_shipped"},
				{Type: "depends_on", TargetWorkItem: "dep_open"},
			}},
			{ID: "dep_shipped", Kind: "milestone", Status: "shipped"},
			{ID: "dep_open", Kind: "milestone", Status: "in_flight"},
		},
		events: []mcp.Event{
			ev("m1", "work.ack_amended", "2026-05-01T00:00:00Z", map[string]any{"amendment_type": "scope_change"}),
			ev("m1", "work.ack_amended", "2026-05-08T00:00:00Z", map[string]any{"amendment_type": "scope_change"}),
		},
	}

	c := NewCacheWithWindow(4)
	if _, ok := c.Get(); ok {
		t.Fatalf("fresh cache should be invalid")
	}
	if err := c.Rebuild(context.Background(), fc); err != nil {
		t.Fatalf("Rebuild: %v", err)
	}
	ind, ok := c.Get()
	if !ok {
		t.Fatalf("cache should be valid after Rebuild")
	}
	if ind.DependencyClosureRate != 0.5 {
		t.Fatalf("closure rate = %v, want 0.5", ind.DependencyClosureRate)
	}
	if ind.ScopeChangeVelocity != 0.5 {
		t.Fatalf("scope velocity = %v, want 0.5", ind.ScopeChangeVelocity)
	}
	if ind.ComputedAt.IsZero() {
		t.Fatalf("ComputedAt should be set")
	}

	// Invalidate flips validity but retains the snapshot.
	c.Invalidate()
	if _, ok := c.Get(); ok {
		t.Fatalf("cache should be invalid after Invalidate")
	}
}

// TestCacheRebuildError: a fetch error leaves the prior snapshot untouched.
func TestCacheRebuildError(t *testing.T) {
	c := NewCache()
	if err := c.Rebuild(context.Background(), &fakeClient{err: context.DeadlineExceeded}); err == nil {
		t.Fatalf("expected Rebuild to surface the fetch error")
	}
	if _, ok := c.Get(); ok {
		t.Fatalf("cache should remain invalid after a failed Rebuild")
	}
}
