CREATE TABLE IF NOT EXISTS issue_relations (
    issue_id      TEXT NOT NULL REFERENCES issues(id),
    relation_type TEXT NOT NULL,
    target_issue  TEXT NOT NULL,
    PRIMARY KEY (issue_id, relation_type, target_issue)
);
CREATE INDEX IF NOT EXISTS idx_issue_relations_target ON issue_relations(target_issue);
