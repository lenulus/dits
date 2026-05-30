package handlers

import (
	"net/http"

	"github.com/lenulus/pf/internal/pilot/app"
)

// registerApp mounts the embedded DITS RE prototype React app under /app/.
// GET /app/ serves the host page (index.html); GET /app/{path...} serves the
// embedded JSX/CSS assets (e.g. /app/app/App.jsx, /app/ds/shared.css). The
// app boots from CDN React + @babel/standalone and its own window.DATA seed —
// Tracks B and C layer live data and a write API on top of this scaffolding.
//
// The coordinator wires this from Register() (see handlers.go) alongside the
// existing route registrations.
func (s *Server) registerApp(mux *http.ServeMux) {
	mux.Handle("GET /app/", http.StripPrefix("/app/", app.Handler()))
}
