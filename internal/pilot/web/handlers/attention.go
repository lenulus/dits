// The "For you" Attention view (§9.5) and its inline-action partial endpoints.
// Attention computes nine substrate-derived buckets client-side from the live
// milestone / decision / outcome / RFC lists, then renders each as an AttSection
// of AttRows. Each row carries inline HTMX action buttons (Accept/Reject/
// Resolve/Escalate/Spawn) that POST to /partials/attention/... — the handler
// performs one MCP mutation and swaps the row to a "done" state in place, with
// no full-page reload.
//
// "me" actor: Pilot has no auth yet (Track F), so there is no per-user filter —
// every bucket shows all matching work across the project. The summary line
// notes this. When identity lands, the buckets gain a `me`-scoped predicate.
package handlers

import (
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/lenulus/pf/internal/pilot/mcp"
)

// attRow is one computed Attention entry. The action buttons are rendered from
// the bucket the row belongs to (see attActions).
type attRow struct {
	sev   string // info | warn | crit
	chip  template.HTML
	id    string
	title string
	ctx   string
	// actions rendered into the row's trailing action cell.
	actions template.HTML
}

// buildAttentionBody computes the nine Attention buckets and renders them.
//
//	milestones — kind=milestone (ACKs, targets, stale status, risks, diagnostics)
//	decisions  — kind=decision_block (idle ≥5d)
//	outcomes   — kind=outcome_assessment (status overdue)
//	rfcs       — kind=rfc (approved/resourced, dead-on-arrival ≥30d)
func buildAttentionBody(milestones, decisions, outcomes, rfcs []mcp.WorkItem) template.HTML {
	now := time.Now()

	type bucket struct {
		title string
		sev   string
		rows  []attRow
	}

	// --- ACKs needing attention (pending) ---
	var ackPending []attRow
	for _, m := range milestones {
		a, ok := latestAck(m)
		if !ok {
			continue
		}
		if a.Specifier == "pending" {
			ackPending = append(ackPending, attRow{
				sev: "warn", chip: pill("y", "S-ACK pending"), id: rowID(m), title: m.Title,
				ctx:     "Specifier side has not accepted the commitment",
				actions: ackActions(rowID(m), "specifier"),
			})
		}
		if a.Builder == "pending" {
			ackPending = append(ackPending, attRow{
				sev: "warn", chip: pill("y", "B-ACK pending"), id: rowID(m), title: m.Title,
				ctx:     "Builder side has not accepted the commitment",
				actions: ackActions(rowID(m), "builder"),
			})
		}
	}

	// --- Targets passed or imminent (≤21d) ---
	var targetPassed, targetSoon []attRow
	for _, m := range milestones {
		if m.Status == "shipped" || m.Status == "aborted" {
			continue
		}
		d, ok := daysUntilTarget(m.Target(), m.TargetPrecision(), now)
		if !ok {
			continue
		}
		switch {
		case d < 0:
			targetPassed = append(targetPassed, attRow{
				sev: "crit", chip: pill("r", fmt.Sprintf("%dd past", -d)), id: rowID(m), title: m.Title,
				ctx:     fmt.Sprintf("Target %s · status %s", formatTarget(m.Target(), m.TargetPrecision()), statusLabel(m.Status)),
				actions: statusActions(rowID(m)),
			})
		case d <= 21:
			targetSoon = append(targetSoon, attRow{
				sev: "warn", chip: pill("y", fmt.Sprintf("%dd to target", d)), id: rowID(m), title: m.Title,
				ctx:     fmt.Sprintf("Target %s · status %s", formatTarget(m.Target(), m.TargetPrecision()), statusLabel(m.Status)),
				actions: openOnlyAction(rowID(m)),
			})
		}
	}

	// --- Stale status updates (>14d on live work) ---
	var stale []attRow
	for _, m := range milestones {
		if m.Status != "in_flight" && m.Status != "ack_committed" {
			continue
		}
		d, ok := daysSince(m.StatusUpdatedAt(), now)
		if !ok || d <= 14 {
			continue
		}
		stale = append(stale, attRow{
			sev: "warn", chip: pill("y", fmt.Sprintf("%dd silent", d)), id: rowID(m), title: m.Title,
			ctx:     fmt.Sprintf("No status update in %d days · RYG %s", d, strings.ToUpper(rygLabel(m.RYG()))),
			actions: statusActions(rowID(m)),
		})
	}

	// --- High-severity risks ---
	var risksHigh []attRow
	for _, m := range milestones {
		for _, rk := range m.Risks() {
			if rk.Severity != "high" {
				continue
			}
			risksHigh = append(risksHigh, attRow{
				sev: "crit", chip: pill("r", "high"), id: rowID(m), title: m.Title,
				ctx:     rk.Body,
				actions: openOnlyAction(rowID(m)),
			})
		}
	}

	// --- Diagnostics ---
	var diagnostics []attRow
	for _, m := range milestones {
		if len(m.Diagnostics) == 0 {
			continue
		}
		msgs := make([]string, 0, len(m.Diagnostics))
		for _, d := range m.Diagnostics {
			msgs = append(msgs, d.Message)
		}
		plural := "s"
		if len(m.Diagnostics) == 1 {
			plural = ""
		}
		diagnostics = append(diagnostics, attRow{
			sev: "warn", chip: pill("y", fmt.Sprintf("%d flag%s", len(m.Diagnostics), plural)),
			id: rowID(m), title: m.Title, ctx: strings.Join(msgs, " · "),
			actions: openOnlyAction(rowID(m)),
		})
	}

	// --- Decisions idle ≥5d ---
	var idleDecisions []attRow
	for _, d := range decisions {
		if d.Status != "open" {
			continue
		}
		idle, ok := daysSince(d.UpdatedAt, now)
		if !ok || idle < 5 {
			continue
		}
		sev, variant := "warn", "y"
		if idle >= 9 {
			sev, variant = "crit", "r"
		}
		idleDecisions = append(idleDecisions, attRow{
			sev: sev, chip: pill(variant, fmt.Sprintf("%dd idle", idle)), id: rowID(d), title: d.Title,
			ctx:     fmt.Sprintf("Idle %d days · open DecisionBlock", idle),
			actions: decisionActions(rowID(d)),
		})
	}

	// --- Outcomes overdue ---
	var overdueOutcomes []attRow
	for _, o := range outcomes {
		if o.Status != "overdue" {
			continue
		}
		overdueOutcomes = append(overdueOutcomes, attRow{
			sev: "warn", chip: pill("r", "overdue"), id: rowID(o), title: o.Title,
			ctx:     "Assessment SLA has passed",
			actions: openOnlyAction(rowID(o)),
		})
	}

	// --- Rejected ACKs ---
	var rejected []attRow
	for _, m := range milestones {
		a, ok := latestAck(m)
		if !ok {
			continue
		}
		if a.Specifier == "rejected" || a.Builder == "rejected" {
			rejected = append(rejected, attRow{
				sev: "crit", chip: pill("r", "rejected"), id: rowID(m), title: m.Title,
				ctx:     "An ACK side rejected the commitment — re-file or amend",
				actions: openOnlyAction(rowID(m)),
			})
		}
	}

	// --- Approved RFCs dead on arrival (≥30d, no milestone derived) ---
	derivedFrom := map[string]bool{}
	for _, m := range milestones {
		if rfc := m.FromRFC(); rfc != "" {
			derivedFrom[rfc] = true
		}
	}
	var deadRFCs []attRow
	for _, r := range rfcs {
		if r.Status != "approved" && r.Status != "resourced" {
			continue
		}
		if derivedFrom[rowID(r)] || derivedFrom[r.ID] {
			continue
		}
		d, ok := daysSince(r.UpdatedAt, now)
		if !ok || d < 30 {
			continue
		}
		deadRFCs = append(deadRFCs, attRow{
			sev: "warn", chip: pill("plum", "RFC · 30d+ idle"), id: rowID(r), title: r.Title,
			ctx:     fmt.Sprintf("Approved %d days ago with nothing spawned", d),
			actions: spawnActions(rowID(r)),
		})
	}

	buckets := []bucket{
		{"ACKs needing attention", "warn", ackPending},
		{"Targets passed or imminent", critIf(len(targetPassed) > 0), append(append([]attRow{}, targetPassed...), targetSoon...)},
		{"Stale status updates", "warn", stale},
		{"High-severity risks on your work", "crit", risksHigh},
		{"Diagnostics on your work", "warn", diagnostics},
		{"Decisions idle on your work", critIf(anyCrit(idleDecisions)), idleDecisions},
		{"Outcome assessments overdue", "warn", overdueOutcomes},
		{"Rejected ACKs in your view", "crit", rejected},
		{"Approved RFCs with nothing spawned", "warn", deadRFCs},
	}

	total := 0
	for _, bk := range buckets {
		total += len(bk.rows)
	}

	var b strings.Builder
	fmt.Fprintf(&b, `<div class="att"><div class="att-summary"><div class="att-summary__line"><span class="att-summary__count">%d</span><span class="att-summary__lab">things across the project.</span></div><div class="att-summary__quip">No identity yet — every bucket shows all work. Specific is kind; the substrate catches every change.</div></div>`, total)
	for _, bk := range buckets {
		writeAttSection(&b, bk.title, bk.sev, bk.rows)
	}
	if total == 0 {
		b.WriteString(`<div class="empty-line">Nothing on you today. Watch for silence — that's usually data.</div>`)
	}
	b.WriteString(`</div>`)
	return template.HTML(b.String())
}

// writeAttSection renders one AttSection (skipped when empty).
func writeAttSection(b *strings.Builder, title, sev string, rows []attRow) {
	if len(rows) == 0 {
		return
	}
	fmt.Fprintf(b, `<div class="att-section"><div class="att-section__head"><span class="att-sev att-sev--%s"></span><h3 class="att-section__title">%s</h3><span class="att-section__count">%d</span></div><div class="att-rows">`,
		sev, template.HTMLEscapeString(title), len(rows))
	for _, r := range rows {
		b.WriteString(string(renderAttRow(r)))
	}
	b.WriteString(`</div></div>`)
}

// renderAttRow renders one Attention row with its inline action cell. The row
// is a hx-target so an action can swap it to a done state in place.
func renderAttRow(r attRow) template.HTML {
	return template.HTML(fmt.Sprintf(
		`<div class="att-row" id="att-%s"><span class="att-sev att-sev--%s"></span><span class="att-row__chip">%s</span><a class="att-row__id dx-id" href="/portfolio?open=%s">%s</a><span class="att-row__body"><span class="att-row__title">%s</span><span class="att-row__ctx">%s</span></span><span class="att-row__actions">%s</span></div>`,
		template.HTMLEscapeString(attDOMID(r.id)), template.HTMLEscapeString(r.sev), r.chip,
		template.HTMLEscapeString(r.id), template.HTMLEscapeString(r.id),
		template.HTMLEscapeString(r.title), template.HTMLEscapeString(r.ctx), r.actions))
}

// attDOMID makes a slug safe for a DOM id (the shared id may contain a slash).
func attDOMID(id string) string {
	return strings.NewReplacer("/", "-", " ", "-", ".", "-").Replace(id)
}

// --- inline action button groups (HTMX POST → swap the row) ---

// hxBtn renders one HTMX action button that POSTs to path and swaps the parent
// att-row in place (hx-target the enclosing row, outerHTML swap).
func hxBtn(label, variant, path, rowDOMID string) string {
	return fmt.Sprintf(
		`<button class="rx-btn rx-btn--%s rx-btn--sm" hx-post="%s" hx-target="#att-%s" hx-swap="outerHTML">%s</button>`,
		template.HTMLEscapeString(variant), template.HTMLEscapeString(path),
		template.HTMLEscapeString(rowDOMID), template.HTMLEscapeString(label))
}

func ackActions(id, who string) template.HTML {
	dom := attDOMID(id)
	return template.HTML(
		hxBtn("Accept", "accent", "/partials/attention/ack/"+id+"/"+who+"/accept", dom) +
			hxBtn("Reject", "ghost", "/partials/attention/ack/"+id+"/"+who+"/reject", dom))
}

func statusActions(id string) template.HTML {
	dom := attDOMID(id)
	return template.HTML(
		hxBtn("Resolve", "accent", "/partials/attention/status/"+id+"/in_flight", dom) +
			hxBtn("Escalate", "ghost", "/partials/attention/escalate/"+id, dom))
}

func decisionActions(id string) template.HTML {
	dom := attDOMID(id)
	return template.HTML(
		hxBtn("Resolve", "accent", "/partials/attention/status/"+id+"/resolved", dom) +
			hxBtn("Escalate", "ghost", "/partials/attention/status/"+id+"/escalated", dom))
}

func spawnActions(id string) template.HTML {
	dom := attDOMID(id)
	return template.HTML(
		hxBtn("Spawn milestone", "accent", "/partials/attention/spawn/"+id, dom) +
			openLink(id))
}

func openOnlyAction(id string) template.HTML {
	return template.HTML(openLink(id))
}

// openLink is a plain (non-HTMX) Open button routing to the record's panel.
func openLink(id string) string {
	return fmt.Sprintf(`<a class="rx-btn rx-btn--ghost rx-btn--sm" href="/portfolio?open=%s">Open</a>`,
		template.HTMLEscapeString(id))
}

// attDone renders the row replacement after a successful inline action: the row
// collapses to a muted "done" line so the queue visibly shrinks.
func attDone(id, label string) template.HTML {
	return template.HTML(fmt.Sprintf(
		`<div class="att-row" id="att-%s"><span class="att-sev att-sev--info"></span><span class="att-row__chip">%s</span><span class="att-row__id dx-id">%s</span><span class="att-row__body"><span class="att-row__title" style="color:var(--ink-3)">%s</span><span class="att-row__ctx">Filed as a signed event — refresh to recompute the queue.</span></span><span class="att-row__actions"></span></div>`,
		template.HTMLEscapeString(attDOMID(id)), pill("g", "done"),
		template.HTMLEscapeString(id), template.HTMLEscapeString(label)))
}

// --- partial endpoints ---

// registerAttentionPartials wires the inline Attention action routes. Each
// performs one MCP mutation and swaps the originating row to a done state.
func (s *Server) registerAttentionPartials(mux *http.ServeMux) {
	mux.HandleFunc("POST /partials/attention/ack/{id}/{who}/{action}", s.attAck)
	mux.HandleFunc("POST /partials/attention/status/{id}/{status}", s.attStatus)
	mux.HandleFunc("POST /partials/attention/escalate/{id}", s.attEscalate)
	mux.HandleFunc("POST /partials/attention/spawn/{id}", s.attSpawn)
}

func (s *Server) attAck(w http.ResponseWriter, r *http.Request) {
	id, who, action := r.PathValue("id"), r.PathValue("who"), r.PathValue("action")
	var err error
	switch action {
	case "accept":
		err = s.Client.AckAccept(r.Context(), id, who, "")
	case "reject":
		err = s.Client.AckReject(r.Context(), id, who, "")
	default:
		http.Error(w, "unknown ack action", http.StatusBadRequest)
		return
	}
	if err != nil {
		renderPartial(w, attActionError(id, err))
		return
	}
	s.Indicators.Invalidate()
	renderPartial(w, attDone(id, who+" ACK "+action+"ed"))
}

func (s *Server) attStatus(w http.ResponseWriter, r *http.Request) {
	id, status := r.PathValue("id"), r.PathValue("status")
	if err := s.Client.SetStatus(r.Context(), id, status); err != nil {
		renderPartial(w, attActionError(id, err))
		return
	}
	s.Indicators.Invalidate()
	renderPartial(w, attDone(id, "status → "+statusLabel(status)))
}

func (s *Server) attEscalate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.Client.SetStatus(r.Context(), id, "escalated"); err != nil {
		renderPartial(w, attActionError(id, err))
		return
	}
	renderPartial(w, attDone(id, "escalated to Leadership"))
}

func (s *Server) attSpawn(w http.ResponseWriter, r *http.Request) {
	rfcID := r.PathValue("id")
	newID, err := s.Client.WorkCreate(r.Context(), "milestone", "Milestone from "+rfcID, "")
	if err != nil {
		renderPartial(w, attActionError(rfcID, err))
		return
	}
	// Record lineage: the new milestone derives_from the originating RFC.
	if err := s.Client.Link(r.Context(), newID, "derived_from", rfcID); err != nil {
		renderPartial(w, attActionError(rfcID, err))
		return
	}
	renderPartial(w, attDone(rfcID, "spawned milestone "+newID))
}

// attActionError swaps the row to an inline error notice (the substrate refused
// the write; the row stays so the user can retry from the record).
func attActionError(id string, err error) template.HTML {
	return template.HTML(fmt.Sprintf(
		`<div class="att-row" id="att-%s"><span class="att-sev att-sev--crit"></span><span class="att-row__chip">%s</span><span class="att-row__id dx-id">%s</span><span class="att-row__body"><span class="att-row__title">Action failed</span><span class="att-row__ctx">%s</span></span><span class="att-row__actions">%s</span></div>`,
		template.HTMLEscapeString(attDOMID(id)), pill("r", "error"),
		template.HTMLEscapeString(id), template.HTMLEscapeString(err.Error()),
		openLink(id)))
}

// --- small derived helpers (Attention-local) ---

// critIf returns "crit" when cond holds, else "warn".
func critIf(cond bool) string {
	if cond {
		return "crit"
	}
	return "warn"
}

func anyCrit(rows []attRow) bool {
	for _, r := range rows {
		if r.sev == "crit" {
			return true
		}
	}
	return false
}

// daysSince parses an RFC3339 timestamp and returns whole days elapsed to now.
func daysSince(ts string, now time.Time) (int, bool) {
	t, ok := parseTime(ts)
	if !ok {
		return 0, false
	}
	return int(now.Sub(t).Hours() / 24), true
}

// daysUntilTarget parses a delivery target (respecting Q/M/D precision) and
// returns whole days from now to the target end-date.
func daysUntilTarget(value, precision string, now time.Time) (int, bool) {
	t, ok := parseTarget(value, precision)
	if !ok {
		return 0, false
	}
	return int(t.Sub(now).Hours() / 24), true
}

// parseTime accepts RFC3339 or a bare YYYY-MM-DD date.
func parseTime(ts string) (time.Time, bool) {
	ts = strings.TrimSpace(ts)
	if ts == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, ts); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// parseTarget resolves a free-form delivery target to its end-instant. Quarters
// ("2026 Q3") resolve to the last day of the quarter; months/dates to their
// natural end; everything else falls back to a date parse.
func parseTarget(value, precision string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}
	// Quarter: "2026 Q3" / "2026Q3".
	if precision == "Q" || strings.Contains(value, "Q") {
		var year, q int
		if n, _ := fmt.Sscanf(strings.ReplaceAll(value, " ", ""), "%dQ%d", &year, &q); n == 2 && q >= 1 && q <= 4 {
			endMonth := time.Month(q * 3)
			// First day of the month after the quarter, minus a day.
			first := time.Date(year, endMonth, 1, 0, 0, 0, 0, time.UTC)
			return first.AddDate(0, 1, -1), true
		}
	}
	// Exact date YYYY-MM-DD.
	if t, err := time.Parse("2006-01-02", value[:min(len(value), 10)]); err == nil {
		return t, true
	}
	// Month YYYY-MM → end of month.
	if t, err := time.Parse("2006-01", value[:min(len(value), 7)]); err == nil {
		return t.AddDate(0, 1, -1), true
	}
	return time.Time{}, false
}
