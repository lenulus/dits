// Package handlers holds one HTTP handler per Pilot UI route and a Server that
// carries the typed MCP client. See implementation-plan-v2 §9.2 (the twelve
// routes across four sidebar sections: Workspace, Methodology, Substrate,
// External).
//
// Each handler fetches live substrate data over MCP (s.Client) and maps it
// into the generic web view models (see builders.go), then renders the layout
// chrome (TopBar, Sidebar, header, hintbar, ⌘K) via the web package. With the
// stub client (no MCP endpoint configured) queries return empty, so views
// render their empty state and the chrome still works.
package handlers

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strings"

	"github.com/lenulus/pf/internal/pilot/mcp"
	"github.com/lenulus/pf/internal/pilot/projections"
	"github.com/lenulus/pf/internal/pilot/web"
)

// Server carries the dependencies the route handlers need: the typed MCP
// client and the in-memory indicator cache. Future: session/auth middleware.
type Server struct {
	Client     mcp.Client
	Indicators *projections.Cache
	Actors     *web.ActorDirectory
}

// New returns a Server backed by the given MCP client.
func New(client mcp.Client) *Server {
	return &Server{
		Client:     client,
		Indicators: projections.NewCache(),
		Actors:     web.NewActorDirectory(client),
	}
}

// Register wires the twelve UI routes, the default-landing redirect, and the
// embedded static-asset mount onto mux.
func (s *Server) Register(mux *http.ServeMux) {
	mux.Handle("GET /static/", http.StripPrefix("/static/", web.StaticHandler()))

	// The prototype React app (data-backed pivot) is served at /app. It coexists
	// with the hand-rolled server-rendered views during the transition.
	s.registerApp(mux)

	// Workspace.
	mux.HandleFunc("GET /for-you", s.ForYou) // default landing — Attention view
	mux.HandleFunc("GET /leadership", s.Leadership)
	mux.HandleFunc("GET /portfolio", s.Portfolio)
	mux.HandleFunc("GET /ack", s.Ack)
	mux.HandleFunc("GET /rfcs", s.Rfcs)
	mux.HandleFunc("GET /log", s.Log)

	// Methodology.
	mux.HandleFunc("GET /decisions", s.Decisions)
	mux.HandleFunc("GET /outcomes", s.Outcomes)

	// Substrate.
	mux.HandleFunc("GET /roles", s.Roles)
	mux.HandleFunc("GET /taxonomies", s.Taxonomies)
	mux.HandleFunc("GET /events", s.Events)

	// External — public, unauthenticated (§9.6).
	mux.HandleFunc("GET /roadmap", s.Roadmap)

	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/for-you", http.StatusFound)
	})

	// Phase-5 write path: POST endpoints that perform one MCP mutation each.
	s.registerMutations(mux)

	// Pilot pivot: the prototype app's live data feed + JSON write endpoint.
	mux.HandleFunc("GET /api/data", s.apiData)
	s.registerAPIMutate(mux)

	// HTMX partial-swap endpoints (/partials/...) — inline cell edits, panel
	// tab/row swaps, in-place Attention actions. See partials.go.
	s.registerPartials(mux)
}

// renderLayout renders the standard app shell for a view key.
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

// milestones fetches all milestone work items, logging on error.
func (s *Server) milestones(ctx context.Context) []mcp.WorkItem {
	items, err := s.Client.WorkList(ctx, mcp.Filters{Kind: "milestone"})
	if err != nil {
		log.Printf("pilot/web: WorkList(milestone): %v", err)
	}
	return items
}

// errBody renders a substrate-unreachable notice as a view body.
func errBody(err error) template.HTML {
	return template.HTML(`<div class="empty-line">Substrate query failed: <strong>` +
		template.HTMLEscapeString(err.Error()) + `</strong>. Is <code>dits-mcp</code> reachable? ` +
		`(Pilot serves with --mcp-command / --project; without it the stub client renders empty state.)</div>`)
}

// ForYou serves GET /for-you — the default landing Attention view (§9.5).
func (s *Server) ForYou(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	items := s.milestones(ctx)
	decisions, _ := s.Client.WorkList(ctx, mcp.Filters{Kind: "decision_block"})
	outcomes, _ := s.Client.WorkList(ctx, mcp.Filters{Kind: "outcome_assessment"})
	rfcs, _ := s.Client.WorkList(ctx, mcp.Filters{Kind: "rfc"})
	renderLayout(w, "attention", func(p *web.Page) {
		p.Body = buildAttentionBody(items, decisions, outcomes, rfcs)
		p.Counters = fmt.Sprintf("%d milestones", len(items))
	})
}

// Leadership serves GET /leadership — portfolio rollup + the four leading
// indicators (§9.2). The indicator cache is rebuilt on each load; a later
// pass invalidates it on write instead (§10.3).
func (s *Server) Leadership(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	items := s.milestones(ctx)
	decisions, _ := s.Client.WorkList(ctx, mcp.Filters{Kind: "decision_block"})
	outcomes, _ := s.Client.WorkList(ctx, mcp.Filters{Kind: "outcome_assessment"})
	meta, _ := s.Client.MetaGet(ctx)
	var ind projections.Indicators
	if err := s.Indicators.Rebuild(ctx, s.Client); err == nil {
		ind, _ = s.Indicators.Get()
	}
	renderLayout(w, "leadership", func(p *web.Page) {
		p.Body = buildIndicatorRow(ind) + buildLeadershipBody(items, decisions, outcomes, meta)
	})
}

// Portfolio serves GET /portfolio — the Sheet primitive over live milestones
// (§9.3). ?open=<id> opens the slide-over; ?group / ?preset / ?tab adjust it.
func (s *Server) Portfolio(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	ctx := r.Context()
	items := s.milestones(ctx)
	renderLayout(w, "portfolio", func(p *web.Page) {
		p.Sheet = buildPortfolioSheet(items, PortfolioParamsFromQuery(q, items))
		p.Counters = fmt.Sprintf("%d milestones", len(items))
		if open := q.Get("open"); open != "" {
			panel := web.NewMilestonePanel(open, open, q.Get("tab"))
			// Fetch the opened record so the panel hosts live, editable controls.
			if item, err := s.Client.WorkGet(ctx, open); err == nil {
				panel.Title = item.Title
				panel.Body = s.buildMilestoneTab(ctx, item, panel.ActiveTab)
				panel.Footer = milestoneFooter(item)
			}
			p.Panel = panel
		}
	})
}

// Ack serves GET /ack — the bulk-ack workspace (§9.2).
func (s *Server) Ack(w http.ResponseWriter, r *http.Request) {
	items := s.milestones(r.Context())
	renderLayout(w, "ack", func(p *web.Page) {
		p.Sheet = buildAckSheet(items)
		p.Counters = fmt.Sprintf("%d milestones", len(items))
	})
}

// Rfcs serves GET /rfcs — the RFC review queue.
func (s *Server) Rfcs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	items, err := s.Client.WorkList(ctx, mcp.Filters{Kind: "rfc"})
	renderLayout(w, "rfcs", func(p *web.Page) {
		if err != nil {
			p.Body = errBody(err)
			return
		}
		p.Sheet = buildRfcSheet(items)
		p.Counters = fmt.Sprintf("%d RFCs", len(items))
		if open := r.URL.Query().Get("open"); open != "" {
			if item, err := s.Client.WorkGet(ctx, open); err == nil {
				panel := web.NewDetailPanel(open, item.Title)
				panel.Body = s.rfcDetailBody(ctx, item)
				p.Panel = panel
			}
		}
	})
}

// Log serves GET /log — Pilot's Log (observations across milestones).
func (s *Server) Log(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	events, err := s.Client.EventsList(ctx, "work.observation_recorded", "", 100)
	milestones := s.milestones(ctx)
	renderLayout(w, "log", func(p *web.Page) {
		if err != nil {
			p.Body = errBody(err)
			return
		}
		p.Body = buildLogBody(events, milestones)
		p.Counters = fmt.Sprintf("%d entries", len(events))
	})
}

// Decisions serves GET /decisions — DecisionBlocks.
func (s *Server) Decisions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	items, err := s.Client.WorkList(ctx, mcp.Filters{Kind: "decision_block"})
	renderLayout(w, "decisions", func(p *web.Page) {
		if err != nil {
			p.Body = errBody(err)
			return
		}
		p.Sheet = buildDecisionsSheet(items, sharedIDs(s.milestones(ctx)))
		p.Counters = fmt.Sprintf("%d decisions", len(items))
		if open := r.URL.Query().Get("open"); open != "" {
			if item, err := s.Client.WorkGet(ctx, open); err == nil {
				panel := web.NewDetailPanel(open, item.Title)
				panel.Body = decisionDetailBody(item)
				panel.Footer = decisionFooter(item)
				p.Panel = panel
			}
		}
	})
}

// Outcomes serves GET /outcomes — OutcomeAssessments.
func (s *Server) Outcomes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	items, err := s.Client.WorkList(ctx, mcp.Filters{Kind: "outcome_assessment"})
	renderLayout(w, "outcomes", func(p *web.Page) {
		if err != nil {
			p.Body = errBody(err)
			return
		}
		p.Sheet = buildOutcomesSheet(items, sharedIDs(s.milestones(ctx)))
		p.Counters = fmt.Sprintf("%d assessments", len(items))
		if open := r.URL.Query().Get("open"); open != "" {
			if item, err := s.Client.WorkGet(ctx, open); err == nil {
				panel := web.NewDetailPanel(open, item.Title)
				panel.Body = outcomeDetailBody(item)
				p.Panel = panel
			}
		}
	})
}

// Roles serves GET /roles — role bindings with diagnostics (§9.7).
func (s *Server) Roles(w http.ResponseWriter, r *http.Request) {
	items := s.milestones(r.Context())
	renderLayout(w, "roles", func(p *web.Page) {
		p.Sheet = buildRolesSheet(items)
	})
}

// Taxonomies serves GET /taxonomies — Org / Product / Goals trees from meta.
func (s *Server) Taxonomies(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	meta, err := s.Client.MetaGet(ctx)
	milestones := s.milestones(ctx)
	renderLayout(w, "taxonomies", func(p *web.Page) {
		if err != nil {
			p.Body = errBody(err)
			return
		}
		p.Body = buildTaxonomyBody(meta, r.URL.Query().Get("tax"), milestones)
	})
}

// Events serves GET /events — the read-only signed event log.
func (s *Server) Events(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	events, err := s.Client.EventsList(ctx, "", "", 200)
	all, _ := s.Client.WorkList(ctx, mcp.Filters{}) // resolve event targets to shared ids
	renderLayout(w, "events", func(p *web.Page) {
		if err != nil {
			p.Body = errBody(err)
			return
		}
		p.Body = buildEventsBody(events, sharedIDs(all))
		p.Counters = fmt.Sprintf("%d recent events", len(events))
	})
}

// Roadmap serves GET /roadmap — the public, unauthenticated filtered
// projection (§9.6). Renders its own minimal layout (no sidebar).
func (s *Server) Roadmap(w http.ResponseWriter, r *http.Request) {
	items := buildRoadmapItems(s.milestones(r.Context()))
	page := web.NewPage("roadmap")
	var b strings.Builder
	if len(items) == 0 {
		b.WriteString(`<div class="empty-line">No committed milestones to show yet.</div>`)
	}
	for _, m := range items {
		b.WriteString(`<div class="roadmap-card"><span style="flex:1"><div class="ttl">` +
			template.HTMLEscapeString(m.Title) + `</div><div class="sub">` +
			template.HTMLEscapeString(productLeaf(m)) + `</div></span><span class="pill">` +
			string(statusPill(m.Status)) + `</span></div>`)
	}
	page.Body = template.HTML(b.String())
	var buf bytes.Buffer
	if err := web.Render(&buf, "roadmap", page); err != nil {
		log.Printf("pilot/web: render roadmap: %v", err)
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = buf.WriteTo(w)
}

func productLeaf(m mcp.WorkItem) string {
	for _, c := range m.Classifications {
		if c.TaxonomySlug == "product" {
			parts := strings.Split(c.NodeSlug, "/")
			return strings.Join(parts[max(0, len(parts)-2):], " / ")
		}
	}
	return ""
}
