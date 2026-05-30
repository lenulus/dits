// Track C — slide-over panel completion. This file holds the panel tab bodies
// that aren't the original ACK/Status editing surfaces (Stages, Deps, Updates,
// Receipts), the ACK-tab enrichments (diagnostics line, ACK history, stages
// preview, delivery-target editor), the RFC / Decision / Outcome detail bodies,
// the panel footer action rows, and the /partials/panel/... HTMX routes that
// swap a single tab body in place.
//
// Every control here is server-rendered trusted markup over escaped substrate
// values. Mutations live in mutations.go (one MCP call each); the bodies below
// are pure projection of mcp.WorkItem / mcp.Event into the prototype's
// vocabulary (panels.jsx). Reuses app.css classes only — no new CSS.
package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/lenulus/pf/internal/pilot/mcp"
)

// registerPanelPartials wires the Track-C HTMX tab/fragment routes. The
// coordinator calls this from registerPartials.
func (s *Server) registerPanelPartials(mux *http.ServeMux) {
	mux.HandleFunc("GET /partials/panel/{id}/tab/{tab}", s.panelTab)
	mux.HandleFunc("GET /partials/panel/{id}/ack-history", s.panelAckHistory)
}

// panelAckHistory returns the ACK history table fragment (lazy-loaded by the
// ACK tab via hx-trigger=load, since it needs the global ack event feed).
func (s *Server) panelAckHistory(w http.ResponseWriter, r *http.Request) {
	renderPartial(w, template.HTML(s.ackHistory(r.Context(), r.PathValue("id"))))
}

// panelTab returns one rendered milestone tab body as an HTMX fragment.
func (s *Server) panelTab(w http.ResponseWriter, r *http.Request) {
	id, tab := r.PathValue("id"), r.PathValue("tab")
	item, err := s.Client.WorkGet(r.Context(), id)
	if err != nil {
		renderPartial(w, errBody(err))
		return
	}
	renderPartial(w, s.buildMilestoneTab(r.Context(), item, tab))
}

// buildMilestoneTab dispatches to the body builder for tab. The Deps / Updates /
// Receipts tabs need extra substrate queries, so this carries a context and the
// server (for the MCP client); the ACK / Status / Stages tabs are pure.
func (s *Server) buildMilestoneTab(ctx context.Context, item mcp.WorkItem, tab string) template.HTML {
	switch tab {
	case "Status":
		return statusTab(item)
	case "ACK":
		return ackTab(item)
	case "Stages":
		return stagesTab(item)
	case "Deps":
		return s.depsTab(ctx, item)
	case "Updates":
		return s.updatesTab(ctx, item)
	case "Receipts":
		return s.receiptsTab(ctx, item)
	default:
		return template.HTML(`<div class="so-section"><div class="empty-line">` +
			template.HTMLEscapeString(tab) + ` — no content.</div></div>`)
	}
}

// --- shared small renderers ---

// sectionHead renders a `.so-section` flex header with an optional count badge
// and an optional red "required" chip (panels.jsx so-section pattern).
func sectionHead(title string, count int, req string) string {
	out := `<div class="so-section"><span>` + template.HTMLEscapeString(title) + `</span>`
	if count >= 0 {
		out += fmt.Sprintf(`<span class="so-section__count">%d</span>`, count)
	}
	if req != "" {
		out += `<span class="so-section__req">` + template.HTMLEscapeString(req) + `</span>`
	}
	return out + `</div>`
}

// emptyLine renders the prototype's italic empty-state line.
func emptyLine(s string) string {
	return `<div class="empty-line">` + template.HTMLEscapeString(s) + `</div>`
}

// diagLine renders one diagnostics line; constraint violations get the red
// treatment (panels.jsx MilestoneAck diag-stack).
func diagStack(item mcp.WorkItem) string {
	if len(item.Diagnostics) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<div class="diag-stack">`)
	for _, d := range item.Diagnostics {
		cls, lab := "diag-line", "advisory"
		if d.Severity == "violation" {
			cls, lab = "diag-line diag-line--violation", "violation"
		}
		fmt.Fprintf(&b, `<div class="%s"><span class="diag-line__lab">%s</span><span class="diag-line__body">%s</span></div>`,
			cls, lab, template.HTMLEscapeString(d.Message))
	}
	b.WriteString(`</div>`)
	return b.String()
}

// stagesPreview renders the compact inline stage chips used on the ACK tab.
func stagesPreview(item mcp.WorkItem) string {
	if len(item.Stages) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<div class="so-line"><span class="lab">Stages</span><span class="val"><div class="stages" style="gap:14px">`)
	for _, st := range item.Stages {
		state := st.State
		if state == "" {
			state = "open"
		}
		fmt.Fprintf(&b, `<span class="stage stage--%s"><span class="stage__dot"></span><span class="stage__lab">%s</span><span class="stage__when">%s</span></span>`,
			template.HTMLEscapeString(state), template.HTMLEscapeString(st.Label), template.HTMLEscapeString(st.Date))
	}
	b.WriteString(`</div></span></div>`)
	return b.String()
}

// --- C.1: delivery-target editor (Q/M/D precision) ---

// targetEditor renders the click-to-open delivery-target editor + a stages
// hint, posting to /m/{id}/target (mutations.go). The closed state shows the
// value and precision; the open form is a small inline GET-swapped fragment.
func targetEditor(item mcp.WorkItem) string {
	id := rowID(item)
	esc := template.HTMLEscapeString
	target := item.Target()
	if target == "" {
		target = "—"
	}
	prec := item.TargetPrecision()
	stageHint := "No staging defined yet"
	if n := len(item.Stages); n > 0 {
		stageHint = fmt.Sprintf("%d stage%s defined", n, pluralS(n))
	}
	precOpts := ""
	for _, p := range []struct{ v, lab string }{{"Q", "Quarter"}, {"M", "Month"}, {"D", "Day"}} {
		sel := ""
		if p.v == prec {
			sel = " selected"
		}
		precOpts += fmt.Sprintf(`<option value="%s"%s>%s</option>`, p.v, sel, p.lab)
	}
	return fmt.Sprintf(`<div class="so-line"><span class="lab">Delivery target</span><span class="val">
  <form method="post" action="/m/%s/target" hx-post="/m/%s/target" hx-target="[data-panel-body]" hx-swap="innerHTML" style="display:flex;gap:8px;align-items:center">
    <input name="target" class="so-input" style="width:auto;flex:1" value="%s" placeholder="2026 Q3 / 2026-09-30">
    <select name="precision" class="so-select">%s</select>
    <button class="rx-btn rx-btn--accent" type="submit">Set</button>
  </form>
  <span class="so-target__hint">%s</span>
</span></div>`, esc(id), esc(id), esc(target), precOpts, esc(stageHint))
}

// pluralS returns "" for n==1 else "s".
func pluralS(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// --- C.1: ACK history table ---

// ackHistoryTable renders the ACK lifecycle history for a record from the
// global ack_* event feed filtered to this work item (panels.jsx ack-hist).
// EventsList is by type only, so we union the five ack event types and filter.
func (s *Server) ackHistory(ctx context.Context, id string) string {
	type histRow struct{ when, who, action, actor, note string }
	var rows []histRow
	add := func(evType, action string) {
		evs, err := s.Client.EventsList(ctx, evType, "", 200)
		if err != nil {
			return
		}
		for _, e := range evs {
			if e.WorkItemID != id {
				continue
			}
			var p struct {
				Who    string `json:"who"`
				Note   string `json:"note"`
				Reason string `json:"reason"`
			}
			_ = json.Unmarshal(e.Payload, &p)
			note := p.Note
			if note == "" {
				note = p.Reason
			}
			rows = append(rows, histRow{shortDayTS(e.Timestamp), p.Who, action, e.ActorID, note})
		}
	}
	add("work.ack_filed", "filed")
	add("work.ack_accepted", "accepted")
	add("work.ack_rejected", "rejected")
	add("work.ack_cleared", "cleared")
	add("work.ack_amended", "amended")
	sort.Slice(rows, func(i, j int) bool { return rows[i].when > rows[j].when })

	var b strings.Builder
	b.WriteString(sectionHead("ACK history", len(rows), ""))
	if len(rows) == 0 {
		b.WriteString(emptyLine("No ACKs filed yet. Roles haven't stood behind this commitment."))
		return b.String()
	}
	b.WriteString(`<div class="ack-hist">`)
	for _, h := range rows {
		whoLab := ""
		switch h.who {
		case "specifier":
			whoLab = "S-ACK"
		case "builder":
			whoLab = "B-ACK"
		default:
			whoLab = "ACK"
		}
		fmt.Fprintf(&b, `<div class="ack-hist__row"><span class="ack-hist__when">%s</span><span class="ack-hist__who">%s</span>%s<span>%s</span><span class="ack-hist__note">%s</span></div>`,
			template.HTMLEscapeString(h.when), whoLab, ackActionPill(h.action),
			avatarHTML(h.actor), template.HTMLEscapeString(h.note))
	}
	b.WriteString(`</div>`)
	return b.String()
}

// ackActionPill colours an ACK history action token.
func ackActionPill(action string) template.HTML {
	v := "neutral"
	switch action {
	case "accepted":
		v = "g"
	case "rejected":
		v = "r"
	case "amended", "cleared":
		v = "y"
	}
	return pill(v, action)
}

// shortDayTS trims an RFC3339 timestamp to its date.
func shortDayTS(ts string) string {
	if len(ts) >= 10 {
		return ts[:10]
	}
	return ts
}

// --- C.3: Stages tab ---

// stagesTab renders the staged-delivery timeline with per-stage state/date/
// precision and an add/edit form posting the full list to ScheduleSet
// (panels.jsx MilestoneStages). Each row is editable inline; the form submits
// every stage so ScheduleSet (which replaces the list) stays consistent.
func stagesTab(item mcp.WorkItem) template.HTML {
	id := rowID(item)
	esc := template.HTMLEscapeString
	stages := item.Stages
	var b strings.Builder

	fmt.Fprintf(&b, `<div class="so-meta"><span>%d stage%s</span><span style="margin-left:auto">final target · %s</span></div>`,
		len(stages), pluralS(len(stages)), esc(orDash(item.Target())))

	if len(stages) == 0 {
		b.WriteString(emptyLineSerif("No staging defined. Add Dogfood / Beta / GA stages with their own dates."))
	}

	// hiddenStages emits hidden inputs replaying a stage set, so add/remove can
	// resubmit the full list to /m/{id}/stages (ScheduleSet replaces the list).
	hiddenStages := func(sts []mcp.Stage) string {
		var h strings.Builder
		for _, s := range sts {
			fmt.Fprintf(&h, `<input type="hidden" name="key" value="%s"><input type="hidden" name="label" value="%s"><input type="hidden" name="date" value="%s"><input type="hidden" name="precision" value="%s"><input type="hidden" name="state" value="%s">`,
				esc(s.Key), esc(s.Label), esc(s.Date), esc(orVal(s.Precision, "Q")), esc(orVal(s.State, "open")))
		}
		return h.String()
	}
	stateLabel := func(st string) string {
		switch st {
		case "done":
			return "Completed"
		case "soon":
			return "Next up"
		default:
			return "Open"
		}
	}

	// Stage cards (panels.jsx MilestoneStages): dot · label/state · date · prec.
	b.WriteString(`<div style="display:flex;flex-direction:column;gap:10px;padding:12px 0">`)
	for i, s := range stages {
		state := orVal(s.State, "open")
		others := append(append([]mcp.Stage{}, stages[:i]...), stages[i+1:]...)
		fmt.Fprintf(&b, `<div class="stage-row stage-row--%s">
  <span class="stage__dot" style="width:14px;height:14px"></span>
  <span><div style="font-family:var(--font-display);font-size:15px;font-weight:500;color:var(--ink-0);letter-spacing:-0.005em">%s</div><div style="font-family:var(--font-mono);font-size:10.5px;color:var(--ink-3)">%s</div></span>
  <span style="margin-left:auto;font-family:var(--font-mono);font-size:13px;color:var(--ink-1)">%s</span>
  <span style="font-family:var(--font-mono);font-size:9px;padding:1px 5px;background:var(--paper-2);border-radius:3px;color:var(--ink-4);margin-left:8px">%s</span>
  <form method="post" action="/m/%s/stages" hx-post="/m/%s/stages" hx-target="[data-panel-body]" hx-swap="innerHTML" style="margin:0 0 0 8px" title="remove stage">%s<button type="submit" style="background:none;border:none;color:var(--ink-4);cursor:pointer;font-size:14px;line-height:1">&times;</button></form>
</div>`,
			esc(state), esc(s.Label), esc(stateLabel(state)), esc(orDash(s.Date)), esc(orVal(s.Precision, "Q")),
			esc(id), esc(id), hiddenStages(others))
	}
	b.WriteString(`</div>`)

	// + add stage — a reveal-on-click inline form that appends to the timeline.
	precOpts := ""
	for _, p := range []string{"Q", "M", "D"} {
		precOpts += fmt.Sprintf(`<option value="%s">%s</option>`, p, p)
	}
	stateOpts := ""
	for _, st := range []struct{ v, lab string }{{"open", "Open"}, {"soon", "Next up"}, {"done", "Completed"}} {
		stateOpts += fmt.Sprintf(`<option value="%s">%s</option>`, st.v, st.lab)
	}
	fmt.Fprintf(&b, `<details class="so-add-stage"><summary class="so-mini-add">+ add stage</summary>
  <form method="post" action="/m/%s/stages" hx-post="/m/%s/stages" hx-target="[data-panel-body]" hx-swap="innerHTML" style="display:flex;gap:6px;align-items:center;padding:8px 0;flex-wrap:wrap">%s
    <input name="label" class="so-input" style="flex:1;min-width:120px" placeholder="Label (Dogfood / Beta / GA)">
    <input name="date" class="so-input" style="width:110px" placeholder="2026 Q3 / 2026-09-30">
    <select name="precision" class="so-select">%s</select>
    <select name="state" class="so-select">%s</select>
    <input type="hidden" name="key" value="">
    <button class="rx-btn rx-btn--accent rx-btn--sm" type="submit">Add</button>
  </form>
</details>`, esc(id), esc(id), hiddenStages(stages), precOpts, stateOpts)
	return template.HTML(b.String())
}

// emptyLineSerif renders the prototype's italic serif empty-state paragraph.
func emptyLineSerif(s string) string {
	return `<div style="padding:14px 0;font-family:var(--font-serif);font-style:italic;color:var(--ink-3);font-size:13.5px">` +
		template.HTMLEscapeString(s) + `</div>`
}

// --- C.4: Deps tab ---

// depsTab renders upstream (this depends_on) and downstream (others depending
// on this) milestone links with add/remove (Link/Unlink) and click-to-navigate
// (panels.jsx MilestoneDeps). Downstream is computed from the full milestone
// list (no inverse-relation query exists).
func (s *Server) depsTab(ctx context.Context, item mcp.WorkItem) template.HTML {
	id := rowID(item)
	esc := template.HTMLEscapeString
	all := s.milestones(ctx)
	// Relations store the canonical work-item ID, but rows/links use the human
	// shared ID — key the lookup by both so dep targets resolve to a title.
	byID := map[string]mcp.WorkItem{}
	for _, m := range all {
		byID[rowID(m)] = m
		if m.ID != "" {
			byID[m.ID] = m
		}
		if m.SharedID != "" {
			byID[m.SharedID] = m
		}
	}

	var upstream []string
	upSet := map[string]bool{}
	for _, rel := range item.Relations {
		if rel.Type == "depends_on" {
			upstream = append(upstream, rel.TargetWorkItem)
			upSet[rel.TargetWorkItem] = true
			if m, ok := byID[rel.TargetWorkItem]; ok {
				upSet[rowID(m)] = true // also mark the shared id so the picker hides it
			}
		}
	}
	var downstream []mcp.WorkItem
	for _, m := range all {
		if rowID(m) == id {
			continue
		}
		for _, rel := range m.Relations {
			if rel.Type == "depends_on" && rel.TargetWorkItem == id {
				downstream = append(downstream, m)
			}
		}
	}

	depRow := func(targetID, title, status, dir string) string {
		t := title
		if t == "" {
			t = targetID
		}
		remove := ""
		if dir == "up" {
			remove = fmt.Sprintf(`<form method="post" action="/m/%s/dep/remove" hx-post="/m/%s/dep/remove" hx-target="[data-panel-body]" hx-swap="innerHTML" style="margin:0">
  <input type="hidden" name="target" value="%s">
  <button type="submit" style="background:none;border:none;font-family:var(--font-mono);font-size:9px;color:var(--ink-4);cursor:pointer;padding:2px 5px">remove</button>
</form>`, esc(id), esc(id), esc(targetID))
		}
		dirLab := "depends"
		if dir == "down" {
			dirLab = "blocks"
		}
		return fmt.Sprintf(`<div class="dep-row"><a href="/portfolio?open=%s" style="display:flex;align-items:center;gap:8px;flex:1;text-decoration:none;color:inherit">
  <span style="font-family:var(--font-mono);font-size:9px;letter-spacing:0.1em;text-transform:uppercase;color:var(--ink-4);width:54px">%s</span>
  <span class="dx-id" style="color:var(--accent);width:88px">%s</span>
  <span style="flex:1;font-size:13px;color:var(--ink-0);font-weight:500;overflow:hidden;text-overflow:ellipsis;white-space:nowrap">%s</span>
  <span>%s</span></a>%s</div>`,
			esc(targetID), dirLab, esc(targetID), esc(t), statusPill(status), remove)
	}

	var b strings.Builder
	// Upstream.
	b.WriteString(sectionHead("Upstream (this depends on)", len(upstream), ""))
	b.WriteString(`<div class="dep-list">`)
	if len(upstream) == 0 {
		b.WriteString(emptyLine("No upstream dependencies."))
	}
	for _, dep := range upstream {
		m, ok := byID[dep]
		shared := dep
		if ok {
			shared = rowID(m)
		}
		b.WriteString(depRow(shared, m.Title, m.Status, "up"))
	}
	b.WriteString(`</div>`)

	// Add a dependency: a reveal-on-click datalist search over the candidates.
	b.WriteString(fmt.Sprintf(`<details class="dep-add-wrap"><summary class="so-mini-add">+ link dependency</summary>
  <form method="post" action="/m/%s/dep/add" hx-post="/m/%s/dep/add" hx-target="[data-panel-body]" hx-swap="innerHTML" class="dep-add">
  <input name="target" class="so-input" list="dep-cands-%s" placeholder="Search milestones to link…" autocomplete="off">
  <button class="rx-btn rx-btn--ghost" type="submit">+ link</button>
  <datalist id="dep-cands-%s">`, esc(id), esc(id), esc(id), esc(id)))
	for _, m := range all {
		mid := rowID(m)
		if mid == id || upSet[mid] {
			continue
		}
		fmt.Fprintf(&b, `<option value="%s">%s</option>`, esc(mid), esc(m.Title))
	}
	b.WriteString(`</datalist></form></details>`)

	// Downstream.
	b.WriteString(sectionHead("Downstream (blocks)", len(downstream), ""))
	b.WriteString(`<div class="dep-list">`)
	if len(downstream) == 0 {
		b.WriteString(emptyLine("Nothing depends on this milestone."))
	}
	for _, m := range downstream {
		b.WriteString(depRow(rowID(m), m.Title, m.Status, "down"))
	}
	b.WriteString(`</div>`)
	return template.HTML(b.String())
}

// --- C.5: Updates tab ---

// updatesTab renders a merged feed of the work item's observations (with the
// entry_type chip from the Data blob) and its status transitions (the
// work.status_set events filtered to this record) — newest first (panels.jsx
// MilestoneUpdates), plus a quick-entry composer posting a status observation.
func (s *Server) updatesTab(ctx context.Context, item mcp.WorkItem) template.HTML {
	id := rowID(item)
	type feedRow struct {
		when, kind, body, actor string
		transition              bool
		from, to                string
	}
	var rows []feedRow

	for _, o := range item.Observations {
		var d mcp.ObsData
		if len(o.Data) > 0 {
			_ = json.Unmarshal(o.Data, &d)
		}
		kind := d.EntryType
		if kind == "" {
			kind = "note"
		}
		rows = append(rows, feedRow{when: shortDayTS(o.Timestamp), kind: kind, body: o.Summary})
	}
	if evs, err := s.Client.EventsList(ctx, "work.status_set", "", 200); err == nil {
		for _, e := range evs {
			if e.WorkItemID != id {
				continue
			}
			var p struct {
				Status string `json:"status"`
				From   string `json:"from"`
			}
			_ = json.Unmarshal(e.Payload, &p)
			rows = append(rows, feedRow{when: shortDayTS(e.Timestamp), transition: true, from: p.From, to: p.Status, actor: e.ActorID})
		}
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].when > rows[j].when })

	var b strings.Builder
	// Quick-entry composer (a status observation; ⌘+Enter posts).
	b.WriteString(fmt.Sprintf(`<form method="post" action="/m/%s/status-update" hx-post="/m/%s/status-update" hx-target="[data-panel-body]" hx-swap="innerHTML" class="status-post" style="margin-bottom:12px">
  <textarea name="narrative" class="status-post__ta" rows="2" placeholder="Quick update — Cmd+Enter posts a status entry." data-cmd-enter-submit></textarea>
  <div class="status-post__actions"><span style="font-family:var(--font-mono);font-size:10px;color:var(--ink-4)">⌘+↵ to post</span><button class="rx-btn rx-btn--accent" type="submit">Post</button></div>
</form>`, rowID(item), rowID(item)))

	b.WriteString(`<div class="upd-feed-bar">Combined feed · observations and status transitions</div>`)
	if len(rows) == 0 {
		b.WriteString(`<div style="padding:14px 0;font-family:var(--font-serif);font-style:italic;color:var(--ink-3);font-size:13.5px">No updates yet on this milestone.</div>`)
	}
	for _, r := range rows {
		if r.transition {
			fmt.Fprintf(&b, `<div class="upd-feed-row upd-feed-row--transition"><span class="upd-feed-row__when">%s</span><span class="upd-feed-row__chip">%s</span><span class="upd-feed-row__body"><strong>%s</strong> <span style="color:var(--ink-4)">← %s</span> <span style="font-family:var(--font-mono);font-size:10px;color:var(--ink-4)">by %s</span></span></div>`,
				template.HTMLEscapeString(r.when), pill("neutral", "status"),
				template.HTMLEscapeString(statusLabel(r.to)), template.HTMLEscapeString(statusLabel(r.from)),
				template.HTMLEscapeString(r.actor))
			continue
		}
		fmt.Fprintf(&b, `<div class="upd-feed-row"><span class="upd-feed-row__when">%s</span><span class="upd-feed-row__chip">%s</span><span class="upd-feed-row__body">%s</span></div>`,
			template.HTMLEscapeString(r.when), entryKindPill(r.kind), template.HTMLEscapeString(r.body))
	}
	return template.HTML(b.String())
}

// entryKindPill colours an observation entry_type chip.
func entryKindPill(kind string) template.HTML {
	v := "blue"
	switch kind {
	case "risk":
		v = "y"
	case "integrity_call":
		v = "r"
	case "decision":
		v = "plum"
	case "status":
		v = "g"
	}
	return pill(v, strings.ReplaceAll(kind, "_", " "))
}

// --- C.6: Receipts tab ---

// receiptsTab renders the signed event log scoped to this record. EventsList is
// by type only, so we pull the recent all-types window and filter by
// WorkItemID (panels.jsx MilestoneActivity). A per-record events client method
// would let this drop the client-side filter — noted to the coordinator.
func (s *Server) receiptsTab(ctx context.Context, item mcp.WorkItem) template.HTML {
	id := rowID(item)
	evs, err := s.Client.EventsList(ctx, "", "", 400)
	if err != nil {
		return errBody(err)
	}
	var scoped []mcp.Event
	for _, e := range evs {
		if e.WorkItemID == id {
			scoped = append(scoped, e)
		}
	}
	sort.SliceStable(scoped, func(i, j int) bool { return scoped[i].Timestamp > scoped[j].Timestamp })

	var b strings.Builder
	if len(scoped) == 0 {
		b.WriteString(`<div style="padding:14px 0;font-family:var(--font-serif);font-style:italic;color:var(--ink-3);font-size:13.5px">No events scoped to this milestone in the visible window.</div>`)
		return template.HTML(b.String())
	}
	b.WriteString(`<div style="font-family:var(--font-mono);font-size:11.5px">`)
	for _, e := range scoped {
		fmt.Fprintf(&b, `<div class="rx-row" style="padding:6px 0;border-bottom:1px solid var(--hairline);color:var(--ink-2);display:flex;gap:10px"><span style="color:var(--accent);min-width:160px">%s</span><span style="flex:1">%s</span><span style="color:var(--ink-4)">%s</span></div>`,
			template.HTMLEscapeString(e.Type), template.HTMLEscapeString(e.ActorID), template.HTMLEscapeString(shortDayTS(e.Timestamp)))
	}
	b.WriteString(`</div>`)
	return template.HTML(b.String())
}

// --- C.7: panel footer action rows ---

// milestoneFooter renders the Accept-both / Amend(clears) footer for a
// milestone (panels.jsx RecordPanel footer). Accept-both shows until both sides
// are accepted; once aligned, Amend (which clears the ACKs) shows instead.
func milestoneFooter(item mcp.WorkItem) template.HTML {
	id := rowID(item)
	esc := template.HTMLEscapeString
	a, has := latestAck(item)
	aligned := has && a.Specifier == "accepted" && a.Builder == "accepted"
	if has && aligned {
		return template.HTML(fmt.Sprintf(`<form method="post" action="/m/%s/amend" style="margin:0"><input type="hidden" name="amendment_type" value="scope_change"><input type="hidden" name="reason" value="Amendment from panel footer."><button class="rx-btn rx-btn--ghost" type="submit">Amend (clears ACKs)</button></form>`, esc(id)))
	}
	return template.HTML(fmt.Sprintf(`<form method="post" action="/m/%s/ack/both/accept" style="margin:0"><button class="rx-btn rx-btn--accent" type="submit">Accept both</button></form>`, esc(id)))
}

// decisionFooter renders Resolve / Escalate for an open DecisionBlock.
func decisionFooter(item mcp.WorkItem) template.HTML {
	id := rowID(item)
	esc := template.HTMLEscapeString
	if item.Status != "open" {
		return ""
	}
	return template.HTML(fmt.Sprintf(`<form method="post" action="/d/%s/status" style="margin:0;display:flex;gap:8px"><input type="hidden" name="status" value="resolved"><button class="rx-btn rx-btn--accent" type="submit">Resolve</button></form><form method="post" action="/d/%s/status" style="margin:0"><input type="hidden" name="status" value="escalated"><button class="rx-btn rx-btn--ghost" type="submit">Escalate</button></form>`, esc(id), esc(id)))
}

// --- C.8: RFC / Decision / Outcome detail bodies ---

// rfcDetailBody renders the RFC detail panel: summary, acceptance criteria,
// the milestones produced from this RFC (FromRFC lineage), and a "spawn
// milestone from RFC" action (panels.jsx RfcDetail).
func (s *Server) rfcDetailBody(ctx context.Context, item mcp.WorkItem) template.HTML {
	id := rowID(item)
	esc := template.HTMLEscapeString
	var produced []mcp.WorkItem
	for _, m := range s.milestones(ctx) {
		if m.FromRFC() == id {
			produced = append(produced, m)
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, `<div class="so-meta">%s<span>target=%s</span></div>`, statusPill(item.Status), esc(orDash(item.Target())))
	fmt.Fprintf(&b, `<div class="so-line"><span class="lab">Specifier</span><span class="val">%s</span></div>`, actorCell(roleActor(item, "specifier")))
	fmt.Fprintf(&b, `<div class="so-line"><span class="lab">Summary</span><span class="val" style="white-space:pre-wrap">%s</span></div>`, esc(orDash(item.Body)))

	req := ""
	if item.Status == "approved" && len(produced) == 0 {
		req = "approved, nothing spawned"
	}
	b.WriteString(sectionHead("Produced milestones", len(produced), req))
	if len(produced) == 0 {
		b.WriteString(emptyLine("No milestones produced from this RFC yet. Approved RFCs that don't spawn work get flagged."))
	} else {
		b.WriteString(`<div class="dep-list">`)
		for _, m := range produced {
			fmt.Fprintf(&b, `<a class="dep-row" href="/portfolio?open=%s" style="text-decoration:none;color:inherit"><span class="dx-id" style="color:var(--accent);width:88px">%s</span><span style="flex:1;font-size:13px;color:var(--ink-0);font-weight:500;overflow:hidden;text-overflow:ellipsis;white-space:nowrap">%s</span><span>%s</span></a>`,
				esc(rowID(m)), esc(rowID(m)), esc(m.Title), statusPill(m.Status))
		}
		b.WriteString(`</div>`)
	}
	if item.Status == "approved" || item.Status == "resourced" {
		fmt.Fprintf(&b, `<form method="post" action="/rfc/%s/spawn" style="margin-top:10px"><button type="submit" class="so-mini-add" style="width:100%%;text-align:left;cursor:pointer">+ spawn milestone from this RFC</button></form>`, esc(id))
	}
	return template.HTML(b.String())
}

// decisionDetailBody renders the DecisionBlock detail panel with an
// auto-escalation notice driven by idle days (panels.jsx DecisionDetail).
func decisionDetailBody(item mcp.WorkItem) template.HTML {
	esc := template.HTMLEscapeString
	idle := decisionIdleDays(item)
	var b strings.Builder
	fmt.Fprintf(&b, `<div class="so-meta">%s<span>idle %dd</span></div>`, statusPill(item.Status), idle)
	fmt.Fprintf(&b, `<div class="so-line"><span class="lab">Owner</span><span class="val">%s</span></div>`, actorCell(roleActor(item, "owner")))
	fmt.Fprintf(&b, `<div class="so-line"><span class="lab">Summary</span><span class="val" style="white-space:pre-wrap">%s</span></div>`, esc(orDash(item.Body)))
	if item.Status == "open" && idle >= 5 {
		cls := "cdiag"
		var msg string
		if idle >= 9 {
			cls = "cdiag cdiag--violation"
			msg = "Threshold exceeded. Scheduler will emit review_requested → leadership on next poll."
		} else {
			msg = fmt.Sprintf("%d day%s until auto-escalation.", 9-idle, pluralS(9-idle))
		}
		fmt.Fprintf(&b, `<div class="%s" style="margin-top:14px"><div><span class="lab">Auto-escalation</span> %s</div></div>`, cls, template.HTMLEscapeString(msg))
	}
	return template.HTML(b.String())
}

// outcomeDetailBody renders the OutcomeAssessment detail panel: verdict × value
// selects (FieldSet) + a note, plus the landed-low-value pattern note
// (panels.jsx OutcomeDetail).
func outcomeDetailBody(item mcp.WorkItem) template.HTML {
	id := rowID(item)
	esc := template.HTMLEscapeString
	result := item.Field("result")
	value := item.Field("value")

	verdictOpts := selectOptions([][2]string{{"", "—"}, {"achieved", "Achieved"}, {"partial", "Partial"}, {"missed", "Missed"}, {"aborted", "Aborted"}}, result)
	valueOpts := selectOptions([][2]string{{"", "—"}, {"high", "high"}, {"medium", "medium"}, {"low", "low"}}, value)

	var b strings.Builder
	fmt.Fprintf(&b, `<div class="so-meta">%s</div>`, statusPill(item.Status))
	fmt.Fprintf(&b, `<div class="so-line"><span class="lab">Verdict</span><span class="val" style="display:flex;gap:8px;align-items:center">
  <form method="post" action="/o/%s/field" hx-post="/o/%s/field" hx-target="[data-panel-body]" hx-swap="innerHTML" style="margin:0;display:flex;gap:8px;align-items:center">
    <input type="hidden" name="field" value="result">
    <select name="value" class="so-select" onchange="this.form.requestSubmit()">%s</select>
  </form>
  <span>/</span>
  <form method="post" action="/o/%s/field" hx-post="/o/%s/field" hx-target="[data-panel-body]" hx-swap="innerHTML" style="margin:0">
    <input type="hidden" name="field" value="value">
    <select name="value" class="so-select" onchange="this.form.requestSubmit()">%s</select>
  </form>
</span></div>`, esc(id), esc(id), verdictOpts, esc(id), esc(id), valueOpts)
	fmt.Fprintf(&b, `<div class="so-line"><span class="lab">Note</span><span class="val" style="white-space:pre-wrap">%s</span></div>`, esc(orDash(item.Body)))
	if result == "achieved" && value == "low" {
		b.WriteString(`<div class="cdiag" style="margin-top:14px"><div><span class="lab">Pattern</span> Landed but didn't matter. Worth a pattern block on Leadership.</div></div>`)
	}
	return template.HTML(b.String())
}

// selectOptions renders <option> tags from (value,label) pairs, marking sel.
func selectOptions(pairs [][2]string, sel string) string {
	out := ""
	for _, p := range pairs {
		s := ""
		if p[0] == sel {
			s = " selected"
		}
		out += fmt.Sprintf(`<option value="%s"%s>%s</option>`, template.HTMLEscapeString(p[0]), s, template.HTMLEscapeString(p[1]))
	}
	return out
}

// --- small shared helpers ---

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func orVal(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// decisionIdleDays derives days since the last status/update activity from
// UpdatedAt (best-effort; the substrate has no decision-specific idle counter).
func decisionIdleDays(item mcp.WorkItem) int {
	ts := item.UpdatedAt
	if ts == "" {
		ts = item.CreatedAt
	}
	if d, ok := daysSinceTS(ts); ok {
		return d
	}
	return 0
}

// daysSinceTS parses an RFC3339 timestamp and returns whole days elapsed.
func daysSinceTS(ts string) (int, bool) {
	if ts == "" {
		return 0, false
	}
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		// Fall back to a date-only prefix.
		t, err = time.Parse("2006-01-02", shortDayTS(ts))
		if err != nil {
			return 0, false
		}
	}
	d := int(time.Since(t).Hours() / 24)
	if d < 0 {
		d = 0
	}
	return d, true
}
