// Package handlers holds one HTTP handler per Pilot UI route and a Register
// helper that maps the twelve GET paths (+ the /static mount) onto a mux.
// See implementation-plan-v2 §9.2 (the twelve routes across four sidebar
// sections: Workspace, Methodology, Substrate, External).
//
// Phase-5 foundation: each handler renders the layout chrome (TopBar,
// Sidebar, header, hintbar, ⌘K) for its view via the web package. The
// Portfolio handler additionally renders a live Sheet primitive + slide-over
// panel against a small hardcoded demo dataset so the gate is visibly
// working. No live MCP data flows yet — swapping demo data for typed MCP
// results is a one-function change (see the demoPortfolio* helpers): a
// handler builds the same *web.SheetModel / *web.PanelModel from MCP rows
// instead of literals.
package handlers

import (
	"bytes"
	"log"
	"net/http"

	"github.com/lenulus/pf/internal/pilot/web"
)

// Register wires the twelve UI routes, the default-landing redirect, and the
// embedded static-asset mount onto mux.
//
// TODO(phase 5): inject the typed MCP client, session/auth middleware, and
// the renderer; gate /roadmap as the only unauthenticated route (§9.6).
func Register(mux *http.ServeMux) {
	// Static assets (CSS/JS/marks) from the embedded FS.
	mux.Handle("GET /static/", http.StripPrefix("/static/", web.StaticHandler()))

	// Workspace.
	mux.HandleFunc("GET /for-you", ForYou) // default landing — Attention view
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

// renderLayout renders the standard app shell for a view key, running the
// optional customise hook to attach a body / sheet / panel. Centralising
// this keeps each route handler a one-liner and makes the demo→MCP swap
// local to the per-view customise func.
func renderLayout(w http.ResponseWriter, key string, customise func(*web.Page)) {
	page := web.NewPage(key)
	if customise != nil {
		customise(page)
	}
	var buf bytes.Buffer
	if err := web.Render(&buf, "layout", page); err != nil {
		log.Printf("pilot/web: render %q: %v", key, err)
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = buf.WriteTo(w)
}

// ForYou serves GET /for-you — the default landing Attention view (§9.5).
func ForYou(w http.ResponseWriter, r *http.Request) {
	renderLayout(w, "attention", func(p *web.Page) {
		p.Body = todo("/for-you", "the Attention view — ACKs needed, targets passing, stale updates, risks, idle decisions")
	})
}

// Leadership serves GET /leadership — portfolio rollup + indicators (§9.2).
func Leadership(w http.ResponseWriter, r *http.Request) {
	renderLayout(w, "leadership", func(p *web.Page) {
		p.Body = todo("/leadership", "the four leading indicators, goal rollup, and pattern blocks")
	})
}

// Portfolio serves GET /portfolio — the Sheet primitive (§9.3). This route
// renders a live demo sheet + (on ?open=<id>) the slide-over panel, proving
// the gate. ?group=<key> toggles a column group; ?preset=<id> applies a JTBD
// preset; ?tab=<key> deep-links a panel tab.
func Portfolio(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	renderLayout(w, "portfolio", func(p *web.Page) {
		p.Sheet = demoPortfolioSheet(q.Get("preset"), q.Get("group"), q.Get("open"))
		p.Counters = demoPortfolioCounter()
		if open := q.Get("open"); open != "" {
			p.Panel = demoPortfolioPanel(open, q.Get("tab"))
		}
	})
}

// Ack serves GET /ack — the bulk-ack workspace (S-ACK / B-ACK pivots).
func Ack(w http.ResponseWriter, r *http.Request) {
	renderLayout(w, "ack", func(p *web.Page) {
		p.Body = todo("/ack", "the bulk-ack workspace — Specifier/Builder ACK pivots with A/P/R inline")
	})
}

// Rfcs serves GET /rfcs — the RFC review queue.
func Rfcs(w http.ResponseWriter, r *http.Request) {
	renderLayout(w, "rfcs", func(p *web.Page) {
		p.Body = todo("/rfcs", "the RFC review queue")
	})
}

// Log serves GET /log — Pilot's Log view.
func Log(w http.ResponseWriter, r *http.Request) {
	renderLayout(w, "log", func(p *web.Page) {
		p.Body = todo("/log", "Pilot's Log — quick-entry row + signed journal")
	})
}

// Decisions serves GET /decisions — DecisionBlocks.
func Decisions(w http.ResponseWriter, r *http.Request) {
	renderLayout(w, "decisions", func(p *web.Page) {
		p.Body = todo("/decisions", "the DecisionBlocks queue")
	})
}

// Outcomes serves GET /outcomes — OutcomeAssessments.
func Outcomes(w http.ResponseWriter, r *http.Request) {
	renderLayout(w, "outcomes", func(p *web.Page) {
		p.Body = todo("/outcomes", "Outcome assessments — verdict + value")
	})
}

// Roles serves GET /roles — role bindings with diagnostics (§9.7).
func Roles(w http.ResponseWriter, r *http.Request) {
	renderLayout(w, "roles", func(p *web.Page) {
		p.Body = todo("/roles", "role bindings with constraint diagnostics")
	})
}

// Taxonomies serves GET /taxonomies — Org / Product / Goals tabs.
func Taxonomies(w http.ResponseWriter, r *http.Request) {
	renderLayout(w, "taxonomies", func(p *web.Page) {
		p.Body = todo("/taxonomies", "the Org / Product / Goals taxonomy trees")
	})
}

// Events serves GET /events — read-only event log.
func Events(w http.ResponseWriter, r *http.Request) {
	renderLayout(w, "events", func(p *web.Page) {
		p.Body = todo("/events", "the read-only signed event log")
	})
}

// Roadmap serves GET /roadmap — the public, unauthenticated, filtered
// projection (§9.6). Renders its own minimal layout (no sidebar).
func Roadmap(w http.ResponseWriter, r *http.Request) {
	page := web.NewPage("roadmap")
	var buf bytes.Buffer
	if err := web.Render(&buf, "roadmap", page); err != nil {
		log.Printf("pilot/web: render roadmap: %v", err)
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = buf.WriteTo(w)
}
