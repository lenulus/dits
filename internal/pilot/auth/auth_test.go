package auth

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"
)

// memStore is an in-memory userSessionStore for the Manager tests. Declared
// here (not imported from pilot/store) to avoid the store→auth import cycle.
type memStore struct {
	users    map[string]User
	sessions map[string]Session
}

func newMemStore() *memStore {
	return &memStore{users: map[string]User{}, sessions: map[string]Session{}}
}

func (m *memStore) GetUser(_ context.Context, id string) (User, error) {
	u, ok := m.users[id]
	if !ok {
		return User{}, ErrNoSession // any error ⇒ first sign-in
	}
	return u, nil
}
func (m *memStore) PutUser(_ context.Context, u User) error { m.users[u.ID] = u; return nil }
func (m *memStore) GetSession(_ context.Context, id string) (Session, error) {
	s, ok := m.sessions[id]
	if !ok {
		return Session{}, ErrNoSession
	}
	return s, nil
}
func (m *memStore) PutSession(_ context.Context, s Session) error { m.sessions[s.ID] = s; return nil }
func (m *memStore) DeleteSession(_ context.Context, id string) error {
	delete(m.sessions, id)
	return nil
}

// TestSignInFlowProvisionsAndRegisters exercises the full dev sign-in: exchange
// → first-sign-in user creation → custodial key provision → actor_register →
// session issue → resolve. A second sign-in for the same subject reuses the
// user and does NOT re-provision/register.
func TestSignInFlowProvisionsAndRegisters(t *testing.T) {
	ctx := context.Background()
	store := newMemStore()

	var provisioned []string
	var registered []struct{ actor, pubHex string }
	provision := func(_ context.Context, actor string) ([]byte, error) {
		provisioned = append(provisioned, actor)
		pub, _, _ := ed25519.GenerateKey(rand.Reader)
		return pub, nil
	}
	register := func(_ context.Context, actor, pubHex string) error {
		registered = append(registered, struct{ actor, pubHex string }{actor, pubHex})
		return nil
	}

	m := NewManager(NewDevProvider(""), store, provision, register)

	sess, err := m.SignIn(ctx, "alice")
	if err != nil {
		t.Fatalf("SignIn: %v", err)
	}
	if sess.Actor != "actor_pilot_alice" {
		t.Errorf("session actor = %q, want actor_pilot_alice", sess.Actor)
	}
	if len(provisioned) != 1 || provisioned[0] != "actor_pilot_alice" {
		t.Errorf("provisioned = %v, want one actor_pilot_alice", provisioned)
	}
	if len(registered) != 1 || registered[0].actor != "actor_pilot_alice" || registered[0].pubHex == "" {
		t.Errorf("registered = %+v, want one actor_pilot_alice with a hex key", registered)
	}

	// Resolve the live session.
	got, err := m.Resolve(ctx, sess.ID)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.Actor != sess.Actor {
		t.Errorf("resolved actor = %q, want %q", got.Actor, sess.Actor)
	}

	// Second sign-in: same subject, no re-provision/register.
	if _, err := m.SignIn(ctx, "alice"); err != nil {
		t.Fatalf("SignIn 2: %v", err)
	}
	if len(provisioned) != 1 {
		t.Errorf("second sign-in re-provisioned: %v", provisioned)
	}
	if len(registered) != 1 {
		t.Errorf("second sign-in re-registered: %+v", registered)
	}
}

// TestResolveExpiredSession deletes and rejects an expired session.
func TestResolveExpiredSession(t *testing.T) {
	ctx := context.Background()
	store := newMemStore()
	m := NewManager(NewDevProvider(""), store, nil, nil)

	clock := time.Unix(1_700_000_000, 0).UTC()
	m.SetClock(func() time.Time { return clock })
	m.SessionTTL = time.Hour

	sess, err := m.SignIn(ctx, "bob")
	if err != nil {
		t.Fatalf("SignIn: %v", err)
	}
	// Advance past expiry.
	clock = clock.Add(2 * time.Hour)
	if _, err := m.Resolve(ctx, sess.ID); err != ErrNoSession {
		t.Errorf("Resolve(expired) = %v, want ErrNoSession", err)
	}
	if _, ok := store.sessions[sess.ID]; ok {
		t.Errorf("expired session was not evicted")
	}
}

// TestSelfSovereignSkipsCustodial: a Manager with no provisioner/registrar still
// signs the user in (§8.3 opt-out) without minting a Pilot-held key.
func TestSelfSovereignSkipsCustodial(t *testing.T) {
	ctx := context.Background()
	m := NewManager(NewDevProvider(""), newMemStore(), nil, nil)
	sess, err := m.SignIn(ctx, "carol")
	if err != nil {
		t.Fatalf("SignIn: %v", err)
	}
	if sess.Actor != "actor_pilot_carol" {
		t.Errorf("actor = %q, want actor_pilot_carol", sess.Actor)
	}
}

// TestStubProviderExchangeErrors documents the explicit non-wired provider.
func TestStubProviderExchangeErrors(t *testing.T) {
	if _, err := NewStubProvider().Exchange(context.Background(), "x"); err == nil {
		t.Errorf("stub Exchange should error")
	}
}
