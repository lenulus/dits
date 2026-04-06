package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultMetaConfig(t *testing.T) {
	m := DefaultMetaConfig("PROJ")
	assert.Equal(t, MetaVersion(1), m.Version)
	assert.Equal(t, "PROJ", m.ProjectKey)
	assert.Len(t, m.WorkKinds, 8)
	assert.Len(t, m.Workflows, 3)
	assert.Len(t, m.Priorities, 4)
	assert.Len(t, m.Labels, 0)
	assert.Len(t, m.ArtifactTypes, 8)
	assert.Len(t, m.EvidenceTypes, 4)
	assert.Len(t, m.RelationTypes, 9)
	assert.Len(t, m.LeasePolicies, 1)
	assert.Len(t, m.ReviewPolicies, 0)
}

func TestMetaConfig_HasWorkKind(t *testing.T) {
	m := DefaultMetaConfig("TEST")
	assert.True(t, m.HasWorkKind("task"))
	assert.True(t, m.HasWorkKind("execution"))
	assert.True(t, m.HasWorkKind("eval"))
	assert.False(t, m.HasWorkKind("epic"))
}

func TestMetaConfig_HasStatus(t *testing.T) {
	m := DefaultMetaConfig("TEST")
	// Default workflow
	assert.True(t, m.HasStatus("open"))
	assert.True(t, m.HasStatus("in_progress"))
	assert.True(t, m.HasStatus("closed"))
	// Execution workflow
	assert.True(t, m.HasStatus("pending"))
	assert.True(t, m.HasStatus("active"))
	assert.True(t, m.HasStatus("completed"))
	assert.True(t, m.HasStatus("failed"))
	// Review workflow
	assert.True(t, m.HasStatus("pending_review"))
	assert.True(t, m.HasStatus("in_review"))
	assert.True(t, m.HasStatus("approved"))
	assert.True(t, m.HasStatus("changes_requested"))
	assert.True(t, m.HasStatus("rejected"))
	// Invalid
	assert.False(t, m.HasStatus("invalid"))
}

func TestMetaConfig_HasArtifactType(t *testing.T) {
	m := DefaultMetaConfig("TEST")
	assert.True(t, m.HasArtifactType("log"))
	assert.True(t, m.HasArtifactType("patch"))
	assert.True(t, m.HasArtifactType("model_response"))
	assert.False(t, m.HasArtifactType("unknown"))
}

func TestMetaConfig_HasRelationType(t *testing.T) {
	m := DefaultMetaConfig("TEST")
	assert.True(t, m.HasRelationType("blocks"))
	assert.True(t, m.HasRelationType("blocked_by"))
	assert.True(t, m.HasRelationType("supersedes"))
	assert.False(t, m.HasRelationType("unknown"))
}

func TestMetaConfig_GetWorkflowForKind(t *testing.T) {
	m := DefaultMetaConfig("TEST")
	wf := m.GetWorkflowForKind("task")
	require.NotNil(t, wf)
	assert.Equal(t, "default", wf.Slug)

	wf = m.GetWorkflowForKind("execution")
	require.NotNil(t, wf)
	assert.Equal(t, "execution", wf.Slug)

	wf = m.GetWorkflowForKind("eval")
	require.NotNil(t, wf)
	assert.Equal(t, "default", wf.Slug)

	wf = m.GetWorkflowForKind("nonexistent")
	assert.Nil(t, wf)
}

func TestMetaConfig_GetLeasePolicy(t *testing.T) {
	m := DefaultMetaConfig("TEST")
	lp := m.GetLeasePolicy("execution")
	require.NotNil(t, lp)
	assert.Equal(t, 300, lp.DefaultDurationSec)
	assert.Equal(t, 3600, lp.MaxDurationSec)
	assert.Equal(t, 10, lp.MaxRenewals)

	lp = m.GetLeasePolicy("task")
	assert.Nil(t, lp)
}

func TestMetaConfig_AddRemoveLabel(t *testing.T) {
	m := DefaultMetaConfig("TEST")
	require.Len(t, m.Labels, 0)

	err := m.AddLabel(Label{Slug: "bug", Name: "Bug", Color: "#ff0000"})
	require.NoError(t, err)
	assert.Len(t, m.Labels, 1)
	assert.Equal(t, MetaVersion(2), m.Version)

	// Duplicate
	err = m.AddLabel(Label{Slug: "bug", Name: "Bug"})
	assert.Error(t, err)

	err = m.RemoveLabel("bug")
	require.NoError(t, err)
	assert.Len(t, m.Labels, 0)
	assert.Equal(t, MetaVersion(3), m.Version)

	// Remove non-existent
	err = m.RemoveLabel("bug")
	assert.Error(t, err)
}

func TestMetaConfig_AddRemoveWorkKind(t *testing.T) {
	m := DefaultMetaConfig("TEST")
	initialCount := len(m.WorkKinds)

	err := m.AddWorkKind(WorkKind{Slug: "epic", Name: "Epic", WorkflowSlug: "default"})
	require.NoError(t, err)
	assert.Len(t, m.WorkKinds, initialCount+1)
	assert.Equal(t, MetaVersion(2), m.Version)

	// Duplicate
	err = m.AddWorkKind(WorkKind{Slug: "epic", Name: "Epic", WorkflowSlug: "default"})
	assert.Error(t, err)

	// Invalid workflow
	err = m.AddWorkKind(WorkKind{Slug: "custom", Name: "Custom", WorkflowSlug: "nonexistent"})
	assert.Error(t, err)

	err = m.RemoveWorkKind("epic")
	require.NoError(t, err)
	assert.Len(t, m.WorkKinds, initialCount)
	assert.Equal(t, MetaVersion(3), m.Version)

	// Remove non-existent
	err = m.RemoveWorkKind("epic")
	assert.Error(t, err)
}

func TestMetaConfig_AddArtifactType(t *testing.T) {
	m := DefaultMetaConfig("TEST")
	initialCount := len(m.ArtifactTypes)

	err := m.AddArtifactType(ArtifactType{Slug: "diagram", Name: "Diagram"})
	require.NoError(t, err)
	assert.Len(t, m.ArtifactTypes, initialCount+1)
	assert.Equal(t, MetaVersion(2), m.Version)

	// Duplicate
	err = m.AddArtifactType(ArtifactType{Slug: "diagram", Name: "Diagram"})
	assert.Error(t, err)
}

func TestMetaConfig_AddRelationType(t *testing.T) {
	m := DefaultMetaConfig("TEST")
	initialCount := len(m.RelationTypes)

	err := m.AddRelationType(RelationType{Slug: "implements", Name: "Implements", Inverse: "implemented_by"})
	require.NoError(t, err)
	assert.Len(t, m.RelationTypes, initialCount+1)
	assert.Equal(t, MetaVersion(2), m.Version)

	// Duplicate
	err = m.AddRelationType(RelationType{Slug: "implements", Name: "Implements"})
	assert.Error(t, err)
}
