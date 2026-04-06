package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func leaseHolder(id ActorID) *ActorID { return &id }

func baseWorkItem() *WorkItem {
	return &WorkItem{
		ID: "wrk_001", Kind: "execution", Status: "open",
		Labels: []string{}, Assignees: []ActorID{}, Comments: []Comment{},
		Artifacts: []Artifact{}, Relations: []Relation{}, Checkpoints: []Checkpoint{},
		Observations: []Observation{}, Findings: []Finding{}, Attempts: []ExecutionAttempt{},
		Evals: []Eval{}, Outcomes: []Outcome{},
	}
}

// --- Lease ---

func TestProtocol_LeaseWhenUnleased(t *testing.T) {
	wi := baseWorkItem()
	e := Event{Type: EventWorkLeased, ActorID: "actor_a",
		Payload: MustMarshalPayload(LeasedPayload{LeaseID: "lea_001"})}
	assert.NoError(t, ValidateProtocol(e, wi))
}

func TestProtocol_LeaseWhenAlreadyLeased(t *testing.T) {
	wi := baseWorkItem()
	wi.LeaseHolder = leaseHolder("actor_b")
	e := Event{Type: EventWorkLeased, ActorID: "actor_a",
		Payload: MustMarshalPayload(LeasedPayload{LeaseID: "lea_002"})}
	err := ValidateProtocol(e, wi)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already leased")
}

// --- Lease Release ---

func TestProtocol_LeaseReleaseByHolder(t *testing.T) {
	wi := baseWorkItem()
	wi.LeaseHolder = leaseHolder("actor_a")
	e := Event{Type: EventWorkLeaseReleased, ActorID: "actor_a",
		Payload: MustMarshalPayload(LeaseReleasedPayload{LeaseID: "lea_001"})}
	assert.NoError(t, ValidateProtocol(e, wi))
}

func TestProtocol_LeaseReleaseByNonHolder(t *testing.T) {
	wi := baseWorkItem()
	wi.LeaseHolder = leaseHolder("actor_a")
	e := Event{Type: EventWorkLeaseReleased, ActorID: "actor_b",
		Payload: MustMarshalPayload(LeaseReleasedPayload{LeaseID: "lea_001"})}
	err := ValidateProtocol(e, wi)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not lease holder")
}

func TestProtocol_LeaseReleaseNoLease(t *testing.T) {
	wi := baseWorkItem()
	e := Event{Type: EventWorkLeaseReleased, ActorID: "actor_a",
		Payload: MustMarshalPayload(LeaseReleasedPayload{LeaseID: "lea_001"})}
	err := ValidateProtocol(e, wi)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no active lease")
}

// --- Lease Renewal ---

func TestProtocol_LeaseRenewByHolder(t *testing.T) {
	wi := baseWorkItem()
	wi.LeaseHolder = leaseHolder("actor_a")
	e := Event{Type: EventWorkLeaseRenewed, ActorID: "actor_a",
		Payload: MustMarshalPayload(LeaseRenewedPayload{LeaseID: "lea_001"})}
	assert.NoError(t, ValidateProtocol(e, wi))
}

func TestProtocol_LeaseRenewByNonHolder(t *testing.T) {
	wi := baseWorkItem()
	wi.LeaseHolder = leaseHolder("actor_a")
	e := Event{Type: EventWorkLeaseRenewed, ActorID: "actor_b",
		Payload: MustMarshalPayload(LeaseRenewedPayload{LeaseID: "lea_001"})}
	err := ValidateProtocol(e, wi)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not lease holder")
}

// --- Execution Start ---

func TestProtocol_ExecutionStartWithLease(t *testing.T) {
	wi := baseWorkItem()
	wi.LeaseHolder = leaseHolder("actor_a")
	e := Event{Type: EventWorkExecutionStarted, ActorID: "actor_a",
		Payload: MustMarshalPayload(ExecutionStartedPayload{AttemptID: "atp_001"})}
	assert.NoError(t, ValidateProtocol(e, wi))
}

func TestProtocol_ExecutionStartNoLease(t *testing.T) {
	wi := baseWorkItem()
	e := Event{Type: EventWorkExecutionStarted, ActorID: "actor_a",
		Payload: MustMarshalPayload(ExecutionStartedPayload{AttemptID: "atp_001"})}
	err := ValidateProtocol(e, wi)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no active lease")
}

func TestProtocol_ExecutionStartByNonHolder(t *testing.T) {
	wi := baseWorkItem()
	wi.LeaseHolder = leaseHolder("actor_a")
	e := Event{Type: EventWorkExecutionStarted, ActorID: "actor_b",
		Payload: MustMarshalPayload(ExecutionStartedPayload{AttemptID: "atp_001"})}
	err := ValidateProtocol(e, wi)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not lease holder")
}

func TestProtocol_ExecutionStartWithRunningAttempt(t *testing.T) {
	wi := baseWorkItem()
	wi.LeaseHolder = leaseHolder("actor_a")
	atp := AttemptID("atp_001")
	wi.CurrentAttempt = &atp
	wi.Attempts = []ExecutionAttempt{
		{AttemptID: "atp_001", Status: "running", Authoritative: true, ActorID: "actor_a"},
	}
	e := Event{Type: EventWorkExecutionStarted, ActorID: "actor_a",
		Payload: MustMarshalPayload(ExecutionStartedPayload{AttemptID: "atp_002"})}
	err := ValidateProtocol(e, wi)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already running")
}

// --- Execution Complete ---

func TestProtocol_ExecutionCompleteByHolder(t *testing.T) {
	wi := baseWorkItem()
	wi.LeaseHolder = leaseHolder("actor_a")
	atp := AttemptID("atp_001")
	wi.CurrentAttempt = &atp
	wi.Attempts = []ExecutionAttempt{
		{AttemptID: "atp_001", Status: "running", Authoritative: true, ActorID: "actor_a"},
	}
	e := Event{Type: EventWorkExecutionCompleted, ActorID: "actor_a",
		Payload: MustMarshalPayload(ExecutionCompletedPayload{AttemptID: "atp_001"})}
	assert.NoError(t, ValidateProtocol(e, wi))
}

func TestProtocol_ExecutionCompleteByNonHolder(t *testing.T) {
	wi := baseWorkItem()
	wi.LeaseHolder = leaseHolder("actor_a")
	atp := AttemptID("atp_001")
	wi.CurrentAttempt = &atp
	wi.Attempts = []ExecutionAttempt{
		{AttemptID: "atp_001", Status: "running", Authoritative: true, ActorID: "actor_a"},
	}
	e := Event{Type: EventWorkExecutionCompleted, ActorID: "actor_b",
		Payload: MustMarshalPayload(ExecutionCompletedPayload{AttemptID: "atp_001"})}
	err := ValidateProtocol(e, wi)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not lease holder")
}

func TestProtocol_ExecutionCompleteNoAttempt(t *testing.T) {
	wi := baseWorkItem()
	wi.LeaseHolder = leaseHolder("actor_a")
	e := Event{Type: EventWorkExecutionCompleted, ActorID: "actor_a",
		Payload: MustMarshalPayload(ExecutionCompletedPayload{AttemptID: "atp_001"})}
	err := ValidateProtocol(e, wi)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no active attempt")
}

func TestProtocol_ExecutionCompleteNonRunning(t *testing.T) {
	wi := baseWorkItem()
	wi.LeaseHolder = leaseHolder("actor_a")
	atp := AttemptID("atp_001")
	wi.CurrentAttempt = &atp
	wi.Attempts = []ExecutionAttempt{
		{AttemptID: "atp_001", Status: "completed", Authoritative: true, ActorID: "actor_a"},
	}
	e := Event{Type: EventWorkExecutionCompleted, ActorID: "actor_a",
		Payload: MustMarshalPayload(ExecutionCompletedPayload{AttemptID: "atp_001"})}
	err := ValidateProtocol(e, wi)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not running")
}

// --- Execution Failed ---

func TestProtocol_ExecutionFailByHolder(t *testing.T) {
	wi := baseWorkItem()
	wi.LeaseHolder = leaseHolder("actor_a")
	atp := AttemptID("atp_001")
	wi.CurrentAttempt = &atp
	wi.Attempts = []ExecutionAttempt{
		{AttemptID: "atp_001", Status: "running", Authoritative: true, ActorID: "actor_a"},
	}
	e := Event{Type: EventWorkExecutionFailed, ActorID: "actor_a",
		Payload: MustMarshalPayload(ExecutionFailedPayload{AttemptID: "atp_001", Error: "boom"})}
	assert.NoError(t, ValidateProtocol(e, wi))
}

// --- Checkpoint ---

func TestProtocol_CheckpointByHolder(t *testing.T) {
	wi := baseWorkItem()
	wi.LeaseHolder = leaseHolder("actor_a")
	atp := AttemptID("atp_001")
	wi.CurrentAttempt = &atp
	e := Event{Type: EventWorkCheckpointed, ActorID: "actor_a",
		Payload: MustMarshalPayload(CheckpointedPayload{AttemptID: "atp_001"})}
	assert.NoError(t, ValidateProtocol(e, wi))
}

func TestProtocol_CheckpointByNonHolder(t *testing.T) {
	wi := baseWorkItem()
	wi.LeaseHolder = leaseHolder("actor_a")
	atp := AttemptID("atp_001")
	wi.CurrentAttempt = &atp
	e := Event{Type: EventWorkCheckpointed, ActorID: "actor_b",
		Payload: MustMarshalPayload(CheckpointedPayload{AttemptID: "atp_001"})}
	err := ValidateProtocol(e, wi)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not lease holder")
}

func TestProtocol_CheckpointNoAttempt(t *testing.T) {
	wi := baseWorkItem()
	wi.LeaseHolder = leaseHolder("actor_a")
	e := Event{Type: EventWorkCheckpointed, ActorID: "actor_a",
		Payload: MustMarshalPayload(CheckpointedPayload{AttemptID: "atp_001"})}
	err := ValidateProtocol(e, wi)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no active attempt")
}

// --- Block / Unblock ---

func TestProtocol_BlockWhenNotBlocked(t *testing.T) {
	wi := baseWorkItem()
	e := Event{Type: EventWorkBlocked, Payload: MustMarshalPayload(BlockedPayload{Reason: "dep"})}
	assert.NoError(t, ValidateProtocol(e, wi))
}

func TestProtocol_BlockWhenAlreadyBlocked(t *testing.T) {
	wi := baseWorkItem()
	wi.Blocked = true
	e := Event{Type: EventWorkBlocked, Payload: MustMarshalPayload(BlockedPayload{Reason: "dep"})}
	err := ValidateProtocol(e, wi)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already blocked")
}

func TestProtocol_UnblockWhenBlocked(t *testing.T) {
	wi := baseWorkItem()
	wi.Blocked = true
	e := Event{Type: EventWorkUnblocked, Payload: MustMarshalPayload(UnblockedPayload{Reason: "done"})}
	assert.NoError(t, ValidateProtocol(e, wi))
}

func TestProtocol_UnblockWhenNotBlocked(t *testing.T) {
	wi := baseWorkItem()
	e := Event{Type: EventWorkUnblocked, Payload: MustMarshalPayload(UnblockedPayload{Reason: "done"})}
	err := ValidateProtocol(e, wi)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not blocked")
}

// --- Created ---

func TestProtocol_CreatedNilState(t *testing.T) {
	e := Event{Type: EventWorkCreated,
		Payload: MustMarshalPayload(WorkCreatedPayload{Title: "Test", Kind: "task"})}
	assert.NoError(t, ValidateProtocol(e, nil))
}

// --- Passthrough events ---

func TestProtocol_LifecycleEventsPass(t *testing.T) {
	wi := baseWorkItem()
	for _, et := range []EventType{EventWorkTitleSet, EventWorkBodySet, EventWorkCommented, EventWorkClosed, EventWorkReopened, EventWorkLabelAdded, EventWorkAssigned} {
		e := Event{Type: et, Payload: MustMarshalPayload(struct{}{})}
		assert.NoError(t, ValidateProtocol(e, wi), "event type %s should pass", et)
	}
}
