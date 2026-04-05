CREATE TABLE IF NOT EXISTS work_item_checkpoints (
    event_id      TEXT PRIMARY KEY REFERENCES events(id),
    work_item_id  TEXT NOT NULL REFERENCES work_items(id),
    attempt_id    TEXT NOT NULL,
    summary       TEXT NOT NULL DEFAULT '',
    progress      REAL NOT NULL DEFAULT 0,
    next_step     TEXT NOT NULL DEFAULT '',
    data          TEXT,
    timestamp     TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_work_item_checkpoints_wi ON work_item_checkpoints(work_item_id);

CREATE TABLE IF NOT EXISTS work_item_observations (
    event_id      TEXT PRIMARY KEY REFERENCES events(id),
    work_item_id  TEXT NOT NULL REFERENCES work_items(id),
    summary       TEXT NOT NULL,
    data          TEXT,
    produced_by   TEXT,
    timestamp     TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_work_item_observations_wi ON work_item_observations(work_item_id);

CREATE TABLE IF NOT EXISTS work_item_findings (
    event_id      TEXT PRIMARY KEY REFERENCES events(id),
    work_item_id  TEXT NOT NULL REFERENCES work_items(id),
    statement     TEXT NOT NULL,
    confidence    REAL NOT NULL DEFAULT 0,
    source        TEXT NOT NULL DEFAULT '',
    evidence_refs TEXT,
    produced_by   TEXT,
    retracted     INTEGER NOT NULL DEFAULT 0,
    timestamp     TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_work_item_findings_wi ON work_item_findings(work_item_id);

CREATE TABLE IF NOT EXISTS work_item_attempts (
    attempt_id      TEXT PRIMARY KEY,
    work_item_id    TEXT NOT NULL REFERENCES work_items(id),
    number          INTEGER NOT NULL,
    actor_id        TEXT NOT NULL,
    started_at      TEXT NOT NULL,
    completed_at    TEXT,
    status          TEXT NOT NULL DEFAULT 'running',
    last_checkpoint TEXT
);
CREATE INDEX IF NOT EXISTS idx_work_item_attempts_wi ON work_item_attempts(work_item_id);
