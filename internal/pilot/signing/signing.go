// Package signing holds Pilot's custodial Ed25519 signing path and the
// key-wrapping (KMS / local-master-key) abstraction. See
// implementation-plan-v2 §8.2: Pilot generates a custodial keypair per user
// on first sign-in, stores it WRAPPED in its SQLite, and signs mutations
// server-side using the same canonical-JSON + Ed25519 algorithm DITS uses,
// so DITS cannot tell a Pilot-signed event from a CLI-signed one.
//
// The canonical-JSON encoding is reimplemented in this package's canonical.go
// (Pilot may not import internal/crypto); a test pins it to a literal the
// substrate also reproduces.
package signing

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
)

// ErrNotImplemented is returned by skeleton signing methods.
var ErrNotImplemented = errors.New("pilot/signing: not implemented")

// Signer signs a canonicalised event payload on behalf of an actor, using
// that actor's (unwrapped) custodial Ed25519 key. The canonical-JSON
// encoding MUST match DITS's crypto.CanonicalEventJSON (§8.2) — reimplemented
// in the Pilot tree rather than imported, to keep the forbidden-imports
// boundary intact.
type Signer interface {
	// Sign returns the Ed25519 signature over the canonical event bytes.
	Sign(ctx context.Context, actor string, canonical []byte) (sig []byte, err error)
	// PublicKey returns the actor's custodial public key (for actor_register).
	PublicKey(ctx context.Context, actor string) (pub []byte, err error)
}

// Wrapper wraps and unwraps custodial private keys at rest. Backed by a
// local master key (single-binary deploys) or an external KMS (§8.2).
type Wrapper interface {
	Wrap(ctx context.Context, plaintext []byte) (wrapped []byte, err error)
	Unwrap(ctx context.Context, wrapped []byte) (plaintext []byte, err error)
}

// KeyStore is the subset of pilot/store.Store the custodial Signer needs:
// persistence for wrapped Ed25519 private keys, keyed by actor. Declared as a
// local interface so signing does not depend on the whole Store surface (and
// so tests can supply an in-memory map).
type KeyStore interface {
	GetWrappedKey(ctx context.Context, actor string) ([]byte, error)
	PutWrappedKey(ctx context.Context, actor string, wrapped []byte) error
}

// custodialSigner is the real Signer: it keeps each actor's Ed25519 private
// key WRAPPED in the KeyStore and unwraps-then-signs per request. The private
// key never persists in plaintext.
type custodialSigner struct {
	store   KeyStore
	wrapper Wrapper
}

// NewSigner returns a custodial Signer backed by store (for wrapped keys) and
// wrapper (for at-rest encryption). EnsureKey generates a key on first use.
func NewSigner(store KeyStore, wrapper Wrapper) Signer {
	return &custodialSigner{store: store, wrapper: wrapper}
}

// EnsureKey guarantees actor has a custodial keypair, generating and storing a
// wrapped one on first call, and returns its public key. Idempotent: a second
// call returns the existing key's public half. This is what the auth layer
// calls on first sign-in before actor_register. NewSigner returns the Signer
// interface, so callers type-assert to EnsureKeyer (or use Provision).
func (s *custodialSigner) EnsureKey(ctx context.Context, actor string) (ed25519.PublicKey, error) {
	if wrapped, err := s.store.GetWrappedKey(ctx, actor); err == nil && len(wrapped) > 0 {
		priv, err := s.unwrapPriv(ctx, wrapped)
		if err != nil {
			return nil, err
		}
		return priv.Public().(ed25519.PublicKey), nil
	}

	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		return nil, fmt.Errorf("generating custodial key for %s: %w", actor, err)
	}
	wrapped, err := s.wrapper.Wrap(ctx, priv)
	if err != nil {
		return nil, fmt.Errorf("wrapping custodial key for %s: %w", actor, err)
	}
	if err := s.store.PutWrappedKey(ctx, actor, wrapped); err != nil {
		return nil, fmt.Errorf("storing custodial key for %s: %w", actor, err)
	}
	return pub, nil
}

func (s *custodialSigner) Sign(ctx context.Context, actor string, canonical []byte) ([]byte, error) {
	priv, err := s.loadPriv(ctx, actor)
	if err != nil {
		return nil, err
	}
	return ed25519.Sign(priv, canonical), nil
}

func (s *custodialSigner) PublicKey(ctx context.Context, actor string) ([]byte, error) {
	priv, err := s.loadPriv(ctx, actor)
	if err != nil {
		return nil, err
	}
	return []byte(priv.Public().(ed25519.PublicKey)), nil
}

// loadPriv fetches and unwraps actor's private key, erroring if none exists
// (callers must EnsureKey first).
func (s *custodialSigner) loadPriv(ctx context.Context, actor string) (ed25519.PrivateKey, error) {
	wrapped, err := s.store.GetWrappedKey(ctx, actor)
	if err != nil {
		return nil, fmt.Errorf("loading custodial key for %s: %w", actor, err)
	}
	if len(wrapped) == 0 {
		return nil, fmt.Errorf("no custodial key for actor %s", actor)
	}
	return s.unwrapPriv(ctx, wrapped)
}

func (s *custodialSigner) unwrapPriv(ctx context.Context, wrapped []byte) (ed25519.PrivateKey, error) {
	plain, err := s.wrapper.Unwrap(ctx, wrapped)
	if err != nil {
		return nil, fmt.Errorf("unwrapping custodial key: %w", err)
	}
	if len(plain) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("unwrapped key has wrong size %d (want %d)", len(plain), ed25519.PrivateKeySize)
	}
	return ed25519.PrivateKey(plain), nil
}

// EnsureKeyer is implemented by the custodial Signer: it provisions an actor's
// key on first sign-in. The auth layer uses it (Provision) without depending on
// the concrete type.
type EnsureKeyer interface {
	EnsureKey(ctx context.Context, actor string) (ed25519.PublicKey, error)
}

// Provision ensures actor has a custodial key via signer (which must be a
// *custodialSigner / EnsureKeyer), returning the public key. A non-provisioning
// Signer (e.g. the stub) yields ErrNotImplemented.
func Provision(ctx context.Context, signer Signer, actor string) (ed25519.PublicKey, error) {
	if ek, ok := signer.(EnsureKeyer); ok {
		return ek.EnsureKey(ctx, actor)
	}
	return nil, ErrNotImplemented
}

// stubSigner is the no-op Signer for handlers wired with no key material.
type stubSigner struct{}

// NewStubSigner returns a Signer that does not hold or use any keys. Used by
// handlers running without a configured custodial path.
func NewStubSigner() Signer { return stubSigner{} }

func (stubSigner) Sign(context.Context, string, []byte) ([]byte, error) {
	return nil, ErrNotImplemented
}

func (stubSigner) PublicKey(context.Context, string) ([]byte, error) {
	return nil, ErrNotImplemented
}
