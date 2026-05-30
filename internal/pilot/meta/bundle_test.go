package meta

import (
	"encoding/json"
	"testing"
)

// TestBundleValid asserts the embedded RE bundle passes structural
// validation (valid JSON, resolvable workflows, known categories /
// cardinalities / severities, full vocabulary).
func TestBundleValid(t *testing.T) {
	if err := Validate(); err != nil {
		t.Fatalf("Validate(): %v", err)
	}
}

// TestBundleIsValidJSON is a standalone json.Valid sanity check on the raw
// embedded bytes, independent of the schema validation.
func TestBundleIsValidJSON(t *testing.T) {
	if b := Bundle(); !json.Valid(b) {
		t.Fatalf("Bundle() is not valid JSON")
	}
}

// TestBundleHasREVocabulary asserts the bundle carries the exact RE
// vocabulary the methodology depends on: 5 work kinds, 5 roles, 3 role
// constraints, 3 taxonomies (§10.1).
func TestBundleHasREVocabulary(t *testing.T) {
	var mc metaConfig
	if err := json.Unmarshal(Bundle(), &mc); err != nil {
		t.Fatalf("unmarshal bundle: %v", err)
	}

	wantKinds := map[string]bool{"rfc": true, "milestone": true, "decision_block": true, "outcome_assessment": true, "goal": true}
	gotKinds := map[string]bool{}
	for _, k := range mc.WorkKinds {
		gotKinds[k.Slug] = true
	}
	assertSet(t, "work kinds", gotKinds, wantKinds)

	wantRoles := map[string]bool{"specifier": true, "builder": true, "pilot": true, "reviewer": true, "leadership": true}
	gotRoles := map[string]bool{}
	for _, r := range mc.Roles {
		gotRoles[r.Slug] = true
	}
	assertSet(t, "roles", gotRoles, wantRoles)

	wantConstraints := map[string]bool{"no_role_collapse": true, "pilot_independence": true, "requires_product_classification": true}
	gotConstraints := map[string]bool{}
	for _, c := range mc.RoleConstraints {
		gotConstraints[c.Slug] = true
	}
	assertSet(t, "role constraints", gotConstraints, wantConstraints)

	wantTaxonomies := map[string]bool{"org": true, "product": true, "goals": true}
	gotTaxonomies := map[string]bool{}
	for _, tx := range mc.Taxonomies {
		gotTaxonomies[tx.Slug] = true
		// §10.1: taxonomies ship as empty skeletons; the consumer fills nodes.
		if len(tx.Nodes) != 0 {
			t.Errorf("taxonomy %q should ship empty (got %d nodes)", tx.Slug, len(tx.Nodes))
		}
	}
	assertSet(t, "taxonomies", gotTaxonomies, wantTaxonomies)
}

// TestBundleWorkflowsResolve asserts every work kind's workflow_slug names a
// defined workflow — the cross-reference Validate enforces, checked directly
// here so a regression points at the right place.
func TestBundleWorkflowsResolve(t *testing.T) {
	var mc metaConfig
	if err := json.Unmarshal(Bundle(), &mc); err != nil {
		t.Fatalf("unmarshal bundle: %v", err)
	}
	defined := map[string]bool{}
	for _, wf := range mc.Workflows {
		defined[wf.Slug] = true
	}
	for _, k := range mc.WorkKinds {
		if !defined[k.WorkflowSlug] {
			t.Errorf("work kind %q references undefined workflow %q", k.Slug, k.WorkflowSlug)
		}
	}
}

// assertSet fails if got != want as sets, reporting both directions.
func assertSet(t *testing.T, label string, got, want map[string]bool) {
	t.Helper()
	for w := range want {
		if !got[w] {
			t.Errorf("%s: missing %q", label, w)
		}
	}
	for g := range got {
		if !want[g] {
			t.Errorf("%s: unexpected %q", label, g)
		}
	}
}
