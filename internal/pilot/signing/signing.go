// Package signing holds Pilot's custodial Ed25519 signing path and the
// key-wrapping (KMS / local-master-key) abstraction. See
// implementation-plan-v2 §8.2: Pilot generates a custodial keypair per user
// on first sign-in, stores it WRAPPED in its SQLite, and signs mutations
// server-side using the same canonical-JSON + Ed25519 algorithm DITS uses,
// so DITS cannot tell a Pilot-signed event from a CLI-signed one.
//
// Phase-4 skeleton: Signer and Wrapper interfaces + stubs. No real crypto,
// no key generation, no KMS calls.
package signing

import (
	"context"
	"errors"
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

// stubSigner is the Phase-4 no-op Signer.
type stubSigner struct{}

// NewStubSigner returns a Signer that does not yet hold or use any keys.
// TODO(phase 4): generate Ed25519 keys on first sign-in; unwrap-then-sign
// per mutation; wire PublicKey to actor_register via the mcp package.
func NewStubSigner() Signer { return stubSigner{} }

func (stubSigner) Sign(context.Context, string, []byte) ([]byte, error) {
	return nil, ErrNotImplemented
}

func (stubSigner) PublicKey(context.Context, string) ([]byte, error) {
	return nil, ErrNotImplemented
}

// stubWrapper is the Phase-4 no-op Wrapper.
type stubWrapper struct{}

// NewStubWrapper returns a Wrapper with no backing master key or KMS.
// TODO(phase 4): implement local-master-key AEAD wrap and a KMS adapter.
func NewStubWrapper() Wrapper { return stubWrapper{} }

func (stubWrapper) Wrap(context.Context, []byte) ([]byte, error) {
	return nil, ErrNotImplemented
}

func (stubWrapper) Unwrap(context.Context, []byte) ([]byte, error) {
	return nil, ErrNotImplemented
}
