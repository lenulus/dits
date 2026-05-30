// View metadata for the twelve Pilot routes and the four sidebar sections.
// Mirrors the prototype's App.jsx VIEWS map and Sidebar (§9.2). The header
// eyebrow/h1/sub text is lifted from VIEWS so the chrome reads identically.
package web

import "html/template"

// View is the chrome metadata for one route: the sidebar label, the
// monospace eyebrow tag, the icon key (matches the prototype's Lucide-ish
// icon set), and the header h1/sub copy.
type View struct {
	Key   string // route key, e.g. "portfolio"; path is "/" + Key (attention → /for-you)
	Path  string // the actual GET path
	Label string // sidebar nav label
	Tag   string // eyebrow tag, e.g. "DATA"
	Icon  string // icon key in the embedded icon set
	H1    string // page header
	Sub   string // page sub (serif italic dek)
}

// NavSection groups views in the sidebar (Workspace / Methodology /
// Substrate / External), matching App.jsx's Sidebar sections.
type NavSection struct {
	Label string
	Keys  []string // view keys, in display order
}

// Views is the canonical map of route key → chrome metadata. Copy is lifted
// verbatim from App.jsx VIEWS so the prototype port reads identically.
var Views = map[string]View{
	"attention": {
		Key: "attention", Path: "/for-you", Label: "For you", Tag: "ATTENTION", Icon: "home",
		H1:  "For you",
		Sub: "Everything the substrate thinks you should touch today. ACKs pending, targets passing, risks flagged, decisions idle. Inline actions — no detours.",
	},
	"leadership": {
		Key: "leadership", Path: "/leadership", Label: "Leadership", Tag: "LEADERSHIP", Icon: "home",
		H1:  "Leadership portfolio",
		Sub: "Four leading indicators on the substrate. Goal rollup, signal health, pattern blocks. No project list.",
	},
	"portfolio": {
		Key: "portfolio", Path: "/portfolio", Label: "Portfolio", Tag: "DATA", Icon: "panel",
		H1:  "Portfolio",
		Sub: "Every cell is a signed event waiting to happen. Click to edit. Cmd+Enter opens the record panel. No modals.",
	},
	"ack": {
		Key: "ack", Path: "/ack", Label: "ACK", Tag: "ACK", Icon: "check",
		H1:  "ACK",
		Sub: "Specifier ACK and Builder ACK — the only people who can stand behind a commitment. Click any cell. A = Accept · P = Pending · R = Reject.",
	},
	"rfcs": {
		Key: "rfcs", Path: "/rfcs", Label: "RFCs", Tag: "GOVERNANCE", Icon: "flag",
		H1:  "RFC review queue",
		Sub: "Specifiers propose. Leadership approves. Backlog and resourcing live here. Drag the status cell forward.",
	},
	"log": {
		Key: "log", Path: "/log", Label: "Pilot's Log", Tag: "PILOT", Icon: "compass",
		H1:  "Pilot's Log",
		Sub: "Pilot’s journal. The top row is a quick-entry: pick a kind, type the note, Cmd+Enter files a signed event.",
	},
	"decisions": {
		Key: "decisions", Path: "/decisions", Label: "Decisions", Tag: "BLOCKED", Icon: "scale",
		H1:  "DecisionBlocks",
		Sub: "What is the team stuck on? Idle ≥5d turns the chip yellow; ≥9d the scheduler escalates to Leadership.",
	},
	"outcomes": {
		Key: "outcomes", Path: "/outcomes", Label: "Outcomes", Tag: "EVAL", Icon: "check",
		H1:  "Outcome assessments",
		Sub: "Did the milestone matter? Verdict + value. “Achieved · low” is the pattern that wakes Leadership.",
	},
	"roles": {
		Key: "roles", Path: "/roles", Label: "Role bindings", Tag: "SUBSTRATE", Icon: "users",
		H1:  "Role bindings",
		Sub: "Specifier, Builder, Pilot, Reviewer, Leadership. Constraints flag — they don’t gate. Every change is an event.",
	},
	"taxonomies": {
		Key: "taxonomies", Path: "/taxonomies", Label: "Taxonomies", Tag: "SUBSTRATE", Icon: "tag",
		H1:  "Taxonomies",
		Sub: "Three independent classification axes — org, product, and goals. Goals: Goal → node → result. Nodes live in meta, versioned with the project.",
	},
	"events": {
		Key: "events", Path: "/events", Label: "Event log", Tag: "SUBSTRATE", Icon: "book",
		H1:  "Event log",
		Sub: "The DAG’s ground truth. Every row is a signed event. Read-only here — mutate elsewhere, the log catches it.",
	},
	"roadmap": {
		Key: "roadmap", Path: "/roadmap", Label: "Customer roadmap", Tag: "EXTERNAL", Icon: "map",
		H1:  "Customer roadmap",
		Sub: "The only public surface. No RYG, no indicators, no internal commentary. Just committed scope and timing.",
	},
}

// NavSections is the sidebar layout: four sections, twelve items, in the
// prototype's order.
var NavSections = []NavSection{
	{Label: "Workspace", Keys: []string{"attention", "leadership", "portfolio", "ack", "rfcs", "log"}},
	{Label: "Methodology", Keys: []string{"decisions", "outcomes"}},
	{Label: "Substrate", Keys: []string{"roles", "taxonomies", "events"}},
	{Label: "External", Keys: []string{"roadmap"}},
}

// navItem is the rendered sidebar entry the layout template ranges over.
type navItem struct {
	View
	Active bool
	Count  string // optional count badge; empty = omit
}

// navSectionVM is a section with its items resolved for rendering.
type navSectionVM struct {
	Label string
	Items []navItem
}

// Page is the top-level view-model the layout template renders. Handlers
// build a Page (chrome + optional Sheet/Panel) and call Render(w,"layout",p).
type Page struct {
	Active   string         // active view key
	View     View           // the active view's chrome metadata
	Nav      []navSectionVM // resolved sidebar
	Counters string         // eyebrow counter text (e.g. "12 total")
	Body     template.HTML  // pre-rendered body HTML (sheet, attention list, etc.)
	Sheet    *SheetModel    // optional: when set, layout renders the sheet primitive
	Panel    *PanelModel    // optional: when set, the slide-over opens for this record
}

// NewPage assembles a Page for the given view key with the sidebar resolved
// and active item marked. Counts are left blank in the foundation; handlers
// or a later projection step can fill them.
func NewPage(activeKey string) *Page {
	v := Views[activeKey]
	nav := make([]navSectionVM, 0, len(NavSections))
	for _, sec := range NavSections {
		items := make([]navItem, 0, len(sec.Keys))
		for _, k := range sec.Keys {
			items = append(items, navItem{
				View:   Views[k],
				Active: k == activeKey,
			})
		}
		nav = append(nav, navSectionVM{Label: sec.Label, Items: items})
	}
	return &Page{Active: activeKey, View: v, Nav: nav}
}
