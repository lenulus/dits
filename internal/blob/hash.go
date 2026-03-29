package blob

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
)

// HashPrefix is the prefix for content hashes.
const HashPrefix = "sha256:"

// ComputeHash computes the sha256 hash of the data from the reader.
func ComputeHash(r io.Reader) (string, error) {
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return "", err
	}
	return HashPrefix + hex.EncodeToString(h.Sum(nil)), nil
}

// ComputeFileHash computes the sha256 hash of a file.
func ComputeFileHash(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return "", 0, err
	}

	if info.Size() > MaxBlobSize {
		return "", 0, fmt.Errorf("file too large: %d bytes (max %d)", info.Size(), MaxBlobSize)
	}

	hash, err := ComputeHash(f)
	if err != nil {
		return "", 0, err
	}
	return hash, info.Size(), nil
}

// StripPrefix removes the "sha256:" prefix from a hash string.
func StripPrefix(hash string) string {
	if len(hash) > len(HashPrefix) && hash[:len(HashPrefix)] == HashPrefix {
		return hash[len(HashPrefix):]
	}
	return hash
}
