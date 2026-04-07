package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"

	"github.com/lenulus/pf/internal/domain"
	"github.com/lenulus/pf/internal/logging"
	"github.com/lenulus/pf/internal/project"
	"github.com/lenulus/pf/internal/server"
	"github.com/lenulus/pf/internal/store"
	"github.com/lenulus/pf/internal/workops"
	mcpgo "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

// TestReadyFilterConformance pins the canonical readiness contract
// across all three call paths:
//
//   - the dits_work_list MCP tool (internal/mcp)
//   - the GET /api/v2/work?ready=true HTTP endpoint (internal/server)
//   - the in-process domain.IsReady reference oracle
//
// All three are expected to return the same set of work item IDs for a
// given project state. Without this test the two handlers can drift
// silently — exactly what ChatGPT's review caught after the inline MCP
// filter was first written.
//
// We populate one project with a deliberate mix:
//   - a plain ready item
//   - a leased item                 → not ready
//   - a blocked item                → not ready
//   - a closed item                 → not ready
//   - an item with a running attempt (lease released afterwards) →
//     not ready (this is the case the SQL-only filter misses and the
//     reason the v2 handler now applies domain.IsReady in Go)
func TestReadyFilterConformance(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	proj, err := project.Init(root, "CONF")
	if err != nil {
		t.Fatalf("project.Init: %v", err)
	}
	w := &workops.WorkOps{Proj: proj}
	w.SetLogger(logging.Discard())

	// 1. plain ready
	plain, err := w.CreateWorkItem(ctx, "task", "plain", "", nil)
	if err != nil {
		t.Fatalf("create plain: %v", err)
	}

	// 2. leased — create then lease.
	leased, err := w.CreateWorkItem(ctx, "task", "leased", "", nil)
	if err != nil {
		t.Fatalf("create leased: %v", err)
	}
	if _, err := w.Lease(ctx, leased.WorkItem.ID); err != nil {
		t.Fatalf("lease leased: %v", err)
	}

	// 3. blocked — create then block.
	blocked, err := w.CreateWorkItem(ctx, "task", "blocked", "", nil)
	if err != nil {
		t.Fatalf("create blocked: %v", err)
	}
	if _, err := w.Block(ctx, blocked.WorkItem.ID, "waiting"); err != nil {
		t.Fatalf("block: %v", err)
	}

	// 4. closed — create then close.
	closed, err := w.CreateWorkItem(ctx, "task", "closed", "", nil)
	if err != nil {
		t.Fatalf("create closed: %v", err)
	}
	if _, err := w.Close(ctx, closed.WorkItem.ID); err != nil {
		t.Fatalf("close: %v", err)
	}

	// 5. running-attempt — create, lease, start, then RELEASE the lease.
	//    What's left: an open, unblocked, unleased item with a running
	//    authoritative attempt. The SQL ready filter (which only checks
	//    lease_holder IS NULL) would call this ready; domain.IsReady
	//    would not. This is the case the v2 handler's switch from SQL
	//    pushdown to a domain.IsReady post-pass exists to catch.
	running, err := w.CreateWorkItem(ctx, "task", "running", "", nil)
	if err != nil {
		t.Fatalf("create running: %v", err)
	}
	if _, err := w.Lease(ctx, running.WorkItem.ID); err != nil {
		t.Fatalf("lease running: %v", err)
	}
	if _, err := w.Start(ctx, running.WorkItem.ID); err != nil {
		t.Fatalf("start running: %v", err)
	}
	if _, err := w.LeaseRelease(ctx, running.WorkItem.ID, "test"); err != nil {
		t.Fatalf("release running: %v", err)
	}

	// Compute the oracle: load reduced state for every item, run
	// domain.IsReady directly, collect the IDs that come back ready.
	meta, err := w.LoadMeta(ctx)
	if err != nil {
		t.Fatalf("LoadMeta: %v", err)
	}
	openStatuses := meta.OpenStatuses()
	allItems, err := w.ListWorkItems(ctx, store.WorkItemFilter{}, false)
	if err != nil {
		t.Fatalf("ListWorkItems: %v", err)
	}
	expected := map[string]bool{}
	for i := range allItems {
		wi := allItems[i]
		if domain.IsReady(&wi, openStatuses) {
			expected[string(wi.ID)] = true
		}
	}
	// Sanity: only the plain item should be ready in this fixture.
	if !expected[string(plain.WorkItem.ID)] || len(expected) != 1 {
		t.Fatalf("oracle wrong: expected only %s ready, got %v",
			plain.WorkItem.ID, expected)
	}

	// --- Path A: dits_work_list MCP tool ---

	mcpIDs := callMCPReadyList(t, root)

	// --- Path B: GET /api/v2/work?ready=true HTTP endpoint ---

	// Hand the *same* DB + blob handles to the server so it sees the
	// same project state. workops keeps proj.DB open; the server can
	// read from it concurrently because the SQLITE_BUSY fix serializes
	// writers via the sync handler mutex (and we don't sync here).
	srv := server.New(proj.DB, proj.Blobs, logging.Discard())
	httpSrv := httptest.NewServer(srv)
	defer httpSrv.Close()
	httpIDs := callHTTPReadyList(t, httpSrv.URL)

	// All three sets must be identical.
	want := sortedKeys(expected)
	if !equalStrings(want, mcpIDs) {
		t.Errorf("MCP ready set ≠ oracle\n want=%v\n got =%v", want, mcpIDs)
	}
	if !equalStrings(want, httpIDs) {
		t.Errorf("HTTP ready set ≠ oracle\n want=%v\n got =%v", want, httpIDs)
	}
	if !equalStrings(mcpIDs, httpIDs) {
		t.Errorf("MCP ready set ≠ HTTP ready set\n mcp =%v\n http=%v",
			mcpIDs, httpIDs)
	}

	if err := proj.DB.Close(); err != nil {
		t.Fatalf("close project DB: %v", err)
	}
}

func callMCPReadyList(t *testing.T, root string) []string {
	t.Helper()
	srv := NewServer(Config{ProjectRoot: root})
	cli, err := mcpgo.NewInProcessClient(srv)
	if err != nil {
		t.Fatalf("NewInProcessClient: %v", err)
	}
	defer cli.Close()
	if err := cli.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{Name: "conf", Version: "0"}
	if _, err := cli.Initialize(context.Background(), initReq); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	callReq := mcp.CallToolRequest{}
	callReq.Params.Name = "dits_work_list"
	callReq.Params.Arguments = map[string]any{"ready": true}
	res, err := cli.CallTool(context.Background(), callReq)
	if err != nil {
		t.Fatalf("dits_work_list: %v", err)
	}
	if res.IsError {
		t.Fatalf("dits_work_list returned error result: %s", contentString(res))
	}

	var items []domain.WorkItem
	if err := json.Unmarshal([]byte(contentString(res)), &items); err != nil {
		t.Fatalf("decode mcp ready list: %v", err)
	}
	out := make([]string, 0, len(items))
	for _, wi := range items {
		out = append(out, string(wi.ID))
	}
	sort.Strings(out)
	return out
}

func callHTTPReadyList(t *testing.T, baseURL string) []string {
	t.Helper()
	resp, err := http.Get(baseURL + "/api/v2/work?ready=true")
	if err != nil {
		t.Fatalf("v2 list: %v", err)
	}
	defer resp.Body.Close()

	var body struct {
		WorkItems []domain.WorkItem `json:"work_items"`
		Count     int               `json:"count"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode v2 list: %v", err)
	}
	out := make([]string, 0, len(body.WorkItems))
	for _, wi := range body.WorkItems {
		out = append(out, string(wi.ID))
	}
	sort.Strings(out)
	return out
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
