package domain

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReduce_BasicCreation(t *testing.T) {
	t0 := time.Date(2026, 3, 29, 10, 0, 0, 0, time.UTC)
	events := []Event{
		{
			ID: "evt_001", WorkItemID: "wrk_001", Type: EventWorkCreated,
			ActorID: "actor_alice", Timestamp: t0,
			Payload: MustMarshalPayload(WorkCreatedPayload{Title: "Fix login bug", Body: "Login fails on Safari", Kind: "issue"}),
		},
	}

	wi, err := Reduce(events)
	require.NoError(t, err)
	assert.Equal(t, WorkItemID("wrk_001"), wi.ID)
	assert.Equal(t, "Fix login bug", wi.Title)
	assert.Equal(t, "Login fails on Safari", wi.Body)
	assert.Equal(t, "issue", wi.Kind)
	assert.Equal(t, "open", wi.Status)
	assert.Equal(t, "medium", wi.Priority)
	assert.Equal(t, ActorID("actor_alice"), wi.CreatedBy)
	assert.Equal(t, 1, wi.EventCount)
}

func TestReduce_DefaultKind(t *testing.T) {
	t0 := time.Date(2026, 3, 29, 10, 0, 0, 0, time.UTC)
	events := []Event{
		{
			ID: "evt_001", WorkItemID: "wrk_001", Type: EventWorkCreated,
			ActorID: "actor_alice", Timestamp: t0,
			Payload: MustMarshalPayload(WorkCreatedPayload{Title: "Test"}),
		},
	}

	wi, err := Reduce(events)
	require.NoError(t, err)
	assert.Equal(t, "task", wi.Kind)
}

func TestReduce_FullLifecycle(t *testing.T) {
	t0 := time.Date(2026, 3, 29, 10, 0, 0, 0, time.UTC)
	events := []Event{
		{
			ID: "evt_001", WorkItemID: "wrk_001", Type: EventWorkCreated,
			ActorID: "actor_alice", Timestamp: t0,
			Payload: MustMarshalPayload(WorkCreatedPayload{Title: "Bug", Body: "Desc", Kind: "issue"}),
		},
		{
			ID: "evt_002", WorkItemID: "wrk_001", Type: EventWorkTitleSet,
			ParentEventIDs: []EventID{"evt_001"},
			ActorID: "actor_alice", Timestamp: t0.Add(1 * time.Minute),
			Payload: MustMarshalPayload(TitleSetPayload{Title: "Login Bug"}),
		},
		{
			ID: "evt_003", WorkItemID: "wrk_001", Type: EventWorkLabelAdded,
			ParentEventIDs: []EventID{"evt_002"},
			ActorID: "actor_bob", Timestamp: t0.Add(2 * time.Minute),
			Payload: MustMarshalPayload(LabelPayload{LabelSlug: "bug"}),
		},
		{
			ID: "evt_004", WorkItemID: "wrk_001", Type: EventWorkAssigned,
			ParentEventIDs: []EventID{"evt_003"},
			ActorID: "actor_alice", Timestamp: t0.Add(3 * time.Minute),
			Payload: MustMarshalPayload(AssignPayload{Assignee: "actor_bob"}),
		},
		{
			ID: "evt_005", WorkItemID: "wrk_001", Type: EventWorkCommented,
			ParentEventIDs: []EventID{"evt_004"},
			ActorID: "actor_bob", Timestamp: t0.Add(4 * time.Minute),
			Payload: MustMarshalPayload(CommentPayload{Body: "Working on it"}),
		},
		{
			ID: "evt_006", WorkItemID: "wrk_001", Type: EventWorkStatusSet,
			ParentEventIDs: []EventID{"evt_005"},
			ActorID: "actor_bob", Timestamp: t0.Add(5 * time.Minute),
			Payload: MustMarshalPayload(StatusSetPayload{From: "open", To: "in_progress"}),
		},
		{
			ID: "evt_007", WorkItemID: "wrk_001", Type: EventWorkClosed,
			ParentEventIDs: []EventID{"evt_006"},
			ActorID: "actor_bob", Timestamp: t0.Add(10 * time.Minute),
			Payload: MustMarshalPayload(ClosedPayload{Reason: "fixed"}),
		},
	}

	wi, err := Reduce(events)
	require.NoError(t, err)
	assert.Equal(t, "Login Bug", wi.Title)
	assert.Equal(t, "closed", wi.Status)
	assert.Equal(t, []string{"bug"}, wi.Labels)
	assert.Equal(t, []ActorID{"actor_bob"}, wi.Assignees)
	assert.Len(t, wi.Comments, 1)
	assert.Equal(t, "Working on it", wi.Comments[0].Body)
	assert.NotNil(t, wi.ClosedAt)
	assert.Equal(t, 7, wi.EventCount)
}

func TestReduce_Reopen(t *testing.T) {
	t0 := time.Date(2026, 3, 29, 10, 0, 0, 0, time.UTC)
	events := []Event{
		{
			ID: "evt_001", WorkItemID: "wrk_001", Type: EventWorkCreated,
			ActorID: "actor_alice", Timestamp: t0,
			Payload: MustMarshalPayload(WorkCreatedPayload{Title: "Test", Kind: "task"}),
		},
		{
			ID: "evt_002", WorkItemID: "wrk_001", Type: EventWorkClosed,
			ParentEventIDs: []EventID{"evt_001"},
			ActorID: "actor_alice", Timestamp: t0.Add(1 * time.Minute),
			Payload: MustMarshalPayload(ClosedPayload{}),
		},
		{
			ID: "evt_003", WorkItemID: "wrk_001", Type: EventWorkReopened,
			ParentEventIDs: []EventID{"evt_002"},
			ActorID: "actor_alice", Timestamp: t0.Add(2 * time.Minute),
			Payload: MustMarshalPayload(struct{}{}),
		},
	}

	wi, err := Reduce(events)
	require.NoError(t, err)
	assert.Equal(t, "open", wi.Status)
	assert.Nil(t, wi.ClosedAt)
}

func TestReduce_LabelAddRemove(t *testing.T) {
	t0 := time.Date(2026, 3, 29, 10, 0, 0, 0, time.UTC)
	events := []Event{
		{
			ID: "evt_001", WorkItemID: "wrk_001", Type: EventWorkCreated,
			ActorID: "actor_alice", Timestamp: t0,
			Payload: MustMarshalPayload(WorkCreatedPayload{Title: "Test", Kind: "task"}),
		},
		{
			ID: "evt_002", WorkItemID: "wrk_001", Type: EventWorkLabelAdded,
			ParentEventIDs: []EventID{"evt_001"},
			ActorID: "actor_alice", Timestamp: t0.Add(1 * time.Minute),
			Payload: MustMarshalPayload(LabelPayload{LabelSlug: "bug"}),
		},
		{
			ID: "evt_003", WorkItemID: "wrk_001", Type: EventWorkLabelAdded,
			ParentEventIDs: []EventID{"evt_002"},
			ActorID: "actor_alice", Timestamp: t0.Add(2 * time.Minute),
			Payload: MustMarshalPayload(LabelPayload{LabelSlug: "urgent"}),
		},
		// Duplicate add — idempotent
		{
			ID: "evt_004", WorkItemID: "wrk_001", Type: EventWorkLabelAdded,
			ParentEventIDs: []EventID{"evt_003"},
			ActorID: "actor_alice", Timestamp: t0.Add(3 * time.Minute),
			Payload: MustMarshalPayload(LabelPayload{LabelSlug: "bug"}),
		},
		{
			ID: "evt_005", WorkItemID: "wrk_001", Type: EventWorkLabelRemoved,
			ParentEventIDs: []EventID{"evt_004"},
			ActorID: "actor_alice", Timestamp: t0.Add(4 * time.Minute),
			Payload: MustMarshalPayload(LabelPayload{LabelSlug: "bug"}),
		},
	}

	wi, err := Reduce(events)
	require.NoError(t, err)
	assert.Equal(t, []string{"urgent"}, wi.Labels)
}

func TestReduce_LeaseCycle(t *testing.T) {
	t0 := time.Date(2026, 3, 29, 10, 0, 0, 0, time.UTC)
	exp1 := t0.Add(5 * time.Minute)
	exp2 := t0.Add(10 * time.Minute)

	events := []Event{
		{
			ID: "evt_001", WorkItemID: "wrk_001", Type: EventWorkCreated,
			ActorID: "actor_alice", Timestamp: t0,
			Payload: MustMarshalPayload(WorkCreatedPayload{Title: "Task", Kind: "execution"}),
		},
		{
			ID: "evt_002", WorkItemID: "wrk_001", Type: EventWorkLeased,
			ParentEventIDs: []EventID{"evt_001"},
			ActorID: "actor_agent1", Timestamp: t0.Add(1 * time.Minute),
			Payload: MustMarshalPayload(LeasedPayload{
				LeaseID: "lea_001", LeaseDurationSec: 300, LeaseExpiresAt: exp1, Generation: 1,
			}),
		},
		{
			ID: "evt_003", WorkItemID: "wrk_001", Type: EventWorkLeaseRenewed,
			ParentEventIDs: []EventID{"evt_002"},
			ActorID: "actor_agent1", Timestamp: t0.Add(4 * time.Minute),
			Payload: MustMarshalPayload(LeaseRenewedPayload{
				LeaseID: "lea_001", LeaseExpiresAt: exp2, Generation: 2,
			}),
		},
		{
			ID: "evt_004", WorkItemID: "wrk_001", Type: EventWorkLeaseReleased,
			ParentEventIDs: []EventID{"evt_003"},
			ActorID: "actor_agent1", Timestamp: t0.Add(8 * time.Minute),
			Payload: MustMarshalPayload(LeaseReleasedPayload{
				LeaseID: "lea_001", Reason: "done",
			}),
		},
	}

	// After lease
	wi, err := Reduce(events[:2])
	require.NoError(t, err)
	require.NotNil(t, wi.LeaseHolder)
	assert.Equal(t, ActorID("actor_agent1"), *wi.LeaseHolder)
	assert.Equal(t, exp1, *wi.LeaseExpiresAt)

	// After renew
	wi, err = Reduce(events[:3])
	require.NoError(t, err)
	assert.Equal(t, exp2, *wi.LeaseExpiresAt)

	// After release
	wi, err = Reduce(events)
	require.NoError(t, err)
	assert.Nil(t, wi.LeaseHolder)
	assert.Nil(t, wi.LeaseExpiresAt)
}

func TestReduce_ExecutionAttempt(t *testing.T) {
	t0 := time.Date(2026, 3, 29, 10, 0, 0, 0, time.UTC)
	events := []Event{
		{
			ID: "evt_001", WorkItemID: "wrk_001", Type: EventWorkCreated,
			ActorID: "actor_alice", Timestamp: t0,
			Payload: MustMarshalPayload(WorkCreatedPayload{Title: "Run deploy", Kind: "execution"}),
		},
		{
			ID: "evt_002", WorkItemID: "wrk_001", Type: EventWorkExecutionStarted,
			ParentEventIDs: []EventID{"evt_001"},
			ActorID: "actor_agent1", Timestamp: t0.Add(1 * time.Minute),
			Payload: MustMarshalPayload(ExecutionStartedPayload{AttemptID: "atp_001", AttemptNumber: 1}),
		},
		{
			ID: "evt_003", WorkItemID: "wrk_001", Type: EventWorkCheckpointed,
			ParentEventIDs: []EventID{"evt_002"},
			ActorID: "actor_agent1", Timestamp: t0.Add(2 * time.Minute),
			Payload: MustMarshalPayload(CheckpointedPayload{
				AttemptID: "atp_001", Summary: "Step 1 done", Progress: 0.5, NextStep: "Step 2",
			}),
		},
		{
			ID: "evt_004", WorkItemID: "wrk_001", Type: EventWorkCheckpointed,
			ParentEventIDs: []EventID{"evt_003"},
			ActorID: "actor_agent1", Timestamp: t0.Add(3 * time.Minute),
			Payload: MustMarshalPayload(CheckpointedPayload{
				AttemptID: "atp_001", Summary: "Step 2 done", Progress: 1.0,
			}),
		},
		{
			ID: "evt_005", WorkItemID: "wrk_001", Type: EventWorkExecutionCompleted,
			ParentEventIDs: []EventID{"evt_004"},
			ActorID: "actor_agent1", Timestamp: t0.Add(4 * time.Minute),
			Payload: MustMarshalPayload(ExecutionCompletedPayload{AttemptID: "atp_001", Summary: "Deploy succeeded"}),
		},
	}

	wi, err := Reduce(events)
	require.NoError(t, err)

	require.Len(t, wi.Attempts, 1)
	assert.Equal(t, AttemptID("atp_001"), wi.Attempts[0].AttemptID)
	assert.Equal(t, uint32(1), wi.Attempts[0].Number)
	assert.Equal(t, "completed", wi.Attempts[0].Status)
	assert.NotNil(t, wi.Attempts[0].CompletedAt)
	assert.NotNil(t, wi.Attempts[0].LastCheckpoint)
	assert.Equal(t, EventID("evt_004"), *wi.Attempts[0].LastCheckpoint)

	// Attempt is completed, so no running attempt.
	assert.Nil(t, wi.CurrentAttempt, "no running attempt after completion")
	assert.True(t, wi.Attempts[0].Authoritative)

	require.Len(t, wi.Checkpoints, 2)
	assert.Equal(t, 0.5, wi.Checkpoints[0].Progress)
	assert.Equal(t, 1.0, wi.Checkpoints[1].Progress)
}

func TestReduce_FailedAttemptRetry(t *testing.T) {
	t0 := time.Date(2026, 3, 29, 10, 0, 0, 0, time.UTC)
	events := []Event{
		{
			ID: "evt_001", WorkItemID: "wrk_001", Type: EventWorkCreated,
			ActorID: "actor_alice", Timestamp: t0,
			Payload: MustMarshalPayload(WorkCreatedPayload{Title: "Deploy", Kind: "execution"}),
		},
		{
			ID: "evt_002", WorkItemID: "wrk_001", Type: EventWorkExecutionStarted,
			ParentEventIDs: []EventID{"evt_001"},
			ActorID: "actor_agent1", Timestamp: t0.Add(1 * time.Minute),
			Payload: MustMarshalPayload(ExecutionStartedPayload{AttemptID: "atp_001", AttemptNumber: 1}),
		},
		{
			ID: "evt_003", WorkItemID: "wrk_001", Type: EventWorkExecutionFailed,
			ParentEventIDs: []EventID{"evt_002"},
			ActorID: "actor_agent1", Timestamp: t0.Add(2 * time.Minute),
			Payload: MustMarshalPayload(ExecutionFailedPayload{AttemptID: "atp_001", Error: "timeout", Retryable: true}),
		},
		{
			ID: "evt_004", WorkItemID: "wrk_001", Type: EventWorkExecutionStarted,
			ParentEventIDs: []EventID{"evt_003"},
			ActorID: "actor_agent1", Timestamp: t0.Add(3 * time.Minute),
			Payload: MustMarshalPayload(ExecutionStartedPayload{AttemptID: "atp_002", AttemptNumber: 2}),
		},
		{
			ID: "evt_005", WorkItemID: "wrk_001", Type: EventWorkExecutionCompleted,
			ParentEventIDs: []EventID{"evt_004"},
			ActorID: "actor_agent1", Timestamp: t0.Add(4 * time.Minute),
			Payload: MustMarshalPayload(ExecutionCompletedPayload{AttemptID: "atp_002", Summary: "success"}),
		},
	}

	wi, err := Reduce(events)
	require.NoError(t, err)

	require.Len(t, wi.Attempts, 2)
	assert.Equal(t, "failed", wi.Attempts[0].Status)
	assert.Equal(t, "completed", wi.Attempts[1].Status)
	assert.Equal(t, uint32(2), wi.Attempts[1].Number)
	// Both attempts completed/failed — no running attempt.
	assert.Nil(t, wi.CurrentAttempt, "no running attempt after all completed/failed")
}

func TestReduce_BlockedUnblocked(t *testing.T) {
	t0 := time.Date(2026, 3, 29, 10, 0, 0, 0, time.UTC)
	events := []Event{
		{
			ID: "evt_001", WorkItemID: "wrk_001", Type: EventWorkCreated,
			ActorID: "actor_alice", Timestamp: t0,
			Payload: MustMarshalPayload(WorkCreatedPayload{Title: "Task", Kind: "task"}),
		},
		{
			ID: "evt_002", WorkItemID: "wrk_001", Type: EventWorkBlocked,
			ParentEventIDs: []EventID{"evt_001"},
			ActorID: "actor_alice", Timestamp: t0.Add(1 * time.Minute),
			Payload: MustMarshalPayload(BlockedPayload{Reason: "waiting for dependency"}),
		},
	}

	wi, err := Reduce(events)
	require.NoError(t, err)
	assert.True(t, wi.Blocked)
	assert.Equal(t, "waiting for dependency", wi.BlockedReason)

	events = append(events, Event{
		ID: "evt_003", WorkItemID: "wrk_001", Type: EventWorkUnblocked,
		ParentEventIDs: []EventID{"evt_002"},
		ActorID: "actor_alice", Timestamp: t0.Add(2 * time.Minute),
		Payload: MustMarshalPayload(UnblockedPayload{Reason: "dependency resolved"}),
	})

	wi, err = Reduce(events)
	require.NoError(t, err)
	assert.False(t, wi.Blocked)
	assert.Empty(t, wi.BlockedReason)
}

func TestReduce_ObservationFindingRetraction(t *testing.T) {
	t0 := time.Date(2026, 3, 29, 10, 0, 0, 0, time.UTC)
	events := []Event{
		{
			ID: "evt_001", WorkItemID: "wrk_001", Type: EventWorkCreated,
			ActorID: "actor_alice", Timestamp: t0,
			Payload: MustMarshalPayload(WorkCreatedPayload{Title: "Investigation", Kind: "investigation"}),
		},
		{
			ID: "evt_002", WorkItemID: "wrk_001", Type: EventWorkObservationRecorded,
			ParentEventIDs: []EventID{"evt_001"},
			ActorID: "actor_agent1", Timestamp: t0.Add(1 * time.Minute),
			Payload: MustMarshalPayload(ObservationRecordedPayload{
				Summary: "CPU spike at 10:00",
				Data:    json.RawMessage(`{"cpu_pct": 95}`),
			}),
		},
		{
			ID: "evt_003", WorkItemID: "wrk_001", Type: EventWorkFindingRecorded,
			ParentEventIDs: []EventID{"evt_002"},
			ActorID: "actor_agent1", Timestamp: t0.Add(2 * time.Minute),
			Payload: MustMarshalPayload(FindingRecordedPayload{
				Statement:  "Memory leak in service X",
				Confidence: 0.8,
				Source:     "log analysis",
			}),
		},
		{
			ID: "evt_004", WorkItemID: "wrk_001", Type: EventWorkFindingRetracted,
			ParentEventIDs: []EventID{"evt_003"},
			ActorID: "actor_agent1", Timestamp: t0.Add(3 * time.Minute),
			Payload: MustMarshalPayload(FindingRetractedPayload{
				OriginalEventID: "evt_003",
				Reason:          "false positive",
			}),
		},
	}

	wi, err := Reduce(events)
	require.NoError(t, err)

	require.Len(t, wi.Observations, 1)
	assert.Equal(t, "CPU spike at 10:00", wi.Observations[0].Summary)

	require.Len(t, wi.Findings, 1)
	assert.Equal(t, "Memory leak in service X", wi.Findings[0].Statement)
	assert.Equal(t, 0.8, wi.Findings[0].Confidence)
	assert.True(t, wi.Findings[0].Retracted)
}

func TestReduce_ArtifactAddRemove(t *testing.T) {
	t0 := time.Date(2026, 3, 29, 10, 0, 0, 0, time.UTC)
	events := []Event{
		{
			ID: "evt_001", WorkItemID: "wrk_001", Type: EventWorkCreated,
			ActorID: "actor_alice", Timestamp: t0,
			Payload: MustMarshalPayload(WorkCreatedPayload{Title: "Test", Kind: "task"}),
		},
		{
			ID: "evt_002", WorkItemID: "wrk_001", Type: EventWorkArtifactAdded,
			ParentEventIDs: []EventID{"evt_001"},
			ActorID: "actor_alice", Timestamp: t0.Add(1 * time.Minute),
			Payload: MustMarshalPayload(ArtifactAddedPayload{
				ArtifactID: "art_001", ContentHash: "sha256:abcdef",
				Filename: "screenshot.png", MimeType: "image/png", SizeBytes: 12345,
				ArtifactType: "screenshot", SemanticRole: "evidence",
			}),
		},
		{
			ID: "evt_003", WorkItemID: "wrk_001", Type: EventWorkArtifactAdded,
			ParentEventIDs: []EventID{"evt_002"},
			ActorID: "actor_bob", Timestamp: t0.Add(2 * time.Minute),
			Payload: MustMarshalPayload(ArtifactAddedPayload{
				ArtifactID: "art_002", ContentHash: "sha256:fedcba",
				Filename: "logs.txt", MimeType: "text/plain", SizeBytes: 5678,
				ArtifactType: "log",
			}),
		},
		// Duplicate — idempotent
		{
			ID: "evt_004", WorkItemID: "wrk_001", Type: EventWorkArtifactAdded,
			ParentEventIDs: []EventID{"evt_003"},
			ActorID: "actor_alice", Timestamp: t0.Add(3 * time.Minute),
			Payload: MustMarshalPayload(ArtifactAddedPayload{
				ArtifactID: "art_001", ContentHash: "sha256:abcdef",
				Filename: "screenshot.png", MimeType: "image/png", SizeBytes: 12345,
			}),
		},
	}

	wi, err := Reduce(events)
	require.NoError(t, err)
	require.Len(t, wi.Artifacts, 2)
	assert.Equal(t, ArtifactID("art_001"), wi.Artifacts[0].ID)
	assert.Equal(t, "screenshot", wi.Artifacts[0].ArtifactType)
	assert.Equal(t, ArtifactID("art_002"), wi.Artifacts[1].ID)

	// Remove first artifact.
	events = append(events, Event{
		ID: "evt_005", WorkItemID: "wrk_001", Type: EventWorkArtifactRemoved,
		ParentEventIDs: []EventID{"evt_004"},
		ActorID: "actor_alice", Timestamp: t0.Add(4 * time.Minute),
		Payload: MustMarshalPayload(ArtifactRemovedPayload{ArtifactID: "art_001"}),
	})

	wi, err = Reduce(events)
	require.NoError(t, err)
	require.Len(t, wi.Artifacts, 1)
	assert.Equal(t, ArtifactID("art_002"), wi.Artifacts[0].ID)
}

func TestReduce_EvidenceAttached(t *testing.T) {
	t0 := time.Date(2026, 3, 29, 10, 0, 0, 0, time.UTC)
	events := []Event{
		{
			ID: "evt_001", WorkItemID: "wrk_001", Type: EventWorkCreated,
			ActorID: "actor_alice", Timestamp: t0,
			Payload: MustMarshalPayload(WorkCreatedPayload{Title: "Investigation", Kind: "investigation"}),
		},
		{
			ID: "evt_002", WorkItemID: "wrk_001", Type: EventWorkEvidenceAttached,
			ParentEventIDs: []EventID{"evt_001"},
			ActorID: "actor_agent1", Timestamp: t0.Add(1 * time.Minute),
			Payload: MustMarshalPayload(EvidenceAttachedPayload{
				ArtifactID: "art_001", ContentHash: "sha256:abc",
				Filename: "trace.json", MimeType: "application/json", SizeBytes: 9999,
				ArtifactType: "trace", SemanticRole: "evidence",
				ProducedBy: &ProducedBy{ActorID: "actor_agent1", Tool: "profiler"},
			}),
		},
	}

	wi, err := Reduce(events)
	require.NoError(t, err)
	require.Len(t, wi.Artifacts, 1)
	assert.Equal(t, "trace", wi.Artifacts[0].ArtifactType)
	assert.Equal(t, "evidence", wi.Artifacts[0].SemanticRole)
	require.NotNil(t, wi.Artifacts[0].ProducedBy)
	assert.Equal(t, "profiler", wi.Artifacts[0].ProducedBy.Tool)
}

func TestReduce_RelationAddRemove(t *testing.T) {
	t0 := time.Date(2026, 3, 29, 10, 0, 0, 0, time.UTC)
	events := []Event{
		{
			ID: "evt_001", WorkItemID: "wrk_001", Type: EventWorkCreated,
			ActorID: "actor_alice", Timestamp: t0,
			Payload: MustMarshalPayload(WorkCreatedPayload{Title: "Test", Kind: "task"}),
		},
		{
			ID: "evt_002", WorkItemID: "wrk_001", Type: EventWorkLinked,
			ParentEventIDs: []EventID{"evt_001"},
			ActorID: "actor_alice", Timestamp: t0.Add(1 * time.Minute),
			Payload: MustMarshalPayload(RelationPayload{RelationType: "blocks", TargetWorkItem: "wrk_002"}),
		},
		// Duplicate — idempotent
		{
			ID: "evt_003", WorkItemID: "wrk_001", Type: EventWorkLinked,
			ParentEventIDs: []EventID{"evt_002"},
			ActorID: "actor_alice", Timestamp: t0.Add(2 * time.Minute),
			Payload: MustMarshalPayload(RelationPayload{RelationType: "blocks", TargetWorkItem: "wrk_002"}),
		},
	}

	wi, err := Reduce(events)
	require.NoError(t, err)
	require.Len(t, wi.Relations, 1)
	assert.Equal(t, "blocks", wi.Relations[0].Type)
	assert.Equal(t, WorkItemID("wrk_002"), wi.Relations[0].TargetWorkItem)

	events = append(events, Event{
		ID: "evt_004", WorkItemID: "wrk_001", Type: EventWorkUnlinked,
		ParentEventIDs: []EventID{"evt_003"},
		ActorID: "actor_alice", Timestamp: t0.Add(3 * time.Minute),
		Payload: MustMarshalPayload(RelationPayload{RelationType: "blocks", TargetWorkItem: "wrk_002"}),
	})

	wi, err = Reduce(events)
	require.NoError(t, err)
	assert.Empty(t, wi.Relations)
}

func TestReduce_SharedIDAssigned(t *testing.T) {
	t0 := time.Date(2026, 3, 29, 10, 0, 0, 0, time.UTC)
	events := []Event{
		{
			ID: "evt_001", WorkItemID: "wrk_001", Type: EventWorkCreated,
			ActorID: "actor_alice", Timestamp: t0,
			Payload: MustMarshalPayload(WorkCreatedPayload{Title: "Test", Kind: "task"}),
		},
		{
			ID: "evt_002", WorkItemID: "wrk_001", Type: EventWorkSharedIDAssigned,
			ParentEventIDs: []EventID{"evt_001"},
			ActorID: "actor_system", Timestamp: t0.Add(1 * time.Minute),
			Payload: MustMarshalPayload(SharedIDAssignedPayload{SharedID: "PROJ-42"}),
		},
	}

	wi, err := Reduce(events)
	require.NoError(t, err)
	assert.Equal(t, SharedID("PROJ-42"), wi.SharedID)
}

func TestReduce_Empty(t *testing.T) {
	_, err := Reduce(nil)
	assert.Error(t, err)
}

func TestReduce_PlanningEvents(t *testing.T) {
	t0 := time.Date(2026, 3, 29, 10, 0, 0, 0, time.UTC)
	events := []Event{
		{
			ID: "evt_001", WorkItemID: "wrk_001", Type: EventWorkCreated,
			ActorID: "actor_alice", Timestamp: t0,
			Payload: MustMarshalPayload(WorkCreatedPayload{Title: "Plan something", Kind: "plan"}),
		},
		{
			ID: "evt_002", WorkItemID: "wrk_001", Type: EventWorkPlanProposed,
			ParentEventIDs: []EventID{"evt_001"},
			ActorID: "actor_agent1", Timestamp: t0.Add(1 * time.Minute),
			Payload: MustMarshalPayload(PlanProposedPayload{Plan: "Do X then Y", Summary: "Two-step plan"}),
		},
		{
			ID: "evt_003", WorkItemID: "wrk_001", Type: EventWorkPlanAccepted,
			ParentEventIDs: []EventID{"evt_002"},
			ActorID: "actor_alice", Timestamp: t0.Add(2 * time.Minute),
			Payload: MustMarshalPayload(PlanAcceptedPayload{PlanEventID: "evt_002"}),
		},
		{
			ID: "evt_004", WorkItemID: "wrk_001", Type: EventWorkDecisionRecorded,
			ParentEventIDs: []EventID{"evt_003"},
			ActorID: "actor_alice", Timestamp: t0.Add(3 * time.Minute),
			Payload: MustMarshalPayload(DecisionRecordedPayload{Decision: "Go with plan A", Rationale: "Lower risk"}),
		},
	}

	wi, err := Reduce(events)
	require.NoError(t, err)
	assert.Equal(t, t0.Add(3*time.Minute), wi.UpdatedAt)
	assert.Equal(t, 4, wi.EventCount)
}

func TestReduce_HandoffReviewEvents(t *testing.T) {
	t0 := time.Date(2026, 3, 29, 10, 0, 0, 0, time.UTC)
	events := []Event{
		{
			ID: "evt_001", WorkItemID: "wrk_001", Type: EventWorkCreated,
			ActorID: "actor_alice", Timestamp: t0,
			Payload: MustMarshalPayload(WorkCreatedPayload{Title: "Review task", Kind: "artifact_review"}),
		},
		{
			ID: "evt_002", WorkItemID: "wrk_001", Type: EventWorkReviewRequested,
			ParentEventIDs: []EventID{"evt_001"},
			ActorID: "actor_alice", Timestamp: t0.Add(1 * time.Minute),
			Payload: MustMarshalPayload(ReviewRequestedPayload{ReviewID: "rev_001", Scope: "code review"}),
		},
		{
			ID: "evt_003", WorkItemID: "wrk_001", Type: EventWorkReviewCompleted,
			ParentEventIDs: []EventID{"evt_002"},
			ActorID: "actor_bob", Timestamp: t0.Add(2 * time.Minute),
			Payload: MustMarshalPayload(ReviewCompletedPayload{ReviewID: "rev_001", Verdict: "approve"}),
		},
		{
			ID: "evt_004", WorkItemID: "wrk_001", Type: EventWorkHandedOff,
			ParentEventIDs: []EventID{"evt_003"},
			ActorID: "actor_alice", Timestamp: t0.Add(3 * time.Minute),
			Payload: MustMarshalPayload(HandedOffPayload{
				HandoffID: "hof_001", From: "actor_alice", To: "actor_bob", Context: "Please deploy",
			}),
		},
		{
			ID: "evt_005", WorkItemID: "wrk_001", Type: EventWorkHandoffAccepted,
			ParentEventIDs: []EventID{"evt_004"},
			ActorID: "actor_bob", Timestamp: t0.Add(4 * time.Minute),
			Payload: MustMarshalPayload(HandoffAcceptedPayload{HandoffID: "hof_001"}),
		},
	}

	wi, err := Reduce(events)
	require.NoError(t, err)
	assert.Equal(t, t0.Add(4*time.Minute), wi.UpdatedAt)
	assert.Equal(t, 5, wi.EventCount)
}

func TestReduce_ConcurrentAttempts(t *testing.T) {
	t0 := time.Date(2026, 3, 29, 10, 0, 0, 0, time.UTC)
	events := []Event{
		{
			ID: "evt_001", WorkItemID: "wrk_001", Type: EventWorkCreated,
			ActorID: "actor_alice", Timestamp: t0,
			Payload: MustMarshalPayload(WorkCreatedPayload{Title: "Task", Kind: "execution"}),
		},
		{
			ID: "evt_002", WorkItemID: "wrk_001", Type: EventWorkExecutionStarted,
			ParentEventIDs: []EventID{"evt_001"},
			ActorID: "actor_agent1", Timestamp: t0.Add(1 * time.Minute),
			Payload: MustMarshalPayload(ExecutionStartedPayload{AttemptID: "atp_001", AttemptNumber: 1}),
		},
		{
			ID: "evt_003", WorkItemID: "wrk_001", Type: EventWorkExecutionStarted,
			ParentEventIDs: []EventID{"evt_001"},
			ActorID: "actor_agent2", Timestamp: t0.Add(1 * time.Minute),
			Payload: MustMarshalPayload(ExecutionStartedPayload{AttemptID: "atp_002", AttemptNumber: 1}),
		},
	}

	wi, err := Reduce(events)
	require.NoError(t, err)
	// Both attempts stored
	require.Len(t, wi.Attempts, 2)
	assert.Equal(t, AttemptID("atp_001"), wi.Attempts[0].AttemptID)
	assert.Equal(t, AttemptID("atp_002"), wi.Attempts[1].AttemptID)
	// CurrentAttempt is the last one in causal order
	require.NotNil(t, wi.CurrentAttempt)
	assert.Equal(t, AttemptID("atp_002"), *wi.CurrentAttempt)
}

func TestReduce_CommentWithProvenance(t *testing.T) {
	t0 := time.Date(2026, 3, 29, 10, 0, 0, 0, time.UTC)
	events := []Event{
		{
			ID: "evt_001", WorkItemID: "wrk_001", Type: EventWorkCreated,
			ActorID: "actor_alice", Timestamp: t0,
			Payload: MustMarshalPayload(WorkCreatedPayload{Title: "Test", Kind: "task"}),
		},
		{
			ID: "evt_002", WorkItemID: "wrk_001", Type: EventWorkCommented,
			ParentEventIDs: []EventID{"evt_001"},
			ActorID: "actor_agent1", Timestamp: t0.Add(1 * time.Minute),
			Payload: MustMarshalPayload(CommentPayload{
				Body:       "Analysis complete",
				ProducedBy: &ProducedBy{ActorID: "actor_agent1", Model: "claude-3.5-sonnet"},
			}),
		},
	}

	wi, err := Reduce(events)
	require.NoError(t, err)
	require.Len(t, wi.Comments, 1)
	require.NotNil(t, wi.Comments[0].ProducedBy)
	assert.Equal(t, "claude-3.5-sonnet", wi.Comments[0].ProducedBy.Model)
}

func TestReduce_EvalRequestedAndCompleted(t *testing.T) {
	t0 := time.Date(2026, 3, 29, 10, 0, 0, 0, time.UTC)
	events := []Event{
		{
			ID: "evt_001", WorkItemID: "wrk_001", Type: EventWorkCreated,
			ActorID: "actor_alice", Timestamp: t0,
			Payload: MustMarshalPayload(WorkCreatedPayload{Title: "Eval test", Kind: "eval"}),
		},
		{
			ID: "evt_002", WorkItemID: "wrk_001", Type: EventWorkEvalRequested,
			ParentEventIDs: []EventID{"evt_001"},
			ActorID: "actor_agent1", Timestamp: t0.Add(1 * time.Minute),
			Payload: MustMarshalPayload(EvalRequestedPayload{
				EvalID: "evl_001", SubjectRef: "sha256:abc", Scope: "code quality",
			}),
		},
		{
			ID: "evt_003", WorkItemID: "wrk_001", Type: EventWorkEvalCompleted,
			ParentEventIDs: []EventID{"evt_002"},
			ActorID: "actor_agent1", Timestamp: t0.Add(2 * time.Minute),
			Payload: MustMarshalPayload(EvalCompletedPayload{
				EvalID:     "evl_001",
				SubjectRef: "sha256:abc",
				Metrics:    json.RawMessage(`{"score": 0.92, "issues": 2}`),
				Verdict:    "pass",
				ProducedBy: &ProducedBy{ActorID: "actor_agent1", Model: "judge-model"},
			}),
		},
	}

	wi, err := Reduce(events)
	require.NoError(t, err)

	// eval_requested does not materialize
	// eval_completed materializes
	require.Len(t, wi.Evals, 1)
	assert.Equal(t, EvalID("evl_001"), wi.Evals[0].EvalID)
	assert.Equal(t, "sha256:abc", wi.Evals[0].SubjectRef)
	assert.Equal(t, "pass", wi.Evals[0].Verdict)
	assert.JSONEq(t, `{"score": 0.92, "issues": 2}`, string(wi.Evals[0].Metrics))
	require.NotNil(t, wi.Evals[0].ProducedBy)
	assert.Equal(t, "judge-model", wi.Evals[0].ProducedBy.Model)
}

func TestReduce_SplitBrainLeaseLineage(t *testing.T) {
	// Two agents diverge offline and both lease + execute against the same work item.
	// After merge, only the winning lease lineage's attempts are authoritative.
	t0 := time.Date(2026, 3, 29, 10, 0, 0, 0, time.UTC)

	createEvt := Event{
		ID: "evt_001", WorkItemID: "wrk_001", Type: EventWorkCreated,
		ActorID: "actor_alice", Timestamp: t0,
		Payload: MustMarshalPayload(WorkCreatedPayload{Title: "Split brain test", Kind: "execution"}),
	}

	// Branch A: agent1 leases and starts (earlier timestamp)
	leaseA := Event{
		ID: "evt_A1", WorkItemID: "wrk_001", Type: EventWorkLeased,
		ParentEventIDs: []EventID{"evt_001"},
		ActorID: "actor_agent1", Timestamp: t0.Add(1 * time.Minute),
		Payload: MustMarshalPayload(LeasedPayload{
			LeaseID: "lea_A", LeaseDurationSec: 300,
			LeaseExpiresAt: t0.Add(6 * time.Minute), Generation: 1,
		}),
	}
	startA := Event{
		ID: "evt_A2", WorkItemID: "wrk_001", Type: EventWorkExecutionStarted,
		ParentEventIDs: []EventID{"evt_A1"},
		ActorID: "actor_agent1", Timestamp: t0.Add(2 * time.Minute),
		Payload: MustMarshalPayload(ExecutionStartedPayload{AttemptID: "atp_A1", AttemptNumber: 1}),
	}
	cpA := Event{
		ID: "evt_A3", WorkItemID: "wrk_001", Type: EventWorkCheckpointed,
		ParentEventIDs: []EventID{"evt_A2"},
		ActorID: "actor_agent1", Timestamp: t0.Add(3 * time.Minute),
		Payload: MustMarshalPayload(CheckpointedPayload{AttemptID: "atp_A1", Summary: "A progress", Progress: 0.5}),
	}

	// Branch B: agent2 leases and starts (later timestamp — wins in causal order)
	leaseB := Event{
		ID: "evt_B1", WorkItemID: "wrk_001", Type: EventWorkLeased,
		ParentEventIDs: []EventID{"evt_001"},
		ActorID: "actor_agent2", Timestamp: t0.Add(1*time.Minute + 30*time.Second),
		Payload: MustMarshalPayload(LeasedPayload{
			LeaseID: "lea_B", LeaseDurationSec: 300,
			LeaseExpiresAt: t0.Add(6*time.Minute + 30*time.Second), Generation: 1,
		}),
	}
	startB := Event{
		ID: "evt_B2", WorkItemID: "wrk_001", Type: EventWorkExecutionStarted,
		ParentEventIDs: []EventID{"evt_B1"},
		ActorID: "actor_agent2", Timestamp: t0.Add(2*time.Minute + 30*time.Second),
		Payload: MustMarshalPayload(ExecutionStartedPayload{AttemptID: "atp_B1", AttemptNumber: 1}),
	}

	// After sync, all events are in the DAG. Causal order processes them:
	// evt_001 -> (concurrent: evt_A1 @ t+1m, evt_B1 @ t+1m30s)
	// In causal order, A1 comes before B1 (earlier timestamp).
	// Then A2, A3, B2 follow.
	// Last-writer-wins: B1 is later in causal order, so B's lease wins.
	events := CausalOrder([]Event{createEvt, leaseA, startA, cpA, leaseB, startB})

	wi, err := Reduce(events)
	require.NoError(t, err)

	// LeaseHolder should be agent2 (B won)
	require.NotNil(t, wi.LeaseHolder)
	assert.Equal(t, ActorID("actor_agent2"), *wi.LeaseHolder)

	// Both attempts exist
	require.Len(t, wi.Attempts, 2)

	// A's attempt is NOT authoritative (wrong lease holder)
	attemptA := findAttempt(wi.Attempts, "atp_A1")
	require.NotNil(t, attemptA)
	assert.False(t, attemptA.Authoritative, "A's attempt should be non-authoritative (losing lease)")
	assert.Equal(t, uint32(0), attemptA.Number, "non-authoritative attempt number should be 0")

	// B's attempt IS authoritative
	attemptB := findAttempt(wi.Attempts, "atp_B1")
	require.NotNil(t, attemptB)
	assert.True(t, attemptB.Authoritative, "B's attempt should be authoritative (winning lease)")
	assert.Equal(t, uint32(1), attemptB.Number, "authoritative attempt should be numbered 1")

	// CurrentAttempt should be B's (the authoritative running one)
	require.NotNil(t, wi.CurrentAttempt)
	assert.Equal(t, AttemptID("atp_B1"), *wi.CurrentAttempt)

	// Both checkpoints still exist (history preserved)
	assert.Len(t, wi.Checkpoints, 1) // only A had a checkpoint
}

func TestReduce_NoLeaseAllAttemptsAuthoritative(t *testing.T) {
	// When there's no lease conflict, all attempts should be authoritative.
	t0 := time.Date(2026, 3, 29, 10, 0, 0, 0, time.UTC)
	events := []Event{
		{
			ID: "evt_001", WorkItemID: "wrk_001", Type: EventWorkCreated,
			ActorID: "actor_alice", Timestamp: t0,
			Payload: MustMarshalPayload(WorkCreatedPayload{Title: "Normal", Kind: "execution"}),
		},
		{
			ID: "evt_002", WorkItemID: "wrk_001", Type: EventWorkLeased,
			ParentEventIDs: []EventID{"evt_001"},
			ActorID: "actor_agent1", Timestamp: t0.Add(1 * time.Minute),
			Payload: MustMarshalPayload(LeasedPayload{
				LeaseID: "lea_001", LeaseDurationSec: 300,
				LeaseExpiresAt: t0.Add(6 * time.Minute), Generation: 1,
			}),
		},
		{
			ID: "evt_003", WorkItemID: "wrk_001", Type: EventWorkExecutionStarted,
			ParentEventIDs: []EventID{"evt_002"},
			ActorID: "actor_agent1", Timestamp: t0.Add(2 * time.Minute),
			Payload: MustMarshalPayload(ExecutionStartedPayload{AttemptID: "atp_001", AttemptNumber: 1}),
		},
		{
			ID: "evt_004", WorkItemID: "wrk_001", Type: EventWorkExecutionCompleted,
			ParentEventIDs: []EventID{"evt_003"},
			ActorID: "actor_agent1", Timestamp: t0.Add(3 * time.Minute),
			Payload: MustMarshalPayload(ExecutionCompletedPayload{AttemptID: "atp_001", Summary: "done"}),
		},
	}

	wi, err := Reduce(events)
	require.NoError(t, err)

	require.Len(t, wi.Attempts, 1)
	assert.True(t, wi.Attempts[0].Authoritative)
	assert.Equal(t, uint32(1), wi.Attempts[0].Number)
}

func TestReduce_LeaseReleasedAllAttemptsAuthoritative(t *testing.T) {
	// After lease release, no active lease holder — all attempts authoritative.
	t0 := time.Date(2026, 3, 29, 10, 0, 0, 0, time.UTC)
	events := []Event{
		{
			ID: "evt_001", WorkItemID: "wrk_001", Type: EventWorkCreated,
			ActorID: "actor_alice", Timestamp: t0,
			Payload: MustMarshalPayload(WorkCreatedPayload{Title: "Released", Kind: "execution"}),
		},
		{
			ID: "evt_002", WorkItemID: "wrk_001", Type: EventWorkLeased,
			ParentEventIDs: []EventID{"evt_001"},
			ActorID: "actor_agent1", Timestamp: t0.Add(1 * time.Minute),
			Payload: MustMarshalPayload(LeasedPayload{
				LeaseID: "lea_001", LeaseDurationSec: 300,
				LeaseExpiresAt: t0.Add(6 * time.Minute), Generation: 1,
			}),
		},
		{
			ID: "evt_003", WorkItemID: "wrk_001", Type: EventWorkExecutionStarted,
			ParentEventIDs: []EventID{"evt_002"},
			ActorID: "actor_agent1", Timestamp: t0.Add(2 * time.Minute),
			Payload: MustMarshalPayload(ExecutionStartedPayload{AttemptID: "atp_001", AttemptNumber: 1}),
		},
		{
			ID: "evt_004", WorkItemID: "wrk_001", Type: EventWorkLeaseReleased,
			ParentEventIDs: []EventID{"evt_003"},
			ActorID: "actor_agent1", Timestamp: t0.Add(3 * time.Minute),
			Payload: MustMarshalPayload(LeaseReleasedPayload{LeaseID: "lea_001", Reason: "done"}),
		},
	}

	wi, err := Reduce(events)
	require.NoError(t, err)

	assert.Nil(t, wi.LeaseHolder)
	require.Len(t, wi.Attempts, 1)
	assert.True(t, wi.Attempts[0].Authoritative, "no active lease = all authoritative")
}
