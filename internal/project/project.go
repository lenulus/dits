package project

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/lenulus/pf/internal/blob"
	"github.com/lenulus/pf/internal/crypto"
	"github.com/lenulus/pf/internal/domain"
	"github.com/lenulus/pf/internal/store"
	"github.com/lenulus/pf/internal/store/sqlite"
)

const DitsDir = ".dits"

type Config struct {
	ProjectKey string         `json:"project_key"`
	NodeID     domain.NodeID  `json:"node_id"`
	ActorID    domain.ActorID `json:"actor_id"`
	ServerURL  string         `json:"server_url,omitempty"`
}

// SaveConfig writes the config back to disk.
func (p *Project) SaveConfig() error {
	ditsPath := filepath.Join(p.Root, DitsDir)
	data, err := json.MarshalIndent(p.Config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(ditsPath, "config.json"), data, 0o644)
}

type Project struct {
	Root     string
	Config   Config
	Identity *crypto.Identity
	DB       store.DB
	Blobs    blob.Store
}

// PrivKey returns the project's signing key, or nil if no identity.
func (p *Project) PrivKey() ed25519.PrivateKey {
	if p.Identity == nil {
		return nil
	}
	k, _ := p.Identity.PrivKey()
	return k
}

// Init creates a new DITS project in the given directory.
func Init(root, projectKey string) (*Project, error) {
	ditsPath := filepath.Join(root, DitsDir)
	if err := os.MkdirAll(ditsPath, 0o755); err != nil {
		return nil, fmt.Errorf("creating .dits directory: %w", err)
	}

	// Generate identity.
	identity, err := crypto.GenerateIdentity()
	if err != nil {
		return nil, fmt.Errorf("generating identity: %w", err)
	}
	if err := crypto.SaveIdentity(ditsPath, identity); err != nil {
		return nil, fmt.Errorf("saving identity: %w", err)
	}

	cfg := Config{
		ProjectKey: projectKey,
		NodeID:     domain.NewNodeID(),
		ActorID:    identity.ActorID,
	}

	cfgData, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(ditsPath, "config.json"), cfgData, 0o644); err != nil {
		return nil, err
	}

	db, err := sqlite.Open(filepath.Join(ditsPath, "dits.db"))
	if err != nil {
		return nil, err
	}

	// Save default meta.
	meta := domain.DefaultMetaConfig(projectKey)
	if err := db.SaveMeta(context.Background(), &meta); err != nil {
		db.Close()
		return nil, err
	}

	blobs, err := blob.NewFSStore(filepath.Join(ditsPath, "blobs"))
	if err != nil {
		db.Close()
		return nil, err
	}

	return &Project{Root: root, Config: cfg, Identity: identity, DB: db, Blobs: blobs}, nil
}

// Load opens an existing DITS project from the given directory.
func Load(root string) (*Project, error) {
	ditsPath := filepath.Join(root, DitsDir)

	cfgData, err := os.ReadFile(filepath.Join(ditsPath, "config.json"))
	if err != nil {
		return nil, fmt.Errorf("reading config: %w (is this a DITS project? run 'dits init')", err)
	}

	var cfg Config
	if err := json.Unmarshal(cfgData, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	// Load identity (optional for backward compat with old projects).
	identity, err := crypto.LoadIdentity(ditsPath)
	if err != nil {
		return nil, fmt.Errorf("loading identity: %w", err)
	}

	db, err := sqlite.Open(filepath.Join(ditsPath, "dits.db"))
	if err != nil {
		return nil, err
	}

	blobs, err := blob.NewFSStore(filepath.Join(ditsPath, "blobs"))
	if err != nil {
		db.Close()
		return nil, err
	}

	return &Project{Root: root, Config: cfg, Identity: identity, DB: db, Blobs: blobs}, nil
}

// FindRoot walks up from the current directory to find a .dits directory.
func FindRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, DitsDir, "config.json")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("not a DITS project (no .dits directory found)")
		}
		dir = parent
	}
}
