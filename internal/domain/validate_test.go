package domain

import (
	"testing"

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

func TestValidateEvent_ValidWorkCreated(t *testing.T) {
	meta := testMeta()
	e := Event{
		Type:    EventWorkCreated,
		Payload: MustMarshalPayload(WorkCreatedPayload{Title: "Test", Kind: "task"}),
	}
	assert.NoError(t, ValidateEvent(e, meta))
}

func TestValidateEvent_InvalidWorkKind(t *testing.T) {
	meta := testMeta()
	e := Event{
		Type:    EventWorkCreated,
		Payload: MustMarshalPayload(WorkCreatedPayload{Title: "Test", Kind: "epic"}),
	}
	err := ValidateEvent(e, meta)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown work kind")
}

func TestValidateEvent_EmptyKindPasses(t *testing.T) {
	meta := testMeta()
	e := Event{
		Type:    EventWorkCreated,
		Payload: MustMarshalPayload(WorkCreatedPayload{Title: "Test"}),
	}
	assert.NoError(t, ValidateEvent(e, meta))
}

func TestValidateEvent_ValidLabel(t *testing.T) {
	meta := testMeta()
	e := Event{
		Type:    EventWorkLabelAdded,
		Payload: MustMarshalPayload(LabelPayload{LabelSlug: "bug"}),
	}
	assert.NoError(t, ValidateEvent(e, meta))
}

func TestValidateEvent_InvalidLabel(t *testing.T) {
	meta := testMeta()
	e := Event{
		Type:    EventWorkLabelAdded,
		Payload: MustMarshalPayload(LabelPayload{LabelSlug: "nonexistent"}),
	}
	err := ValidateEvent(e, meta)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown label")
}

func TestValidateEvent_ValidStatus(t *testing.T) {
	meta := testMeta()
	e := Event{
		Type:    EventWorkStatusSet,
		Payload: MustMarshalPayload(StatusSetPayload{From: "open", To: "in_progress"}),
	}
	assert.NoError(t, ValidateEvent(e, meta))
}

func TestValidateEvent_InvalidStatus(t *testing.T) {
	meta := testMeta()
	e := Event{
		Type:    EventWorkStatusSet,
		Payload: MustMarshalPayload(StatusSetPayload{From: "open", To: "invalid"}),
	}
	err := ValidateEvent(e, meta)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown status")
}

func TestValidateEvent_InvalidPriority(t *testing.T) {
	meta := testMeta()
	e := Event{
		Type:    EventWorkPrioritySet,
		Payload: MustMarshalPayload(PrioritySetPayload{Priority: "urgent"}),
	}
	err := ValidateEvent(e, meta)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown priority")
}

func TestValidateEvent_ValidArtifactType(t *testing.T) {
	meta := testMeta()
	e := Event{
		Type: EventWorkArtifactAdded,
		Payload: MustMarshalPayload(ArtifactAddedPayload{
			ArtifactID: "art_001", ContentHash: "sha256:abc",
			Filename: "f.txt", SizeBytes: 100, ArtifactType: "log",
		}),
	}
	assert.NoError(t, ValidateEvent(e, meta))
}

func TestValidateEvent_InvalidArtifactType(t *testing.T) {
	meta := testMeta()
	e := Event{
		Type: EventWorkArtifactAdded,
		Payload: MustMarshalPayload(ArtifactAddedPayload{
			ArtifactID: "art_001", ContentHash: "sha256:abc",
			Filename: "f.txt", SizeBytes: 100, ArtifactType: "nonexistent",
		}),
	}
	err := ValidateEvent(e, meta)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown artifact type")
}

func TestValidateEvent_EmptyArtifactTypePasses(t *testing.T) {
	meta := testMeta()
	e := Event{
		Type: EventWorkArtifactAdded,
		Payload: MustMarshalPayload(ArtifactAddedPayload{
			ArtifactID: "art_001", ContentHash: "sha256:abc",
			Filename: "f.txt", SizeBytes: 100,
		}),
	}
	assert.NoError(t, ValidateEvent(e, meta))
}

func TestValidateEvent_ValidRelationType(t *testing.T) {
	meta := testMeta()
	e := Event{
		Type:    EventWorkLinked,
		Payload: MustMarshalPayload(RelationPayload{RelationType: "blocks", TargetWorkItem: "wrk_002"}),
	}
	assert.NoError(t, ValidateEvent(e, meta))
}

func TestValidateEvent_InvalidRelationType(t *testing.T) {
	meta := testMeta()
	e := Event{
		Type:    EventWorkLinked,
		Payload: MustMarshalPayload(RelationPayload{RelationType: "unknown_rel", TargetWorkItem: "wrk_002"}),
	}
	err := ValidateEvent(e, meta)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown relation type")
}

func TestValidateEvent_EvidenceAttachedArtifactType(t *testing.T) {
	meta := testMeta()
	e := Event{
		Type: EventWorkEvidenceAttached,
		Payload: MustMarshalPayload(EvidenceAttachedPayload{
			ArtifactID: "art_001", ContentHash: "sha256:abc",
			Filename: "f.txt", SizeBytes: 100, ArtifactType: "nonexistent", SemanticRole: "evidence",
		}),
	}
	err := ValidateEvent(e, meta)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown artifact type")
}

func TestValidateEvent_NilMeta(t *testing.T) {
	e := Event{
		Type:    EventWorkLabelAdded,
		Payload: MustMarshalPayload(LabelPayload{LabelSlug: "anything"}),
	}
	assert.NoError(t, ValidateEvent(e, nil))
}

func TestValidateEvent_ExecutionStatusFromExecutionWorkflow(t *testing.T) {
	meta := testMeta()
	e := Event{
		Type:    EventWorkStatusSet,
		Payload: MustMarshalPayload(StatusSetPayload{From: "pending", To: "active"}),
	}
	assert.NoError(t, ValidateEvent(e, meta))
}

func TestValidateEvent_ReviewStatusFromReviewWorkflow(t *testing.T) {
	meta := testMeta()
	e := Event{
		Type:    EventWorkStatusSet,
		Payload: MustMarshalPayload(StatusSetPayload{From: "pending_review", To: "approved"}),
	}
	assert.NoError(t, ValidateEvent(e, meta))
}
