// GET /api/data — the live read adapter that drives the embedded prototype
// React app (/app). It assembles the substrate's state into JSON whose shape is
// byte-for-byte the prototype's hardcoded window.DATA (app/data.jsx), so the
// app can `fetch('/api/data')` in place of the seed dataset.
//
// Every key/field name below reproduces data.jsx exactly (camelCase, the
// prototype's vocabulary), independent of the snake_case/PascalCase MCP DTOs.
// Derivable fields (ryg, target, acks, roles, classifications, stages, risks,
// nextSteps, status, deps, fromRfc, diagnostics) are computed from the live
// substrate; signals that have no clean substrate source (fresh, sig, age,
// indicators per-row, portfolio sparklines) are defaulted to a sensible shape.
//
// All work-item references are resolved from raw wrk_ ids to human shared ids
// (resolveShared), so deps / decision+outcome milestone / event targets / role
// bindings all read as REDEMO-style ids the app can cross-link.
package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/lenulus/pf/internal/pilot/mcp"
	"github.com/lenulus/pf/internal/pilot/projections"
)

// --- window.DATA payload shape (mirrors app/data.jsx exactly) ---

type apiPayload struct {
	ACTORS           map[string]apiActor     `json:"ACTORS"`
	TAXONOMIES       map[string]apiTaxonomy  `json:"TAXONOMIES"`
	ROLES            []apiRole               `json:"ROLES"`
	ROLE_CONSTRAINTS []apiRoleConstraint     `json:"ROLE_CONSTRAINTS"`
	MILESTONES       []apiMilestone          `json:"MILESTONES"`
	RFCS             []apiRFC                `json:"RFCS"`
	PILOT_LOG        []apiLogEntry           `json:"PILOT_LOG"`
	DECISIONS        []apiDecision           `json:"DECISIONS"`
	OUTCOMES         []apiOutcome            `json:"OUTCOMES"`
	ROLE_BINDINGS    []apiRoleBinding        `json:"ROLE_BINDINGS"`
	EVENT_LOG        []apiEvent              `json:"EVENT_LOG"`
	ACK_AMENDMENTS   map[string][]apiAmend   `json:"ACK_AMENDMENTS"`
	INDICATORS       map[string]apiIndicator `json:"INDICATORS"`
	// CURRENT_USER is the actor the "For you" / ACK views treat as "me". Until
	// auth lands (Track F), default to the actor with the most role bindings so
	// the attention view has content; the app falls back to a constant.
	CURRENT_USER string `json:"CURRENT_USER"`
}

// pickCurrentUser returns the actor with the most role bindings, or "".
func pickCurrentUser(bindings []apiRoleBinding) string {
	counts := map[string]int{}
	best, bestN := "", 0
	for _, b := range bindings {
		if b.Actor == nil || *b.Actor == "" {
			continue
		}
		a := *b.Actor
		counts[a]++
		if counts[a] > bestN {
			best, bestN = a, counts[a]
		}
	}
	return best
}

type apiActor struct {
	ID       string `json:"id"`
	Initials string `json:"initials"`
	Name     string `json:"name"`
	Color    string `json:"color"`
	Role     string `json:"role"`
	Agent    bool   `json:"agent,omitempty"`
}

type apiTaxonomy struct {
	Slug   string            `json:"slug"`
	Name   string            `json:"name"`
	Levels []string          `json:"levels"`
	Nodes  []apiTaxonomyNode `json:"nodes"`
}

type apiTaxonomyNode struct {
	Slug       string  `json:"slug"`
	Name       string  `json:"name"`
	Parent     *string `json:"parent"`
	Result     string  `json:"result,omitempty"`
	ResultNote string  `json:"resultNote,omitempty"`
}

type apiRole struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Cardinality string `json:"cardinality"`
}

type apiRoleConstraint struct {
	Slug        string         `json:"slug"`
	Name        string         `json:"name"`
	Predicate   string         `json:"predicate"`
	Args        map[string]any `json:"args,omitempty"`
	Severity    string         `json:"severity"`
	Description string         `json:"description,omitempty"`
}

type apiAckRow struct {
	Who    string `json:"who"`
	Actor  string `json:"actor"`
	Action string `json:"action"`
	When   string `json:"when"`
	Note   string `json:"note"`
}

type apiStage struct {
	Key       string `json:"key"`
	Label     string `json:"label"`
	Date      string `json:"date"`
	Precision string `json:"precision"`
	State     string `json:"state"`
}

type apiRisk struct {
	Body     string `json:"body"`
	Severity string `json:"severity"`
	By       string `json:"by"`
	When     string `json:"when"`
}

type apiNextStep struct {
	Body  string `json:"body"`
	Owner string `json:"owner"`
	When  string `json:"when"`
}

type apiMilestone struct {
	ID              string         `json:"id"`
	Kind            string         `json:"kind"`
	Title           string         `json:"title"`
	Status          string         `json:"status"`
	RYG             string         `json:"ryg"`
	SpecifierAck    string         `json:"specifierAck"`
	BuilderAck      string         `json:"builderAck"`
	AckHistory      []apiAckRow    `json:"ackHistory"`
	Target          string         `json:"target"`
	TargetPrecision string         `json:"targetPrecision"`
	TargetISO       string         `json:"targetISO,omitempty"`
	CustomerVisible bool           `json:"customerVisible"`
	Specifier       string         `json:"specifier"`
	Builder         string         `json:"builder"`
	Pilot           string         `json:"pilot"`
	OrgNode         *string        `json:"orgNode"`
	ProductNode     *string        `json:"productNode"`
	ScopeChanges    int            `json:"scopeChanges"`
	DepClosed       []int          `json:"depClosed"`
	Latency         *string        `json:"latency"`
	Fresh           string         `json:"fresh"`
	Age             string         `json:"age"`
	Sig             string         `json:"sig"`
	Indicators      map[string]any `json:"indicators"`
	Diagnostics     []string       `json:"diagnostics"`
	Stages          []apiStage     `json:"stages,omitempty"`
	StatusNarrative string         `json:"statusNarrative"`
	StatusUpdatedAt string         `json:"statusUpdatedAt"`
	StatusUpdatedBy string         `json:"statusUpdatedBy"`
	Risks           []apiRisk      `json:"risks"`
	NextSteps       []apiNextStep  `json:"nextSteps"`
	Deps            []string       `json:"deps"`
	FromRfc         string         `json:"fromRfc,omitempty"`
	Parent          string         `json:"parent,omitempty"`
}

type apiRFC struct {
	ID         string `json:"id"`
	Kind       string `json:"kind"`
	Title      string `json:"title"`
	Status     string `json:"status"`
	Target     string `json:"target"`
	Specifier  string `json:"specifier"`
	Age        string `json:"age"`
	Sig        string `json:"sig"`
	ApprovedAt string `json:"approvedAt,omitempty"`
	Summary    string `json:"summary"`
}

type apiLogEntry struct {
	Num       int    `json:"num"`
	Milestone string `json:"milestone"`
	Kind      string `json:"kind"`
	By        string `json:"by"`
	Time      string `json:"time"`
	Body      string `json:"body"`
	Integrity string `json:"integrity,omitempty"`
}

type apiDecision struct {
	ID        string `json:"id"`
	Milestone string `json:"milestone"`
	Status    string `json:"status"`
	Opened    string `json:"opened"`
	IdleDays  int    `json:"idleDays"`
	Owner     string `json:"owner"`
	Summary   string `json:"summary"`
}

type apiOutcome struct {
	ID        string `json:"id"`
	Milestone string `json:"milestone"`
	Status    string `json:"status"`
	Due       string `json:"due"`
	Result    string `json:"result"`
	Value     string `json:"value"`
	By        string `json:"by"`
	Note      string `json:"note"`
}

type apiRoleBinding struct {
	ID         string  `json:"id"`
	Milestone  string  `json:"milestone"`
	Role       string  `json:"role"`
	Actor      *string `json:"actor"`
	Diagnostic string  `json:"diagnostic,omitempty"`
}

type apiEvent struct {
	Num     int    `json:"num"`
	Type    string `json:"type"`
	Target  string `json:"target"`
	Actor   string `json:"actor"`
	Time    string `json:"time"`
	Payload string `json:"payload"`
	Sig     string `json:"sig"`
}

type apiAmend struct {
	ID     string   `json:"id"`
	Type   string   `json:"type"`
	By     string   `json:"by"`
	When   string   `json:"when"`
	Fields []string `json:"fields"`
	Reason string   `json:"reason"`
}

type apiIndicator struct {
	Value any    `json:"value"`
	Delta string `json:"delta"`
	Kind  string `json:"kind"`
	Unit  string `json:"unit,omitempty"`
	Spark string `json:"spark"`
}

// apiData serves GET /api/data — the full window.DATA payload over live data.
func (s *Server) apiData(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	payload := s.buildAPIData(ctx)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	if err := enc.Encode(payload); err != nil {
		log.Printf("pilot/web: /api/data encode: %v", err)
	}
}

// buildAPIData assembles the whole payload. Each query is best-effort: a failed
// fetch logs and yields an empty section so the app still renders.
func (s *Server) buildAPIData(ctx context.Context) apiPayload {
	milestones := s.milestones(ctx)
	rfcs, _ := s.Client.WorkList(ctx, mcp.Filters{Kind: "rfc"})
	decisions, _ := s.Client.WorkList(ctx, mcp.Filters{Kind: "decision_block"})
	outcomes, _ := s.Client.WorkList(ctx, mcp.Filters{Kind: "outcome_assessment"})
	meta, _ := s.Client.MetaGet(ctx)
	actorRecs, _ := s.Client.ActorList(ctx)
	allEvents, _ := s.Client.EventsList(ctx, "", "", 400)

	// Resolver: raw wrk_ ids (and shared ids) → human shared id, across every
	// work kind so deps/targets/parents resolve no matter what they point at.
	all := make([]mcp.WorkItem, 0, len(milestones)+len(rfcs)+len(decisions)+len(outcomes))
	all = append(all, milestones...)
	all = append(all, rfcs...)
	all = append(all, decisions...)
	all = append(all, outcomes...)
	shared := sharedIDs(all)

	// ackHistory + ack_amendments are derived from the ack_* event stream.
	ackEvents := ackEventsByWorkItem(allEvents)

	roleBindings := buildAPIRoleBindings(milestones, shared)
	return apiPayload{
		ACTORS:           buildAPIActors(actorRecs, milestones),
		TAXONOMIES:       buildAPITaxonomies(meta),
		ROLES:            buildAPIRoles(meta),
		ROLE_CONSTRAINTS: buildAPIRoleConstraints(meta),
		MILESTONES:       buildAPIMilestones(milestones, shared, ackEvents),
		RFCS:             buildAPIRFCs(rfcs),
		PILOT_LOG:        buildAPIPilotLog(allEvents, shared),
		DECISIONS:        buildAPIDecisions(decisions, shared),
		OUTCOMES:         buildAPIOutcomes(outcomes, shared),
		ROLE_BINDINGS:    roleBindings,
		EVENT_LOG:        buildAPIEventLog(allEvents, shared),
		ACK_AMENDMENTS:   buildAPIAmendments(allEvents, shared),
		INDICATORS:       s.buildAPIIndicators(ctx),
		CURRENT_USER:     pickCurrentUser(roleBindings),
	}
}

// --- ACTORS ---

func buildAPIActors(recs []mcp.ActorRecord, milestones []mcp.WorkItem) map[string]apiActor {
	out := map[string]apiActor{}
	add := func(id string) {
		if id == "" {
			if _, ok := out[id]; ok {
				return
			}
		}
		if _, ok := out[id]; ok {
			return
		}
		out[id] = apiActor{
			ID:       id,
			Initials: actorInitials(id),
			Name:     humanizeActor(id),
			Color:    actorColor(id),
			Role:     "",
			Agent:    looksLikeAgent(id),
		}
	}
	for _, rec := range recs {
		add(rec.ActorID)
	}
	// Backfill any actor referenced by a role binding but not registered, plus a
	// role label derived from the first role binding we see for them.
	roleOf := map[string]string{}
	for _, m := range milestones {
		for _, rb := range m.RoleBindings {
			if rb.Actor == "" {
				continue
			}
			add(rb.Actor)
			if roleOf[rb.Actor] == "" {
				roleOf[rb.Actor] = roleTitle(rb.RoleSlug)
			}
		}
	}
	for id, role := range roleOf {
		if a, ok := out[id]; ok && a.Role == "" && !a.Agent {
			a.Role = role
			out[id] = a
		}
	}
	return out
}

// humanizeActor turns "ejackson"/"e.jackson" into "E. Jackson"-ish; falls back
// to the raw id when it can't split sensibly.
func humanizeActor(id string) string {
	if id == "" {
		return ""
	}
	sep := func(r rune) bool { return r == '.' || r == '_' || r == '-' || r == ' ' }
	parts := strings.FieldsFunc(id, sep)
	if len(parts) >= 2 {
		first := strings.ToUpper(parts[0][:1])
		last := titleCase(parts[len(parts)-1])
		return first + ". " + last
	}
	// Single token like "ejackson": initial + capitalized remainder.
	rs := []rune(id)
	if len(rs) >= 2 {
		return strings.ToUpper(string(rs[0])) + ". " + titleCase(string(rs[1:]))
	}
	return titleCase(id)
}

func titleCase(s string) string {
	if s == "" {
		return ""
	}
	rs := []rune(s)
	return strings.ToUpper(string(rs[0])) + string(rs[1:])
}

// looksLikeAgent flags bot/agent actor ids so the app renders the agent chip.
func looksLikeAgent(id string) bool {
	l := strings.ToLower(id)
	return strings.Contains(l, "bot") || strings.Contains(l, "agent") || strings.HasSuffix(l, ".svc")
}

func roleTitle(slug string) string { return titleCase(slug) }

// --- TAXONOMIES ---

func buildAPITaxonomies(meta mcp.Meta) map[string]apiTaxonomy {
	out := map[string]apiTaxonomy{}
	for _, t := range meta.Taxonomies {
		nodes := make([]apiTaxonomyNode, 0, len(t.Nodes))
		for _, n := range t.Nodes {
			if n.Retired {
				continue
			}
			node := apiTaxonomyNode{Slug: n.Slug, Name: n.Name}
			if n.ParentSlug != "" {
				p := n.ParentSlug
				node.Parent = &p
			}
			// goals nodes carry {result, resultNote} in opaque metadata.
			if len(n.Metadata) > 0 {
				var md struct {
					Result     string `json:"result"`
					ResultNote string `json:"resultNote"`
				}
				if err := json.Unmarshal(n.Metadata, &md); err == nil {
					node.Result = md.Result
					node.ResultNote = md.ResultNote
				}
			}
			nodes = append(nodes, node)
		}
		out[t.Slug] = apiTaxonomy{Slug: t.Slug, Name: t.Name, Levels: t.Levels, Nodes: nodes}
	}
	return out
}

// --- ROLES / ROLE_CONSTRAINTS ---

func buildAPIRoles(meta mcp.Meta) []apiRole {
	out := make([]apiRole, 0, len(meta.Roles))
	for _, r := range meta.Roles {
		out = append(out, apiRole{Slug: r.Slug, Name: r.Name, Cardinality: r.Cardinality})
	}
	return out
}

func buildAPIRoleConstraints(meta mcp.Meta) []apiRoleConstraint {
	out := make([]apiRoleConstraint, 0, len(meta.RoleConstraints))
	for _, c := range meta.RoleConstraints {
		out = append(out, apiRoleConstraint{
			Slug:      c.Slug,
			Name:      c.Name,
			Predicate: c.Predicate,
			Args:      c.Args,
			Severity:  c.Severity,
		})
	}
	return out
}

// --- MILESTONES ---

func buildAPIMilestones(items []mcp.WorkItem, shared map[string]string, ackEvents map[string][]ackEvt) []apiMilestone {
	out := make([]apiMilestone, 0, len(items))
	for _, m := range items {
		out = append(out, buildAPIMilestone(m, shared, ackEvents))
	}
	return out
}

func buildAPIMilestone(m mcp.WorkItem, shared map[string]string, ackEvents map[string][]ackEvt) apiMilestone {
	specAck, buildAck := "pending", "pending"
	if a, ok := latestAck(m); ok {
		if a.Specifier != "" {
			specAck = a.Specifier
		}
		if a.Builder != "" {
			buildAck = a.Builder
		}
	}

	row := apiMilestone{
		ID:              rowID(m),
		Kind:            "milestone",
		Title:           m.Title,
		Status:          m.Status,
		RYG:             m.RYG(),
		SpecifierAck:    specAck,
		BuilderAck:      buildAck,
		AckHistory:      buildAckHistory(m, ackEvents),
		Target:          m.Target(),
		TargetPrecision: m.TargetPrecision(),
		TargetISO:       m.Field("target_iso"),
		CustomerVisible: m.CustomerVisible(),
		Specifier:       roleActor(m, "specifier"),
		Builder:         roleActor(m, "builder"),
		Pilot:           roleActor(m, "pilot"),
		OrgNode:         classNodeSlug(m, "org"),
		ProductNode:     classNodeSlug(m, "product"),
		ScopeChanges:    countScopeChanges(ackEvents[m.ID]),
		DepClosed:       depClosed(m, shared),
		Latency:         optString(m.Field("latency")),
		Fresh:           freshness(m),
		Age:             ageLabel(m),
		Sig:             shortSig(latestEventSig(m)),
		Indicators:      map[string]any{},
		Diagnostics:     diagnosticMessages(m),
		Stages:          buildAPIStages(m.Stages),
		StatusNarrative: m.StatusNarrative(),
		StatusUpdatedAt: shortDayTS(m.StatusUpdatedAt()),
		StatusUpdatedBy: statusUpdatedBy(m),
		Risks:           buildAPIRisks(m.Risks()),
		NextSteps:       buildAPINextSteps(m.NextSteps()),
		Deps:            depList(m, shared),
		FromRfc:         resolveShared(shared, m.FromRFC()),
		Parent:          resolveShared(shared, parentOf(m)),
	}
	return row
}

func buildAPIStages(stages []mcp.Stage) []apiStage {
	out := make([]apiStage, 0, len(stages))
	for _, s := range stages {
		out = append(out, apiStage{Key: s.Key, Label: s.Label, Date: s.Date, Precision: s.Precision, State: s.State})
	}
	return out
}

func buildAPIRisks(risks []mcp.Risk) []apiRisk {
	out := make([]apiRisk, 0, len(risks))
	for _, r := range risks {
		out = append(out, apiRisk{Body: r.Body, Severity: r.Severity, By: r.By, When: r.When})
	}
	return out
}

func buildAPINextSteps(steps []mcp.NextStep) []apiNextStep {
	out := make([]apiNextStep, 0, len(steps))
	for _, s := range steps {
		out = append(out, apiNextStep{Body: s.Body, Owner: s.Owner, When: s.When})
	}
	return out
}

// classNodeSlug returns the classification node slug for a taxonomy, or nil so the
// prototype renders an "unclassified"/null state (matches data.jsx's null).
func classNodeSlug(m mcp.WorkItem, taxonomy string) *string {
	for _, c := range m.Classifications {
		if c.TaxonomySlug == taxonomy {
			node := c.NodeSlug
			return &node
		}
	}
	return nil
}

// diagnosticMessages flattens the engine diagnostics to the prototype's plain
// string list (the app renders each as a chip).
func diagnosticMessages(m mcp.WorkItem) []string {
	out := make([]string, 0, len(m.Diagnostics))
	for _, d := range m.Diagnostics {
		out = append(out, d.Message)
	}
	return out
}

// depList resolves the work item's depends_on relations to shared ids.
func depList(m mcp.WorkItem, shared map[string]string) []string {
	out := []string{}
	for _, r := range m.Relations {
		if r.Type == "depends_on" {
			out = append(out, resolveShared(shared, r.TargetWorkItem))
		}
	}
	return out
}

// depClosed is the [closed,total] pair the prototype renders as "N/M deps".
// Total = depends_on count; closed = best-effort 0 (substrate has no per-edge
// closure flag in the WorkItem DTO), so we emit [0,total].
func depClosed(m mcp.WorkItem, shared map[string]string) []int {
	total := 0
	for _, r := range m.Relations {
		if r.Type == "depends_on" {
			total++
		}
	}
	return []int{0, total}
}

// parentOf returns the parent work item id from a parent_of/child relation, or
// the substrate's relation form, else "".
func parentOf(m mcp.WorkItem) string {
	for _, r := range m.Relations {
		if r.Type == "child_of" || r.Type == "parent" {
			return r.TargetWorkItem
		}
	}
	return ""
}

// statusUpdatedBy derives the author of the latest status narrative from its
// observation. The DTO doesn't carry an author on the observation, so fall back
// to the milestone's specifier.
func statusUpdatedBy(m mcp.WorkItem) string {
	// typedObs author isn't exposed; default to specifier as a sensible owner.
	return roleActor(m, "specifier")
}

// freshness maps the milestone's update age to fresh|aging|stale, mirroring the
// prototype's freshness vocabulary.
func freshness(m mcp.WorkItem) string {
	d := idleDays(m)
	switch {
	case d <= 7:
		return "fresh"
	case d <= 21:
		return "aging"
	default:
		return "stale"
	}
}

// ageLabel renders the update age as "Nd" (the prototype's compact age token).
func ageLabel(m mcp.WorkItem) string {
	return fmt.Sprintf("%dd", idleDays(m))
}

// latestEventSig returns a deterministic short signature seed for the row. The
// WorkItem DTO carries no event ids, so derive from the stable work-item id.
func latestEventSig(m mcp.WorkItem) string {
	return fmt.Sprintf("%08x", fnv32(m.ID))
}

func fnv32(s string) uint32 {
	var h uint32 = 2166136261
	for _, b := range []byte(s) {
		h ^= uint32(b)
		h *= 16777619
	}
	return h
}

func optString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// --- ACK history / amendments (from the ack_* event stream) ---

type ackEvt struct {
	action    string // accepted | rejected | cleared | amended | filed
	who       string
	actor     string
	when      string // YYYY-MM-DD
	note      string
	amendType string
	fields    []string
	id        string
}

// ackEventsByWorkItem buckets the ack_* events per raw work-item id, newest
// first, decoding the who/note/reason/amendment payload.
func ackEventsByWorkItem(events []mcp.Event) map[string][]ackEvt {
	actionByType := map[string]string{
		"work.ack_filed":       "filed",
		"work.ack_accepted":    "accepted",
		"work.ack_rejected":    "rejected",
		"work.ack_cleared":     "cleared",
		"work.ack_recommitted": "accepted",
		"work.ack_amended":     "amended",
	}
	out := map[string][]ackEvt{}
	for _, e := range events {
		action, ok := actionByType[e.Type]
		if !ok {
			continue
		}
		var p struct {
			Who           string   `json:"who"`
			Note          string   `json:"note"`
			Reason        string   `json:"reason"`
			AmendmentType string   `json:"amendment_type"`
			Fields        []string `json:"fields"`
		}
		_ = json.Unmarshal(e.Payload, &p)
		note := firstNonEmpty(p.Note, p.Reason)
		out[e.WorkItemID] = append(out[e.WorkItemID], ackEvt{
			action:    action,
			who:       p.Who,
			actor:     e.ActorID,
			when:      shortDayTS(e.Timestamp),
			note:      note,
			amendType: p.AmendmentType,
			fields:    p.Fields,
			id:        e.ID,
		})
	}
	for id := range out {
		evs := out[id]
		sort.SliceStable(evs, func(i, j int) bool { return evs[i].when > evs[j].when })
		out[id] = evs
	}
	return out
}

// buildAckHistory renders the per-milestone ACK history rows (newest first).
func buildAckHistory(m mcp.WorkItem, ackEvents map[string][]ackEvt) []apiAckRow {
	out := []apiAckRow{}
	for _, e := range ackEvents[m.ID] {
		if e.action == "amended" || e.action == "filed" {
			continue // amendments live in ACK_AMENDMENTS; history is accept/reject/clear
		}
		who := e.who
		actor := e.actor
		if who == "" {
			// Infer the side from the bound role of the acting actor.
			if actor == roleActor(m, "builder") {
				who = "builder"
			} else {
				who = "specifier"
			}
		}
		if actor == "" {
			actor = roleActor(m, who)
		}
		out = append(out, apiAckRow{Who: who, Actor: actor, Action: e.action, When: e.when, Note: e.note})
	}
	return out
}

// countScopeChanges counts scope_change amendments for a milestone (the
// prototype's scopeChanges signal).
func countScopeChanges(evs []ackEvt) int {
	n := 0
	for _, e := range evs {
		if e.action == "amended" && e.amendType == "scope_change" {
			n++
		}
	}
	return n
}

// buildAPIAmendments groups ack_amended events per shared milestone id.
func buildAPIAmendments(events []mcp.Event, shared map[string]string) map[string][]apiAmend {
	out := map[string][]apiAmend{}
	for _, e := range events {
		if e.Type != "work.ack_amended" {
			continue
		}
		var p struct {
			AmendmentType string   `json:"amendment_type"`
			Reason        string   `json:"reason"`
			Fields        []string `json:"fields"`
		}
		_ = json.Unmarshal(e.Payload, &p)
		key := resolveShared(shared, e.WorkItemID)
		fields := p.Fields
		if fields == nil {
			fields = []string{}
		}
		out[key] = append(out[key], apiAmend{
			ID:     shortSig(e.ID),
			Type:   p.AmendmentType,
			By:     e.ActorID,
			When:   shortDayTS(e.Timestamp),
			Fields: fields,
			Reason: p.Reason,
		})
	}
	for k := range out {
		evs := out[k]
		sort.SliceStable(evs, func(i, j int) bool { return evs[i].When > evs[j].When })
		out[k] = evs
	}
	return out
}

// --- RFCS ---

func buildAPIRFCs(items []mcp.WorkItem) []apiRFC {
	out := make([]apiRFC, 0, len(items))
	for _, r := range items {
		out = append(out, apiRFC{
			ID:         rowID(r),
			Kind:       "rfc",
			Title:      r.Title,
			Status:     r.Status,
			Target:     firstNonEmpty(r.Target(), "—"),
			Specifier:  roleActor(r, "specifier"),
			Age:        ageLabel(r),
			Sig:        shortSig(latestEventSig(r)),
			ApprovedAt: r.Field("approved_at"),
			Summary:    firstNonEmpty(r.Body, r.Title),
		})
	}
	return out
}

// --- PILOT_LOG (from observations) ---

// pilotLogKinds maps an observation entry_type to the prototype log kind. The
// substrate journals risk/next/status plus integrity/stress entries via the
// opaque ObsData discriminator.
func buildAPIPilotLog(events []mcp.Event, shared map[string]string) []apiLogEntry {
	out := []apiLogEntry{}
	num := 0
	for _, e := range events {
		if e.Type != "work.observation_recorded" {
			continue
		}
		var p struct {
			Summary string          `json:"summary"`
			Data    json.RawMessage `json:"data"`
		}
		_ = json.Unmarshal(e.Payload, &p)
		var d struct {
			EntryType string `json:"entry_type"`
			Integrity string `json:"ack_integrity"`
		}
		if len(p.Data) > 0 {
			_ = json.Unmarshal(p.Data, &d)
		}
		out = append(out, apiLogEntry{
			Milestone: resolveShared(shared, e.WorkItemID),
			Kind:      firstNonEmpty(d.EntryType, "observation"),
			By:        e.ActorID,
			Time:      logTime(e.Timestamp),
			Body:      p.Summary,
			Integrity: d.Integrity,
		})
	}
	// Number newest-first the way the prototype seeds them (descending §).
	num = len(out)
	for i := range out {
		out[i].Num = num - i
	}
	return out
}

// --- DECISIONS ---

func buildAPIDecisions(items []mcp.WorkItem, shared map[string]string) []apiDecision {
	out := make([]apiDecision, 0, len(items))
	for _, it := range items {
		out = append(out, apiDecision{
			ID:        rowID(it),
			Milestone: resolveShared(shared, parentMilestone(it)),
			Status:    it.Status,
			Opened:    shortDayTS(it.CreatedAt),
			IdleDays:  idleDays(it),
			Owner:     roleActor(it, "owner"),
			Summary:   firstNonEmpty(it.Title, it.Body),
		})
	}
	return out
}

// --- OUTCOMES ---

func buildAPIOutcomes(items []mcp.WorkItem, shared map[string]string) []apiOutcome {
	out := make([]apiOutcome, 0, len(items))
	for _, it := range items {
		out = append(out, apiOutcome{
			ID:        rowID(it),
			Milestone: resolveShared(shared, parentMilestone(it)),
			Status:    it.Status,
			Due:       firstNonEmpty(shortDayTS(it.Field("sla_due")), "—"),
			Result:    firstNonEmpty(it.Field("verdict"), "—"),
			Value:     firstNonEmpty(it.Field("value"), "—"),
			By:        roleActor(it, "evaluator"),
			Note:      it.Body,
		})
	}
	return out
}

// --- ROLE_BINDINGS ---

func buildAPIRoleBindings(milestones []mcp.WorkItem, shared map[string]string) []apiRoleBinding {
	out := []apiRoleBinding{}
	n := 0
	for _, m := range milestones {
		// One diagnostic message per milestone, attached to its bindings.
		var diag string
		if len(m.Diagnostics) > 0 {
			diag = m.Diagnostics[0].Message
		}
		for _, rb := range m.RoleBindings {
			n++
			out = append(out, apiRoleBinding{
				ID:         fmt.Sprintf("rb-%d", n),
				Milestone:  resolveShared(shared, rowID(m)),
				Role:       rb.RoleSlug,
				Actor:      optString(rb.Actor),
				Diagnostic: diag,
			})
		}
	}
	return out
}

// --- EVENT_LOG ---

func buildAPIEventLog(events []mcp.Event, shared map[string]string) []apiEvent {
	out := make([]apiEvent, 0, len(events))
	n := len(events)
	for i, e := range events {
		out = append(out, apiEvent{
			Num:     n - i,
			Type:    e.Type,
			Target:  resolveShared(shared, e.WorkItemID),
			Actor:   e.ActorID,
			Time:    logTime(e.Timestamp),
			Payload: eventPayloadSummary(e),
			Sig:     shortSig(e.ID),
		})
	}
	return out
}

// --- INDICATORS (portfolio aggregate) ---

func (s *Server) buildAPIIndicators(ctx context.Context) map[string]apiIndicator {
	var ind projections.Indicators
	if err := s.Indicators.Rebuild(ctx, s.Client); err == nil {
		ind, _ = s.Indicators.Get()
	}
	days := func(d time.Duration) string {
		if d <= 0 {
			return "—"
		}
		return fmt.Sprintf("%.1f", d.Hours()/24)
	}
	return map[string]apiIndicator{
		"depRate": {
			Value: round2(ind.DependencyClosureRate),
			Delta: "+0.04 wk", Kind: "up",
			Spark: "0,18 10,15 20,16 30,14 40,12 50,11 60,10 70,8 80,9 90,7 100,6 110,5",
		},
		"scopeVel": {
			Value: round2(ind.ScopeChangeVelocity),
			Delta: "+0.10 wk", Kind: "warn", Unit: "/wk",
			Spark: "0,12 10,11 20,14 30,13 40,10 50,12 60,8 70,9 80,6 90,7 100,5 110,4",
		},
		"decFric": {
			Value: days(ind.DecisionBlockFriction),
			Delta: "+1.1 d", Kind: "down", Unit: "d",
			Spark: "0,18 10,17 20,15 30,16 40,12 50,13 60,10 70,9 80,8 90,7 100,6 110,4",
		},
		"ackLat": {
			Value: days(ind.AckToStartLatency),
			Delta: "−0.3 d", Kind: "up", Unit: "d",
			Spark: "0,16 10,14 20,13 30,11 40,10 50,9 60,11 70,8 80,7 90,8 100,6 110,5",
		},
	}
}

func round2(f float64) float64 {
	return float64(int(f*100+0.5)) / 100
}

// logTime renders an RFC3339 timestamp as the prototype's "YYYY-MM-DD HH:MM".
func logTime(ts string) string {
	if t, err := time.Parse(time.RFC3339, ts); err == nil {
		return t.Format("2006-01-02 15:04")
	}
	if len(ts) >= 16 {
		return strings.Replace(ts[:16], "T", " ", 1)
	}
	return ts
}
