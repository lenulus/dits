CREATE TABLE IF NOT EXISTS issue_attachments (
    attachment_id TEXT PRIMARY KEY,
    issue_id      TEXT NOT NULL REFERENCES issues(id),
    content_hash  TEXT NOT NULL,
    filename      TEXT NOT NULL,
    mime_type     TEXT NOT NULL DEFAULT '',
    size_bytes    INTEGER NOT NULL DEFAULT 0,
    added_by      TEXT NOT NULL,
    added_at      TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_issue_attachments_issue ON issue_attachments(issue_id);
CREATE INDEX IF NOT EXISTS idx_issue_attachments_hash ON issue_attachments(content_hash);
