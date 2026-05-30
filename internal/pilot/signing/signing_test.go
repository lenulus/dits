package signing

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"io"
	"testing"
)

// memKeyStore is an in-memory KeyStore for the signer round-trip test.
type memKeyStore struct{ m map[string][]byte }

func newMemKeyStore() *memKeyStore { return &memKeyStore{m: map[string][]byte{}} }

func (s *memKeyStore) GetWrappedKey(_ context.Context, actor string) ([]byte, error) {
	return s.m[actor], nil
}
func (s *memKeyStore) PutWrappedKey(_ context.Context, actor string, wrapped []byte) error {
	s.m[actor] = wrapped
	return nil
}

func newMasterKey(t *testing.T) []byte {
	t.Helper()
	k := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, k); err != nil {
		t.Fatalf("master key: %v", err)
	}
	return k
}

// TestWrapperRoundTrip checks the local AES-GCM wrapper unwraps what it wraps
// and that distinct calls produce distinct ciphertexts (fresh nonce).
func TestWrapperRoundTrip(t *testing.T) {
	w, err := NewLocalWrapper(newMasterKey(t))
	if err != nil {
		t.Fatalf("NewLocalWrapper: %v", err)
	}
	ctx := context.Background()
	plain := []byte("a custodial private key's bytes")

	w1, err := w.Wrap(ctx, plain)
	if err != nil {
		t.Fatalf("Wrap: %v", err)
	}
	w2, err := w.Wrap(ctx, plain)
	if err != nil {
		t.Fatalf("Wrap 2: %v", err)
	}
	if string(w1) == string(w2) {
		t.Errorf("two wraps of the same plaintext were identical (nonce not random)")
	}
	got, err := w.Unwrap(ctx, w1)
	if err != nil {
		t.Fatalf("Unwrap: %v", err)
	}
	if string(got) != string(plain) {
		t.Errorf("unwrap mismatch: got %q want %q", got, plain)
	}

	// A wrapper with a different master key must not unwrap.
	other, _ := NewLocalWrapper(newMasterKey(t))
	if _, err := other.Unwrap(ctx, w1); err == nil {
		t.Errorf("expected unwrap failure under a different master key")
	}
}

// TestCustodialSignerRoundTrip provisions a key on first use, signs the shared
// canonical vector, and verifies the signature with the actor's public key —
// the same Ed25519-over-canonical-JSON contract DITS uses. It also confirms key
// provisioning is idempotent (same public key on a second EnsureKey).
func TestCustodialSignerRoundTrip(t *testing.T) {
	ctx := context.Background()
	wrapper, err := NewLocalWrapper(newMasterKey(t))
	if err != nil {
		t.Fatalf("wrapper: %v", err)
	}
	store := newMemKeyStore()
	signer := NewSigner(store, wrapper)

	const actor = "actor_pilot_test"

	pub, err := Provision(ctx, signer, actor)
	if err != nil {
		t.Fatalf("Provision: %v", err)
	}
	if len(pub) != ed25519.PublicKeySize {
		t.Fatalf("public key size = %d, want %d", len(pub), ed25519.PublicKeySize)
	}
	if len(store.m[actor]) == 0 {
		t.Fatalf("wrapped key was not persisted")
	}

	// Provisioning is idempotent: same actor → same key.
	pub2, err := Provision(ctx, signer, actor)
	if err != nil {
		t.Fatalf("Provision 2: %v", err)
	}
	if string(pub) != string(pub2) {
		t.Errorf("EnsureKey was not idempotent: public keys differ")
	}

	// PublicKey() agrees with the provisioned key.
	pkBytes, err := signer.PublicKey(ctx, actor)
	if err != nil {
		t.Fatalf("PublicKey: %v", err)
	}
	if string(pkBytes) != string(pub) {
		t.Errorf("PublicKey disagrees with provisioned key")
	}

	// Sign the canonical vector and verify with the actor's public key.
	canonical, err := CanonicalEventJSON(canonicalVectorEvent())
	if err != nil {
		t.Fatalf("canonical: %v", err)
	}
	sig, err := signer.Sign(ctx, actor, canonical)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if !ed25519.Verify(ed25519.PublicKey(pub), canonical, sig) {
		t.Errorf("signature did not verify against the actor's public key")
	}

	// Signing for an unprovisioned actor errors.
	if _, err := signer.Sign(ctx, "actor_unknown", canonical); err == nil {
		t.Errorf("expected error signing for an unprovisioned actor")
	}
}

// TestStubSignerNotImplemented documents the stub's behavior.
func TestStubSignerNotImplemented(t *testing.T) {
	s := NewStubSigner()
	if _, err := s.Sign(context.Background(), "a", nil); err == nil {
		t.Errorf("stub Sign should return an error")
	}
	if _, err := Provision(context.Background(), s, "a"); err == nil {
		t.Errorf("Provision over a non-provisioning signer should error")
	}
}
