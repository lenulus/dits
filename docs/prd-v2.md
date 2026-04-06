# PRD: DITS v2 — Distributed Coordination Substrate

**Status:** Draft
**Author:** Anthony Laforge
**Version:** 0.4
**Branch:** v2

---

## 1. Problem Statement

DITS v1 is a distributed issue tracker. It works well for that: append-only event log, DAG causality, deterministic reduction, signed actors, synced meta, content-addressed attachments, local-first sync.

But the domain model is shaped like Jira. The primary object is an issue. The event vocabulary covers ticketing: created, commented, labeled, closed. The query surfaces return lists of tickets.

Agents do not think in tickets. They work on investigations, plans, execution runs, reviews, handoffs, remediation actions, decision threads. When the primitive is "issue," everything gets squeezed into ticketing semantics. Comments become scratch state. Status becomes a lossy compression of execution progress. Attachments become undifferentiated blobs.

v1 treats coordination as a **stateful record of work** — things happened, here is the history. v2 treats it as a **durable coordination substrate for multiple actors, including agents** — here is the live state, here is who owns what, here is what is ready. The engine is already the hard part. The limitation is the semantic layer.

v2 keeps the engine. Changes the center of gravity.

v2 is a **new versioned system**, not an in-place migration of v1 repositories. New domain model, new event vocabulary, new tables. The engine (DAG, sync, blob store, signatures) carries forward. v1 data does not convert automatically — this is a clean break at the semantic layer.

---

## 2. Design Goals

1. **Work items, not issues** — the primary object is a durable coordination object with a `kind`, not a ticket with a `type_slug`
2. **Agent-native coordination** — first-class events for leasing work, checkpointing progress, recording evidence, proposing plans, and handing off between actors
3. **Structured provenance** — every meaningful event carries optional metadata about what produced it (model, tool, version, source artifacts)
4. **Artifacts over attachments** — files have types, semantic roles, and producer metadata — they are outputs, not footnotes
5. **Machine query surfaces** — endpoints that answer "what should I do next?" not just "list all tickets"
6. **Human projection preserved** — issue tracking is one view over the substrate, not a separate system
7. **Autonomous optimization loop support** — the substrate supports iterative propose-execute-eval-retry loops where the evaluated subject may be a work item, attempt, artifact, or plan; eval results are structured, queryable, and tied to provenance so agents can improve over time without relying on unstructured logs

---

## 3. Non-Goals

- Multi-project federation (cross-project references)
- Real-time event streaming (WebSocket/SSE)
- Permission/authorization system (RBAC, ACLs)
- Agent orchestration logic (DITS records coordination state, it does not direct agents)
- Binary wire protocol
- Encryption at rest
- Gap-free sequential numbering

---

## 4. Key Design Principle

> **The primitive is a durable coordination object with causal history and materialized operational views. Ticketing is one human-facing projection of that substrate.**

A work item is the unit of coordination. It has a kind. An "issue" is a work item with `kind=issue`. The event vocabulary is the API. HTTP endpoints are query surfaces, not the coordination mechanism. Two agents coordinate by appending events to the same work item's DAG.

> **Operational coordination state (lease ownership, active attempts, blocked flags) is first-class and not reducible to workflow status.** Status remains a human- and policy-facing abstraction over work progression. A work item can be `status=in_progress` while also being blocked, leased, and on its third attempt — those are independent dimensions, not redundant encodings of the same thing.

---

## 5. Domain Model

### 5.1 Identity Types

| Type | Format | Description |
|------|--------|-------------|
| `WorkItemID` | `wrk_<ULID>` | Primary object identifier |
| `EventID` | `evt_<ULID>` | Immutable event identifier |
| `SharedID` | `PROJ-<N>` | Server-assigned, human-friendly |
| `ActorID` | `actor_<pubkey_prefix>` | Derived from Ed25519 public key |
| `NodeID` | `node_<ULID>` | DITS instance identifier |
| `ArtifactID` | `art_<ULID>` | Content reference |
| `LeaseID` | `lea_<ULID>` | Lease identifier |
| `AttemptID` | `atp_<ULID>` | Execution attempt identifier |
| `ReviewID` | `rev_<ULID>` | Review instance identifier |
| `HandoffID` | `hof_<ULID>` | Handoff instance identifier |
| `EvalID` | `evl_<ULID>` | Eval instance identifier |

### 5.2 Work Item (Primary Object)

Generalizes `Issue`. Materialized from events. Issue becomes a specialization (`kind=issue`).

```go
type WorkItem struct {
    ID              WorkItemID
    SharedID        SharedID
    Kind            string       // task, issue, investigation, plan, decision, execution, handoff, eval
    Title           string
    Body            string
    Status          string
    Priority        string
    Labels          []string
    Assignees       []ActorID

    // Coordination state
    LeaseHolder       *ActorID
    LeaseExpiresAt  *time.Time
    CurrentAttempt  *AttemptID
    Blocked         bool
    BlockedReason   string

    // Timestamps
    CreatedBy       ActorID
    CreatedAt       time.Time
    UpdatedAt       time.Time
    ClosedAt        *time.Time

    // Collections
    Comments        []Comment
    Artifacts       []Artifact
    Relations       []Relation
    Checkpoints     []Checkpoint
    Observations    []Observation
    Findings        []Finding
    Attempts        []ExecutionAttempt

    EventCount      int
    HeadEvents      []EventID
}
```

### 5.3 Work Kinds

Default kinds, extensible via meta:

| Kind | Description |
|------|-------------|
| `task` | A unit of work to be done |
| `issue` | A bug, defect, or problem to fix |
| `investigation` | Research or analysis to produce findings |
| `plan` | A structured proposal for a course of action |
| `decision` | A choice to be made between alternatives |
| `execution` | A concrete run of a plan or procedure |
| `handoff` | A transfer of responsibility between actors |
| `eval` | Machine-performed assessment of artifacts, attempts, findings, or plans |

### 5.4 Artifact

Generalizes `Attachment`. Content-addressed, with type and provenance.

An artifact has two identities:
- **`ArtifactID`** (`art_<ULID>`) — identifies the reference record (per-work-item, carries role/type/provenance)
- **`ContentHash`** (`sha256:<hex>`) — identifies the underlying bytes (global, content-addressed)

The same content hash can appear in multiple artifacts with different roles, types, and provenance. Removing an artifact removes the reference from the work item; the blob remains in the content-addressed store (garbage collection is a separate concern).

```go
type Artifact struct {
    ID            ArtifactID
    ContentHash   string        // sha256:<hex>
    Filename      string
    MimeType      string
    SizeBytes     int64
    ArtifactType  string        // log, patch, screenshot, report, json_output, trace, plan, model_response
    SemanticRole  string        // evidence, proposal, intermediate_output, final_output, review_input, debug_trace
    ProducedBy    *ProducedBy
    AddedBy       ActorID
    AddedAt       time.Time
}
```

### 5.5 Provenance

Provenance exists at two distinct layers:

- **Event provenance** (`EmittedBy` on Event struct): who/what created and emitted the event itself. This is the orchestration layer — the agent or human that decided to record this event.
- **Content provenance** (`ProducedBy` in payloads): who/what produced the content being referenced. This is the subject layer — the model that generated a response, the tool that observed a log, the agent that wrote a patch.

These are often the same actor, but not always. An orchestration agent may emit an `evidence_attached` event for an artifact that was produced by a different tool or model.

```go
// EmittedBy identifies who/what created the event.
type EmittedBy struct {
    ActorID  ActorID `json:"actor_id"`
    AgentID  string  `json:"agent_id,omitempty"`
    Version  string  `json:"version,omitempty"`
}

// ProducedBy identifies who/what produced the referenced content.
type ProducedBy struct {
    ActorID            ActorID  `json:"actor_id"`
    AgentID            string   `json:"agent_id,omitempty"`
    Model              string   `json:"model,omitempty"`             // e.g. "claude-3.5-sonnet"
    Tool               string   `json:"tool,omitempty"`              // e.g. "code-search", "grep"
    Version            string   `json:"version,omitempty"`
    PromptRef          string   `json:"prompt_ref,omitempty"`        // content hash of prompt
    SourceArtifactRefs []string `json:"source_artifact_refs,omitempty"` // input artifact hashes
}
```

### 5.6 Execution Attempt

```go
type ExecutionAttempt struct {
    AttemptID       AttemptID
    Number          uint32      // 1, 2, 3...
    ActorID         ActorID
    StartedAt       time.Time
    CompletedAt     *time.Time
    Status          string      // running, completed, failed, abandoned
    LastCheckpoint  *EventID
}
```

### 5.7 Checkpoint

```go
type Checkpoint struct {
    EventID     EventID
    AttemptID   AttemptID
    Summary     string
    Progress    float64         // 0.0 - 1.0
    NextStep    string
    Data        json.RawMessage // arbitrary structured state
    Timestamp   time.Time
}
```

### 5.8 Observation

```go
type Observation struct {
    EventID     EventID
    Summary     string
    Data        json.RawMessage
    ProducedBy  *ProducedBy
    Timestamp   time.Time
}
```

### 5.9 Finding (Epistemic Assertion)

A finding is a structured claim about reality — what was observed, concluded, or hypothesized. Distinct from lease claims (which are about ownership).

```go
type Finding struct {
    EventID       EventID
    Statement     string
    Confidence    float64     // 0.0 - 1.0
    Source        string
    EvidenceRefs  []string    // artifact content hashes
    ProducedBy    *ProducedBy
    Retracted     bool
    Timestamp     time.Time
}
```

### 5.10 Eval (Machine Assessment)

An eval is a structured, repeatable machine-performed assessment. Evals target artifacts, attempts, findings, plans, or whole work items. Distinct from reviews, which are human-performed assessments for governance and approval.

> **Eval is machine-performed assessment. Review is human-performed assessment.**
> Evals are rubric- or metric-driven and intended to support automated control loops (plan → execute → eval → retry). Reviews are human judgments used for governance, approval, critique, or policy gates.

```go
type Eval struct {
    EvalID      EvalID
    EventID     EventID
    SubjectKind string          // work_item, artifact, attempt, finding, plan
    SubjectRef  string          // ID or content hash of the subject being evaluated
    RubricRef   string          // optional reference to the rubric/policy used
    Summary     string          // human-readable summary of the eval result
    Metrics     json.RawMessage // structured metric results
    Verdict     string          // pass, fail, partial, etc.
    ProducedBy  *ProducedBy
    Timestamp   time.Time
}
```

### 5.11 Outcome (Selection Decision)

An outcome records which output was accepted (retained) or rejected (discarded) in an optimization loop. Supports the propose-execute-eval-retain cycle.

```go
type Outcome struct {
    EventID     EventID
    SubjectKind string  // attempt, artifact
    SubjectRef  string  // attempt ID, artifact ID, or content hash
    Decision    string  // "retained" or "discarded"
    Reason      string
    EvalRef     string  // optional — eval that informed this decision
    ActorID     ActorID
    Timestamp   time.Time
}
```

The WorkItem tracks `RetainedOutcomeRef *string` — the subject ref of the currently retained output. This is set by `work.outcome_retained` and cleared if the same ref is later discarded.

### 5.12 Relation Types

Formalized core set, extensible via meta:

| Slug | Name | Inverse |
|------|------|---------|
| `blocks` | Blocks | `blocked_by` |
| `blocked_by` | Blocked by | `blocks` |
| `depends_on` | Depends on | `depended_on_by` |
| `parent_of` | Parent of | `child_of` |
| `child_of` | Child of | `parent_of` |
| `relates_to` | Relates to | `relates_to` |
| `duplicates` | Duplicates | `duplicated_by` |
| `derived_from` | Derived from | `derived_into` |
| `produced_artifact` | Produced artifact | — |
| `supports_finding` | Supports finding | — |
| `supersedes` | Supersedes | `superseded_by` |

---

## 6. Event Vocabulary

Event naming convention: `work.<verb_past_tense>`.

The `Event` struct carries `EmittedBy` for event-level provenance. Payload-level `produced_by` fields capture content provenance where applicable.

```go
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
```

### 6.1 Lifecycle Events

| Event | Payload | Notes |
|-------|---------|-------|
| `work.created` | `{title, body?, kind, schema_ref?}` | Primary creation event |
| `work.title_set` | `{title}` | |
| `work.body_set` | `{body}` | |
| `work.status_set` | `{from, to}` | Validated against workflow |
| `work.priority_set` | `{priority}` | |
| `work.label_added` | `{label_slug}` | Validated against meta |
| `work.label_removed` | `{label_slug}` | |
| `work.assigned` | `{assignee}` | |
| `work.unassigned` | `{assignee}` | |
| `work.commented` | `{body, produced_by?}` | |
| `work.closed` | `{reason?}` | |
| `work.reopened` | `{}` | |
| `work.shared_id_assigned` | `{shared_id}` | Server-assigned |

### 6.2 Execution / Ownership Events

| Event | Payload |
|-------|---------|
| `work.leased` | `{lease_id, lease_duration_secs, lease_expires_at, generation}` |
| `work.lease_released` | `{lease_id, reason}` |
| `work.lease_renewed` | `{lease_id, lease_expires_at, generation}` |
| `work.execution_started` | `{attempt_id, attempt_number, plan_ref?}` |
| `work.execution_completed` | `{attempt_id, summary, output_artifact_refs?}` |
| `work.execution_failed` | `{attempt_id, error, retryable, output_artifact_refs?}` |
| `work.execution_abandoned` | `{attempt_id, reason}` |

### 6.3 Checkpoint / Progress Events

| Event | Payload |
|-------|---------|
| `work.checkpointed` | `{attempt_id, summary, progress, next_step?, data?}` |
| `work.progress_reported` | `{attempt_id, progress, message}` |
| `work.blocked` | `{reason, blocked_by_ref?}` |
| `work.unblocked` | `{reason}` |

### 6.4 Evidence / Observation Events

| Event | Payload |
|-------|---------|
| `work.observation_recorded` | `{summary, data?, produced_by?}` |
| `work.evidence_attached` | `{artifact_id, content_hash, filename, mime_type, size_bytes, artifact_type, semantic_role, produced_by?}` |
| `work.finding_recorded` | `{statement, confidence, source?, evidence_refs?, produced_by?}` |
| `work.finding_retracted` | `{original_event_id, reason}` |

### 6.5 Planning / Decision Events

| Event | Payload |
|-------|---------|
| `work.plan_proposed` | `{plan, summary, produced_by?}` |
| `work.plan_accepted` | `{plan_event_id, comment?}` |
| `work.plan_rejected` | `{plan_event_id, reason}` |
| `work.step_added` | `{step_index, description, depends_on?}` |
| `work.decision_recorded` | `{decision, rationale, alternatives?, produced_by?}` |

### 6.6 Handoff / Review Events

| Event | Payload |
|-------|---------|
| `work.review_requested` | `{review_id, reviewer?, scope, artifact_refs?}` |
| `work.review_completed` | `{review_id, verdict, comment?, produced_by?}` |
| `work.handed_off` | `{handoff_id, from, to, context, artifact_refs?}` |
| `work.handoff_accepted` | `{handoff_id, comment?}` |
| `work.handoff_rejected` | `{handoff_id, reason}` |

### 6.7 Eval Events

| Event | Payload |
|-------|---------|
| `work.eval_requested` | `{eval_id, subject_kind?, subject_ref, rubric_ref?, scope}` |
| `work.eval_completed` | `{eval_id, subject_kind?, subject_ref, rubric_ref?, summary?, metrics?, verdict, produced_by?}` |

### 6.8 Outcome Events

| Event | Payload |
|-------|---------|
| `work.outcome_retained` | `{subject_kind, subject_ref, reason?, eval_ref?}` |
| `work.outcome_discarded` | `{subject_kind, subject_ref, reason?}` |

Outcome events record selection decisions in optimization loops. After executing and evaluating multiple approaches, the retained outcome marks the accepted/winning output. Discarded outcomes are superseded. If the discarded subject matches the currently retained ref, the retained ref is cleared.

### 6.9 Relation / Artifact Events

| Event | Payload |
|-------|---------|
| `work.linked` | `{relation_type, target_work_item}` |
| `work.unlinked` | `{relation_type, target_work_item}` |
| `work.artifact_added` | `{artifact_id, content_hash, filename, mime_type, size_bytes, artifact_type?, semantic_role?, produced_by?}` |
| `work.artifact_removed` | `{artifact_id}` |

---

## 7. Coordination Semantics

### 7.1 Lease Model

Leasing a work item creates a lease with an expiration time. Leases are materialized into a coordination table.

**Rules:**
- Only one active lease per work item at a time
- Leasing a work item with an active non-expired lease fails
- A lease can be renewed (extends expiration) or released (explicit relinquish)
- If a lease expires without renewal, any actor may claim
- The `generation` counter is monotonically increasing per work item — prevents stale renewals

**Materialization:**
Leases are a hybrid of events + wall-clock time. The `coordination_leases` table is an operational cache. Rebuild logic: replay all lease/release/renew events in causal order, then expire leases where `expires_at < now()`.

```go
type Lease struct {
    WorkItemID  WorkItemID
    LeaseHolder   ActorID
    LeaseID     LeaseID
    Generation  uint64
    ExpiresAt   time.Time
    RenewedAt   time.Time
}
```

### 7.2 Execution Attempts

A work item may have multiple attempts:

- Attempt 1: failed
- Attempt 2: partially succeeded, abandoned
- Attempt 3: handed off to human
- Attempt 4: completed

Each attempt has a number (sequential per work item) and an ID. Checkpoints are recorded against the current attempt. An attempt ends with `execution_completed`, `execution_failed`, or `execution_abandoned`.

Failed attempts with `retryable=true` signal that a new attempt is appropriate.

### 7.3 Blocked / Unblocked

`work.blocked` sets the work item as blocked with a reason and optional reference to the blocking work item. `work.unblocked` clears it. This is a simple flag — the latest event wins.

### 7.4 Handoff Protocol

```
Actor A                          Actor B
  |                                |
  |--- work.handed_off ----------->|
  |    (handoff_id, context,       |
  |     artifact_refs)             |
  |                                |
  |<-- work.handoff_accepted ------|
  |    or                          |
  |<-- work.handoff_rejected ------|
```

Each handoff has a `HandoffID` for durable sub-entity identity. Rejection returns effective ownership to the handing-off actor. Multiple concurrent handoffs to different actors are possible (each with a distinct `handoff_id`).

### 7.5 Review Protocol

```
Actor A                          Reviewer
  |                                |
  |--- work.review_requested ----->|
  |    (review_id, scope,          |
  |     artifact_refs)             |
  |                                |
  |<-- work.review_completed ------|
  |    (review_id, verdict:        |
  |     approve | request_changes  |
  |     | reject)                  |
```

Each review has a `ReviewID` for durable sub-entity identity. Multiple concurrent reviews are supported (each with a distinct `review_id`).

### 7.6 Operational Lineage

When two actors diverge offline and both emit lease/execution events against the same work item, the merged DAG after sync contains both branches. The reducer must deterministically select one as the authoritative coordination lineage.

**Rules:**

1. **Lease validity is gated by causal order.** The reducer processes events in deterministic causal order. When two concurrent `work.leased` events exist, the one that appears later in causal order (by the standard tiebreak: timestamp → event ID) becomes the active lease. The earlier one is superseded.

2. **Downstream events inherit lineage validity.** Execution attempts, checkpoints, and completion events on a superseded lease branch are preserved in history but marked as non-authoritative. They do not drive live operational state (`LeaseHolder`, `CurrentAttempt`, etc.).

3. **Attempt numbering is derived, not trusted.** `AttemptNumber` in `work.execution_started` payloads is advisory. The reducer computes canonical attempt numbers from the causal ordering of authoritative `work.execution_started` events during materialization.

**Split-brain example:**

```
Branch A (offline):  leased(A1) → execution_started(X1) → checkpointed(C1)
Branch B (offline):  leased(B1) → execution_started(X2) → checkpointed(C2)

After sync and causal ordering:
  - A1 and B1 are concurrent (same parent)
  - Tiebreak: e.g., B1 wins (later timestamp or higher event ID)
  - B1's lease is authoritative
  - X2, C2 are authoritative attempts/checkpoints
  - A1 is superseded; X1, C1 are non-authoritative (orphaned)
  - All events remain in history for audit
```

**Materialization:**

The `Authoritative` flag on `ExecutionAttempt` indicates whether the attempt belongs to the winning lease lineage. `CurrentAttempt` only references authoritative attempts. Orphaned attempts and their checkpoints remain queryable for debugging and audit but do not affect the work item's live coordination state.

---

## 8. Meta Schema

Evolves from workflow config to coordination schema.

```go
type MetaConfig struct {
    Version         MetaVersion      `json:"version"`
    ProjectKey      string           `json:"project_key"`

    // Work structure
    WorkKinds       []WorkKind       `json:"work_kinds"`
    Workflows       []Workflow       `json:"workflows"`
    Priorities      []string         `json:"priorities"`
    Labels          []Label          `json:"labels"`

    // Artifact & evidence
    ArtifactTypes   []ArtifactType   `json:"artifact_types"`
    EvidenceTypes   []EvidenceType   `json:"evidence_types"`

    // Relations
    RelationTypes   []RelationType   `json:"relation_types"`

    // Policies
    LeasePolicies   []LeasePolicy    `json:"lease_policies"`
    ReviewPolicies  []ReviewPolicy   `json:"review_policies"`
}
```

### Work Kind

```go
type WorkKind struct {
    Slug          string `json:"slug"`
    Name          string `json:"name"`
    WorkflowSlug  string `json:"workflow_slug"`
    Description   string `json:"description,omitempty"`
}
```

### Artifact Type

```go
type ArtifactType struct {
    Slug        string   `json:"slug"`
    Name        string   `json:"name"`
    MimeTypes   []string `json:"mime_types,omitempty"`
}
```

### Evidence Type

```go
type EvidenceType struct {
    Slug string `json:"slug"`
    Name string `json:"name"`
}
```

### Relation Type

```go
type RelationType struct {
    Slug    string `json:"slug"`
    Name    string `json:"name"`
    Inverse string `json:"inverse,omitempty"`
}
```

### Lease Policy

```go
type LeasePolicy struct {
    WorkKindSlug       string `json:"work_kind_slug"`
    DefaultDurationSec int    `json:"default_duration_sec"`
    MaxDurationSec     int    `json:"max_duration_sec"`
    MaxRenewals        int    `json:"max_renewals"`
}
```

### Review Policy

```go
type ReviewPolicy struct {
    WorkKindSlug    string `json:"work_kind_slug"`
    RequiredReviews int    `json:"required_reviews"`
}
```

### Default Meta

```go
func DefaultMetaConfig(projectKey string) MetaConfig {
    return MetaConfig{
        Version:    1,
        ProjectKey: projectKey,
        WorkKinds: []WorkKind{
            {Slug: "task", Name: "Task", WorkflowSlug: "default"},
            {Slug: "issue", Name: "Issue", WorkflowSlug: "default"},
            {Slug: "investigation", Name: "Investigation", WorkflowSlug: "default"},
            {Slug: "plan", Name: "Plan", WorkflowSlug: "default"},
            {Slug: "decision", Name: "Decision", WorkflowSlug: "default"},
            {Slug: "execution", Name: "Execution", WorkflowSlug: "execution"},
            {Slug: "handoff", Name: "Handoff", WorkflowSlug: "default"},
            {Slug: "eval", Name: "Eval", WorkflowSlug: "default"},
        },
        Workflows: []Workflow{
            {
                Slug: "default",
                Name: "Default",
                Statuses: []WorkflowStatus{
                    {Slug: "open", Name: "Open", Category: "open"},
                    {Slug: "in_progress", Name: "In Progress", Category: "in_progress"},
                    {Slug: "closed", Name: "Closed", Category: "done"},
                },
            },
            // NOTE: Execution workflow statuses are human-facing reporting states,
            // not authoritative operational truth. The actual lease, attempt, and
            // blocked state lives in first-class coordination fields. These statuses
            // exist so humans and dashboards can see a simplified progression.
            {
                Slug: "execution",
                Name: "Execution",
                Statuses: []WorkflowStatus{
                    {Slug: "pending", Name: "Pending", Category: "open"},
                    {Slug: "active", Name: "Active", Category: "in_progress"},
                    {Slug: "completed", Name: "Completed", Category: "done"},
                    {Slug: "failed", Name: "Failed", Category: "done"},
                },
            },
            {
                Slug: "review",
                Name: "Review",
                Statuses: []WorkflowStatus{
                    {Slug: "pending_review", Name: "Pending Review", Category: "open"},
                    {Slug: "in_review", Name: "In Review", Category: "in_progress"},
                    {Slug: "approved", Name: "Approved", Category: "done"},
                    {Slug: "changes_requested", Name: "Changes Requested", Category: "in_progress"},
                    {Slug: "rejected", Name: "Rejected", Category: "done"},
                },
            },
        },
        Priorities: []string{"low", "medium", "high", "critical"},
        Labels:     []Label{},
        ArtifactTypes: []ArtifactType{
            {Slug: "log", Name: "Log"},
            {Slug: "patch", Name: "Patch"},
            {Slug: "screenshot", Name: "Screenshot"},
            {Slug: "report", Name: "Report"},
            {Slug: "json_output", Name: "JSON Output"},
            {Slug: "trace", Name: "Trace"},
            {Slug: "plan", Name: "Plan"},
            {Slug: "model_response", Name: "Model Response"},
        },
        EvidenceTypes: []EvidenceType{
            {Slug: "observation", Name: "Observation"},
            {Slug: "measurement", Name: "Measurement"},
            {Slug: "test_result", Name: "Test Result"},
            {Slug: "log_analysis", Name: "Log Analysis"},
        },
        RelationTypes: []RelationType{
            {Slug: "blocks", Name: "Blocks", Inverse: "blocked_by"},
            {Slug: "blocked_by", Name: "Blocked by", Inverse: "blocks"},
            {Slug: "depends_on", Name: "Depends on", Inverse: "depended_on_by"},
            {Slug: "parent_of", Name: "Parent of", Inverse: "child_of"},
            {Slug: "child_of", Name: "Child of", Inverse: "parent_of"},
            {Slug: "relates_to", Name: "Relates to", Inverse: "relates_to"},
            {Slug: "duplicates", Name: "Duplicates", Inverse: "duplicated_by"},
            {Slug: "derived_from", Name: "Derived from", Inverse: "derived_into"},
            {Slug: "supersedes", Name: "Supersedes", Inverse: "superseded_by"},
        },
        LeasePolicies: []LeasePolicy{
            {WorkKindSlug: "execution", DefaultDurationSec: 300, MaxDurationSec: 3600, MaxRenewals: 10},
        },
        ReviewPolicies: []ReviewPolicy{},
    }
}
```

---

## 9. Query API

All v2 endpoints under `/api/v2/`.

### Work Items

| Method | Path | Query Params | Description |
|--------|------|-------------|-------------|
| GET | `/api/v2/work` | `kind`, `status`, `label`, `claimed_by`, `blocked`, `ready`, `q`, `limit`, `offset` | List work items |
| GET | `/api/v2/work/{id}` | — | Full work item with all collections |
| GET | `/api/v2/work/{id}/events` | `since`, `type` | Event stream for a work item |
| GET | `/api/v2/work/{id}/artifacts` | `type`, `role` | Artifacts for a work item |
| GET | `/api/v2/work/{id}/attempts` | — | Execution attempts |
| GET | `/api/v2/work/{id}/checkpoints` | `attempt_id` | Checkpoints |

### Coordination

| Method | Path | Query Params | Description |
|--------|------|-------------|-------------|
| GET | `/api/v2/leases` | `expired`, `claimed_by` | Active or expired leases |
| GET | `/api/v2/work?ready=true` | — | Unclaimed, unblocked, open work items |
| GET | `/api/v2/work?blocked=true` | — | Currently blocked work items |
| GET | `/api/v2/work?claimed_by={actor}` | — | Work items claimed by an actor |

### Events & Artifacts

| Method | Path | Query Params | Description |
|--------|------|-------------|-------------|
| GET | `/api/v2/events` | `since`, `type`, `actor_id`, `limit` | Cross-item event query |
| GET | `/api/v2/artifacts/{hash}/metadata` | — | Artifact metadata by content hash |

### `ready=true` semantics (v2 baseline readiness)

Returns work items where:
- Status is in an `open` category
- No active lease (unclaimed or lease expired)
- Not blocked (no unresolved `work.blocked` event)

This is the "what should I do next?" query for agents.

**Note:** This is v2 baseline readiness — a query convenience, not a universal scheduler truth. It answers a narrow question ("is anything obviously preventing work from starting?"), not a complete planning predicate. Future versions may evolve this into policy-driven readiness that also considers dependency satisfaction, required review completion, missing artifacts, and lease policy constraints.

---

## 10. Implementation Phases

### Phase 1: Domain Model + Core Events

Rewrite the domain layer. New primary object is `WorkItem`. New event types for lifecycle and execution.

**Scope:**
- `internal/domain/workitem.go` — WorkItem, Artifact, EmittedBy, ProducedBy, Checkpoint, Observation, Finding, ExecutionAttempt
- `internal/domain/event.go` — all `work.*` event type constants and payload structs
- `internal/domain/identity.go` — WorkItemID, ArtifactID, LeaseID, AttemptID generators
- `internal/domain/reduce.go` — new reducer for WorkItem from `work.*` events
- `internal/domain/validate.go` — validation for new event types against meta
- `internal/domain/meta.go` — extended MetaConfig with WorkKinds, ArtifactTypes, RelationTypes, policies
- Tests for reducer, validation, causal ordering with new event types

**Result:** Domain compiles and all reducer tests pass with new event types.

### Phase 2: Coordination Layer

Leases, execution attempts, and the hybrid coordination table.

**Scope:**
- SQLite migrations: `work_items` table, `coordination_leases`, `execution_attempts`, `work_checkpoints`
- `internal/store/store.go` — `WorkItemStore`, `CoordinationStore` interfaces
- `internal/store/sqlite/` — implementations
- Lease expiration logic (wall-clock based)
- Extend sync engine for new materialization

**Result:** Coordination state materializes correctly. Lease claim/release/expire works.

### Phase 3: Artifacts + Provenance + Query API

Rich artifacts, provenance metadata, and agent-oriented query endpoints.

**Scope:**
- SQLite migration: `work_artifacts` table with `artifact_type`, `semantic_role`, `produced_by_json`
- `internal/store/store.go` — `ArtifactStore` interface
- `internal/server/` — all `/api/v2/` endpoints
- Extended blob metadata
- Meta validation for artifact types, evidence types

**Result:** Full query API works. Artifacts with provenance are queryable.

### Phase 4: CLI + End-to-End

CLI commands for the new domain, plus full integration testing.

**Scope:**
- `dits work create`, `dits work list`, `dits work show`, `dits work lease`, `dits work lease-release`, `dits work checkpoint`, `dits work complete`, `dits work fail`, `dits work review`, `dits work handoff`
- `dits work observe`, `dits work evidence`, `dits work finding`, `dits work plan`
- Note: `complete` and `fail` operate on the current execution attempt, not the work item lifecycle. They emit `work.execution_completed` / `work.execution_failed` against the active attempt.
- `dits issue` as alias for `dits work --kind=issue`
- E2E test: two agents coordinating via leases and checkpoints through server
- Documentation updates

**Result:** Full system works end-to-end with agent coordination.

---

## 11. Invariants

These hold at all times, regardless of event ordering or node topology:

1. **Deterministic materialization** — the same event set must yield the same materialized WorkItem, regardless of which node reduces it
2. **At most one active lease** — a work item has at most one non-expired lease at any time
3. **Lease generation monotonicity** — expired leases are not renewable without a matching generation; stale renewals are rejected
4. **Artifacts are content-addressed and immutable by hash** — the same hash always references the same bytes
5. **SharedIDs are presentation, never canonical identity** — SharedID is a human convenience; WorkItemID is the durable key
6. **Operational state is independent of status** — lease, attempt, and blocked state do not imply or require a particular workflow status
7. **Events are immutable** — no event is ever modified or deleted after creation
8. **The coordination table is a rebuildable cache** — it can be dropped and reconstructed from events + wall-clock time
9. **Attempt IDs are globally unique, attempt numbers are derived** — AttemptID is canonical; attempt numbers are computed from the authoritative execution-start events during materialization, not relied upon as client-assigned global truth
10. **ReviewIDs, HandoffIDs, and EvalIDs uniquely identify durable sub-entities** — they are not ephemeral references but first-class objects within a work item's history, supporting concurrent reviews, handoffs, and evals
11. **Operational conflict resolution** — when concurrent lease lineages exist, the reducer deterministically selects one authoritative coordination lineage; events on losing lineages remain in history but do not affect live operational state
12. **Eval is machine judgment, review is human judgment** — evals are rubric/metric-driven for autonomous control loops; reviews are human assessments for governance and approval gates

---

## 12. Open Questions

Implementation decisions to resolve during build-out, not design blockers:

1. ~~**Lease acquisition: events-only or server-side CAS?**~~ **Resolved:** Events-only with operational lineage. Concurrent lease events are both accepted; the reducer deterministically selects one as authoritative via causal ordering. See §7.6.

2. ~~**Attempt number allocation under concurrency.**~~ **Resolved:** AttemptID is canonical. Attempt numbers are derived from authoritative execution-start events during materialization, not trusted from client payloads. See §7.6.

3. **Review/handoff acceptance side effects.** When a handoff is accepted, should the system automatically update assignment or create a lease for the accepting actor? Or does acceptance remain purely historical, requiring the acceptor to explicitly lease and start execution? The latter is more composable; the former is more ergonomic.

4. **Blob garbage collection.** When an artifact is removed from a work item, the blob remains in the content-addressed store. When (if ever) are unreferenced blobs cleaned up? Options: never (storage is cheap), manual GC command, reference-counted with periodic sweep. This is a deployment concern, not a protocol concern, but should be documented.

5. ~~**Eval as a first-class concept.**~~ **Resolved:** Eval implemented as work kind + event pair (`work.eval_requested` / `work.eval_completed`). Eval = machine judgment, review = human judgment. `artifact_review` removed from defaults — eval + review cover its semantics.

---

## 13. Success Criteria

1. An agent can lease a work item, checkpoint progress 3 times, and complete execution — all recorded as events in the DAG and visible via `dits work show`
2. Two agents attempting to lease the same work item: one succeeds, the other gets a conflict (lease already held)
3. A lease expires after its duration, and a different agent can then lease the work item
4. The coordination table can be rebuilt from events + wall-clock time (drop and recreate test)
5. Artifacts with provenance metadata are queryable by type, role, and producer
6. `GET /api/v2/work?ready=true` correctly returns only unclaimed, unblocked, open work items
7. Findings and observations are preserved with confidence scores and source references
8. Handoff between two actors completes with accept/reject semantics
9. All materialized state is deterministic: same events produce same WorkItem regardless of which node reduces them
10. `dits issue create` works as a convenience that creates a `work.created` event with `kind=issue`
