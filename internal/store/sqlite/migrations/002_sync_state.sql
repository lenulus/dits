-- Sync remotes
CREATE TABLE IF NOT EXISTS sync_remotes (
    node_id      TEXT PRIMARY KEY,
    url          TEXT,
    last_sync_at TEXT
);

-- Per-remote head tracking
CREATE TABLE IF NOT EXISTS sync_remote_heads (
    node_id  TEXT NOT NULL REFERENCES sync_remotes(node_id),
    event_id TEXT NOT NULL,
    PRIMARY KEY (node_id, event_id)
);
