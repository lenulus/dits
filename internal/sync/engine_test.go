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

func createIssueEvent(issueID domain.CanonicalID, title string, actor domain.ActorID, ts time.Time) domain.Event {
	return domain.Event{
		ID:      domain.NewEventID(),
		IssueID: issueID,
		Type:    domain.EventIssueCreated,
		ActorID: actor,
		Timestamp: ts,
		Payload: domain.MustMarshalPayload(domain.IssueCreatedPayload{Title: title}),
	}
}

func commentEvent(issueID domain.CanonicalID, parents []domain.EventID, body string, actor domain.ActorID, ts time.Time) domain.Event {
	return domain.Event{
		ID:             domain.NewEventID(),
		IssueID:        issueID,
		Type:           domain.EventIssueCommented,
		ParentEventIDs: parents,
		ActorID:        actor,
		Timestamp:      ts,
		Payload:        domain.MustMarshalPayload(domain.CommentPayload{Body: body}),
	}
}

func titleEvent(issueID domain.CanonicalID, parents []domain.EventID, title string, actor domain.ActorID, ts time.Time) domain.Event {
	return domain.Event{
		ID:             domain.NewEventID(),
		IssueID:        issueID,
		Type:           domain.EventIssueTitleSet,
		ParentEventIDs: parents,
		ActorID:        actor,
		Timestamp:      ts,
		Payload:        domain.MustMarshalPayload(domain.TitleSetPayload{Title: title}),
	}
}

func materialize(t *testing.T, db *sqlite.Store, issueID domain.CanonicalID) *domain.Issue {
	t.Helper()
	ctx := context.Background()
	events, err := db.GetEventsForIssue(ctx, issueID)
	require.NoError(t, err)
	ordered := domain.CausalOrder(events)
	issue, err := domain.Reduce(ordered)
	require.NoError(t, err)

	existing, _ := db.GetIssue(ctx, issueID)
	if existing != nil && existing.SharedID != "" {
		issue.SharedID = existing.SharedID
	}

	require.NoError(t, db.UpsertIssue(ctx, issue))
	return issue
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

	// Client creates an issue locally.
	issueID := domain.CanonicalID("iss_test_001")
	evt := createIssueEvent(issueID, "Fix login", "actor_alice", t0)
	require.NoError(t, clientDB.AppendEvents(ctx, []domain.Event{evt}))
	materialize(t, clientDB, issueID)
	clientDB.AllocateSharedID(ctx, issueID, "TEST")

	// Client builds sync request and sends to server.
	req, err := clientEngine.BuildSyncRequest(ctx, clientNode, "TEST", serverNode)
	require.NoError(t, err)
	assert.Len(t, req.Events, 1)

	// Server processes.
	resp, err := serverEngine.HandleSync(ctx, *req)
	require.NoError(t, err)
	assert.Len(t, resp.Events, 0) // server had nothing to send back

	// Server should now have the issue.
	serverIssue, err := serverDB.GetIssue(ctx, issueID)
	require.NoError(t, err)
	require.NotNil(t, serverIssue)
	assert.Equal(t, "Fix login", serverIssue.Title)

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

	// Client A creates an issue.
	issueID := domain.CanonicalID("iss_test_002")
	evt1 := createIssueEvent(issueID, "Server error", "actor_alice", t0)
	require.NoError(t, clientADB.AppendEvents(ctx, []domain.Event{evt1}))
	materialize(t, clientADB, issueID)

	// A syncs -> server.
	reqA, err := clientAEngine.BuildSyncRequest(ctx, nodeA, "TEST", serverNode)
	require.NoError(t, err)
	respA, err := serverEngine.HandleSync(ctx, *reqA)
	require.NoError(t, err)
	require.NoError(t, clientAEngine.ApplySync(ctx, respA, serverNode))

	// B syncs <- server (gets A's issue).
	reqB, err := clientBEngine.BuildSyncRequest(ctx, nodeB, "TEST", serverNode)
	require.NoError(t, err)
	assert.Len(t, reqB.Events, 0) // B has nothing to push
	respB, err := serverEngine.HandleSync(ctx, *reqB)
	require.NoError(t, err)
	assert.Len(t, respB.Events, 1) // B gets A's event
	require.NoError(t, clientBEngine.ApplySync(ctx, respB, serverNode))

	// B should now have the issue.
	issueB, err := clientBDB.GetIssue(ctx, issueID)
	require.NoError(t, err)
	require.NotNil(t, issueB)
	assert.Equal(t, "Server error", issueB.Title)

	// B comments on the issue.
	headsB, err := clientBDB.GetHeads(ctx, issueID)
	require.NoError(t, err)
	evt2 := commentEvent(issueID, headsB, "I can reproduce this", "actor_bob", t0.Add(5*time.Minute))
	require.NoError(t, clientBDB.AppendEvents(ctx, []domain.Event{evt2}))
	materialize(t, clientBDB, issueID)

	// B syncs -> server.
	reqB2, err := clientBEngine.BuildSyncRequest(ctx, nodeB, "TEST", serverNode)
	require.NoError(t, err)
	assert.Len(t, reqB2.Events, 1) // B's comment
	respB2, err := serverEngine.HandleSync(ctx, *reqB2)
	require.NoError(t, err)
	require.NoError(t, clientBEngine.ApplySync(ctx, respB2, serverNode))

	// A syncs <- server (gets B's comment).
	reqA2, err := clientAEngine.BuildSyncRequest(ctx, nodeA, "TEST", serverNode)
	require.NoError(t, err)
	respA2, err := serverEngine.HandleSync(ctx, *reqA2)
	require.NoError(t, err)
	assert.Len(t, respA2.Events, 1) // A gets B's comment
	require.NoError(t, clientAEngine.ApplySync(ctx, respA2, serverNode))

	// A should now have B's comment.
	issueA, err := clientADB.GetIssue(ctx, issueID)
	require.NoError(t, err)
	require.NotNil(t, issueA)
	assert.Len(t, issueA.Comments, 1)
	assert.Equal(t, "I can reproduce this", issueA.Comments[0].Body)
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

	// Both clients start with the same issue (via server).
	issueID := domain.CanonicalID("iss_test_003")
	evt1 := createIssueEvent(issueID, "Original title", "actor_alice", t0)

	// A creates and syncs.
	require.NoError(t, clientADB.AppendEvents(ctx, []domain.Event{evt1}))
	materialize(t, clientADB, issueID)

	reqA, _ := clientAEngine.BuildSyncRequest(ctx, nodeA, "TEST", serverNode)
	respA, _ := serverEngine.HandleSync(ctx, *reqA)
	clientAEngine.ApplySync(ctx, respA, serverNode)

	// B syncs to get the issue.
	reqB, _ := clientBEngine.BuildSyncRequest(ctx, nodeB, "TEST", serverNode)
	respB, _ := serverEngine.HandleSync(ctx, *reqB)
	clientBEngine.ApplySync(ctx, respB, serverNode)

	// Both now have the issue. Make concurrent edits.
	headsA, _ := clientADB.GetHeads(ctx, issueID)
	headsB, _ := clientBDB.GetHeads(ctx, issueID)

	// A changes title.
	evtA := titleEvent(issueID, headsA, "Title from A", "actor_alice", t0.Add(1*time.Minute))
	require.NoError(t, clientADB.AppendEvents(ctx, []domain.Event{evtA}))
	materialize(t, clientADB, issueID)

	// B changes title (slightly later).
	evtB := titleEvent(issueID, headsB, "Title from B", "actor_bob", t0.Add(2*time.Minute))
	require.NoError(t, clientBDB.AppendEvents(ctx, []domain.Event{evtB}))
	materialize(t, clientBDB, issueID)

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
	// Both edits are concurrent (same parent). Causal order tiebreaks:
	// evtA timestamp < evtB timestamp, so evtA is applied first, evtB second.
	// Final title = "Title from B" (last-writer-wins in causal order).
	issueServer, _ := serverDB.GetIssue(ctx, issueID)
	issueA, _ := clientADB.GetIssue(ctx, issueID)
	issueAB, _ := clientBDB.GetIssue(ctx, issueID)

	// All three must have the same title.
	require.NotNil(t, issueServer)
	require.NotNil(t, issueA)
	require.NotNil(t, issueAB)
	assert.Equal(t, issueServer.Title, issueA.Title, "server and A should converge")
	assert.Equal(t, issueServer.Title, issueAB.Title, "server and B should converge")
	assert.Equal(t, "Title from B", issueServer.Title, "later timestamp wins in causal order")
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
