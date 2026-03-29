package crypto

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/lenulus/pf/internal/domain"
)

// Identity holds an Ed25519 keypair for signing events.
type Identity struct {
	ActorID    domain.ActorID `json:"actor_id"`
	PublicKey  string         `json:"public_key"`  // hex-encoded
	PrivateKey string         `json:"private_key"` // hex-encoded
}

// GenerateIdentity creates a new Ed25519 keypair and derives an ActorID from the public key.
func GenerateIdentity() (*Identity, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generating keypair: %w", err)
	}

	// ActorID derived from first 16 bytes of public key hex.
	actorID := domain.ActorID("actor_" + hex.EncodeToString(pub[:8]))

	return &Identity{
		ActorID:    actorID,
		PublicKey:  hex.EncodeToString(pub),
		PrivateKey: hex.EncodeToString(priv),
	}, nil
}

// PrivKey returns the decoded private key.
func (id *Identity) PrivKey() (ed25519.PrivateKey, error) {
	b, err := hex.DecodeString(id.PrivateKey)
	if err != nil {
		return nil, err
	}
	return ed25519.PrivateKey(b), nil
}

// PubKey returns the decoded public key.
func (id *Identity) PubKey() (ed25519.PublicKey, error) {
	b, err := hex.DecodeString(id.PublicKey)
	if err != nil {
		return nil, err
	}
	return ed25519.PublicKey(b), nil
}

// SaveIdentity writes the identity to a file.
func SaveIdentity(dir string, id *Identity) error {
	data, err := json.MarshalIndent(id, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "identity.json"), data, 0o600)
}

// LoadIdentity reads the identity from a file.
func LoadIdentity(dir string) (*Identity, error) {
	data, err := os.ReadFile(filepath.Join(dir, "identity.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var id Identity
	if err := json.Unmarshal(data, &id); err != nil {
		return nil, err
	}
	return &id, nil
}
