package domain

import (
	"testing"
	"time"
)

func TestComputeAckRollup(t *testing.T) {
	cases := []struct {
		s, b AckState
		want AckRollup
	}{
		{AckAccepted, AckAccepted, RollupAligned},
		{AckPending, AckPending, RollupBothPending},
		{AckAccepted, AckPending, RollupBuilderPending},
		{AckPending, AckAccepted, RollupSpecifierPending},
		{AckRejected, AckAccepted, RollupRejected},
		{AckAccepted, AckRejected, RollupRejected},
		{AckRejected, AckPending, RollupRejected},
	}
	for _, c := range cases {
		if got := ComputeAckRollup(c.s, c.b); got != c.want {
			t.Errorf("ComputeAckRollup(%s,%s) = %s, want %s", c.s, c.b, got, c.want)
		}
	}
}

func TestIsMaterialAmendment(t *testing.T) {
	for _, m := range []string{AmendmentScopeChange, AmendmentTimelineChange, AmendmentTargetChange} {
		if !IsMaterialAmendment(m) {
			t.Errorf("%s should be material", m)
		}
	}
	if IsMaterialAmendment(AmendmentClarification) {
		t.Error("clarification should not be material")
	}
}

// ackEvent builds a chained ACK event for reducer tests.
func ackEvent(wiID WorkItemID, typ EventType, payload any, parents []EventID, ts time.Time) Event {
	return Event{
		ID:             NewEventID(),
		WorkItemID:     wiID,
		Type:           typ,
		ParentEventIDs: parents,
		ActorID:        "actor_test",
		Timestamp:      ts,
		Payload:        MustMarshalPayload(payload),
	}
}

// TestReduce_AckFiledAndAccept pins the reducer materialization: file then
// both sides accept yields an aligned commitment.
func TestReduce_AckFiledAndAccept(t *testing.T) {
	wiID := WorkItemID("wrk_ack")
	t0 := time.Now().UTC()
	created := ackEvent(wiID, EventWorkCreated, WorkCreatedPayload{Title: "m", Kind: "milestone"}, nil, t0)
	filed := ackEvent(wiID, EventWorkAckFiled, AckFiledPayload{AckID: "ack_1", ScopeSummary: "ship it"}, []EventID{created.ID}, t0.Add(time.Minute))
	acceptB := ackEvent(wiID, EventWorkAckAccepted, AckSidePayload{Who: AckSideBuilder}, []EventID{filed.ID}, t0.Add(2*time.Minute))
	acceptS := ackEvent(wiID, EventWorkAckAccepted, AckSidePayload{Who: AckSideSpecifier}, []EventID{acceptB.ID}, t0.Add(3*time.Minute))

	wi, err := Reduce([]Event{created, filed, acceptB, acceptS})
	if err != nil {
		t.Fatalf("Reduce: %v", err)
	}
	if len(wi.Acks) != 1 {
		t.Fatalf("expected 1 ack, got %d", len(wi.Acks))
	}
	a := wi.Acks[0]
	if a.ScopeSummary != "ship it" {
		t.Errorf("scope summary not materialized: %q", a.ScopeSummary)
	}
	if a.Rollup() != RollupAligned {
		t.Errorf("rollup = %s, want aligned", a.Rollup())
	}
}

// TestReduce_AckClearedResetsSides pins that an ack_cleared(both) event resets
// both sides to pending during reduction.
func TestReduce_AckClearedResetsSides(t *testing.T) {
	wiID := WorkItemID("wrk_clear")
	t0 := time.Now().UTC()
	created := ackEvent(wiID, EventWorkCreated, WorkCreatedPayload{Title: "m", Kind: "milestone"}, nil, t0)
	filed := ackEvent(wiID, EventWorkAckFiled, AckFiledPayload{AckID: "ack_1"}, []EventID{created.ID}, t0.Add(time.Minute))
	acceptB := ackEvent(wiID, EventWorkAckAccepted, AckSidePayload{Who: AckSideBuilder}, []EventID{filed.ID}, t0.Add(2*time.Minute))
	acceptS := ackEvent(wiID, EventWorkAckAccepted, AckSidePayload{Who: AckSideSpecifier}, []EventID{acceptB.ID}, t0.Add(3*time.Minute))
	amended := ackEvent(wiID, EventWorkAckAmended, AckAmendedPayload{AmendmentType: AmendmentScopeChange}, []EventID{acceptS.ID}, t0.Add(4*time.Minute))
	cleared := ackEvent(wiID, EventWorkAckCleared, AckClearedPayload{Who: AckSideBoth, Reason: "material_amendment"}, []EventID{amended.ID}, t0.Add(5*time.Minute))

	wi, err := Reduce([]Event{created, filed, acceptB, acceptS, amended, cleared})
	if err != nil {
		t.Fatalf("Reduce: %v", err)
	}
	a := wi.Acks[0]
	if a.Rollup() != RollupBothPending {
		t.Errorf("rollup after clear = %s, want both_pending", a.Rollup())
	}
	if len(a.Amendments) != 1 || a.Amendments[0].Type != AmendmentScopeChange {
		t.Errorf("expected one scope_change amendment recorded, got %+v", a.Amendments)
	}
}

func TestValidateEvent_Ack(t *testing.T) {
	meta := &MetaConfig{Version: 1}
	wiID := WorkItemID("wrk")
	t0 := time.Now().UTC()

	bad := ackEvent(wiID, EventWorkAckAccepted, AckSidePayload{Who: "pilot"}, nil, t0)
	if err := ValidateEvent(bad, meta); err == nil {
		t.Error("expected error for invalid ack side 'pilot'")
	}
	good := ackEvent(wiID, EventWorkAckAccepted, AckSidePayload{Who: AckSideBuilder}, nil, t0)
	if err := ValidateEvent(good, meta); err != nil {
		t.Errorf("unexpected error for valid ack side: %v", err)
	}
	badAmd := ackEvent(wiID, EventWorkAckAmended, AckAmendedPayload{AmendmentType: "rename"}, nil, t0)
	if err := ValidateEvent(badAmd, meta); err == nil {
		t.Error("expected error for invalid amendment type")
	}
	clearBoth := ackEvent(wiID, EventWorkAckCleared, AckClearedPayload{Who: AckSideBoth}, nil, t0)
	if err := ValidateEvent(clearBoth, meta); err != nil {
		t.Errorf("ack_cleared(both) should be valid: %v", err)
	}
}
