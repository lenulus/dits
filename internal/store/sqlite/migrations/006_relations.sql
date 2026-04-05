CREATE TABLE IF NOT EXISTS work_item_relations (
    work_item_id     TEXT NOT NULL REFERENCES work_items(id),
    relation_type    TEXT NOT NULL,
    target_work_item TEXT NOT NULL,
    PRIMARY KEY (work_item_id, relation_type, target_work_item)
);
CREATE INDEX IF NOT EXISTS idx_work_item_relations_target ON work_item_relations(target_work_item);
