// Demo data for the Phase-5 foundation. None of this touches MCP — it is a
// small hardcoded dataset so the rendered chrome (sheet, panel, chips) is
// visibly working before the typed MCP client is wired. Each builder here is
// the seam where MCP data replaces literals: a handler will construct the
// same *web.SheetModel / *web.PanelModel from MCP rows instead of these
// constants, with no template or layout changes.
package handlers

import (
	"fmt"
	"html/template"

	"github.com/lenulus/pf/internal/pilot/web"
)

// todo renders a uniform placeholder body for views whose content lands as
// the views fan out in Phase 5. Kept here so handlers stay one-liners.
func todo(route, what string) template.HTML {
	return template.HTML(fmt.Sprintf(
		`<div class="rx-panel"><div class="rx-panel__body">`+
			`<div class="empty-line">TODO(phase 5): %s — <strong>%s</strong>. `+
			`Chrome (TopBar · Sidebar · header · hintbar · ⌘K) is live; this view's body lands as the views fan out.</div>`+
			`</div></div>`, template.HTMLEscapeString(route), template.HTMLEscapeString(what)))
}

// chip is a small helper that emits a dx-pill fragment (trusted markup, never
// user input) for the demo cells.
func pill(variant, label string) template.HTML {
	return template.HTML(fmt.Sprintf(`<span class="dx-pill dx-pill--%s"><span class="dot"></span>%s</span>`,
		variant, template.HTMLEscapeString(label)))
}

// demoMilestone is one hardcoded portfolio row.
type demoMilestone struct {
	ID, Title, Status, RYG, Spec, Builder, Target, Product string
}

var demoMilestones = []demoMilestone{
	{"PROJ-204", "Custodial signing path", "in_flight", "g", "E. Jackson", "K. Rivas", "2026 Q3", "identity/keys"},
	{"PROJ-211", "MCP tool surface buildout", "ack_committed", "y", "E. Jackson", "—", "2026 Q3", "substrate/mcp"},
	{"PROJ-198", "Indicator projection cache", "draft", "r", "M. Okafor", "K. Rivas", "2026 Q4", "—"},
	{"PROJ-220", "Public roadmap projection", "shipped", "g", "E. Jackson", "L. Tran", "2026 Q2", "external/roadmap"},
}

// demoPortfolioSheet builds the demo Portfolio sheet: the canonical column
// groups (§9.3), JTBD presets, and a handful of rows with rendered chips.
// preset selects a JTBD preset (default "overview"); group toggles a single
// column group on top of the preset; activeID marks the open row.
func demoPortfolioSheet(preset, group, activeID string) *web.SheetModel {
	if preset == "" {
		preset = "overview"
	}
	collapsed := web.CollapsedFromPreset(preset)
	if group != "" {
		// Toggle the requested group on top of the preset (the band-click path).
		collapsed[group] = !collapsed[group]
	}

	cols := []web.Column{
		// Identity (frozen — ID + Title are left-sticky, §9.3 non-negotiable).
		{Key: "id", Label: "ID", Width: 96, Group: "identity", Frozen: true, Readonly: true},
		{Key: "title", Label: "Title", Width: 260, Group: "identity", Frozen: true},
		// Classification.
		{Key: "product", Label: "Product", Width: 150, Group: "classification"},
		// Health.
		{Key: "ryg", Label: "RYG", Width: 110, Group: "health"},
		{Key: "status", Label: "Status", Width: 140, Group: "health"},
		// ACKs.
		{Key: "specAck", Label: "Spec ACK", Width: 110, Group: "acks"},
		{Key: "buildAck", Label: "Build ACK", Width: 110, Group: "acks"},
		// Roles.
		{Key: "specifier", Label: "Specifier", Width: 140, Group: "roles"},
		{Key: "builder", Label: "Builder", Width: 140, Group: "roles"},
		// Delivery.
		{Key: "target", Label: "Target", Width: 110, Group: "delivery"},
		// Updates.
		{Key: "updates", Label: "Status →", Width: 200, Group: "narrative"},
		// Signals.
		{Key: "deps", Label: "Deps", Width: 90, Group: "signals", Num: true},
	}

	rows := make([]web.Row, 0, len(demoMilestones))
	for _, m := range demoMilestones {
		rows = append(rows, web.Row{
			ID: m.ID,
			Cells: map[string]web.Cell{
				"id":        {HTML: template.HTML(`<span class="dx-id">` + m.ID + `</span>`)},
				"title":     {Raw: m.Title},
				"product":   {Raw: m.Product},
				"ryg":       {HTML: rygPill(m.RYG)},
				"status":    {Raw: statusLabel(m.Status)},
				"specAck":   {HTML: pill("g", "accepted")},
				"buildAck":  {HTML: ackPill(m.Builder)},
				"specifier": {Raw: m.Spec},
				"builder":   {Raw: m.Builder},
				"target":    {Raw: m.Target},
				"updates":   {Raw: "—"},
				"deps":      {Raw: "0/0"},
			},
		})
	}

	s := &web.SheetModel{
		Columns:      cols,
		Rows:         rows,
		Groups:       web.PortfolioGroups,
		Presets:      web.PortfolioPresets,
		Collapsed:    collapsed,
		Selectable:   true,
		AddLabel:     "New milestone…",
		ActiveID:     activeID,
		NavColumnIdx: 1, // Title opens the panel
	}
	s.ComputeLayout()
	return s
}

func rygPill(ryg string) template.HTML {
	switch ryg {
	case "g":
		return pill("g", "Green")
	case "y":
		return pill("y", "Yellow")
	case "r":
		return pill("r", "Red")
	default:
		return pill("stale", "Stale")
	}
}

func ackPill(builder string) template.HTML {
	if builder == "—" {
		return pill("stale", "pending")
	}
	return pill("neutral", "pending")
}

func statusLabel(s string) string {
	switch s {
	case "in_flight":
		return "In flight"
	case "ack_committed":
		return "ACK committed"
	case "shipped":
		return "Shipped"
	case "draft":
		return "Draft"
	default:
		return s
	}
}

func demoPortfolioCounter() string {
	return fmt.Sprintf("%d total", len(demoMilestones))
}

// demoPortfolioPanel builds the slide-over for an open milestone row, opening
// on the requested tab (default ACK).
func demoPortfolioPanel(id, tab string) *web.PanelModel {
	title := id
	for _, m := range demoMilestones {
		if m.ID == id {
			title = m.Title
			break
		}
	}
	return web.NewMilestonePanel(id, title, tab)
}
