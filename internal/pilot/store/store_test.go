package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/lenulus/pf/internal/pilot/auth"
)

func newStore(t *testing.T) Store {
	t.Helper()
	s, err := NewSQLite(":memory:")
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestSQLiteUsersRoundTrip(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)

	if _, err := s.GetUser(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetUser(missing) = %v, want ErrNotFound", err)
	}

	u := auth.User{ID: "user_1", Actor: "actor_pilot_1", Email: "a@dev.local"}
	if err := s.PutUser(ctx, u); err != nil {
		t.Fatalf("PutUser: %v", err)
	}
	got, err := s.GetUser(ctx, "user_1")
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if got != u {
		t.Errorf("GetUser = %+v, want %+v", got, u)
	}

	// Upsert.
	u.Email = "b@dev.local"
	if err := s.PutUser(ctx, u); err != nil {
		t.Fatalf("PutUser upsert: %v", err)
	}
	got, _ = s.GetUser(ctx, "user_1")
	if got.Email != "b@dev.local" {
		t.Errorf("upsert email = %q, want b@dev.local", got.Email)
	}
}

func TestSQLiteSessionsRoundTrip(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)

	now := time.Unix(1_700_000_000, 0).UTC()
	sess := auth.Session{
		ID: "sess_1", UserID: "user_1", Actor: "actor_pilot_1",
		IssuedAt: now, ExpiresAt: now.Add(time.Hour),
	}
	if err := s.PutSession(ctx, sess); err != nil {
		t.Fatalf("PutSession: %v", err)
	}
	got, err := s.GetSession(ctx, "sess_1")
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got.ID != sess.ID || got.Actor != sess.Actor || !got.ExpiresAt.Equal(sess.ExpiresAt) {
		t.Errorf("GetSession = %+v, want %+v", got, sess)
	}

	if err := s.DeleteSession(ctx, "sess_1"); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	if _, err := s.GetSession(ctx, "sess_1"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetSession after delete = %v, want ErrNotFound", err)
	}
}

func TestSQLiteWrappedKeysAndCache(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)

	if _, err := s.GetWrappedKey(ctx, "actor_x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetWrappedKey(missing) = %v, want ErrNotFound", err)
	}
	if err := s.PutWrappedKey(ctx, "actor_x", []byte{1, 2, 3}); err != nil {
		t.Fatalf("PutWrappedKey: %v", err)
	}
	wk, err := s.GetWrappedKey(ctx, "actor_x")
	if err != nil || string(wk) != string([]byte{1, 2, 3}) {
		t.Fatalf("GetWrappedKey = %v, %v", wk, err)
	}

	if err := s.PutCache(ctx, "kpi:open", []byte("42")); err != nil {
		t.Fatalf("PutCache: %v", err)
	}
	cv, err := s.GetCache(ctx, "kpi:open")
	if err != nil || string(cv) != "42" {
		t.Fatalf("GetCache = %q, %v", cv, err)
	}
}
