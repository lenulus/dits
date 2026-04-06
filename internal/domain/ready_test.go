package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func readyWorkItem() *WorkItem {
	return &WorkItem{
		ID: "wrk_001", Kind: "task", Status: "open", Priority: "medium",
		Labels: []string{}, Assignees: []ActorID{}, Comments: []Comment{},
		Artifacts: []Artifact{}, Relations: []Relation{}, Checkpoints: []Checkpoint{},
		Observations: []Observation{}, Findings: []Finding{}, Evals: []Eval{},
		Outcomes: []Outcome{},
		Attempts: []ExecutionAttempt{},
	}
}

var openStatuses = []string{"open", "pending"}

func TestIsReady_OpenUnleasedUnblocked(t *testing.T) {
	wi := readyWorkItem()
	assert.True(t, IsReady(wi, openStatuses))
}

func TestIsReady_Blocked(t *testing.T) {
	wi := readyWorkItem()
	wi.Blocked = true
	assert.False(t, IsReady(wi, openStatuses))
}

func TestIsReady_Leased(t *testing.T) {
	wi := readyWorkItem()
	a := ActorID("actor_a")
	wi.LeaseHolder = &a
	assert.False(t, IsReady(wi, openStatuses))
}

func TestIsReady_RunningAttempt(t *testing.T) {
	wi := readyWorkItem()
	wi.Attempts = []ExecutionAttempt{
		{AttemptID: "atp_001", Status: "running", Authoritative: true},
	}
	assert.False(t, IsReady(wi, openStatuses))
}

func TestIsReady_CompletedAttemptOK(t *testing.T) {
	wi := readyWorkItem()
	wi.Attempts = []ExecutionAttempt{
		{AttemptID: "atp_001", Status: "completed", Authoritative: true},
	}
	assert.True(t, IsReady(wi, openStatuses))
}

func TestIsReady_ClosedStatusNotReady(t *testing.T) {
	wi := readyWorkItem()
	wi.Status = "closed"
	assert.False(t, IsReady(wi, openStatuses))
}

func TestIsReady_NilWorkItem(t *testing.T) {
	assert.False(t, IsReady(nil, openStatuses))
}

func TestIsReady_NonAuthoritativeRunningOK(t *testing.T) {
	// Non-authoritative running attempt should not block readiness.
	wi := readyWorkItem()
	wi.Attempts = []ExecutionAttempt{
		{AttemptID: "atp_001", Status: "running", Authoritative: false},
	}
	assert.True(t, IsReady(wi, openStatuses))
}
