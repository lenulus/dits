package workops_test

import (
	"context"
	"testing"

	"github.com/lenulus/pf/internal/domain"
	"github.com/lenulus/pf/internal/logging"
	"github.com/lenulus/pf/internal/project"
	"github.com/lenulus/pf/internal/workops"
)

// newREOps initializes a project and augments its meta with a taxonomy and
// roles so the RE substrate events validate against real meta references.
func newREOps(t *testing.T) *workops.WorkOps {
	t.Helper()
	root := t.TempDir()
	proj, err := project.Init(root, "WTEST")
	if err != nil {
		t.Fatalf("project.Init: %v", err)
	}
	w := &workops.WorkOps{Proj: proj}
	w.SetLogger(logging.Discard())
	t.Cleanup(func() { _ = w.Shutdown() })

	ctx := context.Background()
	meta, err := proj.DB.GetCurrentMeta(ctx)
	if err != nil {
		t.Fatalf("GetCurrentMeta: %v", err)
	}
	if err := meta.AddTaxonomy(domain.Taxonomy{
		Slug: "org", Name: "Organization",
		Nodes: []domain.TaxonomyNode{
			{Slug: "acme", Name: "Acme"},
			{Slug: "acme/platform", Name: "Platform", ParentSlug: "acme"},
		},
	}); err != nil {
		t.Fatalf("AddTaxonomy: %v", err)
	}
	if err := meta.AddRole(domain.Role{Slug: "pilot", Name: "Pilot", Cardinality: "exactly_one"}); err != nil {
		t.Fatalf("AddRole: %v", err)
	}
	if err := proj.DB.SaveMeta(ctx, meta); err != nil {
		t.Fatalf("SaveMeta: %v", err)
	}
	return w
}

// TestClassifyAndBindRole drives Classify/Declassify/BindRole/UnbindRole
// through WorkOps and asserts the materialized work item reflects each.
func TestClassifyAndBindRole(t *testing.T) {
	ctx := context.Background()
	w := newREOps(t)

	created, err := w.CreateWorkItem(ctx, "task", "re item", "", nil)
	if err != nil {
		t.Fatalf("CreateWorkItem: %v", err)
	}
	id := created.WorkItem.ID

	// Classify into a known node.
	wi, err := w.Classify(ctx, id, "org", "acme/platform")
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if len(wi.Classifications) != 1 || wi.Classifications[0].NodeSlug != "acme/platform" {
		t.Fatalf("expected one classification on acme/platform, got %+v", wi.Classifications)
	}

	// Unknown taxonomy/node must be rejected by validation.
	if _, err := w.Classify(ctx, id, "org", "ghost"); err == nil {
		t.Fatalf("expected error classifying into unknown node")
	}

	// Bind a role.
	wi, err = w.BindRole(ctx, id, "pilot", domain.ActorID("kokafor"))
	if err != nil {
		t.Fatalf("BindRole: %v", err)
	}
	if len(wi.RoleBindings) != 1 || wi.RoleBindings[0].Actor != "kokafor" {
		t.Fatalf("expected pilot bound to kokafor, got %+v", wi.RoleBindings)
	}

	// Re-binding the same role replaces the actor.
	wi, err = w.BindRole(ctx, id, "pilot", domain.ActorID("krivas"))
	if err != nil {
		t.Fatalf("BindRole(rebind): %v", err)
	}
	if len(wi.RoleBindings) != 1 || wi.RoleBindings[0].Actor != "krivas" {
		t.Fatalf("expected pilot rebound to krivas, got %+v", wi.RoleBindings)
	}

	// Unknown role rejected.
	if _, err := w.BindRole(ctx, id, "ghost", domain.ActorID("kokafor")); err == nil {
		t.Fatalf("expected error binding unknown role")
	}

	// Unbind the role.
	wi, err = w.UnbindRole(ctx, id, "pilot", domain.ActorID("krivas"))
	if err != nil {
		t.Fatalf("UnbindRole: %v", err)
	}
	if len(wi.RoleBindings) != 0 {
		t.Fatalf("expected no role bindings after unbind, got %+v", wi.RoleBindings)
	}

	// Declassify removes the classification.
	wi, err = w.Declassify(ctx, id, "org", "acme/platform")
	if err != nil {
		t.Fatalf("Declassify: %v", err)
	}
	if len(wi.Classifications) != 0 {
		t.Fatalf("expected no classifications after declassify, got %+v", wi.Classifications)
	}
}
