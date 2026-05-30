package projections

import (
	"context"
	"sync"
	"time"

	"github.com/lenulus/pf/internal/pilot/mcp"
)

// DefaultWindowWeeks is the rolling window (in weeks) used to normalize
// rate-style indicators such as scope-change velocity (plan §10.3).
const DefaultWindowWeeks = 4

// Indicators is a snapshot of the four RE leading indicators plus the time it
// was computed. Durations are reported as-is; views format them.
type Indicators struct {
	DependencyClosureRate float64       // 0..1
	ScopeChangeVelocity   float64       // scope_change amendments per week
	RFCDecisionFriction   time.Duration // median RFC draft→approved
	DecisionBlockFriction time.Duration // median DecisionBlock open→resolved/escalated
	AckToStartLatency     time.Duration // median aligned→first execution_started
	ComputedAt            time.Time
}

// Cache holds the most recently computed indicator snapshot in memory. It is
// rebuilt on demand (Rebuild) and, per §10.3, the caller invalidates it when
// Pilot writes an event that affects an indicator. v1 computes in Pilot to
// keep DITS substrate-clean; this can be promoted to a DITS-side
// indicators_get tool later without changing the Cache's surface.
//
// The Cache is safe for concurrent use.
type Cache struct {
	mu          sync.RWMutex
	windowWeeks int
	values      Indicators
	valid       bool
}

// NewCache returns an empty cache using DefaultWindowWeeks.
func NewCache() *Cache {
	return &Cache{windowWeeks: DefaultWindowWeeks}
}

// NewCacheWithWindow returns an empty cache using the given rolling window in
// weeks (values < 1 fall back to DefaultWindowWeeks).
func NewCacheWithWindow(weeks int) *Cache {
	if weeks < 1 {
		weeks = DefaultWindowWeeks
	}
	return &Cache{windowWeeks: weeks}
}

// Rebuild fetches the global event log and work items via the client and
// recomputes all four indicators, replacing the cached snapshot. On any fetch
// error the previous snapshot is left untouched and the error is returned.
func (c *Cache) Rebuild(ctx context.Context, client mcp.Client) error {
	events, err := client.EventsList(ctx, "", "", 0)
	if err != nil {
		return err
	}
	items, err := client.WorkList(ctx, mcp.Filters{})
	if err != nil {
		return err
	}

	rfcFriction, dbFriction := DecisionFriction(events)

	c.mu.Lock()
	defer c.mu.Unlock()
	weeks := c.windowWeeks
	if weeks < 1 {
		weeks = DefaultWindowWeeks
	}
	c.values = Indicators{
		DependencyClosureRate: DependencyClosureRate(items),
		ScopeChangeVelocity:   ScopeChangeVelocity(events, weeks),
		RFCDecisionFriction:   rfcFriction,
		DecisionBlockFriction: dbFriction,
		AckToStartLatency:     AckToStartLatency(events),
		ComputedAt:            time.Now(),
	}
	c.valid = true
	return nil
}

// Get returns the cached indicator snapshot and whether it is valid (i.e. a
// successful Rebuild has run since the last Invalidate).
func (c *Cache) Get() (Indicators, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.values, c.valid
}

// Invalidate marks the cached snapshot stale. The next Get reports !valid until
// Rebuild runs again. The retained values are left in place so callers may
// still display a "last known" snapshot if they choose.
func (c *Cache) Invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.valid = false
}
