package project_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/lenulus/pf/internal/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInit(t *testing.T) {
	root := t.TempDir()
	proj, err := project.Init(root, "TEST")
	require.NoError(t, err)
	defer proj.DB.Close()

	// .dits directory created
	_, err = os.Stat(filepath.Join(root, ".dits"))
	assert.NoError(t, err)

	// config.json created
	_, err = os.Stat(filepath.Join(root, ".dits", "config.json"))
	assert.NoError(t, err)

	// identity.json created
	_, err = os.Stat(filepath.Join(root, ".dits", "identity.json"))
	assert.NoError(t, err)

	// dits.db created
	_, err = os.Stat(filepath.Join(root, ".dits", "dits.db"))
	assert.NoError(t, err)

	// Config correct
	assert.Equal(t, "TEST", proj.Config.ProjectKey)
	assert.NotEmpty(t, proj.Config.NodeID)
	assert.NotEmpty(t, proj.Config.ActorID)
	assert.Contains(t, string(proj.Config.ActorID), "actor_")

	// Identity present
	require.NotNil(t, proj.Identity)
	assert.NotEmpty(t, proj.Identity.PublicKey)

	// Default meta saved
	meta, err := proj.DB.GetCurrentMeta(context.Background())
	require.NoError(t, err)
	require.NotNil(t, meta)
	assert.Equal(t, "TEST", meta.ProjectKey)
	assert.Len(t, meta.WorkKinds, 9)
}

func TestLoad(t *testing.T) {
	root := t.TempDir()

	// Init first
	proj1, err := project.Init(root, "DEMO")
	require.NoError(t, err)
	proj1.DB.Close()

	// Load
	proj2, err := project.Load(root)
	require.NoError(t, err)
	defer proj2.DB.Close()

	assert.Equal(t, "DEMO", proj2.Config.ProjectKey)
	assert.Equal(t, proj1.Config.NodeID, proj2.Config.NodeID)
	assert.Equal(t, proj1.Config.ActorID, proj2.Config.ActorID)
	require.NotNil(t, proj2.Identity)
}

func TestLoad_NotAProject(t *testing.T) {
	root := t.TempDir()
	_, err := project.Load(root)
	assert.Error(t, err)
}

func TestSaveConfig(t *testing.T) {
	root := t.TempDir()
	proj, err := project.Init(root, "TEST")
	require.NoError(t, err)
	defer proj.DB.Close()

	proj.Config.ServerURL = "http://localhost:9999"
	require.NoError(t, proj.SaveConfig())

	// Reload and verify
	proj2, err := project.Load(root)
	require.NoError(t, err)
	defer proj2.DB.Close()
	assert.Equal(t, "http://localhost:9999", proj2.Config.ServerURL)
}

func TestPrivKey(t *testing.T) {
	root := t.TempDir()
	proj, err := project.Init(root, "TEST")
	require.NoError(t, err)
	defer proj.DB.Close()

	key := proj.PrivKey()
	assert.NotNil(t, key)
	assert.Len(t, key, 64) // Ed25519 private key is 64 bytes
}

func TestPrivKey_NilIdentity(t *testing.T) {
	proj := &project.Project{}
	assert.Nil(t, proj.PrivKey())
}

func TestFindRoot(t *testing.T) {
	root := t.TempDir()
	proj, err := project.Init(root, "TEST")
	require.NoError(t, err)
	proj.DB.Close()

	// Create a subdirectory and find root from there
	sub := filepath.Join(root, "deep", "nested")
	require.NoError(t, os.MkdirAll(sub, 0o755))

	// Change to subdirectory
	orig, _ := os.Getwd()
	defer os.Chdir(orig)
	os.Chdir(sub)

	found, err := project.FindRoot()
	require.NoError(t, err)
	// Resolve symlinks for macOS /var -> /private/var.
	expectedRoot, _ := filepath.EvalSymlinks(root)
	actualFound, _ := filepath.EvalSymlinks(found)
	assert.Equal(t, expectedRoot, actualFound)
}
