// Package projections computes Pilot's four RE indicators as pure functions
// over the DITS event log (fetched via MCP) and caches them in memory. See
// implementation-plan-v2 §10.3:
//
//   - Dependency closure rate — depends_on edges on shipped milestones.
//   - Scope-change velocity — ack_amended/scope_change per milestone per week.
//   - Decision friction — median RFC created->approved; DecisionBlock open->resolved.
//   - ACK-to-start latency — median aligned->first execution_started.
//
// Phase-4 skeleton: the four functions return zero values, plus an
// in-memory Cache stub. No event-log reads, no real computation.
package projections

import (
	"context"
	"sync"

	"github.com/lenulus/pf/internal/pilot/mcp"
)

// DependencyClosureRate returns the fraction of depends_on edges closed on
// shipped milestones in the window (§10.3).
// TODO(phase 6): compute from MCP-fetched event data.
func DependencyClosureRate(ctx context.Context, c mcp.Client) (float64, error) {
	return 0, nil
}

// ScopeChangeVelocity returns scope-change amendments per milestone per week.
// TODO(phase 6): compute from work.ack_amended events.
func ScopeChangeVelocity(ctx context.Context, c mcp.Client) (float64, error) {
	return 0, nil
}

// DecisionFriction returns median RFC and DecisionBlock turnaround durations.
// TODO(phase 6): compute medians over created->approved / open->resolved.
func DecisionFriction(ctx context.Context, c mcp.Client) (float64, error) {
	return 0, nil
}

// AckToStartLatency returns the median aligned->first-execution latency.
// TODO(phase 6): compute from aligned and work.execution_started events.
func AckToStartLatency(ctx context.Context, c mcp.Client) (float64, error) {
	return 0, nil
}

// Cache holds computed indicator values in memory, rebuilt on Pilot restart
// and invalidated per-indicator when Pilot writes an affecting event (§10.3).
type Cache struct {
	mu     sync.RWMutex
	values map[string]float64
}

// NewCache returns an empty indicator cache.
func NewCache() *Cache {
	return &Cache{values: make(map[string]float64)}
}

// Get returns the cached value for an indicator key and whether it was set.
// TODO(phase 6): key by indicator + scope (project/window) and wire reads
// to serve from cache, computing on miss.
func (c *Cache) Get(key string) (float64, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	v, ok := c.values[key]
	return v, ok
}

// Invalidate drops the cached value for an indicator key.
// TODO(phase 6): call on every Pilot-written event that affects the key.
func (c *Cache) Invalidate(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.values, key)
}
