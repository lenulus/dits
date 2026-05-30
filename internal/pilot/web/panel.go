// The slide-over panel view-model — the Go port of the prototype's
// RecordPanel + SlideOver (app/panels.jsx, ds/shared.jsx). 640px right-side
// panel that ends 36px above the viewport so the hintbar sits below
// (enforced by .rx-slideover in app.css §9.4). Tab order is the
// post-iteration final: ACK · Status · Stages · Deps · Updates · Receipts.
package web

import "html/template"

// PanelTab is one tab in the slide-over tab bar.
type PanelTab struct {
	Key    string // tab id, e.g. "ACK"
	Label  string // visible label (usually == Key)
	Active bool
}

// MilestoneTabs is the canonical tab order for a milestone record (§9.4).
var MilestoneTabs = []string{"ACK", "Status", "Stages", "Deps", "Updates", "Receipts"}

// PanelModel is everything panel.html needs. Bodies are placeholders in the
// foundation; the per-tab content lands as the views fan out.
type PanelModel struct {
	ID        string        // record id line (e.g. "PROJ-204 · §251")
	Title     string        // record title
	Tabs      []PanelTab    // tab bar
	ActiveTab string        // active tab key
	Body      template.HTML // pre-rendered tab body (placeholder for now)
	Sig       string        // "signed · <sig>" footer
}

// NewMilestonePanel builds a PanelModel with the milestone tab bar, opening
// on initialTab (defaults to "ACK", the prototype's milestone default).
//
// TODO(phase 5): per-tab body rendering (ACK stand-behind cards, Status RYG
// control + composer, Stages, Deps, Updates feed, Receipts) as the views
// fan out. Today the body is a placeholder.
func NewMilestonePanel(id, title, initialTab string) *PanelModel {
	if initialTab == "" {
		initialTab = "ACK"
	}
	tabs := make([]PanelTab, 0, len(MilestoneTabs))
	for _, t := range MilestoneTabs {
		tabs = append(tabs, PanelTab{Key: t, Label: t, Active: t == initialTab})
	}
	return &PanelModel{ID: id, Title: title, Tabs: tabs, ActiveTab: initialTab}
}
