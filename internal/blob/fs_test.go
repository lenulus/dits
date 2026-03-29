package blob

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFSStore_PutGetHasDelete(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFSStore(dir)
	require.NoError(t, err)

	ctx := context.Background()
	data := []byte("hello world")
	hash, err := ComputeHash(bytes.NewReader(data))
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(hash, HashPrefix))

	// Put.
	err = store.Put(ctx, hash, bytes.NewReader(data))
	require.NoError(t, err)

	// Has.
	exists, err := store.Has(ctx, hash)
	require.NoError(t, err)
	assert.True(t, exists)

	// Get.
	rc, err := store.Get(ctx, hash)
	require.NoError(t, err)
	got, err := io.ReadAll(rc)
	rc.Close()
	require.NoError(t, err)
	assert.Equal(t, data, got)

	// Idempotent Put.
	err = store.Put(ctx, hash, bytes.NewReader(data))
	require.NoError(t, err)

	// Delete.
	err = store.Delete(ctx, hash)
	require.NoError(t, err)

	exists, err = store.Has(ctx, hash)
	require.NoError(t, err)
	assert.False(t, exists)

	// Delete non-existent is no-op.
	err = store.Delete(ctx, hash)
	require.NoError(t, err)
}

func TestFSStore_HashMismatch(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFSStore(dir)
	require.NoError(t, err)

	ctx := context.Background()
	err = store.Put(ctx, "sha256:0000000000000000000000000000000000000000000000000000000000000000", bytes.NewReader([]byte("data")))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "hash mismatch")
}

func TestFSStore_NotFound(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFSStore(dir)
	require.NoError(t, err)

	ctx := context.Background()
	_, err = store.Get(ctx, "sha256:nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestComputeFileHash(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	require.NoError(t, os.WriteFile(path, []byte("test content"), 0o644))

	hash, size, err := ComputeFileHash(path)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(hash, HashPrefix))
	assert.Equal(t, int64(12), size)
}
