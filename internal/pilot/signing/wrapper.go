package signing

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"
)

// localWrapper wraps custodial keys at rest with AES-256-GCM under a single
// local master key — the single-binary deploy mode of §8.2. The master key is
// held in memory for the process lifetime (loaded from config/KMS by the
// caller). An external-KMS Wrapper is the documented alternative seam: swap
// NewLocalWrapper for a KMS-backed Wrapper with the same interface.
type localWrapper struct {
	gcm cipher.AEAD
}

// NewLocalWrapper returns a Wrapper keyed by a 32-byte AES-256 master key.
// A wrong-length key is an error (callers derive/load exactly 32 bytes).
func NewLocalWrapper(masterKey []byte) (Wrapper, error) {
	if len(masterKey) != 32 {
		return nil, fmt.Errorf("master key must be 32 bytes, got %d", len(masterKey))
	}
	block, err := aes.NewCipher(masterKey)
	if err != nil {
		return nil, fmt.Errorf("creating AES cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("creating GCM: %w", err)
	}
	return &localWrapper{gcm: gcm}, nil
}

// Wrap returns nonce||ciphertext. A fresh random nonce is prepended so the same
// plaintext wraps to distinct ciphertexts.
func (w *localWrapper) Wrap(_ context.Context, plaintext []byte) ([]byte, error) {
	nonce := make([]byte, w.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generating nonce: %w", err)
	}
	return w.gcm.Seal(nonce, nonce, plaintext, nil), nil
}

// Unwrap reverses Wrap, splitting the leading nonce from the ciphertext.
func (w *localWrapper) Unwrap(_ context.Context, wrapped []byte) ([]byte, error) {
	ns := w.gcm.NonceSize()
	if len(wrapped) < ns {
		return nil, fmt.Errorf("wrapped key too short: %d bytes", len(wrapped))
	}
	nonce, ciphertext := wrapped[:ns], wrapped[ns:]
	plain, err := w.gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("unwrapping key: %w", err)
	}
	return plain, nil
}

// stubWrapper is the no-op Wrapper for handlers with no master key.
type stubWrapper struct{}

// NewStubWrapper returns a Wrapper with no backing master key or KMS.
func NewStubWrapper() Wrapper { return stubWrapper{} }

func (stubWrapper) Wrap(context.Context, []byte) ([]byte, error) {
	return nil, ErrNotImplemented
}

func (stubWrapper) Unwrap(context.Context, []byte) ([]byte, error) {
	return nil, ErrNotImplemented
}
