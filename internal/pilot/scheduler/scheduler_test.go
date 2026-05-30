package scheduler

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/lenulus/pf/internal/pilot/mcp"
)

// fakeClient is an in-package mcp.Client for driving RunOnce. It embeds the
// package's stub (mcp.NewStub) so the many unused interface methods are no-ops,
// and overrides the four the scheduler exercises: WorkList serves canned
// responses keyed by "<kind>/<status>" (status "" = any), while WorkCreate /
// SetStatus / Link record their calls for assertions.
type fakeClient struct {
	mcp.Client // embedded stub: default no-op impls for everything else

	// lists maps "<kind>/<status>" → rows. Look-up tries the exact key, then
	// "<kind>/" (any-status) so a single canned list can answer both.
	lists map[string][]mcp.WorkItem

	creates  []createCall
	statuses []statusCall
	links    []linkCall

	// nextID is handed back from WorkCreate (incrementing) so Link targets are
	// distinguishable.
	nextID int

	// failCreate, when set, makes WorkCreate return it (error-path test).
	failCreate error
}

type createCall struct{ kind, title, body string }
type statusCall struct{ id, status string }
type linkCall struct{ id, relType, target string }

func newFake() *fakeClient {
	return &fakeClient{Client: mcp.NewStub(), lists: map[string][]mcp.WorkItem{}}
}

func (f *fakeClient) put(kind, status string, items ...mcp.WorkItem) {
	f.lists[kind+"/"+status] = items
}

func (f *fakeClient) WorkList(_ context.Context, flt mcp.Filters) ([]mcp.WorkItem, error) {
	if v, ok := f.lists[flt.Kind+"/"+flt.Status]; ok {
		return v, nil
	}
	if v, ok := f.lists[flt.Kind+"/"]; ok && flt.Status == "" {
		return v, nil
	}
	return nil, nil
}

func (f *fakeClient) WorkCreate(_ context.Context, kind, title, body string) (string, error) {
	if f.failCreate != nil {
		return "", f.failCreate
	}
	f.creates = append(f.creates, createCall{kind, title, body})
	f.nextID++
	return idFor(f.nextID), nil
}

func (f *fakeClient) SetStatus(_ context.Context, id, status string) error {
	f.statuses = append(f.statuses, statusCall{id, status})
	return nil
}

func (f *fakeClient) Link(_ context.Context, id, relType, target string) error {
	f.links = append(f.links, linkCall{id, relType, target})
	return nil
}

func idFor(n int) string {
	return "OA-NEW-" + strconv.Itoa(n) // stable, unique marker per WorkCreate
}

// fixedNow returns a clock pinned to t.
func fixedNow(t time.Time) func() time.Time { return func() time.Time { return t } }

// ref point for all relative timestamps in the tests.
var base = time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)

func rfc(d time.Duration) string { return base.Add(d).Format(time.RFC3339) }

// --- Action 1: outcome assessments for shipped milestones ---

func TestRunOnce_CreatesAssessmentForUnpairedShippedMilestone(t *testing.T) {
	f := newFake()
	f.put(kindMilestone, statusShipped, mcp.WorkItem{ID: "wrk_1", SharedID: "PROJ-1", Kind: kindMilestone, Title: "Ship payments", Status: statusShipped})
	// No existing assessments.
	f.put(kindAssessment, "" /* none */)

	sum, err := newSched(f).RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if sum.AssessmentsCreated != 1 {
		t.Fatalf("AssessmentsCreated = %d, want 1", sum.AssessmentsCreated)
	}
	if len(f.creates) != 1 {
		t.Fatalf("WorkCreate calls = %d, want 1", len(f.creates))
	}
	if got := f.creates[0]; got.kind != kindAssessment || got.title != "Outcome: Ship payments" {
		t.Errorf("WorkCreate = %+v, want kind=%s title=%q", got, kindAssessment, "Outcome: Ship payments")
	}
	if len(f.links) != 1 {
		t.Fatalf("Link calls = %d, want 1", len(f.links))
	}
	if got := f.links[0]; got.relType != relRelatesTo || got.target != "PROJ-1" {
		t.Errorf("Link = %+v, want relType=%s target=PROJ-1", got, relRelatesTo)
	}
}

func TestRunOnce_SkipsMilestoneWithExistingPairingByRelation(t *testing.T) {
	f := newFake()
	f.put(kindMilestone, statusShipped, mcp.WorkItem{ID: "wrk_1", SharedID: "PROJ-1", Kind: kindMilestone, Title: "Ship payments", Status: statusShipped})
	// An assessment already relates_to the milestone (by SharedID).
	f.put(kindAssessment, "", mcp.WorkItem{
		ID: "wrk_oa", Kind: kindAssessment, Title: "anything",
		Relations: []mcp.Relation{{Type: relRelatesTo, TargetWorkItem: "PROJ-1"}},
	})

	sum, err := newSched(f).RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if sum.AssessmentsCreated != 0 || len(f.creates) != 0 || len(f.links) != 0 {
		t.Fatalf("expected no creation; got created=%d creates=%d links=%d", sum.AssessmentsCreated, len(f.creates), len(f.links))
	}
}

func TestRunOnce_SkipsMilestoneWithExistingPairingByTitle(t *testing.T) {
	f := newFake()
	f.put(kindMilestone, statusShipped, mcp.WorkItem{ID: "wrk_1", SharedID: "PROJ-1", Kind: kindMilestone, Title: "Ship payments", Status: statusShipped})
	// An assessment matches by the title convention only (no relation).
	f.put(kindAssessment, "", mcp.WorkItem{ID: "wrk_oa", Kind: kindAssessment, Title: "Outcome: Ship payments"})

	sum, err := newSched(f).RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if sum.AssessmentsCreated != 0 || len(f.creates) != 0 {
		t.Fatalf("expected no creation; got created=%d creates=%d", sum.AssessmentsCreated, len(f.creates))
	}
}

// --- Action 2: past-SLA pending assessments ---

func TestRunOnce_MarksPastSLAAssessmentOverdue(t *testing.T) {
	f := newFake()
	// 100 days old pending assessment (> 90d SLA) and a fresh one (10d).
	f.put(kindAssessment, statusPending,
		mcp.WorkItem{ID: "wrk_old", SharedID: "OA-OLD", Kind: kindAssessment, Status: statusPending, CreatedAt: rfc(-100 * 24 * time.Hour)},
		mcp.WorkItem{ID: "wrk_new", SharedID: "OA-NEW", Kind: kindAssessment, Status: statusPending, CreatedAt: rfc(-10 * 24 * time.Hour)},
	)

	sum, err := newSched(f).RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if sum.MarkedOverdue != 1 {
		t.Fatalf("MarkedOverdue = %d, want 1", sum.MarkedOverdue)
	}
	if len(f.statuses) != 1 || f.statuses[0] != (statusCall{"OA-OLD", statusOverdue}) {
		t.Fatalf("SetStatus calls = %+v, want one {OA-OLD overdue}", f.statuses)
	}
}

func TestRunOnce_SkipsAssessmentWithUnparseableTimestamp(t *testing.T) {
	f := newFake()
	f.put(kindAssessment, statusPending,
		mcp.WorkItem{ID: "wrk_x", SharedID: "OA-X", Kind: kindAssessment, Status: statusPending, CreatedAt: ""},
		mcp.WorkItem{ID: "wrk_y", SharedID: "OA-Y", Kind: kindAssessment, Status: statusPending, CreatedAt: "not-a-time"},
	)
	sum, err := newSched(f).RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if sum.MarkedOverdue != 0 || len(f.statuses) != 0 {
		t.Fatalf("expected no overdue marking; got marked=%d statuses=%+v", sum.MarkedOverdue, f.statuses)
	}
}

// --- Action 3: idle open DecisionBlocks ---

func TestRunOnce_EscalatesIdleDecision(t *testing.T) {
	f := newFake()
	// 10 days idle (> 9d window) and a fresh 2-day one.
	f.put(kindDecision, statusOpen,
		mcp.WorkItem{ID: "wrk_d1", SharedID: "DB-1", Kind: kindDecision, Status: statusOpen, UpdatedAt: rfc(-10 * 24 * time.Hour)},
		mcp.WorkItem{ID: "wrk_d2", SharedID: "DB-2", Kind: kindDecision, Status: statusOpen, UpdatedAt: rfc(-2 * 24 * time.Hour)},
	)

	sum, err := newSched(f).RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if sum.Escalated != 1 {
		t.Fatalf("Escalated = %d, want 1", sum.Escalated)
	}
	if len(f.statuses) != 1 || f.statuses[0] != (statusCall{"DB-1", statusEscalated}) {
		t.Fatalf("SetStatus calls = %+v, want one {DB-1 escalated}", f.statuses)
	}
}

func TestRunOnce_DecisionIdleFallsBackToCreatedAt(t *testing.T) {
	f := newFake()
	// No UpdatedAt → idle measured from CreatedAt (12 days ago → escalate).
	f.put(kindDecision, statusOpen,
		mcp.WorkItem{ID: "wrk_d", SharedID: "DB-3", Kind: kindDecision, Status: statusOpen, CreatedAt: rfc(-12 * 24 * time.Hour)},
	)
	sum, err := newSched(f).RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if sum.Escalated != 1 || len(f.statuses) != 1 || f.statuses[0].status != statusEscalated {
		t.Fatalf("expected one escalation via CreatedAt fallback; got escalated=%d statuses=%+v", sum.Escalated, f.statuses)
	}
}

// --- Whole-cycle no-op + error path ---

func TestRunOnce_AllFreshIsNoOp(t *testing.T) {
	f := newFake()
	f.put(kindMilestone, statusShipped, mcp.WorkItem{ID: "wrk_1", SharedID: "PROJ-1", Kind: kindMilestone, Title: "Ship", Status: statusShipped})
	f.put(kindAssessment, "", mcp.WorkItem{ID: "wrk_oa", Kind: kindAssessment, Title: "Outcome: Ship",
		Relations: []mcp.Relation{{Type: relRelatesTo, TargetWorkItem: "PROJ-1"}}})
	f.put(kindAssessment, statusPending, mcp.WorkItem{ID: "wrk_p", SharedID: "OA-P", Kind: kindAssessment, Status: statusPending, CreatedAt: rfc(-5 * 24 * time.Hour)})
	f.put(kindDecision, statusOpen, mcp.WorkItem{ID: "wrk_d", SharedID: "DB-1", Kind: kindDecision, Status: statusOpen, UpdatedAt: rfc(-1 * 24 * time.Hour)})

	sum, err := newSched(f).RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if (sum != Summary{}) {
		t.Fatalf("expected zero Summary, got %+v", sum)
	}
	if len(f.creates)+len(f.links)+len(f.statuses) != 0 {
		t.Fatalf("expected no mutations; creates=%d links=%d statuses=%d", len(f.creates), len(f.links), len(f.statuses))
	}
}

func TestRunOnce_CreateErrorIsReported(t *testing.T) {
	f := newFake()
	f.failCreate = errors.New("boom")
	f.put(kindMilestone, statusShipped, mcp.WorkItem{ID: "wrk_1", SharedID: "PROJ-1", Kind: kindMilestone, Title: "Ship", Status: statusShipped})
	f.put(kindAssessment, "" /* none */)

	_, err := newSched(f).RunOnce(context.Background())
	if err == nil {
		t.Fatal("expected error from failing WorkCreate, got nil")
	}
}

// newSched builds a Scheduler over f with the clock pinned to base so the
// relative timestamps in each test are deterministic.
func newSched(f *fakeClient) *Scheduler {
	s := New(f, time.Minute)
	s.SetClock(fixedNow(base))
	return s
}
