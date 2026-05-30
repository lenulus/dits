// Live view-model builders: map MCP DTOs (mcp.WorkItem / mcp.Event / mcp.Meta)
// into the generic web view models the templates render. These are the seam
// the Phase-5 foundation left open — each builder replaces the deleted demo
// dataset with real substrate data fetched over MCP.
package handlers

import (
	"fmt"
	"html/template"
	"sort"
	"strings"
	"time"

	"github.com/lenulus/pf/internal/pilot/mcp"
	"github.com/lenulus/pf/internal/pilot/projections"
)

// buildEventsBody renders the read-only signed event log (EventLogView).
func buildEventsBody(events []mcp.Event) template.HTML {
	var b strings.Builder
	b.WriteString(`<div style="background:var(--surface);border:1px solid var(--hairline);border-radius:5px;box-shadow:var(--shadow-1)">`)
	b.WriteString(`<div class="evt-row" style="background:var(--paper-2);border-bottom:1px solid var(--hairline-strong);font-family:var(--font-mono);font-size:9.5px;color:var(--ink-3);text-transform:uppercase;letter-spacing:0.1em;font-weight:500"><span>Type</span><span>Target</span><span>Actor</span><span style="text-align:right">Time</span></div>`)
	for _, e := range events {
		fmt.Fprintf(&b,
			`<div class="evt-row"><span class="typ">%s</span><span class="tgt">%s</span><span>%s</span><span class="tim">%s</span></div>`,
			template.HTMLEscapeString(e.Type),
			template.HTMLEscapeString(e.WorkItemID),
			template.HTMLEscapeString(e.ActorID),
			template.HTMLEscapeString(e.Timestamp))
	}
	b.WriteString(`</div>`)
	return template.HTML(b.String())
}

// buildTaxonomyBody renders the Org / Product / Goals taxonomy trees from meta.
func buildTaxonomyBody(meta mcp.Meta) template.HTML {
	var b strings.Builder
	if len(meta.Taxonomies) == 0 {
		return template.HTML(`<div class="empty-line">No taxonomies in meta yet. Run <code>pilot init</code> to apply the RE bundle, then add nodes.</div>`)
	}
	for _, tx := range meta.Taxonomies {
		fmt.Fprintf(&b, `<div class="rx-panel" style="margin-bottom:14px"><div class="rx-panel__head"><span class="rx-panel__title">%s</span><span class="rx-panel__meta">%s · %d nodes</span></div><div class="rx-panel__body">`,
			template.HTMLEscapeString(tx.Name), template.HTMLEscapeString(strings.Join(tx.Levels, " → ")), len(tx.Nodes))
		if len(tx.Nodes) == 0 {
			b.WriteString(`<div class="empty-line">empty skeleton — consumer fills nodes</div>`)
		}
		for _, n := range tx.Nodes {
			depth := strings.Count(n.Slug, "/")
			fmt.Fprintf(&b, `<div class="tx-node"><span class="indent" style="width:%dpx"></span><span style="display:flex;flex-direction:column;flex:1;min-width:0"><span class="name">%s</span><span class="slug">%s</span></span></div>`,
				8+depth*18, template.HTMLEscapeString(n.Name), template.HTMLEscapeString(n.Slug))
		}
		b.WriteString(`</div></div>`)
	}
	return template.HTML(b.String())
}

// buildAttentionBody computes the "For you" buckets client-side from
// milestones (§9.5). v1 surfaces the substrate-derivable buckets: pending
// ACKs, rejected ACKs, and diagnostics on your work.
func buildAttentionBody(items []mcp.WorkItem, me string) template.HTML {
	type row struct{ chip, variant, id, title, ctx string }
	var ackPending, rejected, diagnostics []row
	for _, m := range items {
		if a, ok := latestAck(m); ok {
			if a.Specifier == "pending" || a.Builder == "pending" {
				ackPending = append(ackPending, row{"ACK pending", "y", rowID(m), m.Title,
					"alignment " + mcp.AckRollup(a.Specifier, a.Builder)})
			}
			if a.Specifier == "rejected" || a.Builder == "rejected" {
				rejected = append(rejected, row{"rejected", "r", rowID(m), m.Title, "an ACK side rejected the commitment"})
			}
		}
		if len(m.Diagnostics) > 0 {
			msgs := make([]string, 0, len(m.Diagnostics))
			for _, d := range m.Diagnostics {
				msgs = append(msgs, d.Message)
			}
			diagnostics = append(diagnostics, row{fmt.Sprintf("%d flag", len(m.Diagnostics)), "y", rowID(m), m.Title, strings.Join(msgs, " · ")})
		}
	}

	var b strings.Builder
	section := func(title string, rows []row) {
		if len(rows) == 0 {
			return
		}
		fmt.Fprintf(&b, `<div class="att-section"><div class="att-section__head"><span class="att-sev att-sev--warn"></span><h3 class="att-section__title">%s</h3><span class="att-section__count">%d</span></div><div class="att-rows">`,
			template.HTMLEscapeString(title), len(rows))
		for _, r := range rows {
			fmt.Fprintf(&b, `<a class="att-row" href="/portfolio?open=%s"><span class="att-sev att-sev--%s"></span><span class="att-row__chip">%s</span><span class="att-row__id dx-id">%s</span><span class="att-row__body"><span class="att-row__title">%s</span><span class="att-row__ctx">%s</span></span></a>`,
				template.HTMLEscapeString(r.id), template.HTMLEscapeString(r.variant), pill(r.variant, r.chip),
				template.HTMLEscapeString(r.id), template.HTMLEscapeString(r.title), template.HTMLEscapeString(r.ctx))
		}
		b.WriteString(`</div></div>`)
	}
	total := len(ackPending) + len(rejected) + len(diagnostics)
	fmt.Fprintf(&b, `<div class="att"><div class="att-summary"><div class="att-summary__line"><span class="att-summary__count">%d</span><span class="att-summary__lab">things on you.</span></div><div class="att-summary__quip">Specific is kind. The substrate catches every change.</div></div>`, total)
	section("ACKs needing attention", ackPending)
	section("Rejected ACKs in your view", rejected)
	section("Diagnostics on your work", diagnostics)
	if total == 0 {
		b.WriteString(`<div class="empty-line">Nothing on you today. Watch for silence — that's usually data.</div>`)
	}
	b.WriteString(`</div>`)
	return template.HTML(b.String())
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

// buildLeadershipBody renders the in-flight milestone table (pattern blocks
// land alongside the indicator projections).
func buildLeadershipBody(items []mcp.WorkItem) template.HTML {
	inflight := make([]mcp.WorkItem, 0)
	for _, m := range items {
		if m.Status == "in_flight" {
			inflight = append(inflight, m)
		}
	}
	sort.Slice(inflight, func(i, j int) bool { return rowID(inflight[i]) < rowID(inflight[j]) })
	var b strings.Builder
	fmt.Fprintf(&b, `<div class="rx-panel"><div class="rx-panel__head"><span class="rx-panel__title">In flight</span><span class="rx-panel__meta">%d milestones</span></div><table class="rx-table"><thead><tr><th>ID</th><th>Title</th><th>Alignment</th><th>Pilot</th><th>Status</th></tr></thead><tbody>`, len(inflight))
	for _, m := range inflight {
		rollup := "both_pending"
		if a, ok := latestAck(m); ok {
			rollup = mcp.AckRollup(a.Specifier, a.Builder)
		}
		fmt.Fprintf(&b, `<tr><td class="id">%s</td><td style="font-weight:500">%s</td><td>%s</td><td>%s</td><td>%s</td></tr>`,
			template.HTMLEscapeString(rowID(m)), template.HTMLEscapeString(m.Title),
			rollupPill(rollup), actorCell(roleActor(m, "pilot")), statusPill(m.Status))
	}
	b.WriteString(`</tbody></table></div>`)
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
