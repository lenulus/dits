package domain

import (
	"encoding/json"
	"time"
)

type EventType string

// Event Classification:
//
// Authoritative control — operational state transitions requiring protocol validation:
//   lease, lease_released, lease_renewed, execution_started/completed/failed/abandoned,
//   blocked, unblocked
//
// Decision/gating — selection events that reference prior proposals or requests:
//   review_requested/completed, eval_requested/completed, plan_accepted/rejected,
//   handoff_accepted/rejected, outcome_retained/discarded
//
// Observational — evidence and progress records:
//   observation_recorded, finding_recorded/retracted, checkpointed, progress_reported,
//   evidence_attached
//
// Lifecycle/projection — human-facing state:
//   created, title_set, body_set, status_set, priority_set, label_added/removed,
//   assigned/unassigned, commented, closed, reopened

// --- Lifecycle Events ---

const (
	EventWorkCreated         EventType = "work.created"
	EventWorkTitleSet        EventType = "work.title_set"
	EventWorkBodySet         EventType = "work.body_set"
	EventWorkStatusSet       EventType = "work.status_set"
	EventWorkPrioritySet     EventType = "work.priority_set"
	EventWorkLabelAdded      EventType = "work.label_added"
	EventWorkLabelRemoved    EventType = "work.label_removed"
	EventWorkAssigned        EventType = "work.assigned"
	EventWorkUnassigned      EventType = "work.unassigned"
	EventWorkCommented       EventType = "work.commented"
	EventWorkClosed   EventType = "work.closed"
	EventWorkReopened EventType = "work.reopened"
)

// --- Execution / Ownership Events ---

const (
	EventWorkLeased             EventType = "work.leased"
	EventWorkLeaseReleased      EventType = "work.lease_released"
	EventWorkLeaseRenewed       EventType = "work.lease_renewed"
	EventWorkExecutionStarted   EventType = "work.execution_started"
	EventWorkExecutionCompleted EventType = "work.execution_completed"
	EventWorkExecutionFailed    EventType = "work.execution_failed"
	EventWorkExecutionAbandoned EventType = "work.execution_abandoned"
)

// --- Checkpoint / Progress Events ---

const (
	EventWorkCheckpointed     EventType = "work.checkpointed"
	EventWorkProgressReported EventType = "work.progress_reported"
	EventWorkBlocked          EventType = "work.blocked"
	EventWorkUnblocked        EventType = "work.unblocked"
)

// --- Evidence / Observation Events ---

const (
	EventWorkObservationRecorded EventType = "work.observation_recorded"
	EventWorkEvidenceAttached    EventType = "work.evidence_attached"
	EventWorkFindingRecorded     EventType = "work.finding_recorded"
	EventWorkFindingRetracted    EventType = "work.finding_retracted"
)

// --- Planning / Decision Events ---

const (
	EventWorkPlanProposed     EventType = "work.plan_proposed"
	EventWorkPlanAccepted     EventType = "work.plan_accepted"
	EventWorkPlanRejected     EventType = "work.plan_rejected"
	EventWorkStepAdded        EventType = "work.step_added"
	EventWorkDecisionRecorded EventType = "work.decision_recorded"
)

// --- Handoff / Review Events ---

const (
	EventWorkReviewRequested  EventType = "work.review_requested"
	EventWorkReviewCompleted  EventType = "work.review_completed"
	EventWorkHandedOff        EventType = "work.handed_off"
	EventWorkHandoffAccepted  EventType = "work.handoff_accepted"
	EventWorkHandoffRejected  EventType = "work.handoff_rejected"
)

// --- Eval Events ---

const (
	EventWorkEvalRequested EventType = "work.eval_requested"
	EventWorkEvalCompleted EventType = "work.eval_completed"
)

// --- Outcome Events ---

const (
	EventWorkOutcomeRetained  EventType = "work.outcome_retained"
	EventWorkOutcomeDiscarded EventType = "work.outcome_discarded"
)

// --- Relation / Artifact Events ---

const (
	EventWorkLinked          EventType = "work.linked"
	EventWorkUnlinked        EventType = "work.unlinked"
	EventWorkArtifactAdded   EventType = "work.artifact_added"
	EventWorkArtifactRemoved EventType = "work.artifact_removed"
)

// --- RE Substrate (generic): classification & role bindings ---

const (
	EventWorkClassified   EventType = "work.classified"
	EventWorkDeclassified EventType = "work.declassified"
	EventWorkRoleBound    EventType = "work.role_bound"
	EventWorkRoleUnbound  EventType = "work.role_unbound"
)

// --- ACK lifecycle (generic two-party commitment) ---

const (
	EventWorkAckFiled    EventType = "work.ack_filed"
	EventWorkAckAccepted EventType = "work.ack_accepted"
	EventWorkAckRejected EventType = "work.ack_rejected"
	EventWorkAckCleared  EventType = "work.ack_cleared"
	EventWorkAckAmended  EventType = "work.ack_amended"
)

// Event is an immutable record of a state change in the system.
type Event struct {
	ID             EventID         `json:"id"`
	WorkItemID     WorkItemID      `json:"work_item_id"`
	Type           EventType       `json:"type"`
	ParentEventIDs []EventID       `json:"parent_event_ids"`
	MetaVersion    MetaVersion     `json:"meta_version"`
	ActorID        ActorID         `json:"actor_id"`
	Timestamp      time.Time       `json:"timestamp"`
	Payload        json.RawMessage `json:"payload"`
	EmittedBy      *EmittedBy      `json:"emitted_by,omitempty"`
	Signature      []byte          `json:"signature,omitempty"`
}

// --- Lifecycle Payloads ---

type WorkCreatedPayload struct {
	Title     string `json:"title"`
	Body      string `json:"body,omitempty"`
	Kind      string `json:"kind"`
	SchemaRef string `json:"schema_ref,omitempty"`
}

type TitleSetPayload struct {
	Title string `json:"title"`
}

type BodySetPayload struct {
	Body string `json:"body"`
}

type StatusSetPayload struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type PrioritySetPayload struct {
	Priority string `json:"priority"`
}

type LabelPayload struct {
	LabelSlug string `json:"label_slug"`
}

type AssignPayload struct {
	Assignee ActorID `json:"assignee"`
}

type CommentPayload struct {
	Body       string      `json:"body"`
	ProducedBy *ProducedBy `json:"produced_by,omitempty"`
}

type ClosedPayload struct {
	Reason string `json:"reason,omitempty"`
}

// --- Execution / Ownership Payloads ---

type LeasedPayload struct {
	LeaseID          LeaseID   `json:"lease_id"`
	LeaseDurationSec int       `json:"lease_duration_secs"`
	LeaseExpiresAt   time.Time `json:"lease_expires_at"`
	Generation       uint64    `json:"generation"`
}

type LeaseReleasedPayload struct {
	LeaseID LeaseID `json:"lease_id"`
	Reason  string  `json:"reason"`
}

type LeaseRenewedPayload struct {
	LeaseID        LeaseID   `json:"lease_id"`
	LeaseExpiresAt time.Time `json:"lease_expires_at"`
	Generation     uint64    `json:"generation"`
}

type ExecutionStartedPayload struct {
	AttemptID AttemptID `json:"attempt_id"`
	PlanRef   string    `json:"plan_ref,omitempty"`
}

type ExecutionCompletedPayload struct {
	AttemptID          AttemptID `json:"attempt_id"`
	Summary            string    `json:"summary"`
	OutputArtifactRefs []string  `json:"output_artifact_refs,omitempty"`
}

type ExecutionFailedPayload struct {
	AttemptID          AttemptID `json:"attempt_id"`
	Error              string    `json:"error"`
	Retryable          bool      `json:"retryable"`
	OutputArtifactRefs []string  `json:"output_artifact_refs,omitempty"`
}

type ExecutionAbandonedPayload struct {
	AttemptID AttemptID `json:"attempt_id"`
	Reason    string    `json:"reason"`
}

// --- Checkpoint / Progress Payloads ---

type CheckpointedPayload struct {
	AttemptID AttemptID       `json:"attempt_id"`
	Summary   string          `json:"summary"`
	Progress  float64         `json:"progress"`
	NextStep  string          `json:"next_step,omitempty"`
	Data      json.RawMessage `json:"data,omitempty"`
}

type ProgressReportedPayload struct {
	AttemptID AttemptID `json:"attempt_id"`
	Progress  float64   `json:"progress"`
	Message   string    `json:"message"`
}

type BlockedPayload struct {
	Reason       string `json:"reason"`
	BlockedByRef string `json:"blocked_by_ref,omitempty"`
}

type UnblockedPayload struct {
	Reason string `json:"reason"`
}

// --- Evidence / Observation Payloads ---

type ObservationRecordedPayload struct {
	Summary    string          `json:"summary"`
	Data       json.RawMessage `json:"data,omitempty"`
	ProducedBy *ProducedBy     `json:"produced_by,omitempty"`
}

type EvidenceAttachedPayload struct {
	ArtifactID   ArtifactID  `json:"artifact_id"`
	ContentHash  string      `json:"content_hash"`
	Filename     string      `json:"filename"`
	MimeType     string      `json:"mime_type"`
	SizeBytes    int64       `json:"size_bytes"`
	ArtifactType string      `json:"artifact_type"`
	SemanticRole string      `json:"semantic_role"`
	ProducedBy   *ProducedBy `json:"produced_by,omitempty"`
}

type FindingRecordedPayload struct {
	Statement    string      `json:"statement"`
	Confidence   float64     `json:"confidence"`
	Source       string      `json:"source,omitempty"`
	EvidenceRefs []string    `json:"evidence_refs,omitempty"`
	ProducedBy   *ProducedBy `json:"produced_by,omitempty"`
}

type FindingRetractedPayload struct {
	OriginalEventID EventID `json:"original_event_id"`
	Reason          string  `json:"reason"`
}

// --- Planning / Decision Payloads ---

type PlanProposedPayload struct {
	Plan       string      `json:"plan"`
	Summary    string      `json:"summary"`
	ProducedBy *ProducedBy `json:"produced_by,omitempty"`
}

type PlanAcceptedPayload struct {
	PlanEventID EventID `json:"plan_event_id"`
	Comment     string  `json:"comment,omitempty"`
}

type PlanRejectedPayload struct {
	PlanEventID EventID `json:"plan_event_id"`
	Reason      string  `json:"reason"`
}

type StepAddedPayload struct {
	StepIndex   int      `json:"step_index"`
	Description string   `json:"description"`
	DependsOn   []string `json:"depends_on,omitempty"`
}

type DecisionRecordedPayload struct {
	Decision     string      `json:"decision"`
	Rationale    string      `json:"rationale"`
	Alternatives []string    `json:"alternatives,omitempty"`
	ProducedBy   *ProducedBy `json:"produced_by,omitempty"`
}

// --- Handoff / Review Payloads ---

type ReviewRequestedPayload struct {
	ReviewID     ReviewID `json:"review_id"`
	Reviewer     ActorID  `json:"reviewer,omitempty"`
	Scope        string   `json:"scope"`
	ArtifactRefs []string `json:"artifact_refs,omitempty"`
}

type ReviewCompletedPayload struct {
	ReviewID   ReviewID    `json:"review_id"`
	Verdict    string      `json:"verdict"` // approve, request_changes, reject
	Comment    string      `json:"comment,omitempty"`
	ProducedBy *ProducedBy `json:"produced_by,omitempty"`
}

type HandedOffPayload struct {
	HandoffID    HandoffID `json:"handoff_id"`
	From         ActorID   `json:"from"`
	To           ActorID   `json:"to"`
	Context      string    `json:"context"`
	ArtifactRefs []string  `json:"artifact_refs,omitempty"`
}

type HandoffAcceptedPayload struct {
	HandoffID HandoffID `json:"handoff_id"`
	Comment   string    `json:"comment,omitempty"`
}

type HandoffRejectedPayload struct {
	HandoffID HandoffID `json:"handoff_id"`
	Reason    string    `json:"reason"`
}

// --- Eval Payloads ---

type EvalRequestedPayload struct {
	EvalID      EvalID `json:"eval_id"`
	SubjectKind string `json:"subject_kind,omitempty"` // work_item, artifact, attempt, finding, plan
	SubjectRef  string `json:"subject_ref"`
	RubricRef   string `json:"rubric_ref,omitempty"`
	Scope       string `json:"scope"`
}

type EvalCompletedPayload struct {
	EvalID      EvalID          `json:"eval_id"`
	SubjectKind string          `json:"subject_kind,omitempty"`
	SubjectRef  string          `json:"subject_ref"`
	RubricRef   string          `json:"rubric_ref,omitempty"`
	Summary     string          `json:"summary,omitempty"`
	Metrics     json.RawMessage `json:"metrics,omitempty"`
	Verdict     string          `json:"verdict"` // pass, fail, partial, etc.
	ProducedBy  *ProducedBy     `json:"produced_by,omitempty"`
}

// --- Outcome Payloads ---

type OutcomeRetainedPayload struct {
	SubjectKind string `json:"subject_kind"` // attempt, artifact
	SubjectRef  string `json:"subject_ref"`
	Reason      string `json:"reason,omitempty"`
	EvalRef     string `json:"eval_ref,omitempty"` // eval ID that informed this decision
}

type OutcomeDiscardedPayload struct {
	SubjectKind string `json:"subject_kind"`
	SubjectRef  string `json:"subject_ref"`
	Reason      string `json:"reason,omitempty"`
}

// --- Relation / Artifact Payloads ---

type RelationPayload struct {
	RelationType   string     `json:"relation_type"`
	TargetWorkItem WorkItemID `json:"target_work_item"`
}

type ArtifactAddedPayload struct {
	ArtifactID   ArtifactID  `json:"artifact_id"`
	ContentHash  string      `json:"content_hash"`
	Filename     string      `json:"filename"`
	MimeType     string      `json:"mime_type"`
	SizeBytes    int64       `json:"size_bytes"`
	ArtifactType string      `json:"artifact_type,omitempty"`
	SemanticRole string      `json:"semantic_role,omitempty"`
	ProducedBy   *ProducedBy `json:"produced_by,omitempty"`
}

type ArtifactRemovedPayload struct {
	ArtifactID ArtifactID `json:"artifact_id"`
}

// --- RE Substrate Payloads ---

// ClassificationPayload is shared by classified & declassified, mirroring how
// RelationPayload serves both linked & unlinked.
type ClassificationPayload struct {
	TaxonomySlug string `json:"taxonomy_slug"`
	NodeSlug     string `json:"node_slug"`
}

// RoleBindingPayload is shared by role_bound & role_unbound.
type RoleBindingPayload struct {
	RoleSlug string  `json:"role_slug"`
	Actor    ActorID `json:"actor_id"`
	Scope    string  `json:"scope,omitempty"`
}

// --- ACK Lifecycle Payloads ---

// AckFiledPayload records a commitment the Specifier files for bilateral
// acceptance. The commitment fields are free-form text the methodology
// interprets.
type AckFiledPayload struct {
	AckID              string `json:"ack_id"`
	ScopeSummary       string `json:"scope_summary"`
	DeliveryTiming     string `json:"delivery_timing"`
	TargetOutcome      string `json:"target_outcome"`
	AcceptanceCriteria string `json:"acceptance_criteria"`
}

// AckSidePayload is shared by ack_accepted & ack_rejected: one side stands
// behind (or rejects) the current commitment. Who is "specifier" | "builder".
type AckSidePayload struct {
	Who  string `json:"who"`
	Note string `json:"note,omitempty"`
}

// AckClearedPayload resets one or both sides to pending. It is auto-emitted
// when a material amendment lands after acceptance. Who is "specifier" |
// "builder" | "both".
type AckClearedPayload struct {
	Who    string `json:"who"`
	Reason string `json:"reason,omitempty"`
}

// AckAmendedPayload records a change to a filed commitment. AmendmentType is
// one of scope_change | timeline_change | target_change | clarification.
type AckAmendedPayload struct {
	AmendmentType string   `json:"amendment_type"`
	Fields        []string `json:"fields,omitempty"`
	Reason        string   `json:"reason,omitempty"`
}

// MustMarshalPayload marshals a payload to JSON, panicking on error.
func MustMarshalPayload(v any) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return data
}
