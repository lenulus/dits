CREATE TABLE IF NOT EXISTS work_item_outcomes (
    event_id      TEXT PRIMARY KEY REFERENCES events(id),
    work_item_id  TEXT NOT NULL REFERENCES work_items(id),
    subject_kind  TEXT NOT NULL,
    subject_ref   TEXT NOT NULL,
    decision      TEXT NOT NULL,
    reason        TEXT NOT NULL DEFAULT '',
    eval_ref      TEXT NOT NULL DEFAULT '',
    actor_id      TEXT NOT NULL,
    timestamp     TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_work_item_outcomes_wi ON work_item_outcomes(work_item_id);
