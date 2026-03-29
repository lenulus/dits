package project

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/lenulus/pf/internal/domain"
	"github.com/lenulus/pf/internal/store"
	"github.com/lenulus/pf/internal/store/sqlite"
)

const DitsDir = ".dits"

type Config struct {
	ProjectKey string       `json:"project_key"`
	NodeID     domain.NodeID `json:"node_id"`
	ActorID    domain.ActorID `json:"actor_id"`
}

type Project struct {
	Root   string
	Config Config
	DB     store.DB
}

// Init creates a new DITS project in the given directory.
func Init(root, projectKey string) (*Project, error) {
	ditsPath := filepath.Join(root, DitsDir)
	if err := os.MkdirAll(ditsPath, 0o755); err != nil {
		return nil, fmt.Errorf("creating .dits directory: %w", err)
	}

	cfg := Config{
		ProjectKey: projectKey,
		NodeID:     domain.NewNodeID(),
		ActorID:    domain.ActorID(fmt.Sprintf("actor_%s", projectKey)),
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

	return &Project{Root: root, Config: cfg, DB: db}, nil
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

	db, err := sqlite.Open(filepath.Join(ditsPath, "dits.db"))
	if err != nil {
		return nil, err
	}

	return &Project{Root: root, Config: cfg, DB: db}, nil
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
