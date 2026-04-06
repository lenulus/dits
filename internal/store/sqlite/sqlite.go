package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/lenulus/pf/internal/domain"
	"github.com/lenulus/pf/internal/store"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

type Store struct {
	db *sql.DB
}

var _ store.DB = (*Store)(nil)

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("running migrations: %w", err)
	}
	return s, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate() error {
	migrations := []string{
		"migrations/001_initial.sql",
		"migrations/002_sync_state.sql",
		"migrations/003_attachments.sql",
		"migrations/004_actors.sql",
		"migrations/005_overlay.sql",
		"migrations/006_relations.sql",
		"migrations/007_coordination.sql",
		"migrations/008_evals.sql",
		"migrations/009_outcomes.sql",
	}
	for _, m := range migrations {
		data, err := migrationsFS.ReadFile(m)
		if err != nil {
			return fmt.Errorf("reading %s: %w", m, err)
		}
		if _, err := s.db.Exec(string(data)); err != nil {
			return fmt.Errorf("executing %s: %w", m, err)
		}
	}

	// Idempotent column additions for schema evolution.
	s.addColumnIfNotExists("work_items", "retained_outcome_ref", "TEXT")

	return nil
}

func (s *Store) addColumnIfNotExists(table, column, colType string) {
	rows, err := s.db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, typ string
		var notnull int
		var dflt sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			return
		}
		if name == column {
			return // already exists
		}
	}
	s.db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, colType))
}

// --- EventStore ---

func (s *Store) AppendEvents(ctx context.Context, events []domain.Event) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, e := range events {
		payload := string(e.Payload)

		var emittedBy *string
		if e.EmittedBy != nil {
			data, _ := json.Marshal(e.EmittedBy)
			s := string(data)
			emittedBy = &s
		}

		_, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO events (id, work_item_id, type, payload, meta_version, actor_id, timestamp, emitted_by, signature)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			e.ID, e.WorkItemID, e.Type, payload, e.MetaVersion, e.ActorID,
			e.Timestamp.UTC().Format(time.RFC3339Nano), emittedBy, e.Signature,
		)
		if err != nil {
			return fmt.Errorf("inserting event %s: %w", e.ID, err)
		}

		for _, pid := range e.ParentEventIDs {
			_, err := tx.ExecContext(ctx,
				`INSERT OR IGNORE INTO event_parents (event_id, parent_id) VALUES (?, ?)`,
				e.ID, pid,
			)
			if err != nil {
				return fmt.Errorf("inserting parent edge %s->%s: %w", e.ID, pid, err)
			}
		}

		// Update DAG heads: remove parents from heads, add this event.
		if len(e.ParentEventIDs) > 0 {
			placeholders := make([]string, len(e.ParentEventIDs))
			args := []any{string(e.WorkItemID)}
			for i, pid := range e.ParentEventIDs {
				placeholders[i] = "?"
				args = append(args, string(pid))
			}
			_, err = tx.ExecContext(ctx,
				fmt.Sprintf(`DELETE FROM dag_heads WHERE work_item_id = ? AND event_id IN (%s)`,
					strings.Join(placeholders, ",")),
				args...,
			)
			if err != nil {
				return fmt.Errorf("removing parent heads: %w", err)
			}
		}

		_, err = tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO dag_heads (work_item_id, event_id) VALUES (?, ?)`,
			e.WorkItemID, e.ID,
		)
		if err != nil {
			return fmt.Errorf("inserting head: %w", err)
		}
	}

	return tx.Commit()
}

func (s *Store) GetEvent(ctx context.Context, id domain.EventID) (*domain.Event, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, work_item_id, type, payload, meta_version, actor_id, timestamp, emitted_by, signature FROM events WHERE id = ?`, id)

	e, err := scanEvent(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	parents, err := s.getParents(ctx, e.ID)
	if err != nil {
		return nil, err
	}
	e.ParentEventIDs = parents
	return e, nil
}

func (s *Store) GetEventsForWorkItem(ctx context.Context, workItemID domain.WorkItemID) ([]domain.Event, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, work_item_id, type, payload, meta_version, actor_id, timestamp, emitted_by, signature
		 FROM events WHERE work_item_id = ? ORDER BY timestamp`, workItemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []domain.Event
	for rows.Next() {
		e, err := scanEventRows(rows)
		if err != nil {
			return nil, err
		}
		parents, err := s.getParents(ctx, e.ID)
		if err != nil {
			return nil, err
		}
		e.ParentEventIDs = parents
		events = append(events, *e)
	}
	return events, rows.Err()
}

func (s *Store) GetHeads(ctx context.Context, workItemID domain.WorkItemID) ([]domain.EventID, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT event_id FROM dag_heads WHERE work_item_id = ?`, workItemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var heads []domain.EventID
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		heads = append(heads, domain.EventID(id))
	}
	return heads, rows.Err()
}

func (s *Store) GetAllHeads(ctx context.Context) ([]domain.EventID, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT event_id FROM dag_heads`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var heads []domain.EventID
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		heads = append(heads, domain.EventID(id))
	}
	return heads, rows.Err()
}

func (s *Store) getParents(ctx context.Context, eventID domain.EventID) ([]domain.EventID, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT parent_id FROM event_parents WHERE event_id = ?`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var parents []domain.EventID
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		parents = append(parents, domain.EventID(id))
	}
	return parents, rows.Err()
}

// --- WorkItemStore ---

func (s *Store) UpsertWorkItem(ctx context.Context, wi *domain.WorkItem) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var closedAt *string
	if wi.ClosedAt != nil {
		v := wi.ClosedAt.UTC().Format(time.RFC3339Nano)
		closedAt = &v
	}

	var sharedID *string
	if wi.SharedID != "" {
		v := string(wi.SharedID)
		sharedID = &v
	}

	var leaseHolder *string
	if wi.LeaseHolder != nil {
		v := string(*wi.LeaseHolder)
		leaseHolder = &v
	}

	var leaseExpiresAt *string
	if wi.LeaseExpiresAt != nil {
		v := wi.LeaseExpiresAt.UTC().Format(time.RFC3339Nano)
		leaseExpiresAt = &v
	}

	var currentAttempt *string
	if wi.CurrentAttempt != nil {
		v := string(*wi.CurrentAttempt)
		currentAttempt = &v
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO work_items (id, shared_id, kind, title, body, status, priority,
		  lease_holder, lease_expires_at, current_attempt, blocked, blocked_reason,
		  retained_outcome_ref,
		  created_by, created_at, updated_at, closed_at, event_count)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   shared_id = excluded.shared_id,
		   kind = excluded.kind,
		   title = excluded.title,
		   body = excluded.body,
		   status = excluded.status,
		   priority = excluded.priority,
		   lease_holder = excluded.lease_holder,
		   lease_expires_at = excluded.lease_expires_at,
		   current_attempt = excluded.current_attempt,
		   blocked = excluded.blocked,
		   blocked_reason = excluded.blocked_reason,
		   retained_outcome_ref = excluded.retained_outcome_ref,
		   updated_at = excluded.updated_at,
		   closed_at = excluded.closed_at,
		   event_count = excluded.event_count`,
		wi.ID, sharedID, wi.Kind, wi.Title, wi.Body, wi.Status, wi.Priority,
		leaseHolder, leaseExpiresAt, currentAttempt,
		boolToInt(wi.Blocked), wi.BlockedReason, wi.RetainedOutcomeRef,
		wi.CreatedBy, wi.CreatedAt.UTC().Format(time.RFC3339Nano),
		wi.UpdatedAt.UTC().Format(time.RFC3339Nano), closedAt, wi.EventCount,
	)
	if err != nil {
		return fmt.Errorf("upserting work item: %w", err)
	}

	// Replace labels.
	_, _ = tx.ExecContext(ctx, `DELETE FROM work_item_labels WHERE work_item_id = ?`, wi.ID)
	for _, l := range wi.Labels {
		if _, err := tx.ExecContext(ctx, `INSERT INTO work_item_labels (work_item_id, label_slug) VALUES (?, ?)`, wi.ID, l); err != nil {
			return err
		}
	}

	// Replace assignees.
	_, _ = tx.ExecContext(ctx, `DELETE FROM work_item_assignees WHERE work_item_id = ?`, wi.ID)
	for _, a := range wi.Assignees {
		if _, err := tx.ExecContext(ctx, `INSERT INTO work_item_assignees (work_item_id, actor_id) VALUES (?, ?)`, wi.ID, a); err != nil {
			return err
		}
	}

	// Replace comments.
	_, _ = tx.ExecContext(ctx, `DELETE FROM work_item_comments WHERE work_item_id = ?`, wi.ID)
	for _, c := range wi.Comments {
		var producedBy *string
		if c.ProducedBy != nil {
			data, _ := json.Marshal(c.ProducedBy)
			s := string(data)
			producedBy = &s
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO work_item_comments (event_id, work_item_id, actor_id, body, produced_by, timestamp) VALUES (?, ?, ?, ?, ?, ?)`,
			c.EventID, wi.ID, c.ActorID, c.Body, producedBy, c.Timestamp.UTC().Format(time.RFC3339Nano),
		); err != nil {
			return err
		}
	}

	// Replace artifacts.
	_, _ = tx.ExecContext(ctx, `DELETE FROM work_item_artifacts WHERE work_item_id = ?`, wi.ID)
	for _, a := range wi.Artifacts {
		var producedBy *string
		if a.ProducedBy != nil {
			data, _ := json.Marshal(a.ProducedBy)
			s := string(data)
			producedBy = &s
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO work_item_artifacts (artifact_id, work_item_id, content_hash, filename, mime_type, size_bytes, artifact_type, semantic_role, produced_by, added_by, added_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			a.ID, wi.ID, a.ContentHash, a.Filename, a.MimeType, a.SizeBytes,
			a.ArtifactType, a.SemanticRole, producedBy,
			a.AddedBy, a.AddedAt.UTC().Format(time.RFC3339Nano),
		); err != nil {
			return err
		}
	}

	// Replace relations.
	_, _ = tx.ExecContext(ctx, `DELETE FROM work_item_relations WHERE work_item_id = ?`, wi.ID)
	for _, r := range wi.Relations {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO work_item_relations (work_item_id, relation_type, target_work_item) VALUES (?, ?, ?)`,
			wi.ID, r.Type, r.TargetWorkItem); err != nil {
			return err
		}
	}

	// Replace checkpoints.
	_, _ = tx.ExecContext(ctx, `DELETE FROM work_item_checkpoints WHERE work_item_id = ?`, wi.ID)
	for _, cp := range wi.Checkpoints {
		var data *string
		if cp.Data != nil {
			s := string(cp.Data)
			data = &s
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO work_item_checkpoints (event_id, work_item_id, attempt_id, summary, progress, next_step, data, timestamp)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			cp.EventID, wi.ID, cp.AttemptID, cp.Summary, cp.Progress, cp.NextStep, data,
			cp.Timestamp.UTC().Format(time.RFC3339Nano),
		); err != nil {
			return err
		}
	}

	// Replace observations.
	_, _ = tx.ExecContext(ctx, `DELETE FROM work_item_observations WHERE work_item_id = ?`, wi.ID)
	for _, obs := range wi.Observations {
		var data, producedBy *string
		if obs.Data != nil {
			s := string(obs.Data)
			data = &s
		}
		if obs.ProducedBy != nil {
			d, _ := json.Marshal(obs.ProducedBy)
			s := string(d)
			producedBy = &s
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO work_item_observations (event_id, work_item_id, summary, data, produced_by, timestamp)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			obs.EventID, wi.ID, obs.Summary, data, producedBy,
			obs.Timestamp.UTC().Format(time.RFC3339Nano),
		); err != nil {
			return err
		}
	}

	// Replace findings.
	_, _ = tx.ExecContext(ctx, `DELETE FROM work_item_findings WHERE work_item_id = ?`, wi.ID)
	for _, f := range wi.Findings {
		var evidenceRefs, producedBy *string
		if len(f.EvidenceRefs) > 0 {
			d, _ := json.Marshal(f.EvidenceRefs)
			s := string(d)
			evidenceRefs = &s
		}
		if f.ProducedBy != nil {
			d, _ := json.Marshal(f.ProducedBy)
			s := string(d)
			producedBy = &s
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO work_item_findings (event_id, work_item_id, statement, confidence, source, evidence_refs, produced_by, retracted, timestamp)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			f.EventID, wi.ID, f.Statement, f.Confidence, f.Source, evidenceRefs, producedBy,
			boolToInt(f.Retracted), f.Timestamp.UTC().Format(time.RFC3339Nano),
		); err != nil {
			return err
		}
	}

	// Replace attempts.
	_, _ = tx.ExecContext(ctx, `DELETE FROM work_item_attempts WHERE work_item_id = ?`, wi.ID)
	for _, a := range wi.Attempts {
		var completedAt, lastCheckpoint *string
		if a.CompletedAt != nil {
			v := a.CompletedAt.UTC().Format(time.RFC3339Nano)
			completedAt = &v
		}
		if a.LastCheckpoint != nil {
			v := string(*a.LastCheckpoint)
			lastCheckpoint = &v
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO work_item_attempts (attempt_id, work_item_id, number, actor_id, started_at, completed_at, status, last_checkpoint, authoritative)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			a.AttemptID, wi.ID, a.Number, a.ActorID,
			a.StartedAt.UTC().Format(time.RFC3339Nano), completedAt, a.Status, lastCheckpoint,
			boolToInt(a.Authoritative),
		); err != nil {
			return err
		}
	}

	// Replace evals.
	_, _ = tx.ExecContext(ctx, `DELETE FROM work_item_evals WHERE work_item_id = ?`, wi.ID)
	for _, ev := range wi.Evals {
		var metrics, producedBy *string
		if ev.Metrics != nil {
			s := string(ev.Metrics)
			metrics = &s
		}
		if ev.ProducedBy != nil {
			d, _ := json.Marshal(ev.ProducedBy)
			s := string(d)
			producedBy = &s
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO work_item_evals (eval_id, event_id, work_item_id, subject_kind, subject_ref, rubric_ref, summary, metrics, verdict, produced_by, timestamp)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			ev.EvalID, ev.EventID, wi.ID, ev.SubjectKind, ev.SubjectRef, ev.RubricRef, ev.Summary,
			metrics, ev.Verdict, producedBy, ev.Timestamp.UTC().Format(time.RFC3339Nano),
		); err != nil {
			return err
		}
	}

	// Replace outcomes.
	_, _ = tx.ExecContext(ctx, `DELETE FROM work_item_outcomes WHERE work_item_id = ?`, wi.ID)
	for _, oc := range wi.Outcomes {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO work_item_outcomes (event_id, work_item_id, subject_kind, subject_ref, decision, reason, eval_ref, actor_id, timestamp)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			oc.EventID, wi.ID, oc.SubjectKind, oc.SubjectRef, oc.Decision, oc.Reason, oc.EvalRef,
			oc.ActorID, oc.Timestamp.UTC().Format(time.RFC3339Nano),
		); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *Store) GetWorkItem(ctx context.Context, id domain.WorkItemID) (*domain.WorkItem, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, shared_id, kind, title, body, status, priority,
		  lease_holder, lease_expires_at, current_attempt, blocked, blocked_reason,
		  retained_outcome_ref, created_by, created_at, updated_at, closed_at, event_count
		 FROM work_items WHERE id = ?`, id)
	return s.scanWorkItem(ctx, row)
}

func (s *Store) GetWorkItemBySharedID(ctx context.Context, id domain.SharedID) (*domain.WorkItem, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, shared_id, kind, title, body, status, priority,
		  lease_holder, lease_expires_at, current_attempt, blocked, blocked_reason,
		  retained_outcome_ref, created_by, created_at, updated_at, closed_at, event_count
		 FROM work_items WHERE shared_id = ?`, id)
	return s.scanWorkItem(ctx, row)
}

func (s *Store) ListWorkItems(ctx context.Context, filter store.WorkItemFilter) ([]domain.WorkItem, error) {
	query := `SELECT w.id, w.shared_id, w.kind, w.title, w.body, w.status, w.priority,
	  w.lease_holder, w.lease_expires_at, w.current_attempt, w.blocked, w.blocked_reason,
	  w.retained_outcome_ref, w.created_by, w.created_at, w.updated_at, w.closed_at, w.event_count
	  FROM work_items w`
	var conditions []string
	var args []any

	if filter.Status != "" {
		conditions = append(conditions, "w.status = ?")
		args = append(args, filter.Status)
	}
	if len(filter.Statuses) > 0 {
		placeholders := make([]string, len(filter.Statuses))
		for i, s := range filter.Statuses {
			placeholders[i] = "?"
			args = append(args, s)
		}
		conditions = append(conditions, fmt.Sprintf("w.status IN (%s)", strings.Join(placeholders, ",")))
	}
	if filter.Kind != "" {
		conditions = append(conditions, "w.kind = ?")
		args = append(args, filter.Kind)
	}
	if filter.Label != "" {
		query += " JOIN work_item_labels wl ON w.id = wl.work_item_id"
		conditions = append(conditions, "wl.label_slug = ?")
		args = append(args, filter.Label)
	}
	if filter.Assignee != "" {
		query += " JOIN work_item_assignees wa ON w.id = wa.work_item_id"
		conditions = append(conditions, "wa.actor_id = ?")
		args = append(args, filter.Assignee)
	}
	if filter.ClaimedBy != "" {
		conditions = append(conditions, "w.lease_holder = ?")
		args = append(args, string(filter.ClaimedBy))
	}
	if filter.Ready != nil && *filter.Ready {
		conditions = append(conditions, "(w.lease_holder IS NULL OR w.lease_expires_at < ?)")
		args = append(args, time.Now().UTC().Format(time.RFC3339Nano))
	}
	if filter.Blocked != nil {
		conditions = append(conditions, "w.blocked = ?")
		args = append(args, boolToInt(*filter.Blocked))
	}
	if filter.Query != "" {
		conditions = append(conditions, "(w.title LIKE ? OR w.body LIKE ?)")
		q := "%" + filter.Query + "%"
		args = append(args, q, q)
	}

	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY w.updated_at DESC"

	if filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", filter.Limit)
	}
	if filter.Offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", filter.Offset)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []domain.WorkItem
	for rows.Next() {
		wi, err := s.scanWorkItemRows(ctx, rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *wi)
	}
	return items, rows.Err()
}

func (s *Store) AllocateSharedID(ctx context.Context, workItemID domain.WorkItemID, projectKey string) (domain.SharedID, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx,
		`INSERT OR IGNORE INTO shared_id_counter (project_key, next_id) VALUES (?, 1)`, projectKey)
	if err != nil {
		return "", err
	}

	var nextID int
	err = tx.QueryRowContext(ctx,
		`SELECT next_id FROM shared_id_counter WHERE project_key = ?`, projectKey).Scan(&nextID)
	if err != nil {
		return "", err
	}

	_, err = tx.ExecContext(ctx,
		`UPDATE shared_id_counter SET next_id = next_id + 1 WHERE project_key = ?`, projectKey)
	if err != nil {
		return "", err
	}

	sharedID := domain.SharedID(fmt.Sprintf("%s-%d", projectKey, nextID))

	_, err = tx.ExecContext(ctx, `UPDATE work_items SET shared_id = ? WHERE id = ?`, sharedID, workItemID)
	if err != nil {
		return "", err
	}

	if err := tx.Commit(); err != nil {
		return "", err
	}
	return sharedID, nil
}

// --- MetaStore ---

func (s *Store) GetCurrentMeta(ctx context.Context) (*domain.MetaConfig, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT config_json FROM meta_config ORDER BY version DESC LIMIT 1`)

	var configJSON string
	if err := row.Scan(&configJSON); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	var meta domain.MetaConfig
	if err := json.Unmarshal([]byte(configJSON), &meta); err != nil {
		return nil, err
	}
	return &meta, nil
}

func (s *Store) SaveMeta(ctx context.Context, meta *domain.MetaConfig) error {
	data, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO meta_config (version, config_json) VALUES (?, ?)`,
		meta.Version, string(data))
	return err
}

func (s *Store) SaveMetaIfVersion(ctx context.Context, meta *domain.MetaConfig, expectedVersion domain.MetaVersion) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var currentVersion int64
	err = tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM meta_config`).Scan(&currentVersion)
	if err != nil {
		return err
	}
	if domain.MetaVersion(currentVersion) != expectedVersion {
		return store.ErrMetaConflict
	}

	data, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx,
		`INSERT OR REPLACE INTO meta_config (version, config_json) VALUES (?, ?)`,
		meta.Version, string(data))
	if err != nil {
		return err
	}
	return tx.Commit()
}

// --- Helpers ---

type scanner interface {
	Scan(dest ...any) error
}

func scanEvent(row scanner) (*domain.Event, error) {
	var e domain.Event
	var id, workItemID, eventType, payload, actorID, ts string
	var metaVersion int64
	var emittedBy sql.NullString
	var sig []byte

	err := row.Scan(&id, &workItemID, &eventType, &payload, &metaVersion, &actorID, &ts, &emittedBy, &sig)
	if err != nil {
		return nil, err
	}

	t, _ := time.Parse(time.RFC3339Nano, ts)
	e.ID = domain.EventID(id)
	e.WorkItemID = domain.WorkItemID(workItemID)
	e.Type = domain.EventType(eventType)
	e.Payload = json.RawMessage(payload)
	e.MetaVersion = domain.MetaVersion(metaVersion)
	e.ActorID = domain.ActorID(actorID)
	e.Timestamp = t
	e.Signature = sig

	if emittedBy.Valid {
		var eb domain.EmittedBy
		if err := json.Unmarshal([]byte(emittedBy.String), &eb); err == nil {
			e.EmittedBy = &eb
		}
	}

	return &e, nil
}

func scanEventRows(rows *sql.Rows) (*domain.Event, error) {
	return scanEvent(rows)
}

func (s *Store) scanWorkItem(ctx context.Context, row *sql.Row) (*domain.WorkItem, error) {
	wi := &domain.WorkItem{}
	var sharedID, closedAt, leaseHolder, leaseExpiresAt, currentAttempt, retainedOutcomeRef sql.NullString
	var createdAt, updatedAt string
	var blocked int

	err := row.Scan(&wi.ID, &sharedID, &wi.Kind, &wi.Title, &wi.Body, &wi.Status, &wi.Priority,
		&leaseHolder, &leaseExpiresAt, &currentAttempt, &blocked, &wi.BlockedReason,
		&retainedOutcomeRef, &wi.CreatedBy, &createdAt, &updatedAt, &closedAt, &wi.EventCount)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	if sharedID.Valid {
		wi.SharedID = domain.SharedID(sharedID.String)
	}
	wi.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	wi.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	if closedAt.Valid {
		t, _ := time.Parse(time.RFC3339Nano, closedAt.String)
		wi.ClosedAt = &t
	}
	if leaseHolder.Valid {
		a := domain.ActorID(leaseHolder.String)
		wi.LeaseHolder = &a
	}
	if leaseExpiresAt.Valid {
		t, _ := time.Parse(time.RFC3339Nano, leaseExpiresAt.String)
		wi.LeaseExpiresAt = &t
	}
	if currentAttempt.Valid {
		a := domain.AttemptID(currentAttempt.String)
		wi.CurrentAttempt = &a
	}
	wi.Blocked = blocked != 0
	if retainedOutcomeRef.Valid {
		s := retainedOutcomeRef.String
		wi.RetainedOutcomeRef = &s
	}

	if err := s.loadWorkItemCollections(ctx, wi); err != nil {
		return nil, err
	}
	clearExpiredLease(wi)
	return wi, nil
}

func (s *Store) scanWorkItemRows(ctx context.Context, rows *sql.Rows) (*domain.WorkItem, error) {
	wi := &domain.WorkItem{}
	var sharedID, closedAt, leaseHolder, leaseExpiresAt, currentAttempt, retainedOutcomeRef sql.NullString
	var createdAt, updatedAt string
	var blocked int

	err := rows.Scan(&wi.ID, &sharedID, &wi.Kind, &wi.Title, &wi.Body, &wi.Status, &wi.Priority,
		&leaseHolder, &leaseExpiresAt, &currentAttempt, &blocked, &wi.BlockedReason,
		&retainedOutcomeRef, &wi.CreatedBy, &createdAt, &updatedAt, &closedAt, &wi.EventCount)
	if err != nil {
		return nil, err
	}

	if sharedID.Valid {
		wi.SharedID = domain.SharedID(sharedID.String)
	}
	wi.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	wi.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	if closedAt.Valid {
		t, _ := time.Parse(time.RFC3339Nano, closedAt.String)
		wi.ClosedAt = &t
	}
	if leaseHolder.Valid {
		a := domain.ActorID(leaseHolder.String)
		wi.LeaseHolder = &a
	}
	if leaseExpiresAt.Valid {
		t, _ := time.Parse(time.RFC3339Nano, leaseExpiresAt.String)
		wi.LeaseExpiresAt = &t
	}
	if currentAttempt.Valid {
		a := domain.AttemptID(currentAttempt.String)
		wi.CurrentAttempt = &a
	}
	wi.Blocked = blocked != 0
	if retainedOutcomeRef.Valid {
		s := retainedOutcomeRef.String
		wi.RetainedOutcomeRef = &s
	}

	if err := s.loadWorkItemCollections(ctx, wi); err != nil {
		return nil, err
	}
	clearExpiredLease(wi)
	return wi, nil
}

// clearExpiredLease clears lease fields if the lease has expired (wall-clock).
// This is a query-time concern — the materialized state in the DB is not modified.
func clearExpiredLease(wi *domain.WorkItem) {
	if wi.LeaseExpiresAt != nil && wi.LeaseExpiresAt.Before(time.Now().UTC()) {
		wi.LeaseHolder = nil
		wi.LeaseExpiresAt = nil
	}
}

func (s *Store) loadWorkItemCollections(ctx context.Context, wi *domain.WorkItem) error {
	// Labels
	rows, err := s.db.QueryContext(ctx, `SELECT label_slug FROM work_item_labels WHERE work_item_id = ?`, wi.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	wi.Labels = []string{}
	for rows.Next() {
		var l string
		if err := rows.Scan(&l); err != nil {
			return err
		}
		wi.Labels = append(wi.Labels, l)
	}

	// Assignees
	rows2, err := s.db.QueryContext(ctx, `SELECT actor_id FROM work_item_assignees WHERE work_item_id = ?`, wi.ID)
	if err != nil {
		return err
	}
	defer rows2.Close()
	wi.Assignees = []domain.ActorID{}
	for rows2.Next() {
		var a string
		if err := rows2.Scan(&a); err != nil {
			return err
		}
		wi.Assignees = append(wi.Assignees, domain.ActorID(a))
	}

	// Comments
	rows3, err := s.db.QueryContext(ctx,
		`SELECT event_id, actor_id, body, produced_by, timestamp FROM work_item_comments WHERE work_item_id = ? ORDER BY timestamp`, wi.ID)
	if err != nil {
		return err
	}
	defer rows3.Close()
	wi.Comments = []domain.Comment{}
	for rows3.Next() {
		var c domain.Comment
		var ts string
		var producedBy sql.NullString
		if err := rows3.Scan(&c.EventID, &c.ActorID, &c.Body, &producedBy, &ts); err != nil {
			return err
		}
		c.Timestamp, _ = time.Parse(time.RFC3339Nano, ts)
		if producedBy.Valid {
			var pb domain.ProducedBy
			if err := json.Unmarshal([]byte(producedBy.String), &pb); err == nil {
				c.ProducedBy = &pb
			}
		}
		wi.Comments = append(wi.Comments, c)
	}

	// Artifacts
	rows4, err := s.db.QueryContext(ctx,
		`SELECT artifact_id, content_hash, filename, mime_type, size_bytes, artifact_type, semantic_role, produced_by, added_by, added_at
		 FROM work_item_artifacts WHERE work_item_id = ? ORDER BY added_at`, wi.ID)
	if err != nil {
		return err
	}
	defer rows4.Close()
	wi.Artifacts = []domain.Artifact{}
	for rows4.Next() {
		var a domain.Artifact
		var ts string
		var producedBy sql.NullString
		if err := rows4.Scan(&a.ID, &a.ContentHash, &a.Filename, &a.MimeType, &a.SizeBytes,
			&a.ArtifactType, &a.SemanticRole, &producedBy, &a.AddedBy, &ts); err != nil {
			return err
		}
		a.AddedAt, _ = time.Parse(time.RFC3339Nano, ts)
		if producedBy.Valid {
			var pb domain.ProducedBy
			if err := json.Unmarshal([]byte(producedBy.String), &pb); err == nil {
				a.ProducedBy = &pb
			}
		}
		wi.Artifacts = append(wi.Artifacts, a)
	}

	// Relations
	rows5, err := s.db.QueryContext(ctx,
		`SELECT relation_type, target_work_item FROM work_item_relations WHERE work_item_id = ? ORDER BY relation_type, target_work_item`, wi.ID)
	if err != nil {
		return err
	}
	defer rows5.Close()
	wi.Relations = []domain.Relation{}
	for rows5.Next() {
		var r domain.Relation
		if err := rows5.Scan(&r.Type, &r.TargetWorkItem); err != nil {
			return err
		}
		wi.Relations = append(wi.Relations, r)
	}

	// Checkpoints
	rows6, err := s.db.QueryContext(ctx,
		`SELECT event_id, attempt_id, summary, progress, next_step, data, timestamp
		 FROM work_item_checkpoints WHERE work_item_id = ? ORDER BY timestamp`, wi.ID)
	if err != nil {
		return err
	}
	defer rows6.Close()
	wi.Checkpoints = []domain.Checkpoint{}
	for rows6.Next() {
		var cp domain.Checkpoint
		var ts string
		var data sql.NullString
		if err := rows6.Scan(&cp.EventID, &cp.AttemptID, &cp.Summary, &cp.Progress, &cp.NextStep, &data, &ts); err != nil {
			return err
		}
		cp.Timestamp, _ = time.Parse(time.RFC3339Nano, ts)
		if data.Valid {
			cp.Data = json.RawMessage(data.String)
		}
		wi.Checkpoints = append(wi.Checkpoints, cp)
	}

	// Observations
	rows7, err := s.db.QueryContext(ctx,
		`SELECT event_id, summary, data, produced_by, timestamp
		 FROM work_item_observations WHERE work_item_id = ? ORDER BY timestamp`, wi.ID)
	if err != nil {
		return err
	}
	defer rows7.Close()
	wi.Observations = []domain.Observation{}
	for rows7.Next() {
		var obs domain.Observation
		var ts string
		var data, producedBy sql.NullString
		if err := rows7.Scan(&obs.EventID, &obs.Summary, &data, &producedBy, &ts); err != nil {
			return err
		}
		obs.Timestamp, _ = time.Parse(time.RFC3339Nano, ts)
		if data.Valid {
			obs.Data = json.RawMessage(data.String)
		}
		if producedBy.Valid {
			var pb domain.ProducedBy
			if err := json.Unmarshal([]byte(producedBy.String), &pb); err == nil {
				obs.ProducedBy = &pb
			}
		}
		wi.Observations = append(wi.Observations, obs)
	}

	// Findings
	rows8, err := s.db.QueryContext(ctx,
		`SELECT event_id, statement, confidence, source, evidence_refs, produced_by, retracted, timestamp
		 FROM work_item_findings WHERE work_item_id = ? ORDER BY timestamp`, wi.ID)
	if err != nil {
		return err
	}
	defer rows8.Close()
	wi.Findings = []domain.Finding{}
	for rows8.Next() {
		var f domain.Finding
		var ts string
		var evidenceRefs, producedBy sql.NullString
		var retracted int
		if err := rows8.Scan(&f.EventID, &f.Statement, &f.Confidence, &f.Source, &evidenceRefs, &producedBy, &retracted, &ts); err != nil {
			return err
		}
		f.Timestamp, _ = time.Parse(time.RFC3339Nano, ts)
		f.Retracted = retracted != 0
		if evidenceRefs.Valid {
			json.Unmarshal([]byte(evidenceRefs.String), &f.EvidenceRefs)
		}
		if producedBy.Valid {
			var pb domain.ProducedBy
			if err := json.Unmarshal([]byte(producedBy.String), &pb); err == nil {
				f.ProducedBy = &pb
			}
		}
		wi.Findings = append(wi.Findings, f)
	}

	// Attempts
	rows9, err := s.db.QueryContext(ctx,
		`SELECT attempt_id, number, actor_id, started_at, completed_at, status, last_checkpoint, authoritative
		 FROM work_item_attempts WHERE work_item_id = ? ORDER BY number`, wi.ID)
	if err != nil {
		return err
	}
	defer rows9.Close()
	wi.Attempts = []domain.ExecutionAttempt{}
	for rows9.Next() {
		var a domain.ExecutionAttempt
		var startedAt string
		var completedAt, lastCheckpoint sql.NullString
		var authoritative int
		if err := rows9.Scan(&a.AttemptID, &a.Number, &a.ActorID, &startedAt, &completedAt, &a.Status, &lastCheckpoint, &authoritative); err != nil {
			return err
		}
		a.StartedAt, _ = time.Parse(time.RFC3339Nano, startedAt)
		a.Authoritative = authoritative != 0
		if completedAt.Valid {
			t, _ := time.Parse(time.RFC3339Nano, completedAt.String)
			a.CompletedAt = &t
		}
		if lastCheckpoint.Valid {
			eid := domain.EventID(lastCheckpoint.String)
			a.LastCheckpoint = &eid
		}
		wi.Attempts = append(wi.Attempts, a)
	}

	// Evals
	rows10, err := s.db.QueryContext(ctx,
		`SELECT eval_id, event_id, subject_kind, subject_ref, rubric_ref, summary, metrics, verdict, produced_by, timestamp
		 FROM work_item_evals WHERE work_item_id = ? ORDER BY timestamp`, wi.ID)
	if err != nil {
		return err
	}
	defer rows10.Close()
	wi.Evals = []domain.Eval{}
	for rows10.Next() {
		var ev domain.Eval
		var ts string
		var metrics, producedBy sql.NullString
		if err := rows10.Scan(&ev.EvalID, &ev.EventID, &ev.SubjectKind, &ev.SubjectRef, &ev.RubricRef, &ev.Summary, &metrics, &ev.Verdict, &producedBy, &ts); err != nil {
			return err
		}
		ev.Timestamp, _ = time.Parse(time.RFC3339Nano, ts)
		if metrics.Valid {
			ev.Metrics = json.RawMessage(metrics.String)
		}
		if producedBy.Valid {
			var pb domain.ProducedBy
			if err := json.Unmarshal([]byte(producedBy.String), &pb); err == nil {
				ev.ProducedBy = &pb
			}
		}
		wi.Evals = append(wi.Evals, ev)
	}

	// Outcomes
	rows11, err := s.db.QueryContext(ctx,
		`SELECT event_id, subject_kind, subject_ref, decision, reason, eval_ref, actor_id, timestamp
		 FROM work_item_outcomes WHERE work_item_id = ? ORDER BY timestamp`, wi.ID)
	if err != nil {
		return err
	}
	defer rows11.Close()
	wi.Outcomes = []domain.Outcome{}
	for rows11.Next() {
		var oc domain.Outcome
		var ts string
		if err := rows11.Scan(&oc.EventID, &oc.SubjectKind, &oc.SubjectRef, &oc.Decision, &oc.Reason, &oc.EvalRef, &oc.ActorID, &ts); err != nil {
			return err
		}
		oc.Timestamp, _ = time.Parse(time.RFC3339Nano, ts)
		wi.Outcomes = append(wi.Outcomes, oc)
	}

	return nil
}

func marshalEventIDs(ids []domain.EventID) string {
	if len(ids) == 0 {
		return "[]"
	}
	data, _ := json.Marshal(ids)
	return string(data)
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// --- EventStore extensions ---

func (s *Store) HasEvent(ctx context.Context, id domain.EventID) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE id = ?`, id).Scan(&count)
	return count > 0, err
}

func (s *Store) GetAffectedWorkItemIDs(ctx context.Context, events []domain.Event) ([]domain.WorkItemID, error) {
	seen := make(map[domain.WorkItemID]struct{})
	var ids []domain.WorkItemID
	for _, e := range events {
		if _, ok := seen[e.WorkItemID]; !ok {
			seen[e.WorkItemID] = struct{}{}
			ids = append(ids, e.WorkItemID)
		}
	}
	return ids, nil
}

// --- SyncStore ---

func (s *Store) GetRemoteHeads(ctx context.Context, nodeID domain.NodeID) ([]domain.EventID, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT event_id FROM sync_remote_heads WHERE node_id = ?`, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var heads []domain.EventID
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		heads = append(heads, domain.EventID(id))
	}
	return heads, rows.Err()
}

func (s *Store) SetRemoteHeads(ctx context.Context, nodeID domain.NodeID, heads []domain.EventID) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, `DELETE FROM sync_remote_heads WHERE node_id = ?`, nodeID)
	if err != nil {
		return err
	}

	for _, h := range heads {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO sync_remote_heads (node_id, event_id) VALUES (?, ?)`, nodeID, h)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *Store) GetSyncRemoteURL(ctx context.Context, nodeID domain.NodeID) (string, error) {
	var url string
	err := s.db.QueryRowContext(ctx, `SELECT url FROM sync_remotes WHERE node_id = ?`, nodeID).Scan(&url)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return url, err
}

func (s *Store) SetSyncRemote(ctx context.Context, nodeID domain.NodeID, url string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sync_remotes (node_id, url) VALUES (?, ?)
		 ON CONFLICT(node_id) DO UPDATE SET url = excluded.url`,
		nodeID, url)
	return err
}

// --- ActorStore ---

func (s *Store) RegisterActor(ctx context.Context, actorID domain.ActorID, publicKey string, nodeID domain.NodeID) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO actors (actor_id, public_key, node_id) VALUES (?, ?, ?)
		 ON CONFLICT(actor_id) DO UPDATE SET public_key = excluded.public_key, node_id = excluded.node_id`,
		actorID, publicKey, nodeID)
	return err
}

func (s *Store) GetActorPublicKey(ctx context.Context, actorID domain.ActorID) (string, error) {
	var pubKey string
	err := s.db.QueryRowContext(ctx, `SELECT public_key FROM actors WHERE actor_id = ?`, actorID).Scan(&pubKey)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return pubKey, err
}

// --- OverlayStore ---

func (s *Store) SetAnnotation(ctx context.Context, workItemID domain.WorkItemID, key, value string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO overlay_annotations (work_item_id, key, value) VALUES (?, ?, ?)
		 ON CONFLICT(work_item_id, key) DO UPDATE SET value = excluded.value`,
		workItemID, key, value)
	return err
}

func (s *Store) GetAnnotations(ctx context.Context, workItemID domain.WorkItemID) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT key, value FROM overlay_annotations WHERE work_item_id = ? ORDER BY key`, workItemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		result[k] = v
	}
	return result, rows.Err()
}

func (s *Store) DeleteAnnotation(ctx context.Context, workItemID domain.WorkItemID, key string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM overlay_annotations WHERE work_item_id = ? AND key = ?`, workItemID, key)
	return err
}

func (s *Store) AddPrivateLabel(ctx context.Context, workItemID domain.WorkItemID, label string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO overlay_labels (work_item_id, label) VALUES (?, ?)`,
		workItemID, label)
	return err
}

func (s *Store) RemovePrivateLabel(ctx context.Context, workItemID domain.WorkItemID, label string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM overlay_labels WHERE work_item_id = ? AND label = ?`, workItemID, label)
	return err
}

func (s *Store) GetPrivateLabels(ctx context.Context, workItemID domain.WorkItemID) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT label FROM overlay_labels WHERE work_item_id = ? ORDER BY label`, workItemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var labels []string
	for rows.Next() {
		var l string
		if err := rows.Scan(&l); err != nil {
			return nil, err
		}
		labels = append(labels, l)
	}
	return labels, rows.Err()
}

// --- Query API ---

func (s *Store) ListEvents(ctx context.Context, filter store.EventFilter) ([]domain.Event, error) {
	query := `SELECT id, work_item_id, type, payload, meta_version, actor_id, timestamp, emitted_by, signature FROM events`
	var conditions []string
	var args []any

	if filter.WorkItemID != "" {
		conditions = append(conditions, "work_item_id = ?")
		args = append(args, string(filter.WorkItemID))
	}
	if !filter.Since.IsZero() {
		conditions = append(conditions, "timestamp > ?")
		args = append(args, filter.Since.UTC().Format(time.RFC3339Nano))
	}
	if filter.Type != "" {
		conditions = append(conditions, "type = ?")
		args = append(args, string(filter.Type))
	}
	if filter.ActorID != "" {
		conditions = append(conditions, "actor_id = ?")
		args = append(args, string(filter.ActorID))
	}

	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY timestamp"

	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	query += fmt.Sprintf(" LIMIT %d", limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []domain.Event
	for rows.Next() {
		e, err := scanEventRows(rows)
		if err != nil {
			return nil, err
		}
		parents, err := s.getParents(ctx, e.ID)
		if err != nil {
			return nil, err
		}
		e.ParentEventIDs = parents
		events = append(events, *e)
	}
	return events, rows.Err()
}

func (s *Store) GetArtifactsByHash(ctx context.Context, contentHash string) ([]domain.Artifact, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT artifact_id, content_hash, filename, mime_type, size_bytes, artifact_type, semantic_role, produced_by, added_by, added_at
		 FROM work_item_artifacts WHERE content_hash = ? ORDER BY added_at`, contentHash)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var artifacts []domain.Artifact
	for rows.Next() {
		var a domain.Artifact
		var ts string
		var producedBy sql.NullString
		if err := rows.Scan(&a.ID, &a.ContentHash, &a.Filename, &a.MimeType, &a.SizeBytes,
			&a.ArtifactType, &a.SemanticRole, &producedBy, &a.AddedBy, &ts); err != nil {
			return nil, err
		}
		a.AddedAt, _ = time.Parse(time.RFC3339Nano, ts)
		if producedBy.Valid {
			var pb domain.ProducedBy
			if err := json.Unmarshal([]byte(producedBy.String), &pb); err == nil {
				a.ProducedBy = &pb
			}
		}
		artifacts = append(artifacts, a)
	}
	return artifacts, rows.Err()
}
