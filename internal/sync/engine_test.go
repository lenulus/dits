package sync_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lenulus/pf/internal/domain"
	"github.com/lenulus/pf/internal/store/sqlite"
	dsync "github.com/lenulus/pf/internal/sync"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupDB(t *testing.T, name string) *sqlite.Store {
	t.Helper()
	dir := t.TempDir()
	db, err := sqlite.Open(filepath.Join(dir, name+".db"))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	// Init meta.
	meta := domain.DefaultMetaConfig("TEST")
	require.NoError(t, db.SaveMeta(context.Background(), &meta))
	return db
}

func createWorkItemEvent(workItemID domain.WorkItemID, title string, actor domain.ActorID, ts time.Time) domain.Event {
	return domain.Event{
		ID:         domain.NewEventID(),
		WorkItemID: workItemID,
		Type:       domain.EventWorkCreated,
		ActorID:    actor,
		Timestamp:  ts,
		Payload:    domain.MustMarshalPayload(domain.WorkCreatedPayload{Title: title, Kind: "task"}),
	}
}

func commentEvent(workItemID domain.WorkItemID, parents []domain.EventID, body string, actor domain.ActorID, ts time.Time) domain.Event {
	return domain.Event{
		ID:             domain.NewEventID(),
		WorkItemID:     workItemID,
		Type:           domain.EventWorkCommented,
		ParentEventIDs: parents,
		ActorID:        actor,
		Timestamp:      ts,
		Payload:        domain.MustMarshalPayload(domain.CommentPayload{Body: body}),
	}
}

func titleEvent(workItemID domain.WorkItemID, parents []domain.EventID, title string, actor domain.ActorID, ts time.Time) domain.Event {
	return domain.Event{
		ID:             domain.NewEventID(),
		WorkItemID:     workItemID,
		Type:           domain.EventWorkTitleSet,
		ParentEventIDs: parents,
		ActorID:        actor,
		Timestamp:      ts,
		Payload:        domain.MustMarshalPayload(domain.TitleSetPayload{Title: title}),
	}
}

func materialize(t *testing.T, db *sqlite.Store, workItemID domain.WorkItemID) *domain.WorkItem {
	t.Helper()
	ctx := context.Background()
	events, err := db.GetEventsForWorkItem(ctx, workItemID)
	require.NoError(t, err)
	ordered := domain.CausalOrder(events)
	wi, err := domain.Reduce(ordered)
	require.NoError(t, err)

	existing, _ := db.GetWorkItem(ctx, workItemID)
	if existing != nil && existing.SharedID != "" {
		wi.SharedID = existing.SharedID
	}

	require.NoError(t, db.UpsertWorkItem(ctx, wi))
	return wi
}

func TestSync_BasicPushPull(t *testing.T) {
	ctx := context.Background()
	serverDB := setupDB(t, "server")
	clientDB := setupDB(t, "client")

	serverEngine := dsync.NewEngine(serverDB)
	clientEngine := dsync.NewEngine(clientDB)

	clientNode := domain.NodeID("client-1")
	serverNode := domain.NodeID("server")
	t0 := time.Date(2026, 3, 29, 10, 0, 0, 0, time.UTC)

	// Client creates a work item locally.
	wiID := domain.WorkItemID("wrk_test_001")
	evt := createWorkItemEvent(wiID, "Fix login", "actor_alice", t0)
	require.NoError(t, clientDB.AppendEvents(ctx, []domain.Event{evt}))
	materialize(t, clientDB, wiID)
	clientDB.AllocateSharedID(ctx, wiID, "TEST")

	// Client builds sync request and sends to server.
	req, err := clientEngine.BuildSyncRequest(ctx, clientNode, "TEST", serverNode)
	require.NoError(t, err)
	assert.Len(t, req.Events, 1)

	// Server processes.
	resp, err := serverEngine.HandleSync(ctx, *req)
	require.NoError(t, err)
	assert.Len(t, resp.Events, 0)

	// Server should now have the work item.
	serverWI, err := serverDB.GetWorkItem(ctx, wiID)
	require.NoError(t, err)
	require.NotNil(t, serverWI)
	assert.Equal(t, "Fix login", serverWI.Title)

	// Client applies response.
	require.NoError(t, clientEngine.ApplySync(ctx, resp, serverNode))

	// Second sync should be a no-op.
	req2, err := clientEngine.BuildSyncRequest(ctx, clientNode, "TEST", serverNode)
	require.NoError(t, err)
	assert.Len(t, req2.Events, 0, "second sync should push nothing")
}

func TestSync_TwoClients(t *testing.T) {
	ctx := context.Background()
	serverDB := setupDB(t, "server")
	clientADB := setupDB(t, "clientA")
	clientBDB := setupDB(t, "clientB")

	serverEngine := dsync.NewEngine(serverDB)
	clientAEngine := dsync.NewEngine(clientADB)
	clientBEngine := dsync.NewEngine(clientBDB)

	nodeA := domain.NodeID("client-A")
	nodeB := domain.NodeID("client-B")
	serverNode := domain.NodeID("server")
	t0 := time.Date(2026, 3, 29, 10, 0, 0, 0, time.UTC)

	// Client A creates a work item.
	wiID := domain.WorkItemID("wrk_test_002")
	evt1 := createWorkItemEvent(wiID, "Server error", "actor_alice", t0)
	require.NoError(t, clientADB.AppendEvents(ctx, []domain.Event{evt1}))
	materialize(t, clientADB, wiID)

	// A syncs -> server.
	reqA, err := clientAEngine.BuildSyncRequest(ctx, nodeA, "TEST", serverNode)
	require.NoError(t, err)
	respA, err := serverEngine.HandleSync(ctx, *reqA)
	require.NoError(t, err)
	require.NoError(t, clientAEngine.ApplySync(ctx, respA, serverNode))

	// B syncs <- server (gets A's work item).
	reqB, err := clientBEngine.BuildSyncRequest(ctx, nodeB, "TEST", serverNode)
	require.NoError(t, err)
	assert.Len(t, reqB.Events, 0)
	respB, err := serverEngine.HandleSync(ctx, *reqB)
	require.NoError(t, err)
	assert.Len(t, respB.Events, 1)
	require.NoError(t, clientBEngine.ApplySync(ctx, respB, serverNode))

	// B should now have the work item.
	wiB, err := clientBDB.GetWorkItem(ctx, wiID)
	require.NoError(t, err)
	require.NotNil(t, wiB)
	assert.Equal(t, "Server error", wiB.Title)

	// B comments on the work item.
	headsB, err := clientBDB.GetHeads(ctx, wiID)
	require.NoError(t, err)
	evt2 := commentEvent(wiID, headsB, "I can reproduce this", "actor_bob", t0.Add(5*time.Minute))
	require.NoError(t, clientBDB.AppendEvents(ctx, []domain.Event{evt2}))
	materialize(t, clientBDB, wiID)

	// B syncs -> server.
	reqB2, err := clientBEngine.BuildSyncRequest(ctx, nodeB, "TEST", serverNode)
	require.NoError(t, err)
	assert.Len(t, reqB2.Events, 1)
	respB2, err := serverEngine.HandleSync(ctx, *reqB2)
	require.NoError(t, err)
	require.NoError(t, clientBEngine.ApplySync(ctx, respB2, serverNode))

	// A syncs <- server (gets B's comment).
	reqA2, err := clientAEngine.BuildSyncRequest(ctx, nodeA, "TEST", serverNode)
	require.NoError(t, err)
	respA2, err := serverEngine.HandleSync(ctx, *reqA2)
	require.NoError(t, err)
	assert.Len(t, respA2.Events, 1)
	require.NoError(t, clientAEngine.ApplySync(ctx, respA2, serverNode))

	// A should now have B's comment.
	wiA, err := clientADB.GetWorkItem(ctx, wiID)
	require.NoError(t, err)
	require.NotNil(t, wiA)
	assert.Len(t, wiA.Comments, 1)
	assert.Equal(t, "I can reproduce this", wiA.Comments[0].Body)
}

func TestSync_ConcurrentEditsConverge(t *testing.T) {
	ctx := context.Background()
	serverDB := setupDB(t, "server")
	clientADB := setupDB(t, "clientA")
	clientBDB := setupDB(t, "clientB")

	serverEngine := dsync.NewEngine(serverDB)
	clientAEngine := dsync.NewEngine(clientADB)
	clientBEngine := dsync.NewEngine(clientBDB)

	nodeA := domain.NodeID("client-A")
	nodeB := domain.NodeID("client-B")
	serverNode := domain.NodeID("server")
	t0 := time.Date(2026, 3, 29, 10, 0, 0, 0, time.UTC)

	// Both clients start with the same work item (via server).
	wiID := domain.WorkItemID("wrk_test_003")
	evt1 := createWorkItemEvent(wiID, "Original title", "actor_alice", t0)

	// A creates and syncs.
	require.NoError(t, clientADB.AppendEvents(ctx, []domain.Event{evt1}))
	materialize(t, clientADB, wiID)

	reqA, _ := clientAEngine.BuildSyncRequest(ctx, nodeA, "TEST", serverNode)
	respA, _ := serverEngine.HandleSync(ctx, *reqA)
	clientAEngine.ApplySync(ctx, respA, serverNode)

	// B syncs to get the work item.
	reqB, _ := clientBEngine.BuildSyncRequest(ctx, nodeB, "TEST", serverNode)
	respB, _ := serverEngine.HandleSync(ctx, *reqB)
	clientBEngine.ApplySync(ctx, respB, serverNode)

	// Both now have the work item. Make concurrent edits.
	headsA, _ := clientADB.GetHeads(ctx, wiID)
	headsB, _ := clientBDB.GetHeads(ctx, wiID)

	// A changes title.
	evtA := titleEvent(wiID, headsA, "Title from A", "actor_alice", t0.Add(1*time.Minute))
	require.NoError(t, clientADB.AppendEvents(ctx, []domain.Event{evtA}))
	materialize(t, clientADB, wiID)

	// B changes title (slightly later).
	evtB := titleEvent(wiID, headsB, "Title from B", "actor_bob", t0.Add(2*time.Minute))
	require.NoError(t, clientBDB.AppendEvents(ctx, []domain.Event{evtB}))
	materialize(t, clientBDB, wiID)

	// A syncs first.
	reqA2, _ := clientAEngine.BuildSyncRequest(ctx, nodeA, "TEST", serverNode)
	respA2, _ := serverEngine.HandleSync(ctx, *reqA2)
	clientAEngine.ApplySync(ctx, respA2, serverNode)

	// B syncs (pushes B's edit, pulls A's edit).
	reqB2, _ := clientBEngine.BuildSyncRequest(ctx, nodeB, "TEST", serverNode)
	respB2, _ := serverEngine.HandleSync(ctx, *reqB2)
	clientBEngine.ApplySync(ctx, respB2, serverNode)

	// A syncs again to get B's edit.
	reqA3, _ := clientAEngine.BuildSyncRequest(ctx, nodeA, "TEST", serverNode)
	respA3, _ := serverEngine.HandleSync(ctx, *reqA3)
	clientAEngine.ApplySync(ctx, respA3, serverNode)

	// All three should converge to the same title.
	wiServer, _ := serverDB.GetWorkItem(ctx, wiID)
	wiA, _ := clientADB.GetWorkItem(ctx, wiID)
	wiAB, _ := clientBDB.GetWorkItem(ctx, wiID)

	require.NotNil(t, wiServer)
	require.NotNil(t, wiA)
	require.NotNil(t, wiAB)
	assert.Equal(t, wiServer.Title, wiA.Title, "server and A should converge")
	assert.Equal(t, wiServer.Title, wiAB.Title, "server and B should converge")
	assert.Equal(t, "Title from B", wiServer.Title, "later timestamp wins in causal order")
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
