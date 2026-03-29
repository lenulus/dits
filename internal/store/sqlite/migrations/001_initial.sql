-- Events (source of truth)
CREATE TABLE IF NOT EXISTS events (
    id            TEXT PRIMARY KEY,
    issue_id      TEXT NOT NULL,
    type          TEXT NOT NULL,
    payload       TEXT NOT NULL,
    meta_version  INTEGER NOT NULL DEFAULT 0,
    actor_id      TEXT NOT NULL,
    timestamp     TEXT NOT NULL,
    signature     BLOB,
    created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
CREATE INDEX IF NOT EXISTS idx_events_issue_id ON events(issue_id);
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
    issue_id TEXT NOT NULL,
    event_id TEXT NOT NULL REFERENCES events(id),
    PRIMARY KEY (issue_id, event_id)
);

-- Materialized issues
CREATE TABLE IF NOT EXISTS issues (
    id          TEXT PRIMARY KEY,
    shared_id   TEXT UNIQUE,
    title       TEXT NOT NULL,
    body        TEXT NOT NULL DEFAULT '',
    status      TEXT NOT NULL DEFAULT 'open',
    type_slug   TEXT NOT NULL DEFAULT 'task',
    priority    TEXT NOT NULL DEFAULT 'medium',
    created_by  TEXT NOT NULL,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL,
    closed_at   TEXT,
    event_count INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_issues_status ON issues(status);
CREATE INDEX IF NOT EXISTS idx_issues_updated_at ON issues(updated_at);

CREATE TABLE IF NOT EXISTS issue_labels (
    issue_id   TEXT NOT NULL REFERENCES issues(id),
    label_slug TEXT NOT NULL,
    PRIMARY KEY (issue_id, label_slug)
);

CREATE TABLE IF NOT EXISTS issue_assignees (
    issue_id TEXT NOT NULL REFERENCES issues(id),
    actor_id TEXT NOT NULL,
    PRIMARY KEY (issue_id, actor_id)
);

CREATE TABLE IF NOT EXISTS issue_comments (
    event_id  TEXT PRIMARY KEY REFERENCES events(id),
    issue_id  TEXT NOT NULL REFERENCES issues(id),
    actor_id  TEXT NOT NULL,
    body      TEXT NOT NULL,
    timestamp TEXT NOT NULL
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
