// Package constraints evaluates a project's role constraints against a
// materialized work item. It is a pure function of (work item, meta config,
// actor positions): given those inputs it returns a deterministic list of
// diagnostics. Constraints are advisory — they never reject events.
//
// The substrate has no actor→taxonomy-node store yet, so the org position of
// each actor is supplied as a parameter rather than read from state. This
// keeps the package pure and unit-testable and defers the persistence design.
package constraints

import (
	"fmt"

	"github.com/lenulus/pf/internal/domain"
)

// ActorPositions maps an actor to the taxonomy node slug they occupy. Used by
// hierarchy-aware predicates (e.g. not_reports_to_within). Today this is the
// org-taxonomy position; the map keys the node slug per actor.
type ActorPositions map[domain.ActorID]string

// Evaluate runs every role constraint in meta against the work item and
// returns the diagnostics produced. A nil meta or work item yields no
// diagnostics. The result order follows meta.RoleConstraints.
func Evaluate(wi *domain.WorkItem, meta *domain.MetaConfig, pos ActorPositions) []domain.Diagnostic {
	if wi == nil || meta == nil {
		return nil
	}
	var diags []domain.Diagnostic
	for _, c := range meta.RoleConstraints {
		switch c.Predicate {
		case "distinct_actors":
			diags = appendNonNil(diags, evalDistinctActors(wi, c))
		case "requires_classification":
			diags = appendNonNil(diags, evalRequiresClassification(wi, c))
		case "not_reports_to_within":
			diags = appendNonNil(diags, evalNotReportsToWithin(wi, meta, pos, c))
		case "classified_in_same_node":
			diags = appendNonNil(diags, evalClassifiedInSameNode(wi, pos, c))
		}
	}
	return diags
}

// --- Predicates ---

// distinct_actors: the actors bound to the listed roles must be distinct.
func evalDistinctActors(wi *domain.WorkItem, c domain.RoleConstraint) *domain.Diagnostic {
	roles := argStrings(c.Args, "roles")
	seen := make(map[domain.ActorID]string)
	for _, role := range roles {
		actor := boundActor(wi, role)
		if actor == "" {
			continue
		}
		if other, ok := seen[actor]; ok {
			return diag(c, fmt.Sprintf("roles %s and %s share actor %s", other, role, actor))
		}
		seen[actor] = role
	}
	return nil
}

// requires_classification: the work item must carry at least one
// classification in the named taxonomy.
func evalRequiresClassification(wi *domain.WorkItem, c domain.RoleConstraint) *domain.Diagnostic {
	taxonomy := argString(c.Args, "taxonomy")
	if taxonomy == "" {
		return nil
	}
	for _, cl := range wi.Classifications {
		if cl.TaxonomySlug == taxonomy {
			return nil
		}
	}
	return diag(c, fmt.Sprintf("work item is not classified in taxonomy %q", taxonomy))
}

// not_reports_to_within: the actor bound to `role` must not report to the
// actors bound to `other_roles` within `max_levels` of `taxonomy`. "Reports
// to X within N" means X's node is an ancestor of the role-actor's node,
// reachable by walking ParentSlug up to N hops.
func evalNotReportsToWithin(wi *domain.WorkItem, meta *domain.MetaConfig, pos ActorPositions, c domain.RoleConstraint) *domain.Diagnostic {
	role := argString(c.Args, "role")
	otherRoles := argStrings(c.Args, "other_roles")
	taxonomy := argString(c.Args, "taxonomy")
	maxLevels := argInt(c.Args, "max_levels")
	if role == "" || taxonomy == "" || maxLevels <= 0 {
		return nil
	}
	tx := meta.GetTaxonomy(taxonomy)
	if tx == nil {
		return nil
	}
	actor := boundActor(wi, role)
	if actor == "" {
		return nil
	}
	start := pos[actor]
	if start == "" {
		return nil
	}
	for _, other := range otherRoles {
		oActor := boundActor(wi, other)
		if oActor == "" {
			continue
		}
		oNode := pos[oActor]
		if oNode == "" {
			continue
		}
		if levels := ancestorWithin(tx, start, oNode, maxLevels); levels > 0 {
			return diag(c, fmt.Sprintf("%s reports to %s (%d level%s)", role, other, levels, plural(levels)))
		}
	}
	return nil
}

// classified_in_same_node: the role-actor's position node must equal the work
// item's classification node in the named taxonomy. If either input is
// absent, the predicate does not fire.
func evalClassifiedInSameNode(wi *domain.WorkItem, pos ActorPositions, c domain.RoleConstraint) *domain.Diagnostic {
	taxonomy := argString(c.Args, "taxonomy")
	role := argString(c.Args, "role")
	if taxonomy == "" || role == "" {
		return nil
	}
	actor := boundActor(wi, role)
	if actor == "" {
		return nil
	}
	actorNode := pos[actor]
	if actorNode == "" {
		return nil
	}
	var wiNode string
	for _, cl := range wi.Classifications {
		if cl.TaxonomySlug == taxonomy {
			wiNode = cl.NodeSlug
			break
		}
	}
	if wiNode == "" {
		return nil
	}
	if actorNode != wiNode {
		return diag(c, fmt.Sprintf("%s is positioned in %q but work item is classified in %q", role, actorNode, wiNode))
	}
	return nil
}

// --- Helpers ---

// ancestorWithin returns the number of hops (1..maxLevels) from `start` up to
// `ancestor` by walking ParentSlug, or 0 if `ancestor` is not within
// maxLevels above `start`.
func ancestorWithin(tx *domain.Taxonomy, start, ancestor string, maxLevels int) int {
	node := findNode(tx, start)
	for hops := 1; node != nil && hops <= maxLevels; hops++ {
		if node.ParentSlug == "" {
			return 0
		}
		if node.ParentSlug == ancestor {
			return hops
		}
		node = findNode(tx, node.ParentSlug)
	}
	return 0
}

func findNode(tx *domain.Taxonomy, slug string) *domain.TaxonomyNode {
	for i := range tx.Nodes {
		if tx.Nodes[i].Slug == slug {
			return &tx.Nodes[i]
		}
	}
	return nil
}

// boundActor returns the actor bound to the role on the work item, or "".
func boundActor(wi *domain.WorkItem, role string) domain.ActorID {
	for _, rb := range wi.RoleBindings {
		if rb.RoleSlug == role {
			return rb.Actor
		}
	}
	return ""
}

func diag(c domain.RoleConstraint, msg string) *domain.Diagnostic {
	return &domain.Diagnostic{ConstraintSlug: c.Slug, Severity: c.Severity, Message: msg}
}

func appendNonNil(diags []domain.Diagnostic, d *domain.Diagnostic) []domain.Diagnostic {
	if d != nil {
		diags = append(diags, *d)
	}
	return diags
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// --- Arg readers ---
//
// Constraint args arrive as map[string]any decoded from JSON, so numbers are
// float64 and arrays are []any. These readers normalize that.

func argString(args map[string]any, key string) string {
	if v, ok := args[key].(string); ok {
		return v
	}
	return ""
}

func argStrings(args map[string]any, key string) []string {
	switch v := args[key].(type) {
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, e := range v {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func argInt(args map[string]any, key string) int {
	switch v := args[key].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	}
	return 0
}
