// Track E identity & cross-cutting HTMX endpoints: the searchable ActorPicker
// popover (shared by Tracks B and C for editing actor/role cells) and the ⌘K
// command-palette record search + create endpoints. All live under
// /partials/... and follow the partials.go convention: one MCP read or write,
// then an HTML (or JSON) fragment written with renderPartial — no layout chrome.
//
// Coordinator wiring (handlers.go, Server):
//   - Add `Actors *web.ActorDirectory` to Server and init it in New(client):
//     return &Server{Client: client, Indicators: ..., Actors: web.NewActorDirectory(client)}
//   - Call s.registerIdentityPartials(mux) from registerPartials.
//
// Cross-track contracts (Tracks B & C target these):
//
//	GET  /partials/actor-picker?id=<workitem>&role=<roleSlug>[&col=<cellId>]
//	     → the ActorPicker popover (search input + avatar options + Unassign).
//	     The options POST to /partials/actor-pick to commit a BindRole.
//	POST /partials/actor-pick   form: id, role, actor (empty actor ⇒ unbind),
//	     [col], [bound] (current actor to unbind on a clear/reassign)
//	     → the updated avatar cell fragment (the same markup avatarHTML/actorCell
//	       produce), so the caller swaps it straight back into the cell.
//	GET  /partials/palette/search?q=<text>  → JSON {groups:[{label,items:[...]}]}
//	     for palette.js to merge with its NAVIGATE list.
//	POST /partials/palette/create  form: kind (milestone|rfc|decision), title
//	     → HX-Redirect header to the new record's panel (no body).
package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strings"

	"github.com/lenulus/pf/internal/pilot/mcp"
	"github.com/lenulus/pf/internal/pilot/web"
)

// pickerLimit caps how many actor options the picker shows at once; the search
// input narrows the set, so a short list stays readable.
const pickerLimit = 8

// registerIdentityPartials wires Track E's /partials endpoints onto mux. The
// coordinator calls this from registerPartials in partials.go.
func (s *Server) registerIdentityPartials(mux *http.ServeMux) {
	mux.HandleFunc("GET /partials/actor-picker", s.actorPicker)
	mux.HandleFunc("POST /partials/actor-pick", s.actorPick)
	mux.HandleFunc("GET /partials/palette/search", s.paletteSearch)
	mux.HandleFunc("POST /partials/palette/create", s.paletteCreate)
}

// actorDir returns the actor directory, tolerating a nil field (e.g. a Server
// built before the coordinator wires Actors) by falling back to a fresh
// directory over the same client. Keeps the endpoint resilient during the
// concurrent integration window.
func (s *Server) actorDir() *web.ActorDirectory {
	if s.Actors != nil {
		return s.Actors
	}
	return web.NewActorDirectory(s.Client)
}

// actorPicker serves GET /partials/actor-picker — the searchable popover. It is
// generic: it takes the work-item id and the role slug to bind on select (plus
// an optional opaque col token the caller round-trips so it can re-target its
// own cell). The options and the live-search input both POST to actorPick.
func (s *Server) actorPicker(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	id, role, col := q.Get("id"), q.Get("role"), q.Get("col")
	bound := q.Get("bound") // current actor, so we can highlight + offer Unassign
	search := q.Get("q")

	actors := s.actorDir().Search(r.Context(), search, pickerLimit)
	renderPartial(w, actorPickerHTML(id, role, col, bound, search, actors))
}

// actorPick serves POST /partials/actor-pick — commits the selection (BindRole,
// or UnbindRole when actor is empty) and returns the updated avatar cell so the
// caller swaps it back into place. A reassignment unbinds the prior actor first
// when the role is single-valued; callers pass the prior actor as `bound`.
func (s *Server) actorPick(w http.ResponseWriter, r *http.Request) {
	id := r.FormValue("id")
	role := r.FormValue("role")
	actor := strings.TrimSpace(r.FormValue("actor"))
	bound := strings.TrimSpace(r.FormValue("bound"))

	if id == "" || role == "" {
		http.Error(w, "actor-pick: id and role required", http.StatusBadRequest)
		return
	}

	if actor == "" {
		// Clear: unbind the currently bound actor, if any.
		if bound != "" {
			if err := s.Client.UnbindRole(r.Context(), id, role, bound); err != nil {
				fail(w, err)
				return
			}
		}
	} else {
		// Reassign: unbind the prior actor before binding the new one so a
		// single-valued role doesn't end up with two bindings.
		if bound != "" && bound != actor {
			// A failed unbind is non-fatal — the bind below is the intent; log
			// shape mirrors the substrate-never-silently-drops contract by
			// surfacing the bind error instead.
			_ = s.Client.UnbindRole(r.Context(), id, role, bound)
		}
		if err := s.Client.BindRole(r.Context(), id, role, actor); err != nil {
			fail(w, err)
			return
		}
	}
	s.Indicators.Invalidate()
	renderPartial(w, actorCellSwap(actor))
}

// paletteSearch serves GET /partials/palette/search — record jump-in for ⌘K. It
// queries milestones/RFCs/decisions over MCP, filters by q (id or title), and
// returns grouped JSON the palette merges below its static NAVIGATE list.
func (s *Server) paletteSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	res := paletteResult{Groups: []paletteGroup{}}

	add := func(label, kind, view, icon string) {
		items := s.paletteRecords(r.Context(), kind, view, icon, q, 6)
		if len(items) > 0 {
			res.Groups = append(res.Groups, paletteGroup{Label: label, Items: items})
		}
	}
	add("MILESTONES", "milestone", "portfolio", "panel")
	add("RFCS", "rfc", "rfcs", "flag")
	add("DECISIONS", "decision_block", "decisions", "scale")

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(res)
}

// paletteRecords lists one kind's work items and maps the ones matching q into
// palette rows that navigate to the record's panel (e.g. /portfolio?open=ID).
func (s *Server) paletteRecords(ctx context.Context, kind, view, icon, q string, limit int) []paletteItem {
	items, err := s.Client.WorkList(ctx, mcp.Filters{Kind: kind})
	if err != nil {
		return nil
	}
	ql := strings.ToLower(q)
	var out []paletteItem
	for _, it := range items {
		id := rowID(it)
		if q != "" && !strings.Contains(strings.ToLower(it.Title), ql) && !strings.Contains(strings.ToLower(id), ql) {
			continue
		}
		title := it.Title
		if title == "" {
			title = id
		}
		out = append(out, paletteItem{
			Label: title,
			Sub:   id,
			Icon:  icon,
			Href:  fmt.Sprintf("/%s?open=%s", view, id),
		})
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

// paletteCreate serves POST /partials/palette/create — the ⌘K Create actions.
// It maps the palette's record word to a substrate kind, calls WorkCreate, and
// returns an HX-Redirect to the new record's panel.
func (s *Server) paletteCreate(w http.ResponseWriter, r *http.Request) {
	what := r.FormValue("kind")
	title := strings.TrimSpace(r.FormValue("title"))
	if title == "" {
		title = "Untitled " + what
	}
	kind, view := paletteKind(what)
	if kind == "" {
		http.Error(w, "palette-create: unknown kind", http.StatusBadRequest)
		return
	}
	newID, err := s.Client.WorkCreate(r.Context(), kind, title, "")
	if err != nil {
		fail(w, err)
		return
	}
	s.Indicators.Invalidate()
	w.Header().Set("HX-Redirect", fmt.Sprintf("/%s?open=%s", view, newID))
	w.WriteHeader(http.StatusOK)
}

// paletteKind maps a palette create word to its (substrate kind, landing view).
func paletteKind(what string) (kind, view string) {
	switch what {
	case "milestone":
		return "milestone", "portfolio"
	case "rfc":
		return "rfc", "rfcs"
	case "decision":
		return "decision_block", "decisions"
	default:
		return "", ""
	}
}

// --- JSON shapes for palette/search ---

type paletteResult struct {
	Groups []paletteGroup `json:"groups"`
}

type paletteGroup struct {
	Label string        `json:"label"`
	Items []paletteItem `json:"items"`
}

type paletteItem struct {
	Label string `json:"label"`
	Sub   string `json:"sub"`
	Icon  string `json:"icon"`
	Href  string `json:"href"`
}

// --- picker markup ---

// actorPickerHTML renders the ActorPicker popover (mirrors the prototype's
// sheet.jsx ActorPicker: ap / ap-input / ap-list / ap-opt / ap-clear). The
// search input live-filters via hx-get back to this same endpoint; selecting an
// option POSTs to actor-pick. id/role/col/bound ride hidden inputs on each
// option's form so the commit knows what to write and what to swap.
func actorPickerHTML(id, role, col, bound, search string, actors []mcp.ActorRecord) template.HTML {
	esc := template.HTMLEscapeString
	var b strings.Builder
	b.WriteString(`<div class="ap" id="actor-picker">`)

	// Search input: live-filters the list in place (hx-get re-renders the whole
	// popover, narrowing ap-list).
	b.WriteString(`<input class="ap-input" type="text" name="q" placeholder="Search actor…" autofocus`)
	b.WriteString(` value="` + esc(search) + `"`)
	b.WriteString(` hx-get="/partials/actor-picker"`)
	b.WriteString(` hx-trigger="input changed delay:120ms, search"`)
	b.WriteString(` hx-target="#actor-picker" hx-swap="outerHTML"`)
	b.WriteString(` hx-include="this"`)
	// Round-trip id/role/col/bound on the search request via hidden inputs.
	b.WriteString(`>`)
	for k, v := range map[string]string{"id": id, "role": role, "col": col, "bound": bound} {
		if v != "" {
			b.WriteString(`<input type="hidden" name="` + esc(k) + `" value="` + esc(v) + `">`)
		}
	}

	b.WriteString(`<div class="ap-list">`)
	for _, a := range actors {
		b.WriteString(actorOptionForm(id, role, col, bound, a.ActorID, a.ActorID == bound))
	}
	if bound != "" {
		// Unassign clears the current binding.
		b.WriteString(actorClearForm(id, role, col, bound))
	}
	if len(actors) == 0 {
		b.WriteString(`<div class="ap-opt"><span class="ap-name placeholder">no actors</span></div>`)
	}
	b.WriteString(`</div></div>`)
	return template.HTML(b.String())
}

// actorOptionForm renders one selectable actor row as a form that POSTs the
// bind to actor-pick and swaps the avatar cell back via the caller's col token.
func actorOptionForm(id, role, col, bound, actor string, active bool) string {
	esc := template.HTMLEscapeString
	cls := "ap-opt"
	if active {
		cls += " is-active"
	}
	// hx-target: if the caller passed a col cell id we swap that; otherwise we
	// swap the closest cell (the editor host) via the response's outerHTML.
	target := `closest .sht-cell`
	if col != "" {
		target = "#" + col
	}
	return `<form class="` + cls + `" style="cursor:pointer"` +
		` hx-post="/partials/actor-pick"` +
		` hx-target="` + esc(target) + `" hx-swap="outerHTML">` +
		`<input type="hidden" name="id" value="` + esc(id) + `">` +
		`<input type="hidden" name="role" value="` + esc(role) + `">` +
		hidden("col", col) + hidden("bound", bound) +
		`<input type="hidden" name="actor" value="` + esc(actor) + `">` +
		`<span class="ap-av" style="background:` + actorColor(actor) + `">` + esc(actorInitials(actor)) + `</span>` +
		`<span class="ap-name">` + esc(actor) + `</span>` +
		`<button type="submit" style="display:none"></button>` +
		`</form>`
}

// actorClearForm renders the "Unassign" row (POSTs an empty actor ⇒ unbind).
func actorClearForm(id, role, col, bound string) string {
	esc := template.HTMLEscapeString
	target := `closest .sht-cell`
	if col != "" {
		target = "#" + col
	}
	return `<form class="ap-clear"` +
		` hx-post="/partials/actor-pick"` +
		` hx-target="` + esc(target) + `" hx-swap="outerHTML" style="cursor:pointer">` +
		`<input type="hidden" name="id" value="` + esc(id) + `">` +
		`<input type="hidden" name="role" value="` + esc(role) + `">` +
		hidden("col", col) + hidden("bound", bound) +
		`<input type="hidden" name="actor" value="">` +
		`<button type="submit" style="all:unset;cursor:pointer;width:100%;text-align:left">Unassign</button>` +
		`</form>`
}

// hidden emits a hidden input only when v is non-empty (keeps the markup tidy).
func hidden(name, v string) string {
	if v == "" {
		return ""
	}
	return `<input type="hidden" name="` + template.HTMLEscapeString(name) +
		`" value="` + template.HTMLEscapeString(v) + `">`
}

// actorCellSwap is the fragment returned after a successful bind/unbind: the
// updated actor cell wrapped in a .sht-cell so it swaps cleanly back into the
// sheet (matching the editor host the caller replaced). Reuses actorCell so the
// avatar+name markup stays identical to the initial render.
func actorCellSwap(actor string) template.HTML {
	return template.HTML(`<div class="sht-cell">` + string(actorCell(actor)) + `</div>`)
}
