package domain

import (
	"encoding/json"
	"fmt"
)

// ValidateProtocol checks coordination invariants against the current materialized
// WorkItem state. This is the second validation tier:
//
//  1. Schema validation (ValidateEvent) — checks meta references
//  2. Protocol validation (ValidateProtocol) — checks coordination invariants
//  3. Reducer (ApplyEvent) — deterministic materialization, no rejections
//
// Protocol validation is authoritative at event creation time (CLI). During sync
// ingestion, it is advisory (log warnings, don't reject) because the reducer
// handles concurrent offline divergence via operational lineage.
//
// wi may be nil for work.created events (no prior state exists).
func ValidateProtocol(e Event, wi *WorkItem) error {
	switch e.Type {

	case EventWorkCreated:
		// No prior state needed. Valid as root event.
		return nil

	case EventWorkLeased:
		if wi == nil {
			return fmt.Errorf("cannot lease: work item does not exist")
		}
		if wi.LeaseHolder != nil {
			return fmt.Errorf("cannot lease: already leased by %s", *wi.LeaseHolder)
		}

	case EventWorkLeaseReleased:
		if wi == nil {
			return fmt.Errorf("cannot release lease: work item does not exist")
		}
		if wi.LeaseHolder == nil {
			return fmt.Errorf("cannot release lease: no active lease")
		}
		if e.ActorID != *wi.LeaseHolder {
			return fmt.Errorf("cannot release lease: actor %s is not lease holder %s", e.ActorID, *wi.LeaseHolder)
		}

	case EventWorkLeaseRenewed:
		if wi == nil {
			return fmt.Errorf("cannot renew lease: work item does not exist")
		}
		if wi.LeaseHolder == nil {
			return fmt.Errorf("cannot renew lease: no active lease")
		}
		if e.ActorID != *wi.LeaseHolder {
			return fmt.Errorf("cannot renew lease: actor %s is not lease holder %s", e.ActorID, *wi.LeaseHolder)
		}
		// Validate generation monotonicity.
		var p LeaseRenewedPayload
		if err := json.Unmarshal(e.Payload, &p); err == nil {
			if p.Generation <= wi.LeaseGeneration {
				return fmt.Errorf("cannot renew lease: generation %d is not greater than current %d", p.Generation, wi.LeaseGeneration)
			}
		}

	case EventWorkExecutionStarted:
		if wi == nil {
			return fmt.Errorf("cannot start execution: work item does not exist")
		}
		if wi.LeaseHolder == nil {
			return fmt.Errorf("cannot start execution: no active lease")
		}
		if e.ActorID != *wi.LeaseHolder {
			return fmt.Errorf("cannot start execution: actor %s is not lease holder %s", e.ActorID, *wi.LeaseHolder)
		}
		// Check for running authoritative attempt.
		for _, a := range wi.Attempts {
			if a.Status == "running" && a.Authoritative {
				return fmt.Errorf("cannot start execution: attempt %s is already running", a.AttemptID)
			}
		}

	case EventWorkExecutionCompleted, EventWorkExecutionFailed, EventWorkExecutionAbandoned:
		if wi == nil {
			return fmt.Errorf("cannot complete/fail attempt: work item does not exist")
		}
		if wi.CurrentAttempt == nil {
			return fmt.Errorf("cannot complete/fail attempt: no active attempt")
		}
		if wi.LeaseHolder != nil && e.ActorID != *wi.LeaseHolder {
			return fmt.Errorf("cannot complete/fail attempt: actor %s is not lease holder %s", e.ActorID, *wi.LeaseHolder)
		}
		// Verify the referenced attempt is running.
		var p struct {
			AttemptID AttemptID `json:"attempt_id"`
		}
		if err := json.Unmarshal(e.Payload, &p); err == nil {
			if a := findAttemptInSlice(wi.Attempts, p.AttemptID); a != nil {
				if a.Status != "running" {
					return fmt.Errorf("cannot complete/fail attempt %s: status is %q, not running", p.AttemptID, a.Status)
				}
			}
		}

	case EventWorkCheckpointed:
		if wi == nil {
			return fmt.Errorf("cannot checkpoint: work item does not exist")
		}
		if wi.CurrentAttempt == nil {
			return fmt.Errorf("cannot checkpoint: no active attempt")
		}
		if wi.LeaseHolder != nil && e.ActorID != *wi.LeaseHolder {
			return fmt.Errorf("cannot checkpoint: actor %s is not lease holder %s", e.ActorID, *wi.LeaseHolder)
		}

	case EventWorkBlocked:
		if wi != nil && wi.Blocked {
			return fmt.Errorf("cannot block: already blocked")
		}

	case EventWorkUnblocked:
		if wi != nil && !wi.Blocked {
			return fmt.Errorf("cannot unblock: not blocked")
		}

	// --- Reference integrity checks ---
	// These validate that cross-event references point to existing entities
	// in the materialized WorkItem state.

	case EventWorkFindingRetracted:
		if wi != nil {
			var p FindingRetractedPayload
			if err := json.Unmarshal(e.Payload, &p); err == nil {
				found := false
				for _, f := range wi.Findings {
					if f.EventID == p.OriginalEventID {
						found = true
						break
					}
				}
				if !found {
					return fmt.Errorf("cannot retract finding: event %s not found in findings", p.OriginalEventID)
				}
			}
		}

	case EventWorkOutcomeRetained, EventWorkOutcomeDiscarded:
		if wi != nil {
			var subjectRef string
			var subjectKind string
			if e.Type == EventWorkOutcomeRetained {
				var p OutcomeRetainedPayload
				if err := json.Unmarshal(e.Payload, &p); err == nil {
					subjectRef = p.SubjectRef
					subjectKind = p.SubjectKind
				}
			} else {
				var p OutcomeDiscardedPayload
				if err := json.Unmarshal(e.Payload, &p); err == nil {
					subjectRef = p.SubjectRef
					subjectKind = p.SubjectKind
				}
			}
			if subjectRef != "" && !subjectExistsInWorkItem(wi, subjectKind, subjectRef) {
				return fmt.Errorf("cannot %s outcome: subject %s %q not found in work item",
					map[EventType]string{EventWorkOutcomeRetained: "retain", EventWorkOutcomeDiscarded: "discard"}[e.Type],
					subjectKind, subjectRef)
			}
		}

	// NOTE: Reference integrity for plans (plan_accepted → plan_proposed),
	// reviews (review_completed → review_requested), handoffs
	// (handoff_accepted → handed_off), and evals (eval_completed →
	// eval_requested) requires event-level lookups in the DAG, not just
	// materialized WorkItem state. These events are not materialized into
	// sub-entity collections. Full reference integrity for these event
	// families is a future improvement.
	}

	return nil
}

// subjectExistsInWorkItem checks if a subject reference exists in the work item's
// attempts (by AttemptID) or artifacts (by ArtifactID or ContentHash).
func subjectExistsInWorkItem(wi *WorkItem, kind, ref string) bool {
	switch kind {
	case "attempt":
		for _, a := range wi.Attempts {
			if string(a.AttemptID) == ref {
				return true
			}
		}
	case "artifact":
		for _, a := range wi.Artifacts {
			if string(a.ID) == ref || a.ContentHash == ref {
				return true
			}
		}
	default:
		// Unknown kind — allow through (soft check).
		return true
	}
	return false
}

// findAttemptInSlice finds an attempt by ID in a slice. Returns nil if not found.
// This is a local helper to avoid importing the reduce.go helper (which operates
// on mutable slices).
func findAttemptInSlice(attempts []ExecutionAttempt, id AttemptID) *ExecutionAttempt {
	for i := range attempts {
		if attempts[i].AttemptID == id {
			return &attempts[i]
		}
	}
	return nil
}
