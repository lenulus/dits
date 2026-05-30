// Package mcp is Pilot's typed client wrapper over the DITS MCP tool
// surface. It is the ONLY channel through which Pilot reaches the DITS
// substrate (see implementation-plan-v2 §2 topology and §8.1): Pilot owns no
// store and imports no DITS-core package. Every method maps to exactly one
// dits_* tool call and parses its JSON result text into Pilot-side DTOs.
//
// Transport: the real client launches a separate `dits-mcp` process and speaks
// MCP over its stdio (NOT in-process — that would require importing
// internal/mcp, which Pilot is forbidden from doing). A NewStub() is kept for
// handlers that run with no MCP endpoint configured.
package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	mcpgo "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

// ErrNotImplemented is returned by the stub client whose methods are not wired
// to a live MCP endpoint.
var ErrNotImplemented = errors.New("pilot/mcp: not implemented")

// --- Pilot-side DTOs ---
//
// These mirror the JSON the dits_* tools emit. DITS-core's domain.WorkItem and
// its sub-structs carry NO json tags, so they serialize with their Go field
// names (ID, Title, RoleBindings, ...). The tags below reproduce those exact
// names. Meta, Event, and ActorRecord on the DITS side DO carry snake_case
// tags, reproduced accordingly. Pilot cannot import internal/domain, so these
// are independent types.

// WorkItem is the Pilot-side projection of a DITS work item over MCP.
type WorkItem struct {
	ID              string           `json:"ID"`
	SharedID        string           `json:"SharedID"`
	Kind            string           `json:"Kind"`
	Title           string           `json:"Title"`
	Body            string           `json:"Body"`
	Status          string           `json:"Status"`
	Priority        string           `json:"Priority"`
	Blocked         bool             `json:"Blocked"`
	Classifications []Classification `json:"Classifications"`
	RoleBindings    []RoleBinding    `json:"RoleBindings"`
	Acks            []Ack            `json:"Acks"`
	Diagnostics     []Diagnostic     `json:"Diagnostics"`
	Relations       []Relation       `json:"Relations"`
	// Generic projection state set via work.field_set / work.schedule_set.
	// DITS attaches no meaning to these; Pilot derives the RE methodology
	// surface (RYG, target, risks, …) from them via the accessors below.
	Fields       map[string]string `json:"Fields"`
	Stages       []Stage           `json:"Stages"`
	Observations []Observation     `json:"Observations"`
	// Timestamps are RFC3339 strings (domain.WorkItem has no json tags, so
	// these arrive PascalCase). Empty when the substrate omitted them.
	CreatedAt string `json:"CreatedAt"`
	UpdatedAt string `json:"UpdatedAt"`
}

// Stage is one entry of a work item's staged delivery timeline.
type Stage struct {
	Key       string `json:"Key"`
	Label     string `json:"Label"`
	Date      string `json:"Date"`
	Precision string `json:"Precision"`
	State     string `json:"State"`
}

// Observation is a materialized work.observation_recorded. Data is an opaque
// blob; the RE methodology rides a typed discriminator in it (see ObsData).
type Observation struct {
	Summary   string          `json:"Summary"`
	Data      json.RawMessage `json:"Data"`
	Timestamp string          `json:"Timestamp"`
}

// ObsData is Pilot's convention for the opaque observation Data blob. The
// entry_type discriminator partitions the journal into status / risk / next
// entries. These names live ONLY in Pilot — DITS never interprets them.
type ObsData struct {
	EntryType string `json:"entry_type"` // status | risk | next | (else: a generic log kind)
	Severity  string `json:"severity,omitempty"`
	Owner     string `json:"owner,omitempty"`
	By        string `json:"by,omitempty"`
}

// Risk is a derived high/medium/low concern (entry_type=risk observation).
type Risk struct {
	Body     string
	Severity string
	By       string
	When     string
}

// NextStep is a derived planned action (entry_type=next observation).
type NextStep struct {
	Body  string
	Owner string
	When  string
}

// --- RE methodology projection (derived Pilot-side from Fields/Stages) ---
//
// These accessors interpret the generic substrate projection as the RE
// methodology surface. The field-name conventions (ryg, target, …) live ONLY
// here in Pilot — DITS never sees them. See re-pilot-completion-plan.md §2.

// Field returns the raw scalar projection value for key, or "".
func (w WorkItem) Field(key string) string {
	if w.Fields == nil {
		return ""
	}
	return w.Fields[key]
}

// RYG is the red/yellow/green health call (g | y | r), or "" if unset.
func (w WorkItem) RYG() string { return w.Field("ryg") }

// Target is the delivery target value (free-form: "2026 Q3", "2026-09-30", …).
func (w WorkItem) Target() string { return w.Field("target") }

// TargetPrecision is Q | M | D, defaulting to Q when unset.
func (w WorkItem) TargetPrecision() string {
	if p := w.Field("target_precision"); p != "" {
		return p
	}
	return "Q"
}

// CustomerVisible reports whether this work item is on the public roadmap.
func (w WorkItem) CustomerVisible() bool { return w.Field("customer_visible") == "true" }

// typedObs returns observations whose Data.entry_type matches kind, newest
// first (observations arrive oldest-first in the materialized slice).
func (w WorkItem) typedObs(kind string) []Observation {
	var out []Observation
	for i := len(w.Observations) - 1; i >= 0; i-- {
		o := w.Observations[i]
		var d ObsData
		if len(o.Data) > 0 {
			_ = json.Unmarshal(o.Data, &d)
		}
		if d.EntryType == kind {
			out = append(out, o)
		}
	}
	return out
}

// StatusNarrative is the latest status paragraph (newest entry_type=status
// observation's summary), or "".
func (w WorkItem) StatusNarrative() string {
	if xs := w.typedObs("status"); len(xs) > 0 {
		return xs[0].Summary
	}
	return ""
}

// StatusUpdatedAt is the timestamp (RFC3339) of the latest status observation.
func (w WorkItem) StatusUpdatedAt() string {
	if xs := w.typedObs("status"); len(xs) > 0 {
		return xs[0].Timestamp
	}
	return ""
}

// Risks lists the work item's open risks, newest first.
func (w WorkItem) Risks() []Risk {
	var out []Risk
	for _, o := range w.typedObs("risk") {
		var d ObsData
		_ = json.Unmarshal(o.Data, &d)
		out = append(out, Risk{Body: o.Summary, Severity: d.Severity, By: d.By, When: shortDay(o.Timestamp)})
	}
	return out
}

// NextSteps lists the work item's planned next steps, newest first.
func (w WorkItem) NextSteps() []NextStep {
	var out []NextStep
	for _, o := range w.typedObs("next") {
		var d ObsData
		_ = json.Unmarshal(o.Data, &d)
		out = append(out, NextStep{Body: o.Summary, Owner: d.Owner, When: shortDay(o.Timestamp)})
	}
	return out
}

// shortDay trims an RFC3339 timestamp to its date (YYYY-MM-DD).
func shortDay(ts string) string {
	if len(ts) >= 10 {
		return ts[:10]
	}
	return ts
}

// FromRFC is the originating RFC id (lineage), derived from a derived_from
// relation, or "".
func (w WorkItem) FromRFC() string {
	for _, r := range w.Relations {
		if r.Type == "derived_from" {
			return r.TargetWorkItem
		}
	}
	return ""
}

// Relation links this work item to another (depends_on, relates_to, …).
type Relation struct {
	Type           string `json:"Type"`
	TargetWorkItem string `json:"TargetWorkItem"`
}

// Classification places a work item within a node of a taxonomy.
type Classification struct {
	TaxonomySlug string `json:"TaxonomySlug"`
	NodeSlug     string `json:"NodeSlug"`
}

// RoleBinding binds an actor to a named role on a work item.
type RoleBinding struct {
	RoleSlug string `json:"RoleSlug"`
	Actor    string `json:"Actor"`
}

// Ack is a materialized two-party commitment. Rollup is computed Pilot-side
// from Specifier/Builder via AckRollup.
type Ack struct {
	AckID              string `json:"AckID"`
	ScopeSummary       string `json:"ScopeSummary"`
	DeliveryTiming     string `json:"DeliveryTiming"`
	TargetOutcome      string `json:"TargetOutcome"`
	AcceptanceCriteria string `json:"AcceptanceCriteria"`
	Specifier          string `json:"Specifier"` // pending | accepted | rejected
	Builder            string `json:"Builder"`
	FiledBy            string `json:"FiledBy"`
}

// Rollup derives the alignment of this commitment's two sides.
func (a Ack) Rollup() string { return AckRollup(a.Specifier, a.Builder) }

// Diagnostic is an advisory finding produced by the constraints engine.
type Diagnostic struct {
	ConstraintSlug string `json:"ConstraintSlug"`
	Severity       string `json:"Severity"` // warning | violation
	Message        string `json:"Message"`
}

// Meta is the project's structured vocabulary, as far as Pilot's views need.
type Meta struct {
	Version         int              `json:"version"`
	ProjectKey      string           `json:"project_key"`
	Taxonomies      []Taxonomy       `json:"taxonomies"`
	Roles           []Role           `json:"roles"`
	RoleConstraints []RoleConstraint `json:"role_constraints"`
}

// Taxonomy is a named hierarchy of nodes work items can be classified into.
type Taxonomy struct {
	Slug        string         `json:"slug"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Levels      []string       `json:"levels,omitempty"`
	Nodes       []TaxonomyNode `json:"nodes"`
}

// TaxonomyNode is a single node within a taxonomy.
type TaxonomyNode struct {
	Slug       string          `json:"slug"`
	Name       string          `json:"name"`
	ParentSlug string          `json:"parent_slug,omitempty"`
	Metadata   json.RawMessage `json:"metadata,omitempty"`
	Retired    bool            `json:"retired,omitempty"`
}

// Role is a named position an actor can be bound to on a work item.
type Role struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Cardinality string `json:"cardinality"`
}

// RoleConstraint is a coordination policy evaluated against role bindings.
type RoleConstraint struct {
	Slug      string         `json:"slug"`
	Name      string         `json:"name"`
	Predicate string         `json:"predicate"`
	Args      map[string]any `json:"args,omitempty"`
	Severity  string         `json:"severity"`
}

// ActorRecord is a registered actor's stored identity.
type ActorRecord struct {
	ActorID   string `json:"actor_id"`
	PublicKey string `json:"public_key"`
	NodeID    string `json:"node_id,omitempty"`
	FirstSeen string `json:"first_seen,omitempty"`
}

// Event is a raw event from the DITS event log.
type Event struct {
	ID         string          `json:"id"`
	WorkItemID string          `json:"work_item_id"`
	Type       string          `json:"type"`
	ActorID    string          `json:"actor_id"`
	Timestamp  string          `json:"timestamp"`
	Payload    json.RawMessage `json:"payload"`
}

// Filters captures the optional dits_work_list filter args the views use.
// Zero-value fields are omitted from the tool call.
type Filters struct {
	Kind      string
	Status    string
	Role      string
	RoleActor string
	Taxonomy  string
	Node      string
	Alignment string
	Limit     int
	Offset    int
}

// args renders the filters as a dits_work_list argument map, skipping zero
// values so server-side defaults apply.
func (f Filters) args() map[string]any {
	m := map[string]any{}
	if f.Kind != "" {
		m["kind"] = f.Kind
	}
	if f.Status != "" {
		m["status"] = f.Status
	}
	if f.Role != "" {
		m["role"] = f.Role
	}
	if f.RoleActor != "" {
		m["role_actor"] = f.RoleActor
	}
	if f.Taxonomy != "" {
		m["taxonomy"] = f.Taxonomy
	}
	if f.Node != "" {
		m["node"] = f.Node
	}
	if f.Alignment != "" {
		m["alignment"] = f.Alignment
	}
	if f.Limit > 0 {
		m["limit"] = f.Limit
	}
	if f.Offset > 0 {
		m["offset"] = f.Offset
	}
	return m
}

// AckRollup derives the alignment of a commitment's two sides. Mirrors
// domain.ComputeAckRollup token-for-token; kept Pilot-side because Pilot
// cannot import internal/domain.
func AckRollup(specifier, builder string) string {
	const (
		accepted = "accepted"
		rejected = "rejected"
	)
	if specifier == rejected || builder == rejected {
		return "rejected"
	}
	if specifier == accepted && builder == accepted {
		return "aligned"
	}
	if specifier != accepted && builder != accepted {
		return "both_pending"
	}
	if specifier != accepted {
		return "specifier_pending"
	}
	return "builder_pending"
}

// --- Client interface ---

// Client is the typed surface Pilot code uses to talk to DITS over MCP. Every
// method corresponds to a single dits_* tool call.
type Client interface {
	// Queries.
	WorkList(ctx context.Context, f Filters) ([]WorkItem, error)
	WorkGet(ctx context.Context, id string) (WorkItem, error)
	EventsList(ctx context.Context, eventType, since string, limit int) ([]Event, error)
	RoleBindingsList(ctx context.Context, id string) ([]RoleBinding, error)
	DiagnosticsGet(ctx context.Context, id string) ([]Diagnostic, error)
	MetaGet(ctx context.Context) (Meta, error)
	ActorList(ctx context.Context) ([]ActorRecord, error)

	// Classification & role bindings.
	Classify(ctx context.Context, id, taxonomy, node string) error
	Declassify(ctx context.Context, id, taxonomy, node string) error
	BindRole(ctx context.Context, id, role, actor string) error
	UnbindRole(ctx context.Context, id, role, actor string) error

	// ACK lifecycle.
	AckFile(ctx context.Context, id, scopeSummary, deliveryTiming, targetOutcome, acceptanceCriteria string) error
	AckAccept(ctx context.Context, id, who, note string) error
	AckReject(ctx context.Context, id, who, note string) error
	AckAmend(ctx context.Context, id, amendmentType string, fields []string, reason string) error

	// Generic projection (methodology state). FieldSet writes a scalar
	// projection field (ryg/target/…); ScheduleSet replaces the staged
	// timeline; Observe records a (typed) journal entry — status/risk/next
	// ride here via the opaque data blob.
	FieldSet(ctx context.Context, id, field, value string) error
	ScheduleSet(ctx context.Context, id string, stages []Stage) error
	Observe(ctx context.Context, id, summary string, data json.RawMessage) error

	// Lifecycle mutations (used by the scheduler and the mutation UI).
	WorkCreate(ctx context.Context, kind, title, body string) (string, error)
	SetStatus(ctx context.Context, id, status string) error
	Link(ctx context.Context, id, relType, target string) error
	Unlink(ctx context.Context, id, relType, target string) error

	// Meta administration. MetaApply's signature is depended on by Pilot's
	// first-run init flow and must not change.
	MetaApply(ctx context.Context, json []byte) error
	TaxonomyNodeAdd(ctx context.Context, taxonomy, slug, name, parentSlug string) error
	TaxonomyNodeMove(ctx context.Context, taxonomy, slug, newParentSlug string) error
	TaxonomyNodeRetire(ctx context.Context, taxonomy, slug string) error
	// TaxonomyNodeSet writes an opaque JSON metadata blob on a node (e.g. a
	// goals node's {result, resultNote}).
	TaxonomyNodeSet(ctx context.Context, taxonomy, slug string, metadata json.RawMessage) error

	// Identity.
	ActorRegister(ctx context.Context, actorID, publicKey, nodeID string) error

	// Close releases the underlying transport (and subprocess, for stdio).
	Close() error
}

// --- Real stdio client ---

// Config configures the dits-mcp subprocess the real client dials.
type Config struct {
	// Command is the dits-mcp executable (default "dits-mcp").
	Command string
	// Args are extra args appended after the defaults ("serve", "--project", root).
	Args []string
	// ProjectRoot is the DITS project root passed as --project.
	ProjectRoot string
	// Env is the subprocess environment; nil inherits the parent's.
	Env []string
}

// client is the live MCP-over-stdio implementation of Client.
type client struct {
	mc *mcpgo.Client
}

// NewClient spawns a dits-mcp subprocess over stdio, performs the MCP
// handshake, and returns a ready Client. Callers must Close() it.
func NewClient(ctx context.Context, cfg Config) (Client, error) {
	command := cfg.Command
	if command == "" {
		command = "dits-mcp"
	}
	// dits-mcp is a flag-only binary (no subcommand): `dits-mcp -project <dir>`.
	// A positional "serve" would make flag.Parse stop before -project, so the
	// project would never be set — pass the flag directly.
	var args []string
	if cfg.ProjectRoot != "" {
		args = append(args, "-project", cfg.ProjectRoot)
	}
	args = append(args, cfg.Args...)

	mc, err := mcpgo.NewStdioMCPClient(command, cfg.Env, args...)
	if err != nil {
		return nil, fmt.Errorf("pilot/mcp: launching %s: %w", command, err)
	}

	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{Name: "pilot", Version: "0"}
	if _, err := mc.Initialize(ctx, initReq); err != nil {
		_ = mc.Close()
		return nil, fmt.Errorf("pilot/mcp: initialize: %w", err)
	}
	return &client{mc: mc}, nil
}

func (c *client) Close() error {
	if c.mc == nil {
		return nil
	}
	return c.mc.Close()
}

// rawCall invokes a dits_* tool and returns its result text, surfacing an
// error-result as a Go error.
func (c *client) rawCall(ctx context.Context, tool string, args map[string]any) (string, error) {
	req := mcp.CallToolRequest{}
	req.Params.Name = tool
	req.Params.Arguments = args
	res, err := c.mc.CallTool(ctx, req)
	if err != nil {
		return "", fmt.Errorf("pilot/mcp: %s: %w", tool, err)
	}
	text := resultText(res)
	if res.IsError {
		return "", fmt.Errorf("pilot/mcp: %s: %s", tool, text)
	}
	return text, nil
}

// call invokes a tool and unmarshals its JSON result text into out.
func (c *client) call(ctx context.Context, tool string, args map[string]any, out any) error {
	text, err := c.rawCall(ctx, tool, args)
	if err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	return decodeResult(text, out)
}

// callVoid invokes a tool and discards a successful result.
func (c *client) callVoid(ctx context.Context, tool string, args map[string]any) error {
	_, err := c.rawCall(ctx, tool, args)
	return err
}

func (c *client) WorkList(ctx context.Context, f Filters) ([]WorkItem, error) {
	var out []WorkItem
	if err := c.call(ctx, "dits_work_list", f.args(), &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *client) WorkGet(ctx context.Context, id string) (WorkItem, error) {
	var out WorkItem
	err := c.call(ctx, "dits_work_show", map[string]any{"id": id}, &out)
	return out, err
}

func (c *client) EventsList(ctx context.Context, eventType, since string, limit int) ([]Event, error) {
	args := map[string]any{}
	if eventType != "" {
		args["type"] = eventType
	}
	if since != "" {
		args["since"] = since
	}
	if limit > 0 {
		args["limit"] = limit
	}
	var out []Event
	if err := c.call(ctx, "dits_events_list", args, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *client) RoleBindingsList(ctx context.Context, id string) ([]RoleBinding, error) {
	var out []RoleBinding
	if err := c.call(ctx, "dits_role_bindings_list", map[string]any{"id": id}, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *client) DiagnosticsGet(ctx context.Context, id string) ([]Diagnostic, error) {
	var out []Diagnostic
	if err := c.call(ctx, "dits_diagnostics_get", map[string]any{"id": id}, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *client) MetaGet(ctx context.Context) (Meta, error) {
	var out Meta
	err := c.call(ctx, "dits_meta_show", map[string]any{}, &out)
	return out, err
}

func (c *client) ActorList(ctx context.Context) ([]ActorRecord, error) {
	var out []ActorRecord
	if err := c.call(ctx, "dits_actor_list", map[string]any{}, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *client) Classify(ctx context.Context, id, taxonomy, node string) error {
	return c.callVoid(ctx, "dits_work_classify", map[string]any{"id": id, "taxonomy": taxonomy, "node": node})
}

func (c *client) Declassify(ctx context.Context, id, taxonomy, node string) error {
	return c.callVoid(ctx, "dits_work_declassify", map[string]any{"id": id, "taxonomy": taxonomy, "node": node})
}

func (c *client) BindRole(ctx context.Context, id, role, actor string) error {
	return c.callVoid(ctx, "dits_work_role_bind", map[string]any{"id": id, "role": role, "actor": actor})
}

func (c *client) UnbindRole(ctx context.Context, id, role, actor string) error {
	return c.callVoid(ctx, "dits_work_role_unbind", map[string]any{"id": id, "role": role, "actor": actor})
}

func (c *client) AckFile(ctx context.Context, id, scopeSummary, deliveryTiming, targetOutcome, acceptanceCriteria string) error {
	return c.callVoid(ctx, "dits_ack_file", map[string]any{
		"id":                  id,
		"scope_summary":       scopeSummary,
		"delivery_timing":     deliveryTiming,
		"target_outcome":      targetOutcome,
		"acceptance_criteria": acceptanceCriteria,
	})
}

func (c *client) AckAccept(ctx context.Context, id, who, note string) error {
	return c.callVoid(ctx, "dits_ack_accept", map[string]any{"id": id, "who": who, "note": note})
}

func (c *client) AckReject(ctx context.Context, id, who, note string) error {
	return c.callVoid(ctx, "dits_ack_reject", map[string]any{"id": id, "who": who, "note": note})
}

func (c *client) AckAmend(ctx context.Context, id, amendmentType string, fields []string, reason string) error {
	args := map[string]any{"id": id, "amendment_type": amendmentType, "reason": reason}
	if len(fields) > 0 {
		args["fields"] = fields
	}
	return c.callVoid(ctx, "dits_ack_amend", args)
}

func (c *client) WorkCreate(ctx context.Context, kind, title, body string) (string, error) {
	var out struct {
		SharedID string   `json:"shared_id"`
		WorkItem WorkItem `json:"work_item"`
	}
	args := map[string]any{"kind": kind, "title": title}
	if body != "" {
		args["body"] = body
	}
	if err := c.call(ctx, "dits_work_create", args, &out); err != nil {
		return "", err
	}
	if out.SharedID != "" {
		return out.SharedID, nil
	}
	return out.WorkItem.ID, nil
}

func (c *client) SetStatus(ctx context.Context, id, status string) error {
	return c.callVoid(ctx, "dits_work_status", map[string]any{"id": id, "status": status})
}

func (c *client) Link(ctx context.Context, id, relType, target string) error {
	return c.callVoid(ctx, "dits_work_link", map[string]any{"id": id, "type": relType, "target": target})
}

func (c *client) Unlink(ctx context.Context, id, relType, target string) error {
	return c.callVoid(ctx, "dits_work_unlink", map[string]any{"id": id, "type": relType, "target": target})
}

func (c *client) FieldSet(ctx context.Context, id, field, value string) error {
	return c.callVoid(ctx, "dits_work_field_set", map[string]any{"id": id, "field": field, "value": value})
}

func (c *client) ScheduleSet(ctx context.Context, id string, stages []Stage) error {
	// The tool's stages_json expects the substrate's lowercase keys
	// (domain.ScheduleStage), but Pilot's Stage decodes the materialized
	// WorkItem with PascalCase. Re-shape to the wire form here.
	raw, err := json.Marshal(stagesWire(stages))
	if err != nil {
		return err
	}
	return c.callVoid(ctx, "dits_work_schedule_set", map[string]any{"id": id, "stages_json": string(raw)})
}

// stageWire is the lowercase send-shape matching domain.ScheduleStage tags.
type stageWire struct {
	Key       string `json:"key"`
	Label     string `json:"label"`
	Date      string `json:"date"`
	Precision string `json:"precision"`
	State     string `json:"state"`
}

func stagesWire(stages []Stage) []stageWire {
	out := make([]stageWire, len(stages))
	for i, s := range stages {
		out[i] = stageWire{Key: s.Key, Label: s.Label, Date: s.Date, Precision: s.Precision, State: s.State}
	}
	return out
}

func (c *client) Observe(ctx context.Context, id, summary string, data json.RawMessage) error {
	args := map[string]any{"id": id, "summary": summary}
	if len(data) > 0 {
		args["data"] = string(data)
	}
	return c.callVoid(ctx, "dits_work_observe", args)
}

func (c *client) MetaApply(ctx context.Context, metaJSON []byte) error {
	return c.callVoid(ctx, "dits_meta_apply", map[string]any{"meta_json": string(metaJSON)})
}

func (c *client) TaxonomyNodeAdd(ctx context.Context, taxonomy, slug, name, parentSlug string) error {
	args := map[string]any{"taxonomy": taxonomy, "slug": slug, "name": name}
	if parentSlug != "" {
		args["parent_slug"] = parentSlug
	}
	return c.callVoid(ctx, "dits_taxonomy_node_add", args)
}

func (c *client) TaxonomyNodeMove(ctx context.Context, taxonomy, slug, newParentSlug string) error {
	return c.callVoid(ctx, "dits_taxonomy_node_move", map[string]any{
		"taxonomy": taxonomy, "slug": slug, "new_parent_slug": newParentSlug,
	})
}

func (c *client) TaxonomyNodeRetire(ctx context.Context, taxonomy, slug string) error {
	return c.callVoid(ctx, "dits_taxonomy_node_retire", map[string]any{"taxonomy": taxonomy, "slug": slug})
}

func (c *client) TaxonomyNodeSet(ctx context.Context, taxonomy, slug string, metadata json.RawMessage) error {
	return c.callVoid(ctx, "dits_taxonomy_node_set", map[string]any{
		"taxonomy": taxonomy, "slug": slug, "metadata": string(metadata),
	})
}

func (c *client) ActorRegister(ctx context.Context, actorID, publicKey, nodeID string) error {
	args := map[string]any{"actor_id": actorID, "public_key": publicKey}
	if nodeID != "" {
		args["node_id"] = nodeID
	}
	return c.callVoid(ctx, "dits_actor_register", args)
}

// --- Result decoding (transport-independent, unit-testable) ---

// resultText concatenates the text content blocks of a tool result.
func resultText(res *mcp.CallToolResult) string {
	if res == nil {
		return ""
	}
	var b strings.Builder
	for _, content := range res.Content {
		if tc, ok := content.(mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

// decodeResult unmarshals a dits_* JSON result string into out. An empty
// string decodes to the zero value (some void tools return a bare object).
func decodeResult(text string, out any) error {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	if err := json.Unmarshal([]byte(text), out); err != nil {
		return fmt.Errorf("pilot/mcp: decoding result: %w", err)
	}
	return nil
}

// --- Stub client ---

// stubClient is a no-op Client for handlers running without an MCP endpoint.
type stubClient struct{}

// NewStub returns a Client whose mutating methods return ErrNotImplemented.
// Query methods return empty results so views render an empty state rather
// than erroring.
func NewStub() Client { return stubClient{} }

func (stubClient) WorkList(context.Context, Filters) ([]WorkItem, error) { return nil, nil }
func (stubClient) WorkGet(context.Context, string) (WorkItem, error) {
	return WorkItem{}, ErrNotImplemented
}
func (stubClient) EventsList(context.Context, string, string, int) ([]Event, error) { return nil, nil }
func (stubClient) RoleBindingsList(context.Context, string) ([]RoleBinding, error)  { return nil, nil }
func (stubClient) DiagnosticsGet(context.Context, string) ([]Diagnostic, error)     { return nil, nil }
func (stubClient) MetaGet(context.Context) (Meta, error)                            { return Meta{}, ErrNotImplemented }
func (stubClient) ActorList(context.Context) ([]ActorRecord, error)                 { return nil, nil }
func (stubClient) Classify(context.Context, string, string, string) error           { return ErrNotImplemented }
func (stubClient) Declassify(context.Context, string, string, string) error         { return ErrNotImplemented }
func (stubClient) BindRole(context.Context, string, string, string) error           { return ErrNotImplemented }
func (stubClient) UnbindRole(context.Context, string, string, string) error         { return ErrNotImplemented }
func (stubClient) AckFile(context.Context, string, string, string, string, string) error {
	return ErrNotImplemented
}
func (stubClient) AckAccept(context.Context, string, string, string) error { return ErrNotImplemented }
func (stubClient) AckReject(context.Context, string, string, string) error { return ErrNotImplemented }
func (stubClient) AckAmend(context.Context, string, string, []string, string) error {
	return ErrNotImplemented
}
func (stubClient) FieldSet(context.Context, string, string, string) error { return ErrNotImplemented }
func (stubClient) ScheduleSet(context.Context, string, []Stage) error      { return ErrNotImplemented }
func (stubClient) Observe(context.Context, string, string, json.RawMessage) error {
	return ErrNotImplemented
}
func (stubClient) WorkCreate(context.Context, string, string, string) (string, error) {
	return "", ErrNotImplemented
}
func (stubClient) SetStatus(context.Context, string, string) error { return ErrNotImplemented }
func (stubClient) Link(context.Context, string, string, string) error {
	return ErrNotImplemented
}
func (stubClient) Unlink(context.Context, string, string, string) error {
	return ErrNotImplemented
}
func (stubClient) MetaApply(context.Context, []byte) error { return ErrNotImplemented }
func (stubClient) TaxonomyNodeAdd(context.Context, string, string, string, string) error {
	return ErrNotImplemented
}
func (stubClient) TaxonomyNodeMove(context.Context, string, string, string) error {
	return ErrNotImplemented
}
func (stubClient) TaxonomyNodeRetire(context.Context, string, string) error { return ErrNotImplemented }
func (stubClient) TaxonomyNodeSet(context.Context, string, string, json.RawMessage) error {
	return ErrNotImplemented
}
func (stubClient) ActorRegister(context.Context, string, string, string) error {
	return ErrNotImplemented
}
func (stubClient) Close() error { return nil }
