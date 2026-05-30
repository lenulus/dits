// Package meta holds the Radical Execution methodology as data: the
// re_default.json bundle (work kinds, workflows, roles, role constraints,
// and empty taxonomy skeletons) plus an embedded loader and validator. See
// implementation-plan-v2 §10.1.
//
// The bundle is shaped as a partial DITS MetaConfig — the exact JSON tags
// from internal/domain/meta.go (work_kinds, workflows, roles,
// role_constraints, taxonomies). Pilot's first-run flow loads it into a
// target project's meta via the dits_meta_apply MCP tool (idempotent: apply
// diffs only). The Pilot tree must NOT import internal/domain (the MCP-only
// boundary, plan §2/§8.1), so Validate unmarshals into a small local mirror
// of the relevant MetaConfig shape rather than the domain types.
package meta

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:embed re_default.json
var reDefault []byte

// Bundle returns the raw RE meta bundle JSON, ready to hand to the
// dits_meta_apply MCP tool. The returned slice is the embedded bytes; treat
// it as read-only.
func Bundle() []byte { return reDefault }

// --- Local mirror of the relevant MetaConfig shape ---
//
// These types mirror the JSON tags of the corresponding domain types
// (internal/domain/meta.go) closely enough to validate the bundle's
// structure and cross-references. They are intentionally a subset — only
// what the RE bundle populates.

type metaConfig struct {
	WorkKinds       []workKind       `json:"work_kinds"`
	Workflows       []workflow       `json:"workflows"`
	Roles           []role           `json:"roles"`
	RoleConstraints []roleConstraint `json:"role_constraints"`
	Taxonomies      []taxonomy       `json:"taxonomies"`
}

type workKind struct {
	Slug         string `json:"slug"`
	Name         string `json:"name"`
	WorkflowSlug string `json:"workflow_slug"`
}

type workflow struct {
	Slug        string           `json:"slug"`
	Name        string           `json:"name"`
	Statuses    []workflowStatus `json:"statuses"`
	Transitions []transition     `json:"transitions"`
}

type workflowStatus struct {
	Slug     string `json:"slug"`
	Name     string `json:"name"`
	Category string `json:"category"`
}

type transition struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type role struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Cardinality string `json:"cardinality"`
}

type roleConstraint struct {
	Slug      string         `json:"slug"`
	Name      string         `json:"name"`
	Predicate string         `json:"predicate"`
	Args      map[string]any `json:"args"`
	Severity  string         `json:"severity"`
}

type taxonomy struct {
	Slug   string   `json:"slug"`
	Name   string   `json:"name"`
	Levels []string `json:"levels"`
	Nodes  []any    `json:"nodes"`
}

// expected RE vocabulary — the contract the bundle must satisfy.
var (
	expectedWorkKinds   = []string{"rfc", "milestone", "decision_block", "outcome_assessment", "goal"}
	expectedRoles       = []string{"specifier", "builder", "pilot", "reviewer", "leadership"}
	expectedConstraints = []string{"no_role_collapse", "pilot_independence", "requires_product_classification"}
	expectedTaxonomies  = []string{"org", "product", "goals"}

	validCardinalities = map[string]bool{"exactly_one": true, "at_most_one": true, "many": true}
	validCategories    = map[string]bool{"open": true, "in_progress": true, "done": true}
	validSeverities    = map[string]bool{"warning": true, "violation": true}
)

// Validate parses the embedded bundle and asserts it is structurally sound
// and carries the full RE vocabulary:
//   - valid JSON unmarshaling into the MetaConfig mirror;
//   - every expected work kind, role, constraint, and taxonomy is present;
//   - each work kind's workflow_slug resolves to a defined workflow;
//   - workflow statuses use known categories and each transition's from/to
//     (other than the "*" wildcard) names a defined status;
//   - role cardinalities and constraint severities are from the known sets.
//
// It returns the first problem found, or nil when the bundle is valid.
func Validate() error {
	if !json.Valid(reDefault) {
		return fmt.Errorf("re_default.json is not valid JSON")
	}
	var mc metaConfig
	if err := json.Unmarshal(reDefault, &mc); err != nil {
		return fmt.Errorf("unmarshal re_default.json: %w", err)
	}

	// Index workflows by slug, validating their internals as we go.
	workflows := make(map[string]workflow, len(mc.Workflows))
	for _, wf := range mc.Workflows {
		if wf.Slug == "" {
			return fmt.Errorf("workflow with empty slug")
		}
		if _, dup := workflows[wf.Slug]; dup {
			return fmt.Errorf("duplicate workflow %q", wf.Slug)
		}
		if len(wf.Statuses) == 0 {
			return fmt.Errorf("workflow %q has no statuses", wf.Slug)
		}
		statuses := make(map[string]bool, len(wf.Statuses))
		for _, st := range wf.Statuses {
			if st.Slug == "" {
				return fmt.Errorf("workflow %q has a status with empty slug", wf.Slug)
			}
			if !validCategories[st.Category] {
				return fmt.Errorf("workflow %q status %q has invalid category %q", wf.Slug, st.Slug, st.Category)
			}
			statuses[st.Slug] = true
		}
		for _, tr := range wf.Transitions {
			if tr.From != "*" && !statuses[tr.From] {
				return fmt.Errorf("workflow %q transition from unknown status %q", wf.Slug, tr.From)
			}
			if tr.To != "*" && !statuses[tr.To] {
				return fmt.Errorf("workflow %q transition to unknown status %q", wf.Slug, tr.To)
			}
		}
		workflows[wf.Slug] = wf
	}

	// Work kinds: present + each workflow_slug resolves.
	kinds := make(map[string]bool, len(mc.WorkKinds))
	for _, k := range mc.WorkKinds {
		if k.Slug == "" {
			return fmt.Errorf("work kind with empty slug")
		}
		if _, ok := workflows[k.WorkflowSlug]; !ok {
			return fmt.Errorf("work kind %q references undefined workflow %q", k.Slug, k.WorkflowSlug)
		}
		kinds[k.Slug] = true
	}
	if err := requireAll("work kind", kinds, expectedWorkKinds); err != nil {
		return err
	}

	// Roles: present + valid cardinality.
	roles := make(map[string]bool, len(mc.Roles))
	for _, r := range mc.Roles {
		if !validCardinalities[r.Cardinality] {
			return fmt.Errorf("role %q has invalid cardinality %q", r.Slug, r.Cardinality)
		}
		roles[r.Slug] = true
	}
	if err := requireAll("role", roles, expectedRoles); err != nil {
		return err
	}

	// Role constraints: present + valid severity + non-empty predicate.
	constraints := make(map[string]bool, len(mc.RoleConstraints))
	for _, c := range mc.RoleConstraints {
		if c.Predicate == "" {
			return fmt.Errorf("role constraint %q has empty predicate", c.Slug)
		}
		if !validSeverities[c.Severity] {
			return fmt.Errorf("role constraint %q has invalid severity %q", c.Slug, c.Severity)
		}
		constraints[c.Slug] = true
	}
	if err := requireAll("role constraint", constraints, expectedConstraints); err != nil {
		return err
	}

	// Taxonomies: present as (empty) skeletons.
	taxonomies := make(map[string]bool, len(mc.Taxonomies))
	for _, t := range mc.Taxonomies {
		taxonomies[t.Slug] = true
	}
	if err := requireAll("taxonomy", taxonomies, expectedTaxonomies); err != nil {
		return err
	}

	return nil
}

// requireAll asserts every want is a key in have.
func requireAll(label string, have map[string]bool, want []string) error {
	for _, w := range want {
		if !have[w] {
			return fmt.Errorf("%s %q missing from RE bundle", label, w)
		}
	}
	return nil
}
