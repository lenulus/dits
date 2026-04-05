CREATE TABLE IF NOT EXISTS overlay_annotations (
    work_item_id TEXT NOT NULL,
    key          TEXT NOT NULL,
    value        TEXT NOT NULL,
    PRIMARY KEY (work_item_id, key)
);

CREATE TABLE IF NOT EXISTS overlay_labels (
    work_item_id TEXT NOT NULL,
    label        TEXT NOT NULL,
    PRIMARY KEY (work_item_id, label)
);
