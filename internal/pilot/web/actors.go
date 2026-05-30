// The actor directory — a small read-through cache over the substrate's
// dits_actor_list. The substrate carries NO display name or colour for an
// actor (an ActorRecord is just id + public key + node), so Pilot derives
// initials and colour deterministically from the actor id (see
// handlers/cells.go: actorInitials/actorColor). The directory's job is simply
// to enumerate the known actor set so the searchable ActorPicker and the
// per-view filter fields have options to offer.
//
// The cache is a Pilot-side convenience, not a source of truth: the substrate
// remains authoritative, and BindRole/UnbindRole writes go straight through
// mcp.Client. We refresh on a short TTL so a freshly registered actor shows up
// in the picker without a process restart, but never block a render on it.
package web

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/lenulus/pf/internal/pilot/mcp"
)

// actorCacheTTL bounds how stale the cached actor set may be. The directory is
// only ever used to populate picker/filter options, so a coarse TTL is fine.
const actorCacheTTL = 30 * time.Second

// ActorDirectory is a TTL-cached view of the substrate's known actors. It is
// safe for concurrent use by request handlers. Construct one with
// NewActorDirectory(client) and hang it off the Server.
type ActorDirectory struct {
	client mcp.Client

	mu      sync.Mutex
	actors  []mcp.ActorRecord
	fetched time.Time
}

// NewActorDirectory returns a directory backed by the given MCP client. The
// first List call populates the cache lazily; nothing is fetched here.
func NewActorDirectory(client mcp.Client) *ActorDirectory {
	return &ActorDirectory{client: client}
}

// List returns the known actor set, sorted by actor id, refreshing from the
// substrate when the cache is empty or older than actorCacheTTL. A query error
// is non-fatal: the last good snapshot (possibly empty) is returned so the
// picker degrades to "no options" rather than failing the request.
func (d *ActorDirectory) List(ctx context.Context) []mcp.ActorRecord {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.actors == nil || time.Since(d.fetched) > actorCacheTTL {
		if recs, err := d.client.ActorList(ctx); err == nil {
			// Sort by id so picker order is stable across refreshes.
			sort.Slice(recs, func(i, j int) bool { return recs[i].ActorID < recs[j].ActorID })
			d.actors = recs
			d.fetched = time.Now()
		} else if d.actors == nil {
			// First fetch failed: cache an empty slice so we return a non-nil
			// snapshot and retry on the next TTL window rather than every call.
			d.actors = []mcp.ActorRecord{}
			d.fetched = time.Now()
		}
	}
	out := make([]mcp.ActorRecord, len(d.actors))
	copy(out, d.actors)
	return out
}

// Search returns the actors whose id contains q (case-insensitive), capped at
// limit. An empty q returns the first limit actors. The cap keeps the picker
// list short; the input narrows it further as the user types.
func (d *ActorDirectory) Search(ctx context.Context, q string, limit int) []mcp.ActorRecord {
	all := d.List(ctx)
	q = foldLower(q)
	var out []mcp.ActorRecord
	for _, a := range all {
		if q == "" || contains(foldLower(a.ActorID), q) {
			out = append(out, a)
			if limit > 0 && len(out) >= limit {
				break
			}
		}
	}
	return out
}

// Invalidate drops the cached snapshot so the next List refetches. Call it
// after registering a new actor so the picker reflects it immediately.
func (d *ActorDirectory) Invalidate() {
	d.mu.Lock()
	d.actors = nil
	d.mu.Unlock()
}

// foldLower lowercases ASCII letters (the actor-id alphabet); it avoids pulling
// in strings just for ToLower and matches the lightweight style of cells.go.
func foldLower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}

// contains reports whether sub occurs in s (both already folded).
func contains(s, sub string) bool {
	if sub == "" {
		return true
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
