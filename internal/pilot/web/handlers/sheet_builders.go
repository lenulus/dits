// Sheet view-model builders — the interactive grids (Portfolio, ACK, RFC,
// Decisions/Outcomes, Roles). Track B owns these together with the sheet
// primitive (web/sheet.go, sheet.html, sheet.js) and the inline-edit partial
// endpoints (sheet_edit.go), so the whole spreadsheet surface lives in one
// ownership boundary. Non-sheet view bodies (Attention, Leadership, Log,
// Taxonomies, Events) live in builders.go.
package handlers

import (
	"fmt"
	"html/template"
	"strconv"
	"strings"
	"time"

	"github.com/lenulus/pf/internal/pilot/mcp"
	"github.com/lenulus/pf/internal/pilot/web"
)

// --- shared editor option sets (mirror views.jsx M_STATUSES / RYG / etc.) ---

var statusOptions = []web.Option{
	{Value: "draft", Label: "Draft"},
	{Value: "ack_filed", Label: "ACK filed"},
	{Value: "ack_committed", Label: "ACK committed"},
	{Value: "in_flight", Label: "In flight"},
	{Value: "shipped", Label: "Shipped"},
	{Value: "aborted", Label: "Aborted"},
}

var rygOptions = []web.Option{
	{Value: "g", Label: "Green"},
	{Value: "y", Label: "Yellow"},
	{Value: "r", Label: "Red"},
}

var visibilityOptions = []web.Option{
	{Value: "true", Label: "Public"},
	{Value: "false", Label: "Internal"},
}

var rfcStatusOptions = []web.Option{
	{Value: "draft", Label: "Draft"},
	{Value: "leadership_review", Label: "Leadership review"},
	{Value: "approved", Label: "Approved"},
	{Value: "backlog", Label: "Backlog"},
	{Value: "resourced", Label: "Resourced"},
	{Value: "rejected", Label: "Rejected"},
}

var decisionStatusOptions = []web.Option{
	{Value: "open", Label: "Open"},
	{Value: "resolved", Label: "Resolved"},
	{Value: "escalated", Label: "Escalated"},
}

var outcomeStatusOptions = []web.Option{
	{Value: "pending", Label: "Pending"},
	{Value: "completed", Label: "Completed"},
	{Value: "overdue", Label: "Overdue"},
}

var verdictOptions = []web.Option{
	{Value: "achieved", Label: "Achieved"},
	{Value: "partial", Label: "Partial"},
	{Value: "missed", Label: "Missed"},
	{Value: "aborted", Label: "Aborted"},
}

var valueOptions = []web.Option{
	{Value: "high", Label: "high"},
	{Value: "medium", Label: "medium"},
	{Value: "low", Label: "low"},
}

// portfolioColumns is the canonical Portfolio column set + groups (§9.3),
// shared by the Portfolio and (a subset) the ACK view. Editable columns carry
// an Edit descriptor so sheet.html emits the hx-* inline-editor wiring.
func portfolioColumns() []web.Column {
	return []web.Column{
		{Key: "id", Label: "ID", Width: 96, Group: "identity", Frozen: true, Readonly: true},
		{Key: "title", Label: "Title", Width: 260, Group: "identity", Frozen: true, Edit: "text"},
		{Key: "product", Label: "Product", Width: 150, Group: "classification", Edit: "taxonomy"},
		{Key: "org", Label: "Org", Width: 150, Group: "classification", Edit: "taxonomy"},
		{Key: "status", Label: "Status", Width: 140, Group: "health", Edit: "select", Options: statusOptions},
		{Key: "ryg", Label: "RYG", Width: 82, Group: "health", Edit: "select", Options: rygOptions},
		{Key: "specAck", Label: "S-ACK", Width: 104, Group: "acks", Edit: "ack"},
		{Key: "buildAck", Label: "B-ACK", Width: 104, Group: "acks", Edit: "ack"},
		{Key: "specifier", Label: "Specifier", Width: 140, Group: "roles", Edit: "actor"},
		{Key: "builder", Label: "Builder", Width: 140, Group: "roles", Edit: "actor"},
		{Key: "pilot", Label: "Pilot", Width: 140, Group: "roles", Edit: "actor"},
		{Key: "target", Label: "Target", Width: 140, Group: "delivery", Edit: "target"},
		{Key: "stages", Label: "Stages", Width: 260, Group: "delivery", Readonly: true},
		{Key: "align", Label: "Alignment", Width: 140, Group: "delivery", Readonly: true},
		{Key: "statusNarrative", Label: "Status →", Width: 300, Group: "narrative", Readonly: true},
		{Key: "risks", Label: "Risks", Width: 220, Group: "narrative", Readonly: true},
		{Key: "nextSteps", Label: "Next", Width: 200, Group: "narrative", Readonly: true},
		{Key: "visibility", Label: "Visibility", Width: 96, Group: "signals", Edit: "select", Options: visibilityOptions},
		{Key: "diag", Label: "Diag", Width: 64, Group: "signals", Num: true, Readonly: true},
	}
}

// PortfolioParams carries the query-param state the Portfolio handler threads
// into the sheet builder (preset/group/open + the active filter set). The
// coordinator constructs it from r.URL.Query() (see the report).
type PortfolioParams struct {
	Preset       string
	Open         string   // active panel record id
	GroupBy      string   // row group-by: "" | status | team | product | quarter
	Collapsed    []string // the full collapsed-group set (band toggles send this)
	CollapsedSet bool     // whether ?collapsed was present (authoritative over preset)
	Filters      []web.FilterPill
}

// PortfolioParamsFromQuery parses the Portfolio handler's URL query into a
// PortfolioParams. The coordinator threads r.URL.Query() through here so the
// query-param contract (preset / group / open / groupby / flt_<field>=<value>)
// lives in Track B. Active filter chips are every flt_<field> param; their
// human label/value come from the field menu (filterFieldLabel + the option
// set), so the toolbar can re-render the chip without extra state.
func PortfolioParamsFromQuery(q map[string][]string, items []mcp.WorkItem) PortfolioParams {
	get := func(k string) string {
		if v := q[k]; len(v) > 0 {
			return v[0]
		}
		return ""
	}
	groupBy := get("groupby")
	if groupBy == "none" {
		groupBy = ""
	}
	p := PortfolioParams{
		Preset:  get("preset"),
		Open:    get("open"),
		GroupBy: groupBy,
	}
	// The band toggles send the full collapsed-group set as ?collapsed=a,b,c
	// (an empty value means "everything expanded"). When present it is
	// authoritative over the preset's default set, so each group folds/unfolds
	// independently rather than the URL holding a single toggled group.
	if raw, ok := q["collapsed"]; ok {
		p.CollapsedSet = true
		for _, g := range strings.Split(raw[0], ",") {
			if g = strings.TrimSpace(g); g != "" {
				p.Collapsed = append(p.Collapsed, g)
			}
		}
	}
	fields := portfolioFilterFields(items)
	for _, fd := range fields {
		raw := get("flt_" + fd.Field)
		if raw == "" {
			continue
		}
		p.Filters = append(p.Filters, web.FilterPill{
			Field:    fd.Field,
			Label:    fd.Label,
			Op:       "is",
			Value:    raw,
			ValueLab: filterValueLabel(fd, raw),
		})
	}
	return p
}

// filterValueLabel resolves the display label for a filter value (enum option
// label, or the raw value for actor/bool fields).
func filterValueLabel(fd web.FilterField, value string) string {
	for _, o := range fd.Options {
		if o.Value == value {
			return o.Label
		}
	}
	if fd.Kind == "bool" {
		return "true"
	}
	return value
}

// buildPortfolioSheet maps milestones into the Portfolio sheet. Filters are
// applied server-side over the milestone slice via the methodology accessors;
// group-by bands the surviving rows; the toolbar reflects the active state.
func buildPortfolioSheet(items []mcp.WorkItem, p PortfolioParams) *web.SheetModel {
	preset := p.Preset
	if preset == "" {
		preset = "overview"
	}
	// Collapsed set: the explicit band-toggle set when present, else the
	// preset's default. With an explicit set, reflect which preset (if any) it
	// matches in the toolbar — otherwise "custom".
	var collapsed map[string]bool
	if p.CollapsedSet {
		collapsed = map[string]bool{}
		for _, g := range p.Collapsed {
			collapsed[g] = true
		}
		preset = presetForCollapsed(collapsed)
	} else {
		collapsed = web.CollapsedFromPreset(preset)
	}

	// Server-side filter: AND every active chip.
	kept := items[:0:0]
	for _, m := range items {
		if matchesFilters(m, p.Filters) {
			kept = append(kept, m)
		}
	}

	allRows := make([]web.Row, 0, len(kept))
	for _, m := range kept {
		allRows = append(allRows, portfolioRow(m))
	}

	s := &web.SheetModel{
		Columns:      portfolioColumns(),
		Rows:         allRows,
		Groups:       web.PortfolioGroups,
		Presets:      web.PortfolioPresets,
		Collapsed:    collapsed,
		GroupBy:      p.GroupBy,
		Selectable:   true,
		AddLabel:     "New milestone…",
		AddKind:      "milestone",
		BulkURL:      "/partials/sheet/bulk",
		ActiveID:     p.Open,
		NavColumnIdx: 1,
		Toolbar: &web.Toolbar{
			BaseURL:      "/portfolio",
			Preset:       preset,
			Presets:      web.PortfolioPresets,
			GroupBy:      orNone(p.GroupBy),
			GroupByOpts:  portfolioGroupByOptions,
			Filters:      p.Filters,
			FilterFields: portfolioFilterFields(items),
			Total:        len(items),
			AddLabel:     "New milestone",
		},
	}
	// Row grouping bands (group-by). Quarter=Target, Team=org, Product=product
	// top, Status=status.
	if p.GroupBy != "" {
		s.Groups2 = groupRows(kept, p.GroupBy)
		s.Rows = nil // rows render through Groups2 instead
	}
	s.ComputeLayout()
	return s
}

// presetForCollapsed returns the preset whose default collapsed-set equals the
// given set, or "custom" when none matches (mirrors views.jsx presetFor).
func presetForCollapsed(set map[string]bool) string {
	for _, p := range web.PortfolioPresets {
		if len(p.Collapsed) != len(set) {
			continue
		}
		match := true
		for _, g := range p.Collapsed {
			if !set[g] {
				match = false
				break
			}
		}
		if match {
			return p.Value
		}
	}
	return "custom"
}

// portfolioRow renders one milestone into the Portfolio cell map.
func portfolioRow(m mcp.WorkItem) web.Row {
	spec := roleActor(m, "specifier")
	build := roleActor(m, "builder")
	pilot := roleActor(m, "pilot")
	specAck, buildAck, rollup := "pending", "pending", "both_pending"
	if a, ok := latestAck(m); ok {
		specAck, buildAck = a.Specifier, a.Builder
		rollup = mcp.AckRollup(a.Specifier, a.Builder)
	}
	return web.Row{
		ID: rowID(m),
		Cells: map[string]web.Cell{
			"id":              {HTML: idCell(rowID(m))},
			"title":           {Raw: m.Title},
			"product":         {HTML: classCell(m, "product")},
			"org":             {HTML: classCell(m, "org")},
			"status":          {HTML: statusPill(m.Status)},
			"ryg":             {HTML: rygPill(m.RYG())},
			"specAck":         {HTML: ackStatePill(specAck)},
			"buildAck":        {HTML: ackStatePill(buildAck)},
			"specifier":       {HTML: actorCell(spec)},
			"builder":         {HTML: actorCell(build)},
			"pilot":           {HTML: actorCell(pilot)},
			"target":          {HTML: targetCell(m.Target(), m.TargetPrecision())},
			"stages":          {HTML: stagesCell(m.Stages)},
			"align":           {HTML: rollupPill(rollup)},
			"statusNarrative": {HTML: statusNarrativeCell(m)},
			"risks":           {HTML: risksCell(m.Risks())},
			"nextSteps":       {HTML: nextCell(m.NextSteps())},
			"visibility":      {HTML: visibilityPill(m.CustomerVisible())},
			"diag":            {HTML: diagBadge(m)},

			// Collapsed-group summaries (rendered in the single summary column
			// when a group folds; mirror views.jsx PORTFOLIO_GROUPS.summary).
			"__group_classification": {HTML: classCell(m, "product")},
			"__group_health":         {HTML: healthSummary(m)},
			"__group_acks":           {HTML: rollupPill(rollup)},
			"__group_roles":          {HTML: roleStack(spec, build, pilot)},
			"__group_delivery":       {HTML: targetCell(m.Target(), m.TargetPrecision())},
			"__group_narrative":      {HTML: narrativeSummary(m)},
			"__group_signals":        {HTML: signalsSummary(m)},
		},
	}
}

// --- collapsed-group summary + cell renderers (mirror views.jsx) ---

// healthSummary: RYG pill + a muted status label (Health group, collapsed).
func healthSummary(m mcp.WorkItem) template.HTML {
	return template.HTML(`<span style="display:inline-flex;align-items:center;gap:6px">` +
		string(rygPill(m.RYG())) +
		`<span style="font-family:var(--font-mono);font-size:10px;color:var(--ink-3)">` +
		template.HTMLEscapeString(statusLabel(m.Status)) + `</span></span>`)
}

// roleStack: the Specifier/Builder/Pilot avatars side by side (Roles group).
func roleStack(spec, build, pilot string) template.HTML {
	return template.HTML(`<span class="role-stack">` +
		string(avatarHTML(spec)) + string(avatarHTML(build)) + string(avatarHTML(pilot)) + `</span>`)
}

// narrativeSummary: status-present dot + risk/next counts (Updates group).
func narrativeSummary(m mcp.WorkItem) template.HTML {
	nr, nn := len(m.Risks()), len(m.NextSteps())
	statusColor := "var(--ink-5)"
	if m.StatusNarrative() != "" {
		statusColor = "var(--accent)"
	}
	riskColor := "var(--ink-5)"
	if nr > 0 {
		riskColor = "var(--ryg-yellow-ink)"
	}
	nextColor := "var(--ink-5)"
	if nn > 0 {
		nextColor = "var(--ink-1)"
	}
	return template.HTML(fmt.Sprintf(
		`<span style="display:inline-flex;align-items:center;gap:8px;font-family:var(--font-mono);font-size:10.5px;color:var(--ink-3)">`+
			`<span style="color:%s">&#9679; status</span>`+
			`<span style="color:%s">%d risk%s</span>`+
			`<span style="color:%s">%d next</span></span>`,
		statusColor, riskColor, nr, plur(nr), nextColor, nn))
}

// signalsSummary: diagnostics badge + visibility pill (Signals group).
func signalsSummary(m mcp.WorkItem) template.HTML {
	return template.HTML(`<span style="display:inline-flex;align-items:center;gap:6px">` +
		string(diagBadge(m)) + string(visibilityPill(m.CustomerVisible())) + `</span>`)
}

// stagesCell: the mini staged-timeline (Dogfood/Beta/GA dots + dates).
func stagesCell(stages []mcp.Stage) template.HTML {
	if len(stages) == 0 {
		return template.HTML(`<span class="placeholder">no stages</span>`)
	}
	var b strings.Builder
	b.WriteString(`<span class="stages">`)
	for _, s := range stages {
		state := s.State
		if state == "" {
			state = "open"
		}
		fmt.Fprintf(&b, `<span class="stage stage--%s"><span class="stage__dot"></span><span class="stage__lab">%s</span><span class="stage__when">%s</span></span>`,
			template.HTMLEscapeString(state), template.HTMLEscapeString(s.Label),
			template.HTMLEscapeString(formatTarget(s.Date, s.Precision)))
	}
	b.WriteString(`</span>`)
	return template.HTML(b.String())
}

// statusNarrativeCell: the status paragraph + who/when (read-only in the grid;
// edit it from the panel Status tab).
func statusNarrativeCell(m mcp.WorkItem) template.HTML {
	narr := m.StatusNarrative()
	if narr == "" {
		return template.HTML(`<span class="placeholder">no status yet</span>`)
	}
	when := m.StatusUpdatedAt()
	if len(when) > 10 {
		when = when[:10]
	}
	return template.HTML(`<span style="display:inline-flex;flex-direction:column;min-width:0;line-height:1.2">` +
		`<span style="font-size:12px;color:var(--ink-1);overflow:hidden;text-overflow:ellipsis;white-space:nowrap">` +
		template.HTMLEscapeString(narr) + `</span>` +
		`<span style="font-family:var(--font-mono);font-size:9.5px;color:var(--ink-4)">` +
		template.HTMLEscapeString(when) + `</span></span>`)
}

// risksCell: count badge + first risk body (Risks column).
func risksCell(risks []mcp.Risk) template.HTML {
	if len(risks) == 0 {
		return template.HTML(`<span class="placeholder">—</span>`)
	}
	high := 0
	for _, r := range risks {
		if r.Severity == "high" {
			high++
		}
	}
	cls := "dx-diag-count"
	if high > 0 {
		cls += " dx-diag-count--violation"
	}
	return template.HTML(fmt.Sprintf(
		`<span style="display:inline-flex;align-items:center;gap:6px;min-width:0"><span class="%s">%d</span>`+
			`<span style="font-family:var(--font-serif);font-style:italic;font-size:12px;color:var(--ink-2);overflow:hidden;text-overflow:ellipsis;white-space:nowrap">%s</span></span>`,
		cls, len(risks), template.HTMLEscapeString(risks[0].Body)))
}

// nextCell: count badge + first next-step body (Next column).
func nextCell(next []mcp.NextStep) template.HTML {
	if len(next) == 0 {
		return template.HTML(`<span class="placeholder">—</span>`)
	}
	return template.HTML(fmt.Sprintf(
		`<span style="display:inline-flex;align-items:center;gap:6px;min-width:0"><span class="dx-diag-count" style="background:var(--accent)">%d</span>`+
			`<span style="font-family:var(--font-serif);font-style:italic;font-size:12px;color:var(--ink-2);overflow:hidden;text-overflow:ellipsis;white-space:nowrap">%s</span></span>`,
		len(next), template.HTMLEscapeString(next[0].Body)))
}

// --- group-by ---

// portfolioGroupByOptions mirror App.jsx's group-by Pivot.
var portfolioGroupByOptions = []web.Option{
	{Value: "none", Label: "None"},
	{Value: "status", Label: "Status"},
	{Value: "team", Label: "Team"},
	{Value: "product", Label: "Product"},
	{Value: "quarter", Label: "Quarter"},
}

// groupRows bands the milestone slice by the chosen key, preserving first-seen
// group order. Quarter=Target, Team=org top, Product=product top, Status=status.
func groupRows(items []mcp.WorkItem, by string) []web.RowGroup {
	var order []string
	buckets := map[string][]web.Row{}
	for _, m := range items {
		key := groupKeyOf(m, by)
		if _, ok := buckets[key]; !ok {
			order = append(order, key)
		}
		buckets[key] = append(buckets[key], portfolioRow(m))
	}
	out := make([]web.RowGroup, 0, len(order))
	for _, k := range order {
		out = append(out, web.RowGroup{Key: k, Rows: buckets[k], Count: len(buckets[k])})
	}
	return out
}

func groupKeyOf(m mcp.WorkItem, by string) string {
	switch by {
	case "status":
		if m.Status == "" {
			return "—"
		}
		return statusLabel(m.Status)
	case "team":
		return classLeaf(m, "org")
	case "product":
		return classTop(m, "product")
	case "quarter":
		if t := m.Target(); t != "" {
			return t
		}
		return "—"
	}
	return "—"
}

// --- server-side filtering (mirrors views.jsx evalFilter) ---

// matchesFilters returns true when the milestone passes every active chip
// (AND-combination). Unknown fields pass (forward-compatible).
func matchesFilters(m mcp.WorkItem, filters []web.FilterPill) bool {
	for _, f := range filters {
		if !evalFilter(m, f.Field, f.Value) {
			return false
		}
	}
	return true
}

func evalFilter(m mcp.WorkItem, field, value string) bool {
	switch field {
	case "productTop":
		if value == "_none" {
			return classOf(m, "product") == ""
		}
		return strings.HasPrefix(classOf(m, "product"), value)
	case "orgNode":
		org := classOf(m, "org")
		return org == value || strings.HasPrefix(org, value+"/")
	case "specifier":
		return roleActor(m, "specifier") == value
	case "builder":
		return roleActor(m, "builder") == value
	case "pilot":
		return roleActor(m, "pilot") == value
	case "status":
		return m.Status == value
	case "ryg":
		return m.RYG() == value
	case "target":
		return m.Target() == value
	case "alignment":
		specAck, buildAck := "pending", "pending"
		if a, ok := latestAck(m); ok {
			specAck, buildAck = a.Specifier, a.Builder
		}
		return mcp.AckRollup(specAck, buildAck) == value
	case "customerVisible":
		return strconv.FormatBool(m.CustomerVisible()) == value
	case "_hasDiagnostics":
		if value == "violation" {
			for _, d := range m.Diagnostics {
				if d.Severity == "violation" {
					return true
				}
			}
			return false
		}
		return len(m.Diagnostics) > 0
	case "_hasRisksHigh":
		for _, r := range m.Risks() {
			if r.Severity == "high" {
				return true
			}
		}
		return false
	case "_isStale":
		return isStale(m, 14)
	case "_fromRfc":
		switch value {
		case "_any":
			return m.FromRFC() != ""
		case "_none":
			return m.FromRFC() == ""
		default:
			return m.FromRFC() == value
		}
	}
	return true
}

// isStale reports whether the latest status update is older than days (or there
// is none). Mirrors views.jsx evalFilter('_isStale').
func isStale(m mcp.WorkItem, days int) bool {
	ts := m.StatusUpdatedAt()
	if ts == "" {
		return true
	}
	when, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return true
	}
	return time.Since(when) > time.Duration(days)*24*time.Hour
}

// portfolioFilterFields builds the +Add-filter menu (the 12
// PORTFOLIO_FILTER_FIELDS). Enum option sets are derived from the live data
// where the prototype derived them from the seed (product roots, org nodes,
// targets, origin RFCs).
func portfolioFilterFields(items []mcp.WorkItem) []web.FilterField {
	return []web.FilterField{
		{Field: "productTop", Label: "Product", Kind: "enum", Options: productRootOptions(items)},
		{Field: "orgNode", Label: "Team", Kind: "enum", Options: orgNodeOptions(items)},
		{Field: "specifier", Label: "Specifier", Kind: "actor"},
		{Field: "builder", Label: "Builder", Kind: "actor"},
		{Field: "pilot", Label: "Pilot", Kind: "actor"},
		{Field: "status", Label: "Status", Kind: "enum", Options: statusOptions},
		{Field: "ryg", Label: "RYG", Kind: "enum", Options: rygOptions},
		{Field: "alignment", Label: "Alignment", Kind: "enum", Options: []web.Option{
			{Value: "aligned", Label: "Aligned"}, {Value: "both_pending", Label: "Both pending"},
			{Value: "specifier_pending", Label: "Specifier pending"}, {Value: "builder_pending", Label: "Builder pending"},
			{Value: "rejected", Label: "Rejected"},
		}},
		{Field: "target", Label: "Target", Kind: "enum", Options: targetOptions(items)},
		{Field: "customerVisible", Label: "Visibility", Kind: "enum", Options: visibilityOptions},
		{Field: "_hasDiagnostics", Label: "Diagnostics", Kind: "enum", Options: []web.Option{
			{Value: "any", Label: "Any flagged"}, {Value: "violation", Label: "Violations only"},
		}},
		{Field: "_hasRisksHigh", Label: "High-severity risk", Kind: "bool"},
		{Field: "_isStale", Label: "Stale (>14d)", Kind: "bool"},
		{Field: "_fromRfc", Label: "Origin RFC", Kind: "enum", Options: fromRfcOptions(items)},
	}
}

// --- option-set derivations ---

func productRootOptions(items []mcp.WorkItem) []web.Option {
	out := dedupClassRoots(items, "product")
	out = append(out, web.Option{Value: "_none", Label: "Unclassified"})
	return out
}

func orgNodeOptions(items []mcp.WorkItem) []web.Option {
	seen := map[string]bool{}
	var out []web.Option
	for _, m := range items {
		if n := classOf(m, "org"); n != "" && !seen[n] {
			seen[n] = true
			out = append(out, web.Option{Value: n, Label: classLeafOf(n)})
		}
	}
	return out
}

func targetOptions(items []mcp.WorkItem) []web.Option {
	seen := map[string]bool{}
	var out []web.Option
	for _, m := range items {
		if t := m.Target(); t != "" && !seen[t] {
			seen[t] = true
			out = append(out, web.Option{Value: t, Label: t})
		}
	}
	if len(out) == 0 {
		for _, t := range []string{"2026 Q2", "2026 Q3", "2026 Q4", "2027 Q1"} {
			out = append(out, web.Option{Value: t, Label: t})
		}
	}
	return out
}

func fromRfcOptions(items []mcp.WorkItem) []web.Option {
	out := []web.Option{{Value: "_any", Label: "Any RFC"}, {Value: "_none", Label: "No RFC origin"}}
	seen := map[string]bool{}
	for _, m := range items {
		if id := m.FromRFC(); id != "" && !seen[id] {
			seen[id] = true
			out = append(out, web.Option{Value: id, Label: id})
		}
	}
	return out
}

func dedupClassRoots(items []mcp.WorkItem, taxonomy string) []web.Option {
	seen := map[string]bool{}
	var out []web.Option
	for _, m := range items {
		top := classTopSlug(m, taxonomy)
		if top != "" && !seen[top] {
			seen[top] = true
			out = append(out, web.Option{Value: top, Label: top})
		}
	}
	return out
}

// classOf returns the full node slug a work item is classified into for the
// given taxonomy, or "".
func classOf(m mcp.WorkItem, taxonomy string) string {
	for _, c := range m.Classifications {
		if c.TaxonomySlug == taxonomy {
			return c.NodeSlug
		}
	}
	return ""
}

func classLeaf(m mcp.WorkItem, taxonomy string) string {
	if n := classOf(m, taxonomy); n != "" {
		return classLeafOf(n)
	}
	return "—"
}

func classTop(m mcp.WorkItem, taxonomy string) string {
	if t := classTopSlug(m, taxonomy); t != "" {
		return t
	}
	return "unclassified"
}

func classTopSlug(m mcp.WorkItem, taxonomy string) string {
	n := classOf(m, taxonomy)
	if n == "" {
		return ""
	}
	if i := strings.IndexByte(n, '/'); i >= 0 {
		return n[:i]
	}
	return n
}

func classLeafOf(node string) string {
	if i := strings.LastIndexByte(node, '/'); i >= 0 {
		return node[i+1:]
	}
	return node
}

func orNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}

// --- extra cell renderers (Portfolio RYG / target / visibility) ---

// rygPill renders the red/yellow/green health call.
func rygPill(ryg string) template.HTML {
	switch ryg {
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

// targetCell renders the delivery target + a precision tag (Q/M/D).
func targetCell(target, precision string) template.HTML {
	if target == "" {
		return template.HTML(`<span class="placeholder">—</span>`)
	}
	if precision == "" {
		precision = "Q"
	}
	return template.HTML(`<span style="display:inline-flex;align-items:center;gap:6px;font-family:var(--font-mono);font-size:11.5px;color:var(--ink-1)">` +
		`<span>` + template.HTMLEscapeString(target) + `</span>` +
		`<span style="font-family:var(--font-mono);font-size:9px;padding:1px 4px;background:var(--paper-1);color:var(--ink-4);border-radius:3px;letter-spacing:0.05em">` +
		template.HTMLEscapeString(precision) + `</span></span>`)
}

// visibilityPill renders the customer-visibility flag (public / internal).
func visibilityPill(visible bool) template.HTML {
	if visible {
		return pill("blue", "public")
	}
	return pill("neutral", "internal")
}

// buildSimpleSheet builds a flat (no column-group) sheet from items, given a
// column set and pre-built rows. Used by RFC/Decisions/Outcomes/Roles.
func buildSimpleSheet(cols []web.Column, rows []web.Row, addLabel, addKind string) *web.SheetModel {
	s := &web.SheetModel{
		Columns:      cols,
		Rows:         rows,
		Selectable:   true,
		AddLabel:     addLabel,
		AddKind:      addKind,
		BulkURL:      "/partials/sheet/bulk",
		NavColumnIdx: 1,
	}
	s.ComputeLayout()
	return s
}

func buildRfcSheet(items []mcp.WorkItem) *web.SheetModel {
	cols := []web.Column{
		{Key: "id", Label: "ID", Width: 96, Frozen: true, Readonly: true},
		{Key: "title", Label: "Title", Width: 280, Frozen: true, Edit: "text"},
		{Key: "status", Label: "Status", Width: 160, Edit: "select", Options: rfcStatusOptions},
		{Key: "specifier", Label: "Specifier", Width: 160, Edit: "actor"},
		{Key: "target", Label: "Target", Width: 120, Edit: "target"},
		{Key: "body", Label: "Summary", Width: 480, Readonly: true},
	}
	rows := make([]web.Row, 0, len(items))
	for _, r := range items {
		rows = append(rows, web.Row{ID: rowID(r), Cells: map[string]web.Cell{
			"id":        {HTML: idCell(rowID(r))},
			"title":     {Raw: r.Title},
			"status":    {HTML: statusPill(r.Status)},
			"specifier": {HTML: actorCell(roleActor(r, "specifier"))},
			"target":    {HTML: targetCell(r.Target(), r.TargetPrecision())},
			"body":      {Raw: truncate(r.Body, 140)},
		}})
	}
	return buildSimpleSheet(cols, rows, "New RFC…", "rfc")
}

// buildAckSheet is the bulk-ack workspace: S-ACK / B-ACK / alignment per
// milestone (§9.2, AckView).
func buildAckSheet(items []mcp.WorkItem) *web.SheetModel {
	cols := []web.Column{
		{Key: "id", Label: "ID", Width: 96, Frozen: true, Readonly: true},
		{Key: "title", Label: "Milestone", Width: 280, Frozen: true, Edit: "text"},
		{Key: "specifier", Label: "Specifier", Width: 150, Edit: "actor"},
		{Key: "specAck", Label: "Specifier ACK", Width: 150, Edit: "ack"},
		{Key: "builder", Label: "Builder", Width: 150, Edit: "actor"},
		{Key: "buildAck", Label: "Builder ACK", Width: 150, Edit: "ack"},
		{Key: "align", Label: "Alignment", Width: 150, Readonly: true},
	}
	rows := make([]web.Row, 0, len(items))
	for _, m := range items {
		specAck, buildAck, rollup := "pending", "pending", "both_pending"
		if a, ok := latestAck(m); ok {
			specAck, buildAck = a.Specifier, a.Builder
			rollup = mcp.AckRollup(a.Specifier, a.Builder)
		}
		rows = append(rows, web.Row{ID: rowID(m), Cells: map[string]web.Cell{
			"id":        {HTML: idCell(rowID(m))},
			"title":     {Raw: m.Title},
			"specifier": {HTML: actorCell(roleActor(m, "specifier"))},
			"specAck":   {HTML: ackStatePill(specAck)},
			"builder":   {HTML: actorCell(roleActor(m, "builder"))},
			"buildAck":  {HTML: ackStatePill(buildAck)},
			"align":     {HTML: rollupPill(rollup)},
		}})
	}
	return buildSimpleSheet(cols, rows, "", "")
}

// buildDecisionsSheet — DecisionBlocks with the prototype's idle-days colour
// scale (≥9d red / ≥5d yellow / else green) + owner picker (§9 DecisionsView).
func buildDecisionsSheet(items []mcp.WorkItem) *web.SheetModel {
	cols := []web.Column{
		{Key: "id", Label: "ID", Width: 88, Frozen: true, Readonly: true},
		{Key: "milestone", Label: "Milestone", Width: 120, Frozen: true, Readonly: true},
		{Key: "summary", Label: "What needs deciding", Width: 420, Edit: "text"},
		{Key: "status", Label: "Status", Width: 130, Edit: "select", Options: decisionStatusOptions},
		{Key: "owner", Label: "Owner", Width: 150, Edit: "actor"},
		{Key: "opened", Label: "Opened", Width: 110, Readonly: true},
		{Key: "idle", Label: "Idle", Width: 80, Num: true, Align: "right", Readonly: true},
	}
	rows := make([]web.Row, 0, len(items))
	for _, it := range items {
		idle := idleDays(it)
		rows = append(rows, web.Row{ID: rowID(it), Cells: map[string]web.Cell{
			"id":        {HTML: idCell(rowID(it))},
			"milestone": {HTML: milestoneRef(parentMilestone(it))},
			"summary":   {Raw: truncate(firstNonEmpty(it.Title, it.Body), 120)},
			"status":    {HTML: statusPill(it.Status)},
			"owner":     {HTML: actorCell(roleActor(it, "owner"))},
			"opened":    {HTML: monoDay(it.CreatedAt)},
			"idle":      {HTML: idlePill(idle)},
		}})
	}
	return buildSimpleSheet(cols, rows, "Open DecisionBlock…", "decision_block")
}

// buildOutcomesSheet — OutcomeAssessments with verdict / value / SLA-due
// (§9 OutcomesView).
func buildOutcomesSheet(items []mcp.WorkItem) *web.SheetModel {
	cols := []web.Column{
		{Key: "id", Label: "ID", Width: 88, Frozen: true, Readonly: true},
		{Key: "milestone", Label: "Milestone", Width: 120, Frozen: true, Readonly: true},
		{Key: "status", Label: "Status", Width: 120, Edit: "select", Options: outcomeStatusOptions},
		{Key: "due", Label: "SLA due", Width: 110, Readonly: true},
		{Key: "verdict", Label: "Verdict", Width: 120, Edit: "select", Options: verdictOptions},
		{Key: "value", Label: "Value", Width: 96, Edit: "select", Options: valueOptions},
		{Key: "by", Label: "Recorded by", Width: 160, Edit: "actor"},
		{Key: "note", Label: "Note", Width: 440, Edit: "text"},
	}
	rows := make([]web.Row, 0, len(items))
	for _, it := range items {
		rows = append(rows, web.Row{ID: rowID(it), Cells: map[string]web.Cell{
			"id":        {HTML: idCell(rowID(it))},
			"milestone": {HTML: milestoneRef(parentMilestone(it))},
			"status":    {HTML: statusPill(it.Status)},
			"due":       {HTML: monoDay(it.Field("sla_due"))},
			"verdict":   {HTML: verdictPill(it.Field("verdict"))},
			"value":     {HTML: valuePill(it.Field("value"))},
			"by":        {HTML: actorCell(roleActor(it, "evaluator"))},
			"note":      {Raw: truncate(it.Body, 140)},
		}})
	}
	return buildSimpleSheet(cols, rows, "", "")
}

// buildKindSheet remains the generic fallback for any other work kind.
func buildKindSheet(items []mcp.WorkItem, addLabel, addKind string) *web.SheetModel {
	cols := []web.Column{
		{Key: "id", Label: "ID", Width: 96, Frozen: true, Readonly: true},
		{Key: "title", Label: "Title", Width: 320, Frozen: true, Edit: "text"},
		{Key: "status", Label: "Status", Width: 150, Edit: "select", Options: statusOptions},
		{Key: "specifier", Label: "Specifier", Width: 160, Edit: "actor"},
		{Key: "body", Label: "Detail", Width: 460, Readonly: true},
	}
	rows := make([]web.Row, 0, len(items))
	for _, it := range items {
		rows = append(rows, web.Row{ID: rowID(it), Cells: map[string]web.Cell{
			"id":        {HTML: idCell(rowID(it))},
			"title":     {Raw: it.Title},
			"status":    {HTML: statusPill(it.Status)},
			"specifier": {HTML: actorCell(roleActor(it, "specifier"))},
			"body":      {Raw: truncate(it.Body, 120)},
		}})
	}
	return buildSimpleSheet(cols, rows, addLabel, addKind)
}

// buildRolesSheet flattens every role binding across milestones into one sheet
// with per-binding diagnostics + the bound actor's inline picker (§9.7 —
// constraints flag, they don't gate). The Actor column is an inline actor
// picker that re-binds the role (BindRole) on commit; row id is
// "<milestone>:<role>:<actor>" so the picker knows what to unbind/rebind.
func buildRolesSheet(items []mcp.WorkItem) *web.SheetModel {
	cols := []web.Column{
		{Key: "milestone", Label: "Milestone", Width: 120, Frozen: true, Readonly: true},
		{Key: "role", Label: "Role", Width: 130, Readonly: true},
		{Key: "actor", Label: "Actor", Width: 200, Edit: "actor"},
		{Key: "actorRole", Label: "Actor role", Width: 140, Readonly: true},
		{Key: "diag", Label: "Constraint diagnostic", Width: 440, Readonly: true},
	}
	var rows []web.Row
	for _, m := range items {
		// One diagnostics summary per milestone, shown on its first binding row.
		diagMsg, violation := "", false
		for i, d := range m.Diagnostics {
			if i > 0 {
				diagMsg += " · "
			}
			diagMsg += d.Message
			if d.Severity == "violation" {
				violation = true
			}
		}
		for j, rb := range m.RoleBindings {
			diag := template.HTML(`<span style="font-family:var(--font-mono);font-size:11px;color:var(--ryg-green)">✓ ok</span>`)
			if j == 0 && diagMsg != "" {
				cls := "dx-diag"
				if violation {
					cls += " dx-diag--violation"
				}
				diag = template.HTML(`<span class="` + cls + `">` + template.HTMLEscapeString(diagMsg) + `</span>`)
			}
			rows = append(rows, web.Row{ID: rowID(m) + ":" + rb.RoleSlug + ":" + rb.Actor, Cells: map[string]web.Cell{
				"milestone": {HTML: milestoneRef(rowID(m))},
				"role":      {HTML: pill(roleVariant(rb.RoleSlug), rb.RoleSlug)},
				"actor":     {HTML: actorCell(rb.Actor)},
				"actorRole": {HTML: actorRoleTag(rb.RoleSlug)},
				"diag":      {HTML: diag},
			}})
		}
	}
	return buildSimpleSheet(cols, rows, "Bind role to milestone…", "")
}

func roleVariant(role string) string {
	switch role {
	case "pilot":
		return "blue"
	case "leadership":
		return "plum"
	default:
		return "neutral"
	}
}

// --- small cell helpers for the methodology sheets ---

func milestoneRef(id string) template.HTML {
	if id == "" {
		return template.HTML(`<span class="placeholder">—</span>`)
	}
	return template.HTML(`<span style="font-family:var(--font-mono);font-size:11.5px;color:var(--accent);font-weight:500">` +
		template.HTMLEscapeString(id) + `</span>`)
}

func actorRoleTag(role string) template.HTML {
	if role == "" {
		return template.HTML(`<span class="placeholder">—</span>`)
	}
	return template.HTML(`<span style="font-family:var(--font-mono);font-size:10px;color:var(--ink-3);letter-spacing:0.06em;text-transform:uppercase">` +
		template.HTMLEscapeString(role) + `</span>`)
}

func monoDay(ts string) template.HTML {
	if ts == "" {
		return template.HTML(`<span class="placeholder">—</span>`)
	}
	day := ts
	if len(ts) >= 10 {
		day = ts[:10]
	}
	return template.HTML(`<span style="font-family:var(--font-mono);font-size:11px;color:var(--ink-3)">` +
		template.HTMLEscapeString(day) + `</span>`)
}

// idlePill colours the decision idle-days count (≥9 red, ≥5 yellow, else green).
func idlePill(days int) template.HTML {
	if days <= 0 {
		return template.HTML(`<span style="color:var(--ink-4);font-family:var(--font-mono)">—</span>`)
	}
	variant := "g"
	if days >= 9 {
		variant = "r"
	} else if days >= 5 {
		variant = "y"
	}
	return template.HTML(`<span class="dx-pill dx-pill--` + variant + `" style="font-family:var(--font-mono)">` +
		strconv.Itoa(days) + `d</span>`)
}

func verdictPill(v string) template.HTML {
	switch v {
	case "achieved":
		return pill("g", "achieved")
	case "partial":
		return pill("y", "partial")
	case "missed":
		return pill("r", "missed")
	case "aborted":
		return pill("neutral", "aborted")
	default:
		return template.HTML(`<span class="placeholder">—</span>`)
	}
}

func valuePill(v string) template.HTML {
	switch v {
	case "high":
		return pill("g", "high")
	case "medium":
		return pill("y", "medium")
	case "low":
		return pill("r", "low")
	default:
		return template.HTML(`<span class="placeholder">—</span>`)
	}
}

// idleDays derives the days since a decision was last touched (UpdatedAt, else
// CreatedAt). Best-effort: unparseable timestamps yield 0.
func idleDays(m mcp.WorkItem) int {
	ts := firstNonEmpty(m.UpdatedAt, m.CreatedAt)
	if ts == "" {
		return 0
	}
	when, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return 0
	}
	d := int(time.Since(when).Hours() / 24)
	if d < 0 {
		return 0
	}
	return d
}

// parentMilestone returns the milestone a decision/outcome hangs off, from a
// derived_from / relates_to relation (best-effort), or "".
func parentMilestone(m mcp.WorkItem) string {
	for _, r := range m.Relations {
		if r.Type == "derived_from" || r.Type == "relates_to" || r.Type == "parent_of" {
			return r.TargetWorkItem
		}
	}
	return ""
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
