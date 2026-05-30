// The Phase-5 write path: POST endpoints that perform a single MCP mutation
// and redirect back to the Portfolio sheet with the record panel re-opened,
// plus the live slide-over panel bodies (ACK + Status) that host the controls.
// Every form posts to one of these; each maps to one client mutator. This is
// the seam the read-only foundation left open.
package handlers

import (
	"fmt"
	"html/template"
	"net/http"
	"strings"

	"github.com/lenulus/pf/internal/pilot/mcp"
)

// milestoneStatuses is the RE milestone workflow (matches the meta bundle).
var milestoneStatuses = []string{"draft", "ack_filed", "ack_committed", "in_flight", "shipped", "aborted"}

// registerMutations wires the POST routes onto mux (called from Register).
func (s *Server) registerMutations(mux *http.ServeMux) {
	mux.HandleFunc("POST /m/{id}/status", s.postStatus)
	mux.HandleFunc("POST /m/{id}/ack-file", s.postAckFile)
	mux.HandleFunc("POST /m/{id}/ack/{who}/{action}", s.postAck)
	mux.HandleFunc("POST /m/{id}/amend", s.postAmend)
	mux.HandleFunc("POST /m/{id}/role", s.postRole)
	mux.HandleFunc("POST /m/{id}/classify", s.postClassify)

	// Track C: methodology-state writes (field_set / observe / schedule_set /
	// link). Each posts one MCP mutation, then returns the freshly-rendered tab
	// body for an HTMX swap (or redirects back for a plain form post).
	mux.HandleFunc("POST /m/{id}/ryg", s.postRYG)
	mux.HandleFunc("POST /m/{id}/target", s.postTarget)
	mux.HandleFunc("POST /m/{id}/status-update", s.postStatusUpdate)
	mux.HandleFunc("POST /m/{id}/risk", s.postRisk)
	mux.HandleFunc("POST /m/{id}/next", s.postNext)
	mux.HandleFunc("POST /m/{id}/stages", s.postStages)
	mux.HandleFunc("POST /m/{id}/dep/add", s.postDepAdd)
	mux.HandleFunc("POST /m/{id}/dep/remove", s.postDepRemove)

	// Decision footer (Resolve / Escalate) and Outcome verdict/value field set.
	mux.HandleFunc("POST /d/{id}/status", s.postDecisionStatus)
	mux.HandleFunc("POST /o/{id}/field", s.postOutcomeField)

	// RFC → milestone lineage (spawn + derived_from link).
	mux.HandleFunc("POST /rfc/{id}/spawn", s.postSpawnMilestone)
}

// back redirects to the Portfolio sheet with the panel re-opened on tab.
func back(w http.ResponseWriter, r *http.Request, id, tab string) {
	http.Redirect(w, r, "/portfolio?open="+id+"&tab="+tab, http.StatusSeeOther)
}

// fail renders a minimal error page (mutations are rare enough that a plain
// message is fine; the substrate never silently drops a write).
func fail(w http.ResponseWriter, err error) {
	http.Error(w, "mutation failed: "+err.Error(), http.StatusBadGateway)
}

func (s *Server) postStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.Client.SetStatus(r.Context(), id, r.FormValue("status")); err != nil {
		fail(w, err)
		return
	}
	s.Indicators.Invalidate()
	back(w, r, id, "Status")
}

func (s *Server) postAckFile(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.Client.AckFile(r.Context(), id,
		r.FormValue("scope_summary"), r.FormValue("delivery_timing"),
		r.FormValue("target_outcome"), r.FormValue("acceptance_criteria")); err != nil {
		fail(w, err)
		return
	}
	back(w, r, id, "ACK")
}

func (s *Server) postAck(w http.ResponseWriter, r *http.Request) {
	id, who, action := r.PathValue("id"), r.PathValue("who"), r.PathValue("action")
	// who="both" is the footer's quick Accept-both: accept each side in turn.
	sides := []string{who}
	if who == "both" {
		sides = []string{"specifier", "builder"}
	}
	for _, side := range sides {
		var err error
		switch action {
		case "accept":
			err = s.Client.AckAccept(r.Context(), id, side, r.FormValue("note"))
		case "reject":
			err = s.Client.AckReject(r.Context(), id, side, r.FormValue("note"))
		default:
			http.Error(w, "unknown ack action", http.StatusBadRequest)
			return
		}
		if err != nil {
			fail(w, err)
			return
		}
	}
	s.Indicators.Invalidate()
	s.swapOrBack(w, r, id, "ACK")
}

func (s *Server) postAmend(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.Client.AckAmend(r.Context(), id, r.FormValue("amendment_type"), nil, r.FormValue("reason")); err != nil {
		fail(w, err)
		return
	}
	s.Indicators.Invalidate()
	back(w, r, id, "ACK")
}

func (s *Server) postRole(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.Client.BindRole(r.Context(), id, r.FormValue("role"), r.FormValue("actor")); err != nil {
		fail(w, err)
		return
	}
	back(w, r, id, "ACK")
}

func (s *Server) postClassify(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.Client.Classify(r.Context(), id, r.FormValue("taxonomy"), r.FormValue("node")); err != nil {
		fail(w, err)
		return
	}
	back(w, r, id, "ACK")
}

// swapOrBack returns the freshly-rendered milestone tab body for an HTMX
// request (in-place panel swap) or redirects back to the re-opened panel for a
// plain form post. The HTMX path re-fetches the work item so derived state
// (rollups, RYG, feeds) re-renders after the mutation.
func (s *Server) swapOrBack(w http.ResponseWriter, r *http.Request, id, tab string) {
	if isHTMX(r) {
		item, err := s.Client.WorkGet(r.Context(), id)
		if err != nil {
			renderPartial(w, errBody(err))
			return
		}
		renderPartial(w, s.buildMilestoneTab(r.Context(), item, tab))
		return
	}
	back(w, r, id, tab)
}

// postRYG sets the red/yellow/green health call (work.field_set ryg).
func (s *Server) postRYG(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.Client.FieldSet(r.Context(), id, "ryg", r.FormValue("ryg")); err != nil {
		fail(w, err)
		return
	}
	s.Indicators.Invalidate()
	s.swapOrBack(w, r, id, "Status")
}

// postTarget sets the delivery target + precision (two field_set writes).
func (s *Server) postTarget(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.Client.FieldSet(r.Context(), id, "target", r.FormValue("target")); err != nil {
		fail(w, err)
		return
	}
	if p := r.FormValue("precision"); p != "" {
		if err := s.Client.FieldSet(r.Context(), id, "target_precision", p); err != nil {
			fail(w, err)
			return
		}
	}
	s.swapOrBack(w, r, id, "ACK")
}

// postStatusUpdate records a status-narrative observation (entry_type=status).
func (s *Server) postStatusUpdate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	body := r.FormValue("narrative")
	if strings.TrimSpace(body) == "" {
		s.swapOrBack(w, r, id, "Status")
		return
	}
	if err := s.Client.Observe(r.Context(), id, body, []byte(`{"entry_type":"status"}`)); err != nil {
		fail(w, err)
		return
	}
	// The composer lives on both Status and Updates; honour the caller's tab.
	tab := r.FormValue("tab")
	if tab == "" {
		tab = "Status"
	}
	s.swapOrBack(w, r, id, tab)
}

// postRisk records a risk finding observation (entry_type=risk + severity).
func (s *Server) postRisk(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	body := r.FormValue("body")
	if strings.TrimSpace(body) == "" {
		s.swapOrBack(w, r, id, "Status")
		return
	}
	sev := r.FormValue("severity")
	if sev == "" {
		sev = "medium"
	}
	by := r.FormValue("by")
	data := fmt.Sprintf(`{"entry_type":"risk","severity":%q,"by":%q}`, sev, by)
	if err := s.Client.Observe(r.Context(), id, body, []byte(data)); err != nil {
		fail(w, err)
		return
	}
	s.swapOrBack(w, r, id, "Status")
}

// postNext records a next-step observation (entry_type=next + owner).
func (s *Server) postNext(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	body := r.FormValue("body")
	if strings.TrimSpace(body) == "" {
		s.swapOrBack(w, r, id, "Status")
		return
	}
	data := fmt.Sprintf(`{"entry_type":"next","owner":%q}`, r.FormValue("owner"))
	if err := s.Client.Observe(r.Context(), id, body, []byte(data)); err != nil {
		fail(w, err)
		return
	}
	s.swapOrBack(w, r, id, "Status")
}

// postStages replaces the staged-delivery timeline (work.schedule_set). The
// Stages form posts parallel key/label/date/precision/state arrays; empty rows
// (no label and no key) are dropped so the blank add-row doesn't persist.
func (s *Server) postStages(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := r.ParseForm(); err != nil {
		fail(w, err)
		return
	}
	keys := r.Form["key"]
	labels := r.Form["label"]
	dates := r.Form["date"]
	precs := r.Form["precision"]
	states := r.Form["state"]
	var stages []mcp.Stage
	for i := range labels {
		key := at(keys, i)
		label := strings.TrimSpace(at(labels, i))
		if label == "" && strings.TrimSpace(key) == "" {
			continue
		}
		if key == "" {
			key = slugify(label)
		}
		stages = append(stages, mcp.Stage{
			Key: key, Label: label, Date: at(dates, i),
			Precision: at(precs, i), State: at(states, i),
		})
	}
	if err := s.Client.ScheduleSet(r.Context(), id, stages); err != nil {
		fail(w, err)
		return
	}
	s.swapOrBack(w, r, id, "Stages")
}

// postDepAdd links an upstream dependency (work.linked depends_on).
func (s *Server) postDepAdd(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	target := strings.TrimSpace(r.FormValue("target"))
	if target == "" {
		s.swapOrBack(w, r, id, "Deps")
		return
	}
	if err := s.Client.Link(r.Context(), id, "depends_on", target); err != nil {
		fail(w, err)
		return
	}
	s.swapOrBack(w, r, id, "Deps")
}

// postDepRemove unlinks an upstream dependency (work.unlinked depends_on).
func (s *Server) postDepRemove(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.Client.Unlink(r.Context(), id, "depends_on", r.FormValue("target")); err != nil {
		fail(w, err)
		return
	}
	s.swapOrBack(w, r, id, "Deps")
}

// postDecisionStatus drives the Decision footer (Resolve / Escalate).
func (s *Server) postDecisionStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.Client.SetStatus(r.Context(), id, r.FormValue("status")); err != nil {
		fail(w, err)
		return
	}
	s.Indicators.Invalidate()
	http.Redirect(w, r, "/decisions?open="+id, http.StatusSeeOther)
}

// postOutcomeField sets an Outcome verdict/value projection field (field_set).
func (s *Server) postOutcomeField(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.Client.FieldSet(r.Context(), id, r.FormValue("field"), r.FormValue("value")); err != nil {
		fail(w, err)
		return
	}
	if isHTMX(r) {
		item, err := s.Client.WorkGet(r.Context(), id)
		if err != nil {
			renderPartial(w, errBody(err))
			return
		}
		renderPartial(w, outcomeDetailBody(item))
		return
	}
	http.Redirect(w, r, "/outcomes?open="+id, http.StatusSeeOther)
}

// postSpawnMilestone creates a milestone from an RFC and links its lineage
// (WorkCreate + Link derived_from), then opens the new milestone's panel.
func (s *Server) postSpawnMilestone(w http.ResponseWriter, r *http.Request) {
	rfcID := r.PathValue("id")
	rfc, err := s.Client.WorkGet(r.Context(), rfcID)
	title := "New milestone — name me"
	if err == nil && rfc.Title != "" {
		title = rfc.Title + " — v1"
	}
	newID, err := s.Client.WorkCreate(r.Context(), "milestone", title, "")
	if err != nil {
		fail(w, err)
		return
	}
	if err := s.Client.Link(r.Context(), newID, "derived_from", rfcID); err != nil {
		fail(w, err)
		return
	}
	s.Indicators.Invalidate()
	http.Redirect(w, r, "/portfolio?open="+newID, http.StatusSeeOther)
}

// at returns the i-th element of xs or "".
func at(xs []string, i int) string {
	if i < len(xs) {
		return xs[i]
	}
	return ""
}

// slugify derives a lowercase, dash-separated key from a label.
func slugify(s string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash && b.Len() > 0 {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

// --- live panel bodies ---

// buildMilestonePanelBody renders the editable body for the opened milestone's
// active tab. ACK / Status / Stages are pure projections of the work item; the
// Deps / Updates / Receipts tabs need extra substrate queries and so are
// rendered through s.buildMilestoneTab (panels_detail.go) — the coordinator's
// Portfolio handler should call that instead so every tab opens server-side on
// first load (this wrapper renders the tabs that don't need a query and points
// the rest at the HTMX tab endpoint).
func buildMilestonePanelBody(item mcp.WorkItem, tab string) template.HTML {
	switch tab {
	case "Status":
		return statusTab(item)
	case "ACK":
		return ackTab(item)
	case "Stages":
		return stagesTab(item)
	default:
		// Deps / Updates / Receipts: the HTMX tab swap (panel.js) fetches the
		// live body; this is the no-JS / first-paint fallback.
		return template.HTML(fmt.Sprintf(
			`<div class="empty-line" hx-get="/partials/panel/%s/tab/%s" hx-trigger="load" hx-swap="innerHTML" hx-target="this">Loading %s…</div>`,
			template.HTMLEscapeString(rowID(item)), template.HTMLEscapeString(tab), template.HTMLEscapeString(tab)))
	}
}

// statusTab renders the Status editing surface (panels.jsx MilestoneStatus):
// the RYG segmented control, the current-status card + post-update composer,
// then Next steps ABOVE Risks, with the "required for Red/Yellow" chip when RYG
// is r/y and no risk is logged.
func statusTab(item mcp.WorkItem) template.HTML {
	id := rowID(item)
	esc := template.HTMLEscapeString
	var b strings.Builder

	// Lifecycle status (kept from the foundation — the workflow transition).
	var opts string
	for _, st := range milestoneStatuses {
		sel := ""
		if st == item.Status {
			sel = " selected"
		}
		opts += fmt.Sprintf(`<option value="%s"%s>%s</option>`, st, sel, statusLabel(st))
	}
	fmt.Fprintf(&b, `<div class="so-section" style="margin-top:0"><span>Lifecycle status</span></div>
<form method="post" action="/m/%s/status" style="display:flex;gap:8px;align-items:center;margin-bottom:8px">
  <select name="status" class="so-select">%s</select>
  <button class="rx-btn rx-btn--accent" type="submit">Set status</button>
</form>`, esc(id), opts)

	// RYG segmented control.
	ryg := item.RYG()
	b.WriteString(`<div class="so-section"><span>Health</span></div><div class="ryg-row">`)
	for _, v := range []struct{ key, lab string }{{"g", "Green"}, {"y", "Yellow"}, {"r", "Red"}} {
		active := ""
		if ryg == v.key {
			active = " is-active"
		}
		fmt.Fprintf(&b, `<form method="post" action="/m/%s/ryg" hx-post="/m/%s/ryg" hx-target="[data-panel-body]" hx-swap="innerHTML" style="margin:0">
  <input type="hidden" name="ryg" value="%s">
  <button type="submit" class="ryg-btn ryg-btn--%s%s" style="width:100%%"><span class="ryg-btn__dot"></span><span class="ryg-btn__lab">%s</span></button>
</form>`, esc(id), esc(id), v.key, v.key, active, v.lab)
	}
	b.WriteString(`</div>`)

	// Current status card.
	b.WriteString(`<div class="so-section"><span>Current status</span></div>`)
	if narrative := item.StatusNarrative(); narrative != "" {
		fmt.Fprintf(&b, `<div class="status-now"><div class="status-now__body">%s</div><div class="status-now__meta">posted %s</div></div>`,
			esc(narrative), esc(orDash(shortDayTS(item.StatusUpdatedAt()))))
	} else {
		b.WriteString(`<div class="status-now status-now--empty">No status posted yet. The silence is itself data — file one when you can.</div>`)
	}
	// Post-update composer (⌘+Enter posts a status observation).
	fmt.Fprintf(&b, `<form method="post" action="/m/%s/status-update" hx-post="/m/%s/status-update" hx-target="[data-panel-body]" hx-swap="innerHTML" class="status-post">
  <div class="status-post__lab">Replace with new update</div>
  <textarea name="narrative" class="status-post__ta" rows="3" placeholder="State of the milestone in one paragraph. Cmd+Enter files it." data-cmd-enter-submit></textarea>
  <div class="status-post__actions"><span style="font-family:var(--font-mono);font-size:10px;color:var(--ink-4)">⌘+↵ to post · supersedes the current status above</span><button class="rx-btn rx-btn--accent" type="submit">Post update</button></div>
</form>`, esc(id), esc(id))

	// Next steps (ABOVE risks).
	next := item.NextSteps()
	b.WriteString(sectionHead("Next steps", len(next), ""))
	b.WriteString(`<div class="upd-list">`)
	if len(next) == 0 {
		b.WriteString(emptyLine("No explicit next steps. Implicit ones don't count."))
	}
	for _, n := range next {
		fmt.Fprintf(&b, `<div class="risk-row" style="grid-template-columns:1fr 130px 100px"><span style="font-family:var(--font-serif);font-size:13px;color:var(--ink-1);line-height:1.45">%s</span><span>%s</span><span style="font-family:var(--font-mono);font-size:10px;color:var(--ink-4);text-align:right">by %s</span></div>`,
			esc(n.Body), actorCell(n.Owner), esc(n.When))
	}
	b.WriteString(`</div>`)
	fmt.Fprintf(&b, `<form method="post" action="/m/%s/next" hx-post="/m/%s/next" hx-target="[data-panel-body]" hx-swap="innerHTML" class="so-stack" style="margin-top:6px;flex-direction:row;gap:6px">
  <input name="body" class="so-input" style="flex:1;width:auto" placeholder="+ next step">
  <input name="owner" class="so-input" style="width:110px" placeholder="owner">
  <button class="rx-btn rx-btn--ghost" type="submit">Add</button>
</form>`, esc(id), esc(id))

	// Risks (with the required-for-Red/Yellow chip).
	risks := item.Risks()
	req := ""
	needsRisk := (ryg == "r" || ryg == "y") && len(risks) == 0
	if needsRisk {
		if ryg == "r" {
			req = "required for Red"
		} else {
			req = "required for Yellow"
		}
	}
	b.WriteString(sectionHead("Risks", len(risks), req))
	b.WriteString(`<div class="upd-list">`)
	if len(risks) == 0 {
		if needsRisk {
			label := "Yellow"
			if ryg == "r" {
				label = "Red"
			}
			b.WriteString(`<div class="empty-line"><strong>RYG is ` + label + ` — the substrate expects at least one risk.</strong></div>`)
		} else {
			b.WriteString(emptyLine("No risks logged. If the team has felt friction, name it."))
		}
	}
	for _, rk := range risks {
		fmt.Fprintf(&b, `<div class="risk-row"><span>%s</span><span style="font-family:var(--font-serif);font-size:13px;color:var(--ink-1);line-height:1.45">%s</span><span style="font-family:var(--font-mono);font-size:10px;color:var(--ink-4);text-align:right">%s · %s</span></div>`,
			riskSevPill(rk.Severity), esc(rk.Body), esc(rk.By), esc(rk.When))
	}
	b.WriteString(`</div>`)
	fmt.Fprintf(&b, `<form method="post" action="/m/%s/risk" hx-post="/m/%s/risk" hx-target="[data-panel-body]" hx-swap="innerHTML" class="so-stack" style="margin-top:6px;flex-direction:row;gap:6px">
  <input name="body" class="so-input" style="flex:1;width:auto" placeholder="+ log risk">
  <select name="severity" class="so-select"><option value="high">high</option><option value="medium" selected>medium</option><option value="low">low</option></select>
  <button class="rx-btn rx-btn--ghost" type="submit">Add</button>
</form>`, esc(id), esc(id))

	return template.HTML(b.String())
}

// riskSevPill colours a risk severity token (high=red, medium=yellow, low=neutral).
func riskSevPill(sev string) template.HTML {
	switch sev {
	case "high":
		return pill("r", "high")
	case "low":
		return pill("neutral", "low")
	default:
		return pill("y", orVal(sev, "medium"))
	}
}

func ackTab(item mcp.WorkItem) template.HTML {
	id := rowID(item)
	esc := template.HTMLEscapeString
	a, has := latestAck(item)

	var b strings.Builder

	// Header meta line (status · alignment · RYG · kind) + diagnostics stack.
	rollup := "both_pending"
	if has {
		rollup = a.Rollup()
	}
	fmt.Fprintf(&b, `<div class="so-meta">%s%s%s<span style="margin-left:auto">kind=milestone</span></div>%s`,
		string(statusPill(item.Status)), string(rollupPill(rollup)), string(rygChip(item.RYG())), diagStack(item))

	// --- Roles & classification: avatar FieldRows w/ click-to-edit picker. ---
	b.WriteString(soSection("Roles & classification", "Specifier authors · Builder commits · Pilot observes"))
	b.WriteString(fieldRow("Specifier", string(editableActorCell(id, "specifier", roleActor(item, "specifier")))))
	b.WriteString(fieldRow("Builder", string(editableActorCell(id, "builder", roleActor(item, "builder")))))
	b.WriteString(fieldRow("Pilot", string(editableActorCell(id, "pilot", roleActor(item, "pilot")))))
	b.WriteString(fieldRow("Org node", classEdit(id, "org", classNode(item, "org"))))
	b.WriteString(fieldRow("Product", classEdit(id, "product", classNode(item, "product"))))

	// --- The commitment: delivery target + scope/outcome/acceptance + stages. ---
	b.WriteString(soSection("The commitment", "what the Specifier and Builder are aligning to: scope, schedule, outcome"))
	b.WriteString(targetEditor(item))
	if rfc := item.FromRFC(); rfc != "" {
		b.WriteString(fieldRow("Origin RFC", fmt.Sprintf(
			`<span class="origin-rfc"><a class="dx-id" style="color:var(--accent)" href="/rfcs?open=%s">%s</a> <span class="origin-rfc__title">spawned from this RFC &rarr;</span></span>`,
			esc(rfc), esc(rfc))))
	}
	if has {
		b.WriteString(fieldRow("Scope", commitVal(a.ScopeSummary)))
		b.WriteString(fieldRow("Target outcome", commitVal(a.TargetOutcome)))
		b.WriteString(fieldRow("Acceptance", commitVal(a.AcceptanceCriteria)))
	}
	b.WriteString(stagesPreview(item))

	// --- The ACKs: two stand-behind cards, or the file form. ---
	if !has {
		b.WriteString(soSection("The ACKs", "no commitment filed yet"))
		fmt.Fprintf(&b, `<form method="post" action="/m/%s/ack-file" hx-post="/m/%s/ack-file" hx-target="[data-panel-body]" hx-swap="innerHTML" class="ack-file">
  <input name="scope_summary" class="so-input" placeholder="Scope — what this milestone commits to">
  <input name="delivery_timing" class="so-input" placeholder="Delivery timing (e.g. 2026 Q3)">
  <input name="target_outcome" class="so-input" placeholder="Target outcome">
  <input name="acceptance_criteria" class="so-input" placeholder="Acceptance criteria">
  <button class="rx-btn rx-btn--accent" type="submit">File ACK</button>
</form>`, esc(id), esc(id))
	} else {
		b.WriteString(soSection("The ACKs", "each role independently stands behind (or rejects)"))
		b.WriteString(`<div class="ack-stack">`)
		b.WriteString(ackSideCard(id, "specifier", roleActor(item, "specifier"), a.Specifier))
		b.WriteString(ackSideCard(id, "builder", roleActor(item, "builder"), a.Builder))
		b.WriteString(`</div>`)
		// Amend (auto-clears on material change).
		fmt.Fprintf(&b, `<form method="post" action="/m/%s/amend" hx-post="/m/%s/amend" hx-target="[data-panel-body]" hx-swap="innerHTML" class="so-amend">
  <select name="amendment_type" class="so-select">
    <option value="scope_change">scope_change</option>
    <option value="timeline_change">timeline_change</option>
    <option value="target_change">target_change</option>
    <option value="clarification">clarification</option>
  </select>
  <input name="reason" class="so-input" placeholder="amendment reason">
  <button class="rx-btn rx-btn--ghost" type="submit">Amend</button>
</form>`, esc(id), esc(id))
	}

	// ACK history — loaded on demand via HTMX (needs the event feed).
	fmt.Fprintf(&b, `<div hx-get="/partials/panel/%s/ack-history" hx-trigger="load" hx-swap="innerHTML" hx-target="this">%s</div>`,
		esc(id), sectionHead("ACK history", -1, ""))

	return template.HTML(b.String())
}

// soSection renders a prototype section header (flex bar: label + optional
// italic hint + trailing rule). Field rows are SIBLINGS after it, not children.
func soSection(title, hint string) string {
	out := `<div class="so-section"><span>` + template.HTMLEscapeString(title) + `</span>`
	if hint != "" {
		out += `<span class="so-section__hint">` + template.HTMLEscapeString(hint) + `</span>`
	}
	return out + `</div>`
}

// fieldRow renders a label/value row (panels.jsx FieldRow → .so-line .lab .val).
func fieldRow(label, valHTML string) string {
	return `<div class="so-line"><span class="lab">` + template.HTMLEscapeString(label) +
		`</span><span class="val">` + valHTML + `</span></div>`
}

// commitVal renders a (read-only) multiline commitment field value, or a
// placeholder. The filed scope/outcome/criteria change via amend/re-file.
func commitVal(s string) string {
	if strings.TrimSpace(s) == "" {
		return `<span class="placeholder">—</span>`
	}
	return `<span style="font-size:13px;color:var(--ink-1);line-height:1.5">` + template.HTMLEscapeString(s) + `</span>`
}

// classNode returns the work item's classification node slug for a taxonomy.
func classNode(item mcp.WorkItem, tax string) string {
	for _, c := range item.Classifications {
		if c.TaxonomySlug == tax {
			return c.NodeSlug
		}
	}
	return ""
}

// classEdit renders a classification value (mono node slug) + a compact inline
// classify form, in the panel FieldRow value slot.
func classEdit(id, tax, slug string) string {
	esc := template.HTMLEscapeString
	disp := `<span class="placeholder">unclassified</span>`
	if slug != "" {
		disp = `<span style="font-family:var(--font-mono);font-size:11.5px;color:var(--ink-2)">` + esc(slug) + `</span>`
	}
	return `<span class="val--inline">` + disp +
		`<form method="post" action="/m/` + esc(id) + `/classify" hx-post="/m/` + esc(id) + `/classify" hx-target="[data-panel-body]" hx-swap="innerHTML" style="display:flex;gap:4px;margin:0;flex:1;min-width:0">` +
		`<input type="hidden" name="taxonomy" value="` + esc(tax) + `">` +
		`<input name="node" class="so-input" style="width:auto;flex:1;min-width:110px" placeholder="` + esc(tax) + ` node slug" value="` + esc(slug) + `">` +
		`<button class="rx-btn rx-btn--ghost rx-btn--sm" type="submit">Set</button></form></span>`
}

// ackSideCard renders one role's stand-behind card (panels.jsx AckSide): who +
// actor avatar, the big state pill, and the Accept/Reject control group.
func ackSideCard(id, who, actor, state string) string {
	esc := template.HTMLEscapeString
	if state == "" {
		state = "pending"
	}
	label := "Specifier ACK"
	if who == "builder" {
		label = "Builder ACK"
	}
	actorHTML := ""
	if actor != "" {
		actorHTML = `<span class="ack-side__actor">` + string(actorCell(actor)) + `</span>`
	}
	btn := func(action, variant, lab string) string {
		active := ""
		if (action == "accept" && state == "accepted") || (action == "reject" && state == "rejected") {
			active = " is-active"
		}
		return `<form method="post" action="/m/` + esc(id) + `/ack/` + esc(who) + `/` + esc(action) +
			`" hx-post="/m/` + esc(id) + `/ack/` + esc(who) + `/` + esc(action) +
			`" hx-target="[data-panel-body]" hx-swap="innerHTML" style="margin:0">` +
			`<button type="submit" class="ackpop__btn ackpop__btn--` + variant + active + `">` + lab + `</button></form>`
	}
	return `<div class="ack-side ack-side--` + esc(state) + `">` +
		`<div class="ack-side__head"><span class="ack-side__who">` + label + `</span>` + actorHTML + `</div>` +
		`<div class="ack-side__state">` +
		`<span class="dx-pill dx-pill--` + ackVariant(state) + `" style="font-size:12px;padding:4px 12px;height:24px">` + ackLabel(state) + `</span>` +
		`<div class="ack-side__btns">` + btn("accept", "g", "Accept") + btn("reject", "r", "Reject") + `</div>` +
		`</div></div>`
}

func ackVariant(state string) string {
	switch state {
	case "accepted":
		return "g"
	case "rejected":
		return "r"
	default:
		return "neutral"
	}
}

func ackLabel(state string) string {
	switch state {
	case "accepted":
		return "Accepted"
	case "rejected":
		return "Rejected"
	default:
		return "Pending"
	}
}

// rygChip renders a small RYG health pill for the panel meta line, or "".
func rygChip(ryg string) template.HTML {
	switch ryg {
	case "g":
		return pill("g", "Green")
	case "y":
		return pill("y", "Yellow")
	case "r":
		return pill("r", "Red")
	default:
		return ""
	}
}
