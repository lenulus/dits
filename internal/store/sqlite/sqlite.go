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
	migrations := []string{"migrations/001_initial.sql", "migrations/002_sync_state.sql", "migrations/003_attachments.sql", "migrations/004_actors.sql"}
	for _, m := range migrations {
		data, err := migrationsFS.ReadFile(m)
		if err != nil {
			return fmt.Errorf("reading %s: %w", m, err)
		}
		if _, err := s.db.Exec(string(data)); err != nil {
			return fmt.Errorf("executing %s: %w", m, err)
		}
	}
	return nil
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
		parents := marshalEventIDs(e.ParentEventIDs)

		_, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO events (id, issue_id, type, payload, meta_version, actor_id, timestamp, signature)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			e.ID, e.IssueID, e.Type, payload, e.MetaVersion, e.ActorID,
			e.Timestamp.UTC().Format(time.RFC3339Nano), e.Signature,
		)
		if err != nil {
			return fmt.Errorf("inserting event %s: %w", e.ID, err)
		}

		// Insert parent edges.
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
			args := []any{string(e.IssueID)}
			for i, pid := range e.ParentEventIDs {
				placeholders[i] = "?"
				args = append(args, string(pid))
			}
			_, err = tx.ExecContext(ctx,
				fmt.Sprintf(`DELETE FROM dag_heads WHERE issue_id = ? AND event_id IN (%s)`,
					strings.Join(placeholders, ",")),
				args...,
			)
			if err != nil {
				return fmt.Errorf("removing parent heads: %w", err)
			}
		}

		_, err = tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO dag_heads (issue_id, event_id) VALUES (?, ?)`,
			e.IssueID, e.ID,
		)
		if err != nil {
			return fmt.Errorf("inserting head: %w", err)
		}

		_ = parents // used for reference only
	}

	return tx.Commit()
}

func (s *Store) GetEvent(ctx context.Context, id domain.EventID) (*domain.Event, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, issue_id, type, payload, meta_version, actor_id, timestamp, signature FROM events WHERE id = ?`, id)

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

func (s *Store) GetEventsForIssue(ctx context.Context, issueID domain.CanonicalID) ([]domain.Event, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, issue_id, type, payload, meta_version, actor_id, timestamp, signature
		 FROM events WHERE issue_id = ? ORDER BY timestamp`, issueID)
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

func (s *Store) GetHeads(ctx context.Context, issueID domain.CanonicalID) ([]domain.EventID, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT event_id FROM dag_heads WHERE issue_id = ?`, issueID)
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

// --- IssueStore ---

func (s *Store) UpsertIssue(ctx context.Context, issue *domain.Issue) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var closedAt *string
	if issue.ClosedAt != nil {
		v := issue.ClosedAt.UTC().Format(time.RFC3339Nano)
		closedAt = &v
	}

	var sharedID *string
	if issue.SharedID != "" {
		v := string(issue.SharedID)
		sharedID = &v
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO issues (id, shared_id, title, body, status, type_slug, priority, created_by, created_at, updated_at, closed_at, event_count)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   shared_id = excluded.shared_id,
		   title = excluded.title,
		   body = excluded.body,
		   status = excluded.status,
		   type_slug = excluded.type_slug,
		   priority = excluded.priority,
		   updated_at = excluded.updated_at,
		   closed_at = excluded.closed_at,
		   event_count = excluded.event_count`,
		issue.ID, sharedID, issue.Title, issue.Body, issue.Status, issue.TypeSlug, issue.Priority,
		issue.CreatedBy, issue.CreatedAt.UTC().Format(time.RFC3339Nano),
		issue.UpdatedAt.UTC().Format(time.RFC3339Nano), closedAt, issue.EventCount,
	)
	if err != nil {
		return fmt.Errorf("upserting issue: %w", err)
	}

	// Replace labels.
	_, _ = tx.ExecContext(ctx, `DELETE FROM issue_labels WHERE issue_id = ?`, issue.ID)
	for _, l := range issue.Labels {
		_, err := tx.ExecContext(ctx, `INSERT INTO issue_labels (issue_id, label_slug) VALUES (?, ?)`, issue.ID, l)
		if err != nil {
			return err
		}
	}

	// Replace assignees.
	_, _ = tx.ExecContext(ctx, `DELETE FROM issue_assignees WHERE issue_id = ?`, issue.ID)
	for _, a := range issue.Assignees {
		_, err := tx.ExecContext(ctx, `INSERT INTO issue_assignees (issue_id, actor_id) VALUES (?, ?)`, issue.ID, a)
		if err != nil {
			return err
		}
	}

	// Replace comments.
	_, _ = tx.ExecContext(ctx, `DELETE FROM issue_comments WHERE issue_id = ?`, issue.ID)
	for _, c := range issue.Comments {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO issue_comments (event_id, issue_id, actor_id, body, timestamp) VALUES (?, ?, ?, ?, ?)`,
			c.EventID, issue.ID, c.ActorID, c.Body, c.Timestamp.UTC().Format(time.RFC3339Nano),
		)
		if err != nil {
			return err
		}
	}

	// Replace attachments.
	_, _ = tx.ExecContext(ctx, `DELETE FROM issue_attachments WHERE issue_id = ?`, issue.ID)
	for _, a := range issue.Attachments {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO issue_attachments (attachment_id, issue_id, content_hash, filename, mime_type, size_bytes, added_by, added_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			a.ID, issue.ID, a.ContentHash, a.Filename, a.MimeType, a.SizeBytes,
			a.AddedBy, a.AddedAt.UTC().Format(time.RFC3339Nano),
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *Store) GetIssue(ctx context.Context, id domain.CanonicalID) (*domain.Issue, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, shared_id, title, body, status, type_slug, priority, created_by, created_at, updated_at, closed_at, event_count
		 FROM issues WHERE id = ?`, id)
	return s.scanIssue(ctx, row)
}

func (s *Store) GetIssueBySharedID(ctx context.Context, id domain.SharedID) (*domain.Issue, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, shared_id, title, body, status, type_slug, priority, created_by, created_at, updated_at, closed_at, event_count
		 FROM issues WHERE shared_id = ?`, id)
	return s.scanIssue(ctx, row)
}

func (s *Store) ListIssues(ctx context.Context, filter store.IssueFilter) ([]domain.Issue, error) {
	query := `SELECT i.id, i.shared_id, i.title, i.body, i.status, i.type_slug, i.priority, i.created_by, i.created_at, i.updated_at, i.closed_at, i.event_count FROM issues i`
	var conditions []string
	var args []any

	if filter.Status != "" {
		conditions = append(conditions, "i.status = ?")
		args = append(args, filter.Status)
	}
	if filter.Label != "" {
		query += " JOIN issue_labels il ON i.id = il.issue_id"
		conditions = append(conditions, "il.label_slug = ?")
		args = append(args, filter.Label)
	}
	if filter.Assignee != "" {
		query += " JOIN issue_assignees ia ON i.id = ia.issue_id"
		conditions = append(conditions, "ia.actor_id = ?")
		args = append(args, filter.Assignee)
	}
	if filter.Query != "" {
		conditions = append(conditions, "(i.title LIKE ? OR i.body LIKE ?)")
		q := "%" + filter.Query + "%"
		args = append(args, q, q)
	}

	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY i.updated_at DESC"

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

	var issues []domain.Issue
	for rows.Next() {
		issue, err := s.scanIssueRows(ctx, rows)
		if err != nil {
			return nil, err
		}
		issues = append(issues, *issue)
	}
	return issues, rows.Err()
}

func (s *Store) AllocateSharedID(ctx context.Context, issueID domain.CanonicalID, projectKey string) (domain.SharedID, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	// Ensure counter row exists.
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

	// Update the issue with the shared ID.
	_, err = tx.ExecContext(ctx, `UPDATE issues SET shared_id = ? WHERE id = ?`, sharedID, issueID)
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

	// Check current HEAD version.
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
	var id, issueID, eventType, payload, actorID, ts string
	var metaVersion int64
	var sig []byte

	err := row.Scan(&id, &issueID, &eventType, &payload, &metaVersion, &actorID, &ts, &sig)
	if err != nil {
		return nil, err
	}

	t, _ := time.Parse(time.RFC3339Nano, ts)
	e.ID = domain.EventID(id)
	e.IssueID = domain.CanonicalID(issueID)
	e.Type = domain.EventType(eventType)
	e.Payload = json.RawMessage(payload)
	e.MetaVersion = domain.MetaVersion(metaVersion)
	e.ActorID = domain.ActorID(actorID)
	e.Timestamp = t
	e.Signature = sig
	return &e, nil
}

func scanEventRows(rows *sql.Rows) (*domain.Event, error) {
	return scanEvent(rows)
}

func (s *Store) scanIssue(ctx context.Context, row *sql.Row) (*domain.Issue, error) {
	issue := &domain.Issue{}
	var sharedID, closedAt sql.NullString
	var createdAt, updatedAt string

	err := row.Scan(&issue.ID, &sharedID, &issue.Title, &issue.Body, &issue.Status,
		&issue.TypeSlug, &issue.Priority, &issue.CreatedBy,
		&createdAt, &updatedAt, &closedAt, &issue.EventCount)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	if sharedID.Valid {
		issue.SharedID = domain.SharedID(sharedID.String)
	}
	issue.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	issue.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	if closedAt.Valid {
		t, _ := time.Parse(time.RFC3339Nano, closedAt.String)
		issue.ClosedAt = &t
	}

	if err := s.loadIssueRelations(ctx, issue); err != nil {
		return nil, err
	}
	return issue, nil
}

func (s *Store) scanIssueRows(ctx context.Context, rows *sql.Rows) (*domain.Issue, error) {
	issue := &domain.Issue{}
	var sharedID, closedAt sql.NullString
	var createdAt, updatedAt string

	err := rows.Scan(&issue.ID, &sharedID, &issue.Title, &issue.Body, &issue.Status,
		&issue.TypeSlug, &issue.Priority, &issue.CreatedBy,
		&createdAt, &updatedAt, &closedAt, &issue.EventCount)
	if err != nil {
		return nil, err
	}

	if sharedID.Valid {
		issue.SharedID = domain.SharedID(sharedID.String)
	}
	issue.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	issue.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	if closedAt.Valid {
		t, _ := time.Parse(time.RFC3339Nano, closedAt.String)
		issue.ClosedAt = &t
	}

	if err := s.loadIssueRelations(ctx, issue); err != nil {
		return nil, err
	}
	return issue, nil
}

func (s *Store) loadIssueRelations(ctx context.Context, issue *domain.Issue) error {
	// Labels
	rows, err := s.db.QueryContext(ctx, `SELECT label_slug FROM issue_labels WHERE issue_id = ?`, issue.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	issue.Labels = []string{}
	for rows.Next() {
		var l string
		if err := rows.Scan(&l); err != nil {
			return err
		}
		issue.Labels = append(issue.Labels, l)
	}

	// Assignees
	rows2, err := s.db.QueryContext(ctx, `SELECT actor_id FROM issue_assignees WHERE issue_id = ?`, issue.ID)
	if err != nil {
		return err
	}
	defer rows2.Close()
	issue.Assignees = []domain.ActorID{}
	for rows2.Next() {
		var a string
		if err := rows2.Scan(&a); err != nil {
			return err
		}
		issue.Assignees = append(issue.Assignees, domain.ActorID(a))
	}

	// Comments
	rows3, err := s.db.QueryContext(ctx,
		`SELECT event_id, actor_id, body, timestamp FROM issue_comments WHERE issue_id = ? ORDER BY timestamp`, issue.ID)
	if err != nil {
		return err
	}
	defer rows3.Close()
	issue.Comments = []domain.Comment{}
	for rows3.Next() {
		var c domain.Comment
		var ts string
		if err := rows3.Scan(&c.EventID, &c.ActorID, &c.Body, &ts); err != nil {
			return err
		}
		c.Timestamp, _ = time.Parse(time.RFC3339Nano, ts)
		issue.Comments = append(issue.Comments, c)
	}

	// Attachments
	rows4, err := s.db.QueryContext(ctx,
		`SELECT attachment_id, content_hash, filename, mime_type, size_bytes, added_by, added_at
		 FROM issue_attachments WHERE issue_id = ? ORDER BY added_at`, issue.ID)
	if err != nil {
		return err
	}
	defer rows4.Close()
	issue.Attachments = []domain.Attachment{}
	for rows4.Next() {
		var a domain.Attachment
		var ts string
		if err := rows4.Scan(&a.ID, &a.ContentHash, &a.Filename, &a.MimeType, &a.SizeBytes, &a.AddedBy, &ts); err != nil {
			return err
		}
		a.AddedAt, _ = time.Parse(time.RFC3339Nano, ts)
		issue.Attachments = append(issue.Attachments, a)
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

// --- EventStore extensions ---

func (s *Store) HasEvent(ctx context.Context, id domain.EventID) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE id = ?`, id).Scan(&count)
	return count > 0, err
}

func (s *Store) GetAffectedIssueIDs(ctx context.Context, events []domain.Event) ([]domain.CanonicalID, error) {
	seen := make(map[domain.CanonicalID]struct{})
	var ids []domain.CanonicalID
	for _, e := range events {
		if _, ok := seen[e.IssueID]; !ok {
			seen[e.IssueID] = struct{}{}
			ids = append(ids, e.IssueID)
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
