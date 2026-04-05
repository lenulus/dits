package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lenulus/pf/internal/blob"
	"github.com/lenulus/pf/internal/domain"
	"github.com/lenulus/pf/internal/server"
	"github.com/lenulus/pf/internal/store/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"log/slog"
)

func setupServer(t *testing.T) (*server.Server, *sqlite.Store) {
	t.Helper()
	dir := t.TempDir()
	db, err := sqlite.Open(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	meta := domain.DefaultMetaConfig("TEST")
	require.NoError(t, db.SaveMeta(context.Background(), &meta))

	blobs, err := blob.NewFSStore(filepath.Join(dir, "blobs"))
	require.NoError(t, err)

	srv := server.New(db, blobs, slog.Default())
	return srv, db
}

func seedWorkItem(t *testing.T, db *sqlite.Store, id domain.WorkItemID, title, kind string, actor domain.ActorID, ts time.Time) {
	t.Helper()
	ctx := context.Background()
	evt := domain.Event{
		ID:         domain.NewEventID(),
		WorkItemID: id,
		Type:       domain.EventWorkCreated,
		ActorID:    actor,
		Timestamp:  ts,
		Payload:    domain.MustMarshalPayload(domain.WorkCreatedPayload{Title: title, Kind: kind}),
	}
	require.NoError(t, db.AppendEvents(ctx, []domain.Event{evt}))

	events, _ := db.GetEventsForWorkItem(ctx, id)
	ordered := domain.CausalOrder(events)
	wi, err := domain.Reduce(ordered)
	require.NoError(t, err)
	require.NoError(t, db.UpsertWorkItem(ctx, wi))
}

func appendEvent(t *testing.T, db *sqlite.Store, wiID domain.WorkItemID, evt domain.Event) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, db.AppendEvents(ctx, []domain.Event{evt}))

	events, _ := db.GetEventsForWorkItem(ctx, wiID)
	ordered := domain.CausalOrder(events)
	wi, err := domain.Reduce(ordered)
	require.NoError(t, err)

	existing, _ := db.GetWorkItem(ctx, wiID)
	if existing != nil && existing.SharedID != "" {
		wi.SharedID = existing.SharedID
	}
	require.NoError(t, db.UpsertWorkItem(ctx, wi))
}

func getJSON(t *testing.T, srv *server.Server, path string) map[string]any {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "GET %s returned %d: %s", path, w.Code, w.Body.String())

	var result map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
	return result
}

func TestV2_ListWorkItems(t *testing.T) {
	srv, db := setupServer(t)
	t0 := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)

	seedWorkItem(t, db, "wrk_001", "Task one", "task", "actor_a", t0)
	seedWorkItem(t, db, "wrk_002", "Bug report", "issue", "actor_b", t0.Add(time.Minute))

	result := getJSON(t, srv, "/api/v2/work")
	assert.Equal(t, float64(2), result["count"])

	// Filter by kind
	result = getJSON(t, srv, "/api/v2/work?kind=issue")
	assert.Equal(t, float64(1), result["count"])
}

func TestV2_GetWorkItem(t *testing.T) {
	srv, db := setupServer(t)
	t0 := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)

	seedWorkItem(t, db, "wrk_001", "My task", "task", "actor_a", t0)

	result := getJSON(t, srv, "/api/v2/work/wrk_001")
	assert.Equal(t, "wrk_001", result["ID"])
	assert.Equal(t, "My task", result["Title"])
}

func TestV2_GetWorkItem_NotFound(t *testing.T) {
	srv, _ := setupServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v2/work/wrk_nonexistent", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestV2_ReadyFilter(t *testing.T) {
	srv, db := setupServer(t)
	ctx := context.Background()
	t0 := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)

	// Create 3 work items: one open, one blocked, one leased
	seedWorkItem(t, db, "wrk_open", "Open task", "task", "actor_a", t0)
	seedWorkItem(t, db, "wrk_blocked", "Blocked task", "task", "actor_a", t0.Add(time.Minute))
	seedWorkItem(t, db, "wrk_leased", "Leased task", "task", "actor_a", t0.Add(2*time.Minute))

	// Block one
	heads, _ := db.GetHeads(ctx, "wrk_blocked")
	appendEvent(t, db, "wrk_blocked", domain.Event{
		ID: domain.NewEventID(), WorkItemID: "wrk_blocked", Type: domain.EventWorkBlocked,
		ParentEventIDs: heads, ActorID: "actor_a", Timestamp: t0.Add(3 * time.Minute),
		Payload: domain.MustMarshalPayload(domain.BlockedPayload{Reason: "dependency"}),
	})

	// Lease one
	heads, _ = db.GetHeads(ctx, "wrk_leased")
	appendEvent(t, db, "wrk_leased", domain.Event{
		ID: domain.NewEventID(), WorkItemID: "wrk_leased", Type: domain.EventWorkLeased,
		ParentEventIDs: heads, ActorID: "actor_b", Timestamp: t0.Add(4 * time.Minute),
		Payload: domain.MustMarshalPayload(domain.LeasedPayload{
			LeaseID: "lea_001", LeaseDurationSec: 300,
			LeaseExpiresAt: t0.Add(9 * time.Minute), Generation: 1,
		}),
	})

	// ready=true should only return the open, unblocked, unleased item
	result := getJSON(t, srv, "/api/v2/work?ready=true")
	count := result["count"].(float64)
	assert.Equal(t, float64(1), count)

	items := result["work_items"].([]any)
	wi := items[0].(map[string]any)
	assert.Equal(t, "wrk_open", wi["ID"])
}

func TestV2_BlockedFilter(t *testing.T) {
	srv, db := setupServer(t)
	ctx := context.Background()
	t0 := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)

	seedWorkItem(t, db, "wrk_001", "Normal", "task", "actor_a", t0)
	seedWorkItem(t, db, "wrk_002", "Blocked", "task", "actor_a", t0.Add(time.Minute))

	heads, _ := db.GetHeads(ctx, "wrk_002")
	appendEvent(t, db, "wrk_002", domain.Event{
		ID: domain.NewEventID(), WorkItemID: "wrk_002", Type: domain.EventWorkBlocked,
		ParentEventIDs: heads, ActorID: "actor_a", Timestamp: t0.Add(2 * time.Minute),
		Payload: domain.MustMarshalPayload(domain.BlockedPayload{Reason: "waiting"}),
	})

	result := getJSON(t, srv, "/api/v2/work?blocked=true")
	assert.Equal(t, float64(1), result["count"])
}

func TestV2_ClaimedByFilter(t *testing.T) {
	srv, db := setupServer(t)
	ctx := context.Background()
	t0 := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)

	seedWorkItem(t, db, "wrk_001", "Task", "execution", "actor_a", t0)

	heads, _ := db.GetHeads(ctx, "wrk_001")
	appendEvent(t, db, "wrk_001", domain.Event{
		ID: domain.NewEventID(), WorkItemID: "wrk_001", Type: domain.EventWorkLeased,
		ParentEventIDs: heads, ActorID: "actor_agent", Timestamp: t0.Add(time.Minute),
		Payload: domain.MustMarshalPayload(domain.LeasedPayload{
			LeaseID: "lea_001", LeaseDurationSec: 300,
			LeaseExpiresAt: t0.Add(6 * time.Minute), Generation: 1,
		}),
	})

	result := getJSON(t, srv, "/api/v2/work?claimed_by=actor_agent")
	assert.Equal(t, float64(1), result["count"])

	result = getJSON(t, srv, "/api/v2/work?claimed_by=actor_nobody")
	assert.Equal(t, float64(0), result["count"])
}

func TestV2_WorkItemEvents(t *testing.T) {
	srv, db := setupServer(t)
	ctx := context.Background()
	t0 := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)

	seedWorkItem(t, db, "wrk_001", "Task", "task", "actor_a", t0)

	heads, _ := db.GetHeads(ctx, "wrk_001")
	appendEvent(t, db, "wrk_001", domain.Event{
		ID: domain.NewEventID(), WorkItemID: "wrk_001", Type: domain.EventWorkCommented,
		ParentEventIDs: heads, ActorID: "actor_a", Timestamp: t0.Add(time.Minute),
		Payload: domain.MustMarshalPayload(domain.CommentPayload{Body: "hello"}),
	})

	result := getJSON(t, srv, "/api/v2/work/wrk_001/events")
	assert.Equal(t, float64(2), result["count"]) // created + commented

	// Filter by type
	result = getJSON(t, srv, "/api/v2/work/wrk_001/events?type=work.commented")
	assert.Equal(t, float64(1), result["count"])
}

func TestV2_WorkItemArtifacts(t *testing.T) {
	srv, db := setupServer(t)
	ctx := context.Background()
	t0 := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)

	seedWorkItem(t, db, "wrk_001", "Investigation", "investigation", "actor_a", t0)

	heads, _ := db.GetHeads(ctx, "wrk_001")
	appendEvent(t, db, "wrk_001", domain.Event{
		ID: domain.NewEventID(), WorkItemID: "wrk_001", Type: domain.EventWorkArtifactAdded,
		ParentEventIDs: heads, ActorID: "actor_a", Timestamp: t0.Add(time.Minute),
		Payload: domain.MustMarshalPayload(domain.ArtifactAddedPayload{
			ArtifactID: "art_001", ContentHash: "sha256:abc", Filename: "log.txt",
			MimeType: "text/plain", SizeBytes: 100, ArtifactType: "log", SemanticRole: "evidence",
		}),
	})

	result := getJSON(t, srv, "/api/v2/work/wrk_001/artifacts")
	assert.Equal(t, float64(1), result["count"])

	// Filter by type
	result = getJSON(t, srv, "/api/v2/work/wrk_001/artifacts?type=log")
	assert.Equal(t, float64(1), result["count"])

	result = getJSON(t, srv, "/api/v2/work/wrk_001/artifacts?type=patch")
	assert.Equal(t, float64(0), result["count"])

	// Filter by role
	result = getJSON(t, srv, "/api/v2/work/wrk_001/artifacts?role=evidence")
	assert.Equal(t, float64(1), result["count"])
}

func TestV2_WorkItemAttempts(t *testing.T) {
	srv, db := setupServer(t)
	ctx := context.Background()
	t0 := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)

	seedWorkItem(t, db, "wrk_001", "Execution", "execution", "actor_a", t0)

	heads, _ := db.GetHeads(ctx, "wrk_001")
	appendEvent(t, db, "wrk_001", domain.Event{
		ID: domain.NewEventID(), WorkItemID: "wrk_001", Type: domain.EventWorkExecutionStarted,
		ParentEventIDs: heads, ActorID: "actor_agent", Timestamp: t0.Add(time.Minute),
		Payload: domain.MustMarshalPayload(domain.ExecutionStartedPayload{AttemptID: "atp_001", AttemptNumber: 1}),
	})

	result := getJSON(t, srv, "/api/v2/work/wrk_001/attempts")
	assert.Equal(t, float64(1), result["count"])
}

func TestV2_WorkItemCheckpoints(t *testing.T) {
	srv, db := setupServer(t)
	ctx := context.Background()
	t0 := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)

	seedWorkItem(t, db, "wrk_001", "Execution", "execution", "actor_a", t0)

	heads, _ := db.GetHeads(ctx, "wrk_001")
	appendEvent(t, db, "wrk_001", domain.Event{
		ID: domain.NewEventID(), WorkItemID: "wrk_001", Type: domain.EventWorkExecutionStarted,
		ParentEventIDs: heads, ActorID: "actor_agent", Timestamp: t0.Add(time.Minute),
		Payload: domain.MustMarshalPayload(domain.ExecutionStartedPayload{AttemptID: "atp_001", AttemptNumber: 1}),
	})

	heads, _ = db.GetHeads(ctx, "wrk_001")
	appendEvent(t, db, "wrk_001", domain.Event{
		ID: domain.NewEventID(), WorkItemID: "wrk_001", Type: domain.EventWorkCheckpointed,
		ParentEventIDs: heads, ActorID: "actor_agent", Timestamp: t0.Add(2 * time.Minute),
		Payload: domain.MustMarshalPayload(domain.CheckpointedPayload{
			AttemptID: "atp_001", Summary: "Step 1", Progress: 0.5,
		}),
	})

	result := getJSON(t, srv, "/api/v2/work/wrk_001/checkpoints")
	assert.Equal(t, float64(1), result["count"])

	// Filter by attempt_id
	result = getJSON(t, srv, "/api/v2/work/wrk_001/checkpoints?attempt_id=atp_001")
	assert.Equal(t, float64(1), result["count"])

	result = getJSON(t, srv, "/api/v2/work/wrk_001/checkpoints?attempt_id=atp_nonexistent")
	assert.Equal(t, float64(0), result["count"])
}

func TestV2_ListEvents(t *testing.T) {
	srv, db := setupServer(t)
	t0 := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)

	seedWorkItem(t, db, "wrk_001", "Task A", "task", "actor_a", t0)
	seedWorkItem(t, db, "wrk_002", "Task B", "task", "actor_b", t0.Add(time.Minute))

	result := getJSON(t, srv, "/api/v2/events")
	assert.Equal(t, float64(2), result["count"])

	// Filter by actor
	result = getJSON(t, srv, "/api/v2/events?actor_id=actor_a")
	assert.Equal(t, float64(1), result["count"])

	// Filter by type
	result = getJSON(t, srv, "/api/v2/events?type=work.created")
	assert.Equal(t, float64(2), result["count"])
}

func TestV2_Meta(t *testing.T) {
	srv, _ := setupServer(t)

	result := getJSON(t, srv, "/api/v2/meta")
	assert.Equal(t, "TEST", result["project_key"])
	assert.Equal(t, float64(1), result["version"])
}

func TestV2_Health(t *testing.T) {
	srv, _ := setupServer(t)
	result := getJSON(t, srv, "/api/v1/health")
	assert.Equal(t, "ok", result["status"])
}

func TestV1_BlobCheckUploadDownload(t *testing.T) {
	srv, _ := setupServer(t)

	// Check — nothing exists yet.
	checkBody := `{"hashes":["sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/blobs/check", strings.NewReader(checkBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var checkResp map[string]any
	json.Unmarshal(w.Body.Bytes(), &checkResp)
	missing := checkResp["missing"].([]any)
	assert.Len(t, missing, 1)

	// Upload — empty file hash.
	hash := "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	req = httptest.NewRequest(http.MethodPut, "/api/v1/blobs/"+hash, strings.NewReader(""))
	req.Header.Set("Content-Type", "application/octet-stream")
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	// Download.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/blobs/"+hash, nil)
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	// Download non-existent.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/blobs/sha256:nonexistent", nil)
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestV1_SyncHandler(t *testing.T) {
	srv, _ := setupServer(t)

	syncBody := `{"node_id":"node_test","project_key":"TEST","heads":[],"events":[],"meta_version":1}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sync", strings.NewReader(syncBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	// Response should contain events key (may be null/empty).
	_, hasEvents := resp["events"]
	assert.True(t, hasEvents, "sync response should contain events field")
}

func TestV1_SyncHandler_InvalidJSON(t *testing.T) {
	srv, _ := setupServer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sync", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestV2_ArtifactRoleFilter(t *testing.T) {
	srv, db := setupServer(t)
	ctx := context.Background()
	t0 := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)

	seedWorkItem(t, db, "wrk_001", "Investigation", "investigation", "actor_a", t0)

	// Add two artifacts with different roles.
	heads, _ := db.GetHeads(ctx, "wrk_001")
	appendEvent(t, db, "wrk_001", domain.Event{
		ID: domain.NewEventID(), WorkItemID: "wrk_001", Type: domain.EventWorkArtifactAdded,
		ParentEventIDs: heads, ActorID: "actor_a", Timestamp: t0.Add(time.Minute),
		Payload: domain.MustMarshalPayload(domain.ArtifactAddedPayload{
			ArtifactID: "art_001", ContentHash: "sha256:aaa", Filename: "trace.json",
			MimeType: "application/json", SizeBytes: 100, ArtifactType: "trace", SemanticRole: "evidence",
		}),
	})

	heads, _ = db.GetHeads(ctx, "wrk_001")
	appendEvent(t, db, "wrk_001", domain.Event{
		ID: domain.NewEventID(), WorkItemID: "wrk_001", Type: domain.EventWorkArtifactAdded,
		ParentEventIDs: heads, ActorID: "actor_a", Timestamp: t0.Add(2 * time.Minute),
		Payload: domain.MustMarshalPayload(domain.ArtifactAddedPayload{
			ArtifactID: "art_002", ContentHash: "sha256:bbb", Filename: "plan.md",
			MimeType: "text/markdown", SizeBytes: 200, ArtifactType: "plan", SemanticRole: "proposal",
		}),
	})

	// All artifacts.
	result := getJSON(t, srv, "/api/v2/work/wrk_001/artifacts")
	assert.Equal(t, float64(2), result["count"])

	// Filter by role.
	result = getJSON(t, srv, "/api/v2/work/wrk_001/artifacts?role=evidence")
	assert.Equal(t, float64(1), result["count"])

	result = getJSON(t, srv, "/api/v2/work/wrk_001/artifacts?role=proposal")
	assert.Equal(t, float64(1), result["count"])
}
