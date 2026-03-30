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
| `shared_ids` | object | Map of canonical ID -> shared ID for new/pulled issues |
| `heads` | string[] | Server's current DAG head event IDs |
| `meta` | MetaConfig | Server's meta config (if newer than client's) |

**Event Object:**

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | Event ID (`evt_<ULID>`) |
| `issue_id` | string | Issue canonical ID (`iss_<ULID>`) |
| `type` | string | Event type (e.g., `issue.created`) |
| `parent_event_ids` | string[] | Parent event IDs in the DAG |
| `meta_version` | integer | Meta version at creation time |
| `actor_id` | string | Actor who created the event |
| `timestamp` | string | RFC3339 timestamp (UTC) |
| `payload` | object | Type-specific payload |
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

**Request Body:**

| Field | Type | Description |
|-------|------|-------------|
| `hashes` | string[] | Content hashes to check |

**Response Body:**

| Field | Type | Description |
|-------|------|-------------|
| `present` | string[] | Hashes that exist on server |
| `missing` | string[] | Hashes that need uploading |

### PUT /api/v1/blobs/{hash}

Upload a blob.

```bash
curl -X PUT http://localhost:8484/api/v1/blobs/sha256:abcdef... \
  -H "Content-Type: application/octet-stream" \
  --data-binary @file.png
```

**Path Parameters:**

| Parameter | Description |
|-----------|-------------|
| `hash` | Content hash (`sha256:<hex>`) |

**Request Body:** Raw binary data. Max 50MB.

**Response:** `{"status": "ok", "hash": "sha256:..."}` (200) or error (400).

The server verifies the uploaded content matches the hash. Mismatches are rejected.

### GET /api/v1/blobs/{hash}

Download a blob.

```bash
curl http://localhost:8484/api/v1/blobs/sha256:abcdef... -o file.png
```

**Response:** Raw binary data with `Content-Type: application/octet-stream`. 404 if not found.

---

## Query API

Read-only endpoints for querying issues and meta configuration.

### GET /api/v1/issues

List issues with optional filtering.

```bash
# All issues
curl http://localhost:8484/api/v1/issues

# Filter by status
curl http://localhost:8484/api/v1/issues?status=open

# Filter by label
curl http://localhost:8484/api/v1/issues?label=bug

# Free-text search
curl http://localhost:8484/api/v1/issues?q=login
```

**Query Parameters:**

| Parameter | Description |
|-----------|-------------|
| `status` | Filter by status (e.g., `open`, `closed`) |
| `label` | Filter by label slug |
| `q` | Free-text search on title and body |

**Response:**
```json
{
  "count": 2,
  "issues": [
    {
      "id": "iss_01...",
      "shared_id": "PROJ-1",
      "title": "Fix login",
      "status": "open",
      "type": "task",
      "priority": "medium",
      "labels": ["bug"],
      "created_by": "actor_abc...",
      "created_at": "2026-03-29T10:00:00Z",
      "updated_at": "2026-03-29T12:00:00Z"
    }
  ]
}
```

### GET /api/v1/issues/{id}

Get full issue details.

```bash
curl http://localhost:8484/api/v1/issues/PROJ-1
```

**Path Parameters:**

| Parameter | Description |
|-----------|-------------|
| `id` | Shared ID (`PROJ-1`) or canonical ID (`iss_01...`) |

**Response:** Full issue object including comments, attachments, relations, assignees, labels.

### GET /api/v1/meta

Get current meta configuration.

```bash
curl http://localhost:8484/api/v1/meta
```

**Response:**
```json
{
  "version": 3,
  "project_key": "PROJ",
  "labels": [
    {"slug": "bug", "name": "Bug", "color": "#ff0000"}
  ],
  "workflows": [
    {
      "slug": "default",
      "name": "Default",
      "statuses": [
        {"slug": "open", "name": "Open", "category": "open"},
        {"slug": "in_progress", "name": "In Progress", "category": "in_progress"},
        {"slug": "closed", "name": "Closed", "category": "done"}
      ],
      "transitions": [
        {"from": "*", "to": "open"},
        {"from": "*", "to": "in_progress"},
        {"from": "*", "to": "closed"}
      ]
    }
  ],
  "issue_types": [
    {"slug": "task", "name": "Task", "workflow_slug": "default"},
    {"slug": "bug", "name": "Bug", "workflow_slug": "default"}
  ],
  "priorities": ["low", "medium", "high", "critical"]
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
| 404 | Not found (issue, blob, or meta doesn't exist) |
| 500 | Internal server error |
