// Package web holds Pilot's server-rendered UI: the embedded templates and
// static assets, the template render helper, and the view-model types the
// handlers populate. See implementation-plan-v2 §9 (the prototype port).
//
// Phase-5 foundation: this lands the chrome (layout shell, sheet primitive,
// slide-over panel, ⌘K palette) plus the generic sheet view-model, so the
// twelve route handlers can render real structure against hardcoded demo
// data. Swapping the demo data for typed MCP results is a one-function
// change per handler (see handlers.go). No live MCP data flows yet.
//
// CSS/JS/marks are lifted near-verbatim from the design prototype
// (docs/proposals/design-prototype) and embedded; the Go template layer
// reproduces the prototype's App.jsx shell, sheet.jsx primitive, and
// panels.jsx slide-over as html/template + small JS islands.
package web

import (
	"embed"
	"html/template"
	"io"
	"io/fs"
	"net/http"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

// templates is parsed once at init. Each top-level page template ("layout")
// composes the named block partials (sheet, panel, palette, etc.).
var templates = template.Must(template.New("pilot").Funcs(funcs).ParseFS(templateFS, "templates/*.html"))

// funcs are the small set of helpers the templates lean on.
var funcs = template.FuncMap{
	// add is used for 1-based indexing / width math in range blocks.
	"add": func(a, b int) int { return a + b },
}

// StaticHandler returns an http.Handler serving the embedded static assets
// (CSS/JS/marks) under their on-disk paths. Mount it at /static/ — e.g.
//
//	mux.Handle("GET /static/", http.StripPrefix("/static/", web.StaticHandler()))
//
// TODO(phase 5): add long-lived cache headers + content hashing once the
// asset set stabilises.
func StaticHandler() http.Handler {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		// staticFS is embedded at build time; a missing subtree is a
		// programmer error, not a runtime condition.
		panic(err)
	}
	return http.FileServer(http.FS(sub))
}

// Render executes the named template against data and writes HTML to w.
// Handlers call Render(w, "layout", page) where page is a *Page.
func Render(w io.Writer, name string, data any) error {
	return templates.ExecuteTemplate(w, name, data)
}
