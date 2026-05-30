// Package auth holds Pilot's identity bridge: an OAuth/OIDC provider
// abstraction and the session-cookie model. See implementation-plan-v2
// §8.2 (identity bridge) and §8.3 (self-sovereign opt-out).
//
// Phase-4 skeleton: provider interface + session/user type stubs. No real
// OAuth flow, token exchange, or cookie crypto yet.
package auth

import (
	"context"
	"errors"
	"time"
)

// ErrNotImplemented is returned by skeleton auth methods.
var ErrNotImplemented = errors.New("pilot/auth: not implemented")

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
// TODO(phase 4): add issuer, subject, email, and verified-email fields.
type Identity struct {
	Subject string
	Email   string
}

// User is a Pilot account, linking an upstream identity to a DITS actor.
// TODO(phase 4): persisted in the store package (Users table).
type User struct {
	ID    string
	Actor string // DITS actor id this user signs as
	Email string
}

// Session is the cookie-backed session model (§8.2). A session signs only
// as its bound actor (enforced later; see plan §8.4 TestSessionScope).
// TODO(phase 4): add CSRF token, secure cookie encoding, and rotation.
type Session struct {
	ID        string
	UserID    string
	Actor     string
	IssuedAt  time.Time
	ExpiresAt time.Time
}

// stubProvider is the Phase-4 no-op Provider.
type stubProvider struct{}

// NewStubProvider returns a Provider that is not yet wired to any OAuth
// endpoint.
// TODO(phase 4): replace with per-upstream OIDC constructors.
func NewStubProvider() Provider { return stubProvider{} }

func (stubProvider) AuthCodeURL(string) string { return "" }

func (stubProvider) Exchange(context.Context, string) (Identity, error) {
	return Identity{}, ErrNotImplemented
}
