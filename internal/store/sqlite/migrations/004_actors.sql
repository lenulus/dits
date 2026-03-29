CREATE TABLE IF NOT EXISTS actors (
    actor_id   TEXT PRIMARY KEY,
    public_key TEXT NOT NULL,
    node_id    TEXT,
    first_seen TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
