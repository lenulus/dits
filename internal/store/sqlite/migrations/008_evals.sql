CREATE TABLE IF NOT EXISTS work_item_evals (
    eval_id       TEXT NOT NULL,
    event_id      TEXT PRIMARY KEY REFERENCES events(id),
    work_item_id  TEXT NOT NULL REFERENCES work_items(id),
    subject_ref   TEXT NOT NULL DEFAULT '',
    metrics       TEXT,
    verdict       TEXT NOT NULL DEFAULT '',
    produced_by   TEXT,
    timestamp     TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_work_item_evals_wi ON work_item_evals(work_item_id);
