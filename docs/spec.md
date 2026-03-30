# DITS Specification

**Version:** 1.0
**Status:** Implemented

---

## 1. Overview

DITS (Distributed Issue Tracking System) is a hybrid, local-first issue tracking system that combines:

- Append-only, event-sourced issue history
- Coordinated identity and schema (meta configuration)
- Deterministic synchronization and reconciliation
- Content-addressed attachment storage
- Cryptographic event signatures

The system separates:

- **Data Plane** (issue events) — distributed, mergeable
- **Control Plane** (meta configuration) — coordinated, versioned
- **Private Layer** (overlay) — non-replicated user state
- **Blob Store** (attachments) — content-addressed, synced separately

---

## 2. Identity Model

### 2.1 Canonical Issue ID

Globally unique, client-generated. Format: `iss_<ULID>`.

Properties: immutable, never reused, primary reference key.

### 2.2 Shared Issue ID

Server-assigned, human-friendly. Format: `<PROJECT_KEY>-<N>` (e.g., `PROJ-1423`).

Properties: monotonic, stable, may contain gaps, never reused.

### 2.3 Event ID

Globally unique, client-generated. Format: `evt_<ULID>`.

### 2.4 Actor ID

Derived from Ed25519 public key. Format: `actor_<first 8 bytes of public key hex>`.

### 2.5 Node ID

Identifies a DITS instance. Format: `node_<ULID>`.

### 2.6 Attachment ID

Client-generated. Format: `att_<ULID>`.

### 2.7 ULID Generation

All ULIDs use a monotonic entropy source to prevent collisions at sub-millisecond creation rates. Entropy is reset if exhausted.

---

## 3. Event Model

### 3.1 Event Structure

All state changes are recorded as immutable events.

```json
{
  "id": "evt_01...",
  "issue_id": "iss_01...",
  "type": "issue.status_set",
  "parent_event_ids": ["evt_00..."],
  "meta_version": 3,
  "actor_id": "actor_abc...",
  "timestamp": "2026-03-29T10:00:00Z",
  "payload": {"from": "open", "to": "in_progress"},
  "signature": "<base64>"
}
```

### 3.2 Event Types

| Type | Payload | Description |
|------|---------|-------------|
| `issue.created` | `{title, body?, type_slug?}` | Creates a new issue |
| `issue.title_set` | `{title}` | Updates issue title |
| `issue.body_set` | `{body}` | Updates issue body |
| `issue.status_set` | `{from, to}` | Changes issue status |
| `issue.label_added` | `{label_slug}` | Adds a label |
| `issue.label_removed` | `{label_slug}` | Removes a label |
| `issue.assigned` | `{assignee}` | Assigns an actor |
| `issue.unassigned` | `{assignee}` | Unassigns an actor |
| `issue.commented` | `{body}` | Adds a comment |
| `issue.priority_set` | `{priority}` | Changes priority |
| `issue.closed` | `{}` | Closes the issue |
| `issue.reopened` | `{}` | Reopens the issue |
| `issue.shared_id_assigned` | `{shared_id}` | Server assigns shared ID |
| `issue.attachment_added` | `{attachment_id, content_hash, filename, mime_type, size_bytes}` | Attaches a file |
| `issue.attachment_removed` | `{attachment_id}` | Removes an attachment |
| `issue.linked` | `{relation_type, target_issue}` | Links to another issue |
| `issue.unlinked` | `{relation_type, target_issue}` | Removes a link |

### 3.3 Event Rules

- Events are **immutable** — never modified or deleted.
- Events are **append-only** — new events are always added, never replace existing ones.
- Event IDs are **globally unique** — ULID with monotonic entropy prevents collisions.
- Events MUST reference their parent events via `parent_event_ids`.
- The first event for an issue (`issue.created`) has an empty parent list.
- Events MUST carry the `meta_version` of the meta config at creation time.
- Labels and statuses referenced in payloads MUST exist at the event's `meta_version`.

---

## 4. DAG Structure

### 4.1 Definition

Events form a directed acyclic graph (DAG) through `parent_event_ids`. Each event references zero or more parent events.

### 4.2 Heads

Heads are events that are not a parent of any other event. They represent the current tips of the DAG for an issue.

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
| Scalar (title, status, priority) | Last writer wins in causal order |
| Set (labels, assignees) | Add/remove operations merge |
| Append-only (comments) | All preserved in order |
| By-ID (attachments, relations) | Add/remove by unique ID |

---

## 5. Meta Configuration

### 5.1 Structure

```json
{
  "version": 3,
  "project_key": "PROJ",
  "labels": [{"slug": "bug", "name": "Bug", "color": "#ff0000"}],
  "workflows": [{
    "slug": "default",
    "name": "Default",
    "statuses": [
      {"slug": "open", "name": "Open", "category": "open"},
      {"slug": "in_progress", "name": "In Progress", "category": "in_progress"},
      {"slug": "closed", "name": "Closed", "category": "done"}
    ],
    "transitions": [{"from": "*", "to": "open"}, {"from": "*", "to": "in_progress"}, {"from": "*", "to": "closed"}]
  }],
  "issue_types": [{"slug": "task", "name": "Task", "workflow_slug": "default"}],
  "priorities": ["low", "medium", "high", "critical"]
}
```

### 5.2 Versioning Rules

- Meta version is monotonically increasing.
- Every mutation increments the version.
- Local updates use optimistic concurrency: `SaveMetaIfVersion` checks that `current_version == expected_version`.
- Sync resolves by highest version wins.

### 5.3 Validation

Events are validated against meta at creation time:

- `issue.created`: `type_slug` must exist in `issue_types`
- `issue.status_set`: `to` status must exist in a workflow
- `issue.label_added`: `label_slug` must exist in `labels`
- `issue.priority_set`: `priority` must exist in `priorities`

Historical events remain valid against the meta version they were created with.

---

## 6. Synchronization Protocol

### 6.1 Endpoint

```
POST /api/v1/sync
```

### 6.2 Request

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

### 6.3 Response

```json
{
  "events": [],
  "shared_ids": {"iss_01...": "PROJ-42"},
  "heads": ["evt_03...", "evt_04..."],
  "meta": null
}
```

### 6.4 Algorithm

**Server (HandleSync):**
1. Register actor (store public key)
2. Verify signatures (warn mode)
3. Ingest client events (`INSERT OR IGNORE`)
4. Accept client meta if version > server version
5. Rematerialize affected issues
6. Allocate shared IDs to new issues
7. Compute events client is missing (BFS from server heads, stop at client heads)
8. Include shared IDs for pulled issues
9. Store client heads as remote state
10. Return response with meta if server version > client version

**Client (ApplySync):**
1. Ingest server events
2. Rematerialize affected issues
3. Apply shared IDs
4. Update meta if server version > local version
5. Store server heads

### 6.5 Missing Event Detection

BFS backward from sender's heads through `parent_event_ids`, stopping at receiver's known heads. All visited events (minus the stop set) are what needs to be transferred.

### 6.6 Idempotency

Events use `INSERT OR IGNORE` on primary key (event ID). Duplicate events from retries are silently dropped. Sync is safe to retry.

---

## 7. Attachments

### 7.1 Principles

- Attachment **metadata** is recorded as issue events.
- Attachment **bytes** are stored in a separate content-addressed blob store.
- Shared sync verifies blob presence separately from event ingestion.
- Removing an attachment removes the reference, not the blob.

### 7.2 Content Addressing

Blobs are keyed by `sha256:<hex>`. Storage path: `<root>/<first2>/<next2>/<full_hash>`.

Properties: deduplication, integrity verification, atomic writes (temp file + rename).

### 7.3 Size Limit

Maximum blob size: 50MB. Enforced at upload time.

### 7.4 Blob Sync

After event sync:
1. Check which pushed hashes the server is missing (`POST /api/v1/blobs/check`)
2. Upload missing blobs (`PUT /api/v1/blobs/{hash}`)
3. Download blobs from pulled events (`GET /api/v1/blobs/{hash}`)

### 7.5 Blob API

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/v1/blobs/check` | Check hash presence |
| PUT | `/api/v1/blobs/{hash}` | Upload blob (verified) |
| GET | `/api/v1/blobs/{hash}` | Download blob |

---

## 8. Signatures

### 8.1 Algorithm

Ed25519. Keypair generated on `dits init`.

### 8.2 Canonical JSON

Signature covers canonical JSON of the event: all fields except `signature`, sorted keys, no extra whitespace. Timestamp formatted as `2006-01-02T15:04:05.999999999Z`.

### 8.3 Verification

Server verifies in **warn mode**: invalid signatures are logged but events are not rejected.

### 8.4 Actor Registry

Clients send `actor_id` and `public_key` in sync requests. Server stores in `actors` table.

---

## 9. Private Overlay

### 9.1 Principle

Overlay data is local-only and never synced.

### 9.2 Types

| Type | Storage | Description |
|------|---------|-------------|
| Annotations | Key-value pairs per issue | Personal notes, custom metadata |
| Private labels | String set per issue | Personal categorization |

### 9.3 Isolation

Each client maintains its own overlay. Syncing does not transfer overlay data.

---

## 10. Shared ID Allocation

### 10.1 Mechanism

Server maintains a per-project counter. On sync, new issues without shared IDs get the next available number.

### 10.2 Properties

- Monotonic (always increasing)
- May have gaps (due to concurrent allocation)
- Never reused
- Format: `<PROJECT_KEY>-<N>`

---

## 11. Server Role

The server is responsible for:
- Shared ID allocation
- Meta validation (highest version wins)
- Signature verification (warn mode)
- Blob storage
- Event relay between clients

The server is NOT:
- The sole source of truth (clients have full event history)
- The owner of issue state (state is derived from events)
- Required for local operations (create, edit, list all work offline)

---

## 12. Storage

### 12.1 Client

SQLite database at `.dits/dits.db`. Blob store at `.dits/blobs/`.

### 12.2 Server

SQLite database (configurable path). Blob store (configurable directory).

### 12.3 Schema

Six incremental migrations:

| # | Tables |
|---|--------|
| 001 | events, event_parents, dag_heads, issues, issue_labels, issue_assignees, issue_comments, shared_id_counter, meta_config |
| 002 | sync_remotes, sync_remote_heads |
| 003 | issue_attachments |
| 004 | actors |
| 005 | overlay_annotations, overlay_labels |
| 006 | issue_relations |

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
9. **Privacy is explicit** — overlay data stays local, sensitive work uses separate repos
