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
	wi.LeaseGeneration = 1
	e := Event{Type: EventWorkLeaseRenewed, ActorID: "actor_a",
		Payload: MustMarshalPayload(LeaseRenewedPayload{LeaseID: "lea_001", Generation: 2})}
	assert.NoError(t, ValidateProtocol(e, wi))
}

func TestProtocol_LeaseRenewStaleGeneration(t *testing.T) {
	wi := baseWorkItem()
	wi.LeaseHolder = leaseHolder("actor_a")
	wi.LeaseGeneration = 3
	e := Event{Type: EventWorkLeaseRenewed, ActorID: "actor_a",
		Payload: MustMarshalPayload(LeaseRenewedPayload{LeaseID: "lea_001", Generation: 2})}
	err := ValidateProtocol(e, wi)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not greater than current")
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

// --- Reference integrity ---

func TestProtocol_FindingRetractedExists(t *testing.T) {
	wi := baseWorkItem()
	wi.Findings = []Finding{{EventID: "evt_f1", Statement: "leak"}}
	e := Event{Type: EventWorkFindingRetracted,
		Payload: MustMarshalPayload(FindingRetractedPayload{OriginalEventID: "evt_f1"})}
	assert.NoError(t, ValidateProtocol(e, wi))
}

func TestProtocol_FindingRetractedNotFound(t *testing.T) {
	wi := baseWorkItem()
	e := Event{Type: EventWorkFindingRetracted,
		Payload: MustMarshalPayload(FindingRetractedPayload{OriginalEventID: "evt_nonexistent"})}
	err := ValidateProtocol(e, wi)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found in findings")
}

func TestProtocol_OutcomeRetainedExistingAttempt(t *testing.T) {
	wi := baseWorkItem()
	wi.Attempts = []ExecutionAttempt{{AttemptID: "atp_001", Status: "completed", Authoritative: true}}
	e := Event{Type: EventWorkOutcomeRetained,
		Payload: MustMarshalPayload(OutcomeRetainedPayload{SubjectKind: "attempt", SubjectRef: "atp_001"})}
	assert.NoError(t, ValidateProtocol(e, wi))
}

func TestProtocol_OutcomeRetainedNonexistentAttempt(t *testing.T) {
	wi := baseWorkItem()
	e := Event{Type: EventWorkOutcomeRetained,
		Payload: MustMarshalPayload(OutcomeRetainedPayload{SubjectKind: "attempt", SubjectRef: "atp_nonexistent"})}
	err := ValidateProtocol(e, wi)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found in work item")
}

func TestProtocol_OutcomeDiscardedExistingArtifact(t *testing.T) {
	wi := baseWorkItem()
	wi.Artifacts = []Artifact{{ID: "art_001", ContentHash: "sha256:abc"}}
	e := Event{Type: EventWorkOutcomeDiscarded,
		Payload: MustMarshalPayload(OutcomeDiscardedPayload{SubjectKind: "artifact", SubjectRef: "sha256:abc"})}
	assert.NoError(t, ValidateProtocol(e, wi))
}

func TestProtocol_OutcomeDiscardedNonexistent(t *testing.T) {
	wi := baseWorkItem()
	e := Event{Type: EventWorkOutcomeDiscarded,
		Payload: MustMarshalPayload(OutcomeDiscardedPayload{SubjectKind: "artifact", SubjectRef: "sha256:nonexistent"})}
	err := ValidateProtocol(e, wi)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found in work item")
}

// --- Lease release identity ---

func TestProtocol_LeaseReleaseMatchingID(t *testing.T) {
	wi := baseWorkItem()
	wi.LeaseHolder = leaseHolder("actor_a")
	lid := LeaseID("lea_001")
	wi.LeaseID = &lid
	e := Event{Type: EventWorkLeaseReleased, ActorID: "actor_a",
		Payload: MustMarshalPayload(LeaseReleasedPayload{LeaseID: "lea_001"})}
	assert.NoError(t, ValidateProtocol(e, wi))
}

func TestProtocol_LeaseReleaseMismatchedID(t *testing.T) {
	wi := baseWorkItem()
	wi.LeaseHolder = leaseHolder("actor_a")
	lid := LeaseID("lea_001")
	wi.LeaseID = &lid
	e := Event{Type: EventWorkLeaseReleased, ActorID: "actor_a",
		Payload: MustMarshalPayload(LeaseReleasedPayload{LeaseID: "lea_999"})}
	err := ValidateProtocolFull(e, wi, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not match active lease")
}

// --- Event-level reference integrity ---

func makeLookup(keys ...string) EventLookup {
	m := make(map[string]bool)
	for _, k := range keys {
		m[k] = true
	}
	return func(key string) bool { return m[key] }
}

func TestProtocol_PlanAcceptedWithProposal(t *testing.T) {
	wi := baseWorkItem()
	lookup := makeLookup(EventLookupKey(EventWorkPlanProposed, "evt_plan1"))
	e := Event{Type: EventWorkPlanAccepted,
		Payload: MustMarshalPayload(PlanAcceptedPayload{PlanEventID: "evt_plan1"})}
	assert.NoError(t, ValidateProtocolFull(e, wi, lookup))
}

func TestProtocol_PlanAcceptedWithoutProposal(t *testing.T) {
	wi := baseWorkItem()
	lookup := makeLookup() // empty — no prior events
	e := Event{Type: EventWorkPlanAccepted,
		Payload: MustMarshalPayload(PlanAcceptedPayload{PlanEventID: "evt_plan1"})}
	err := ValidateProtocolFull(e, wi, lookup)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "plan_proposed event")
}

func TestProtocol_PlanRejectedWithoutProposal(t *testing.T) {
	wi := baseWorkItem()
	lookup := makeLookup()
	e := Event{Type: EventWorkPlanRejected,
		Payload: MustMarshalPayload(PlanRejectedPayload{PlanEventID: "evt_plan1"})}
	err := ValidateProtocolFull(e, wi, lookup)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "plan_proposed event")
}

func TestProtocol_ReviewCompletedWithRequest(t *testing.T) {
	wi := baseWorkItem()
	lookup := makeLookup(EventLookupKey(EventWorkReviewRequested, "rev_001"))
	e := Event{Type: EventWorkReviewCompleted,
		Payload: MustMarshalPayload(ReviewCompletedPayload{ReviewID: "rev_001", Verdict: "approve"})}
	assert.NoError(t, ValidateProtocolFull(e, wi, lookup))
}

func TestProtocol_ReviewCompletedWithoutRequest(t *testing.T) {
	wi := baseWorkItem()
	lookup := makeLookup()
	e := Event{Type: EventWorkReviewCompleted,
		Payload: MustMarshalPayload(ReviewCompletedPayload{ReviewID: "rev_001", Verdict: "approve"})}
	err := ValidateProtocolFull(e, wi, lookup)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "review_requested")
}

func TestProtocol_EvalCompletedWithRequest(t *testing.T) {
	wi := baseWorkItem()
	lookup := makeLookup(EventLookupKey(EventWorkEvalRequested, "evl_001"))
	e := Event{Type: EventWorkEvalCompleted,
		Payload: MustMarshalPayload(EvalCompletedPayload{EvalID: "evl_001", Verdict: "pass"})}
	assert.NoError(t, ValidateProtocolFull(e, wi, lookup))
}

func TestProtocol_EvalCompletedWithoutRequest(t *testing.T) {
	wi := baseWorkItem()
	lookup := makeLookup()
	e := Event{Type: EventWorkEvalCompleted,
		Payload: MustMarshalPayload(EvalCompletedPayload{EvalID: "evl_001", Verdict: "pass"})}
	err := ValidateProtocolFull(e, wi, lookup)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "eval_requested")
}

func TestProtocol_HandoffAcceptedWithHandoff(t *testing.T) {
	wi := baseWorkItem()
	lookup := makeLookup(EventLookupKey(EventWorkHandedOff, "hof_001"))
	e := Event{Type: EventWorkHandoffAccepted,
		Payload: MustMarshalPayload(HandoffAcceptedPayload{HandoffID: "hof_001"})}
	assert.NoError(t, ValidateProtocolFull(e, wi, lookup))
}

func TestProtocol_HandoffAcceptedWithoutHandoff(t *testing.T) {
	wi := baseWorkItem()
	lookup := makeLookup()
	e := Event{Type: EventWorkHandoffAccepted,
		Payload: MustMarshalPayload(HandoffAcceptedPayload{HandoffID: "hof_001"})}
	err := ValidateProtocolFull(e, wi, lookup)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "handed_off")
}

func TestProtocol_HandoffRejectedWithoutHandoff(t *testing.T) {
	wi := baseWorkItem()
	lookup := makeLookup()
	e := Event{Type: EventWorkHandoffRejected,
		Payload: MustMarshalPayload(HandoffRejectedPayload{HandoffID: "hof_001"})}
	err := ValidateProtocolFull(e, wi, lookup)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "handed_off")
}

func TestProtocol_RefCheckSkippedWithNilLookup(t *testing.T) {
	// Without a lookup function, reference checks are skipped (backward compat).
	wi := baseWorkItem()
	e := Event{Type: EventWorkPlanAccepted,
		Payload: MustMarshalPayload(PlanAcceptedPayload{PlanEventID: "evt_nonexistent"})}
	assert.NoError(t, ValidateProtocol(e, wi)) // no error — lookup is nil
}

// --- Passthrough events ---

func TestProtocol_LifecycleEventsPass(t *testing.T) {
	wi := baseWorkItem()
	for _, et := range []EventType{EventWorkTitleSet, EventWorkBodySet, EventWorkCommented, EventWorkClosed, EventWorkReopened, EventWorkLabelAdded, EventWorkAssigned} {
		e := Event{Type: et, Payload: MustMarshalPayload(struct{}{})}
		assert.NoError(t, ValidateProtocol(e, wi), "event type %s should pass", et)
	}
}
