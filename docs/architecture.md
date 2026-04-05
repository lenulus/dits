# Architecture

DITS is structured around three principles: events are the source of truth, the DAG defines causality, and authority is minimal and intentional.

## System Layers

```
+---------------------------+
|   Private Overlay Layer   |  Local annotations, private labels (never synced)
+---------------------------+
|   Control Plane (Meta)    |  Kinds, workflows, policies, labels (versioned, synced)
+---------------------------+
|   Coordination Layer      |  Leases, attempts, checkpoints (materialized from events)
+---------------------------+
|   Data Plane (Events)     |  Append-only event DAG (distributed, synced)
+---------------------------+
|   Blob Store              |  Content-addressed artifact storage (synced separately)
+---------------------------+
```

## Event Sourcing

Every state change is an immutable **event**. Events are never modified or deleted. The current state of a work item is computed by replaying its events in causal order.

```
work.created -> title_set -> label_added -> status_set -> closed
     |                                           |
     +--- commented                              +--- reopened
     |
     +--- leased -> execution_started -> checkpointed -> execution_completed
```

Events carry:
- A globally unique ID (`evt_<ULID>`)
- A reference to the work item (`wrk_<ULID>`)
- Parent event IDs (forming the DAG)
- A meta version (for schema validation)
- An actor ID and timestamp
- A typed payload
- Optional EmittedBy provenance
- An Ed25519 signature

## DAG Structure

Events form a directed acyclic graph (DAG) through `parent_event_ids`. This captures causal relationships:

```
        evt_A (create)
       /     \
    evt_B     evt_C     <- concurrent edits
       \     /
        evt_D (merge)
```

**Heads** are events with no children — the current tips of the DAG for a work item. When a new event is created, the current heads become its parents.

### Storage

The DAG is stored in SQLite with two structures:

| Table | Purpose |
|-------|---------|
| `event_parents` | Edge list (source of truth) |
| `dag_heads` | Materialized leaf set (performance optimization) |

Head maintenance runs inside the same transaction as event insertion: remove parents from heads, add new event as head.

## Reducer

The **reducer** is a pure function: `[]Event -> *WorkItem`. Given causally-ordered events, it produces the materialized work item state.

### Causal Ordering (Kahn's Algorithm)

1. Build in-degree map from parent edges
2. Start with zero-in-degree events (the `work.created` event)
3. Process queue, decrementing children's in-degrees
4. When the queue has multiple candidates (concurrent events), tiebreak by:
   - Timestamp ascending
   - Event ID lexically ascending

This produces a deterministic total order from the partial order — the core of conflict resolution.

### Materialization

The `work_items` table is a **rebuildable cache**. It can always be reconstructed from events. Three materialization strategies:

| Strategy | When Used |
|----------|-----------|
| Full replay | After sync (re-reduce all events for affected work items) |
| Incremental apply | Local operations (single `ApplyEvent` call) |
| Rebuild | Manual rebuild (drop and recreate from events) |

### Conflict Resolution

Concurrent edits (same parent, different events) resolve automatically:

- **Scalar fields** (title, status, priority, kind): Last writer wins in causal order
- **Collection fields** (labels, assignees): Add/remove operations merge naturally
- **Comments, checkpoints, observations, findings**: Append-only, all preserved
- **Artifacts, relations, attempts**: Add/remove by unique ID, no conflicts possible
- **Flags** (blocked): Latest event wins
- **Coordination** (lease_holder, current_attempt): Latest event wins

## Coordination Layer

### Leases

Work items can be leased by actors. One active lease at a time per work item. Leases have expiration times and generation counters to prevent stale renewals.

The reducer materializes lease state from events (`work.leased`, `work.lease_released`, `work.lease_renewed`). Lease expiration (wall-clock comparison) is a query-time concern.

### Execution Attempts

A work item may have multiple execution attempts. Each attempt has a number (sequential), a unique AttemptID, and a status (running, completed, failed, abandoned). Checkpoints are recorded against the current attempt.

### Status Independence

Operational coordination state (lease ownership, active attempts, blocked flags) is first-class and independent of workflow status. Status is a human- and policy-facing abstraction. A work item can be `status=in_progress` while also blocked, leased, and on its third attempt.

## Provenance Model

Two distinct layers:

| Layer | Attached To | Purpose |
|-------|------------|---------|
| **EmittedBy** | Event struct | Who/what created and emitted the event (orchestration layer) |
| **ProducedBy** | Payload fields | Who/what produced the content (subject layer) |

An orchestration agent may emit an `evidence_attached` event for an artifact produced by a different tool or model.

## Sync Protocol

Sync uses a single HTTP endpoint: `POST /api/v1/sync`.

### Flow

```
Client                              Server
  |                                    |
  |--- BuildSyncRequest() ----------->|
  |    (local heads, missing events,  |
  |     meta, actor identity)         |
  |                                    |
  |    HandleSync():                   |
  |    1. Register actor               |
  |    2. Verify signatures (warn)     |
  |    3. Ingest client events         |
  |    4. Accept meta if newer         |
  |    5. Rematerialize affected       |
  |    6. Allocate shared IDs          |
  |    7. Compute missing events       |
  |    8. Store client heads           |
  |                                    |
  |<-- SyncResponse ------------------|
  |    (missing events, shared IDs,   |
  |     server heads, meta)           |
  |                                    |
  |--- ApplySync() (local)            |
  |    1. Ingest server events         |
  |    2. Rematerialize affected       |
  |    3. Apply shared IDs             |
  |    4. Update meta if newer         |
  |    5. Store server heads           |
```

### Finding Missing Events

Both client and server use the same algorithm: BFS backward from their heads through parent edges, stopping at the other side's known heads.

### Idempotency

`INSERT OR IGNORE` on the events table. Duplicate events from retries are silently dropped.

## Meta Configuration

Meta defines the project's structured vocabulary: work kinds, workflows, labels, priorities, artifact types, evidence types, relation types, lease policies, review policies.

### Versioning

Meta has a monotonically increasing version number. Every mutation bumps the version. Sync resolves by highest version wins.

### Validation

Events carry `meta_version`. Kinds, statuses, labels, priorities, artifact types, and relation types referenced in payloads must exist at that version. The CLI validates before creating events.

## Blob Store

Artifacts separate metadata from bytes:

| Component | Where | What |
|-----------|-------|------|
| Event payload | Event log | `artifact_id`, `content_hash`, `filename`, `mime_type`, `size_bytes`, `artifact_type`, `semantic_role`, `produced_by` |
| Blob bytes | Blob store | Actual file content, keyed by SHA256 hash |

### Content Addressing

Blobs are stored at `<root>/<first2>/<next2>/<full_hash>`. This gives deduplication, integrity verification, and atomic writes (temp file + rename).

### Sync

Blob transfer happens after event sync:
1. Client checks which pushed blob hashes the server is missing
2. Client uploads missing blobs
3. Client downloads blobs from pulled events it doesn't have locally

## Identity and Signatures

### Key Generation

`dits init` generates an Ed25519 keypair stored in `.dits/identity.json`. The actor ID is derived from the public key.

### Signing

Every event is signed before storage. The signature covers canonical JSON of the event (all fields except `signature`, sorted keys, no extra whitespace, includes `emitted_by` if present).

### Verification

The server verifies signatures in **warn mode**: invalid signatures are logged but events are not rejected.

## Private Overlay

The overlay layer provides local-only data that never touches the sync protocol:

| Feature | Storage | Purpose |
|---------|---------|---------|
| Annotations | `overlay_annotations` (key-value) | Personal notes, custom metadata |
| Private labels | `overlay_labels` | Personal categorization |

## SQLite Schema

Seven migrations build the schema incrementally:

| Migration | Tables |
|-----------|--------|
| 001_initial | events, event_parents, dag_heads, work_items, work_item_labels, work_item_assignees, work_item_comments, shared_id_counter, meta_config |
| 002_sync_state | sync_remotes, sync_remote_heads |
| 003_artifacts | work_item_artifacts |
| 004_actors | actors |
| 005_overlay | overlay_annotations, overlay_labels |
| 006_relations | work_item_relations |
| 007_coordination | work_item_checkpoints, work_item_observations, work_item_findings, work_item_attempts |

All tables use `CREATE TABLE IF NOT EXISTS` for idempotent migration.

## Package Dependencies

```
cmd/dits       --> cli --> domain, store, sync, crypto, blob, project
cmd/dits-server --> server --> domain, store, sync, blob, crypto

domain     --> (stdlib + oklog/ulid only)
store      --> domain
store/sqlite --> domain, store
sync       --> domain, store, crypto
blob       --> (stdlib only)
crypto     --> domain
server     --> domain, store, sync, blob
cli        --> domain, store, sync, blob, crypto, project
project    --> domain, store, blob, crypto, store/sqlite
```

The `domain` package has zero infrastructure dependencies — it defines types and pure logic only.
