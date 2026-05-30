// The Taxonomies view (§9.7, TaxonomyView) with node CRUD and, for the Goals
// axis, the per-node result column. Three tabs (Organization / Product / Goals)
// select which taxonomy renders via ?tax=; each row shows name/slug, parent,
// a "used by N milestones" count (org/product) or result note (goals), and a
// state cell (active/retired, or the goals result pill). An add-node form and
// per-node retire/move actions post to /partials/taxonomy/...; the Goals result
// is edited inline and persisted via TaxonomyNodeSet.
package handlers

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strings"

	"github.com/lenulus/pf/internal/pilot/mcp"
)

// goalsResults are the result values a goals node can carry.
var goalsResults = []struct{ value, label string }{
	{"achieved", "Achieved"},
	{"partial", "Partial"},
	{"missed", "Missed"},
	{"in_progress", "In progress"},
	{"aborted", "Aborted"},
}

// buildTaxonomyBody renders the active taxonomy tab. `tax` selects which axis
// (org|product|goals, default org); milestones power the per-node usage counts.
func buildTaxonomyBody(meta mcp.Meta, tax string, milestones []mcp.WorkItem) template.HTML {
	if len(meta.Taxonomies) == 0 {
		return template.HTML(`<div class="empty-line">No taxonomies in meta yet. Run <code>pilot init</code> to apply the RE bundle, then add nodes.</div>`)
	}
	if tax == "" {
		tax = "org"
	}

	var active *mcp.Taxonomy
	for i := range meta.Taxonomies {
		if meta.Taxonomies[i].Slug == tax {
			active = &meta.Taxonomies[i]
			break
		}
	}
	if active == nil {
		active = &meta.Taxonomies[0]
		tax = active.Slug
	}
	isGoals := tax == "goals"

	var b strings.Builder

	// --- tab toolbar ---
	b.WriteString(`<div class="sht-toolbar">`)
	for _, t := range meta.Taxonomies {
		cls := "rx-btn rx-btn--ghost rx-btn--sm"
		if t.Slug == tax {
			cls = "rx-btn rx-btn--accent rx-btn--sm"
		}
		fmt.Fprintf(&b, `<a class="%s" href="/taxonomies?tax=%s">%s</a>`,
			cls, template.HTMLEscapeString(t.Slug), template.HTMLEscapeString(t.Name))
	}
	b.WriteString(`<span class="sep">·</span><span class="label-tag">Levels</span>`)
	fmt.Fprintf(&b, `<span style="font-family:var(--font-mono);font-size:11px;color:var(--ink-3)">%s</span>`,
		template.HTMLEscapeString(strings.Join(active.Levels, " → ")))
	b.WriteString(`</div>`)

	// --- add-node form ---
	b.WriteString(string(taxonomyAddForm(tax, active.Nodes)))

	// --- node table ---
	b.WriteString(`<div style="background:var(--surface);border:1px solid var(--hairline);border-radius:5px;box-shadow:var(--shadow-1)">`)
	cols := "1fr 200px 160px 200px"
	headThird, headFourth := "Used by", "State"
	if isGoals {
		cols = "1fr 200px 160px 240px"
		headThird, headFourth = "Result note", "Result"
	}
	fmt.Fprintf(&b, `<div style="display:grid;grid-template-columns:%s;padding:0 14px;background:var(--paper-2);border-bottom:1px solid var(--hairline-strong);height:32px;align-items:center;font-family:var(--font-mono);font-size:9.5px;color:var(--ink-3);text-transform:uppercase;letter-spacing:0.1em"><span>Name / slug</span><span>Parent</span><span>%s</span><span style="text-align:right">%s</span></div>`,
		cols, headThird, headFourth)

	if len(active.Nodes) == 0 {
		b.WriteString(`<div class="empty-line">empty skeleton — add the first node above</div>`)
	}
	for _, n := range active.Nodes {
		b.WriteString(string(renderTaxNode(tax, n, isGoals, milestones, cols)))
	}
	b.WriteString(`</div>`)
	return template.HTML(b.String())
}

// renderTaxNode renders one taxonomy node row.
func renderTaxNode(tax string, n mcp.TaxonomyNode, isGoals bool, milestones []mcp.WorkItem, cols string) template.HTML {
	depth := nodeDepth(n.Slug)
	parent := n.ParentSlug
	if parent == "" {
		parent = "—"
	}

	// third column: used-by count (org/product) or result note (goals).
	var third string
	if isGoals {
		note := nodeResultNote(n)
		if note == "" {
			if depth < 2 {
				note = "—"
			} else {
				note = "unmeasured"
			}
		}
		third = fmt.Sprintf(`<span style="font-family:var(--font-mono);font-size:11px;color:var(--ink-4)">%s</span>`,
			template.HTMLEscapeString(note))
	} else {
		used := usedByCount(tax, n.Slug, milestones)
		color, label := "var(--ink-4)", "unused"
		if used > 0 {
			color = "var(--accent)"
			label = fmt.Sprintf("%d milestone%s", used, plur(used))
		}
		third = fmt.Sprintf(`<span style="font-family:var(--font-mono);font-size:11px;color:%s">%s</span>`, color, label)
	}

	// fourth column: goals result editor, or active/retired state + actions.
	var fourth string
	if isGoals {
		fourth = string(goalsResultCell(n))
	} else {
		stateChip := `<span style="font-family:var(--font-mono);font-size:10px;color:var(--ryg-green)">active</span>`
		if n.Retired {
			stateChip = string(pill("neutral", "retired"))
		}
		actions := ""
		if !n.Retired {
			actions = fmt.Sprintf(`<button class="rx-btn rx-btn--ghost rx-btn--sm" hx-post="/partials/taxonomy/retire/%s/%s" hx-target="closest .tx-node" hx-swap="outerHTML" style="margin-left:8px">Retire</button>`,
				template.HTMLEscapeString(tax), template.HTMLEscapeString(n.Slug))
		}
		fourth = fmt.Sprintf(`<span style="display:inline-flex;align-items:center;justify-content:flex-end">%s%s</span>`, stateChip, actions)
	}

	return template.HTML(fmt.Sprintf(
		`<div class="tx-node" style="display:grid;grid-template-columns:%s;align-items:center"><span style="display:flex;align-items:center;min-width:0"><span class="indent" style="width:%dpx"></span><span style="display:flex;flex-direction:column;min-width:0"><span class="name">%s</span><span class="slug">%s</span></span></span><span style="font-family:var(--font-mono);font-size:10.5px;color:var(--ink-3)">%s</span><span>%s</span><span style="text-align:right">%s</span></div>`,
		cols, 8+depth*18,
		template.HTMLEscapeString(n.Name), template.HTMLEscapeString(n.Slug),
		template.HTMLEscapeString(parent), third, fourth))
}

// goalsResultCell renders the inline goals result editor: a result <select> +
// note input that posts to TaxonomyNodeSet, plus the current result pill.
func goalsResultCell(n mcp.TaxonomyNode) template.HTML {
	cur := nodeResult(n)
	var opts strings.Builder
	opts.WriteString(`<option value="">—</option>`)
	for _, r := range goalsResults {
		sel := ""
		if r.value == cur {
			sel = " selected"
		}
		fmt.Fprintf(&opts, `<option value="%s"%s>%s</option>`,
			template.HTMLEscapeString(r.value), sel, template.HTMLEscapeString(r.label))
	}
	pillHTML := ""
	if cur != "" {
		pillHTML = string(resultPill(cur))
	}
	return template.HTML(fmt.Sprintf(
		`<form hx-post="/partials/taxonomy/result/goals/%s" hx-target="closest .tx-node" hx-swap="outerHTML" style="display:flex;align-items:center;gap:6px;justify-content:flex-end">%s<select name="result" class="so-select" style="width:120px;padding:1px 5px;font-size:10.5px">%s</select><input name="note" class="so-input" value="%s" placeholder="note" style="width:90px;padding:1px 5px;font-size:10.5px"><button type="submit" class="rx-btn rx-btn--ghost rx-btn--sm">Set</button></form>`,
		template.HTMLEscapeString(n.Slug), pillHTML, opts.String(),
		template.HTMLEscapeString(nodeResultNote(n))))
}

// taxonomyAddForm renders the add-node form with a parent <select>.
func taxonomyAddForm(tax string, nodes []mcp.TaxonomyNode) template.HTML {
	var parentOpts strings.Builder
	parentOpts.WriteString(`<option value="">— (top level)</option>`)
	for _, n := range nodes {
		if n.Retired {
			continue
		}
		fmt.Fprintf(&parentOpts, `<option value="%s">%s</option>`,
			template.HTMLEscapeString(n.Slug), template.HTMLEscapeString(n.Slug))
	}
	return template.HTML(fmt.Sprintf(`
<form hx-post="/partials/taxonomy/add/%s" hx-target="body" hx-swap="none" hx-on::after-request="if(event.detail.successful)window.location.reload()" style="display:flex;gap:8px;align-items:center;margin-bottom:12px">
  <input name="slug" class="so-input" placeholder="slug (e.g. platform/api)" style="width:200px">
  <input name="name" class="so-input" placeholder="Display name" style="width:180px">
  <select name="parent_slug" class="so-select" style="width:200px">%s</select>
  <button type="submit" class="rx-btn rx-btn--accent rx-btn--sm">Add node</button>
</form>`, template.HTMLEscapeString(tax), parentOpts.String()))
}

// usedByCount counts milestones classified into a node of the given taxonomy
// whose node slug equals the node slug.
func usedByCount(tax, slug string, milestones []mcp.WorkItem) int {
	n := 0
	for _, m := range milestones {
		for _, c := range m.Classifications {
			if c.TaxonomySlug == tax && c.NodeSlug == slug {
				n++
			}
		}
	}
	return n
}

// nodeResultNote reads a goals node's `resultNote` from its Metadata blob.
func nodeResultNote(n mcp.TaxonomyNode) string {
	if len(n.Metadata) == 0 {
		return ""
	}
	var md struct {
		ResultNote string `json:"resultNote"`
	}
	_ = json.Unmarshal(n.Metadata, &md)
	return md.ResultNote
}

// --- partial endpoints ---

// registerTaxonomyAdmin wires the node-CRUD + goals-result partial routes.
func (s *Server) registerTaxonomyAdmin(mux *http.ServeMux) {
	mux.HandleFunc("POST /partials/taxonomy/add/{tax}", s.taxAdd)
	mux.HandleFunc("POST /partials/taxonomy/retire/{tax}/{slug...}", s.taxRetire)
	mux.HandleFunc("POST /partials/taxonomy/result/{tax}/{slug...}", s.taxResult)
}

func (s *Server) taxAdd(w http.ResponseWriter, r *http.Request) {
	tax := r.PathValue("tax")
	slug := strings.TrimSpace(r.FormValue("slug"))
	name := strings.TrimSpace(r.FormValue("name"))
	if slug == "" || name == "" {
		http.Error(w, "slug and name are required", http.StatusBadRequest)
		return
	}
	if err := s.Client.TaxonomyNodeAdd(r.Context(), tax, slug, name, r.FormValue("parent_slug")); err != nil {
		fail(w, err)
		return
	}
	// The add form reloads the page on success (hx-on::after-request); return
	// 200 with no body so the swap is a no-op.
	w.WriteHeader(http.StatusOK)
}

func (s *Server) taxRetire(w http.ResponseWriter, r *http.Request) {
	tax, slug := r.PathValue("tax"), r.PathValue("slug")
	if err := s.Client.TaxonomyNodeRetire(r.Context(), tax, slug); err != nil {
		fail(w, err)
		return
	}
	// Re-render the row in its retired state. We don't have the full node here,
	// so render a minimal retired row keyed by slug.
	renderPartial(w, taxRetiredRow(tax, slug))
}

func (s *Server) taxResult(w http.ResponseWriter, r *http.Request) {
	tax, slug := r.PathValue("tax"), r.PathValue("slug")
	result := r.FormValue("result")
	note := r.FormValue("note")
	md, _ := json.Marshal(struct {
		Result     string `json:"result"`
		ResultNote string `json:"resultNote"`
	}{Result: result, ResultNote: note})
	if err := s.Client.TaxonomyNodeSet(r.Context(), tax, slug, md); err != nil {
		fail(w, err)
		return
	}
	// Re-render the goals node row with the new result/note.
	n := mcp.TaxonomyNode{Slug: slug, Name: lastSegment(slug), Metadata: md}
	renderPartial(w, renderTaxNode(tax, n, true, nil, "1fr 200px 160px 240px"))
}

// taxRetiredRow renders the minimal retired-state replacement for a node row.
func taxRetiredRow(tax, slug string) template.HTML {
	n := mcp.TaxonomyNode{Slug: slug, Name: lastSegment(slug), Retired: true}
	return renderTaxNode(tax, n, false, nil, "1fr 200px 160px 200px")
}

// lastSegment returns the final path segment of a slug.
func lastSegment(slug string) string {
	if i := strings.LastIndex(slug, "/"); i >= 0 {
		return slug[i+1:]
	}
	return slug
}
