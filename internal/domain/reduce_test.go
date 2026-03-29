package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReduce_BasicIssue(t *testing.T) {
	t0 := time.Date(2026, 3, 29, 10, 0, 0, 0, time.UTC)
	events := []Event{
		{
			ID: "evt_001", IssueID: "iss_001", Type: EventIssueCreated,
			ActorID: "actor_alice", Timestamp: t0,
			Payload: MustMarshalPayload(IssueCreatedPayload{Title: "Fix login bug", Body: "Login fails on Safari"}),
		},
	}

	issue, err := Reduce(events)
	require.NoError(t, err)
	assert.Equal(t, CanonicalID("iss_001"), issue.ID)
	assert.Equal(t, "Fix login bug", issue.Title)
	assert.Equal(t, "Login fails on Safari", issue.Body)
	assert.Equal(t, "open", issue.Status)
	assert.Equal(t, "task", issue.TypeSlug)
	assert.Equal(t, "medium", issue.Priority)
	assert.Equal(t, ActorID("actor_alice"), issue.CreatedBy)
	assert.Equal(t, 1, issue.EventCount)
}

func TestReduce_FullLifecycle(t *testing.T) {
	t0 := time.Date(2026, 3, 29, 10, 0, 0, 0, time.UTC)
	events := []Event{
		{
			ID: "evt_001", IssueID: "iss_001", Type: EventIssueCreated,
			ActorID: "actor_alice", Timestamp: t0,
			Payload: MustMarshalPayload(IssueCreatedPayload{Title: "Bug", Body: "Desc"}),
		},
		{
			ID: "evt_002", IssueID: "iss_001", Type: EventIssueTitleSet,
			ParentEventIDs: []EventID{"evt_001"},
			ActorID: "actor_alice", Timestamp: t0.Add(1 * time.Minute),
			Payload: MustMarshalPayload(TitleSetPayload{Title: "Login Bug"}),
		},
		{
			ID: "evt_003", IssueID: "iss_001", Type: EventIssueLabelAdded,
			ParentEventIDs: []EventID{"evt_002"},
			ActorID: "actor_bob", Timestamp: t0.Add(2 * time.Minute),
			Payload: MustMarshalPayload(LabelPayload{LabelSlug: "bug"}),
		},
		{
			ID: "evt_004", IssueID: "iss_001", Type: EventIssueAssigned,
			ParentEventIDs: []EventID{"evt_003"},
			ActorID: "actor_alice", Timestamp: t0.Add(3 * time.Minute),
			Payload: MustMarshalPayload(AssignPayload{Assignee: "actor_bob"}),
		},
		{
			ID: "evt_005", IssueID: "iss_001", Type: EventIssueCommented,
			ParentEventIDs: []EventID{"evt_004"},
			ActorID: "actor_bob", Timestamp: t0.Add(4 * time.Minute),
			Payload: MustMarshalPayload(CommentPayload{Body: "Working on it"}),
		},
		{
			ID: "evt_006", IssueID: "iss_001", Type: EventIssueStatusSet,
			ParentEventIDs: []EventID{"evt_005"},
			ActorID: "actor_bob", Timestamp: t0.Add(5 * time.Minute),
			Payload: MustMarshalPayload(StatusSetPayload{From: "open", To: "in_progress"}),
		},
		{
			ID: "evt_007", IssueID: "iss_001", Type: EventIssueClosed,
			ParentEventIDs: []EventID{"evt_006"},
			ActorID: "actor_bob", Timestamp: t0.Add(10 * time.Minute),
			Payload: MustMarshalPayload(struct{}{}),
		},
	}

	issue, err := Reduce(events)
	require.NoError(t, err)

	assert.Equal(t, "Login Bug", issue.Title)
	assert.Equal(t, "closed", issue.Status)
	assert.Equal(t, []string{"bug"}, issue.Labels)
	assert.Equal(t, []ActorID{"actor_bob"}, issue.Assignees)
	assert.Len(t, issue.Comments, 1)
	assert.Equal(t, "Working on it", issue.Comments[0].Body)
	assert.NotNil(t, issue.ClosedAt)
	assert.Equal(t, 7, issue.EventCount)
}

func TestReduce_Reopen(t *testing.T) {
	t0 := time.Date(2026, 3, 29, 10, 0, 0, 0, time.UTC)
	events := []Event{
		{
			ID: "evt_001", IssueID: "iss_001", Type: EventIssueCreated,
			ActorID: "actor_alice", Timestamp: t0,
			Payload: MustMarshalPayload(IssueCreatedPayload{Title: "Test"}),
		},
		{
			ID: "evt_002", IssueID: "iss_001", Type: EventIssueClosed,
			ParentEventIDs: []EventID{"evt_001"},
			ActorID: "actor_alice", Timestamp: t0.Add(1 * time.Minute),
			Payload: MustMarshalPayload(struct{}{}),
		},
		{
			ID: "evt_003", IssueID: "iss_001", Type: EventIssueReopened,
			ParentEventIDs: []EventID{"evt_002"},
			ActorID: "actor_alice", Timestamp: t0.Add(2 * time.Minute),
			Payload: MustMarshalPayload(struct{}{}),
		},
	}

	issue, err := Reduce(events)
	require.NoError(t, err)
	assert.Equal(t, "open", issue.Status)
	assert.Nil(t, issue.ClosedAt)
}

func TestReduce_LabelAddRemove(t *testing.T) {
	t0 := time.Date(2026, 3, 29, 10, 0, 0, 0, time.UTC)
	events := []Event{
		{
			ID: "evt_001", IssueID: "iss_001", Type: EventIssueCreated,
			ActorID: "actor_alice", Timestamp: t0,
			Payload: MustMarshalPayload(IssueCreatedPayload{Title: "Test"}),
		},
		{
			ID: "evt_002", IssueID: "iss_001", Type: EventIssueLabelAdded,
			ParentEventIDs: []EventID{"evt_001"},
			ActorID: "actor_alice", Timestamp: t0.Add(1 * time.Minute),
			Payload: MustMarshalPayload(LabelPayload{LabelSlug: "bug"}),
		},
		{
			ID: "evt_003", IssueID: "iss_001", Type: EventIssueLabelAdded,
			ParentEventIDs: []EventID{"evt_002"},
			ActorID: "actor_alice", Timestamp: t0.Add(2 * time.Minute),
			Payload: MustMarshalPayload(LabelPayload{LabelSlug: "urgent"}),
		},
		{
			ID: "evt_004", IssueID: "iss_001", Type: EventIssueLabelRemoved,
			ParentEventIDs: []EventID{"evt_003"},
			ActorID: "actor_alice", Timestamp: t0.Add(3 * time.Minute),
			Payload: MustMarshalPayload(LabelPayload{LabelSlug: "bug"}),
		},
	}

	issue, err := Reduce(events)
	require.NoError(t, err)
	assert.Equal(t, []string{"urgent"}, issue.Labels)
}

func TestReduce_DuplicateLabelAdd(t *testing.T) {
	t0 := time.Date(2026, 3, 29, 10, 0, 0, 0, time.UTC)
	events := []Event{
		{
			ID: "evt_001", IssueID: "iss_001", Type: EventIssueCreated,
			ActorID: "actor_alice", Timestamp: t0,
			Payload: MustMarshalPayload(IssueCreatedPayload{Title: "Test"}),
		},
		{
			ID: "evt_002", IssueID: "iss_001", Type: EventIssueLabelAdded,
			ParentEventIDs: []EventID{"evt_001"},
			ActorID: "actor_alice", Timestamp: t0.Add(1 * time.Minute),
			Payload: MustMarshalPayload(LabelPayload{LabelSlug: "bug"}),
		},
		{
			ID: "evt_003", IssueID: "iss_001", Type: EventIssueLabelAdded,
			ParentEventIDs: []EventID{"evt_002"},
			ActorID: "actor_alice", Timestamp: t0.Add(2 * time.Minute),
			Payload: MustMarshalPayload(LabelPayload{LabelSlug: "bug"}),
		},
	}

	issue, err := Reduce(events)
	require.NoError(t, err)
	assert.Equal(t, []string{"bug"}, issue.Labels)
}

func TestReduce_SharedIDAssigned(t *testing.T) {
	t0 := time.Date(2026, 3, 29, 10, 0, 0, 0, time.UTC)
	events := []Event{
		{
			ID: "evt_001", IssueID: "iss_001", Type: EventIssueCreated,
			ActorID: "actor_alice", Timestamp: t0,
			Payload: MustMarshalPayload(IssueCreatedPayload{Title: "Test"}),
		},
		{
			ID: "evt_002", IssueID: "iss_001", Type: EventSharedIDAssigned,
			ParentEventIDs: []EventID{"evt_001"},
			ActorID: "actor_system", Timestamp: t0.Add(1 * time.Minute),
			Payload: MustMarshalPayload(SharedIDAssignedPayload{SharedID: "PROJ-42"}),
		},
	}

	issue, err := Reduce(events)
	require.NoError(t, err)
	assert.Equal(t, SharedID("PROJ-42"), issue.SharedID)
}

func TestReduce_AttachmentAddRemove(t *testing.T) {
	t0 := time.Date(2026, 3, 29, 10, 0, 0, 0, time.UTC)
	events := []Event{
		{
			ID: "evt_001", IssueID: "iss_001", Type: EventIssueCreated,
			ActorID: "actor_alice", Timestamp: t0,
			Payload: MustMarshalPayload(IssueCreatedPayload{Title: "Test"}),
		},
		{
			ID: "evt_002", IssueID: "iss_001", Type: EventAttachmentAdded,
			ParentEventIDs: []EventID{"evt_001"},
			ActorID: "actor_alice", Timestamp: t0.Add(1 * time.Minute),
			Payload: MustMarshalPayload(AttachmentAddedPayload{
				AttachmentID: "att_001",
				ContentHash:  "sha256:abcdef",
				Filename:     "screenshot.png",
				MimeType:     "image/png",
				SizeBytes:    12345,
			}),
		},
		{
			ID: "evt_003", IssueID: "iss_001", Type: EventAttachmentAdded,
			ParentEventIDs: []EventID{"evt_002"},
			ActorID: "actor_bob", Timestamp: t0.Add(2 * time.Minute),
			Payload: MustMarshalPayload(AttachmentAddedPayload{
				AttachmentID: "att_002",
				ContentHash:  "sha256:fedcba",
				Filename:     "logs.txt",
				MimeType:     "text/plain",
				SizeBytes:    5678,
			}),
		},
	}

	issue, err := Reduce(events)
	require.NoError(t, err)
	require.Len(t, issue.Attachments, 2)
	assert.Equal(t, AttachmentID("att_001"), issue.Attachments[0].ID)
	assert.Equal(t, "screenshot.png", issue.Attachments[0].Filename)
	assert.Equal(t, AttachmentID("att_002"), issue.Attachments[1].ID)

	// Remove first attachment.
	events = append(events, Event{
		ID: "evt_004", IssueID: "iss_001", Type: EventAttachmentRemoved,
		ParentEventIDs: []EventID{"evt_003"},
		ActorID: "actor_alice", Timestamp: t0.Add(3 * time.Minute),
		Payload: MustMarshalPayload(AttachmentRemovedPayload{AttachmentID: "att_001"}),
	})

	issue, err = Reduce(events)
	require.NoError(t, err)
	require.Len(t, issue.Attachments, 1)
	assert.Equal(t, AttachmentID("att_002"), issue.Attachments[0].ID)
}

func TestReduce_AttachmentDuplicate(t *testing.T) {
	t0 := time.Date(2026, 3, 29, 10, 0, 0, 0, time.UTC)
	events := []Event{
		{
			ID: "evt_001", IssueID: "iss_001", Type: EventIssueCreated,
			ActorID: "actor_alice", Timestamp: t0,
			Payload: MustMarshalPayload(IssueCreatedPayload{Title: "Test"}),
		},
		{
			ID: "evt_002", IssueID: "iss_001", Type: EventAttachmentAdded,
			ParentEventIDs: []EventID{"evt_001"},
			ActorID: "actor_alice", Timestamp: t0.Add(1 * time.Minute),
			Payload: MustMarshalPayload(AttachmentAddedPayload{
				AttachmentID: "att_001", ContentHash: "sha256:abc", Filename: "f.txt", SizeBytes: 100,
			}),
		},
		{
			ID: "evt_003", IssueID: "iss_001", Type: EventAttachmentAdded,
			ParentEventIDs: []EventID{"evt_002"},
			ActorID: "actor_alice", Timestamp: t0.Add(2 * time.Minute),
			Payload: MustMarshalPayload(AttachmentAddedPayload{
				AttachmentID: "att_001", ContentHash: "sha256:abc", Filename: "f.txt", SizeBytes: 100,
			}),
		},
	}

	issue, err := Reduce(events)
	require.NoError(t, err)
	assert.Len(t, issue.Attachments, 1)
}

func TestReduce_Empty(t *testing.T) {
	_, err := Reduce(nil)
	assert.Error(t, err)
}
