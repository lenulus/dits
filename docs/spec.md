# DITS Specification

**Version:** 2.0
**Status:** Implemented

---

## 1. Overview

DITS (Distributed Coordination Substrate) is a hybrid, local-first coordination system that combines:

- Append-only, event-sourced work item history
- Coordinated identity and schema (meta configuration)
- Deterministic synchronization and reconciliation
- Content-addressed artifact storage
- Cryptographic event signatures
- Agent-native coordination primitives (leases, attempts, checkpoints, findings)

The system separates:

- **Data Plane** (work item events) — distributed, mergeable
- **Control Plane** (meta configuration) — coordinated, versioned
- **Coordination Layer** (leases, attempts) — materialized from events
- **Private Layer** (overlay) — non-replicated user state
- **Blob Store** (artifacts) — content-addressed, synced separately

---

## 2. Identity Model

### 2.1 Work Item ID

Globally unique, client-generated. Format: `wrk_<ULID>`.

Properties: immutable, never reused, primary reference key.

### 2.2 Shared ID

Server-assigned, human-friendly. Format: `<PROJECT_KEY>-<N>` (e.g., `PROJ-42`).

Properties: monotonic, stable, may contain gaps, never reused. Shared IDs are server-assigned metadata propagated via the `SharedIDs` map in sync responses — they are not events in the work item DAG.

### 2.3 Event ID

Globally unique, client-generated. Format: `evt_<ULID>`.

### 2.4 Actor ID

Derived from Ed25519 public key. Format: `actor_<first 8 bytes of public key hex>`.

### 2.5 Node ID

Identifies a DITS instance. Format: `node_<ULID>`.

### 2.6 Artifact ID

Client-generated. Format: `art_<ULID>`.

### 2.7 Lease ID

Client-generated. Format: `lea_<ULID>`.

### 2.8 Attempt ID

Client-generated. Format: `atp_<ULID>`.

### 2.9 Review ID

Client-generated. Format: `rev_<ULID>`.

### 2.10 Handoff ID

Client-generated. Format: `hof_<ULID>`.

### 2.11 Eval ID

Client-generated. Format: `evl_<ULID>`.

### 2.11 ULID Generation

All ULIDs use a monotonic entropy source to prevent collisions at sub-millisecond creation rates. Entropy is reset if exhausted.

---

## 3. Event Model

### 3.1 Event Structure

All state changes are recorded as immutable events.

```json
{
  "id": "evt_01...",
  "work_item_id": "wrk_01...",
  "type": "work.status_set",
  "parent_event_ids": ["evt_00..."],
  "meta_version": 3,
  "actor_id": "actor_abc...",
  "timestamp": "2026-03-29T10:00:00Z",
  "payload": {"from": "open", "to": "in_progress"},
  "emitted_by": {"actor_id": "actor_abc...", "agent_id": "deploy-bot"},
  "signature": "<base64>"
}
```

### 3.2 Event Types

#### Lifecycle Events

| Type | Payload | Description |
|------|---------|-------------|
| `work.created` | `{title, body?, kind, schema_ref?}` | Creates a new work item |
| `work.title_set` | `{title}` | Updates title |
| `work.body_set` | `{body}` | Updates body |
| `work.status_set` | `{from, to}` | Changes status |
| `work.priority_set` | `{priority}` | Changes priority |
| `work.label_added` | `{label_slug}` | Adds a label |
| `work.label_removed` | `{label_slug}` | Removes a label |
| `work.assigned` | `{assignee}` | Assigns an actor |
| `work.unassigned` | `{assignee}` | Unassigns an actor |
| `work.commented` | `{body, produced_by?}` | Adds a comment |
| `work.closed` | `{reason?}` | Closes the work item |
| `work.reopened` | `{}` | Reopens the work item |

#### Execution / Ownership Events

| Type | Payload |
|------|---------|
| `work.leased` | `{lease_id, lease_duration_secs, lease_expires_at, generation}` |
| `work.lease_released` | `{lease_id, reason}` |
| `work.lease_renewed` | `{lease_id, lease_expires_at, generation}` |
| `work.execution_started` | `{attempt_id, attempt_number, plan_ref?}` |
| `work.execution_completed` | `{attempt_id, summary, output_artifact_refs?}` |
| `work.execution_failed` | `{attempt_id, error, retryable, output_artifact_refs?}` |
| `work.execution_abandoned` | `{attempt_id, reason}` |

#### Checkpoint / Progress Events

| Type | Payload |
|------|---------|
| `work.checkpointed` | `{attempt_id, summary, progress, next_step?, data?}` |
| `work.progress_reported` | `{attempt_id, progress, message}` |
| `work.blocked` | `{reason, blocked_by_ref?}` |
| `work.unblocked` | `{reason}` |

#### Evidence / Observation Events

| Type | Payload |
|------|---------|
| `work.observation_recorded` | `{summary, data?, produced_by?}` |
| `work.evidence_attached` | `{artifact_id, content_hash, filename, mime_type, size_bytes, artifact_type, semantic_role, produced_by?}` |
| `work.finding_recorded` | `{statement, confidence, source?, evidence_refs?, produced_by?}` |
| `work.finding_retracted` | `{original_event_id, reason}` |

#### Planning / Decision Events

| Type | Payload |
|------|---------|
| `work.plan_proposed` | `{plan, summary, produced_by?}` |
| `work.plan_accepted` | `{plan_event_id, comment?}` |
| `work.plan_rejected` | `{plan_event_id, reason}` |
| `work.step_added` | `{step_index, description, depends_on?}` |
| `work.decision_recorded` | `{decision, rationale, alternatives?, produced_by?}` |

#### Handoff / Review Events

| Type | Payload |
|------|---------|
| `work.review_requested` | `{review_id, reviewer?, scope, artifact_refs?}` |
| `work.review_completed` | `{review_id, verdict, comment?, produced_by?}` |
| `work.handed_off` | `{handoff_id, from, to, context, artifact_refs?}` |
| `work.handoff_accepted` | `{handoff_id, comment?}` |
| `work.handoff_rejected` | `{handoff_id, reason}` |

#### Eval Events

| Type | Payload |
|------|---------|
| `work.eval_requested` | `{eval_id, subject_kind?, subject_ref, rubric_ref?, scope}` |
| `work.eval_completed` | `{eval_id, subject_kind?, subject_ref, rubric_ref?, summary?, metrics?, verdict, produced_by?}` |

#### Outcome Events

| Type | Payload |
|------|---------|
| `work.outcome_retained` | `{subject_kind, subject_ref, reason?, eval_ref?}` |
| `work.outcome_discarded` | `{subject_kind, subject_ref, reason?}` |

#### Relation / Artifact Events

| Type | Payload |
|------|---------|
| `work.linked` | `{relation_type, target_work_item}` |
| `work.unlinked` | `{relation_type, target_work_item}` |
| `work.artifact_added` | `{artifact_id, content_hash, filename, mime_type, size_bytes, artifact_type?, semantic_role?, produced_by?}` |
| `work.artifact_removed` | `{artifact_id}` |

### 3.3 Event Rules

- Events are **immutable** — never modified or deleted.
- Events are **append-only** — new events are always added, never replace existing ones.
- Event IDs are **globally unique** — ULID with monotonic entropy prevents collisions.
- Events MUST reference their parent events via `parent_event_ids`.
- The first event for a work item (`work.created`) has an empty parent list.
- Events MUST carry the `meta_version` of the meta config at creation time.
- Labels, statuses, and kinds referenced in payloads MUST exist at the event's `meta_version`.

### 3.4 Protocol Invariants

Three validation tiers enforce event correctness:

1. **Schema validation** — meta references (kinds, statuses, labels, priorities, artifact types, relation types)
2. **Protocol validation** — coordination invariants against current materialized state
3. **Reducer** — deterministic materialization; applies all events without rejection

Protocol invariants enforced at event creation time:

| Event | Required Precondition |
|-------|----------------------|
| `work.leased` | No active lease on the work item |
| `work.lease_released` | Active lease exists; actor is lease holder |
| `work.lease_renewed` | Active lease exists; actor is lease holder |
| `work.execution_started` | Active lease exists; actor is lease holder; no running authoritative attempt |
| `work.execution_completed/failed/abandoned` | Active attempt exists; attempt is running; actor is lease holder |
| `work.checkpointed` | Active attempt exists; actor is lease holder |
| `work.blocked` | Work item is not already blocked |
| `work.unblocked` | Work item is currently blocked |

During sync ingestion, protocol validation is advisory (log, don't reject). The reducer handles concurrent offline divergence via operational lineage resolution.

---

## 4. DAG Structure

### 4.1 Definition

Events form a directed acyclic graph (DAG) through `parent_event_ids`. Each event references zero or more parent events.

### 4.2 Heads

Heads are events that are not a parent of any other event. They represent the current tips of the DAG for a work item.

### 4.3 Causal Ordering

Events are ordered using Kahn's algorithm (topological sort):

1. Compute in-degree for each event (count of parents in the set)
2. Initialize queue with zero-in-degree events
3. Process queue; for each processed event, decrement children's in-degrees
4. When queue has multiple candidates, tiebreak by:
   - Timestamp ascending
   - Event ID lexically ascending
5. Result is a deterministic total order

### 4.4 Conflict Resolution

All clients that have the same set of events will compute the same order and thus the same materialized state. This is the core invariant.

| Field Type | Resolution |
|------------|------------|
| Scalar (title, status, priority, kind) | Last writer wins in causal order |
| Set (labels, assignees) | Add/remove operations merge |
| Append-only (comments, checkpoints, observations, findings, evals) | All preserved in order |
| By-ID (artifacts, relations) | Add/remove by unique ID |
| Flag (blocked) | Latest event wins |
| Lease lineage (lease_holder, lease_expires_at) | Authoritative lineage selection — concurrent leases resolved by causal tiebreak; losing lease superseded |
| Attempt authority (current_attempt, attempts) | Attempts inherit lineage from active lease; non-authoritative attempts preserved but don't drive live state |
| Attempt numbering | Derived from authoritative execution-start ordering during materialization, not from client payloads |

---

## 5. Provenance

### 5.1 EmittedBy (Event-Level)

Who/what created and emitted the event. Attached to the Event struct.

```json
{"actor_id": "actor_abc...", "agent_id": "deploy-bot", "version": "1.2.0"}
```

### 5.2 ProducedBy (Content-Level)

Who/what produced the referenced content. Attached to payloads (artifacts, comments, findings, observations, decisions, plans).

```json
{
  "actor_id": "actor_abc...",
  "model": "claude-3.5-sonnet",
  "tool": "code-search",
  "prompt_ref": "sha256:...",
  "source_artifact_refs": ["sha256:..."]
}
```

These are often the same actor, but not always. An orchestration agent may emit an event for an artifact produced by a different tool.

---

## 6. Coordination Semantics

### 6.1 Lease Model

- One active lease per work item at a time
- Leasing with an active non-expired lease fails
- Leases have expiration time and generation counter
- Can be renewed (extends expiration) or released (explicit relinquish)
- Generation is monotonically increasing per work item

### 6.2 Execution Attempts

- Multiple attempts per work item, numbered sequentially (1, 2, 3...)
- Each attempt has a unique AttemptID and number
- Checkpoints recorded against current attempt
- Attempt ends with: `execution_completed`, `execution_failed`, or `execution_abandoned`

### 6.3 Blocked / Unblocked

Simple flag — latest event wins. `work.blocked` sets blocked with reason, `work.unblocked` clears it.

### 6.4 Status Independence

Operational state (lease, attempt, blocked) is independent of workflow status. A work item can be `status=in_progress`, `leased=true`, `attempts=3`, `blocked=true` simultaneously.

### 6.5 Operational Lineage

When concurrent lease lineages exist (e.g., two offline agents both lease the same work item), the reducer deterministically selects one as authoritative via causal ordering tiebreaks. Events on losing lineages are preserved in history but marked as non-authoritative. Attempt numbers are derived from authoritative execution-start events during materialization, not trusted from client payloads.

### 6.6 Eval vs Review

Eval is machine-performed assessment. Review is human-performed assessment. Evals are rubric/metric-driven for autonomous control loops (plan, execute, eval, retry). Reviews are human judgments for governance and approval gates. Both are first-class coordination activities.

---

## 7. Meta Configuration

### 7.1 Structure

```json
{
  "version": 3,
  "project_key": "PROJ",
  "work_kinds": [{"slug": "task", "name": "Task", "workflow_slug": "default"}],
  "workflows": [{"slug": "default", "statuses": [...], "transitions": [...]}],
  "priorities": ["low", "medium", "high", "critical"],
  "labels": [{"slug": "bug", "name": "Bug", "color": "#ff0000"}],
  "artifact_types": [{"slug": "log", "name": "Log"}],
  "evidence_types": [{"slug": "observation", "name": "Observation"}],
  "relation_types": [{"slug": "blocks", "name": "Blocks", "inverse": "blocked_by"}],
  "lease_policies": [{"work_kind_slug": "execution", "default_duration_sec": 300, "max_duration_sec": 3600, "max_renewals": 10}],
  "review_policies": []
}
```

### 7.2 Default Workflows

Three workflows ship by default:

- **default** — open, in_progress, closed
- **execution** — pending, active, completed, failed
- **review** — pending_review, in_review, approved, changes_requested, rejected

### 7.3 Versioning Rules

- Meta version is monotonically increasing.
- Every mutation increments the version.
- Local updates use optimistic concurrency: `SaveMetaIfVersion` checks current_version == expected_version.
- Sync resolves by highest version wins.

### 7.4 Validation

Events are validated against meta at creation time:

- `work.created`: `kind` must exist in `work_kinds`
- `work.status_set`: `to` status must exist in a workflow
- `work.label_added`: `label_slug` must exist in `labels`
- `work.priority_set`: `priority` must exist in `priorities`
- `work.artifact_added` / `work.evidence_attached`: `artifact_type` must exist (if non-empty)
- `work.linked` / `work.unlinked`: `relation_type` must exist in `relation_types`

---

## 8. Synchronization Protocol

### 8.1 Endpoint

```
POST /api/v1/sync
```

### 8.2 Request

```json
{
  "node_id": "node_01...",
  "project_key": "PROJ",
  "heads": ["evt_01...", "evt_02..."],
  "events": [],
  "meta_version": 3,
  "meta": null,
  "actor_id": "actor_abc...",
  "public_key": "e8748f..."
}
```

### 8.3 Response

```json
{
  "events": [],
  "shared_ids": {"wrk_01...": "PROJ-42"},
  "heads": ["evt_03...", "evt_04..."],
  "meta": null
}
```

### 8.4 Algorithm

**Server (HandleSync):**
1. Register actor (store public key)
2. Verify signatures (warn mode)
3. Ingest client events (`INSERT OR IGNORE`)
4. Accept client meta if version > server version
5. Rematerialize affected work items
6. Allocate shared IDs to new work items
7. Compute events client is missing (BFS from server heads, stop at client heads)
8. Include shared IDs for pulled work items
9. Store client heads as remote state
10. Return response with meta if server version > client version

**Client (ApplySync):**
1. Ingest server events
2. Rematerialize affected work items
3. Apply shared IDs
4. Update meta if server version > local version
5. Store server heads

### 8.5 Idempotency

Events use `INSERT OR IGNORE` on primary key (event ID). Duplicate events from retries are silently dropped. Sync is safe to retry.

---

## 9. Artifacts

### 9.1 Principles

- Artifact **metadata** is recorded as work item events.
- Artifact **bytes** are stored in a separate content-addressed blob store.
- Artifacts have types (`log`, `patch`, `screenshot`, `report`, etc.) and semantic roles (`evidence`, `proposal`, `final_output`, etc.).
- Artifacts carry optional `produced_by` provenance.
- Removing an artifact removes the reference, not the blob.

### 9.2 Content Addressing

Blobs are keyed by `sha256:<hex>`. Storage path: `<root>/<first2>/<next2>/<full_hash>`.

Properties: deduplication, integrity verification, atomic writes (temp file + rename).

### 9.3 Size Limit

Maximum blob size: 50MB. Enforced at upload time.

### 9.4 Blob Sync

After event sync:
1. Check which pushed hashes the server is missing (`POST /api/v1/blobs/check`)
2. Upload missing blobs (`PUT /api/v1/blobs/{hash}`)
3. Download blobs from pulled events (`GET /api/v1/blobs/{hash}`)

---

## 10. Signatures

### 10.1 Algorithm

Ed25519. Keypair generated on `dits init`.

### 10.2 Canonical JSON

Signature covers canonical JSON of the event: all fields except `signature`, sorted keys, no extra whitespace. Includes `emitted_by` if present. Timestamp formatted as `2006-01-02T15:04:05.999999999Z`.

### 10.3 Verification

Server verifies in **warn mode**: invalid signatures are logged but events are not rejected.

### 10.4 Actor Registry

Clients send `actor_id` and `public_key` in sync requests. Server stores in `actors` table.

---

## 11. Private Overlay

### 11.1 Principle

Overlay data is local-only and never synced.

### 11.2 Types

| Type | Storage | Description |
|------|---------|-------------|
| Annotations | Key-value pairs per work item | Personal notes, custom metadata |
| Private labels | String set per work item | Personal categorization |

### 11.3 Isolation

Each client maintains its own overlay. Syncing does not transfer overlay data.

---

## 12. Storage

### 12.1 Client

SQLite database at `.dits/dits.db`. Blob store at `.dits/blobs/`.

### 12.2 Server

SQLite database (configurable path). Blob store (configurable directory).

### 12.3 Schema

Seven incremental migrations:

| # | Tables |
|---|--------|
| 001 | events, event_parents, dag_heads, work_items, work_item_labels, work_item_assignees, work_item_comments, shared_id_counter, meta_config |
| 002 | sync_remotes, sync_remote_heads |
| 003 | work_item_artifacts |
| 004 | actors |
| 005 | overlay_annotations, overlay_labels |
| 006 | work_item_relations |
| 007 | work_item_checkpoints, work_item_observations, work_item_findings, work_item_attempts |
| 008 | work_item_evals |
| 009 | work_item_outcomes |

---

## 13. Design Principles

1. **Identity is not presentation** — canonical IDs are stable, shared IDs are for humans
2. **History is immutable** — events are never modified or deleted
3. **Schema is versioned** — meta changes are tracked and validated
4. **DAG defines truth** — causal order determines state, not timestamps
5. **Time is for humans** — timestamps aid display, not correctness
6. **Authority is minimal** — server coordinates IDs and meta, clients own their history
7. **Blobs are separate** — metadata in events, bytes in content-addressed store
8. **Signatures are pervasive** — every event is signed at creation
9. **Privacy is explicit** — overlay data stays local
10. **Operational state is independent** — lease, attempt, and blocked state do not imply workflow status
11. **Provenance is two-layered** — EmittedBy (event producer) is distinct from ProducedBy (content producer)
12. **Coordination is event-sourced** — leases, attempts, and findings are derived from events, not stored separately
13. **Operational conflict resolution** — concurrent lease lineages resolved deterministically; losing lineage preserved as non-authoritative history
14. **Eval is machine judgment, review is human judgment** — evals for autonomous loops, reviews for governance gates
