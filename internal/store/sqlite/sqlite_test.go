package sqlite_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/lenulus/pf/internal/domain"
	"github.com/lenulus/pf/internal/store"
	"github.com/lenulus/pf/internal/store/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func openTestDB(t *testing.T) *sqlite.Store {
	t.Helper()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db
}

func makeEvent(wiID domain.WorkItemID, evtType domain.EventType, parents []domain.EventID, actor domain.ActorID, ts time.Time, payload any) domain.Event {
	return domain.Event{
		ID:             domain.NewEventID(),
		WorkItemID:     wiID,
		Type:           evtType,
		ParentEventIDs: parents,
		MetaVersion:    1,
		ActorID:        actor,
		Timestamp:      ts,
		Payload:        domain.MustMarshalPayload(payload),
	}
}

// --- Event operations ---

func TestAppendEvents_GetEvent(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	t0 := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)

	evt := makeEvent("wrk_001", domain.EventWorkCreated, nil, "actor_a", t0,
		domain.WorkCreatedPayload{Title: "Test", Kind: "task"})

	require.NoError(t, db.AppendEvents(ctx, []domain.Event{evt}))

	got, err := db.GetEvent(ctx, evt.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, evt.ID, got.ID)
	assert.Equal(t, domain.WorkItemID("wrk_001"), got.WorkItemID)
	assert.Equal(t, domain.EventWorkCreated, got.Type)
}

func TestAppendEvents_Idempotent(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	t0 := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)

	evt := makeEvent("wrk_001", domain.EventWorkCreated, nil, "actor_a", t0,
		domain.WorkCreatedPayload{Title: "Test", Kind: "task"})

	require.NoError(t, db.AppendEvents(ctx, []domain.Event{evt}))
	require.NoError(t, db.AppendEvents(ctx, []domain.Event{evt})) // no error on duplicate
}

func TestGetEventsForWorkItem_Ordering(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	t0 := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)

	evt1 := makeEvent("wrk_001", domain.EventWorkCreated, nil, "actor_a", t0,
		domain.WorkCreatedPayload{Title: "Test", Kind: "task"})
	evt2 := makeEvent("wrk_001", domain.EventWorkCommented, []domain.EventID{evt1.ID}, "actor_a", t0.Add(time.Minute),
		domain.CommentPayload{Body: "hello"})

	require.NoError(t, db.AppendEvents(ctx, []domain.Event{evt1, evt2}))

	events, err := db.GetEventsForWorkItem(ctx, "wrk_001")
	require.NoError(t, err)
	require.Len(t, events, 2)
	assert.Equal(t, evt1.ID, events[0].ID)
	assert.Equal(t, evt2.ID, events[1].ID)
	assert.Equal(t, []domain.EventID{evt1.ID}, events[1].ParentEventIDs)
}

func TestGetHeads_LinearChain(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	t0 := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)

	evt1 := makeEvent("wrk_001", domain.EventWorkCreated, nil, "actor_a", t0,
		domain.WorkCreatedPayload{Title: "Test", Kind: "task"})
	evt2 := makeEvent("wrk_001", domain.EventWorkCommented, []domain.EventID{evt1.ID}, "actor_a", t0.Add(time.Minute),
		domain.CommentPayload{Body: "hello"})

	require.NoError(t, db.AppendEvents(ctx, []domain.Event{evt1}))
	require.NoError(t, db.AppendEvents(ctx, []domain.Event{evt2}))

	heads, err := db.GetHeads(ctx, "wrk_001")
	require.NoError(t, err)
	assert.Equal(t, []domain.EventID{evt2.ID}, heads)
}

func TestGetHeads_DiamondMerge(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	t0 := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)

	evt1 := makeEvent("wrk_001", domain.EventWorkCreated, nil, "actor_a", t0,
		domain.WorkCreatedPayload{Title: "Test", Kind: "task"})
	evt2 := makeEvent("wrk_001", domain.EventWorkCommented, []domain.EventID{evt1.ID}, "actor_a", t0.Add(time.Minute),
		domain.CommentPayload{Body: "branch A"})
	evt3 := makeEvent("wrk_001", domain.EventWorkCommented, []domain.EventID{evt1.ID}, "actor_b", t0.Add(2*time.Minute),
		domain.CommentPayload{Body: "branch B"})

	require.NoError(t, db.AppendEvents(ctx, []domain.Event{evt1, evt2, evt3}))

	heads, err := db.GetHeads(ctx, "wrk_001")
	require.NoError(t, err)
	assert.ElementsMatch(t, []domain.EventID{evt2.ID, evt3.ID}, heads)

	// Merge
	evt4 := makeEvent("wrk_001", domain.EventWorkCommented, []domain.EventID{evt2.ID, evt3.ID}, "actor_a", t0.Add(3*time.Minute),
		domain.CommentPayload{Body: "merged"})
	require.NoError(t, db.AppendEvents(ctx, []domain.Event{evt4}))

	heads, err = db.GetHeads(ctx, "wrk_001")
	require.NoError(t, err)
	assert.Equal(t, []domain.EventID{evt4.ID}, heads)
}

func TestGetAllHeads(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	t0 := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)

	evt1 := makeEvent("wrk_001", domain.EventWorkCreated, nil, "actor_a", t0,
		domain.WorkCreatedPayload{Title: "One", Kind: "task"})
	evt2 := makeEvent("wrk_002", domain.EventWorkCreated, nil, "actor_a", t0.Add(time.Minute),
		domain.WorkCreatedPayload{Title: "Two", Kind: "task"})

	require.NoError(t, db.AppendEvents(ctx, []domain.Event{evt1, evt2}))

	heads, err := db.GetAllHeads(ctx)
	require.NoError(t, err)
	assert.Len(t, heads, 2)
}

func TestHasEvent(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	t0 := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)

	evt := makeEvent("wrk_001", domain.EventWorkCreated, nil, "actor_a", t0,
		domain.WorkCreatedPayload{Title: "Test", Kind: "task"})

	has, err := db.HasEvent(ctx, evt.ID)
	require.NoError(t, err)
	assert.False(t, has)

	require.NoError(t, db.AppendEvents(ctx, []domain.Event{evt}))

	has, err = db.HasEvent(ctx, evt.ID)
	require.NoError(t, err)
	assert.True(t, has)
}

func TestGetAffectedWorkItemIDs(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	t0 := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)

	events := []domain.Event{
		makeEvent("wrk_001", domain.EventWorkCreated, nil, "actor_a", t0, domain.WorkCreatedPayload{Title: "A", Kind: "task"}),
		makeEvent("wrk_002", domain.EventWorkCreated, nil, "actor_a", t0, domain.WorkCreatedPayload{Title: "B", Kind: "task"}),
		makeEvent("wrk_001", domain.EventWorkCommented, nil, "actor_a", t0, domain.CommentPayload{Body: "c"}),
	}

	ids, err := db.GetAffectedWorkItemIDs(ctx, events)
	require.NoError(t, err)
	assert.Len(t, ids, 2) // wrk_001 deduped
}

// --- WorkItem operations ---

func TestUpsertWorkItem_GetWorkItem(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	t0 := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)

	leaseHolder := domain.ActorID("actor_agent")
	leaseExpires := t0.Add(5 * time.Minute)
	currentAttempt := domain.AttemptID("atp_001")
	closedAt := t0.Add(10 * time.Minute)

	wi := &domain.WorkItem{
		ID: "wrk_001", SharedID: "TEST-1", Kind: "execution",
		Title: "Deploy", Body: "Deploy v2", Status: "active", Priority: "high",
		LeaseHolder: &leaseHolder, LeaseExpiresAt: &leaseExpires,
		CurrentAttempt: &currentAttempt, Blocked: true, BlockedReason: "waiting",
		CreatedBy: "actor_a", CreatedAt: t0, UpdatedAt: t0.Add(time.Minute),
		ClosedAt:    &closedAt,
		Labels:      []string{"urgent"},
		Assignees:   []domain.ActorID{"actor_a", "actor_b"},
		Comments:    []domain.Comment{{EventID: "evt_c1", ActorID: "actor_a", Body: "hello", Timestamp: t0}},
		Artifacts:   []domain.Artifact{{ID: "art_001", ContentHash: "sha256:abc", Filename: "f.txt", MimeType: "text/plain", SizeBytes: 100, ArtifactType: "log", SemanticRole: "evidence", AddedBy: "actor_a", AddedAt: t0}},
		Relations:   []domain.Relation{{Type: "blocks", TargetWorkItem: "wrk_002"}},
		Checkpoints: []domain.Checkpoint{{EventID: "evt_cp1", AttemptID: "atp_001", Summary: "step 1", Progress: 0.5, Timestamp: t0}},
		Observations: []domain.Observation{{EventID: "evt_o1", Summary: "CPU spike", Timestamp: t0}},
		Findings:    []domain.Finding{{EventID: "evt_f1", Statement: "leak", Confidence: 0.8, Retracted: false, Timestamp: t0}},
		Attempts:    []domain.ExecutionAttempt{{AttemptID: "atp_001", Number: 1, ActorID: "actor_a", StartedAt: t0, Status: "running"}},
		EventCount:  5,
	}

	require.NoError(t, db.UpsertWorkItem(ctx, wi))

	got, err := db.GetWorkItem(ctx, "wrk_001")
	require.NoError(t, err)
	require.NotNil(t, got)

	assert.Equal(t, domain.WorkItemID("wrk_001"), got.ID)
	assert.Equal(t, domain.SharedID("TEST-1"), got.SharedID)
	assert.Equal(t, "execution", got.Kind)
	assert.Equal(t, "Deploy", got.Title)
	assert.Equal(t, "active", got.Status)
	assert.Equal(t, "high", got.Priority)
	assert.True(t, got.Blocked)
	assert.Equal(t, "waiting", got.BlockedReason)
	require.NotNil(t, got.LeaseHolder)
	assert.Equal(t, domain.ActorID("actor_agent"), *got.LeaseHolder)
	require.NotNil(t, got.CurrentAttempt)
	assert.Equal(t, domain.AttemptID("atp_001"), *got.CurrentAttempt)
	assert.NotNil(t, got.ClosedAt)
	assert.Equal(t, 5, got.EventCount)

	// Collections
	assert.Equal(t, []string{"urgent"}, got.Labels)
	assert.Len(t, got.Assignees, 2)
	assert.Len(t, got.Comments, 1)
	assert.Len(t, got.Artifacts, 1)
	assert.Equal(t, "log", got.Artifacts[0].ArtifactType)
	assert.Equal(t, "evidence", got.Artifacts[0].SemanticRole)
	assert.Len(t, got.Relations, 1)
	assert.Len(t, got.Checkpoints, 1)
	assert.Equal(t, 0.5, got.Checkpoints[0].Progress)
	assert.Len(t, got.Observations, 1)
	assert.Len(t, got.Findings, 1)
	assert.Equal(t, 0.8, got.Findings[0].Confidence)
	assert.Len(t, got.Attempts, 1)
	assert.Equal(t, "running", got.Attempts[0].Status)
}

func TestGetWorkItemBySharedID(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	t0 := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)

	wi := &domain.WorkItem{
		ID: "wrk_001", SharedID: "TEST-1", Kind: "task", Title: "T", Status: "open",
		Priority: "medium", CreatedBy: "actor_a", CreatedAt: t0, UpdatedAt: t0,
		Labels: []string{}, Assignees: []domain.ActorID{}, Comments: []domain.Comment{},
		Artifacts: []domain.Artifact{}, Relations: []domain.Relation{},
		Checkpoints: []domain.Checkpoint{}, Observations: []domain.Observation{},
		Findings: []domain.Finding{}, Attempts: []domain.ExecutionAttempt{},
	}
	require.NoError(t, db.UpsertWorkItem(ctx, wi))

	got, err := db.GetWorkItemBySharedID(ctx, "TEST-1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "T", got.Title)

	got, err = db.GetWorkItemBySharedID(ctx, "TEST-999")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestListWorkItems_Filters(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	t0 := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)

	base := domain.WorkItem{
		Priority: "medium", CreatedBy: "actor_a", CreatedAt: t0, UpdatedAt: t0,
		Labels: []string{}, Assignees: []domain.ActorID{}, Comments: []domain.Comment{},
		Artifacts: []domain.Artifact{}, Relations: []domain.Relation{},
		Checkpoints: []domain.Checkpoint{}, Observations: []domain.Observation{},
		Findings: []domain.Finding{}, Attempts: []domain.ExecutionAttempt{},
	}

	w1 := base
	w1.ID = "wrk_001"
	w1.Kind = "task"
	w1.Title = "Open task"
	w1.Status = "open"
	w1.Labels = []string{"bug"}
	require.NoError(t, db.UpsertWorkItem(ctx, &w1))

	w2 := base
	w2.ID = "wrk_002"
	w2.Kind = "issue"
	w2.Title = "Closed issue"
	w2.Status = "closed"
	require.NoError(t, db.UpsertWorkItem(ctx, &w2))

	leaseHolder := domain.ActorID("actor_b")
	w3 := base
	w3.ID = "wrk_003"
	w3.Kind = "execution"
	w3.Title = "Leased execution"
	w3.Status = "active"
	w3.LeaseHolder = &leaseHolder
	require.NoError(t, db.UpsertWorkItem(ctx, &w3))

	w4 := base
	w4.ID = "wrk_004"
	w4.Kind = "task"
	w4.Title = "Blocked task"
	w4.Status = "open"
	w4.Blocked = true
	w4.BlockedReason = "dep"
	require.NoError(t, db.UpsertWorkItem(ctx, &w4))

	// Status filter
	items, err := db.ListWorkItems(ctx, store.WorkItemFilter{Status: "open"})
	require.NoError(t, err)
	assert.Len(t, items, 2) // wrk_001, wrk_004

	// Kind filter
	items, err = db.ListWorkItems(ctx, store.WorkItemFilter{Kind: "execution"})
	require.NoError(t, err)
	assert.Len(t, items, 1)

	// Label filter
	items, err = db.ListWorkItems(ctx, store.WorkItemFilter{Label: "bug"})
	require.NoError(t, err)
	assert.Len(t, items, 1)

	// ClaimedBy filter
	items, err = db.ListWorkItems(ctx, store.WorkItemFilter{ClaimedBy: "actor_b"})
	require.NoError(t, err)
	assert.Len(t, items, 1)
	assert.Equal(t, domain.WorkItemID("wrk_003"), items[0].ID)

	// Blocked filter
	blocked := true
	items, err = db.ListWorkItems(ctx, store.WorkItemFilter{Blocked: &blocked})
	require.NoError(t, err)
	assert.Len(t, items, 1)
	assert.Equal(t, domain.WorkItemID("wrk_004"), items[0].ID)

	// Multi-status (Statuses)
	items, err = db.ListWorkItems(ctx, store.WorkItemFilter{Statuses: []string{"open", "active"}})
	require.NoError(t, err)
	assert.Len(t, items, 3) // wrk_001, wrk_003, wrk_004

	// Ready = open status + no lease + not blocked
	ready := true
	notBlocked := false
	items, err = db.ListWorkItems(ctx, store.WorkItemFilter{
		Statuses: []string{"open"}, Ready: &ready, Blocked: &notBlocked,
	})
	require.NoError(t, err)
	assert.Len(t, items, 1)
	assert.Equal(t, domain.WorkItemID("wrk_001"), items[0].ID)

	// Query (LIKE)
	items, err = db.ListWorkItems(ctx, store.WorkItemFilter{Query: "Blocked"})
	require.NoError(t, err)
	assert.Len(t, items, 1)

	// Pagination
	items, err = db.ListWorkItems(ctx, store.WorkItemFilter{Limit: 2})
	require.NoError(t, err)
	assert.Len(t, items, 2)

	items, err = db.ListWorkItems(ctx, store.WorkItemFilter{Limit: 2, Offset: 2})
	require.NoError(t, err)
	assert.Len(t, items, 2)
}

func TestAllocateSharedID(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	t0 := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)

	wi := &domain.WorkItem{
		ID: "wrk_001", Kind: "task", Title: "T", Status: "open",
		Priority: "medium", CreatedBy: "actor_a", CreatedAt: t0, UpdatedAt: t0,
		Labels: []string{}, Assignees: []domain.ActorID{}, Comments: []domain.Comment{},
		Artifacts: []domain.Artifact{}, Relations: []domain.Relation{},
		Checkpoints: []domain.Checkpoint{}, Observations: []domain.Observation{},
		Findings: []domain.Finding{}, Attempts: []domain.ExecutionAttempt{},
	}
	require.NoError(t, db.UpsertWorkItem(ctx, wi))

	sid1, err := db.AllocateSharedID(ctx, "wrk_001", "PROJ")
	require.NoError(t, err)
	assert.Equal(t, domain.SharedID("PROJ-1"), sid1)

	wi2 := *wi
	wi2.ID = "wrk_002"
	require.NoError(t, db.UpsertWorkItem(ctx, &wi2))

	sid2, err := db.AllocateSharedID(ctx, "wrk_002", "PROJ")
	require.NoError(t, err)
	assert.Equal(t, domain.SharedID("PROJ-2"), sid2)
}

// --- WorkItem collections with provenance ---

func TestWorkItem_CommentsWithProducedBy(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	t0 := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)

	wi := &domain.WorkItem{
		ID: "wrk_001", Kind: "task", Title: "T", Status: "open",
		Priority: "medium", CreatedBy: "actor_a", CreatedAt: t0, UpdatedAt: t0,
		Labels: []string{}, Assignees: []domain.ActorID{}, Artifacts: []domain.Artifact{},
		Relations: []domain.Relation{}, Checkpoints: []domain.Checkpoint{},
		Observations: []domain.Observation{}, Findings: []domain.Finding{}, Attempts: []domain.ExecutionAttempt{},
		Comments: []domain.Comment{
			{EventID: "evt_c1", ActorID: "actor_agent", Body: "analysis",
				ProducedBy: &domain.ProducedBy{ActorID: "actor_agent", Model: "claude-3.5-sonnet"},
				Timestamp: t0},
		},
	}
	require.NoError(t, db.UpsertWorkItem(ctx, wi))

	got, _ := db.GetWorkItem(ctx, "wrk_001")
	require.Len(t, got.Comments, 1)
	require.NotNil(t, got.Comments[0].ProducedBy)
	assert.Equal(t, "claude-3.5-sonnet", got.Comments[0].ProducedBy.Model)
}

func TestWorkItem_FindingsWithEvidenceRefs(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	t0 := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)

	wi := &domain.WorkItem{
		ID: "wrk_001", Kind: "investigation", Title: "T", Status: "open",
		Priority: "medium", CreatedBy: "actor_a", CreatedAt: t0, UpdatedAt: t0,
		Labels: []string{}, Assignees: []domain.ActorID{}, Comments: []domain.Comment{},
		Artifacts: []domain.Artifact{}, Relations: []domain.Relation{},
		Checkpoints: []domain.Checkpoint{}, Observations: []domain.Observation{},
		Attempts: []domain.ExecutionAttempt{},
		Findings: []domain.Finding{
			{EventID: "evt_f1", Statement: "leak found", Confidence: 0.9,
				EvidenceRefs: []string{"sha256:abc", "sha256:def"},
				Retracted: true, Timestamp: t0},
		},
	}
	require.NoError(t, db.UpsertWorkItem(ctx, wi))

	got, _ := db.GetWorkItem(ctx, "wrk_001")
	require.Len(t, got.Findings, 1)
	assert.Equal(t, 0.9, got.Findings[0].Confidence)
	assert.Equal(t, []string{"sha256:abc", "sha256:def"}, got.Findings[0].EvidenceRefs)
	assert.True(t, got.Findings[0].Retracted)
}

func TestWorkItem_AttemptsWithCompletion(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	t0 := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)
	completedAt := t0.Add(5 * time.Minute)
	lastCP := domain.EventID("evt_cp2")

	wi := &domain.WorkItem{
		ID: "wrk_001", Kind: "execution", Title: "T", Status: "open",
		Priority: "medium", CreatedBy: "actor_a", CreatedAt: t0, UpdatedAt: t0,
		Labels: []string{}, Assignees: []domain.ActorID{}, Comments: []domain.Comment{},
		Artifacts: []domain.Artifact{}, Relations: []domain.Relation{},
		Checkpoints: []domain.Checkpoint{}, Observations: []domain.Observation{}, Findings: []domain.Finding{},
		Attempts: []domain.ExecutionAttempt{
			{AttemptID: "atp_001", Number: 1, ActorID: "actor_a", StartedAt: t0,
				CompletedAt: &completedAt, Status: "completed", LastCheckpoint: &lastCP},
		},
	}
	require.NoError(t, db.UpsertWorkItem(ctx, wi))

	got, _ := db.GetWorkItem(ctx, "wrk_001")
	require.Len(t, got.Attempts, 1)
	assert.Equal(t, "completed", got.Attempts[0].Status)
	require.NotNil(t, got.Attempts[0].CompletedAt)
	require.NotNil(t, got.Attempts[0].LastCheckpoint)
	assert.Equal(t, domain.EventID("evt_cp2"), *got.Attempts[0].LastCheckpoint)
}

// --- Meta operations ---

func TestMeta_SaveAndGet(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	meta := domain.DefaultMetaConfig("TEST")
	require.NoError(t, db.SaveMeta(ctx, &meta))

	got, err := db.GetCurrentMeta(ctx)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "TEST", got.ProjectKey)
	assert.Equal(t, domain.MetaVersion(1), got.Version)
	assert.Len(t, got.WorkKinds, 9)
}

func TestMeta_SaveIfVersion_Success(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	meta := domain.DefaultMetaConfig("TEST")
	require.NoError(t, db.SaveMeta(ctx, &meta))

	meta.Version = 2
	require.NoError(t, db.SaveMetaIfVersion(ctx, &meta, 1))

	got, _ := db.GetCurrentMeta(ctx)
	assert.Equal(t, domain.MetaVersion(2), got.Version)
}

func TestMeta_SaveIfVersion_Conflict(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	meta := domain.DefaultMetaConfig("TEST")
	require.NoError(t, db.SaveMeta(ctx, &meta))

	meta.Version = 2
	err := db.SaveMetaIfVersion(ctx, &meta, 99) // wrong expected version
	assert.Equal(t, store.ErrMetaConflict, err)
}

// --- ListEvents ---

func TestListEvents_Filters(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	t0 := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)

	evt1 := makeEvent("wrk_001", domain.EventWorkCreated, nil, "actor_a", t0,
		domain.WorkCreatedPayload{Title: "A", Kind: "task"})
	evt2 := makeEvent("wrk_001", domain.EventWorkCommented, []domain.EventID{evt1.ID}, "actor_b", t0.Add(time.Minute),
		domain.CommentPayload{Body: "hi"})
	evt3 := makeEvent("wrk_002", domain.EventWorkCreated, nil, "actor_a", t0.Add(2*time.Minute),
		domain.WorkCreatedPayload{Title: "B", Kind: "issue"})

	require.NoError(t, db.AppendEvents(ctx, []domain.Event{evt1, evt2, evt3}))

	// All events
	events, err := db.ListEvents(ctx, store.EventFilter{})
	require.NoError(t, err)
	assert.Len(t, events, 3)

	// By work item
	events, err = db.ListEvents(ctx, store.EventFilter{WorkItemID: "wrk_001"})
	require.NoError(t, err)
	assert.Len(t, events, 2)

	// By type
	events, err = db.ListEvents(ctx, store.EventFilter{Type: domain.EventWorkCreated})
	require.NoError(t, err)
	assert.Len(t, events, 2)

	// By actor
	events, err = db.ListEvents(ctx, store.EventFilter{ActorID: "actor_b"})
	require.NoError(t, err)
	assert.Len(t, events, 1)

	// Since
	events, err = db.ListEvents(ctx, store.EventFilter{Since: t0.Add(30 * time.Second)})
	require.NoError(t, err)
	assert.Len(t, events, 2)
}

func TestGetArtifactsByHash(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	t0 := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)

	wi := &domain.WorkItem{
		ID: "wrk_001", Kind: "task", Title: "T", Status: "open",
		Priority: "medium", CreatedBy: "actor_a", CreatedAt: t0, UpdatedAt: t0,
		Labels: []string{}, Assignees: []domain.ActorID{}, Comments: []domain.Comment{},
		Relations: []domain.Relation{}, Checkpoints: []domain.Checkpoint{},
		Observations: []domain.Observation{}, Findings: []domain.Finding{}, Attempts: []domain.ExecutionAttempt{},
		Artifacts: []domain.Artifact{
			{ID: "art_001", ContentHash: "sha256:abc", Filename: "a.txt", MimeType: "text/plain", SizeBytes: 10, AddedBy: "actor_a", AddedAt: t0},
			{ID: "art_002", ContentHash: "sha256:abc", Filename: "b.txt", MimeType: "text/plain", SizeBytes: 10, AddedBy: "actor_a", AddedAt: t0},
		},
	}
	require.NoError(t, db.UpsertWorkItem(ctx, wi))

	arts, err := db.GetArtifactsByHash(ctx, "sha256:abc")
	require.NoError(t, err)
	assert.Len(t, arts, 2)

	arts, err = db.GetArtifactsByHash(ctx, "sha256:nonexistent")
	require.NoError(t, err)
	assert.Len(t, arts, 0)
}

// --- Overlay ---

func TestOverlay_Annotations(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	require.NoError(t, db.SetAnnotation(ctx, "wrk_001", "note", "important"))
	require.NoError(t, db.SetAnnotation(ctx, "wrk_001", "priority", "high"))

	ann, err := db.GetAnnotations(ctx, "wrk_001")
	require.NoError(t, err)
	assert.Equal(t, "important", ann["note"])
	assert.Equal(t, "high", ann["priority"])

	require.NoError(t, db.DeleteAnnotation(ctx, "wrk_001", "note"))
	ann, _ = db.GetAnnotations(ctx, "wrk_001")
	assert.Len(t, ann, 1)
}

func TestOverlay_PrivateLabels(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	require.NoError(t, db.AddPrivateLabel(ctx, "wrk_001", "my-focus"))
	require.NoError(t, db.AddPrivateLabel(ctx, "wrk_001", "review-later"))
	require.NoError(t, db.AddPrivateLabel(ctx, "wrk_001", "my-focus")) // idempotent

	labels, err := db.GetPrivateLabels(ctx, "wrk_001")
	require.NoError(t, err)
	assert.Len(t, labels, 2)

	require.NoError(t, db.RemovePrivateLabel(ctx, "wrk_001", "my-focus"))
	labels, _ = db.GetPrivateLabels(ctx, "wrk_001")
	assert.Len(t, labels, 1)
}

// --- Sync state ---

func TestSyncState_RemoteHeads(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	heads := []domain.EventID{"evt_001", "evt_002"}
	require.NoError(t, db.SetRemoteHeads(ctx, "server", heads))

	got, err := db.GetRemoteHeads(ctx, "server")
	require.NoError(t, err)
	assert.ElementsMatch(t, heads, got)

	// Update
	require.NoError(t, db.SetRemoteHeads(ctx, "server", []domain.EventID{"evt_003"}))
	got, _ = db.GetRemoteHeads(ctx, "server")
	assert.Equal(t, []domain.EventID{"evt_003"}, got)
}

func TestSyncState_RemoteURL(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	require.NoError(t, db.SetSyncRemote(ctx, "server", "http://localhost:8484"))

	url, err := db.GetSyncRemoteURL(ctx, "server")
	require.NoError(t, err)
	assert.Equal(t, "http://localhost:8484", url)

	// Not found
	url, err = db.GetSyncRemoteURL(ctx, "unknown")
	require.NoError(t, err)
	assert.Empty(t, url)
}

// --- Actor registry ---

func TestActorRegistry(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	require.NoError(t, db.RegisterActor(ctx, "actor_a", "pubkey_hex_a", "node_001"))

	key, err := db.GetActorPublicKey(ctx, "actor_a")
	require.NoError(t, err)
	assert.Equal(t, "pubkey_hex_a", key)

	// Update on conflict
	require.NoError(t, db.RegisterActor(ctx, "actor_a", "pubkey_hex_b", "node_002"))
	key, _ = db.GetActorPublicKey(ctx, "actor_a")
	assert.Equal(t, "pubkey_hex_b", key)

	// Not found
	key, err = db.GetActorPublicKey(ctx, "actor_nonexistent")
	require.NoError(t, err)
	assert.Empty(t, key)
}

// --- EmittedBy persistence ---

func TestAppendEvents_WithEmittedBy(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	t0 := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)

	evt := makeEvent("wrk_001", domain.EventWorkCreated, nil, "actor_a", t0,
		domain.WorkCreatedPayload{Title: "Test", Kind: "task"})
	evt.EmittedBy = &domain.EmittedBy{ActorID: "actor_a", AgentID: "deploy-bot", Version: "1.0"}

	require.NoError(t, db.AppendEvents(ctx, []domain.Event{evt}))

	got, err := db.GetEvent(ctx, evt.ID)
	require.NoError(t, err)
	require.NotNil(t, got.EmittedBy)
	assert.Equal(t, "deploy-bot", got.EmittedBy.AgentID)
	assert.Equal(t, "1.0", got.EmittedBy.Version)
}

// --- Checkpoint with Data ---

func TestWorkItem_CheckpointWithData(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	t0 := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)

	wi := &domain.WorkItem{
		ID: "wrk_001", Kind: "execution", Title: "T", Status: "open",
		Priority: "medium", CreatedBy: "actor_a", CreatedAt: t0, UpdatedAt: t0,
		Labels: []string{}, Assignees: []domain.ActorID{}, Comments: []domain.Comment{},
		Artifacts: []domain.Artifact{}, Relations: []domain.Relation{},
		Observations: []domain.Observation{}, Findings: []domain.Finding{}, Attempts: []domain.ExecutionAttempt{},
		Checkpoints: []domain.Checkpoint{
			{EventID: "evt_cp1", AttemptID: "atp_001", Summary: "step",
				Progress: 0.5, NextStep: "deploy",
				Data: json.RawMessage(`{"status":"building"}`), Timestamp: t0},
		},
	}
	require.NoError(t, db.UpsertWorkItem(ctx, wi))

	got, _ := db.GetWorkItem(ctx, "wrk_001")
	require.Len(t, got.Checkpoints, 1)
	assert.Equal(t, "deploy", got.Checkpoints[0].NextStep)
	assert.JSONEq(t, `{"status":"building"}`, string(got.Checkpoints[0].Data))
}
