// Package scheduler holds Pilot's RE scheduler goroutine (plan §10.2). It
// runs every interval (default 60s) and, per cycle (RunOnce), performs three
// time-based maintenance actions over the substrate via MCP:
//
//  1. Ensure every shipped milestone has a paired outcome_assessment;
//     create + link one if missing.
//  2. Mark pending outcome_assessments past their SLA (default 90d) overdue.
//  3. Escalate DecisionBlocks left open past the idle window (default 9d).
//
// Attribution: §10.2 says the scheduler signs as a dedicated `scheduler.pilot`
// actor. v1 routes every action through dits-mcp, which signs as the project
// actor, so on the wire these events are attributed to the dits-mcp actor —
// not a distinct scheduler identity. Honest attribution via a custodial
// `scheduler.pilot` key is a follow-up, the same gap as Pilot's OAuth/custodial
// signing bridge (§8.2). The custodial path is intentionally not built here.
//
// Design §10.2 wants idle DecisionBlocks to emit `work.review_requested`
// targeting Leadership. The `dits_review_request` tool now exists, so
// escalateIdleDecisions emits a real review request to the leadership role AND
// transitions the status to escalated — the review routes the decision to
// Leadership while the status keeps the substrate-visible signal the read-views
// rely on.
package scheduler

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/lenulus/pf/internal/pilot/mcp"
)

// Cycle defaults (§10.2).
const (
	// DefaultInterval is the scheduler tick period.
	DefaultInterval = 60 * time.Second
	// DefaultAssessmentSLA is how long a pending outcome_assessment may sit
	// before the scheduler marks it overdue.
	DefaultAssessmentSLA = 90 * 24 * time.Hour
	// DefaultDecisionIdle is how long a DecisionBlock may stay open (no
	// update) before the scheduler escalates it.
	DefaultDecisionIdle = 9 * 24 * time.Hour
)

// Work-kind, status, and relation constants this cycle reasons about. These
// match the RE meta bundle (internal/pilot/meta/re_default.json).
const (
	kindMilestone  = "milestone"
	kindAssessment = "outcome_assessment"
	kindDecision   = "decision_block"

	statusShipped   = "shipped"
	statusPending   = "pending"
	statusOpen      = "open"
	statusOverdue   = "overdue"
	statusEscalated = "escalated"

	relRelatesTo = "relates_to"

	// roleLeadership is the reviewer role idle decisions escalate to (§10.2).
	roleLeadership = "leadership"
	// scopeDecisionIdle labels the review request the scheduler emits.
	scopeDecisionIdle = "decision_idle"
)

// Summary reports what a single RunOnce cycle did.
type Summary struct {
	AssessmentsCreated int // shipped milestones that got a new outcome_assessment
	MarkedOverdue      int // pending assessments past SLA set to overdue
	Escalated          int // idle open DecisionBlocks set to escalated
}

// Scheduler runs Pilot's periodic RE maintenance cycle over MCP.
type Scheduler struct {
	client   mcp.Client
	interval time.Duration

	// AssessmentSLA / DecisionIdle are the time windows for actions 2 and 3.
	// Exported so callers can tune them; New seeds the defaults.
	AssessmentSLA time.Duration
	DecisionIdle  time.Duration

	// now is the clock, injected for testable time. Defaults to time.Now.
	now func() time.Time
}

// New returns a Scheduler driving its cycle through client at the given
// interval (use DefaultInterval for the standard tick). SLA/idle windows and
// the clock get their defaults; set the exported fields / call SetClock to
// override.
func New(client mcp.Client, interval time.Duration) *Scheduler {
	if interval <= 0 {
		interval = DefaultInterval
	}
	return &Scheduler{
		client:        client,
		interval:      interval,
		AssessmentSLA: DefaultAssessmentSLA,
		DecisionIdle:  DefaultDecisionIdle,
		now:           time.Now,
	}
}

// SetClock overrides the scheduler's clock (for tests). A nil fn is ignored.
func (s *Scheduler) SetClock(fn func() time.Time) {
	if fn != nil {
		s.now = fn
	}
}

// Run blocks running the cycle every interval until ctx is cancelled. Each
// tick calls RunOnce; a cycle error is logged and the loop continues (a
// transient MCP failure shouldn't kill the scheduler). Returns ctx.Err().
func (s *Scheduler) Run(ctx context.Context) error {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if _, err := s.RunOnce(ctx); err != nil {
				log.Printf("pilot/scheduler: cycle error: %v", err)
			}
		}
	}
}

// RunOnce performs one maintenance cycle and returns what it did. The three
// actions are independent; a failure in one is returned but does not abort
// the others (so a partial cycle still makes progress). When multiple actions
// fail, the first error is returned and the Summary reflects whatever
// succeeded.
func (s *Scheduler) RunOnce(ctx context.Context) (Summary, error) {
	var sum Summary
	var firstErr error
	note := func(err error) {
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}

	n, err := s.ensureAssessments(ctx)
	sum.AssessmentsCreated = n
	note(err)

	n, err = s.markOverdueAssessments(ctx)
	sum.MarkedOverdue = n
	note(err)

	n, err = s.escalateIdleDecisions(ctx)
	sum.Escalated = n
	note(err)

	return sum, firstErr
}

// ensureAssessments creates a paired outcome_assessment for every shipped
// milestone that lacks one (action 1).
func (s *Scheduler) ensureAssessments(ctx context.Context) (int, error) {
	shipped, err := s.client.WorkList(ctx, mcp.Filters{Kind: kindMilestone, Status: statusShipped})
	if err != nil {
		return 0, fmt.Errorf("list shipped milestones: %w", err)
	}
	if len(shipped) == 0 {
		return 0, nil
	}

	// Existing assessments (any status) so we can detect prior pairings.
	assessments, err := s.client.WorkList(ctx, mcp.Filters{Kind: kindAssessment})
	if err != nil {
		return 0, fmt.Errorf("list outcome assessments: %w", err)
	}

	created := 0
	for _, m := range shipped {
		if hasPairedAssessment(m, assessments) {
			continue
		}
		title := assessmentTitle(m)
		newID, err := s.client.WorkCreate(ctx, kindAssessment, title, "")
		if err != nil {
			return created, fmt.Errorf("create assessment for %s: %w", milestoneRef(m), err)
		}
		if err := s.client.Link(ctx, newID, relRelatesTo, milestoneRef(m)); err != nil {
			return created, fmt.Errorf("link assessment %s → %s: %w", newID, milestoneRef(m), err)
		}
		created++
		log.Printf("pilot/scheduler: created outcome_assessment %s for shipped milestone %s", newID, milestoneRef(m))
	}
	return created, nil
}

// markOverdueAssessments sets pending assessments older than the SLA to
// overdue (action 2).
func (s *Scheduler) markOverdueAssessments(ctx context.Context) (int, error) {
	pending, err := s.client.WorkList(ctx, mcp.Filters{Kind: kindAssessment, Status: statusPending})
	if err != nil {
		return 0, fmt.Errorf("list pending assessments: %w", err)
	}
	cutoff := s.now().Add(-s.AssessmentSLA)
	marked := 0
	for _, a := range pending {
		created, ok := parseTime(a.CreatedAt)
		if !ok {
			continue // skip items with empty/unparseable timestamps
		}
		if created.After(cutoff) {
			continue // still within SLA
		}
		if err := s.client.SetStatus(ctx, workRef(a), statusOverdue); err != nil {
			return marked, fmt.Errorf("mark %s overdue: %w", workRef(a), err)
		}
		marked++
		log.Printf("pilot/scheduler: marked outcome_assessment %s overdue (created %s, SLA %s)", workRef(a), a.CreatedAt, s.AssessmentSLA)
	}
	return marked, nil
}

// escalateIdleDecisions escalates open DecisionBlocks idle past the window
// (action 3).
//
// Per §10.2 it emits a real work.review_requested targeting the leadership role
// (via dits_review_request), then transitions the status to escalated. The
// review routes the decision to Leadership; the status keeps the
// substrate-visible signal the read-views rely on.
func (s *Scheduler) escalateIdleDecisions(ctx context.Context) (int, error) {
	open, err := s.client.WorkList(ctx, mcp.Filters{Kind: kindDecision, Status: statusOpen})
	if err != nil {
		return 0, fmt.Errorf("list open decision blocks: %w", err)
	}
	cutoff := s.now().Add(-s.DecisionIdle)
	escalated := 0
	for _, d := range open {
		// Idle is measured from the last update; fall back to creation time.
		ts := d.UpdatedAt
		if ts == "" {
			ts = d.CreatedAt
		}
		updated, ok := parseTime(ts)
		if !ok {
			continue // skip items with empty/unparseable timestamps
		}
		if updated.After(cutoff) {
			continue // still fresh
		}
		if err := s.client.ReviewRequest(ctx, workRef(d), roleLeadership, scopeDecisionIdle); err != nil {
			return escalated, fmt.Errorf("request leadership review for %s: %w", workRef(d), err)
		}
		if err := s.client.SetStatus(ctx, workRef(d), statusEscalated); err != nil {
			return escalated, fmt.Errorf("escalate %s: %w", workRef(d), err)
		}
		escalated++
		log.Printf("pilot/scheduler: escalated idle decision_block %s to leadership review (idle since %s, window %s)", workRef(d), ts, s.DecisionIdle)
	}
	return escalated, nil
}

// hasPairedAssessment reports whether any assessment is already paired with
// milestone m — either by a relates_to relation back to it, or by the title
// convention "Outcome: <ref>". Matching the relation against both the milestone
// ID and SharedID is deliberate: the substrate may store either as the link
// target.
func hasPairedAssessment(m mcp.WorkItem, assessments []mcp.WorkItem) bool {
	wantTitle := assessmentTitle(m)
	for _, a := range assessments {
		if a.Title == wantTitle {
			return true
		}
		for _, rel := range a.Relations {
			if rel.Type != relRelatesTo {
				continue
			}
			if rel.TargetWorkItem == m.ID || (m.SharedID != "" && rel.TargetWorkItem == m.SharedID) {
				return true
			}
		}
	}
	return false
}

// assessmentTitle is the title-convention pairing key (§ task spec).
func assessmentTitle(m mcp.WorkItem) string {
	return "Outcome: " + m.Title
}

// milestoneRef is the identifier to link an assessment to: prefer the stable
// SharedID, fall back to the internal ID.
func milestoneRef(m mcp.WorkItem) string {
	if m.SharedID != "" {
		return m.SharedID
	}
	return m.ID
}

// workRef is the identifier to mutate a work item by: prefer SharedID, fall
// back to ID. (The MCP tools accept either form.)
func workRef(w mcp.WorkItem) string {
	if w.SharedID != "" {
		return w.SharedID
	}
	return w.ID
}

// parseTime parses an RFC3339 timestamp, reporting ok=false for empty or
// unparseable input so callers can skip the item safely.
func parseTime(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}
