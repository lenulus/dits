// HTMX partial-swap foundation. Inline cell edits, panel tab/row swaps, and
// in-place Attention actions are HTMX requests that target a /partials/...
// endpoint and swap a fragment of HTML back in — no full-page reload.
//
// Convention (all tracks follow this):
//   - Read-modifying controls carry hx-* attributes and hit /partials/...
//     (GET for editors/fragments, POST for mutations).
//   - Handlers perform one MCP mutation, then write the replacement fragment
//     with renderPartial (HTML only, no layout chrome).
//   - Each track owns its partial routes in its own file via a
//     register<Track>Partials(mux) function. The coordinator calls each from
//     registerPartials below, so the route table stays the single shared
//     integration point and tracks never edit each other's files.
package handlers

import (
	"html/template"
	"net/http"
)

// registerPartials wires the /partials/... HTMX endpoint family. The
// coordinator adds each track's register<Track>Partials(mux) call here as the
// track lands, keeping handlers the only shared integration file.
func (s *Server) registerPartials(mux *http.ServeMux) {
	s.registerSheetEdit(mux)         // Track B — inline cell editors + commit/bulk/add swaps
	s.registerPanelPartials(mux)     // Track C — panel tab bodies + ack-history
	s.registerIdentityPartials(mux)  // Track E — actor picker + ⌘K search/create
	s.registerAttentionPartials(mux) // Track D — in-place Attention actions
	s.registerLogPartials(mux)       // Track D — Pilot's Log quick-entry
	s.registerTaxonomyAdmin(mux)     // Track D — taxonomy node CRUD
}

// renderPartial writes an HTML fragment as the full response body (no layout).
// HTMX swaps it into the hx-target.
func renderPartial(w http.ResponseWriter, html template.HTML) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(html))
}

// isHTMX reports whether the request came from HTMX (so a handler can return a
// fragment for HTMX and a full redirect for a plain form post).
func isHTMX(r *http.Request) bool { return r.Header.Get("HX-Request") == "true" }
