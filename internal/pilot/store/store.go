// Package store is Pilot's own persistence layer — its private SQLite
// database holding users, sessions, wrapped custodial keys, and the
// indicator cache. See implementation-plan-v2 §2 (Pilot "Holds: Its own
// SQLite") and §8.1. This is entirely separate from DITS's project SQLite,
// which Pilot only ever reaches over MCP.
//
// Phase-4 skeleton: a Store interface + a no-op stub. No real database,
// driver, or schema yet.
package store

import (
	"context"
	"errors"

	"github.com/lenulus/pf/internal/pilot/auth"
)

// ErrNotImplemented is returned by skeleton store methods.
var ErrNotImplemented = errors.New("pilot/store: not implemented")

// ErrNotFound is returned when a requested row does not exist.
var ErrNotFound = errors.New("pilot/store: not found")

// Store is Pilot's persistence surface. It owns four logical tables:
// Users, Sessions, WrappedKeys, and Cache (§8.1).
type Store interface {
	// Users.
	GetUser(ctx context.Context, id string) (auth.User, error)
	PutUser(ctx context.Context, u auth.User) error

	// Sessions.
	GetSession(ctx context.Context, id string) (auth.Session, error)
	PutSession(ctx context.Context, s auth.Session) error
	DeleteSession(ctx context.Context, id string) error

	// WrappedKeys — wrapped custodial Ed25519 private keys, keyed by actor.
	GetWrappedKey(ctx context.Context, actor string) ([]byte, error)
	PutWrappedKey(ctx context.Context, actor string, wrapped []byte) error

	// Cache — persisted indicator values (§10.3).
	GetCache(ctx context.Context, key string) ([]byte, error)
	PutCache(ctx context.Context, key string, value []byte) error

	// Close releases the underlying database.
	Close() error
}

// stubStore is the Phase-4 no-op Store.
type stubStore struct{}

// NewStub returns a Store with no backing database.
// TODO(phase 4): replace with a SQLite-backed implementation (migrations,
// connection pool, prepared statements).
func NewStub() Store { return stubStore{} }

func (stubStore) GetUser(context.Context, string) (auth.User, error) {
	return auth.User{}, ErrNotImplemented
}

func (stubStore) PutUser(context.Context, auth.User) error { return ErrNotImplemented }

func (stubStore) GetSession(context.Context, string) (auth.Session, error) {
	return auth.Session{}, ErrNotImplemented
}

func (stubStore) PutSession(context.Context, auth.Session) error { return ErrNotImplemented }

func (stubStore) DeleteSession(context.Context, string) error { return ErrNotImplemented }

func (stubStore) GetWrappedKey(context.Context, string) ([]byte, error) {
	return nil, ErrNotImplemented
}

func (stubStore) PutWrappedKey(context.Context, string, []byte) error {
	return ErrNotImplemented
}

func (stubStore) GetCache(context.Context, string) ([]byte, error) {
	return nil, ErrNotImplemented
}

func (stubStore) PutCache(context.Context, string, []byte) error {
	return ErrNotImplemented
}

func (stubStore) Close() error { return nil }
