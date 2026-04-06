package domain

import (
	"encoding/json"
	"fmt"
)

// EventLookup checks whether a prior event of the given type with the given
// reference ID exists for this work item. The key format is "<eventType>:<id>".
// Callers provide the implementation:
//   - CLI: queries events table
//   - Sync: tracks seen events in a map during incremental batch simulation
//   - Tests: simple map[string]bool
type EventLookup func(key string) bool

// EventLookupKey builds a lookup key from an event type and reference ID.
func EventLookupKey(eventType EventType, id string) string {
	return string(eventType) + ":" + id
}

// ValidateProtocol checks coordination invariants against the current materialized
// WorkItem state. Convenience wrapper that skips event-level reference checks.
func ValidateProtocol(e Event, wi *WorkItem) error {
	return ValidateProtocolFull(e, wi, nil)
}

// ValidateProtocolFull checks coordination invariants and, when a lookup function
// is provided, validates cross-event references (plan acceptance → prior proposal,
// review completion → prior request, etc.).
//
// Three validation tiers:
//  1. Schema validation (ValidateEvent) — checks meta references
//  2. Protocol validation (ValidateProtocolFull) — checks coordination invariants
//  3. Reducer (ApplyEvent) — deterministic materialization, no rejections
//
// Protocol validation is authoritative at event creation time (CLI). During sync
// ingestion, it is advisory (log warnings, don't reject).
//
// wi may be nil for work.created events (no prior state exists).
// hasEvent may be nil to skip reference integrity checks.
func ValidateProtocolFull(e Event, wi *WorkItem, hasEvent EventLookup) error {
	switch e.Type {

	case EventWorkCreated:
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
		// Validate lease identity if provided.
		var p LeaseReleasedPayload
		if err := json.Unmarshal(e.Payload, &p); err == nil {
			if p.LeaseID != "" && wi.LeaseID != nil && p.LeaseID != *wi.LeaseID {
				return fmt.Errorf("cannot release lease: lease ID %s does not match active lease %s", p.LeaseID, *wi.LeaseID)
			}
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

	// --- Reference integrity against materialized state ---

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
			var subjectRef, subjectKind string
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

	// --- Event-level reference integrity (requires lookup function) ---

	case EventWorkPlanAccepted, EventWorkPlanRejected:
		if hasEvent != nil {
			var p struct {
				PlanEventID EventID `json:"plan_event_id"`
			}
			if err := json.Unmarshal(e.Payload, &p); err == nil && p.PlanEventID != "" {
				key := EventLookupKey(EventWorkPlanProposed, string(p.PlanEventID))
				if !hasEvent(key) {
					return fmt.Errorf("cannot accept/reject plan: plan_proposed event %s not found", p.PlanEventID)
				}
			}
		}

	case EventWorkReviewCompleted:
		if hasEvent != nil {
			var p struct {
				ReviewID ReviewID `json:"review_id"`
			}
			if err := json.Unmarshal(e.Payload, &p); err == nil && p.ReviewID != "" {
				key := EventLookupKey(EventWorkReviewRequested, string(p.ReviewID))
				if !hasEvent(key) {
					return fmt.Errorf("cannot complete review: review_requested with ID %s not found", p.ReviewID)
				}
			}
		}

	case EventWorkEvalCompleted:
		if hasEvent != nil {
			var p struct {
				EvalID EvalID `json:"eval_id"`
			}
			if err := json.Unmarshal(e.Payload, &p); err == nil && p.EvalID != "" {
				key := EventLookupKey(EventWorkEvalRequested, string(p.EvalID))
				if !hasEvent(key) {
					return fmt.Errorf("cannot complete eval: eval_requested with ID %s not found", p.EvalID)
				}
			}
		}

	case EventWorkHandoffAccepted, EventWorkHandoffRejected:
		if hasEvent != nil {
			var p struct {
				HandoffID HandoffID `json:"handoff_id"`
			}
			if err := json.Unmarshal(e.Payload, &p); err == nil && p.HandoffID != "" {
				key := EventLookupKey(EventWorkHandedOff, string(p.HandoffID))
				if !hasEvent(key) {
					return fmt.Errorf("cannot accept/reject handoff: handed_off with ID %s not found", p.HandoffID)
				}
			}
		}
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
		return true
	}
	return false
}

// findAttemptInSlice finds an attempt by ID in a slice.
func findAttemptInSlice(attempts []ExecutionAttempt, id AttemptID) *ExecutionAttempt {
	for i := range attempts {
		if attempts[i].AttemptID == id {
			return &attempts[i]
		}
	}
	return nil
}
