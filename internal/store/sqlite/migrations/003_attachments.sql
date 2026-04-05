CREATE TABLE IF NOT EXISTS work_item_artifacts (
    artifact_id   TEXT PRIMARY KEY,
    work_item_id  TEXT NOT NULL REFERENCES work_items(id),
    content_hash  TEXT NOT NULL,
    filename      TEXT NOT NULL,
    mime_type     TEXT NOT NULL DEFAULT '',
    size_bytes    INTEGER NOT NULL DEFAULT 0,
    artifact_type TEXT NOT NULL DEFAULT '',
    semantic_role TEXT NOT NULL DEFAULT '',
    produced_by   TEXT,
    added_by      TEXT NOT NULL,
    added_at      TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_work_item_artifacts_wi ON work_item_artifacts(work_item_id);
CREATE INDEX IF NOT EXISTS idx_work_item_artifacts_hash ON work_item_artifacts(content_hash);
