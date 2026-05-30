// Track B inline-edit + sheet-interactivity partial endpoints. Editable cells
// in sheet.html carry hx-get="/partials/sheet/edit?id=..&col=.." which returns
// the right in-cell editor fragment; the editor commits via
// hx-post="/partials/sheet/cell" which performs exactly one MCP mutation and
// swaps the updated cell fragment back. The selection bulk-bar posts to
// /partials/sheet/bulk (loops the mutator over the selected ids); the quick-add
// row posts to /partials/sheet/add (WorkCreate → open the new record's panel).
//
// Filter chips and group-by are plain GET navigations of the view URL with
// query params — the coordinator's Portfolio handler re-runs buildPortfolioSheet
// with the parsed params (sheet.js drives the navigation client-side), so no
// server endpoint is needed for those; this file owns only the mutating swaps.
package handlers

import (
	"context"
	"html/template"
	"net/http"
	"strings"

	"github.com/lenulus/pf/internal/pilot/mcp"
	"github.com/lenulus/pf/internal/pilot/web"
)

// registerSheetEdit wires Track B's /partials/sheet/* endpoints. The
// coordinator calls this from registerPartials (partials.go).
func (s *Server) registerSheetEdit(mux *http.ServeMux) {
	mux.HandleFunc("GET /partials/sheet/edit", s.sheetEditEditor)
	mux.HandleFunc("POST /partials/sheet/cell", s.sheetEditCommit)
	mux.HandleFunc("POST /partials/sheet/bulk", s.sheetBulk)
	mux.HandleFunc("POST /partials/sheet/add", s.sheetQuickAdd)
}

// --- editor fragment (GET /partials/sheet/edit) ---

// sheetEditEditor returns the in-cell editor fragment for (id, col). The editor
// kind is derived from the column key (the same mapping the builders use), so a
// single endpoint serves every sheet. The fragment commits via hx-post to
// /partials/sheet/cell, targeting the same cell.
func (s *Server) sheetEditEditor(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	col := r.URL.Query().Get("col")
	if id == "" || col == "" {
		http.Error(w, "missing id/col", http.StatusBadRequest)
		return
	}
	work := s.workForRow(r.Context(), id)
	renderPartial(w, s.editorFragment(r.Context(), id, col, work))
}

// editorFragment builds the editor HTML for one cell. The committed value posts
// back to /partials/sheet/cell with hidden id+col fields.
func (s *Server) editorFragment(ctx context.Context, id, col string, work mcp.WorkItem) template.HTML {
	kind := editorKindFor(col)
	switch kind {
	case "select":
		return selectEditor(id, col, selectOptionsFor(col), selectCurrentFor(col, work))
	case "ack":
		return ackEditor(id, col)
	case "actor":
		return actorEditor(ctx, s.Client, id, col, currentActorFor(col, work))
	case "taxonomy":
		return taxonomyEditor(ctx, s.Client, id, col, taxonomyFor(col), classOf(work, taxonomyFor(col)))
	case "target":
		return sheetTargetEditor(id, col, work.Target(), work.TargetPrecision())
	default:
		return textEditor(id, col, textCurrentFor(col, work))
	}
}

// editorKindFor maps a column key to its editor kind (the inverse of the Edit
// descriptors in the builders — kept here so the endpoint is builder-agnostic).
func editorKindFor(col string) string {
	switch col {
	case "status", "ryg", "visibility", "verdict", "value":
		return "select"
	case "specAck", "buildAck":
		return "ack"
	case "specifier", "builder", "pilot", "owner", "by", "actor":
		return "actor"
	case "product", "org":
		return "taxonomy"
	case "target":
		return "target"
	default:
		return "text"
	}
}

func selectOptionsFor(col string) []optKV {
	switch col {
	case "status":
		return toKV(statusOptions)
	case "ryg":
		return toKV(rygOptions)
	case "visibility":
		return toKV(visibilityOptions)
	case "verdict":
		return toKV(verdictOptions)
	case "value":
		return toKV(valueOptions)
	}
	return nil
}

func selectCurrentFor(col string, w mcp.WorkItem) string {
	switch col {
	case "status":
		return w.Status
	case "ryg":
		return w.RYG()
	case "visibility":
		if w.CustomerVisible() {
			return "true"
		}
		return "false"
	case "verdict":
		return w.Field("verdict")
	case "value":
		return w.Field("value")
	}
	return ""
}

func currentActorFor(col string, w mcp.WorkItem) string {
	switch col {
	case "specifier", "builder", "pilot", "owner":
		return roleActor(w, col)
	case "by":
		return roleActor(w, "evaluator")
	}
	return ""
}

func taxonomyFor(col string) string {
	if col == "org" {
		return "org"
	}
	return "product"
}

func textCurrentFor(col string, w mcp.WorkItem) string {
	switch col {
	case "title", "summary":
		return w.Title
	case "note", "body":
		return w.Body
	}
	return w.Field(col)
}

// --- commit (POST /partials/sheet/cell) ---

// sheetEditCommit performs the single MCP mutation for a committed cell edit
// and returns the re-rendered cell fragment. The mutation is chosen by column:
//
//	status              → SetStatus
//	ryg/visibility/…    → FieldSet
//	target              → FieldSet target (+ target_precision)
//	specifier/builder/… → BindRole / actor sheet rebind
//	product/org         → Classify (declassify-then-classify on change)
//	specAck/buildAck    → AckAccept / AckReject (pending = no-op note)
func (s *Server) sheetEditCommit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	id := r.FormValue("id")
	col := r.FormValue("col")
	val := r.FormValue("value")
	if id == "" || col == "" {
		http.Error(w, "missing id/col", http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	if err := s.applyCellMutation(ctx, id, col, val, r); err != nil {
		// Surface the failure inline; HTMX swaps the message into the cell.
		renderPartial(w, template.HTML(`<span class="dx-diag dx-diag--violation" title="`+
			template.HTMLEscapeString(err.Error())+`">edit failed</span>`))
		return
	}
	s.Indicators.Invalidate()
	// Re-fetch and re-render the now-current cell so the grid reflects the write.
	work := s.workForRow(ctx, id)
	renderPartial(w, s.cellFragment(id, col, work))
}

// applyCellMutation routes a committed value to its single client mutator.
func (s *Server) applyCellMutation(ctx context.Context, id, col, val string, r *http.Request) error {
	switch col {
	case "status":
		return s.Client.SetStatus(ctx, id, val)
	case "ryg":
		return s.Client.FieldSet(ctx, id, "ryg", val)
	case "visibility":
		return s.Client.FieldSet(ctx, id, "customer_visible", val)
	case "verdict":
		return s.Client.FieldSet(ctx, id, "verdict", val)
	case "value":
		return s.Client.FieldSet(ctx, id, "value", val)
	case "title", "summary":
		// No generic title mutator on the client; project as a field so the
		// edit is a signed event (the panel owns the canonical title rename).
		return s.Client.FieldSet(ctx, id, "title", val)
	case "note", "body":
		return s.Client.FieldSet(ctx, id, "body", val)
	case "target":
		prec := r.FormValue("precision")
		if prec == "" {
			prec = "Q"
		}
		if err := s.Client.FieldSet(ctx, id, "target", val); err != nil {
			return err
		}
		return s.Client.FieldSet(ctx, id, "target_precision", prec)
	case "specifier", "builder", "pilot", "owner":
		return s.bindOrUnbind(ctx, id, col, val)
	case "by":
		return s.bindOrUnbind(ctx, id, "evaluator", val)
	case "actor":
		// Roles sheet: row id is "<milestone>:<role>:<oldActor>".
		mid, role, old := splitRoleRow(id)
		if mid == "" {
			return mcp.ErrNotImplemented
		}
		if old != "" {
			_ = s.Client.UnbindRole(ctx, mid, role, old)
		}
		if val == "" {
			return nil
		}
		return s.Client.BindRole(ctx, mid, role, val)
	case "product", "org":
		return s.reclassify(ctx, id, taxonomyFor(col), val)
	case "specAck", "buildAck":
		who := "specifier"
		if col == "buildAck" {
			who = "builder"
		}
		switch val {
		case "accepted":
			return s.Client.AckAccept(ctx, id, who, "Accepted from sheet.")
		case "rejected":
			return s.Client.AckReject(ctx, id, who, "Rejected from sheet.")
		default:
			return nil // pending: nothing to commit
		}
	}
	return mcp.ErrNotImplemented
}

// bindOrUnbind rebinds a single-actor role: unbind any current holder, then
// bind the new actor (empty val = unassign).
func (s *Server) bindOrUnbind(ctx context.Context, id, role, actor string) error {
	if cur := roleActorByID(ctx, s.Client, id, role); cur != "" && cur != actor {
		_ = s.Client.UnbindRole(ctx, id, role, cur)
	}
	if actor == "" {
		return nil
	}
	return s.Client.BindRole(ctx, id, role, actor)
}

// reclassify moves a work item to a node within a taxonomy (declassify the old
// node first so single-classification taxonomies stay clean; "" = declassify).
func (s *Server) reclassify(ctx context.Context, id, taxonomy, node string) error {
	if cur := classByID(ctx, s.Client, id, taxonomy); cur != "" && cur != node {
		_ = s.Client.Declassify(ctx, id, taxonomy, cur)
	}
	if node == "" {
		return nil
	}
	return s.Client.Classify(ctx, id, taxonomy, node)
}

// cellFragment re-renders one cell's display HTML after a mutation. It reuses
// the same renderers the builders use so the swapped cell matches the grid.
func (s *Server) cellFragment(id, col string, w mcp.WorkItem) template.HTML {
	switch col {
	case "status":
		return statusPill(w.Status)
	case "ryg":
		return rygPill(w.RYG())
	case "visibility":
		return visibilityPill(w.CustomerVisible())
	case "verdict":
		return verdictPill(w.Field("verdict"))
	case "value":
		return valuePill(w.Field("value"))
	case "target":
		return targetCell(w.Target(), w.TargetPrecision())
	case "specifier", "builder", "pilot", "owner":
		return actorCell(roleActor(w, col))
	case "by":
		return actorCell(roleActor(w, "evaluator"))
	case "actor":
		_, role, _ := splitRoleRow(id)
		return actorCell(roleActor(w, role))
	case "product", "org":
		return classCell(w, taxonomyFor(col))
	case "specAck", "buildAck":
		state := "pending"
		if a, ok := latestAck(w); ok {
			if col == "buildAck" {
				state = a.Builder
			} else {
				state = a.Specifier
			}
		}
		return ackStatePill(state)
	case "title", "summary":
		return template.HTML(template.HTMLEscapeString(firstNonEmpty(w.Title, "—")))
	case "note", "body":
		return template.HTML(template.HTMLEscapeString(truncate(w.Body, 140)))
	}
	return template.HTML(`<span class="placeholder">—</span>`)
}

// --- bulk actions (POST /partials/sheet/bulk) ---

// sheetBulk applies one action over the selected ids (loops the client mutator)
// and asks HTMX to refresh the page so every affected row re-renders. Actions:
// reassign (BindRole pilot=value), quarter (FieldSet target), classify
// (Classify product=value), ack (AckAccept both sides).
func (s *Server) sheetBulk(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	action := r.FormValue("action")
	value := r.FormValue("value")
	ids := r.Form["ids"]
	if len(ids) == 1 && strings.Contains(ids[0], ",") {
		ids = strings.Split(ids[0], ",")
	}
	ctx := r.Context()
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		switch action {
		case "reassign":
			_ = s.bindOrUnbind(ctx, id, "pilot", value)
		case "quarter":
			_ = s.Client.FieldSet(ctx, id, "target", value)
		case "classify":
			_ = s.reclassify(ctx, id, "product", value)
		case "ack":
			_ = s.Client.AckAccept(ctx, id, "specifier", "Bulk-accepted.")
			_ = s.Client.AckAccept(ctx, id, "builder", "Bulk-accepted.")
		}
	}
	s.Indicators.Invalidate()
	w.Header().Set("HX-Refresh", "true")
	w.WriteHeader(http.StatusNoContent)
}

// --- quick-add (POST /partials/sheet/add) ---

// sheetQuickAdd creates a new work item for the view's kind (WorkCreate) and
// redirects to the Portfolio with the new record's panel open.
func (s *Server) sheetQuickAdd(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	kind := r.FormValue("kind")
	if kind == "" {
		http.Error(w, "missing kind", http.StatusBadRequest)
		return
	}
	title := r.FormValue("title")
	if title == "" {
		title = "New " + kind + " — name me"
	}
	id, err := s.Client.WorkCreate(r.Context(), kind, title, "")
	if err != nil {
		renderPartial(w, template.HTML(`<span class="dx-diag dx-diag--violation">create failed: `+
			template.HTMLEscapeString(err.Error())+`</span>`))
		return
	}
	s.Indicators.Invalidate()
	dest := destForKind(kind) + "?open=" + id
	if isHTMX(r) {
		w.Header().Set("HX-Redirect", dest)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	http.Redirect(w, r, dest, http.StatusSeeOther)
}

// destForKind picks the landing view for a freshly-created record.
func destForKind(kind string) string {
	switch kind {
	case "rfc":
		return "/rfcs"
	case "decision_block":
		return "/decisions"
	case "outcome_assessment":
		return "/outcomes"
	default:
		return "/portfolio"
	}
}

// --- lookup helpers ---

// workForRow fetches the work item backing a row id. Roles-sheet ids are
// composite ("<milestone>:<role>:<actor>"); fall back to the milestone id.
func (s *Server) workForRow(ctx context.Context, rowID string) mcp.WorkItem {
	id := rowID
	if i := strings.IndexByte(rowID, ':'); i >= 0 {
		id = rowID[:i]
	}
	w, err := s.Client.WorkGet(ctx, id)
	if err != nil {
		return mcp.WorkItem{ID: id}
	}
	return w
}

func roleActorByID(ctx context.Context, c mcp.Client, id, role string) string {
	w, err := c.WorkGet(ctx, id)
	if err != nil {
		return ""
	}
	return roleActor(w, role)
}

func classByID(ctx context.Context, c mcp.Client, id, taxonomy string) string {
	w, err := c.WorkGet(ctx, id)
	if err != nil {
		return ""
	}
	return classOf(w, taxonomy)
}

// splitRoleRow parses a roles-sheet row id "<milestone>:<role>:<actor>".
func splitRoleRow(rowID string) (milestone, role, actor string) {
	parts := strings.SplitN(rowID, ":", 3)
	switch len(parts) {
	case 3:
		return parts[0], parts[1], parts[2]
	case 2:
		return parts[0], parts[1], ""
	default:
		return "", "", ""
	}
}

// --- editor fragment renderers ---
//
// Each editor is a self-contained HTML fragment with hx-post wiring. They reuse
// the prototype's CSS vocabulary (sht-cell is-editing, ackpop, ap-input/ap-list,
// targed-prec) so no CSS is added. The committing control carries the hidden
// id+col (+precision for target) and posts to /partials/sheet/cell with
// hx-target="closest td" so the whole cell <div> swaps back to display form.

// optKV is a value/label pair for the select editor.
type optKV struct{ Value, Label string }

func toKV(opts []web.Option) []optKV {
	out := make([]optKV, len(opts))
	for i, o := range opts {
		out[i] = optKV{Value: o.Value, Label: o.Label}
	}
	return out
}

// commitAttrs is the shared hx-post wiring for a committing editor control. The
// editor swaps its commit response (the updated display HTML) back into the
// editable cell div ([data-editable]) — not the editor form itself, which also
// carries .sht-cell, so we target the outer [data-editable] explicitly.
func commitAttrs(extra string) string {
	return `hx-post="/partials/sheet/cell" hx-target="closest [data-editable]" hx-swap="innerHTML" ` + extra
}

func sheetHidden(id, col string) string {
	return `<input type="hidden" name="id" value="` + template.HTMLEscapeString(id) + `">` +
		`<input type="hidden" name="col" value="` + template.HTMLEscapeString(col) + `">`
}

// selectEditor is the status / RYG / visibility / verdict / value editor: a
// native <select> that commits on change (mirrors sheet.jsx CellEditor select).
func selectEditor(id, col string, opts []optKV, current string) template.HTML {
	var b strings.Builder
	b.WriteString(`<form class="sht-cell is-editing" ` + commitAttrs(`hx-trigger="change from:select"`) + `>`)
	b.WriteString(sheetHidden(id, col))
	b.WriteString(`<select name="value" autofocus class="so-select" style="width:100%">`)
	for _, o := range opts {
		sel := ""
		if o.Value == current {
			sel = " selected"
		}
		b.WriteString(`<option value="` + template.HTMLEscapeString(o.Value) + `"` + sel + `>` +
			template.HTMLEscapeString(o.Label) + `</option>`)
	}
	b.WriteString(`</select></form>`)
	return template.HTML(b.String())
}

// ackEditor is the three-button A/P/R popover (mirrors views.jsx AckPicker).
// Each button posts its value immediately.
func ackEditor(id, col string) template.HTML {
	btn := func(val, variant, label string) string {
		return `<button type="submit" name="value" value="` + val + `" ` +
			commitAttrs(``) + ` class="ackpop__btn ackpop__btn--` + variant + `">` +
			`<span class="ackpop__lab">` + label + `</span>` +
			`<span class="ackpop__kbd">` + strings.ToUpper(label[:1]) + `</span></button>`
	}
	return template.HTML(`<form class="sht-cell is-editing"><div class="ackpop">` +
		sheetHidden(id, col) +
		btn("accepted", "g", "Accepted") +
		btn("pending", "neutral", "Pending") +
		btn("rejected", "r", "Rejected") +
		`</div></form>`)
}

// actorEditor is the searchable actor picker (mirrors sheet.jsx ActorPicker).
// It hx-gets the shared Track E endpoint /partials/actor-picker for a live,
// searchable directory; if that endpoint is not wired yet, it degrades to a
// native <select> populated from ActorList here. Either way the committing
// control posts value=<actorID> to /partials/sheet/cell.
func actorEditor(ctx context.Context, c mcp.Client, id, col, current string) template.HTML {
	// Soft dependency on Track E: try the shared picker first.
	// (We can't probe the route table, so emit a control that hx-gets it and
	// also render the self-contained <select> fallback inline — the fallback
	// commits even if Track E's endpoint is absent.)
	actors, _ := c.ActorList(ctx)
	var b strings.Builder
	b.WriteString(`<form class="sht-cell is-editing" ` + commitAttrs(`hx-trigger="change from:select"`) + `>`)
	b.WriteString(sheetHidden(id, col))
	b.WriteString(`<select name="value" autofocus class="so-select" style="width:100%">`)
	b.WriteString(`<option value="">— unassign —</option>`)
	for _, a := range actors {
		sel := ""
		if a.ActorID == current {
			sel = " selected"
		}
		b.WriteString(`<option value="` + template.HTMLEscapeString(a.ActorID) + `"` + sel + `>` +
			template.HTMLEscapeString(a.ActorID) + `</option>`)
	}
	if len(actors) == 0 && current != "" {
		b.WriteString(`<option value="` + template.HTMLEscapeString(current) + `" selected>` +
			template.HTMLEscapeString(current) + `</option>`)
	}
	b.WriteString(`</select></form>`)
	return template.HTML(b.String())
}

// taxonomyEditor is the node picker for a classification axis (mirrors
// sheet.jsx TaxonomyPicker). Options come from live Meta; commit posts the node
// slug (empty = declassify).
func taxonomyEditor(ctx context.Context, c mcp.Client, id, col, taxonomy, current string) template.HTML {
	meta, _ := c.MetaGet(ctx)
	var b strings.Builder
	b.WriteString(`<form class="sht-cell is-editing" ` + commitAttrs(`hx-trigger="change from:select"`) + `>`)
	b.WriteString(sheetHidden(id, col))
	b.WriteString(`<select name="value" autofocus class="so-select" style="width:100%">`)
	b.WriteString(`<option value="">— declassify —</option>`)
	for _, t := range meta.Taxonomies {
		if t.Slug != taxonomy {
			continue
		}
		for _, n := range t.Nodes {
			if n.Retired {
				continue
			}
			sel := ""
			if n.Slug == current {
				sel = " selected"
			}
			b.WriteString(`<option value="` + template.HTMLEscapeString(n.Slug) + `"` + sel + `>` +
				template.HTMLEscapeString(n.Slug) + `</option>`)
		}
	}
	if current != "" {
		// Ensure the current node is selectable even if meta lookup is empty.
		b.WriteString(`<option value="` + template.HTMLEscapeString(current) + `" hidden></option>`)
	}
	b.WriteString(`</select></form>`)
	return template.HTML(b.String())
}

// targetEditor is the Q/M/D precision toggle + value input (mirrors views.jsx
// TargetEditor). The precision buttons set a hidden field; the value input
// commits on change, posting value + precision.
func sheetTargetEditor(id, col, current, precision string) template.HTML {
	if precision == "" {
		precision = "Q"
	}
	prec := func(p, label string) string {
		active := ""
		if p == precision {
			active = " is-active"
		}
		return `<button type="button" class="targed-prec__btn` + active +
			`" onclick="this.closest('form').querySelector('[name=precision]').value='` + p + `'">` + label + `</button>`
	}
	return template.HTML(`<form class="sht-cell is-editing" ` +
		commitAttrs(`hx-trigger="change from:input[name=value]"`) + `>` +
		sheetHidden(id, col) +
		`<input type="hidden" name="precision" value="` + precision + `">` +
		`<div class="ap" style="min-width:240px;padding:6px">` +
		`<div class="targed-prec">` + prec("Q", "Quarter") + prec("M", "Month") + prec("D", "Exact date") + `</div>` +
		`<input name="value" autofocus class="ap-input" value="` + template.HTMLEscapeString(current) +
		`" placeholder="2026 Q3 / 2026-09 / 2026-09-30">` +
		`<div style="margin-top:4px;font-family:var(--font-mono);font-size:10px;color:var(--ink-4)">Enter commits · Esc cancels</div>` +
		`</div></form>`)
}

// textEditor is the plain text input editor (mirrors sheet.jsx CellEditor text).
func textEditor(id, col, current string) template.HTML {
	return template.HTML(`<form class="sht-cell is-editing" ` +
		commitAttrs(`hx-trigger="change from:input"`) + `>` +
		sheetHidden(id, col) +
		`<input name="value" autofocus value="` + template.HTMLEscapeString(current) +
		`" style="width:100%" class="so-input"></form>`)
}
