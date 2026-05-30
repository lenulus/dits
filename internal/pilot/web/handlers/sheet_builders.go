// Sheet view-model builders — the interactive grids (Portfolio, ACK, RFC,
// Decisions/Outcomes, Roles). Track B owns these together with the sheet
// primitive (web/sheet.go, sheet.html, sheet.js) and the inline-edit partial
// endpoints (sheet_edit.go), so the whole spreadsheet surface lives in one
// ownership boundary. Non-sheet view bodies (Attention, Leadership, Log,
// Taxonomies, Events) live in builders.go.
package handlers

import (
	"html/template"

	"github.com/lenulus/pf/internal/pilot/mcp"
	"github.com/lenulus/pf/internal/pilot/web"
)

// portfolioColumns is the canonical Portfolio column set + groups (§9.3),
// shared by the Portfolio and (a subset) the ACK view.
func portfolioColumns() []web.Column {
	return []web.Column{
		{Key: "id", Label: "ID", Width: 96, Group: "identity", Frozen: true, Readonly: true},
		{Key: "title", Label: "Title", Width: 260, Group: "identity", Frozen: true},
		{Key: "product", Label: "Product", Width: 150, Group: "classification"},
		{Key: "org", Label: "Org", Width: 150, Group: "classification"},
		{Key: "status", Label: "Status", Width: 140, Group: "health"},
		{Key: "specAck", Label: "S-ACK", Width: 104, Group: "acks"},
		{Key: "buildAck", Label: "B-ACK", Width: 104, Group: "acks"},
		{Key: "specifier", Label: "Specifier", Width: 140, Group: "roles"},
		{Key: "builder", Label: "Builder", Width: 140, Group: "roles"},
		{Key: "pilot", Label: "Pilot", Width: 140, Group: "roles"},
		{Key: "align", Label: "Alignment", Width: 140, Group: "delivery"},
		{Key: "diag", Label: "Diag", Width: 64, Group: "signals", Num: true},
	}
}

// buildPortfolioSheet maps milestones into the Portfolio sheet.
func buildPortfolioSheet(items []mcp.WorkItem, preset, group, activeID string) *web.SheetModel {
	if preset == "" {
		preset = "overview"
	}
	collapsed := web.CollapsedFromPreset(preset)
	if group != "" {
		collapsed[group] = !collapsed[group]
	}

	rows := make([]web.Row, 0, len(items))
	for _, m := range items {
		spec := roleActor(m, "specifier")
		build := roleActor(m, "builder")
		pilot := roleActor(m, "pilot")
		specAck, buildAck, rollup := "pending", "pending", "both_pending"
		if a, ok := latestAck(m); ok {
			specAck, buildAck = a.Specifier, a.Builder
			rollup = mcp.AckRollup(a.Specifier, a.Builder)
		}
		rows = append(rows, web.Row{
			ID: rowID(m),
			Cells: map[string]web.Cell{
				"id":        {HTML: idCell(rowID(m))},
				"title":     {Raw: m.Title},
				"product":   {HTML: classCell(m, "product")},
				"org":       {HTML: classCell(m, "org")},
				"status":    {HTML: statusPill(m.Status)},
				"specAck":   {HTML: ackStatePill(specAck)},
				"buildAck":  {HTML: ackStatePill(buildAck)},
				"specifier": {HTML: actorCell(spec)},
				"builder":   {HTML: actorCell(build)},
				"pilot":     {HTML: actorCell(pilot)},
				"align":     {HTML: rollupPill(rollup)},
				"diag":      {HTML: diagBadge(m)},
			},
		})
	}

	s := &web.SheetModel{
		Columns:      portfolioColumns(),
		Rows:         rows,
		Groups:       web.PortfolioGroups,
		Presets:      web.PortfolioPresets,
		Collapsed:    collapsed,
		Selectable:   true,
		AddLabel:     "New milestone…",
		ActiveID:     activeID,
		NavColumnIdx: 1,
	}
	s.ComputeLayout()
	return s
}

// buildSimpleSheet builds a flat (no column-group) sheet from items, given a
// column set and a per-row cell builder. Used by RFC/Decisions/Outcomes/Roles.
func buildSimpleSheet(cols []web.Column, rows []web.Row, addLabel string) *web.SheetModel {
	s := &web.SheetModel{
		Columns:      cols,
		Rows:         rows,
		Selectable:   true,
		AddLabel:     addLabel,
		NavColumnIdx: 1,
	}
	s.ComputeLayout()
	return s
}

func buildRfcSheet(items []mcp.WorkItem) *web.SheetModel {
	cols := []web.Column{
		{Key: "id", Label: "ID", Width: 96, Frozen: true, Readonly: true},
		{Key: "title", Label: "Title", Width: 280, Frozen: true},
		{Key: "status", Label: "Status", Width: 160},
		{Key: "specifier", Label: "Specifier", Width: 160},
		{Key: "body", Label: "Summary", Width: 480},
	}
	rows := make([]web.Row, 0, len(items))
	for _, r := range items {
		rows = append(rows, web.Row{ID: rowID(r), Cells: map[string]web.Cell{
			"id":        {HTML: idCell(rowID(r))},
			"title":     {Raw: r.Title},
			"status":    {HTML: statusPill(r.Status)},
			"specifier": {HTML: actorCell(roleActor(r, "specifier"))},
			"body":      {Raw: truncate(r.Body, 140)},
		}})
	}
	return buildSimpleSheet(cols, rows, "New RFC…")
}

// buildAckSheet is the bulk-ack workspace: S-ACK / B-ACK / alignment per
// milestone (§9.2, AckView).
func buildAckSheet(items []mcp.WorkItem) *web.SheetModel {
	cols := []web.Column{
		{Key: "id", Label: "ID", Width: 96, Frozen: true, Readonly: true},
		{Key: "title", Label: "Milestone", Width: 280, Frozen: true},
		{Key: "specifier", Label: "Specifier", Width: 150},
		{Key: "specAck", Label: "Specifier ACK", Width: 150},
		{Key: "builder", Label: "Builder", Width: 150},
		{Key: "buildAck", Label: "Builder ACK", Width: 150},
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
	return buildSimpleSheet(cols, rows, "")
}

func buildKindSheet(items []mcp.WorkItem, addLabel string) *web.SheetModel {
	cols := []web.Column{
		{Key: "id", Label: "ID", Width: 96, Frozen: true, Readonly: true},
		{Key: "title", Label: "Title", Width: 320, Frozen: true},
		{Key: "status", Label: "Status", Width: 150},
		{Key: "specifier", Label: "Specifier", Width: 160},
		{Key: "body", Label: "Detail", Width: 460},
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
	return buildSimpleSheet(cols, rows, addLabel)
}

// buildRolesSheet flattens every role binding across milestones into one sheet
// with per-binding diagnostics (§9.7 — constraints flag, they don't gate).
func buildRolesSheet(items []mcp.WorkItem) *web.SheetModel {
	cols := []web.Column{
		{Key: "milestone", Label: "Milestone", Width: 120, Frozen: true, Readonly: true},
		{Key: "role", Label: "Role", Width: 130},
		{Key: "actor", Label: "Actor", Width: 200},
		{Key: "diag", Label: "Constraint diagnostic", Width: 440, Readonly: true},
	}
	var rows []web.Row
	for _, m := range items {
		// One diagnostics summary per milestone, shown on its first binding row.
		diagMsg := ""
		for i, d := range m.Diagnostics {
			if i > 0 {
				diagMsg += " · "
			}
			diagMsg += d.Message
		}
		for j, rb := range m.RoleBindings {
			cell := template.HTML(`<span style="font-family:var(--font-mono);font-size:11px;color:var(--ryg-green)">✓ ok</span>`)
			if j == 0 && diagMsg != "" {
				cell = template.HTML(`<span class="dx-diag dx-diag--violation">` + template.HTMLEscapeString(diagMsg) + `</span>`)
			}
			rows = append(rows, web.Row{ID: rowID(m) + ":" + rb.RoleSlug, Cells: map[string]web.Cell{
				"milestone": {Raw: rowID(m)},
				"role":      {HTML: pill(roleVariant(rb.RoleSlug), rb.RoleSlug)},
				"actor":     {HTML: actorCell(rb.Actor)},
				"diag":      {HTML: cell},
			}})
		}
	}
	return buildSimpleSheet(cols, rows, "Bind role to milestone…")
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
