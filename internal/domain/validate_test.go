package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testMeta() *MetaConfig {
	m := DefaultMetaConfig("TEST")
	m.Labels = []Label{
		{Slug: "bug", Name: "Bug"},
		{Slug: "feature", Name: "Feature"},
	}
	return &m
}

func TestValidateEvent_ValidIssueCreated(t *testing.T) {
	meta := testMeta()
	e := Event{
		Type:    EventIssueCreated,
		Payload: MustMarshalPayload(IssueCreatedPayload{Title: "Test", TypeSlug: "task"}),
	}
	assert.NoError(t, ValidateEvent(e, meta))
}

func TestValidateEvent_InvalidIssueType(t *testing.T) {
	meta := testMeta()
	e := Event{
		Type:    EventIssueCreated,
		Payload: MustMarshalPayload(IssueCreatedPayload{Title: "Test", TypeSlug: "epic"}),
	}
	err := ValidateEvent(e, meta)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown issue type")
}

func TestValidateEvent_ValidLabel(t *testing.T) {
	meta := testMeta()
	e := Event{
		Type:    EventIssueLabelAdded,
		Payload: MustMarshalPayload(LabelPayload{LabelSlug: "bug"}),
	}
	assert.NoError(t, ValidateEvent(e, meta))
}

func TestValidateEvent_InvalidLabel(t *testing.T) {
	meta := testMeta()
	e := Event{
		Type:    EventIssueLabelAdded,
		Payload: MustMarshalPayload(LabelPayload{LabelSlug: "nonexistent"}),
	}
	err := ValidateEvent(e, meta)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown label")
}

func TestValidateEvent_ValidStatus(t *testing.T) {
	meta := testMeta()
	e := Event{
		Type:    EventIssueStatusSet,
		Payload: MustMarshalPayload(StatusSetPayload{From: "open", To: "in_progress"}),
	}
	assert.NoError(t, ValidateEvent(e, meta))
}

func TestValidateEvent_InvalidStatus(t *testing.T) {
	meta := testMeta()
	e := Event{
		Type:    EventIssueStatusSet,
		Payload: MustMarshalPayload(StatusSetPayload{From: "open", To: "invalid"}),
	}
	err := ValidateEvent(e, meta)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown status")
}

func TestValidateEvent_InvalidPriority(t *testing.T) {
	meta := testMeta()
	e := Event{
		Type:    EventIssuePrioritySet,
		Payload: MustMarshalPayload(PrioritySetPayload{Priority: "urgent"}),
	}
	err := ValidateEvent(e, meta)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown priority")
}

func TestValidateEvent_NilMeta(t *testing.T) {
	e := Event{
		Type:    EventIssueLabelAdded,
		Payload: MustMarshalPayload(LabelPayload{LabelSlug: "anything"}),
	}
	assert.NoError(t, ValidateEvent(e, nil))
}

func TestMetaConfig_AddRemoveLabel(t *testing.T) {
	meta := DefaultMetaConfig("TEST")
	require.Len(t, meta.Labels, 0)

	err := meta.AddLabel(Label{Slug: "bug", Name: "Bug", Color: "#ff0000"})
	require.NoError(t, err)
	assert.Len(t, meta.Labels, 1)
	assert.Equal(t, MetaVersion(2), meta.Version)

	// Duplicate.
	err = meta.AddLabel(Label{Slug: "bug", Name: "Bug"})
	assert.Error(t, err)

	err = meta.RemoveLabel("bug")
	require.NoError(t, err)
	assert.Len(t, meta.Labels, 0)
	assert.Equal(t, MetaVersion(3), meta.Version)

	// Remove non-existent.
	err = meta.RemoveLabel("bug")
	assert.Error(t, err)
}

func TestMetaConfig_HasStatus(t *testing.T) {
	meta := DefaultMetaConfig("TEST")
	assert.True(t, meta.HasStatus("open"))
	assert.True(t, meta.HasStatus("in_progress"))
	assert.True(t, meta.HasStatus("closed"))
	assert.False(t, meta.HasStatus("invalid"))
}

// suppress unused import warning
var _ = time.Now
