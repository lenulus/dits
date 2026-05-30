// Package projections computes Pilot's four RE leading indicators (plan
// §10.3) as PURE functions over the data the mcp.Client already returns —
// []mcp.WorkItem from WorkList and []mcp.Event from EventsList. Keeping them
// pure (inputs → number/duration) makes them unit-testable with synthetic
// fixtures and keeps DITS a clean substrate: v1 computes everything in Pilot.
// A later optimization may promote these to a DITS-side indicators_get tool;
// the signatures here are the contract the Cache and views depend on.
//
// The four indicators:
//   - DependencyClosureRate — of depends_on edges on shipped milestones, the
//     fraction whose target is also shipped.
//   - ScopeChangeVelocity — scope_change ack amendments per week.
//   - DecisionFriction — median RFC draft→approved and DecisionBlock
//     open→resolved/escalated turnaround.
//   - AckToStartLatency — median commitment-aligned→first-execution latency.
package projections

import (
	"encoding/json"
	"sort"
	"time"

	"github.com/lenulus/pf/internal/pilot/mcp"
)

// Event type tokens and status/kind slugs this package keys on. These mirror
// DITS-core event types and the RE meta bundle's workflow slugs; they are
// duplicated here as plain strings because Pilot imports neither domain nor
// the meta bundle.
const (
	evCreated          = "work.created"
	evStatusSet        = "work.status_set"
	evExecutionStarted = "work.execution_started"
	evAckAccepted      = "work.ack_accepted"
	evAckFiled         = "work.ack_filed"
	evAckCleared       = "work.ack_cleared"
	evAckRejected      = "work.ack_rejected"
	evAckAmended       = "work.ack_amended"

	kindRFC           = "rfc"
	kindMilestone     = "milestone"
	kindDecisionBlock = "decision_block"

	statusShipped  = "shipped"
	statusApproved = "approved"

	sideSpecifier = "specifier"
	sideBuilder   = "builder"
)

// decisionBlockClosedStatuses are the DecisionBlock terminal statuses (open →
// here) used by DecisionFriction.
var decisionBlockClosedStatuses = map[string]bool{"resolved": true, "escalated": true}

// DependencyClosureRate returns, among the depends_on relations declared on
// shipped milestones, the fraction whose target work item is also shipped.
// Returns 0 when there are no such edges (nothing to close).
func DependencyClosureRate(items []mcp.WorkItem) float64 {
	shipped := make(map[string]bool, len(items))
	for _, wi := range items {
		if wi.Status == statusShipped {
			shipped[wi.ID] = true
			if wi.SharedID != "" {
				shipped[wi.SharedID] = true
			}
		}
	}

	var total, closed int
	for _, wi := range items {
		if wi.Kind != kindMilestone || wi.Status != statusShipped {
			continue
		}
		for _, rel := range wi.Relations {
			if rel.Type != "depends_on" {
				continue
			}
			total++
			if shipped[rel.TargetWorkItem] {
				closed++
			}
		}
	}
	if total == 0 {
		return 0
	}
	return float64(closed) / float64(total)
}

// ScopeChangeVelocity returns the number of scope_change ACK amendments per
// week across the supplied events. weeks must be >= 1; values < 1 are treated
// as 1 so the result is amendments-per-window rather than a divide-by-zero.
func ScopeChangeVelocity(events []mcp.Event, weeks int) float64 {
	if weeks < 1 {
		weeks = 1
	}
	var count int
	for _, e := range events {
		if e.Type != evAckAmended {
			continue
		}
		var p struct {
			AmendmentType string `json:"amendment_type"`
		}
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			continue
		}
		if p.AmendmentType == "scope_change" {
			count++
		}
	}
	return float64(count) / float64(weeks)
}

// DecisionFriction returns the median RFC draft→approved turnaround and the
// median DecisionBlock open→resolved/escalated turnaround. Either is 0 when no
// completed pair exists for that kind. Events are paired by work item: the
// item's kind and start time come from its work.created event, the end time
// from the first qualifying work.status_set.
func DecisionFriction(events []mcp.Event) (rfcMedian, decisionMedian time.Duration) {
	ordered := byTime(events)

	kindOf := map[string]string{} // workItemID -> kind
	createdAt := map[string]time.Time{}
	for _, e := range ordered {
		if e.Type != evCreated {
			continue
		}
		if _, seen := kindOf[e.WorkItemID]; seen {
			continue
		}
		var p struct {
			Kind string `json:"kind"`
		}
		_ = json.Unmarshal(e.Payload, &p)
		kindOf[e.WorkItemID] = p.Kind
		if t, ok := parseTime(e.Timestamp); ok {
			createdAt[e.WorkItemID] = t
		}
	}

	// First qualifying status_set per work item: approved for RFCs, a
	// resolved/escalated transition for DecisionBlocks.
	rfcEnd := map[string]time.Time{}
	dbEnd := map[string]time.Time{}
	for _, e := range ordered {
		if e.Type != evStatusSet {
			continue
		}
		kind := kindOf[e.WorkItemID]
		if kind != kindRFC && kind != kindDecisionBlock {
			continue
		}
		var p struct {
			To string `json:"to"`
		}
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			continue
		}
		t, ok := parseTime(e.Timestamp)
		if !ok {
			continue
		}
		switch {
		case kind == kindRFC && p.To == statusApproved:
			if _, done := rfcEnd[e.WorkItemID]; !done {
				rfcEnd[e.WorkItemID] = t
			}
		case kind == kindDecisionBlock && decisionBlockClosedStatuses[p.To]:
			if _, done := dbEnd[e.WorkItemID]; !done {
				dbEnd[e.WorkItemID] = t
			}
		}
	}

	rfcMedian = medianDuration(pairDurations(createdAt, rfcEnd))
	decisionMedian = medianDuration(pairDurations(createdAt, dbEnd))
	return rfcMedian, decisionMedian
}

// AckToStartLatency returns the median latency from a milestone's commitment
// becoming aligned (both specifier and builder accepted) to its first
// work.execution_started after that point. Returns 0 when no work item has
// both an aligned commitment and a subsequent execution start.
//
// Alignment is tracked per work item by replaying ACK events in time order:
// ack_accepted(who) sets that side and, once both sides are accepted, latches
// the current-commitment alignedAt; ack_filed/ack_cleared/ack_rejected reset
// both sides AND drop alignment (a fresh or cleared commitment must re-align,
// e.g. after a material amendment). The first execution_started that occurs
// while currently aligned yields the item's latency sample, so a start before
// re-alignment does not count.
func AckToStartLatency(events []mcp.Event) time.Duration {
	ordered := byTime(events)

	type ackState struct {
		specifier, builder bool
		alignedAt          time.Time
		aligned            bool
		sampled            bool // latency already recorded for this item
	}
	state := map[string]*ackState{}
	get := func(id string) *ackState {
		s := state[id]
		if s == nil {
			s = &ackState{}
			state[id] = s
		}
		return s
	}

	var samples []time.Duration

	for _, e := range ordered {
		t, ok := parseTime(e.Timestamp)
		if !ok {
			continue
		}
		s := get(e.WorkItemID)
		switch e.Type {
		case evAckFiled, evAckCleared, evAckRejected:
			s.specifier, s.builder, s.aligned = false, false, false
		case evAckAccepted:
			var p struct {
				Who string `json:"who"`
			}
			if err := json.Unmarshal(e.Payload, &p); err != nil {
				continue
			}
			switch p.Who {
			case sideSpecifier:
				s.specifier = true
			case sideBuilder:
				s.builder = true
			}
			if s.specifier && s.builder && !s.aligned {
				s.aligned = true
				s.alignedAt = t
			}
		case evExecutionStarted:
			if s.aligned && !s.sampled && t.After(s.alignedAt) {
				samples = append(samples, t.Sub(s.alignedAt))
				s.sampled = true
			}
		}
	}

	return medianDuration(samples)
}

// --- helpers ---

// byTime returns the events sorted ascending by parsed timestamp. The input
// slice is not mutated; events with unparseable timestamps sort first.
func byTime(events []mcp.Event) []mcp.Event {
	out := make([]mcp.Event, len(events))
	copy(out, events)
	sort.SliceStable(out, func(i, j int) bool {
		ti, _ := parseTime(out[i].Timestamp)
		tj, _ := parseTime(out[j].Timestamp)
		return ti.Before(tj)
	})
	return out
}

// pairDurations returns end-start for every id present in both maps with a
// non-negative duration.
func pairDurations(start, end map[string]time.Time) []time.Duration {
	var out []time.Duration
	for id, e := range end {
		s, ok := start[id]
		if !ok {
			continue
		}
		if d := e.Sub(s); d >= 0 {
			out = append(out, d)
		}
	}
	return out
}

// medianDuration returns the median of the samples, or 0 when empty. For an
// even count it averages the two middle values.
func medianDuration(samples []time.Duration) time.Duration {
	n := len(samples)
	if n == 0 {
		return 0
	}
	sorted := make([]time.Duration, n)
	copy(sorted, samples)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	if n%2 == 1 {
		return sorted[n/2]
	}
	return (sorted[n/2-1] + sorted[n/2]) / 2
}

// parseTime parses an RFC3339(/Nano) timestamp string. Returns ok=false for
// empty or malformed input.
func parseTime(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, true
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, true
	}
	return time.Time{}, false
}
