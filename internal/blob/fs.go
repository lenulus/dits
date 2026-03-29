package blob

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// FSStore is a content-addressed blob store backed by the local filesystem.
// Blobs are stored at: <root>/<first2>/<next2>/<full_hex_hash>
type FSStore struct {
	root string
}

var _ Store = (*FSStore)(nil)

func NewFSStore(root string) (*FSStore, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("creating blob root: %w", err)
	}
	return &FSStore{root: root}, nil
}

func (s *FSStore) blobPath(hash string) string {
	hex := StripPrefix(hash)
	if len(hex) < 4 {
		return filepath.Join(s.root, hex)
	}
	return filepath.Join(s.root, hex[:2], hex[2:4], hex)
}

func (s *FSStore) Put(_ context.Context, hash string, r io.Reader) error {
	path := s.blobPath(hash)

	// Check if already exists (content-addressed = idempotent).
	if _, err := os.Stat(path); err == nil {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	// Write to temp file, verify hash, then atomic rename.
	tmp, err := os.CreateTemp(filepath.Dir(path), ".blob-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() {
		tmp.Close()
		os.Remove(tmpPath) // Clean up on error; no-op if renamed.
	}()

	// Stream through a hash verifier with size limit.
	h := sha256.New()
	limited := io.LimitReader(r, MaxBlobSize+1)
	written, err := io.Copy(io.MultiWriter(tmp, h), limited)
	if err != nil {
		return fmt.Errorf("writing blob: %w", err)
	}
	if written > MaxBlobSize {
		return fmt.Errorf("blob too large: exceeds %d bytes", MaxBlobSize)
	}

	if err := tmp.Close(); err != nil {
		return err
	}

	// Verify hash.
	computed := HashPrefix + hex.EncodeToString(h.Sum(nil))
	if computed != hash {
		return fmt.Errorf("hash mismatch: expected %s, got %s", hash, computed)
	}

	return os.Rename(tmpPath, path)
}

func (s *FSStore) Get(_ context.Context, hash string) (io.ReadCloser, error) {
	f, err := os.Open(s.blobPath(hash))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("blob not found: %s", hash)
		}
		return nil, err
	}
	return f, nil
}

func (s *FSStore) Has(_ context.Context, hash string) (bool, error) {
	_, err := os.Stat(s.blobPath(hash))
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (s *FSStore) Delete(_ context.Context, hash string) error {
	err := os.Remove(s.blobPath(hash))
	if err != nil && os.IsNotExist(err) {
		return nil
	}
	return err
}
