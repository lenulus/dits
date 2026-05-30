// The generic sheet view-model — the Go port of the prototype's Sheet props
// (app/sheet.jsx). The Sheet primitive is the gate the twelve views fan out
// through (§9.3), so this model is deliberately data-only: columns, rows,
// column groups, JTBD presets, and the collapsed-group set. Handlers build a
// SheetModel from MCP results (demo data for now) and hand it to the layout.
package web

import "html/template"

// Column describes one sheet column. Mirrors sheet.jsx's column shape
// (key,label,width,frozen,group,align,num,readonly,edit,options).
type Column struct {
	Key      string // row-map key this column reads
	Label    string // header label
	Width    int    // fixed px width (0 → DefaultColWidth); load-bearing, see TotalWidth
	Group    string // column-group key (matches a ColumnGroup.Key)
	Frozen   bool   // left-sticky (ID + Title are frozen — §9.3 non-negotiable)
	Align    string // "", "right"
	Num      bool   // monospace tabular-nums, right-aligned
	Readonly bool   // not inline-editable

	// Inline-edit descriptor (Track B). Edit names the editor kind sheet.html
	// emits hx-* attributes for; "" means the cell is not inline-editable even
	// when !Readonly. Options supplies the choices for a "select" editor.
	Edit    string   // "select" | "actor" | "taxonomy" | "target" | "ack" | "text"
	Options []Option // for Edit == "select"
}

// Option is one choice in a select editor (value sent on commit, label shown).
type Option struct {
	Value string
	Label string
}

// ColumnGroup is a collapsible band over a run of columns (§9.3 column
// groups). When collapsed it renders as a single summary cell. Order is
// Identity → Classification → Health → ACKs → Roles → Delivery → Updates →
// Signals (the prototype's fixed order).
type ColumnGroup struct {
	Key        string // group key (matches Column.Group)
	Label      string // band label
	AlwaysOpen bool   // Identity is always open / never collapses
}

// Preset is a JTBD column-group preset (§9.3): a named set of collapsed
// groups. "Overview" (default) collapses everything except Identity+Updates.
type Preset struct {
	Value     string   // preset id
	Label     string   // toolbar label
	Collapsed []string // group keys collapsed under this preset
}

// Cell is one rendered cell value for a row. Raw renders as text; HTML lets a
// handler inject a chip/pill/avatar fragment (built with trusted template
// helpers, never user input).
type Cell struct {
	Raw  string
	HTML template.HTML
}

// Row is an opaque record: its ID plus a cell per column key.
type Row struct {
	ID    string
	Cells map[string]Cell
}

// SheetModel is everything sheet.html needs to render. It carries the
// pre-computed TotalWidth and per-frozen-column left offsets so the
// server-rendered table is correct before the JS island hydrates — these
// are the prototype's hard-won layout fixes (see ComputeLayout).
type SheetModel struct {
	Columns      []Column
	Rows         []Row
	Groups       []ColumnGroup
	Presets      []Preset
	Collapsed    map[string]bool // group key → collapsed
	GroupBy      string          // "", "status", "team", "product", "quarter"
	Selectable   bool            // render the multi-select gutter
	AddLabel     string          // quick-add row label ("" → no add row)
	ActiveID     string          // row currently open in the panel (is-active)
	NavColumnIdx int             // which display-column index opens the panel on click

	// Track B interactive chrome. Toolbar (when non-nil) renders the
	// preset/group-by/filter toolbar above the grid; the partial endpoints
	// drive these via query params. AddKind / AddURL wire the quick-add row to
	// WorkCreate. BulkURL is the bulk-action endpoint the selection bar posts to.
	Toolbar *Toolbar
	Groups2 []RowGroup // pre-grouped rows for GroupBy != "" (row banding)
	AddKind string     // work kind the quick-add row creates ("" → inert add row)
	BulkURL string     // /partials/sheet/bulk endpoint ("" → no bulk bar)

	// Computed by ComputeLayout — do not set directly.
	TotalWidth  int
	frozenLeft  map[string]int // column key → sticky left px
	displayCols []displayCol
}

// Toolbar is the Portfolio toolbar view-model (preset Pivot, group-by Pivot,
// filter chips, +Add-filter popover, count, +New action). Driven by the query
// params the partial endpoints round-trip. Mirrors App.jsx PortfolioToolbar.
type Toolbar struct {
	BaseURL      string        // the view's path, e.g. "/portfolio" (chips/group links target it)
	Preset       string        // active preset value
	Presets      []Preset      // preset Pivot options
	GroupBy      string        // active group-by key
	GroupByOpts  []Option      // group-by Pivot options
	Filters      []FilterPill  // active filter chips
	FilterFields []FilterField // the +Add-filter field menu (two-step popover)
	Total        int           // total-count badge
	AddLabel     string        // "+New …" action label ("" → no action button)
}

// FilterPill is one active filter chip (label · op · value), removable/editable.
type FilterPill struct {
	Field    string // filter field key
	Label    string // human field label
	Op       string // "is" (only op today)
	Value    string // raw value
	ValueLab string // display value
}

// FilterField is one entry in the +Add-filter field menu. Bool fields commit
// immediately; enum/actor fields open a value list (Options / actor picker).
type FilterField struct {
	Field   string   // query-param key
	Label   string   // menu label
	Kind    string   // "enum" | "actor" | "bool"
	Options []Option // value list for "enum"
}

// RowGroup is a contiguous band of rows under a group header (Track B group-by).
type RowGroup struct {
	Key   string // group header label
	Rows  []Row
	Count int
}

// DefaultColWidth matches sheet.jsx's fallback (140px).
const DefaultColWidth = 140

// gutterWidth matches the .sht-gutter / colgroup 32px in the prototype.
const gutterWidth = 32

// summaryWidth matches sheet.jsx's collapsed-group summary column (132px).
const summaryWidth = 132

// displayCol is a resolved column after group collapse: either a real column
// or a synthetic group-summary column.
type displayCol struct {
	Key        string
	Label      string
	Width      int
	Frozen     bool
	Align      string
	Num        bool
	Readonly   bool
	Edit       string
	Options    []Option
	GroupKey   string
	IsSummary  bool
	FrozenLeft int // sticky left offset when Frozen
}

// Editable reports whether this display column opens an inline editor on
// click/Enter (a non-readonly, non-summary column with an editor kind).
func (d displayCol) Editable() bool {
	return !d.Readonly && !d.IsSummary && d.Edit != ""
}

// PortfolioGroups is the canonical column-group order (§9.3). Identity is
// alwaysOpen. Exposed so handlers building portfolio-shaped sheets stay
// consistent.
var PortfolioGroups = []ColumnGroup{
	{Key: "identity", Label: "Identity", AlwaysOpen: true},
	{Key: "classification", Label: "Classification"},
	{Key: "health", Label: "Health"},
	{Key: "acks", Label: "ACKs"},
	{Key: "roles", Label: "Roles"},
	{Key: "delivery", Label: "Delivery"},
	{Key: "narrative", Label: "Updates"},
	{Key: "signals", Label: "Signals"},
}

// PortfolioPresets are the JTBD presets (§9.3). Overview (default) collapses
// everything except Identity (alwaysOpen) and Updates.
var PortfolioPresets = []Preset{
	{Value: "overview", Label: "Overview", Collapsed: []string{"health", "classification", "acks", "roles", "delivery", "signals"}},
	{Value: "ownership", Label: "Ownership", Collapsed: []string{"health", "classification", "acks", "delivery", "narrative", "signals"}},
	{Value: "status", Label: "Status & risk", Collapsed: []string{"health", "classification", "acks", "roles", "delivery", "signals"}},
	{Value: "everything", Label: "Everything", Collapsed: []string{}},
}

// CollapsedFromPreset returns the collapsed-set for a named preset (default
// "overview" if unknown).
func CollapsedFromPreset(preset string) map[string]bool {
	set := map[string]bool{}
	for _, p := range PortfolioPresets {
		if p.Value == preset {
			for _, g := range p.Collapsed {
				set[g] = true
			}
			return set
		}
	}
	// default: overview
	for _, g := range PortfolioPresets[0].Collapsed {
		set[g] = true
	}
	return set
}

// ComputeLayout resolves the display columns (applying group collapse) and
// computes TotalWidth + frozen-left offsets. This reproduces sheet.jsx's
// totalWidth() and frozenLeftOf(): the table width is pinned to the exact
// sum of column widths (+ gutter) so `table-layout: fixed` and the sticky
// frozen columns line up — the bug the prototype debugged at length.
// Handlers must call this after populating Columns/Groups/Collapsed.
func (s *SheetModel) ComputeLayout() {
	s.displayCols = s.resolveDisplayColumns()

	// Frozen-left offsets: each frozen column sits at gutter + sum of the
	// widths of all preceding *contiguous* frozen columns.
	s.frozenLeft = map[string]int{}
	left := 0
	if s.Selectable {
		left = gutterWidth
	}
	for i := range s.displayCols {
		dc := &s.displayCols[i]
		if dc.Frozen {
			dc.FrozenLeft = left
			s.frozenLeft[dc.Key] = left
			left += colWidth(dc.Width)
		} else {
			// once we hit a non-frozen column the frozen run is over
			break
		}
	}

	// Total width = gutter + sum of every display column's width.
	total := 0
	if s.Selectable {
		total = gutterWidth
	}
	for _, dc := range s.displayCols {
		total += colWidth(dc.Width)
	}
	s.TotalWidth = total
}

// DisplayColumns returns the resolved columns (after group collapse) for the
// template. Safe to call after ComputeLayout.
func (s *SheetModel) DisplayColumns() []displayCol { return s.displayCols }

// resolveDisplayColumns collapses any group in Collapsed (except AlwaysOpen)
// into a single synthetic summary column, preserving group order. Mirrors
// sheet.jsx's displayColumns memo.
func (s *SheetModel) resolveDisplayColumns() []displayCol {
	if len(s.Groups) == 0 {
		out := make([]displayCol, 0, len(s.Columns))
		for _, c := range s.Columns {
			out = append(out, displayCol{
				Key: c.Key, Label: c.Label, Width: c.Width, Frozen: c.Frozen,
				Align: c.Align, Num: c.Num, Readonly: c.Readonly,
				Edit: c.Edit, Options: c.Options, GroupKey: c.Group,
			})
		}
		return out
	}
	byGroup := map[string][]Column{}
	for _, c := range s.Columns {
		g := c.Group
		if g == "" {
			g = "_default"
		}
		byGroup[g] = append(byGroup[g], c)
	}
	out := []displayCol{}
	for _, g := range s.Groups {
		collapsed := s.Collapsed[g.Key] && !g.AlwaysOpen
		cols := byGroup[g.Key]
		if collapsed {
			frozen := false
			if len(cols) > 0 {
				frozen = cols[0].Frozen
			}
			out = append(out, displayCol{
				Key: "__group_" + g.Key, Label: g.Label, Width: summaryWidth,
				Readonly: true, Frozen: frozen, GroupKey: g.Key, IsSummary: true,
			})
		} else {
			for _, c := range cols {
				out = append(out, displayCol{
					Key: c.Key, Label: c.Label, Width: c.Width, Frozen: c.Frozen,
					Align: c.Align, Num: c.Num, Readonly: c.Readonly,
					Edit: c.Edit, Options: c.Options, GroupKey: g.Key,
				})
			}
		}
	}
	for _, c := range byGroup["_default"] {
		out = append(out, displayCol{
			Key: c.Key, Label: c.Label, Width: c.Width, Frozen: c.Frozen,
			Align: c.Align, Num: c.Num, Readonly: c.Readonly,
			Edit: c.Edit, Options: c.Options, GroupKey: "_default",
		})
	}
	return out
}

func colWidth(w int) int {
	if w <= 0 {
		return DefaultColWidth
	}
	return w
}

// BandCell is one column-group band in the sheet header (the .sht-bands row).
type BandCell struct {
	Key       string
	Label     string
	Span      int
	Collapsed bool
	Frozen    bool
	Locked    bool // AlwaysOpen group → not clickable
	Left      int  // sticky left offset when Frozen
}

// BandCells returns the column-group band row for the template, mirroring
// sheet.jsx's band-building IIFE. Each band spans its group's display
// columns; a fully-frozen band gets the same sticky left offset as its
// first column. Safe to call after ComputeLayout.
func (s *SheetModel) BandCells() []BandCell {
	if len(s.Groups) == 0 {
		return nil
	}
	// Index display columns by group key, preserving order.
	out := []BandCell{}
	for _, g := range s.Groups {
		// gather display columns belonging to this group
		var span int
		var frozenAll = true
		var sawAny bool
		left := -1
		for _, dc := range s.displayCols {
			if dc.GroupKey != g.Key {
				continue
			}
			sawAny = true
			span++
			if !dc.Frozen {
				frozenAll = false
			} else if left < 0 {
				left = dc.FrozenLeft
			}
		}
		if !sawAny {
			continue
		}
		collapsed := s.Collapsed[g.Key] && !g.AlwaysOpen
		out = append(out, BandCell{
			Key: g.Key, Label: g.Label, Span: span,
			Collapsed: collapsed, Frozen: frozenAll && left >= 0,
			Locked: g.AlwaysOpen, Left: left,
		})
	}
	return out
}
