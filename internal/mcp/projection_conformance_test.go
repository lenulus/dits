package mcp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/lenulus/pf/internal/domain"
	"github.com/lenulus/pf/internal/logging"
	"github.com/lenulus/pf/internal/project"
	"github.com/lenulus/pf/internal/workops"
	mcpgo "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

// TestProjectionToolsConformance exercises the Track-0 generic projection
// surface end-to-end through the MCP tools and verifies it round-trips back
// out of dits_work_show / dits_meta_show — the substrate half of the Track-0
// acceptance ("a milestone round-trips RYG/stages/target/risks/next/narrative/
// visibility through MCP and back into the DTO").
func TestProjectionToolsConformance(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	proj, err := project.Init(root, "CONF")
	if err != nil {
		t.Fatalf("project.Init: %v", err)
	}
	w := &workops.WorkOps{Proj: proj}
	w.SetLogger(logging.Discard())

	// Seed a goals taxonomy directly (there is no add-taxonomy tool; node ops
	// operate on an existing taxonomy).
	meta, err := w.LoadMeta(ctx)
	if err != nil {
		t.Fatalf("LoadMeta: %v", err)
	}
	if err := meta.AddTaxonomy(domain.Taxonomy{Slug: "goals", Name: "Goals", Nodes: []domain.TaxonomyNode{
		{Slug: "customer-trust", Name: "Customer trust"},
	}}); err != nil {
		t.Fatalf("AddTaxonomy: %v", err)
	}
	if err := proj.DB.SaveMeta(ctx, meta); err != nil {
		t.Fatalf("SaveMeta: %v", err)
	}

	// Create a milestone-shaped work item (kind "task" — events are kind-
	// agnostic) plus a second item to link as the origin RFC.
	m, err := w.CreateWorkItem(ctx, "task", "Recurring billing v1", "", nil)
	if err != nil {
		t.Fatalf("create milestone: %v", err)
	}
	rfc, err := w.CreateWorkItem(ctx, "task", "Origin RFC", "", nil)
	if err != nil {
		t.Fatalf("create rfc: %v", err)
	}
	if err := proj.DB.Close(); err != nil {
		t.Fatalf("close DB before MCP: %v", err)
	}

	cli := startInProcMCP(t, root)
	id := string(m.WorkItem.ID)

	// Scalars.
	mustCallOK(t, cli, "dits_work_field_set", map[string]any{"id": id, "field": "ryg", "value": "g"})
	mustCallOK(t, cli, "dits_work_field_set", map[string]any{"id": id, "field": "target", "value": "2026 Q3"})
	mustCallOK(t, cli, "dits_work_field_set", map[string]any{"id": id, "field": "target_precision", "value": "D"})
	mustCallOK(t, cli, "dits_work_field_set", map[string]any{"id": id, "field": "customer_visible", "value": "true"})

	// Staged timeline.
	mustCallOK(t, cli, "dits_work_schedule_set", map[string]any{
		"id":          id,
		"stages_json": `[{"key":"beta","label":"Beta","date":"2026 Q3","precision":"Q","state":"open"},{"key":"ga","label":"GA","date":"2026-09-30","precision":"D","state":"open"}]`,
	})

	// Typed observations: status / risk / next.
	mustCallOK(t, cli, "dits_work_observe", map[string]any{"id": id, "summary": "Two of three subsystems integrated.", "data": `{"entry_type":"status"}`})
	mustCallOK(t, cli, "dits_work_observe", map[string]any{"id": id, "summary": "Webhook IPv6 routes flaky.", "data": `{"entry_type":"risk","severity":"medium"}`})
	mustCallOK(t, cli, "dits_work_observe", map[string]any{"id": id, "summary": "Cut Beta tag.", "data": `{"entry_type":"next","owner":"dnasser"}`})

	// Lineage.
	mustCallOK(t, cli, "dits_work_link", map[string]any{"id": id, "type": "derived_from", "target": string(rfc.WorkItem.ID)})

	// Read the milestone back and assert the projected state survives.
	showRes := mustCallOK(t, cli, "dits_work_show", map[string]any{"id": id})
	var got domain.WorkItem
	if err := json.Unmarshal([]byte(contentString(showRes)), &got); err != nil {
		t.Fatalf("decode work_show: %v", err)
	}
	if got.Fields["ryg"] != "g" || got.Fields["target"] != "2026 Q3" || got.Fields["target_precision"] != "D" || got.Fields["customer_visible"] != "true" {
		t.Errorf("fields did not round-trip: %#v", got.Fields)
	}
	if len(got.Stages) != 2 || got.Stages[0].Key != "beta" || got.Stages[1].Precision != "D" {
		t.Errorf("stages did not round-trip: %#v", got.Stages)
	}
	if len(got.Observations) != 3 {
		t.Errorf("want 3 observations, got %d", len(got.Observations))
	}
	var sawDerivedFrom bool
	for _, r := range got.Relations {
		if r.Type == "derived_from" && string(r.TargetWorkItem) == string(rfc.WorkItem.ID) {
			sawDerivedFrom = true
		}
	}
	if !sawDerivedFrom {
		t.Errorf("derived_from lineage did not round-trip: %#v", got.Relations)
	}

	// Taxonomy node metadata round-trip.
	mustCallOK(t, cli, "dits_taxonomy_node_set", map[string]any{
		"taxonomy": "goals", "slug": "customer-trust",
		"metadata": `{"result":"achieved","resultNote":"1 incident"}`,
	})
	metaRes := mustCallOK(t, cli, "dits_meta_show", map[string]any{})
	var gotMeta domain.MetaConfig
	if err := json.Unmarshal([]byte(contentString(metaRes)), &gotMeta); err != nil {
		t.Fatalf("decode meta_show: %v", err)
	}
	node := gotMeta.GetTaxonomyNode("goals", "customer-trust")
	if node == nil {
		t.Fatalf("goals/customer-trust node missing after set")
	}
	var nm struct {
		Result     string `json:"result"`
		ResultNote string `json:"resultNote"`
	}
	if err := json.Unmarshal(node.Metadata, &nm); err != nil {
		t.Fatalf("decode node metadata: %v", err)
	}
	if nm.Result != "achieved" || nm.ResultNote != "1 incident" {
		t.Errorf("node metadata did not round-trip: %#v", nm)
	}
}

// startInProcMCP spins up an in-process MCP server over the project at root and
// returns an initialized client.
func startInProcMCP(t *testing.T, root string) *mcpgo.Client {
	t.Helper()
	srv := NewServer(Config{ProjectRoot: root})
	cli, err := mcpgo.NewInProcessClient(srv)
	if err != nil {
		t.Fatalf("NewInProcessClient: %v", err)
	}
	t.Cleanup(func() { _ = cli.Close() })
	if err := cli.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{Name: "conf", Version: "0"}
	if _, err := cli.Initialize(context.Background(), initReq); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	return cli
}

func mustCallOK(t *testing.T, cli *mcpgo.Client, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	req := mcp.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = args
	res, err := cli.CallTool(context.Background(), req)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if res.IsError {
		t.Fatalf("%s returned error result: %s", name, contentString(res))
	}
	return res
}
