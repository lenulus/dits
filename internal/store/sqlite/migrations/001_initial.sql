-- Events (source of truth)
CREATE TABLE IF NOT EXISTS events (
    id            TEXT PRIMARY KEY,
    work_item_id  TEXT NOT NULL,
    type          TEXT NOT NULL,
    payload       TEXT NOT NULL,
    meta_version  INTEGER NOT NULL DEFAULT 0,
    actor_id      TEXT NOT NULL,
    timestamp     TEXT NOT NULL,
    emitted_by    TEXT,
    signature     BLOB,
    created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
CREATE INDEX IF NOT EXISTS idx_events_work_item_id ON events(work_item_id);
CREATE INDEX IF NOT EXISTS idx_events_timestamp ON events(timestamp);

-- DAG parent edges
CREATE TABLE IF NOT EXISTS event_parents (
    event_id   TEXT NOT NULL REFERENCES events(id),
    parent_id  TEXT NOT NULL REFERENCES events(id),
    PRIMARY KEY (event_id, parent_id)
);
CREATE INDEX IF NOT EXISTS idx_event_parents_parent ON event_parents(parent_id);

-- DAG heads (materialized leaf set)
CREATE TABLE IF NOT EXISTS dag_heads (
    work_item_id TEXT NOT NULL,
    event_id     TEXT NOT NULL REFERENCES events(id),
    PRIMARY KEY (work_item_id, event_id)
);

-- Materialized work items
CREATE TABLE IF NOT EXISTS work_items (
    id              TEXT PRIMARY KEY,
    shared_id       TEXT UNIQUE,
    kind            TEXT NOT NULL DEFAULT 'task',
    title           TEXT NOT NULL,
    body            TEXT NOT NULL DEFAULT '',
    status          TEXT NOT NULL DEFAULT 'open',
    priority        TEXT NOT NULL DEFAULT 'medium',
    lease_holder    TEXT,
    lease_expires_at TEXT,
    current_attempt TEXT,
    blocked         INTEGER NOT NULL DEFAULT 0,
    blocked_reason  TEXT NOT NULL DEFAULT '',
    created_by      TEXT NOT NULL,
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL,
    closed_at       TEXT,
    event_count     INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_work_items_status ON work_items(status);
CREATE INDEX IF NOT EXISTS idx_work_items_updated_at ON work_items(updated_at);
CREATE INDEX IF NOT EXISTS idx_work_items_kind ON work_items(kind);

CREATE TABLE IF NOT EXISTS work_item_labels (
    work_item_id TEXT NOT NULL REFERENCES work_items(id),
    label_slug   TEXT NOT NULL,
    PRIMARY KEY (work_item_id, label_slug)
);

CREATE TABLE IF NOT EXISTS work_item_assignees (
    work_item_id TEXT NOT NULL REFERENCES work_items(id),
    actor_id     TEXT NOT NULL,
    PRIMARY KEY (work_item_id, actor_id)
);

CREATE TABLE IF NOT EXISTS work_item_comments (
    event_id     TEXT PRIMARY KEY REFERENCES events(id),
    work_item_id TEXT NOT NULL REFERENCES work_items(id),
    actor_id     TEXT NOT NULL,
    body         TEXT NOT NULL,
    produced_by  TEXT,
    timestamp    TEXT NOT NULL
);

-- Shared ID counter
CREATE TABLE IF NOT EXISTS shared_id_counter (
    project_key TEXT PRIMARY KEY,
    next_id     INTEGER NOT NULL DEFAULT 1
);

-- Meta configuration
CREATE TABLE IF NOT EXISTS meta_config (
    version     INTEGER PRIMARY KEY,
    config_json TEXT NOT NULL,
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
