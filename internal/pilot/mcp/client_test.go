package mcp

import (
	"testing"
)

// TestAckRollup pins the Pilot-side rollup to the same tokens DITS-core's
// domain.ComputeAckRollup produces, across the full state matrix.
func TestAckRollup(t *testing.T) {
	cases := []struct {
		specifier, builder, want string
	}{
		{"accepted", "accepted", "aligned"},
		{"pending", "pending", "both_pending"},
		{"accepted", "pending", "builder_pending"},
		{"pending", "accepted", "specifier_pending"},
		{"rejected", "accepted", "rejected"},
		{"accepted", "rejected", "rejected"},
		{"rejected", "pending", "rejected"},
		{"", "", "both_pending"}, // unset behaves as not-accepted
		{"accepted", "", "builder_pending"},
	}
	for _, c := range cases {
		if got := AckRollup(c.specifier, c.builder); got != c.want {
			t.Errorf("AckRollup(%q,%q) = %q, want %q", c.specifier, c.builder, got, c.want)
		}
	}
}

// TestAckRollupMethod confirms the Ack.Rollup convenience matches AckRollup.
func TestAckRollupMethod(t *testing.T) {
	a := Ack{Specifier: "accepted", Builder: "accepted"}
	if a.Rollup() != "aligned" {
		t.Fatalf("Ack.Rollup() = %q, want aligned", a.Rollup())
	}
}

// workShowJSON is a realistic dits_work_show result: the DITS domain.WorkItem
// serialized with its Go field names (no json tags on the core struct), trimmed
// to the fields Pilot's views read.
const workShowJSON = `{
  "ID": "wrk_01HQ",
  "SharedID": "PROJ-176",
  "Kind": "task",
  "Title": "Pilot independence enforcement",
  "Body": "",
  "Status": "open",
  "Priority": "high",
  "Blocked": false,
  "Classifications": [
    {"TaxonomySlug": "product", "NodeSlug": "commerce/checkout"}
  ],
  "RoleBindings": [
    {"RoleSlug": "specifier", "Actor": "ejackson"},
    {"RoleSlug": "builder", "Actor": "tpark"},
    {"RoleSlug": "pilot", "Actor": "kokafor"}
  ],
  "Acks": [
    {"AckID": "ack_1", "ScopeSummary": "Enforce independence", "DeliveryTiming": "2026 Q3",
     "TargetOutcome": "GA", "AcceptanceCriteria": "tests pass",
     "Specifier": "accepted", "Builder": "pending", "FiledBy": "ejackson"}
  ],
  "Diagnostics": [
    {"ConstraintSlug": "pilot_independence", "Severity": "warning", "Message": "pilot reports to builder (1 level)"}
  ]
}`

func TestDecodeWorkItem(t *testing.T) {
	var wi WorkItem
	if err := decodeResult(workShowJSON, &wi); err != nil {
		t.Fatalf("decodeResult: %v", err)
	}
	if wi.ID != "wrk_01HQ" || wi.SharedID != "PROJ-176" || wi.Title != "Pilot independence enforcement" {
		t.Fatalf("scalar fields wrong: %+v", wi)
	}
	if len(wi.Classifications) != 1 || wi.Classifications[0].NodeSlug != "commerce/checkout" {
		t.Fatalf("classifications wrong: %+v", wi.Classifications)
	}
	if len(wi.RoleBindings) != 3 || wi.RoleBindings[2].RoleSlug != "pilot" || wi.RoleBindings[2].Actor != "kokafor" {
		t.Fatalf("role bindings wrong: %+v", wi.RoleBindings)
	}
	if len(wi.Acks) != 1 {
		t.Fatalf("expected 1 ack, got %d", len(wi.Acks))
	}
	if wi.Acks[0].ScopeSummary != "Enforce independence" {
		t.Fatalf("ack scope wrong: %q", wi.Acks[0].ScopeSummary)
	}
	// Specifier accepted, builder pending -> builder_pending.
	if got := wi.Acks[0].Rollup(); got != "builder_pending" {
		t.Fatalf("ack rollup = %q, want builder_pending", got)
	}
	if len(wi.Diagnostics) != 1 || wi.Diagnostics[0].ConstraintSlug != "pilot_independence" {
		t.Fatalf("diagnostics wrong: %+v", wi.Diagnostics)
	}
}

// projectionJSON is a realistic dits_work_show result carrying the Track-0
// generic projection state: scalar Fields, a staged timeline, typed
// observations (status/risk/next), and a derived_from lineage relation.
const projectionJSON = `{
  "ID": "wrk_01HQ",
  "SharedID": "PROJ-204",
  "Kind": "task",
  "Title": "Recurring billing v1",
  "Status": "open",
  "Fields": {"ryg": "g", "target": "2026 Q3", "target_precision": "D", "customer_visible": "true"},
  "Stages": [
    {"Key": "beta", "Label": "Beta", "Date": "2026 Q3", "Precision": "Q", "State": "open"},
    {"Key": "ga", "Label": "GA", "Date": "2026-09-30", "Precision": "D", "State": "open"}
  ],
  "Observations": [
    {"Summary": "Older status.", "Data": {"entry_type": "status"}, "Timestamp": "2026-05-18T09:00:00Z"},
    {"Summary": "Webhook IPv6 routes flaky.", "Data": {"entry_type": "risk", "severity": "medium", "by": "krivas"}, "Timestamp": "2026-05-18T10:00:00Z"},
    {"Summary": "Cut Beta tag.", "Data": {"entry_type": "next", "owner": "dnasser"}, "Timestamp": "2026-05-19T08:00:00Z"},
    {"Summary": "Two of three subsystems integrated.", "Data": {"entry_type": "status"}, "Timestamp": "2026-05-22T16:00:00Z"}
  ],
  "Relations": [
    {"Type": "derived_from", "TargetWorkItem": "wrk_RFC"}
  ]
}`

func TestDecodeProjection(t *testing.T) {
	var wi WorkItem
	if err := decodeResult(projectionJSON, &wi); err != nil {
		t.Fatalf("decodeResult: %v", err)
	}
	if wi.RYG() != "g" {
		t.Errorf("RYG() = %q, want g", wi.RYG())
	}
	if wi.Target() != "2026 Q3" {
		t.Errorf("Target() = %q, want 2026 Q3", wi.Target())
	}
	if wi.TargetPrecision() != "D" {
		t.Errorf("TargetPrecision() = %q, want D", wi.TargetPrecision())
	}
	if !wi.CustomerVisible() {
		t.Errorf("CustomerVisible() = false, want true")
	}
	if len(wi.Stages) != 2 || wi.Stages[1].Key != "ga" {
		t.Errorf("Stages wrong: %+v", wi.Stages)
	}
	// Latest status observation wins.
	if wi.StatusNarrative() != "Two of three subsystems integrated." {
		t.Errorf("StatusNarrative() = %q", wi.StatusNarrative())
	}
	if wi.StatusUpdatedAt() != "2026-05-22T16:00:00Z" {
		t.Errorf("StatusUpdatedAt() = %q", wi.StatusUpdatedAt())
	}
	risks := wi.Risks()
	if len(risks) != 1 || risks[0].Severity != "medium" || risks[0].By != "krivas" || risks[0].When != "2026-05-18" {
		t.Errorf("Risks() wrong: %+v", risks)
	}
	next := wi.NextSteps()
	if len(next) != 1 || next[0].Owner != "dnasser" || next[0].Body != "Cut Beta tag." {
		t.Errorf("NextSteps() wrong: %+v", next)
	}
	if wi.FromRFC() != "wrk_RFC" {
		t.Errorf("FromRFC() = %q, want wrk_RFC", wi.FromRFC())
	}
}

// TestProjectionDefaults confirms a bare work item derives sane defaults.
func TestProjectionDefaults(t *testing.T) {
	var wi WorkItem
	if err := decodeResult(`{"ID":"x","Kind":"task"}`, &wi); err != nil {
		t.Fatalf("decodeResult: %v", err)
	}
	if wi.RYG() != "" || wi.Target() != "" || wi.CustomerVisible() {
		t.Errorf("expected empty scalars, got ryg=%q target=%q vis=%v", wi.RYG(), wi.Target(), wi.CustomerVisible())
	}
	if wi.TargetPrecision() != "Q" {
		t.Errorf("TargetPrecision() default = %q, want Q", wi.TargetPrecision())
	}
	if wi.StatusNarrative() != "" || len(wi.Risks()) != 0 || len(wi.NextSteps()) != 0 || wi.FromRFC() != "" {
		t.Errorf("expected empty derived collections")
	}
}

// roleBindingsJSON is a realistic dits_role_bindings_list result (a bare array).
const roleBindingsJSON = `[
  {"RoleSlug": "specifier", "Actor": "ejackson"},
  {"RoleSlug": "builder", "Actor": "tpark"}
]`

func TestDecodeRoleBindings(t *testing.T) {
	var rbs []RoleBinding
	if err := decodeResult(roleBindingsJSON, &rbs); err != nil {
		t.Fatalf("decodeResult: %v", err)
	}
	if len(rbs) != 2 || rbs[0].RoleSlug != "specifier" || rbs[1].Actor != "tpark" {
		t.Fatalf("role bindings wrong: %+v", rbs)
	}
}

// diagnosticsJSON is a realistic dits_diagnostics_get result.
const diagnosticsJSON = `[
  {"ConstraintSlug": "no_role_collapse", "Severity": "violation", "Message": "roles specifier and builder share actor tpark"},
  {"ConstraintSlug": "requires_product_classification", "Severity": "warning", "Message": "work item is not classified in taxonomy \"product\""}
]`

func TestDecodeDiagnostics(t *testing.T) {
	var diags []Diagnostic
	if err := decodeResult(diagnosticsJSON, &diags); err != nil {
		t.Fatalf("decodeResult: %v", err)
	}
	if len(diags) != 2 {
		t.Fatalf("expected 2 diagnostics, got %d", len(diags))
	}
	if diags[0].Severity != "violation" || diags[1].ConstraintSlug != "requires_product_classification" {
		t.Fatalf("diagnostics wrong: %+v", diags)
	}
}

// metaShowJSON is a realistic dits_meta_show result (snake_case tags on core).
const metaShowJSON = `{
  "version": 3,
  "project_key": "PROJ",
  "taxonomies": [
    {"slug": "product", "name": "Product", "levels": ["group", "product"],
     "nodes": [
       {"slug": "commerce", "name": "Commerce"},
       {"slug": "commerce/checkout", "name": "Checkout", "parent_slug": "commerce"}
     ]}
  ],
  "roles": [
    {"slug": "pilot", "name": "Pilot", "cardinality": "exactly_one"}
  ],
  "role_constraints": [
    {"slug": "pilot_independence", "name": "Pilot independence", "predicate": "not_reports_to_within",
     "args": {"role": "pilot", "max_levels": 2}, "severity": "warning"}
  ]
}`

func TestDecodeMeta(t *testing.T) {
	var m Meta
	if err := decodeResult(metaShowJSON, &m); err != nil {
		t.Fatalf("decodeResult: %v", err)
	}
	if m.Version != 3 || m.ProjectKey != "PROJ" {
		t.Fatalf("meta scalars wrong: %+v", m)
	}
	if len(m.Taxonomies) != 1 || len(m.Taxonomies[0].Nodes) != 2 {
		t.Fatalf("taxonomies wrong: %+v", m.Taxonomies)
	}
	if m.Taxonomies[0].Nodes[1].ParentSlug != "commerce" {
		t.Fatalf("node parent wrong: %+v", m.Taxonomies[0].Nodes[1])
	}
	if len(m.Roles) != 1 || m.Roles[0].Cardinality != "exactly_one" {
		t.Fatalf("roles wrong: %+v", m.Roles)
	}
	if len(m.RoleConstraints) != 1 || m.RoleConstraints[0].Predicate != "not_reports_to_within" {
		t.Fatalf("constraints wrong: %+v", m.RoleConstraints)
	}
	if got := m.RoleConstraints[0].Args["max_levels"]; got != float64(2) {
		t.Fatalf("constraint arg max_levels = %v (%T), want 2", got, got)
	}
}

// actorListJSON is a realistic dits_actor_list result.
const actorListJSON = `[
  {"actor_id": "kokafor", "public_key": "ed25519:deadbeef", "node_id": "node-1", "first_seen": "2026-05-29T10:00:00Z"}
]`

func TestDecodeActorList(t *testing.T) {
	var actors []ActorRecord
	if err := decodeResult(actorListJSON, &actors); err != nil {
		t.Fatalf("decodeResult: %v", err)
	}
	if len(actors) != 1 || actors[0].ActorID != "kokafor" || actors[0].NodeID != "node-1" {
		t.Fatalf("actors wrong: %+v", actors)
	}
}

// eventsListJSON is a realistic dits_events_list result (snake_case tags).
const eventsListJSON = `[
  {"id": "evt_1", "work_item_id": "wrk_01HQ", "type": "work.role_bound", "actor_id": "ejackson",
   "timestamp": "2026-05-29T10:00:00Z", "payload": {"role_slug": "pilot", "actor_id": "kokafor"}}
]`

func TestDecodeEvents(t *testing.T) {
	var events []Event
	if err := decodeResult(eventsListJSON, &events); err != nil {
		t.Fatalf("decodeResult: %v", err)
	}
	if len(events) != 1 || events[0].Type != "work.role_bound" || events[0].WorkItemID != "wrk_01HQ" {
		t.Fatalf("events wrong: %+v", events)
	}
	if len(events[0].Payload) == 0 {
		t.Fatalf("expected raw payload preserved")
	}
}

// TestDecodeEmpty confirms an empty result string is a no-op (not an error).
func TestDecodeEmpty(t *testing.T) {
	var wi WorkItem
	if err := decodeResult("", &wi); err != nil {
		t.Fatalf("empty decode should be a no-op, got %v", err)
	}
	if err := decodeResult("   ", &wi); err != nil {
		t.Fatalf("whitespace decode should be a no-op, got %v", err)
	}
}

// TestFiltersArgs verifies zero-valued filter fields are omitted and set ones
// map to the dits_work_list argument names.
func TestFiltersArgs(t *testing.T) {
	got := Filters{Role: "pilot", RoleActor: "kokafor", Taxonomy: "product", Node: "commerce", Limit: 10}.args()
	want := map[string]any{
		"role": "pilot", "role_actor": "kokafor", "taxonomy": "product", "node": "commerce", "limit": 10,
	}
	if len(got) != len(want) {
		t.Fatalf("args length: got %d want %d (%v)", len(got), len(want), got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("args[%q] = %v, want %v", k, got[k], v)
		}
	}
	// Empty filters yield no args.
	if n := len(Filters{}.args()); n != 0 {
		t.Fatalf("empty filters should yield no args, got %d", n)
	}
}

// TestStubClient confirms the stub satisfies Client, returns empty queries, and
// ErrNotImplemented on mutations.
func TestStubClient(t *testing.T) {
	var c Client = NewStub()
	defer c.Close()

	if items, err := c.WorkList(t.Context(), Filters{}); err != nil || items != nil {
		t.Fatalf("stub WorkList: items=%v err=%v", items, err)
	}
	if err := c.BindRole(t.Context(), "wrk", "pilot", "kokafor"); err != ErrNotImplemented {
		t.Fatalf("stub BindRole: want ErrNotImplemented, got %v", err)
	}
	if err := c.MetaApply(t.Context(), []byte(`{}`)); err != ErrNotImplemented {
		t.Fatalf("stub MetaApply: want ErrNotImplemented, got %v", err)
	}
}
