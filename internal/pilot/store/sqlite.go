package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/lenulus/pf/internal/pilot/auth"

	_ "modernc.org/sqlite" // registers the "sqlite" driver (pure-Go, no cgo).
)

// sqliteStore is the real SQLite-backed Store: Pilot's own database holding
// users, sessions, wrapped custodial keys, and the indicator cache (§8.1). It
// is entirely separate from DITS's project SQLite, which Pilot reaches only
// over MCP.
type sqliteStore struct {
	db *sql.DB
}

// NewSQLite opens (creating if absent) Pilot's database at path and applies the
// schema. Use ":memory:" for an ephemeral store (tests). Callers must Close it.
func NewSQLite(path string) (Store, error) {
	if path == "" {
		path = ":memory:"
	}
	db, err := sql.Open("sqlite", path+"?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("pilot/store: open %s: %w", path, err)
	}
	// A single shared connection keeps an in-memory DB from being dropped
	// between calls (each ":memory:" connection is its own database).
	db.SetMaxOpenConns(1)
	s := &sqliteStore{db: db}
	if err := s.migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// Close releases the database handle.
func (s *sqliteStore) Close() error { return s.db.Close() }

func (s *sqliteStore) migrate(ctx context.Context) error {
	const schema = `
CREATE TABLE IF NOT EXISTS users (
	id    TEXT PRIMARY KEY,
	actor TEXT NOT NULL,
	email TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS sessions (
	id         TEXT PRIMARY KEY,
	user_id    TEXT NOT NULL,
	actor      TEXT NOT NULL,
	issued_at  INTEGER NOT NULL,
	expires_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS wrapped_keys (
	actor   TEXT PRIMARY KEY,
	wrapped BLOB NOT NULL
);
CREATE TABLE IF NOT EXISTS cache (
	key   TEXT PRIMARY KEY,
	value BLOB NOT NULL
);`
	if _, err := s.db.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("pilot/store: migrate: %w", err)
	}
	return nil
}

// --- Users ---

func (s *sqliteStore) GetUser(ctx context.Context, id string) (auth.User, error) {
	var u auth.User
	err := s.db.QueryRowContext(ctx, `SELECT id, actor, email FROM users WHERE id = ?`, id).
		Scan(&u.ID, &u.Actor, &u.Email)
	if errors.Is(err, sql.ErrNoRows) {
		return auth.User{}, ErrNotFound
	}
	if err != nil {
		return auth.User{}, err
	}
	return u, nil
}

func (s *sqliteStore) PutUser(ctx context.Context, u auth.User) error {
	if u.ID == "" {
		return fmt.Errorf("pilot/store: user id is required")
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO users (id, actor, email) VALUES (?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET actor = excluded.actor, email = excluded.email`,
		u.ID, u.Actor, u.Email)
	return err
}

// --- Sessions ---

func (s *sqliteStore) GetSession(ctx context.Context, id string) (auth.Session, error) {
	var (
		sess              auth.Session
		issuedAt, expires int64
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT id, user_id, actor, issued_at, expires_at FROM sessions WHERE id = ?`, id).
		Scan(&sess.ID, &sess.UserID, &sess.Actor, &issuedAt, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return auth.Session{}, ErrNotFound
	}
	if err != nil {
		return auth.Session{}, err
	}
	sess.IssuedAt = time.Unix(issuedAt, 0).UTC()
	sess.ExpiresAt = time.Unix(expires, 0).UTC()
	return sess, nil
}

func (s *sqliteStore) PutSession(ctx context.Context, sess auth.Session) error {
	if sess.ID == "" {
		return fmt.Errorf("pilot/store: session id is required")
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sessions (id, user_id, actor, issued_at, expires_at) VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET user_id = excluded.user_id, actor = excluded.actor,
		     issued_at = excluded.issued_at, expires_at = excluded.expires_at`,
		sess.ID, sess.UserID, sess.Actor, sess.IssuedAt.Unix(), sess.ExpiresAt.Unix())
	return err
}

func (s *sqliteStore) DeleteSession(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE id = ?`, id)
	return err
}

// --- WrappedKeys ---

func (s *sqliteStore) GetWrappedKey(ctx context.Context, actor string) ([]byte, error) {
	var b []byte
	err := s.db.QueryRowContext(ctx, `SELECT wrapped FROM wrapped_keys WHERE actor = ?`, actor).Scan(&b)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return b, nil
}

func (s *sqliteStore) PutWrappedKey(ctx context.Context, actor string, wrapped []byte) error {
	if actor == "" {
		return fmt.Errorf("pilot/store: actor is required")
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO wrapped_keys (actor, wrapped) VALUES (?, ?)
		 ON CONFLICT(actor) DO UPDATE SET wrapped = excluded.wrapped`,
		actor, wrapped)
	return err
}

// --- Cache ---

func (s *sqliteStore) GetCache(ctx context.Context, key string) ([]byte, error) {
	var b []byte
	err := s.db.QueryRowContext(ctx, `SELECT value FROM cache WHERE key = ?`, key).Scan(&b)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return b, nil
}

func (s *sqliteStore) PutCache(ctx context.Context, key string, value []byte) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO cache (key, value) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key, value)
	return err
}
