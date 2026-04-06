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
	Kind     string // task, issue, investigation, plan, decision, execution, handoff, artifact_review
	Title    string
	Body     string
	Status   string
	Priority string
	Labels   []string
	Assignees []ActorID

	// Coordination state
	LeaseHolder    *ActorID
	LeaseExpiresAt *time.Time
	CurrentAttempt *AttemptID
	Blocked        bool
	BlockedReason  string

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

	EventCount int
	HeadEvents []EventID
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
	EvalID     EvalID
	EventID    EventID
	SubjectRef string // content hash, work item ID, or artifact ID being evaluated
	Metrics    json.RawMessage
	Verdict    string // pass, fail, partial, etc.
	ProducedBy *ProducedBy
	Timestamp  time.Time
}
