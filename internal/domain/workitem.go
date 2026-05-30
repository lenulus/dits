package domain

import (
	"encoding/json"
	"time"
)

// EmittedBy identifies who/what created and emitted the event.
// This is event-level provenance — the orchestration layer.
type EmittedBy struct {
	ActorID ActorID `json:"actor_id"`
	AgentID string  `json:"agent_id,omitempty"`
	Version string  `json:"version,omitempty"`
}

// ProducedBy identifies who/what produced the referenced content.
// This is content-level provenance — the subject layer.
type ProducedBy struct {
	ActorID            ActorID  `json:"actor_id"`
	AgentID            string   `json:"agent_id,omitempty"`
	Model              string   `json:"model,omitempty"`
	Tool               string   `json:"tool,omitempty"`
	Version            string   `json:"version,omitempty"`
	PromptRef          string   `json:"prompt_ref,omitempty"`
	SourceArtifactRefs []string `json:"source_artifact_refs,omitempty"`
}

// WorkItem is the primary coordination object. Materialized from events.
// Generalizes Issue from v1. An "issue" is a work item with kind=issue.
type WorkItem struct {
	ID       WorkItemID
	SharedID SharedID
	Kind     string // task, issue, investigation, plan, decision, execution, handoff, eval
	Title    string
	Body     string
	Status   string
	Priority string
	Labels   []string
	Assignees []ActorID

	// Coordination state
	LeaseHolder        *ActorID
	LeaseExpiresAt     *time.Time
	LeaseID            *LeaseID  // current active lease identity
	LeaseGeneration    uint64    // monotonically increasing per work item
	CurrentAttempt     *AttemptID
	Blocked            bool
	BlockedReason      string
	RetainedOutcomeRef *string // subject ref of the currently retained output

	// Timestamps
	CreatedBy ActorID
	CreatedAt time.Time
	UpdatedAt time.Time
	ClosedAt  *time.Time

	// Collections
	Comments     []Comment
	Artifacts    []Artifact
	Relations    []Relation
	Checkpoints  []Checkpoint
	Observations []Observation
	Findings     []Finding
	Attempts     []ExecutionAttempt
	Evals        []Eval
	Outcomes     []Outcome

	// Classification & role coordination (RE substrate).
	Classifications []Classification
	RoleBindings    []RoleBinding
	// Diagnostics are populated externally by the constraints engine, not by
	// Reduce — the domain package must not import constraints.
	Diagnostics []Diagnostic

	// ACK commitments. Each ack_filed appends a new Ack; accept/reject/clear/
	// amend apply to the most recently filed one.
	Acks []Ack

	// Generic projection state (methodology-agnostic). Fields holds scalar
	// key/value projection set via work.field_set (latest write per key wins);
	// Stages holds the staged timeline set via work.schedule_set (replaced
	// wholesale). DITS attaches no meaning to keys, values, or stage fields —
	// a consuming methodology (e.g. RE: ryg, target, customer_visible, the
	// Dogfood/Beta/GA timeline) interprets them.
	Fields map[string]string
	Stages []Stage

	EventCount int
	HeadEvents []EventID
}

// Stage is one materialized entry of a work item's staged delivery timeline.
// All fields are opaque to DITS.
type Stage struct {
	Key       string
	Label     string
	Date      string
	Precision string
	State     string
}

// Ack is a materialized two-party commitment: the filed scope/timing/outcome
// plus each side's stance and the amendment history.
type Ack struct {
	AckID              string
	ScopeSummary       string
	DeliveryTiming     string
	TargetOutcome      string
	AcceptanceCriteria string
	Specifier          AckState // pending | accepted | rejected
	Builder            AckState
	Amendments         []AckAmendment
	FiledBy            ActorID
	FiledAt            time.Time
}

// Rollup derives the alignment of this commitment's two sides.
func (a Ack) Rollup() AckRollup { return ComputeAckRollup(a.Specifier, a.Builder) }

// AckAmendment records one change to a filed commitment.
type AckAmendment struct {
	Type      string // scope_change | timeline_change | target_change | clarification
	Fields    []string
	Reason    string
	ActorID   ActorID
	Timestamp time.Time
}

// Classification places a work item within a node of a taxonomy.
type Classification struct {
	TaxonomySlug string
	NodeSlug     string
}

// RoleBinding binds an actor to a named role on a work item.
type RoleBinding struct {
	RoleSlug string
	Actor    ActorID
}

// Diagnostic is an advisory finding produced by the constraints engine.
type Diagnostic struct {
	ConstraintSlug string
	Severity       string // "warning" | "violation"
	Message        string
}

// Comment on a work item.
type Comment struct {
	EventID    EventID
	ActorID    ActorID
	Body       string
	ProducedBy *ProducedBy
	Timestamp  time.Time
}

// Artifact is a content-addressed file with type, semantic role, and provenance.
// Generalizes Attachment from v1.
type Artifact struct {
	ID           ArtifactID
	ContentHash  string // sha256:<hex>
	Filename     string
	MimeType     string
	SizeBytes    int64
	ArtifactType string // log, patch, screenshot, report, json_output, trace, plan, model_response
	SemanticRole string // evidence, proposal, intermediate_output, final_output, review_input, debug_trace
	ProducedBy   *ProducedBy
	AddedBy      ActorID
	AddedAt      time.Time
}

// Relation links two work items.
type Relation struct {
	Type           string
	TargetWorkItem WorkItemID
}

// ExecutionAttempt tracks a single attempt to execute a work item.
type ExecutionAttempt struct {
	AttemptID      AttemptID
	Number         uint32 // derived from authoritative ordering during materialization
	ActorID        ActorID
	StartedAt      time.Time
	CompletedAt    *time.Time
	Status         string // running, completed, failed, abandoned
	LastCheckpoint *EventID
	Authoritative  bool // true if this attempt belongs to the winning lease lineage
}

// Checkpoint records progress within an execution attempt.
type Checkpoint struct {
	EventID   EventID
	AttemptID AttemptID
	Summary   string
	Progress  float64         // 0.0 - 1.0
	NextStep  string
	Data      json.RawMessage // arbitrary structured state
	Timestamp time.Time
}

// Observation is an unstructured record of something noticed.
type Observation struct {
	EventID    EventID
	Summary    string
	Data       json.RawMessage
	ProducedBy *ProducedBy
	Timestamp  time.Time
}

// Finding is a structured epistemic assertion — what was observed, concluded, or hypothesized.
type Finding struct {
	EventID      EventID
	Statement    string
	Confidence   float64 // 0.0 - 1.0
	Source       string
	EvidenceRefs []string // artifact content hashes
	ProducedBy   *ProducedBy
	Retracted    bool
	Timestamp    time.Time
}

// Eval is a structured, repeatable machine-performed assessment.
// Eval = machine judgment. Review = human judgment.
type Eval struct {
	EvalID      EvalID
	EventID     EventID
	SubjectKind string // work_item, artifact, attempt, finding, plan
	SubjectRef  string // ID or content hash of the subject being evaluated
	RubricRef   string // optional reference to the rubric/policy used
	Summary     string // human-readable summary of the eval result
	Metrics     json.RawMessage
	Verdict     string // pass, fail, partial, etc.
	ProducedBy  *ProducedBy
	Timestamp   time.Time
}

// Outcome records a selection decision — which output was retained or discarded.
type Outcome struct {
	EventID     EventID
	SubjectKind string // attempt, artifact
	SubjectRef  string // attempt ID, artifact ID, or content hash
	Decision    string // "retained" or "discarded"
	Reason      string
	EvalRef     string // optional — eval that informed this decision
	ActorID     ActorID
	Timestamp   time.Time
}
