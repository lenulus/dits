// Live view-model builders: map MCP DTOs (mcp.WorkItem / mcp.Event / mcp.Meta)
// into the generic web view models the templates render. These are the seam
// the Phase-5 foundation left open — each builder replaces the deleted demo
// dataset with real substrate data fetched over MCP.
package handlers

import (
	"encoding/json"
	"fmt"
	"html/template"
	"strings"
	"time"

	"github.com/lenulus/pf/internal/pilot/mcp"
	"github.com/lenulus/pf/internal/pilot/projections"
)

// buildEventsBody renders the read-only signed event log (EventLogView): the
// prototype's 6-column grid — §seq · type · target · payload · actor · time.
// shared resolves a raw work-item id to its human shared id; nil → as-is.
func buildEventsBody(events []mcp.Event, shared map[string]string) template.HTML {
	var b strings.Builder
	b.WriteString(`<div style="background:var(--surface);border:1px solid var(--hairline);border-radius:5px;box-shadow:var(--shadow-1)">`)
	b.WriteString(`<div class="evt-row" style="background:var(--paper-2);border-bottom:1px solid var(--hairline-strong);font-family:var(--font-mono);font-size:9.5px;color:var(--ink-3);text-transform:uppercase;letter-spacing:0.1em;font-weight:500"><span>§</span><span>Type</span><span>Target</span><span>Payload</span><span>Actor</span><span style="text-align:right">Time</span></div>`)
	n := len(events)
	for i, e := range events {
		target := e.WorkItemID
		if s, ok := shared[target]; ok {
			target = s
		}
		fmt.Fprintf(&b,
			`<div class="evt-row"><span class="sec">§%d</span><span class="typ">%s</span><span class="tgt">%s</span><span class="pay">%s</span><span style="display:inline-flex;align-items:center;gap:6px">%s<span class="sig">%s</span></span><span class="tim">%s</span></div>`,
			n-i,
			template.HTMLEscapeString(string(e.Type)),
			template.HTMLEscapeString(target),
			template.HTMLEscapeString(eventPayloadSummary(e)),
			avatarHTML(e.ActorID),
			template.HTMLEscapeString(shortSig(e.ID)),
			template.HTMLEscapeString(shortTime(e.Timestamp)))
	}
	b.WriteString(`</div>`)
	return template.HTML(b.String())
}

// eventPayloadSummary decodes an event payload into the prototype's compact
// "k=v · k=v" description, by type. Unknown types fall back to a trimmed JSON.
func eventPayloadSummary(e mcp.Event) string {
	var p map[string]any
	if len(e.Payload) > 0 {
		_ = json.Unmarshal(e.Payload, &p)
	}
	get := func(k string) string {
		if v, ok := p[k]; ok {
			return fmt.Sprintf("%v", v)
		}
		return ""
	}
	join := func(parts ...string) string {
		var out []string
		for _, s := range parts {
			if s != "" {
				out = append(out, s)
			}
		}
		return strings.Join(out, " · ")
	}
	switch e.Type {
	case "work.created":
		return join(kv("kind", get("kind")), kv("title", get("title")))
	case "work.field_set":
		return join(kv("field", get("field")), kv("value", get("value")))
	case "work.schedule_set":
		if arr, ok := p["stages"].([]any); ok {
			return fmt.Sprintf("%d stage%s", len(arr), plur(len(arr)))
		}
		return "stages set"
	case "work.observation_recorded":
		var d mcp.ObsData
		summary, _ := p["summary"].(string)
		if raw, ok := p["data"]; ok {
			b, _ := json.Marshal(raw)
			_ = json.Unmarshal(b, &d)
		}
		return join(kv("entry_type", d.EntryType), truncate(summary, 60))
	case "work.classified", "work.declassified":
		return join(kv("taxonomy", get("taxonomy_slug")), kv("node", get("node_slug")))
	case "work.role_bound", "work.role_unbound":
		return join(kv("role", get("role_slug")), kv("actor", get("actor_id")))
	case "work.status_set":
		return join(get("from"), "→ "+get("to"))
	case "work.ack_filed":
		return kv("scope", truncate(get("scope_summary"), 50))
	case "work.ack_accepted", "work.ack_rejected", "work.ack_cleared":
		return join(kv("who", get("who")), get("note"), get("reason"))
	case "work.ack_amended":
		return join(kv("type", get("amendment_type")), get("reason"))
	case "work.linked", "work.unlinked":
		return join(kv("rel", get("relation_type")), kv("target", get("target_work_item")))
	case "work.review_requested":
		return join(kv("reviewer", get("reviewer_role")), kv("scope", get("scope")))
	}
	s := strings.TrimSpace(string(e.Payload))
	return truncate(strings.Trim(s, "{}"), 70)
}

func kv(k, v string) string {
	if v == "" {
		return ""
	}
	return k + "=" + v
}

// shortSig renders a short signature-ish suffix from the event id.
func shortSig(id string) string {
	if len(id) >= 8 {
		return id[len(id)-8:]
	}
	return id
}

// shortTime trims an RFC3339 timestamp to "MM-DD HH:MM".
func shortTime(ts string) string {
	if len(ts) >= 16 {
		return ts[5:16]
	}
	return ts
}

// buildIndicatorRow renders the four RE leading indicators (§9.2 KPI row)
// from the projection cache.
func buildIndicatorRow(ind projections.Indicators) template.HTML {
	card := func(label, value, unit string) string {
		return fmt.Sprintf(
			`<div style="background:var(--surface);border:1px solid var(--hairline);border-radius:5px;padding:12px 14px;box-shadow:var(--shadow-1)">`+
				`<div style="font-family:var(--font-mono);font-size:10px;letter-spacing:0.08em;text-transform:uppercase;color:var(--ink-3)">%s</div>`+
				`<div style="font-family:var(--font-display);font-size:24px;font-weight:600;color:var(--ink-0);margin-top:4px">%s<span style="font-size:12px;color:var(--ink-3);font-weight:400"> %s</span></div>`+
				`</div>`,
			template.HTMLEscapeString(label), template.HTMLEscapeString(value), template.HTMLEscapeString(unit))
	}
	days := func(d time.Duration) string {
		if d <= 0 {
			return "—"
		}
		return fmt.Sprintf("%.1f", d.Hours()/24)
	}
	var b strings.Builder
	b.WriteString(`<div class="kpi-row">`)
	b.WriteString(card("Dependency closure", fmt.Sprintf("%.0f", ind.DependencyClosureRate*100), "%"))
	b.WriteString(card("Scope-change velocity", fmt.Sprintf("%.2f", ind.ScopeChangeVelocity), "/wk"))
	b.WriteString(card("Decision friction", days(ind.DecisionBlockFriction), "d"))
	b.WriteString(card("ACK-to-start latency", days(ind.AckToStartLatency), "d"))
	b.WriteString(`</div>`)
	return template.HTML(b.String())
}

// buildRoadmapItems filters milestones to the public projection: committed or
// later. (customer_visible is methodology metadata not yet on the substrate
// WorkItem, so v1 filters on status only — documented follow-up.)
func buildRoadmapItems(items []mcp.WorkItem) []mcp.WorkItem {
	out := make([]mcp.WorkItem, 0)
	for _, m := range items {
		switch m.Status {
		case "ack_committed", "in_flight", "shipped":
			out = append(out, m)
		}
	}
	return out
}

// --- small helpers ---

// rowID prefers the human-facing shared ID, falling back to the work-item ID.
func rowID(it mcp.WorkItem) string {
	if it.SharedID != "" {
		return it.SharedID
	}
	return it.ID
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// nowRef is the reference "now" for elapsed/until computations. Centralised so
// the Attention/Leadership signal logic shares a single clock.
func nowRef() time.Time { return time.Now() }

// plur returns "" for n==1 else "s" (Track-D pluralisation helper).
func plur(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// nodeDepth is the tree depth of a taxonomy node, derived from its slug
// (parent/child paths are slash-separated).
func nodeDepth(slug string) int { return strings.Count(slug, "/") }

// rygLabel maps an RYG token to a human label (g→green, …); empty stays empty.
func rygLabel(v string) string {
	switch v {
	case "g":
		return "green"
	case "y":
		return "yellow"
	case "r":
		return "red"
	default:
		return ""
	}
}

// rygHealthPill renders the RYG health pill. Track-D-local (uniquely named to
// avoid colliding with the sheet's own RYG cell renderer).
func rygHealthPill(v string) template.HTML {
	switch v {
	case "g":
		return pill("g", "Green")
	case "y":
		return pill("y", "Yellow")
	case "r":
		return pill("r", "Red")
	default:
		return template.HTML(`<span class="placeholder">—</span>`)
	}
}

// resultPill renders a goals-node result pill (mirrors the prototype
// RESULT_LABEL/RESULT_KIND).
func resultPill(result string) template.HTML {
	switch result {
	case "achieved":
		return pill("g", "Achieved")
	case "partial":
		return pill("y", "Partial")
	case "missed":
		return pill("r", "Missed")
	case "in_progress":
		return pill("blue", "In progress")
	case "aborted":
		return pill("neutral", "Aborted")
	default:
		return template.HTML(`<span class="placeholder">—</span>`)
	}
}

// formatTarget renders a delivery target with its precision, mirroring the
// prototype formatTarget: quarters pass through, months/dates get month-name
// formatting, everything else passes through verbatim.
func formatTarget(value, precision string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "—"
	}
	months := []string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sept", "Oct", "Nov", "Dec"}
	switch precision {
	case "M":
		if len(value) >= 7 {
			if t, err := time.Parse("2006-01", value[:7]); err == nil {
				return fmt.Sprintf("%s %d", months[int(t.Month())-1], t.Year())
			}
		}
	case "D":
		if len(value) >= 10 {
			if t, err := time.Parse("2006-01-02", value[:10]); err == nil {
				return fmt.Sprintf("%s %d, %d", months[int(t.Month())-1], t.Day(), t.Year())
			}
		}
	}
	return value
}
