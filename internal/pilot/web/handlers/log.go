// Pilot's Log (§9.5, PilotLogView): a top quick-entry row (kind <select> + body
// textarea + ⌘Enter to file) over the journal feed of observation events. The
// quick-entry posts to /partials/log/entry, which records a single
// work.observation_recorded carrying the chosen entry kind in its data blob,
// then swaps a fresh entry to the top of the feed. The feed below renders the
// EventsList of work.observation_recorded, each as a kind chip + target +
// body line (the LogEntry shape from the prototype).
package handlers

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strings"

	"github.com/lenulus/pf/internal/pilot/mcp"
)

// logKinds are the journal entry kinds the quick-entry select offers. They map
// to the entry_type discriminator on the observation data blob (Pilot-only
// convention; DITS never interprets them — see mcp.ObsData).
var logKinds = []struct{ value, label string }{
	{"observation", "Observation"},
	{"risk", "Risk"},
	{"stress_test", "Stress test"},
	{"decision", "Decision"},
	{"integrity_call", "Integrity call"},
}

// buildLogBody renders the quick-entry row above the journal feed.
//
//	events     — EventsList(work.observation_recorded) for the feed.
//	milestones — milestone list for the quick-entry target <select>.
func buildLogBody(events []mcp.Event, milestones []mcp.WorkItem) template.HTML {
	var b strings.Builder
	b.WriteString(string(logQuickEntry(milestones)))
	b.WriteString(`<div id="log-feed" style="background:var(--surface);border:1px solid var(--hairline);border-radius:5px;box-shadow:var(--shadow-1)">`)
	if len(events) == 0 {
		b.WriteString(`<div class="empty-line">No journal entries yet. File the first one above — specific is kind.</div>`)
	}
	for _, e := range events {
		b.WriteString(string(renderLogEntry(e)))
	}
	b.WriteString(`</div>`)
	return template.HTML(b.String())
}

// logQuickEntry renders the top quick-entry row. The form posts via HTMX and
// prepends the new entry to #log-feed; ⌘/Ctrl+Enter submits (JS island below).
func logQuickEntry(milestones []mcp.WorkItem) template.HTML {
	var kindOpts strings.Builder
	for _, k := range logKinds {
		fmt.Fprintf(&kindOpts, `<option value="%s">%s</option>`,
			template.HTMLEscapeString(k.value), template.HTMLEscapeString(k.label))
	}
	var mileOpts strings.Builder
	for _, m := range milestones {
		fmt.Fprintf(&mileOpts, `<option value="%s">%s</option>`,
			template.HTMLEscapeString(rowID(m)), template.HTMLEscapeString(rowID(m)))
	}

	return template.HTML(fmt.Sprintf(`
<form class="qe" hx-post="/partials/log/entry" hx-target="#log-feed" hx-swap="afterbegin" hx-on::after-request="if(event.detail.successful)this.querySelector('textarea').value=''">
  <div class="qe-num">§</div>
  <div class="qe-kind">
    <select name="kind">%s</select>
  </div>
  <div class="qe-body">
    <textarea name="body" rows="2" placeholder="Pilot's note. Cmd+Enter to file. Specific is kind."
      onkeydown="if(event.key==='Enter'&&(event.metaKey||event.ctrlKey)){event.preventDefault();this.form.requestSubmit();}"></textarea>
  </div>
  <div class="qe-action" onclick="this.closest('form').requestSubmit()">FILE<br>§</div>
  <div class="qe-meta">
    <span><strong>Milestone</strong> <select name="milestone" class="so-select" style="display:inline-block;width:120px;padding:1px 5px;font-family:var(--font-mono);font-size:10.5px">%s</select></span>
    <span style="margin-left:auto">⌘+↵ files · the substrate signs it</span>
  </div>
</form>`, kindOpts.String(), mileOpts.String()))
}

// renderLogEntry renders one journal feed row from an observation event: a kind
// chip + milestone chip + the entry body, mirroring the prototype's LogEntry.
func renderLogEntry(e mcp.Event) template.HTML {
	kind, body := observationKindBody(e)
	return template.HTML(fmt.Sprintf(
		`<div class="log-entry" style="display:flex;gap:10px;padding:11px 14px;border-bottom:1px solid var(--hairline)"><span class="log-entry__meta" style="display:flex;flex-direction:column;gap:3px;min-width:96px"><span class="dx-id">%s</span><span style="font-family:var(--font-mono);font-size:9.5px;color:var(--ink-4)">%s</span></span><span style="flex:1;min-width:0"><span style="display:inline-block;font-family:var(--font-mono);font-size:10px;padding:1px 6px;margin-right:8px;color:var(--accent);border:1px solid var(--accent-rule);border-radius:3px;vertical-align:1px">%s</span>%s%s</span></div>`,
		template.HTMLEscapeString(e.ActorID),
		template.HTMLEscapeString(shortDayLocal(e.Timestamp)),
		template.HTMLEscapeString(e.WorkItemID),
		logKindChip(kind),
		template.HTMLEscapeString(body)))
}

// logKindChip renders the entry-kind pill (color-coded by kind).
func logKindChip(kind string) template.HTML {
	variant := map[string]string{
		"risk": "y", "stress_test": "plum", "decision": "blue", "integrity_call": "r",
	}[kind]
	if variant == "" {
		variant = "neutral"
	}
	label := kind
	for _, k := range logKinds {
		if k.value == kind {
			label = k.label
			break
		}
	}
	if label == "" {
		return ""
	}
	return pill(variant, label)
}

// observationKindBody pulls the entry_type and summary off an observation event
// payload. The payload mirrors the dits_work_observe args ({summary, data}).
func observationKindBody(e mcp.Event) (kind, body string) {
	var p struct {
		Summary string          `json:"summary"`
		Data    json.RawMessage `json:"data"`
	}
	if len(e.Payload) > 0 {
		_ = json.Unmarshal(e.Payload, &p)
	}
	body = p.Summary
	if len(p.Data) > 0 {
		var d mcp.ObsData
		// Data may arrive as a JSON object or a JSON-encoded string.
		if err := json.Unmarshal(p.Data, &d); err != nil {
			var s string
			if json.Unmarshal(p.Data, &s) == nil {
				_ = json.Unmarshal([]byte(s), &d)
			}
		}
		kind = d.EntryType
	}
	return kind, body
}

// shortDayLocal trims an RFC3339 timestamp to its date (mirrors mcp.shortDay,
// which is package-private to mcp).
func shortDayLocal(ts string) string {
	if len(ts) >= 10 {
		return ts[:10]
	}
	return ts
}

// --- partial endpoint ---

// registerLogPartials wires the quick-entry POST route.
func (s *Server) registerLogPartials(mux *http.ServeMux) {
	mux.HandleFunc("POST /partials/log/entry", s.postLogEntry)
}

func (s *Server) postLogEntry(w http.ResponseWriter, r *http.Request) {
	id := r.FormValue("milestone")
	body := strings.TrimSpace(r.FormValue("body"))
	kind := r.FormValue("kind")
	if id == "" || body == "" {
		http.Error(w, "milestone and body are required", http.StatusBadRequest)
		return
	}
	data, _ := json.Marshal(mcp.ObsData{EntryType: kind})
	if err := s.Client.Observe(r.Context(), id, body, data); err != nil {
		renderPartial(w, template.HTML(`<div class="empty-line">Could not file entry: `+
			template.HTMLEscapeString(err.Error())+`</div>`))
		return
	}
	// Swap a freshly-rendered entry to the top of the feed. The timestamp is
	// approximate (the substrate stamps the canonical one); the feed reloads
	// authoritative on next view load.
	ev := mcp.Event{
		WorkItemID: id,
		ActorID:    "you",
		Timestamp:  nowRef().Format("2006-01-02"),
		Payload:    mustEntryPayload(body, kind),
	}
	renderPartial(w, renderLogEntry(ev))
}

// mustEntryPayload builds the observation payload shape renderLogEntry reads.
func mustEntryPayload(body, kind string) json.RawMessage {
	data, _ := json.Marshal(mcp.ObsData{EntryType: kind})
	p, _ := json.Marshal(struct {
		Summary string          `json:"summary"`
		Data    json.RawMessage `json:"data"`
	}{Summary: body, Data: data})
	return p
}
