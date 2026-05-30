// Package handlers holds one HTTP handler per Pilot UI route and a Register
// helper that maps the twelve GET paths onto a mux. See
// implementation-plan-v2 §9.2 (the twelve routes across four sidebar
// sections: Workspace, Methodology, Substrate, External).
//
// Phase-4 skeleton: every handler writes a TODO placeholder. No templates,
// no MCP data, no auth/session checks yet (§9.1 stack is Phase 5).
package handlers

import (
	"fmt"
	"net/http"
)

// Register wires the twelve UI routes onto mux. "/for-you" is the default
// landing view (§9.2, §9.5) and is also served at "/".
//
// TODO(phase 5): inject the typed MCP client, session/auth middleware, and
// the html/template renderer; mount the static FileServer and HTMX islands.
func Register(mux *http.ServeMux) {
	// Workspace.
	mux.HandleFunc("GET /for-you", ForYou)   // default landing — Attention view
	mux.HandleFunc("GET /leadership", Leadership)
	mux.HandleFunc("GET /portfolio", Portfolio)
	mux.HandleFunc("GET /ack", Ack)
	mux.HandleFunc("GET /rfcs", Rfcs)
	mux.HandleFunc("GET /log", Log)

	// Methodology.
	mux.HandleFunc("GET /decisions", Decisions)
	mux.HandleFunc("GET /outcomes", Outcomes)

	// Substrate.
	mux.HandleFunc("GET /roles", Roles)
	mux.HandleFunc("GET /taxonomies", Taxonomies)
	mux.HandleFunc("GET /events", Events)

	// External — public, unauthenticated (§9.6).
	mux.HandleFunc("GET /roadmap", Roadmap)

	// Default landing redirects to the Attention view.
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/for-you", http.StatusFound)
	})
}

// placeholder writes a uniform TODO body for a not-yet-implemented route.
func placeholder(w http.ResponseWriter, route string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(w, "TODO(phase 5): %s view not yet implemented\n", route)
}

// ForYou serves GET /for-you — the default landing Attention view (§9.5).
func ForYou(w http.ResponseWriter, r *http.Request) { placeholder(w, "/for-you") }

// Leadership serves GET /leadership — portfolio rollup + indicators (§9.2).
func Leadership(w http.ResponseWriter, r *http.Request) { placeholder(w, "/leadership") }

// Portfolio serves GET /portfolio — the spreadsheet / Sheet primitive (§9.3).
func Portfolio(w http.ResponseWriter, r *http.Request) { placeholder(w, "/portfolio") }

// Ack serves GET /ack — the bulk-ack workspace (S-ACK / B-ACK pivots).
func Ack(w http.ResponseWriter, r *http.Request) { placeholder(w, "/ack") }

// Rfcs serves GET /rfcs — the RFC review queue.
func Rfcs(w http.ResponseWriter, r *http.Request) { placeholder(w, "/rfcs") }

// Log serves GET /log — Pilot's Log view.
func Log(w http.ResponseWriter, r *http.Request) { placeholder(w, "/log") }

// Decisions serves GET /decisions — DecisionBlocks.
func Decisions(w http.ResponseWriter, r *http.Request) { placeholder(w, "/decisions") }

// Outcomes serves GET /outcomes — OutcomeAssessments.
func Outcomes(w http.ResponseWriter, r *http.Request) { placeholder(w, "/outcomes") }

// Roles serves GET /roles — role bindings with diagnostics (§9.7).
func Roles(w http.ResponseWriter, r *http.Request) { placeholder(w, "/roles") }

// Taxonomies serves GET /taxonomies — Org / Product / Goals tabs.
func Taxonomies(w http.ResponseWriter, r *http.Request) { placeholder(w, "/taxonomies") }

// Events serves GET /events — read-only event log.
func Events(w http.ResponseWriter, r *http.Request) { placeholder(w, "/events") }

// Roadmap serves GET /roadmap — the public, unauthenticated, filtered
// projection (§9.6).
func Roadmap(w http.ResponseWriter, r *http.Request) { placeholder(w, "/roadmap") }
