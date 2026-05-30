package constraints

import (
	"testing"

	"github.com/lenulus/pf/internal/domain"
)

// orgTaxonomy mirrors the prototype's org tree (data.jsx). Pilot positions in
// the golden test sit one level below the builder's node.
func orgTaxonomy() domain.Taxonomy {
	return domain.Taxonomy{
		Slug: "org", Name: "Organization",
		Nodes: []domain.TaxonomyNode{
			{Slug: "acme", Name: "Acme"},
			{Slug: "acme/platform", Name: "Platform", ParentSlug: "acme"},
			{Slug: "acme/platform/infra-team", Name: "Infra Team", ParentSlug: "acme/platform"},
			{Slug: "acme/platform/infra-team/infra-pod", Name: "Infra Pod", ParentSlug: "acme/platform/infra-team"},
			{Slug: "acme/platform/payments-team", Name: "Payments Team", ParentSlug: "acme/platform"},
		},
	}
}

func findDiag(diags []domain.Diagnostic, slug string) *domain.Diagnostic {
	for i := range diags {
		if diags[i].ConstraintSlug == slug {
			return &diags[i]
		}
	}
	return nil
}

func TestDistinctActors(t *testing.T) {
	c := domain.RoleConstraint{
		Slug: "no_role_collapse", Predicate: "distinct_actors", Severity: "violation",
		Args: map[string]any{"roles": []any{"specifier", "builder", "pilot"}},
	}
	meta := &domain.MetaConfig{RoleConstraints: []domain.RoleConstraint{c}}

	// Three distinct actors → no diagnostic.
	wi := &domain.WorkItem{RoleBindings: []domain.RoleBinding{
		{RoleSlug: "specifier", Actor: "ejackson"},
		{RoleSlug: "builder", Actor: "tpark"},
		{RoleSlug: "pilot", Actor: "kokafor"},
	}}
	if d := Evaluate(wi, meta, nil); len(d) != 0 {
		t.Fatalf("expected no diagnostics for distinct actors, got %+v", d)
	}

	// Collapsed roles → one diagnostic.
	wi.RoleBindings[2].Actor = "tpark" // pilot == builder
	d := Evaluate(wi, meta, nil)
	if findDiag(d, "no_role_collapse") == nil {
		t.Fatalf("expected no_role_collapse diagnostic, got %+v", d)
	}
}

func TestRequiresClassification(t *testing.T) {
	c := domain.RoleConstraint{
		Slug: "requires_product_classification", Predicate: "requires_classification", Severity: "warning",
		Args: map[string]any{"taxonomy": "product"},
	}
	meta := &domain.MetaConfig{RoleConstraints: []domain.RoleConstraint{c}}

	// Unclassified in product → fires.
	wi := &domain.WorkItem{}
	if findDiag(Evaluate(wi, meta, nil), "requires_product_classification") == nil {
		t.Fatalf("expected requires_product_classification to fire when unclassified")
	}

	// Classified in product → silent.
	wi.Classifications = []domain.Classification{{TaxonomySlug: "product", NodeSlug: "commerce/checkout"}}
	if findDiag(Evaluate(wi, meta, nil), "requires_product_classification") != nil {
		t.Fatalf("expected no diagnostic once classified in product")
	}
}

func TestNotReportsToWithin(t *testing.T) {
	c := domain.RoleConstraint{
		Slug: "pilot_independence", Predicate: "not_reports_to_within", Severity: "warning",
		Args: map[string]any{
			"role": "pilot", "other_roles": []any{"specifier", "builder"},
			"taxonomy": "org", "max_levels": float64(2),
		},
	}
	meta := &domain.MetaConfig{
		Taxonomies:      []domain.Taxonomy{orgTaxonomy()},
		RoleConstraints: []domain.RoleConstraint{c},
	}
	wi := &domain.WorkItem{RoleBindings: []domain.RoleBinding{
		{RoleSlug: "builder", Actor: "tpark"},
		{RoleSlug: "pilot", Actor: "kokafor"},
	}}

	// Pilot one level below builder → fires.
	pos := ActorPositions{
		"tpark":   "acme/platform/infra-team",
		"kokafor": "acme/platform/infra-team/infra-pod",
	}
	if d := findDiag(Evaluate(wi, meta, pos), "pilot_independence"); d == nil {
		t.Fatalf("expected pilot_independence to fire when pilot reports to builder")
	}

	// Pilot in a sibling subtree (not an ancestor relationship) → silent.
	pos["kokafor"] = "acme/platform/payments-team"
	if d := findDiag(Evaluate(wi, meta, pos), "pilot_independence"); d != nil {
		t.Fatalf("expected no diagnostic when pilot does not report to builder, got %+v", d)
	}

	// Independence beyond max_levels → silent. Builder is the org root's child;
	// pilot far below exceeds 2 levels.
	pos["tpark"] = "acme"
	pos["kokafor"] = "acme/platform/infra-team/infra-pod"
	if d := findDiag(Evaluate(wi, meta, pos), "pilot_independence"); d != nil {
		t.Fatalf("expected no diagnostic beyond max_levels, got %+v", d)
	}
}

func TestClassifiedInSameNode(t *testing.T) {
	c := domain.RoleConstraint{
		Slug: "owner_alignment", Predicate: "classified_in_same_node", Severity: "warning",
		Args: map[string]any{"taxonomy": "org", "role": "builder"},
	}
	meta := &domain.MetaConfig{
		Taxonomies:      []domain.Taxonomy{orgTaxonomy()},
		RoleConstraints: []domain.RoleConstraint{c},
	}
	wi := &domain.WorkItem{
		RoleBindings:    []domain.RoleBinding{{RoleSlug: "builder", Actor: "tpark"}},
		Classifications: []domain.Classification{{TaxonomySlug: "org", NodeSlug: "acme/platform/infra-team"}},
	}

	// Builder positioned in the same node as the work item → silent.
	pos := ActorPositions{"tpark": "acme/platform/infra-team"}
	if d := findDiag(Evaluate(wi, meta, pos), "owner_alignment"); d != nil {
		t.Fatalf("expected no diagnostic when builder matches classification node, got %+v", d)
	}

	// Builder positioned elsewhere → fires.
	pos["tpark"] = "acme/platform/payments-team"
	if d := findDiag(Evaluate(wi, meta, pos), "owner_alignment"); d == nil {
		t.Fatalf("expected owner_alignment to fire on node mismatch")
	}

	// Missing inputs → does not fire.
	if d := findDiag(Evaluate(wi, meta, nil), "owner_alignment"); d != nil {
		t.Fatalf("expected no diagnostic when actor position is absent, got %+v", d)
	}
}

// TestRoleConstraint_PilotIndependence is the golden scenario: the prototype's
// PROJ-176 role collapse. specifier=ejackson, builder=tpark, pilot=kokafor;
// pilot is positioned under builder in the org tree; the product taxonomy is
// unclassified. Pins exactly which of the three prototype constraints fire.
func TestRoleConstraint_PilotIndependence(t *testing.T) {
	meta := &domain.MetaConfig{
		Taxonomies: []domain.Taxonomy{orgTaxonomy()},
		RoleConstraints: []domain.RoleConstraint{
			{
				Slug: "no_role_collapse", Name: "No role collapse",
				Predicate: "distinct_actors", Severity: "violation",
				Args: map[string]any{"roles": []any{"specifier", "builder", "pilot"}},
			},
			{
				Slug: "pilot_independence", Name: "Pilot independence",
				Predicate: "not_reports_to_within", Severity: "warning",
				Args: map[string]any{
					"role": "pilot", "other_roles": []any{"specifier", "builder"},
					"taxonomy": "org", "max_levels": float64(2),
				},
			},
			{
				Slug: "requires_product_classification", Name: "Product classification required",
				Predicate: "requires_classification", Severity: "warning",
				Args: map[string]any{"taxonomy": "product"},
			},
		},
	}

	wi := &domain.WorkItem{
		RoleBindings: []domain.RoleBinding{
			{RoleSlug: "specifier", Actor: "ejackson"},
			{RoleSlug: "builder", Actor: "tpark"},
			{RoleSlug: "pilot", Actor: "kokafor"},
		},
		// Deliberately unclassified in the product taxonomy.
	}
	pos := ActorPositions{
		"ejackson": "acme/platform/payments-team",
		"tpark":    "acme/platform/infra-team",
		"kokafor":  "acme/platform/infra-team/infra-pod", // reports to builder, 1 level
	}

	diags := Evaluate(wi, meta, pos)

	// distinct_actors does NOT fire — three distinct actors.
	if d := findDiag(diags, "no_role_collapse"); d != nil {
		t.Fatalf("no_role_collapse should not fire with 3 distinct actors, got %+v", d)
	}

	// pilot_independence DOES fire (warning), pinning the 1-level message.
	pi := findDiag(diags, "pilot_independence")
	if pi == nil {
		t.Fatalf("expected pilot_independence diagnostic to fire")
	}
	if pi.Severity != "warning" {
		t.Fatalf("pilot_independence severity: got %q want warning", pi.Severity)
	}
	if pi.Message != "pilot reports to builder (1 level)" {
		t.Fatalf("pilot_independence message: got %q", pi.Message)
	}

	// requires_product_classification DOES fire (warning).
	rc := findDiag(diags, "requires_product_classification")
	if rc == nil {
		t.Fatalf("expected requires_product_classification diagnostic to fire")
	}
	if rc.Severity != "warning" {
		t.Fatalf("requires_product_classification severity: got %q want warning", rc.Severity)
	}

	// Exactly the two expected diagnostics.
	if len(diags) != 2 {
		t.Fatalf("expected exactly 2 diagnostics, got %d: %+v", len(diags), diags)
	}
}
