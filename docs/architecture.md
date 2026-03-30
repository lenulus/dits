# Architecture

DITS is structured around three principles: events are the source of truth, the DAG defines causality, and authority is minimal and intentional.

## System Layers

```
+---------------------------+
|   Private Overlay Layer   |  Local annotations, private labels (never synced)
+---------------------------+
|   Control Plane (Meta)    |  Labels, workflows, issue types (versioned, synced)
+---------------------------+
|   Data Plane (Events)     |  Append-only event DAG (distributed, synced)
+---------------------------+
|   Blob Store              |  Content-addressed file storage (synced separately)
+---------------------------+
```

## Event Sourcing

Every state change is an immutable **event**. Events are never modified or deleted. The current state of an issue is computed by replaying its events in causal order.

```
issue.created -> title_set -> label_added -> status_set -> closed
     |                                           |
     +--- comment_added                          +--- reopened
```

Events carry:
- A globally unique ID (`evt_<ULID>`)
- A reference to the issue (`iss_<ULID>`)
- Parent event IDs (forming the DAG)
- A meta version (for schema validation)
- An actor ID and timestamp
- A typed payload
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

**Heads** are events with no children — the current tips of the DAG for an issue. When a new event is created, the current heads become its parents.

### Storage

The DAG is stored in SQLite with two structures:

| Table | Purpose |
|-------|---------|
| `event_parents` | Edge list (source of truth) |
| `dag_heads` | Materialized leaf set (performance optimization) |

Head maintenance runs inside the same transaction as event insertion: remove parents from heads, add new event as head.

## Reducer

The **reducer** is a pure function: `[]Event -> *Issue`. Given causally-ordered events, it produces the materialized issue state.

### Causal Ordering (Kahn's Algorithm)

1. Build in-degree map from parent edges
2. Start with zero-in-degree events (the `issue.created` event)
3. Process queue, decrementing children's in-degrees
4. When the queue has multiple candidates (concurrent events), tiebreak by:
   - Timestamp ascending
   - Event ID lexically ascending

This produces a deterministic total order from the partial order — the core of conflict resolution.

### Materialization

The `issues` table is a **rebuildable cache**. It can always be reconstructed from events. Three materialization strategies:

| Strategy | When Used |
|----------|-----------|
| Full replay | After sync (re-reduce all events for affected issues) |
| Incremental apply | Local operations (single `ApplyEvent` call) |
| Rebuild | Manual `dits rebuild` (drop and recreate from events) |

### Conflict Resolution

Concurrent edits (same parent, different events) resolve automatically:

- **Set fields** (title, status, priority): Last writer wins in causal order
- **Collection fields** (labels, assignees): Add/remove operations merge naturally
- **Comments**: Append-only, all preserved
- **Attachments**: Add/remove by ID, no conflicts possible

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

Both client and server use the same algorithm: BFS backward from their heads through parent edges, stopping at the other side's known heads. All visited events (minus the stop set) are what needs to be sent.

### Idempotency

`INSERT OR IGNORE` on the events table (primary key is EventID). Duplicate events from retries are silently dropped.

### Head Tracking

Each node maintains:
- Its own DAG heads (`dag_heads` table)
- Per-remote head knowledge (`sync_remote_heads`)

After sync, the client stores the server's heads. On the next sync, "events to push" = events reachable from local heads but not ancestors of last-known server heads.

## Meta Configuration

Meta defines the project's structured vocabulary: labels, workflows, issue types, priorities.

### Versioning

Meta has a monotonically increasing version number. Every mutation bumps the version:

```
Version 1: default (open, in_progress, closed; task, bug)
Version 2: + label "bug"
Version 3: + label "feature"
Version 4: + issue type "epic"
```

### Validation

Events carry `meta_version`. Labels and statuses referenced in payloads must exist at that version. The CLI validates before creating events.

### Sync

Meta syncs bidirectionally. Highest version wins:
- Client pushes meta in SyncRequest if it has updates
- Server accepts if client version > server version
- Server sends meta in SyncResponse if it has a newer version

### Optimistic Concurrency

`SaveMetaIfVersion` checks that the current HEAD version matches before saving. Prevents concurrent modifications from overwriting each other.

## Blob Store

Attachments separate metadata from bytes:

| Component | Where | What |
|-----------|-------|------|
| Event payload | Event log | `attachment_id`, `content_hash`, `filename`, `mime_type`, `size_bytes` |
| Blob bytes | Blob store | Actual file content, keyed by SHA256 hash |

### Content Addressing

Blobs are stored at `<root>/<first2>/<next2>/<full_hash>`. This gives:
- Deduplication (same content = same hash = stored once)
- Integrity verification (hash checked on write)
- Atomic writes (temp file + rename)

### Sync

Blob transfer happens after event sync:

1. Client checks which pushed blob hashes the server is missing (`POST /api/v1/blobs/check`)
2. Client uploads missing blobs (`PUT /api/v1/blobs/{hash}`)
3. Client downloads blobs from pulled events it doesn't have locally (`GET /api/v1/blobs/{hash}`)

## Identity and Signatures

### Key Generation

`dits init` generates an Ed25519 keypair stored in `.dits/identity.json`. The actor ID is derived from the public key: `actor_<first 8 bytes of public key hex>`.

### Signing

Every event is signed before storage. The signature covers canonical JSON of the event (all fields except `signature`, sorted keys, no extra whitespace).

### Verification

The server verifies signatures in **warn mode**: invalid signatures are logged but events are not rejected. This allows gradual rollout.

### Actor Registry

On sync, clients send their actor ID and public key. The server stores these in the `actors` table for verification.

## Private Overlay

The overlay layer provides local-only data that never touches the sync protocol:

| Feature | Storage | Purpose |
|---------|---------|---------|
| Annotations | `overlay_annotations` (key-value) | Personal notes, custom metadata |
| Private labels | `overlay_labels` | Personal categorization |

Overlay data is per-client. Two clients syncing through the same server maintain independent overlays.

## SQLite Schema

Six migrations build the schema incrementally:

| Migration | Tables |
|-----------|--------|
| 001_initial | events, event_parents, dag_heads, issues, issue_labels, issue_assignees, issue_comments, shared_id_counter, meta_config |
| 002_sync_state | sync_remotes, sync_remote_heads |
| 003_attachments | issue_attachments |
| 004_actors | actors |
| 005_overlay | overlay_annotations, overlay_labels |
| 006_relations | issue_relations |

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
