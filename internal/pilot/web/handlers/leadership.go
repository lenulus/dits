// The Leadership view body (§9.2): the in-flight milestone table, three
// pattern blocks, the adjudication queue, and the goal-health rollup. The
// four-indicator KPI row is rendered separately (buildIndicatorRow); this file
// supplies everything below it.
//
// Pattern blocks and goal health are read-only signal surfaces computed from
// the live substrate:
//   - scope-velocity   — milestones amending scope (amendment relations / ACK
//     amendments) above a baseline.
//   - landed-low-value — outcome assessments with verdict=achieved, value=low.
//   - pilot-collapse   — milestones carrying a role-collapse diagnostic.
//   - goal health      — worst-of-children rollup over the goals taxonomy's
//     per-node result metadata (achieved/partial/missed/in_progress).
package handlers

import (
	"encoding/json"
	"fmt"
	"html/template"
	"sort"
	"strings"

	"github.com/lenulus/pf/internal/pilot/mcp"
)

// buildLeadershipBody renders the in-flight table, pattern blocks, adjudication
// queue, and goal-health rollup. It needs decisions and outcomes (for patterns
// + adjudication) and meta (for goal health) in addition to the milestones.
func buildLeadershipBody(milestones, decisions, outcomes []mcp.WorkItem, meta mcp.Meta) template.HTML {
	var b strings.Builder
	b.WriteString(`<div class="rx-grid-main-side" style="gap:20px;margin-top:0">`)

	// --- main column: in-flight + pattern blocks ---
	b.WriteString(`<div>`)
	b.WriteString(string(leadershipInFlight(milestones)))
	b.WriteString(`<div style="margin-top:16px">`)
	b.WriteString(string(leadershipPatterns(milestones, outcomes)))
	b.WriteString(`</div></div>`)

	// --- side column: adjudication + goal health ---
	b.WriteString(`<div class="rx-stack" style="gap:16px">`)
	b.WriteString(string(leadershipAdjudication(decisions)))
	b.WriteString(string(leadershipGoalHealth(meta)))
	b.WriteString(`</div>`)

	b.WriteString(`</div>`)
	return template.HTML(b.String())
}

// leadershipInFlight renders the in-flight milestone table (RYG / alignment /
// pilot / target columns).
func leadershipInFlight(items []mcp.WorkItem) template.HTML {
	inflight := make([]mcp.WorkItem, 0)
	for _, m := range items {
		if m.Status == "in_flight" {
			inflight = append(inflight, m)
		}
	}
	sort.Slice(inflight, func(i, j int) bool { return rowID(inflight[i]) < rowID(inflight[j]) })

	var b strings.Builder
	fmt.Fprintf(&b, `<div class="rx-panel"><div class="rx-panel__head"><span class="rx-panel__title">In flight</span><span class="rx-panel__meta">%d milestones</span></div><table class="rx-table"><thead><tr><th>ID</th><th>Title</th><th>RYG</th><th>Alignment</th><th>Pilot</th><th>Target</th></tr></thead><tbody>`, len(inflight))
	for _, m := range inflight {
		rollup := "both_pending"
		if a, ok := latestAck(m); ok {
			rollup = a.Rollup()
		}
		fmt.Fprintf(&b, `<tr><td class="id">%s</td><td style="font-weight:500">%s</td><td>%s</td><td>%s</td><td>%s</td><td class="rx-mono" style="font-size:11px">%s</td></tr>`,
			template.HTMLEscapeString(rowID(m)), template.HTMLEscapeString(m.Title),
			rygHealthPill(m.RYG()), rollupPill(rollup), actorCell(roleActor(m, "pilot")),
			template.HTMLEscapeString(formatTarget(m.Target(), m.TargetPrecision())))
	}
	if len(inflight) == 0 {
		b.WriteString(`<tr><td colspan="6"><span class="placeholder">no milestones in flight</span></td></tr>`)
	}
	b.WriteString(`</tbody></table></div>`)
	return template.HTML(b.String())
}

// leadershipPatterns renders the three pattern blocks from live signals.
func leadershipPatterns(milestones, outcomes []mcp.WorkItem) template.HTML {
	// scope-velocity: milestones with ≥1 amendment relation or amendment ACK.
	var scopeVel []mcp.WorkItem
	for _, m := range milestones {
		if amendmentCount(m) >= 1 {
			scopeVel = append(scopeVel, m)
		}
	}
	// landed-low-value: outcomes verdict=achieved, value=low.
	var lowValue []mcp.WorkItem
	for _, o := range outcomes {
		if o.Field("result") == "achieved" && o.Field("value") == "low" {
			lowValue = append(lowValue, o)
		}
	}
	// pilot-collapse: milestones carrying a role-collapse diagnostic.
	var collapse []mcp.WorkItem
	for _, m := range milestones {
		if hasCollapseDiagnostic(m) {
			collapse = append(collapse, m)
		}
	}

	active := 0
	for _, n := range []int{len(scopeVel), len(lowValue), len(collapse)} {
		if n > 0 {
			active++
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, `<div class="rx-panel"><div class="rx-panel__head"><span class="rx-panel__title">Pattern blocks</span><span class="rx-panel__meta">%d active</span></div><div class="rx-panel__body--flush">`, active)

	block := func(variant, name, meta, prose string, last bool) {
		border := "border-bottom:1px solid var(--hairline)"
		if last {
			border = ""
		}
		fmt.Fprintf(&b, `<div style="padding:12px 14px;%s"><div style="display:flex;align-items:center;gap:8px;margin-bottom:5px">%s<span style="font-family:var(--font-mono);font-size:10.5px;color:var(--ink-3)">%s</span></div><div style="font-family:var(--font-serif);font-style:italic;font-size:13.5px;color:var(--ink-2);line-height:1.55">%s</div></div>`,
			border, pill(variant, name), template.HTMLEscapeString(meta), template.HTMLEscapeString(prose))
	}

	emitted := false
	if len(scopeVel) > 0 {
		ids := idsOf(scopeVel, 3)
		block("y", "scope-velocity", fmt.Sprintf("%d milestone%s", len(scopeVel), plur(len(scopeVel))),
			fmt.Sprintf("%s amending scope against the pre-commit baseline. Worth a Specifier sync.", ids), false)
		emitted = true
	}
	if len(lowValue) > 0 {
		o := lowValue[0]
		block("plum", "landed-low-value", fmt.Sprintf("%s · %s", rowID(o), roleActor(o, "specifier")),
			"Shipped on time. Value recorded low. The team should retro this — not a failure of execution, a failure of bet.", false)
		emitted = true
	}
	if len(collapse) > 0 {
		ids := idsOf(collapse, 3)
		block("r", "pilot-collapse", ids,
			"Pilot shares an actor with Builder/Specifier within one level on the org taxonomy. Diagnostic is firing; nothing is blocked. Re-bind.", true)
		emitted = true
	}
	if !emitted {
		b.WriteString(`<div style="padding:14px;font-family:var(--font-serif);font-style:italic;color:var(--ink-3);font-size:13px">No patterns firing. The portfolio is moving cleanly.</div>`)
	}

	b.WriteString(`</div></div>`)
	return template.HTML(b.String())
}

// leadershipAdjudication renders the escalated / idle≥9d decision queue.
func leadershipAdjudication(decisions []mcp.WorkItem) template.HTML {
	type adj struct {
		item mcp.WorkItem
		idle int
	}
	var queue []adj
	for _, d := range decisions {
		idle, _ := daysSince(d.UpdatedAt, nowRef())
		if d.Status == "escalated" || idle >= 9 {
			queue = append(queue, adj{d, idle})
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, `<div class="rx-panel"><div class="rx-panel__head"><span class="rx-panel__title">Adjudication queue</span><span class="rx-panel__meta">%d pending</span></div><div class="rx-panel__body--flush">`, len(queue))
	if len(queue) == 0 {
		b.WriteString(`<div style="padding:14px;font-family:var(--font-serif);font-style:italic;color:var(--ink-3);font-size:13px">Nothing on you today. Watch for silence — that's usually data.</div>`)
	}
	for _, q := range queue {
		variant := "y"
		if q.idle >= 9 {
			variant = "r"
		}
		fmt.Fprintf(&b, `<a href="/decisions" style="display:block;text-decoration:none;padding:10px 14px;border-bottom:1px solid var(--hairline)"><div style="display:flex;align-items:center;gap:8px;margin-bottom:3px"><span class="dx-id" style="color:var(--accent)">%s</span><span style="margin-left:auto">%s</span></div><div style="font-size:12.5px;color:var(--ink-1);line-height:1.4">%s</div></a>`,
			template.HTMLEscapeString(rowID(q.item)), pill(variant, fmt.Sprintf("%dd idle", q.idle)),
			template.HTMLEscapeString(q.item.Title))
	}
	b.WriteString(`</div></div>`)
	return template.HTML(b.String())
}

// leadershipGoalHealth renders the worst-of-children rollup over the goals
// taxonomy's per-node result metadata. Top-level goals roll up the worst result
// among their descendants.
func leadershipGoalHealth(meta mcp.Meta) template.HTML {
	var goals *mcp.Taxonomy
	for i := range meta.Taxonomies {
		if meta.Taxonomies[i].Slug == "goals" {
			goals = &meta.Taxonomies[i]
			break
		}
	}

	var b strings.Builder
	b.WriteString(`<div class="rx-panel"><div class="rx-panel__head"><span class="rx-panel__title">Goal health</span><span class="rx-panel__meta">rolled up</span></div><div class="rx-panel__body">`)
	if goals == nil || len(goals.Nodes) == 0 {
		b.WriteString(`<div class="empty-line">No goals taxonomy yet.</div></div></div>`)
		return template.HTML(b.String())
	}

	// Top-level goal nodes are those with no parent (depth 0).
	type goalRow struct {
		name   string
		result string
	}
	var rows []goalRow
	for _, n := range goals.Nodes {
		if nodeDepth(n.Slug) != 0 {
			continue
		}
		rows = append(rows, goalRow{name: n.Name, result: worstChildResult(n.Slug, goals.Nodes)})
	}

	for i, row := range rows {
		border := "border-bottom:1px solid var(--hairline)"
		if i == len(rows)-1 {
			border = "border-bottom:none"
		}
		fmt.Fprintf(&b, `<div style="display:flex;align-items:center;padding:7px 0;%s"><span style="font-family:var(--font-display);font-size:14px;font-weight:500;color:var(--ink-0)">%s</span><span style="margin-left:auto">%s</span></div>`,
			border, template.HTMLEscapeString(row.name), resultPill(row.result))
	}
	if len(rows) == 0 {
		b.WriteString(`<div class="empty-line">No top-level goals.</div>`)
	}
	b.WriteString(`</div></div>`)
	return template.HTML(b.String())
}

// worstChildResult returns the worst result among a goal's descendants (and the
// node itself), where worst = missed > partial > in_progress > achieved. Returns
// "" when no descendant carries a result.
func worstChildResult(goalSlug string, nodes []mcp.TaxonomyNode) string {
	rank := map[string]int{"missed": 4, "partial": 3, "in_progress": 2, "achieved": 1}
	worst, worstRank := "", 0
	for _, n := range nodes {
		if n.Slug != goalSlug && !strings.HasPrefix(n.Slug, goalSlug+"/") {
			continue
		}
		res := nodeResult(n)
		if r := rank[res]; r > worstRank {
			worst, worstRank = res, r
		}
	}
	return worst
}

// --- pattern-signal helpers ---

// amendmentCount counts a milestone's scope-amendment signals: amendment
// relations plus amendment ACKs.
func amendmentCount(m mcp.WorkItem) int {
	n := 0
	for _, r := range m.Relations {
		if r.Type == "amends" || r.Type == "amended_by" {
			n++
		}
	}
	return n
}

// hasCollapseDiagnostic reports whether a milestone carries a role-collapse
// constraint diagnostic.
func hasCollapseDiagnostic(m mcp.WorkItem) bool {
	for _, d := range m.Diagnostics {
		s := strings.ToLower(d.ConstraintSlug + " " + d.Message)
		if strings.Contains(s, "collapse") || strings.Contains(s, "reports-to") || strings.Contains(s, "pilot") {
			return true
		}
	}
	return false
}

// idsOf renders up to n row ids comma-separated.
func idsOf(items []mcp.WorkItem, n int) string {
	ids := make([]string, 0, n)
	for i, m := range items {
		if i >= n {
			ids = append(ids, "…")
			break
		}
		ids = append(ids, rowID(m))
	}
	return strings.Join(ids, ", ")
}

// nodeResult reads a goals node's `result` from its opaque Metadata blob.
func nodeResult(n mcp.TaxonomyNode) string {
	if len(n.Metadata) == 0 {
		return ""
	}
	var md struct {
		Result string `json:"result"`
	}
	_ = json.Unmarshal(n.Metadata, &md)
	return md.Result
}
