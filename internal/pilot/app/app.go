// Package app embeds the DITS RE prototype React application and serves it as
// static files. The prototype (App/data/sheet/views/panels.jsx + the design
// system in ds/) is loaded verbatim from docs/proposals/design-prototype and
// transpiled in the browser by @babel/standalone — there is no build step.
//
// index.html is the host page: it pulls React 18, ReactDOM, and
// @babel/standalone from unpkg (CDN), links the three CSS files, and loads each
// JSX file with <script type="text/babel" src="...">. data.jsx sets
// window.DATA (the hardcoded seed data); App.jsx renders <App/> into #root.
//
// This package imports nothing from internal/ (it serves embedded bytes only),
// so it stays clear of Pilot's MCP-only topology rule (§2, TestForbiddenImports).
package app

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed index.html app ds
var assetsFS embed.FS

// Handler serves the embedded prototype app. Mount it under a prefix (the web
// layer strips "/app/" before handing the request here), so a request for
// "index.html" serves the host page and "app/App.jsx" serves the JSX source.
// JSX files are served with their default text content type, which is all
// @babel/standalone needs — it fetches the src text and transpiles it.
func Handler() http.Handler {
	return http.FileServer(http.FS(assetsFS))
}

// FS returns the embedded asset tree, for callers that want to read individual
// files (e.g. a track that injects live data into index.html later).
func FS() fs.FS {
	return assetsFS
}
