// Package scheduler holds Pilot's RE scheduler goroutine. See
// implementation-plan-v2 §10.2: it runs every N seconds (default 60) and,
// per cycle, auto-escalates idle DecisionBlocks, ensures shipped milestones
// have a paired outcome_assessment, and marks overdue assessments. All
// actions emit normal signed events via MCP, attributed to a
// `scheduler.pilot` actor.
//
// Phase-4 skeleton: a Scheduler struct whose Run(ctx) is a TODO no-op that
// returns when the context is cancelled.
package scheduler

import (
	"context"
	"time"

	"github.com/lenulus/pf/internal/pilot/mcp"
)

// DefaultInterval is the scheduler tick period (§10.2).
const DefaultInterval = 60 * time.Second

// Scheduler runs Pilot's periodic RE maintenance cycle.
type Scheduler struct {
	client   mcp.Client
	interval time.Duration
}

// New returns a Scheduler that will drive its cycle through the given MCP
// client at the given interval (use DefaultInterval for the standard tick).
func New(client mcp.Client, interval time.Duration) *Scheduler {
	if interval <= 0 {
		interval = DefaultInterval
	}
	return &Scheduler{client: client, interval: interval}
}

// Run blocks running the scheduler loop until ctx is cancelled.
//
// TODO(phase 6): on each tick, perform the three cycle actions from §10.2:
//   - escalate DecisionBlocks open >=5d (work.review_requested),
//   - create missing outcome_assessment items for newly-shipped milestones,
//   - mark OutcomeAssessments past SLA as overdue (work_set_status).
func (s *Scheduler) Run(ctx context.Context) error {
	// Phase-4 no-op: just wait for cancellation so callers can wire it up.
	<-ctx.Done()
	return ctx.Err()
}
