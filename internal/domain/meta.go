package domain

import (
	"encoding/json"
	"fmt"
)

// MetaConfig defines the project's structured vocabulary and coordination policies.
type MetaConfig struct {
	Version    MetaVersion `json:"version"`
	ProjectKey string      `json:"project_key"`

	// Work structure
	WorkKinds  []WorkKind `json:"work_kinds"`
	Workflows  []Workflow `json:"workflows"`
	Priorities []string   `json:"priorities"`
	Labels     []Label    `json:"labels"`

	// Artifact & evidence
	ArtifactTypes []ArtifactType `json:"artifact_types"`
	EvidenceTypes []EvidenceType `json:"evidence_types"`

	// Relations
	RelationTypes []RelationType `json:"relation_types"`

	// Policies
	LeasePolicies  []LeasePolicy  `json:"lease_policies"`
	ReviewPolicies []ReviewPolicy `json:"review_policies"`

	// Classification & role coordination (RE substrate). These are applied by
	// methodology packs (e.g. Pilot), not baked into DefaultMetaConfig.
	Taxonomies      []Taxonomy       `json:"taxonomies,omitempty"`
	Roles           []Role           `json:"roles,omitempty"`
	RoleConstraints []RoleConstraint `json:"role_constraints,omitempty"`
}

type Label struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Color       string `json:"color,omitempty"`
	Description string `json:"description,omitempty"`
}

type Workflow struct {
	Slug        string           `json:"slug"`
	Name        string           `json:"name"`
	Statuses    []WorkflowStatus `json:"statuses"`
	Transitions []Transition     `json:"transitions"`
}

type WorkflowStatus struct {
	Slug     string `json:"slug"`
	Name     string `json:"name"`
	Category string `json:"category"` // "open", "in_progress", "done"
}

type Transition struct {
	From string `json:"from"` // "*" means any
	To   string `json:"to"`
}

type WorkKind struct {
	Slug         string `json:"slug"`
	Name         string `json:"name"`
	WorkflowSlug string `json:"workflow_slug"`
	Description  string `json:"description,omitempty"`
}

type ArtifactType struct {
	Slug      string   `json:"slug"`
	Name      string   `json:"name"`
	MimeTypes []string `json:"mime_types,omitempty"`
}

type EvidenceType struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
}

type RelationType struct {
	Slug    string `json:"slug"`
	Name    string `json:"name"`
	Inverse string `json:"inverse,omitempty"`
}

type LeasePolicy struct {
	WorkKindSlug       string `json:"work_kind_slug"`
	DefaultDurationSec int    `json:"default_duration_sec"`
	MaxDurationSec     int    `json:"max_duration_sec"`
	MaxRenewals        int    `json:"max_renewals"`
}

type ReviewPolicy struct {
	WorkKindSlug    string `json:"work_kind_slug"`
	RequiredReviews int    `json:"required_reviews"`
}

// --- Classification & roles (RE substrate) ---

// Taxonomy is a named hierarchy of nodes that work items can be classified
// into (e.g. an org chart, a product tree, a goal tree).
type Taxonomy struct {
	Slug        string         `json:"slug"`         // e.g. "org"
	Name        string         `json:"name"`         // "Organization"
	Description string         `json:"description,omitempty"`
	Levels      []string       `json:"levels,omitempty"` // advisory naming: ["org", "node", "team"]
	Nodes       []TaxonomyNode `json:"nodes"`
}

// TaxonomyNode is a single node within a taxonomy. Slugs are path-like and
// stable; ParentSlug references another node in the same taxonomy.
type TaxonomyNode struct {
	Slug       string          `json:"slug"` // "acme/platform/payments-team"
	Name       string          `json:"name"`
	ParentSlug string          `json:"parent_slug,omitempty"`
	Metadata   json.RawMessage `json:"metadata,omitempty"`
	Retired    bool            `json:"retired,omitempty"`
}

// Role is a named position an actor can be bound to on a work item.
type Role struct {
	Slug        string `json:"slug"` // "pilot"
	Name        string `json:"name"` // "Pilot"
	Description string `json:"description,omitempty"`
	Cardinality string `json:"cardinality"` // "exactly_one" | "at_most_one" | "many"
}

// RoleConstraint is a coordination policy evaluated against a materialized
// work item's role bindings and classifications. Constraints are advisory:
// they produce diagnostics, never reject events.
type RoleConstraint struct {
	Slug      string         `json:"slug"` // "pilot_independence"
	Name      string         `json:"name"`
	Predicate string         `json:"predicate"` // see internal/constraints
	Args      map[string]any `json:"args,omitempty"`
	Severity  string         `json:"severity"` // "warning" | "violation"
}

// --- Validation ---

func (m *MetaConfig) HasLabel(slug string) bool {
	for _, l := range m.Labels {
		if l.Slug == slug {
			return true
		}
	}
	return false
}

func (m *MetaConfig) HasStatus(status string) bool {
	for _, w := range m.Workflows {
		for _, s := range w.Statuses {
			if s.Slug == status {
				return true
			}
		}
	}
	return false
}

// OpenStatuses returns the deduplicated set of status slugs whose
// category is "open" across all workflows. This is the canonical input
// to IsReady when determining whether a work item is actionable, used
// by the v2 list endpoint and the dits_work_list MCP tool.
func (m *MetaConfig) OpenStatuses() []string {
	if m == nil {
		return nil
	}
	seen := make(map[string]struct{})
	var out []string
	for _, w := range m.Workflows {
		for _, s := range w.Statuses {
			if s.Category != "open" {
				continue
			}
			if _, ok := seen[s.Slug]; ok {
				continue
			}
			seen[s.Slug] = struct{}{}
			out = append(out, s.Slug)
		}
	}
	return out
}

func (m *MetaConfig) HasWorkKind(slug string) bool {
	for _, k := range m.WorkKinds {
		if k.Slug == slug {
			return true
		}
	}
	return false
}

func (m *MetaConfig) HasPriority(priority string) bool {
	for _, p := range m.Priorities {
		if p == priority {
			return true
		}
	}
	return false
}

func (m *MetaConfig) HasArtifactType(slug string) bool {
	for _, t := range m.ArtifactTypes {
		if t.Slug == slug {
			return true
		}
	}
	return false
}

func (m *MetaConfig) HasEvidenceType(slug string) bool {
	for _, t := range m.EvidenceTypes {
		if t.Slug == slug {
			return true
		}
	}
	return false
}

func (m *MetaConfig) HasRelationType(slug string) bool {
	for _, t := range m.RelationTypes {
		if t.Slug == slug {
			return true
		}
	}
	return false
}

func (m *MetaConfig) HasTaxonomy(slug string) bool {
	return m.GetTaxonomy(slug) != nil
}

func (m *MetaConfig) GetTaxonomy(slug string) *Taxonomy {
	for i := range m.Taxonomies {
		if m.Taxonomies[i].Slug == slug {
			return &m.Taxonomies[i]
		}
	}
	return nil
}

func (m *MetaConfig) GetTaxonomyNode(taxSlug, nodeSlug string) *TaxonomyNode {
	tx := m.GetTaxonomy(taxSlug)
	if tx == nil {
		return nil
	}
	for i := range tx.Nodes {
		if tx.Nodes[i].Slug == nodeSlug {
			return &tx.Nodes[i]
		}
	}
	return nil
}

// TaxonomyHasNode reports whether the node exists in the taxonomy,
// regardless of retirement.
func (m *MetaConfig) TaxonomyHasNode(taxSlug, nodeSlug string) bool {
	return m.GetTaxonomyNode(taxSlug, nodeSlug) != nil
}

// TaxonomyHasActiveNode reports whether the node exists and is not retired.
// New classifications must target an active node.
func (m *MetaConfig) TaxonomyHasActiveNode(taxSlug, nodeSlug string) bool {
	n := m.GetTaxonomyNode(taxSlug, nodeSlug)
	return n != nil && !n.Retired
}

func (m *MetaConfig) HasRole(slug string) bool {
	return m.GetRole(slug) != nil
}

func (m *MetaConfig) GetRole(slug string) *Role {
	for i := range m.Roles {
		if m.Roles[i].Slug == slug {
			return &m.Roles[i]
		}
	}
	return nil
}

func (m *MetaConfig) GetWorkflowForKind(kindSlug string) *Workflow {
	wfSlug := ""
	for _, k := range m.WorkKinds {
		if k.Slug == kindSlug {
			wfSlug = k.WorkflowSlug
			break
		}
	}
	if wfSlug == "" {
		return nil
	}
	for i := range m.Workflows {
		if m.Workflows[i].Slug == wfSlug {
			return &m.Workflows[i]
		}
	}
	return nil
}

func (m *MetaConfig) GetLeasePolicy(kindSlug string) *LeasePolicy {
	for i := range m.LeasePolicies {
		if m.LeasePolicies[i].WorkKindSlug == kindSlug {
			return &m.LeasePolicies[i]
		}
	}
	return nil
}

// --- Mutation ---

func (m *MetaConfig) AddLabel(label Label) error {
	if m.HasLabel(label.Slug) {
		return fmt.Errorf("label %q already exists", label.Slug)
	}
	m.Labels = append(m.Labels, label)
	m.Version++
	return nil
}

func (m *MetaConfig) RemoveLabel(slug string) error {
	found := false
	result := make([]Label, 0, len(m.Labels))
	for _, l := range m.Labels {
		if l.Slug == slug {
			found = true
			continue
		}
		result = append(result, l)
	}
	if !found {
		return fmt.Errorf("label %q not found", slug)
	}
	m.Labels = result
	m.Version++
	return nil
}

func (m *MetaConfig) AddWorkKind(wk WorkKind) error {
	if m.HasWorkKind(wk.Slug) {
		return fmt.Errorf("work kind %q already exists", wk.Slug)
	}
	// Verify workflow exists.
	found := false
	for _, w := range m.Workflows {
		if w.Slug == wk.WorkflowSlug {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("workflow %q not found", wk.WorkflowSlug)
	}
	m.WorkKinds = append(m.WorkKinds, wk)
	m.Version++
	return nil
}

func (m *MetaConfig) RemoveWorkKind(slug string) error {
	found := false
	result := make([]WorkKind, 0, len(m.WorkKinds))
	for _, k := range m.WorkKinds {
		if k.Slug == slug {
			found = true
			continue
		}
		result = append(result, k)
	}
	if !found {
		return fmt.Errorf("work kind %q not found", slug)
	}
	m.WorkKinds = result
	m.Version++
	return nil
}

func (m *MetaConfig) AddArtifactType(at ArtifactType) error {
	if m.HasArtifactType(at.Slug) {
		return fmt.Errorf("artifact type %q already exists", at.Slug)
	}
	m.ArtifactTypes = append(m.ArtifactTypes, at)
	m.Version++
	return nil
}

func (m *MetaConfig) AddRelationType(rt RelationType) error {
	if m.HasRelationType(rt.Slug) {
		return fmt.Errorf("relation type %q already exists", rt.Slug)
	}
	m.RelationTypes = append(m.RelationTypes, rt)
	m.Version++
	return nil
}

func (m *MetaConfig) AddTaxonomy(t Taxonomy) error {
	if m.HasTaxonomy(t.Slug) {
		return fmt.Errorf("taxonomy %q already exists", t.Slug)
	}
	m.Taxonomies = append(m.Taxonomies, t)
	m.Version++
	return nil
}

func (m *MetaConfig) AddRole(r Role) error {
	if m.HasRole(r.Slug) {
		return fmt.Errorf("role %q already exists", r.Slug)
	}
	m.Roles = append(m.Roles, r)
	m.Version++
	return nil
}

func (m *MetaConfig) AddRoleConstraint(c RoleConstraint) error {
	for _, existing := range m.RoleConstraints {
		if existing.Slug == c.Slug {
			return fmt.Errorf("role constraint %q already exists", c.Slug)
		}
	}
	m.RoleConstraints = append(m.RoleConstraints, c)
	m.Version++
	return nil
}

// AddTaxonomyNode adds a node to an existing taxonomy. The parent (if given)
// must already exist within the same taxonomy. Bumps Version.
func (m *MetaConfig) AddTaxonomyNode(taxSlug string, node TaxonomyNode) error {
	tx := m.GetTaxonomy(taxSlug)
	if tx == nil {
		return fmt.Errorf("taxonomy %q not found", taxSlug)
	}
	if m.TaxonomyHasNode(taxSlug, node.Slug) {
		return fmt.Errorf("node %q already exists in taxonomy %q", node.Slug, taxSlug)
	}
	if node.ParentSlug != "" && !m.TaxonomyHasNode(taxSlug, node.ParentSlug) {
		return fmt.Errorf("parent node %q not found in taxonomy %q", node.ParentSlug, taxSlug)
	}
	tx.Nodes = append(tx.Nodes, node)
	m.Version++
	return nil
}

// MoveTaxonomyNode re-parents an existing node. The new parent must exist and
// must not be the node itself. Bumps Version.
func (m *MetaConfig) MoveTaxonomyNode(taxSlug, slug, newParentSlug string) error {
	tx := m.GetTaxonomy(taxSlug)
	if tx == nil {
		return fmt.Errorf("taxonomy %q not found", taxSlug)
	}
	if slug == newParentSlug {
		return fmt.Errorf("node %q cannot be its own parent", slug)
	}
	if newParentSlug != "" && !m.TaxonomyHasNode(taxSlug, newParentSlug) {
		return fmt.Errorf("parent node %q not found in taxonomy %q", newParentSlug, taxSlug)
	}
	for i := range tx.Nodes {
		if tx.Nodes[i].Slug == slug {
			tx.Nodes[i].ParentSlug = newParentSlug
			m.Version++
			return nil
		}
	}
	return fmt.Errorf("node %q not found in taxonomy %q", slug, taxSlug)
}

// RetireTaxonomyNode marks a node retired so it can no longer be the target of
// new classifications (existing classifications and declassify still resolve).
// Bumps Version.
func (m *MetaConfig) RetireTaxonomyNode(taxSlug, slug string) error {
	tx := m.GetTaxonomy(taxSlug)
	if tx == nil {
		return fmt.Errorf("taxonomy %q not found", taxSlug)
	}
	for i := range tx.Nodes {
		if tx.Nodes[i].Slug == slug {
			tx.Nodes[i].Retired = true
			m.Version++
			return nil
		}
	}
	return fmt.Errorf("node %q not found in taxonomy %q", slug, taxSlug)
}

func DefaultMetaConfig(projectKey string) MetaConfig {
	return MetaConfig{
		Version:    1,
		ProjectKey: projectKey,
		WorkKinds: []WorkKind{
			{Slug: "task", Name: "Task", WorkflowSlug: "default"},
			{Slug: "issue", Name: "Issue", WorkflowSlug: "default"},
			{Slug: "investigation", Name: "Investigation", WorkflowSlug: "default"},
			{Slug: "plan", Name: "Plan", WorkflowSlug: "default"},
			{Slug: "decision", Name: "Decision", WorkflowSlug: "default"},
			{Slug: "execution", Name: "Execution", WorkflowSlug: "execution"},
			{Slug: "handoff", Name: "Handoff", WorkflowSlug: "default"},
			{Slug: "eval", Name: "Eval", WorkflowSlug: "default"},
		},
		Workflows: []Workflow{
			{
				Slug: "default",
				Name: "Default",
				Statuses: []WorkflowStatus{
					{Slug: "open", Name: "Open", Category: "open"},
					{Slug: "in_progress", Name: "In Progress", Category: "in_progress"},
					{Slug: "closed", Name: "Closed", Category: "done"},
				},
				Transitions: []Transition{
					{From: "*", To: "open"},
					{From: "*", To: "in_progress"},
					{From: "*", To: "closed"},
				},
			},
			{
				Slug: "execution",
				Name: "Execution",
				Statuses: []WorkflowStatus{
					{Slug: "pending", Name: "Pending", Category: "open"},
					{Slug: "active", Name: "Active", Category: "in_progress"},
					{Slug: "completed", Name: "Completed", Category: "done"},
					{Slug: "failed", Name: "Failed", Category: "done"},
				},
				Transitions: []Transition{
					{From: "*", To: "pending"},
					{From: "*", To: "active"},
					{From: "*", To: "completed"},
					{From: "*", To: "failed"},
				},
			},
			{
				Slug: "review",
				Name: "Review",
				Statuses: []WorkflowStatus{
					{Slug: "pending_review", Name: "Pending Review", Category: "open"},
					{Slug: "in_review", Name: "In Review", Category: "in_progress"},
					{Slug: "approved", Name: "Approved", Category: "done"},
					{Slug: "changes_requested", Name: "Changes Requested", Category: "in_progress"},
					{Slug: "rejected", Name: "Rejected", Category: "done"},
				},
				Transitions: []Transition{
					{From: "*", To: "pending_review"},
					{From: "*", To: "in_review"},
					{From: "*", To: "approved"},
					{From: "*", To: "changes_requested"},
					{From: "*", To: "rejected"},
				},
			},
		},
		Priorities: []string{"low", "medium", "high", "critical"},
		Labels:     []Label{},
		ArtifactTypes: []ArtifactType{
			{Slug: "log", Name: "Log"},
			{Slug: "patch", Name: "Patch"},
			{Slug: "screenshot", Name: "Screenshot"},
			{Slug: "report", Name: "Report"},
			{Slug: "json_output", Name: "JSON Output"},
			{Slug: "trace", Name: "Trace"},
			{Slug: "plan", Name: "Plan"},
			{Slug: "model_response", Name: "Model Response"},
		},
		EvidenceTypes: []EvidenceType{
			{Slug: "observation", Name: "Observation"},
			{Slug: "measurement", Name: "Measurement"},
			{Slug: "test_result", Name: "Test Result"},
			{Slug: "log_analysis", Name: "Log Analysis"},
		},
		RelationTypes: []RelationType{
			{Slug: "blocks", Name: "Blocks", Inverse: "blocked_by"},
			{Slug: "blocked_by", Name: "Blocked by", Inverse: "blocks"},
			{Slug: "depends_on", Name: "Depends on", Inverse: "depended_on_by"},
			{Slug: "parent_of", Name: "Parent of", Inverse: "child_of"},
			{Slug: "child_of", Name: "Child of", Inverse: "parent_of"},
			{Slug: "relates_to", Name: "Relates to", Inverse: "relates_to"},
			{Slug: "duplicates", Name: "Duplicates", Inverse: "duplicated_by"},
			{Slug: "derived_from", Name: "Derived from", Inverse: "derived_into"},
			{Slug: "supersedes", Name: "Supersedes", Inverse: "superseded_by"},
		},
		LeasePolicies: []LeasePolicy{
			{WorkKindSlug: "execution", DefaultDurationSec: 300, MaxDurationSec: 3600, MaxRenewals: 10},
		},
		ReviewPolicies: []ReviewPolicy{},
	}
}
