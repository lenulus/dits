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
	var err error
	switch action {
	case "accept":
		err = s.Client.AckAccept(r.Context(), id, who, r.FormValue("note"))
	case "reject":
		err = s.Client.AckReject(r.Context(), id, who, r.FormValue("note"))
	default:
		http.Error(w, "unknown ack action", http.StatusBadRequest)
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	back(w, r, id, "ACK")
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

// --- live panel bodies ---

// buildMilestonePanelBody renders the editable body for the opened milestone's
// active tab. ACK and Status are the working editing surfaces; the remaining
// tabs render a read-only summary for now.
func buildMilestonePanelBody(item mcp.WorkItem, tab string) template.HTML {
	switch tab {
	case "Status":
		return statusTab(item)
	case "ACK":
		return ackTab(item)
	default:
		return template.HTML(`<div class="so-section"><div class="empty-line">` +
			template.HTMLEscapeString(tab) + ` — read-only for now; editing lands with the remaining panel tabs.</div></div>`)
	}
}

func statusTab(item mcp.WorkItem) template.HTML {
	id := rowID(item)
	var opts string
	for _, st := range milestoneStatuses {
		sel := ""
		if st == item.Status {
			sel = " selected"
		}
		opts += fmt.Sprintf(`<option value="%s"%s>%s</option>`, st, sel, statusLabel(st))
	}
	return template.HTML(fmt.Sprintf(`
<div class="so-section">
  <div class="so-section__label">Lifecycle status</div>
  <form method="post" action="/m/%s/status" style="display:flex;gap:8px;align-items:center">
    <select name="status" class="so-select">%s</select>
    <button class="rx-btn rx-btn--accent" type="submit">Set status</button>
  </form>
  <div style="margin-top:6px;font-family:var(--font-mono);font-size:11px;color:var(--ink-4)">current: %s</div>
</div>`, template.HTMLEscapeString(id), opts, template.HTMLEscapeString(statusLabel(item.Status))))
}

func ackTab(item mcp.WorkItem) template.HTML {
	id := rowID(item)
	esc := template.HTMLEscapeString

	// Roles & classification.
	roleForm := func(role string) string {
		cur := roleActor(item, role)
		return fmt.Sprintf(`
    <form method="post" action="/m/%s/role" class="so-roleform">
      <span class="so-roleform__label">%s</span>
      <input type="hidden" name="role" value="%s">
      <input name="actor" class="so-input" placeholder="actor id" value="%s">
      <button class="rx-btn rx-btn--ghost" type="submit">Bind</button>
    </form>`, esc(id), esc(role), esc(role), esc(cur))
	}
	classForm := func(tax string) string {
		cur := ""
		for _, c := range item.Classifications {
			if c.TaxonomySlug == tax {
				cur = c.NodeSlug
			}
		}
		return fmt.Sprintf(`
    <form method="post" action="/m/%s/classify" class="so-roleform">
      <span class="so-roleform__label">%s</span>
      <input type="hidden" name="taxonomy" value="%s">
      <input name="node" class="so-input" placeholder="%s node slug" value="%s">
      <button class="rx-btn rx-btn--ghost" type="submit">Classify</button>
    </form>`, esc(id), esc(tax), esc(tax), esc(tax), esc(cur))
	}

	// The commitment + the two ACK sides (or a file form).
	var commit string
	a, has := latestAck(item)
	if !has {
		commit = fmt.Sprintf(`
  <div class="so-section">
    <div class="so-section__label">The commitment</div>
    <form method="post" action="/m/%s/ack-file" class="so-stack">
      <input name="scope_summary" class="so-input" placeholder="Scope summary">
      <input name="delivery_timing" class="so-input" placeholder="Delivery timing (e.g. 2026 Q3)">
      <input name="target_outcome" class="so-input" placeholder="Target outcome">
      <input name="acceptance_criteria" class="so-input" placeholder="Acceptance criteria">
      <button class="rx-btn rx-btn--accent" type="submit">File ACK</button>
    </form>
  </div>`, esc(id))
	} else {
		sideCard := func(who, state string) string {
			return fmt.Sprintf(`
    <div class="so-ackcard">
      <div class="so-ackcard__head"><strong>%s</strong> %s</div>
      <div class="so-ackbtns">
        <form method="post" action="/m/%s/ack/%s/accept"><button class="rx-btn" style="background:var(--ryg-green);color:#fff;border-color:var(--ryg-green)" type="submit">Accept</button></form>
        <form method="post" action="/m/%s/ack/%s/reject"><button class="rx-btn" style="background:var(--ryg-red);color:#fff;border-color:var(--ryg-red)" type="submit">Reject</button></form>
      </div>
    </div>`, esc(who), string(ackStatePill(state)), esc(id), esc(who), esc(id), esc(who))
		}
		commit = fmt.Sprintf(`
  <div class="so-section">
    <div class="so-section__label">The commitment — %s</div>
    <div class="so-field"><span class="so-field__k">Scope</span> %s</div>
    <div class="so-field"><span class="so-field__k">Delivery</span> %s</div>
    <div class="so-field"><span class="so-field__k">Target</span> %s</div>
    <div class="so-field"><span class="so-field__k">Acceptance</span> %s</div>
  </div>
  <div class="so-section">
    <div class="so-section__label">The ACKs — %s</div>
    %s
    %s
    <form method="post" action="/m/%s/amend" class="so-amend">
      <select name="amendment_type" class="so-select">
        <option value="scope_change">scope_change</option>
        <option value="timeline_change">timeline_change</option>
        <option value="target_change">target_change</option>
        <option value="clarification">clarification</option>
      </select>
      <input name="reason" class="so-input" placeholder="amendment reason">
      <button class="rx-btn rx-btn--ghost" type="submit">Amend</button>
    </form>
  </div>`,
			string(rollupPill(a.Rollup())),
			esc(a.ScopeSummary), esc(a.DeliveryTiming), esc(a.TargetOutcome), esc(a.AcceptanceCriteria),
			string(rollupPill(a.Rollup())),
			sideCard("specifier", a.Specifier), sideCard("builder", a.Builder),
			esc(id))
	}

	body := fmt.Sprintf(`
  <div class="so-section">
    <div class="so-section__label">Roles &amp; classification</div>
    %s%s%s%s%s
  </div>
  %s`,
		roleForm("specifier"), roleForm("builder"), roleForm("pilot"),
		classForm("product"), classForm("org"),
		commit)
	return template.HTML(body)
}
