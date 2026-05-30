package domain

import (
	"encoding/json"
	"fmt"
	"time"
)

// Reduce takes causally-ordered events and produces a materialized WorkItem.
// Events MUST be in causal order (use CausalOrder first).
//
// The reducer is deterministic: it applies all events without validation or
// rejection. Protocol validation (ValidateProtocol) catches invalid events
// before they enter the event log at creation time. The reducer handles
// concurrent offline divergence via the post-reduction resolveOperationalLineage
// pass, which determines authoritative lease lineage and derived attempt numbering.
//
// Three-tier validation model:
//   1. ValidateEvent — schema/meta references (stateless)
//   2. ValidateProtocol — coordination invariants (stateful, authoritative at creation)
//   3. Reduce/ApplyEvent — deterministic materialization (no rejection)
func Reduce(events []Event) (*WorkItem, error) {
	if len(events) == 0 {
		return nil, fmt.Errorf("no events to reduce")
	}

	wi := &WorkItem{
		Labels:       []string{},
		Assignees:    []ActorID{},
		Comments:     []Comment{},
		Artifacts:    []Artifact{},
		Relations:    []Relation{},
		Checkpoints:  []Checkpoint{},
		Observations: []Observation{},
		Findings:     []Finding{},
		Attempts:     []ExecutionAttempt{},
		Evals:        []Eval{},
		Outcomes:     []Outcome{},

		Classifications: []Classification{},
		RoleBindings:    []RoleBinding{},
		Diagnostics:     []Diagnostic{},
		Acks:            []Ack{},
	}

	for _, e := range events {
		if err := ApplyEvent(wi, e); err != nil {
			return nil, fmt.Errorf("applying event %s: %w", e.ID, err)
		}
	}

	// Post-reduction: compute operational lineage and attempt authority.
	resolveOperationalLineage(wi)

	wi.HeadEvents = Heads(events)
	wi.EventCount = len(events)
	return wi, nil
}

// ApplyEvent applies a single event to an existing work item state.
func ApplyEvent(wi *WorkItem, e Event) error {
	switch e.Type {

	// --- Lifecycle ---

	case EventWorkCreated:
		var p WorkCreatedPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		wi.ID = e.WorkItemID
		wi.Title = p.Title
		wi.Body = p.Body
		wi.Kind = p.Kind
		if wi.Kind == "" {
			wi.Kind = "task"
		}
		wi.Status = "open"
		wi.Priority = "medium"
		wi.CreatedBy = e.ActorID
		wi.CreatedAt = e.Timestamp
		wi.UpdatedAt = e.Timestamp

	case EventWorkTitleSet:
		var p TitleSetPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		wi.Title = p.Title
		wi.UpdatedAt = maxTime(wi.UpdatedAt, e.Timestamp)

	case EventWorkBodySet:
		var p BodySetPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		wi.Body = p.Body
		wi.UpdatedAt = maxTime(wi.UpdatedAt, e.Timestamp)

	case EventWorkStatusSet:
		var p StatusSetPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		wi.Status = p.To
		wi.UpdatedAt = maxTime(wi.UpdatedAt, e.Timestamp)

	case EventWorkPrioritySet:
		var p PrioritySetPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		wi.Priority = p.Priority
		wi.UpdatedAt = maxTime(wi.UpdatedAt, e.Timestamp)

	case EventWorkLabelAdded:
		var p LabelPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		if !containsString(wi.Labels, p.LabelSlug) {
			wi.Labels = append(wi.Labels, p.LabelSlug)
		}
		wi.UpdatedAt = maxTime(wi.UpdatedAt, e.Timestamp)

	case EventWorkLabelRemoved:
		var p LabelPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		wi.Labels = removeString(wi.Labels, p.LabelSlug)
		wi.UpdatedAt = maxTime(wi.UpdatedAt, e.Timestamp)

	case EventWorkAssigned:
		var p AssignPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		if !containsActor(wi.Assignees, p.Assignee) {
			wi.Assignees = append(wi.Assignees, p.Assignee)
		}
		wi.UpdatedAt = maxTime(wi.UpdatedAt, e.Timestamp)

	case EventWorkUnassigned:
		var p AssignPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		wi.Assignees = removeActor(wi.Assignees, p.Assignee)
		wi.UpdatedAt = maxTime(wi.UpdatedAt, e.Timestamp)

	case EventWorkCommented:
		var p CommentPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		wi.Comments = append(wi.Comments, Comment{
			EventID:    e.ID,
			ActorID:    e.ActorID,
			Body:       p.Body,
			ProducedBy: p.ProducedBy,
			Timestamp:  e.Timestamp,
		})
		wi.UpdatedAt = maxTime(wi.UpdatedAt, e.Timestamp)

	case EventWorkClosed:
		var p ClosedPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		wi.Status = "closed"
		t := e.Timestamp
		wi.ClosedAt = &t
		wi.UpdatedAt = maxTime(wi.UpdatedAt, e.Timestamp)

	case EventWorkReopened:
		wi.Status = "open"
		wi.ClosedAt = nil
		wi.UpdatedAt = maxTime(wi.UpdatedAt, e.Timestamp)

	// --- Execution / Ownership ---

	case EventWorkLeased:
		var p LeasedPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		holder := e.ActorID
		wi.LeaseHolder = &holder
		wi.LeaseExpiresAt = &p.LeaseExpiresAt
		wi.LeaseID = &p.LeaseID
		wi.LeaseGeneration = p.Generation
		wi.UpdatedAt = maxTime(wi.UpdatedAt, e.Timestamp)

	case EventWorkLeaseReleased:
		var p LeaseReleasedPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		wi.LeaseHolder = nil
		wi.LeaseExpiresAt = nil
		wi.LeaseID = nil
		wi.LeaseGeneration = 0
		wi.UpdatedAt = maxTime(wi.UpdatedAt, e.Timestamp)

	case EventWorkLeaseRenewed:
		var p LeaseRenewedPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		wi.LeaseExpiresAt = &p.LeaseExpiresAt
		wi.LeaseGeneration = p.Generation
		wi.UpdatedAt = maxTime(wi.UpdatedAt, e.Timestamp)

	case EventWorkExecutionStarted:
		var p ExecutionStartedPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		attempt := ExecutionAttempt{
			AttemptID: p.AttemptID,
			Number:    0, // Derived during resolveOperationalLineage; payload number is advisory.
			ActorID:   e.ActorID,
			StartedAt: e.Timestamp,
			Status:    "running",
		}
		wi.Attempts = append(wi.Attempts, attempt)
		wi.CurrentAttempt = &p.AttemptID
		wi.UpdatedAt = maxTime(wi.UpdatedAt, e.Timestamp)

	case EventWorkExecutionCompleted:
		var p ExecutionCompletedPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		if a := findAttempt(wi.Attempts, p.AttemptID); a != nil {
			t := e.Timestamp
			a.CompletedAt = &t
			a.Status = "completed"
		}
		wi.UpdatedAt = maxTime(wi.UpdatedAt, e.Timestamp)

	case EventWorkExecutionFailed:
		var p ExecutionFailedPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		if a := findAttempt(wi.Attempts, p.AttemptID); a != nil {
			t := e.Timestamp
			a.CompletedAt = &t
			a.Status = "failed"
		}
		wi.UpdatedAt = maxTime(wi.UpdatedAt, e.Timestamp)

	case EventWorkExecutionAbandoned:
		var p ExecutionAbandonedPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		if a := findAttempt(wi.Attempts, p.AttemptID); a != nil {
			t := e.Timestamp
			a.CompletedAt = &t
			a.Status = "abandoned"
		}
		wi.UpdatedAt = maxTime(wi.UpdatedAt, e.Timestamp)

	// --- Checkpoint / Progress ---

	case EventWorkCheckpointed:
		var p CheckpointedPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		cp := Checkpoint{
			EventID:   e.ID,
			AttemptID: p.AttemptID,
			Summary:   p.Summary,
			Progress:  p.Progress,
			NextStep:  p.NextStep,
			Data:      p.Data,
			Timestamp: e.Timestamp,
		}
		wi.Checkpoints = append(wi.Checkpoints, cp)
		if a := findAttempt(wi.Attempts, p.AttemptID); a != nil {
			eid := e.ID
			a.LastCheckpoint = &eid
		}
		wi.UpdatedAt = maxTime(wi.UpdatedAt, e.Timestamp)

	case EventWorkProgressReported:
		// Streaming event — not materialized as sub-entity.
		var p ProgressReportedPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		wi.UpdatedAt = maxTime(wi.UpdatedAt, e.Timestamp)

	case EventWorkBlocked:
		var p BlockedPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		wi.Blocked = true
		wi.BlockedReason = p.Reason
		wi.UpdatedAt = maxTime(wi.UpdatedAt, e.Timestamp)

	case EventWorkUnblocked:
		var p UnblockedPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		wi.Blocked = false
		wi.BlockedReason = ""
		wi.UpdatedAt = maxTime(wi.UpdatedAt, e.Timestamp)

	// --- Evidence / Observation ---

	case EventWorkObservationRecorded:
		var p ObservationRecordedPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		wi.Observations = append(wi.Observations, Observation{
			EventID:    e.ID,
			Summary:    p.Summary,
			Data:       p.Data,
			ProducedBy: p.ProducedBy,
			Timestamp:  e.Timestamp,
		})
		wi.UpdatedAt = maxTime(wi.UpdatedAt, e.Timestamp)

	case EventWorkEvidenceAttached:
		var p EvidenceAttachedPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		if !containsArtifact(wi.Artifacts, p.ArtifactID) {
			wi.Artifacts = append(wi.Artifacts, Artifact{
				ID:           p.ArtifactID,
				ContentHash:  p.ContentHash,
				Filename:     p.Filename,
				MimeType:     p.MimeType,
				SizeBytes:    p.SizeBytes,
				ArtifactType: p.ArtifactType,
				SemanticRole: p.SemanticRole,
				ProducedBy:   p.ProducedBy,
				AddedBy:      e.ActorID,
				AddedAt:      e.Timestamp,
			})
		}
		wi.UpdatedAt = maxTime(wi.UpdatedAt, e.Timestamp)

	case EventWorkFindingRecorded:
		var p FindingRecordedPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		wi.Findings = append(wi.Findings, Finding{
			EventID:      e.ID,
			Statement:    p.Statement,
			Confidence:   p.Confidence,
			Source:       p.Source,
			EvidenceRefs: p.EvidenceRefs,
			ProducedBy:   p.ProducedBy,
			Timestamp:    e.Timestamp,
		})
		wi.UpdatedAt = maxTime(wi.UpdatedAt, e.Timestamp)

	case EventWorkFindingRetracted:
		var p FindingRetractedPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		if f := findFindingByEventID(wi.Findings, p.OriginalEventID); f != nil {
			f.Retracted = true
		}
		wi.UpdatedAt = maxTime(wi.UpdatedAt, e.Timestamp)

	// --- Planning / Decision ---
	// These events are recorded in the DAG but not materialized into sub-entity
	// collections. They update UpdatedAt only.

	case EventWorkPlanProposed:
		var p PlanProposedPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		wi.UpdatedAt = maxTime(wi.UpdatedAt, e.Timestamp)

	case EventWorkPlanAccepted:
		var p PlanAcceptedPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		wi.UpdatedAt = maxTime(wi.UpdatedAt, e.Timestamp)

	case EventWorkPlanRejected:
		var p PlanRejectedPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		wi.UpdatedAt = maxTime(wi.UpdatedAt, e.Timestamp)

	case EventWorkStepAdded:
		var p StepAddedPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		wi.UpdatedAt = maxTime(wi.UpdatedAt, e.Timestamp)

	case EventWorkDecisionRecorded:
		var p DecisionRecordedPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		wi.UpdatedAt = maxTime(wi.UpdatedAt, e.Timestamp)

	// --- Handoff / Review ---
	// These events are recorded in the DAG but not materialized into sub-entity
	// collections. They update UpdatedAt only.

	case EventWorkReviewRequested:
		var p ReviewRequestedPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		wi.UpdatedAt = maxTime(wi.UpdatedAt, e.Timestamp)

	case EventWorkReviewCompleted:
		var p ReviewCompletedPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		wi.UpdatedAt = maxTime(wi.UpdatedAt, e.Timestamp)

	case EventWorkHandedOff:
		var p HandedOffPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		wi.UpdatedAt = maxTime(wi.UpdatedAt, e.Timestamp)

	case EventWorkHandoffAccepted:
		var p HandoffAcceptedPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		wi.UpdatedAt = maxTime(wi.UpdatedAt, e.Timestamp)

	case EventWorkHandoffRejected:
		var p HandoffRejectedPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		wi.UpdatedAt = maxTime(wi.UpdatedAt, e.Timestamp)

	// --- Eval ---

	case EventWorkEvalRequested:
		var p EvalRequestedPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		wi.UpdatedAt = maxTime(wi.UpdatedAt, e.Timestamp)

	case EventWorkEvalCompleted:
		var p EvalCompletedPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		wi.Evals = append(wi.Evals, Eval{
			EvalID:      p.EvalID,
			EventID:     e.ID,
			SubjectKind: p.SubjectKind,
			SubjectRef:  p.SubjectRef,
			RubricRef:   p.RubricRef,
			Summary:     p.Summary,
			Metrics:     p.Metrics,
			Verdict:     p.Verdict,
			ProducedBy:  p.ProducedBy,
			Timestamp:   e.Timestamp,
		})
		wi.UpdatedAt = maxTime(wi.UpdatedAt, e.Timestamp)

	// --- Outcome ---

	case EventWorkOutcomeRetained:
		var p OutcomeRetainedPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		wi.Outcomes = append(wi.Outcomes, Outcome{
			EventID: e.ID, SubjectKind: p.SubjectKind, SubjectRef: p.SubjectRef,
			Decision: "retained", Reason: p.Reason, EvalRef: p.EvalRef,
			ActorID: e.ActorID, Timestamp: e.Timestamp,
		})
		wi.RetainedOutcomeRef = &p.SubjectRef
		wi.UpdatedAt = maxTime(wi.UpdatedAt, e.Timestamp)

	case EventWorkOutcomeDiscarded:
		var p OutcomeDiscardedPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		wi.Outcomes = append(wi.Outcomes, Outcome{
			EventID: e.ID, SubjectKind: p.SubjectKind, SubjectRef: p.SubjectRef,
			Decision: "discarded", Reason: p.Reason,
			ActorID: e.ActorID, Timestamp: e.Timestamp,
		})
		// If the discarded ref is the currently retained one, clear it.
		if wi.RetainedOutcomeRef != nil && *wi.RetainedOutcomeRef == p.SubjectRef {
			wi.RetainedOutcomeRef = nil
		}
		wi.UpdatedAt = maxTime(wi.UpdatedAt, e.Timestamp)

	// --- Relation / Artifact ---

	case EventWorkLinked:
		var p RelationPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		rel := Relation{Type: p.RelationType, TargetWorkItem: p.TargetWorkItem}
		if !containsRelation(wi.Relations, rel) {
			wi.Relations = append(wi.Relations, rel)
		}
		wi.UpdatedAt = maxTime(wi.UpdatedAt, e.Timestamp)

	case EventWorkUnlinked:
		var p RelationPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		wi.Relations = removeRelation(wi.Relations, Relation{Type: p.RelationType, TargetWorkItem: p.TargetWorkItem})
		wi.UpdatedAt = maxTime(wi.UpdatedAt, e.Timestamp)

	case EventWorkArtifactAdded:
		var p ArtifactAddedPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		if !containsArtifact(wi.Artifacts, p.ArtifactID) {
			wi.Artifacts = append(wi.Artifacts, Artifact{
				ID:           p.ArtifactID,
				ContentHash:  p.ContentHash,
				Filename:     p.Filename,
				MimeType:     p.MimeType,
				SizeBytes:    p.SizeBytes,
				ArtifactType: p.ArtifactType,
				SemanticRole: p.SemanticRole,
				ProducedBy:   p.ProducedBy,
				AddedBy:      e.ActorID,
				AddedAt:      e.Timestamp,
			})
		}
		wi.UpdatedAt = maxTime(wi.UpdatedAt, e.Timestamp)

	case EventWorkArtifactRemoved:
		var p ArtifactRemovedPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		wi.Artifacts = removeArtifact(wi.Artifacts, p.ArtifactID)
		wi.UpdatedAt = maxTime(wi.UpdatedAt, e.Timestamp)

	// --- Classification / Roles (RE substrate) ---

	case EventWorkClassified:
		var p ClassificationPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		c := Classification{TaxonomySlug: p.TaxonomySlug, NodeSlug: p.NodeSlug}
		if !containsClassification(wi.Classifications, c) {
			wi.Classifications = append(wi.Classifications, c)
		}
		wi.UpdatedAt = maxTime(wi.UpdatedAt, e.Timestamp)

	case EventWorkDeclassified:
		var p ClassificationPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		wi.Classifications = removeClassification(wi.Classifications,
			Classification{TaxonomySlug: p.TaxonomySlug, NodeSlug: p.NodeSlug})
		wi.UpdatedAt = maxTime(wi.UpdatedAt, e.Timestamp)

	case EventWorkRoleBound:
		var p RoleBindingPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		// Re-binding the same role replaces the actor; this models cardinality
		// at materialization without rejecting the event.
		if rb := findRoleBinding(wi.RoleBindings, p.RoleSlug); rb != nil {
			rb.Actor = p.Actor
		} else {
			wi.RoleBindings = append(wi.RoleBindings, RoleBinding{RoleSlug: p.RoleSlug, Actor: p.Actor})
		}
		wi.UpdatedAt = maxTime(wi.UpdatedAt, e.Timestamp)

	case EventWorkRoleUnbound:
		var p RoleBindingPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		wi.RoleBindings = removeRoleBinding(wi.RoleBindings, p.RoleSlug, p.Actor)
		wi.UpdatedAt = maxTime(wi.UpdatedAt, e.Timestamp)

	// --- ACK lifecycle ---

	case EventWorkAckFiled:
		var p AckFiledPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		wi.Acks = append(wi.Acks, Ack{
			AckID:              p.AckID,
			ScopeSummary:       p.ScopeSummary,
			DeliveryTiming:     p.DeliveryTiming,
			TargetOutcome:      p.TargetOutcome,
			AcceptanceCriteria: p.AcceptanceCriteria,
			Specifier:          AckPending,
			Builder:            AckPending,
			Amendments:         []AckAmendment{},
			FiledBy:            e.ActorID,
			FiledAt:            e.Timestamp,
		})
		wi.UpdatedAt = maxTime(wi.UpdatedAt, e.Timestamp)

	case EventWorkAckAccepted:
		var p AckSidePayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		if a := currentAck(wi); a != nil {
			setAckSide(a, p.Who, AckAccepted)
		}
		wi.UpdatedAt = maxTime(wi.UpdatedAt, e.Timestamp)

	case EventWorkAckRejected:
		var p AckSidePayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		if a := currentAck(wi); a != nil {
			setAckSide(a, p.Who, AckRejected)
		}
		wi.UpdatedAt = maxTime(wi.UpdatedAt, e.Timestamp)

	case EventWorkAckCleared:
		var p AckClearedPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		if a := currentAck(wi); a != nil {
			if p.Who == AckSideBoth || p.Who == "" {
				a.Specifier = AckPending
				a.Builder = AckPending
			} else {
				setAckSide(a, p.Who, AckPending)
			}
		}
		wi.UpdatedAt = maxTime(wi.UpdatedAt, e.Timestamp)

	case EventWorkAckAmended:
		var p AckAmendedPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		if a := currentAck(wi); a != nil {
			a.Amendments = append(a.Amendments, AckAmendment{
				Type:      p.AmendmentType,
				Fields:    p.Fields,
				Reason:    p.Reason,
				ActorID:   e.ActorID,
				Timestamp: e.Timestamp,
			})
		}
		wi.UpdatedAt = maxTime(wi.UpdatedAt, e.Timestamp)
	}

	return nil
}

// currentAck returns the most recently filed Ack, or nil if none exists.
// accept/reject/clear/amend events apply to it.
func currentAck(wi *WorkItem) *Ack {
	if len(wi.Acks) == 0 {
		return nil
	}
	return &wi.Acks[len(wi.Acks)-1]
}

// setAckSide sets the specifier or builder side of an ack to the given state.
func setAckSide(a *Ack, who string, state AckState) {
	switch who {
	case AckSideSpecifier:
		a.Specifier = state
	case AckSideBuilder:
		a.Builder = state
	}
}

// --- Operational Lineage ---

// resolveOperationalLineage computes which execution attempts belong to the
// winning lease lineage and marks them as authoritative. Attempts started by
// the current lease holder are authoritative. All others are orphaned
// (preserved in history but do not drive live operational state).
//
// This also recomputes CurrentAttempt to only reference authoritative attempts,
// and derives attempt numbers from authoritative ordering.
func resolveOperationalLineage(wi *WorkItem) {
	if len(wi.Attempts) == 0 {
		return
	}

	// If there's no lease holder (released or never leased), all attempts
	// that existed before lease release are authoritative.
	// If there is a lease holder, only attempts by that actor are authoritative.
	var authNumber uint32
	var latestAuthRunning *AttemptID

	for i := range wi.Attempts {
		a := &wi.Attempts[i]

		if wi.LeaseHolder == nil {
			// No active lease — all attempts are authoritative.
			a.Authoritative = true
		} else {
			// Active lease — only attempts by the lease holder are authoritative.
			a.Authoritative = (a.ActorID == *wi.LeaseHolder)
		}

		if a.Authoritative {
			authNumber++
			a.Number = authNumber

			if a.Status == "running" {
				id := a.AttemptID
				latestAuthRunning = &id
			}
		} else {
			// Non-authoritative attempts keep their original number for display
			// but do not affect operational state.
			a.Number = 0
		}
	}

	// CurrentAttempt only points to the latest authoritative running attempt.
	wi.CurrentAttempt = latestAuthRunning
}

// --- Helpers ---

func maxTime(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

func containsString(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func removeString(s []string, v string) []string {
	result := make([]string, 0, len(s))
	for _, x := range s {
		if x != v {
			result = append(result, x)
		}
	}
	return result
}

func containsActor(s []ActorID, v ActorID) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func removeActor(s []ActorID, v ActorID) []ActorID {
	result := make([]ActorID, 0, len(s))
	for _, x := range s {
		if x != v {
			result = append(result, x)
		}
	}
	return result
}

func containsArtifact(s []Artifact, id ArtifactID) bool {
	for _, x := range s {
		if x.ID == id {
			return true
		}
	}
	return false
}

func removeArtifact(s []Artifact, id ArtifactID) []Artifact {
	result := make([]Artifact, 0, len(s))
	for _, x := range s {
		if x.ID != id {
			result = append(result, x)
		}
	}
	return result
}

func containsRelation(s []Relation, r Relation) bool {
	for _, x := range s {
		if x.Type == r.Type && x.TargetWorkItem == r.TargetWorkItem {
			return true
		}
	}
	return false
}

func removeRelation(s []Relation, r Relation) []Relation {
	result := make([]Relation, 0, len(s))
	for _, x := range s {
		if !(x.Type == r.Type && x.TargetWorkItem == r.TargetWorkItem) {
			result = append(result, x)
		}
	}
	return result
}

func containsClassification(s []Classification, c Classification) bool {
	for _, x := range s {
		if x.TaxonomySlug == c.TaxonomySlug && x.NodeSlug == c.NodeSlug {
			return true
		}
	}
	return false
}

func removeClassification(s []Classification, c Classification) []Classification {
	result := make([]Classification, 0, len(s))
	for _, x := range s {
		if !(x.TaxonomySlug == c.TaxonomySlug && x.NodeSlug == c.NodeSlug) {
			result = append(result, x)
		}
	}
	return result
}

func findRoleBinding(s []RoleBinding, roleSlug string) *RoleBinding {
	for i := range s {
		if s[i].RoleSlug == roleSlug {
			return &s[i]
		}
	}
	return nil
}

func removeRoleBinding(s []RoleBinding, roleSlug string, actor ActorID) []RoleBinding {
	result := make([]RoleBinding, 0, len(s))
	for _, x := range s {
		if !(x.RoleSlug == roleSlug && x.Actor == actor) {
			result = append(result, x)
		}
	}
	return result
}

func findAttempt(attempts []ExecutionAttempt, id AttemptID) *ExecutionAttempt {
	for i := range attempts {
		if attempts[i].AttemptID == id {
			return &attempts[i]
		}
	}
	return nil
}

func findFindingByEventID(findings []Finding, eventID EventID) *Finding {
	for i := range findings {
		if findings[i].EventID == eventID {
			return &findings[i]
		}
	}
	return nil
}
