CREATE TABLE IF NOT EXISTS overlay_annotations (
    issue_id TEXT NOT NULL,
    key      TEXT NOT NULL,
    value    TEXT NOT NULL,
    PRIMARY KEY (issue_id, key)
);

CREATE TABLE IF NOT EXISTS overlay_labels (
    issue_id TEXT NOT NULL,
    label    TEXT NOT NULL,
    PRIMARY KEY (issue_id, label)
);
