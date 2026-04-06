# API Reference

Base URL: `http://<host>:<port>` (default port 8484)

---

## Health

### GET /api/v1/health

```bash
curl http://localhost:8484/api/v1/health
```

**Response:**
```json
{"status": "ok"}
```

---

## Sync

### POST /api/v1/sync

Primary synchronization endpoint. Bidirectional event exchange.

```bash
curl -X POST http://localhost:8484/api/v1/sync \
  -H "Content-Type: application/json" \
  -d '{"node_id":"node_01...","project_key":"PROJ","heads":[],"events":[],"meta_version":1}'
```

**Request Body:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `node_id` | string | Yes | Client node identifier |
| `project_key` | string | Yes | Project key |
| `heads` | string[] | Yes | Client's current DAG head event IDs |
| `events` | Event[] | Yes | Events to push (may be empty) |
| `meta_version` | integer | Yes | Client's current meta version |
| `meta` | MetaConfig | No | Client's meta config (if updated) |
| `actor_id` | string | No | Client's actor ID (for registration) |
| `public_key` | string | No | Client's Ed25519 public key (hex) |

**Response Body:**

| Field | Type | Description |
|-------|------|-------------|
| `events` | Event[] | Events the client is missing |
| `shared_ids` | object | Map of work item ID -> shared ID |
| `heads` | string[] | Server's current DAG head event IDs |
| `meta` | MetaConfig | Server's meta config (if newer than client's) |

**Event Object:**

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | Event ID (`evt_<ULID>`) |
| `work_item_id` | string | Work item ID (`wrk_<ULID>`) |
| `type` | string | Event type (e.g., `work.created`) |
| `parent_event_ids` | string[] | Parent event IDs in the DAG |
| `meta_version` | integer | Meta version at creation time |
| `actor_id` | string | Actor who created the event |
| `timestamp` | string | RFC3339 timestamp (UTC) |
| `payload` | object | Type-specific payload |
| `emitted_by` | object | Optional event-level provenance |
| `signature` | string | Base64-encoded Ed25519 signature |

---

## Blobs

### POST /api/v1/blobs/check

Check which blob hashes exist on the server.

```bash
curl -X POST http://localhost:8484/api/v1/blobs/check \
  -H "Content-Type: application/json" \
  -d '{"hashes":["sha256:abcdef...","sha256:123456..."]}'
```

**Response:**
```json
{"present": ["sha256:abcdef..."], "missing": ["sha256:123456..."]}
```

### PUT /api/v1/blobs/{hash}

Upload a blob. Max 50MB. Server verifies content matches hash.

```bash
curl -X PUT http://localhost:8484/api/v1/blobs/sha256:abcdef... \
  -H "Content-Type: application/octet-stream" \
  --data-binary @file.png
```

### GET /api/v1/blobs/{hash}

Download a blob.

```bash
curl http://localhost:8484/api/v1/blobs/sha256:abcdef... -o file.png
```

---

## v2 Query API

Agent-oriented read-only endpoints for querying work items, events, and coordination state.

### GET /api/v2/work

List work items with filtering.

```bash
# All work items
curl http://localhost:8484/api/v2/work

# Filter by kind
curl http://localhost:8484/api/v2/work?kind=execution

# Filter by status
curl http://localhost:8484/api/v2/work?status=open

# Ready items (open, unleased, unblocked)
curl http://localhost:8484/api/v2/work?ready=true

# Blocked items
curl http://localhost:8484/api/v2/work?blocked=true

# Claimed by actor
curl http://localhost:8484/api/v2/work?claimed_by=actor_abc...

# Free-text search
curl http://localhost:8484/api/v2/work?q=deploy
```

**Query Parameters:**

| Parameter | Description |
|-----------|-------------|
| `kind` | Filter by work kind |
| `status` | Filter by status |
| `label` | Filter by label slug |
| `claimed_by` | Filter by lease holder (actor ID) |
| `blocked` | `true` or `false` |
| `ready` | `true` — open-category status, no lease, not blocked |
| `q` | Free-text search on title and body |
| `limit` | Max results |
| `offset` | Pagination offset |

**Response:**
```json
{
  "count": 2,
  "work_items": [...]
}
```

### GET /api/v2/work/{id}

Get full work item details including all collections.

```bash
curl http://localhost:8484/api/v2/work/PROJ-1
```

Accepts shared ID (`PROJ-1`) or canonical ID (`wrk_01...`).

### GET /api/v2/work/{id}/events

Event stream for a work item.

```bash
curl http://localhost:8484/api/v2/work/PROJ-1/events
curl http://localhost:8484/api/v2/work/PROJ-1/events?type=work.checkpointed
curl http://localhost:8484/api/v2/work/PROJ-1/events?since=2026-04-01T00:00:00Z
```

| Parameter | Description |
|-----------|-------------|
| `type` | Filter by event type |
| `since` | Events after this RFC3339 timestamp |

### GET /api/v2/work/{id}/artifacts

Artifacts for a work item, optionally filtered.

```bash
curl http://localhost:8484/api/v2/work/PROJ-1/artifacts
curl http://localhost:8484/api/v2/work/PROJ-1/artifacts?type=log
curl http://localhost:8484/api/v2/work/PROJ-1/artifacts?role=evidence
```

| Parameter | Description |
|-----------|-------------|
| `type` | Filter by artifact type |
| `role` | Filter by semantic role |

### GET /api/v2/work/{id}/attempts

Execution attempts for a work item.

```bash
curl http://localhost:8484/api/v2/work/PROJ-1/attempts
```

### GET /api/v2/work/{id}/evals

Evals for a work item, optionally filtered.

```bash
curl http://localhost:8484/api/v2/work/PROJ-1/evals
curl http://localhost:8484/api/v2/work/PROJ-1/evals?subject_kind=artifact
curl http://localhost:8484/api/v2/work/PROJ-1/evals?verdict=pass
```

| Parameter | Description |
|-----------|-------------|
| `subject_kind` | Filter by subject kind (work_item, artifact, attempt, finding, plan) |
| `verdict` | Filter by verdict (pass, fail, partial) |

### GET /api/v2/work/{id}/checkpoints

Checkpoints for a work item, optionally filtered by attempt.

```bash
curl http://localhost:8484/api/v2/work/PROJ-1/checkpoints
curl http://localhost:8484/api/v2/work/PROJ-1/checkpoints?attempt_id=atp_01...
```

### GET /api/v2/events

Cross-item event query.

```bash
curl http://localhost:8484/api/v2/events
curl http://localhost:8484/api/v2/events?type=work.created
curl http://localhost:8484/api/v2/events?actor_id=actor_abc...
curl http://localhost:8484/api/v2/events?since=2026-04-01T00:00:00Z&limit=50
```

| Parameter | Description |
|-----------|-------------|
| `type` | Filter by event type |
| `actor_id` | Filter by actor |
| `since` | Events after this RFC3339 timestamp |
| `limit` | Max results (default 100) |

### GET /api/v2/meta

Get current meta configuration.

```bash
curl http://localhost:8484/api/v2/meta
```

**Response:**
```json
{
  "version": 1,
  "project_key": "PROJ",
  "work_kinds": [...],
  "workflows": [...],
  "priorities": ["low", "medium", "high", "critical"],
  "labels": [],
  "artifact_types": [...],
  "evidence_types": [...],
  "relation_types": [...],
  "lease_policies": [...],
  "review_policies": []
}
```

---

## Error Responses

All errors return JSON:

```json
{"error": "description of what went wrong"}
```

| Status | Meaning |
|--------|---------|
| 400 | Bad request (malformed JSON, hash mismatch, validation failure) |
| 404 | Not found (work item, blob, or meta doesn't exist) |
| 500 | Internal server error |
