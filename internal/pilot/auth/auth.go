// Package auth holds Pilot's identity bridge: an OAuth/OIDC provider
// abstraction, a local dev provider, and the session-cookie model. See
// implementation-plan-v2 §8.2 (identity bridge) and §8.3 (self-sovereign
// opt-out).
//
// The Provider interface is the integration seam: NewDevProvider authenticates
// locally (no external IdP, no creds needed) so the whole sign-in → custodial
// key → actor_register → session flow is exercisable end-to-end; a real
// Google/GitHub/OIDC provider is a drop-in replacement implementing the same
// two methods. Real token exchange against an external IdP is the one
// documented remaining seam.
package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

// ErrNotImplemented is returned by skeleton auth methods.
var ErrNotImplemented = errors.New("pilot/auth: not implemented")

// ErrNoSession is returned when a session id is unknown or expired.
var ErrNoSession = errors.New("pilot/auth: no such session")

// Provider is a generic OAuth/OIDC provider, configurable per upstream
// (Google, GitHub, generic OIDC) via Pilot env/flags (§8.2).
type Provider interface {
	// AuthCodeURL returns the URL to redirect the user to for consent.
	AuthCodeURL(state string) string
	// Exchange swaps an authorization code for the authenticated identity.
	Exchange(ctx context.Context, code string) (Identity, error)
}

// Identity is the upstream identity returned after a successful OAuth
// exchange, before it is mapped to a Pilot user / DITS actor.
type Identity struct {
	Subject string
	Email   string
}

// User is a Pilot account, linking an upstream identity to a DITS actor.
type User struct {
	ID    string
	Actor string // DITS actor id this user signs as
	Email string
}

// Session is the cookie-backed session model (§8.2). A session signs only
// as its bound actor (enforced later; see plan §8.4 TestSessionScope).
type Session struct {
	ID        string
	UserID    string
	Actor     string
	IssuedAt  time.Time
	ExpiresAt time.Time
}

// Expired reports whether the session is past its expiry as of now.
func (s Session) Expired(now time.Time) bool { return !s.ExpiresAt.IsZero() && now.After(s.ExpiresAt) }

// userSessionStore is the slice of Pilot's store.Store the session Manager
// needs. Declared locally to avoid an import cycle (store imports auth).
type userSessionStore interface {
	GetUser(ctx context.Context, id string) (User, error)
	PutUser(ctx context.Context, u User) error
	GetSession(ctx context.Context, id string) (Session, error)
	PutSession(ctx context.Context, s Session) error
	DeleteSession(ctx context.Context, id string) error
}

// Provisioner provisions a custodial signing key for an actor on first
// sign-in and returns its public key (hex-encoded by RegisterFunc's caller).
// signing.Provision satisfies this.
type Provisioner func(ctx context.Context, actor string) (pub []byte, err error)

// Registrar registers an actor's public key with DITS (mcp.Client.ActorRegister
// hex-encodes and forwards it).
type Registrar func(ctx context.Context, actor, publicKeyHex string) error

// Manager runs the sign-in flow and the session lifecycle over a store. On a
// user's first sign-in it derives a stable Pilot actor id, provisions that
// actor's custodial key (Provisioner) and registers it with DITS (Registrar),
// then issues a session. Subsequent sign-ins reuse the stored user.
type Manager struct {
	provider  Provider
	store     userSessionStore
	provision Provisioner
	register  Registrar

	// SessionTTL is how long an issued session is valid (default 12h).
	SessionTTL time.Duration
	// now is the clock, injected for tests; defaults to time.Now.
	now func() time.Time
}

// NewManager wires a Manager. provision/register may be nil (the custodial
// bridge is then skipped — a self-sovereign deploy, §8.3 — and the user signs
// as an actor with no Pilot-held key).
func NewManager(p Provider, store userSessionStore, provision Provisioner, register Registrar) *Manager {
	return &Manager{
		provider:   p,
		store:      store,
		provision:  provision,
		register:   register,
		SessionTTL: 12 * time.Hour,
		now:        time.Now,
	}
}

// SetClock overrides the Manager's clock (tests). A nil fn is ignored.
func (m *Manager) SetClock(fn func() time.Time) {
	if fn != nil {
		m.now = fn
	}
}

// AuthCodeURL exposes the provider's consent URL.
func (m *Manager) AuthCodeURL(state string) string { return m.provider.AuthCodeURL(state) }

// SignIn completes an OAuth exchange for code, upserts the resulting user
// (provisioning + registering its custodial key on first sight), and issues a
// session. The returned Session.ID is the cookie value the caller sets.
func (m *Manager) SignIn(ctx context.Context, code string) (Session, error) {
	ident, err := m.provider.Exchange(ctx, code)
	if err != nil {
		return Session{}, fmt.Errorf("exchange: %w", err)
	}
	if ident.Subject == "" {
		return Session{}, fmt.Errorf("upstream identity has no subject")
	}

	userID := "user_" + ident.Subject
	actor := "actor_pilot_" + ident.Subject

	u, err := m.store.GetUser(ctx, userID)
	firstSignIn := err != nil // any error (incl. not-found) ⇒ treat as first sign-in
	if firstSignIn {
		u = User{ID: userID, Actor: actor, Email: ident.Email}
		if err := m.store.PutUser(ctx, u); err != nil {
			return Session{}, fmt.Errorf("persisting user: %w", err)
		}
		// Provision + register the custodial key. Best-effort wiring: when no
		// Provisioner/Registrar is configured this is a no-op (§8.3).
		if m.provision != nil {
			pub, perr := m.provision(ctx, u.Actor)
			if perr != nil {
				return Session{}, fmt.Errorf("provisioning custodial key: %w", perr)
			}
			if m.register != nil {
				if rerr := m.register(ctx, u.Actor, hex.EncodeToString(pub)); rerr != nil {
					return Session{}, fmt.Errorf("registering actor: %w", rerr)
				}
			}
		}
	}

	now := m.now()
	sess := Session{
		ID:        newID(),
		UserID:    u.ID,
		Actor:     u.Actor,
		IssuedAt:  now,
		ExpiresAt: now.Add(m.SessionTTL),
	}
	if err := m.store.PutSession(ctx, sess); err != nil {
		return Session{}, fmt.Errorf("persisting session: %w", err)
	}
	return sess, nil
}

// Resolve loads a live session by id, returning ErrNoSession if it is unknown
// or expired (expired sessions are deleted as a side effect).
func (m *Manager) Resolve(ctx context.Context, id string) (Session, error) {
	sess, err := m.store.GetSession(ctx, id)
	if err != nil {
		return Session{}, ErrNoSession
	}
	if sess.Expired(m.now()) {
		_ = m.store.DeleteSession(ctx, id)
		return Session{}, ErrNoSession
	}
	return sess, nil
}

// SignOut deletes a session (idempotent).
func (m *Manager) SignOut(ctx context.Context, id string) error {
	return m.store.DeleteSession(ctx, id)
}

// newID returns a random 128-bit hex token for user/session ids.
func newID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// --- Dev provider ---

// devProvider is a local, no-IdP Provider for development and tests. Exchange
// treats the authorization code itself as the subject (and synthesizes an
// email), so a sign-in flow can run with no external credentials. This is the
// working LOCAL provider the plan calls for; a real OIDC provider replaces it
// without touching Manager.
type devProvider struct {
	// redirectBase is the local URL AuthCodeURL points at (the dev sign-in page).
	redirectBase string
}

// NewDevProvider returns a Provider that authenticates locally. redirectBase is
// where AuthCodeURL sends the browser (e.g. "/auth/dev"); empty defaults to it.
func NewDevProvider(redirectBase string) Provider {
	if redirectBase == "" {
		redirectBase = "/auth/dev"
	}
	return devProvider{redirectBase: redirectBase}
}

func (d devProvider) AuthCodeURL(state string) string {
	return d.redirectBase + "?state=" + state
}

func (d devProvider) Exchange(_ context.Context, code string) (Identity, error) {
	if code == "" {
		return Identity{}, fmt.Errorf("dev provider: empty code")
	}
	return Identity{Subject: code, Email: code + "@dev.local"}, nil
}

// NewStubProvider is retained for callers that want an explicitly non-wired
// provider; it returns ErrNotImplemented from Exchange.
func NewStubProvider() Provider { return stubProvider{} }

type stubProvider struct{}

func (stubProvider) AuthCodeURL(string) string { return "" }

func (stubProvider) Exchange(context.Context, string) (Identity, error) {
	return Identity{}, ErrNotImplemented
}
