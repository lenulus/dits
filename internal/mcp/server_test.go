package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/lenulus/pf/internal/project"
	mcpgo "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

// TestServerSmoke initializes a temp DITS project, registers all tools on
// an in-process MCP server, and exercises the dispatch path: list (empty),
// create, list (one). This is the load-bearing smoke test the plan calls
// for: it proves the schemas, registration, dispatch, and workops wiring
// all line up.
func TestServerSmoke(t *testing.T) {
	root := t.TempDir()
	proj, err := project.Init(root, "TEST")
	if err != nil {
		t.Fatalf("project.Init: %v", err)
	}
	if err := proj.DB.Close(); err != nil {
		t.Fatalf("close project: %v", err)
	}

	srv := NewServer(Config{ProjectRoot: root})
	cli, err := mcpgo.NewInProcessClient(srv)
	if err != nil {
		t.Fatalf("NewInProcessClient: %v", err)
	}
	defer cli.Close()

	ctx := context.Background()
	if err := cli.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{Name: "smoke", Version: "0.0.0"}
	if _, err := cli.Initialize(ctx, initReq); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	// 1. Empty list.
	listReq := mcp.CallToolRequest{}
	listReq.Params.Name = "dits_work_list"
	listReq.Params.Arguments = map[string]any{}
	res, err := cli.CallTool(ctx, listReq)
	if err != nil {
		t.Fatalf("dits_work_list (empty): %v", err)
	}
	if res.IsError {
		t.Fatalf("dits_work_list (empty) returned error: %s", contentString(res))
	}
	if got := contentString(res); !strings.Contains(got, "[]") {
		t.Fatalf("expected empty list, got: %s", got)
	}

	// 2. Create a work item.
	createReq := mcp.CallToolRequest{}
	createReq.Params.Name = "dits_work_create"
	createReq.Params.Arguments = map[string]any{
		"title": "smoke test item",
		"kind":  "task",
	}
	res, err = cli.CallTool(ctx, createReq)
	if err != nil {
		t.Fatalf("dits_work_create: %v", err)
	}
	if res.IsError {
		t.Fatalf("dits_work_create returned error: %s", contentString(res))
	}

	// 3. List again — should now contain the item.
	res, err = cli.CallTool(ctx, listReq)
	if err != nil {
		t.Fatalf("dits_work_list (after create): %v", err)
	}
	if res.IsError {
		t.Fatalf("dits_work_list (after create) returned error: %s", contentString(res))
	}
	body := contentString(res)
	if !strings.Contains(body, "smoke test item") {
		t.Fatalf("created item not present in list output: %s", body)
	}

	// 4. dits_meta_show — exercises a read-only annotated tool.
	metaReq := mcp.CallToolRequest{}
	metaReq.Params.Name = "dits_meta_show"
	metaReq.Params.Arguments = map[string]any{}
	res, err = cli.CallTool(ctx, metaReq)
	if err != nil {
		t.Fatalf("dits_meta_show: %v", err)
	}
	if res.IsError {
		t.Fatalf("dits_meta_show returned error: %s", contentString(res))
	}
	if !strings.Contains(contentString(res), "TEST") {
		t.Fatalf("meta output missing project key: %s", contentString(res))
	}

	// 5. dits_sync — should fail cleanly with "no server URL configured"
	//    rather than panic, proving the workops wiring is hooked up (not
	//    the previous stub).
	syncReq := mcp.CallToolRequest{}
	syncReq.Params.Name = "dits_sync"
	syncReq.Params.Arguments = map[string]any{}
	res, err = cli.CallTool(ctx, syncReq)
	if err != nil {
		t.Fatalf("dits_sync: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected dits_sync to error without remote, got: %s", contentString(res))
	}
	if !strings.Contains(contentString(res), "no server URL configured") {
		t.Fatalf("dits_sync error wording unexpected: %s", contentString(res))
	}

	// Sanity-check that schema metadata round-trips: list tools and confirm
	// at least the read tools were registered with read-only annotations.
	tools, err := cli.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	var sawReadOnly bool
	for _, tool := range tools.Tools {
		if tool.Name == "dits_work_list" && tool.Annotations.ReadOnlyHint != nil && *tool.Annotations.ReadOnlyHint {
			sawReadOnly = true
		}
	}
	if !sawReadOnly {
		t.Fatalf("dits_work_list not annotated read-only")
	}
}

func contentString(res *mcp.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		switch v := c.(type) {
		case mcp.TextContent:
			b.WriteString(v.Text)
		default:
			data, _ := json.Marshal(v)
			b.Write(data)
		}
	}
	return b.String()
}
