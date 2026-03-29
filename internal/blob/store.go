package blob

import (
	"context"
	"io"
)

const MaxBlobSize = 50 * 1024 * 1024 // 50MB

// Store is the interface for content-addressed blob storage.
type Store interface {
	Put(ctx context.Context, hash string, r io.Reader) error
	Get(ctx context.Context, hash string) (io.ReadCloser, error)
	Has(ctx context.Context, hash string) (bool, error)
	Delete(ctx context.Context, hash string) error
}
