package mcp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/lenulus/pf/internal/domain"
	"github.com/lenulus/pf/internal/project"
	mcpgo "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

// reClient spins up a fresh project, seeds its meta with a taxonomy + roles +
// the prototype's three role constraints, and returns an initialized in-process
// MCP client plus the project root. The seeding goes through workops/store
// directly so the MCP round-trip exercises the read/mutate tools, not setup.
func reClient(t *testing.T) (*mcpgo.Client, string) {
	t.Helper()
	root := t.TempDir()
	proj, err := project.Init(root, "RECONF")
	if err != nil {
		t.Fatalf("project.Init: %v", err)
	}
	ctx := context.Background()

	meta, err := proj.DB.GetCurrentMeta(ctx)
	if err != nil {
		t.Fatalf("GetCurrentMeta: %v", err)
	}
	if err := meta.AddTaxonomy(domain.Taxonomy{
		Slug: "product", Name: "Product",
		Nodes: []domain.TaxonomyNode{
			{Slug: "commerce", Name: "Commerce"},
			{Slug: "commerce/checkout", Name: "Checkout", ParentSlug: "commerce"},
		},
	}); err != nil {
		t.Fatalf("AddTaxonomy: %v", err)
	}
	for _, r := range []domain.Role{
		{Slug: "specifier", Name: "Specifier", Cardinality: "exactly_one"},
		{Slug: "builder", Name: "Builder", Cardinality: "exactly_one"},
		{Slug: "pilot", Name: "Pilot", Cardinality: "exactly_one"},
	} {
		if err := meta.AddRole(r); err != nil {
			t.Fatalf("AddRole %s: %v", r.Slug, err)
		}
	}
	if err := meta.AddRoleConstraint(domain.RoleConstraint{
		Slug: "no_role_collapse", Name: "No role collapse",
		Predicate: "distinct_actors", Severity: "violation",
		Args: map[string]any{"roles": []any{"specifier", "builder", "pilot"}},
	}); err != nil {
		t.Fatalf("AddRoleConstraint distinct: %v", err)
	}
	if err := meta.AddRoleConstraint(domain.RoleConstraint{
		Slug: "requires_product_classification", Name: "Product classification required",
		Predicate: "requires_classification", Severity: "warning",
		Args: map[string]any{"taxonomy": "product"},
	}); err != nil {
		t.Fatalf("AddRoleConstraint requires: %v", err)
	}
	if err := proj.DB.SaveMeta(ctx, meta); err != nil {
		t.Fatalf("SaveMeta: %v", err)
	}
	if err := proj.DB.Close(); err != nil {
		t.Fatalf("close seed DB: %v", err)
	}

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
	initReq.Params.ClientInfo = mcp.Implementation{Name: "re-conf", Version: "0"}
	if _, err := cli.Initialize(context.Background(), initReq); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	return cli, root
}

// call invokes a tool and fails the test if the call errors or returns an error
// result. It returns the text content for decoding.
func call(t *testing.T, cli *mcpgo.Client, name string, args map[string]any) string {
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
	return contentString(res)
}

// callExpectError invokes a tool expecting an error result.
func callExpectError(t *testing.T, cli *mcpgo.Client, name string, args map[string]any) {
	t.Helper()
	req := mcp.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = args
	res, err := cli.CallTool(context.Background(), req)
	if err != nil {
		t.Fatalf("%s transport error: %v", name, err)
	}
	if !res.IsError {
		t.Fatalf("%s expected error result, got ok: %s", name, contentString(res))
	}
}

func createMilestone(t *testing.T, cli *mcpgo.Client, title string) domain.WorkItem {
	t.Helper()
	out := call(t, cli, "dits_work_create", map[string]any{"title": title, "kind": "task"})
	var created struct {
		WorkItem domain.WorkItem `json:"work_item"`
	}
	if err := json.Unmarshal([]byte(out), &created); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	return created.WorkItem
}

// TestRESubstrateRoundTrip drives the Phase 3 tool surface end-to-end:
// classify, role bindings, the ACK lifecycle (file → accept both → aligned →
// material amendment clears), diagnostics, and the global events list.
func TestRESubstrateRoundTrip(t *testing.T) {
	cli, _ := reClient(t)

	wi := createMilestone(t, cli, "PROJ-176 pilot independence")
	id := string(wi.ID)

	// Classify into the product taxonomy.
	call(t, cli, "dits_work_classify", map[string]any{"id": id, "taxonomy": "product", "node": "commerce/checkout"})

	// Unknown node must be rejected by tier-1 validation.
	callExpectError(t, cli, "dits_work_classify", map[string]any{"id": id, "taxonomy": "product", "node": "ghost"})

	// Bind the three distinct roles.
	call(t, cli, "dits_work_role_bind", map[string]any{"id": id, "role": "specifier", "actor": "ejackson"})
	call(t, cli, "dits_work_role_bind", map[string]any{"id": id, "role": "builder", "actor": "tpark"})
	call(t, cli, "dits_work_role_bind", map[string]any{"id": id, "role": "pilot", "actor": "kokafor"})

	// role_bindings_list reflects all three.
	var bindings []domain.RoleBinding
	if err := json.Unmarshal([]byte(call(t, cli, "dits_role_bindings_list", map[string]any{"id": id})), &bindings); err != nil {
		t.Fatalf("decode role bindings: %v", err)
	}
	if len(bindings) != 3 {
		t.Fatalf("expected 3 role bindings, got %d: %+v", len(bindings), bindings)
	}

	// ACK lifecycle: file → accept both → aligned.
	call(t, cli, "dits_ack_file", map[string]any{
		"id": id, "scope_summary": "Enforce pilot independence", "delivery_timing": "2026 Q3",
	})
	call(t, cli, "dits_ack_accept", map[string]any{"id": id, "who": "specifier"})
	call(t, cli, "dits_ack_accept", map[string]any{"id": id, "who": "builder"})

	shown := showWorkItem(t, cli, id)
	if len(shown.Acks) != 1 {
		t.Fatalf("expected 1 ack, got %d", len(shown.Acks))
	}
	if got := domain.ComputeAckRollup(shown.Acks[0].Specifier, shown.Acks[0].Builder); got != domain.RollupAligned {
		t.Fatalf("expected aligned rollup after both accept, got %q", got)
	}
	// work_show surfaces the new fields for free.
	if len(shown.Classifications) != 1 || len(shown.RoleBindings) != 3 {
		t.Fatalf("work_show missing RE fields: classifications=%d bindings=%d",
			len(shown.Classifications), len(shown.RoleBindings))
	}

	// Material amendment (scope_change) after acceptance clears both sides.
	call(t, cli, "dits_ack_amend", map[string]any{
		"id": id, "amendment_type": "scope_change", "reason": "target shifted",
	})
	shown = showWorkItem(t, cli, id)
	if got := domain.ComputeAckRollup(shown.Acks[0].Specifier, shown.Acks[0].Builder); got == domain.RollupAligned {
		t.Fatalf("expected ack cleared (not aligned) after material amendment, got %q", got)
	}

	// Filtered list: by role+actor.
	var byRole []domain.WorkItem
	if err := json.Unmarshal([]byte(call(t, cli, "dits_work_list",
		map[string]any{"role": "pilot", "role_actor": "kokafor"})), &byRole); err != nil {
		t.Fatalf("decode work_list by role: %v", err)
	}
	if len(byRole) != 1 || byRole[0].ID != wi.ID {
		t.Fatalf("expected role+actor filter to return the milestone, got %+v", byRole)
	}

	// Filtered list: by taxonomy prefix.
	var byTax []domain.WorkItem
	if err := json.Unmarshal([]byte(call(t, cli, "dits_work_list",
		map[string]any{"taxonomy": "product", "node": "commerce"})), &byTax); err != nil {
		t.Fatalf("decode work_list by taxonomy: %v", err)
	}
	if len(byTax) != 1 {
		t.Fatalf("expected taxonomy prefix filter to return the milestone, got %d", len(byTax))
	}

	// events_list (global) returns events including our role bindings.
	var events []domain.Event
	if err := json.Unmarshal([]byte(call(t, cli, "dits_events_list",
		map[string]any{"type": string(domain.EventWorkRoleBound)})), &events); err != nil {
		t.Fatalf("decode events_list: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 role_bound events globally, got %d", len(events))
	}
}

// TestDiagnosticsAndMetaAdmin covers diagnostics_get firing on a role-collapse /
// unclassified item, plus the meta-admin and identity tools.
func TestDiagnosticsAndMetaAdmin(t *testing.T) {
	cli, _ := reClient(t)

	wi := createMilestone(t, cli, "collapsed + unclassified")
	id := string(wi.ID)

	// Collapse two roles onto one actor and leave the item unclassified.
	call(t, cli, "dits_work_role_bind", map[string]any{"id": id, "role": "specifier", "actor": "tpark"})
	call(t, cli, "dits_work_role_bind", map[string]any{"id": id, "role": "builder", "actor": "tpark"})
	call(t, cli, "dits_work_role_bind", map[string]any{"id": id, "role": "pilot", "actor": "kokafor"})

	var diags []domain.Diagnostic
	if err := json.Unmarshal([]byte(call(t, cli, "dits_diagnostics_get", map[string]any{"id": id})), &diags); err != nil {
		t.Fatalf("decode diagnostics: %v", err)
	}
	// distinct_actors (collapse) and requires_classification (unclassified) fire;
	// hierarchy predicates stay silent under nil positions.
	if findConstraint(diags, "no_role_collapse") == nil {
		t.Fatalf("expected no_role_collapse diagnostic, got %+v", diags)
	}
	if findConstraint(diags, "requires_product_classification") == nil {
		t.Fatalf("expected requires_product_classification diagnostic, got %+v", diags)
	}

	// Meta admin: add, move, retire a taxonomy node; each bumps the version.
	call(t, cli, "dits_taxonomy_node_add", map[string]any{
		"taxonomy": "product", "slug": "commerce/checkout/payments", "name": "Payments",
		"parent_slug": "commerce/checkout",
	})
	call(t, cli, "dits_taxonomy_node_move", map[string]any{
		"taxonomy": "product", "slug": "commerce/checkout/payments", "new_parent_slug": "commerce",
	})
	retired := call(t, cli, "dits_taxonomy_node_retire", map[string]any{
		"taxonomy": "product", "slug": "commerce/checkout/payments",
	})
	var metaAfter domain.MetaConfig
	if err := json.Unmarshal([]byte(retired), &metaAfter); err != nil {
		t.Fatalf("decode retire result: %v", err)
	}
	if n := metaAfter.GetTaxonomyNode("product", "commerce/checkout/payments"); n == nil || !n.Retired {
		t.Fatalf("expected payments node to be retired")
	}
	// New classification on a retired node must be rejected.
	callExpectError(t, cli, "dits_work_classify", map[string]any{
		"id": id, "taxonomy": "product", "node": "commerce/checkout/payments",
	})

	// Identity: register then list.
	call(t, cli, "dits_actor_register", map[string]any{
		"actor_id": "kokafor", "public_key": "ed25519:deadbeef", "node_id": "node-1",
	})
	out := call(t, cli, "dits_actor_list", nil)
	if !containsActorID(t, out, "kokafor") {
		t.Fatalf("expected kokafor in actor_list, got %s", out)
	}
}

func showWorkItem(t *testing.T, cli *mcpgo.Client, id string) domain.WorkItem {
	t.Helper()
	var wi domain.WorkItem
	if err := json.Unmarshal([]byte(call(t, cli, "dits_work_show", map[string]any{"id": id})), &wi); err != nil {
		t.Fatalf("decode work_show: %v", err)
	}
	return wi
}

func findConstraint(diags []domain.Diagnostic, slug string) *domain.Diagnostic {
	for i := range diags {
		if diags[i].ConstraintSlug == slug {
			return &diags[i]
		}
	}
	return nil
}

func containsActorID(t *testing.T, jsonStr, actorID string) bool {
	t.Helper()
	var actors []struct {
		ActorID string `json:"actor_id"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &actors); err != nil {
		t.Fatalf("decode actor_list: %v", err)
	}
	for _, a := range actors {
		if a.ActorID == actorID {
			return true
		}
	}
	return false
}
