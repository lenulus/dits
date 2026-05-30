package signing

import (
	"encoding/json"
	"testing"
	"time"
)

// canonicalVector is the pinned canonical-JSON encoding of the shared vector
// event. It MUST be byte-for-byte identical to the literal asserted on the
// substrate side (internal/crypto/canonical_vector_test.go: canonicalVector).
// Pilot cannot import internal/crypto, so this pinned literal — verified equal
// on both sides — is the contract that a Pilot-signed event canonicalizes to
// exactly what DITS will verify. Keep the two literals in lockstep.
const canonicalVector = `{"actor_id":"actor_pilot_test","event_type":"work.commented","id":"evt_canon_vector","meta_version":7,"parent_event_ids":["evt_parent_a","evt_parent_b"],"payload":{"body":"canon vector"},"timestamp":"2026-05-30T12:34:56.789Z","work_item_id":"wrk_canon_vector"}`

// canonicalVectorEvent mirrors internal/crypto's canonicalVectorEvent in Pilot's
// independent Event type.
func canonicalVectorEvent() *Event {
	ts, _ := time.Parse(time.RFC3339Nano, "2026-05-30T12:34:56.789Z")
	return &Event{
		ID:             "evt_canon_vector",
		WorkItemID:     "wrk_canon_vector",
		Type:           "work.commented",
		ParentEventIDs: []string{"evt_parent_a", "evt_parent_b"},
		MetaVersion:    7,
		ActorID:        "actor_pilot_test",
		Timestamp:      ts,
		Payload:        json.RawMessage(`{"body":"canon vector"}`),
	}
}

// TestCanonicalAgreesWithSubstrate asserts Pilot's CanonicalEventJSON reproduces
// the shared vector. Because internal/crypto separately asserts it produces the
// same literal, this proves the two independent canonicalizations agree — a
// Pilot-signed event will verify against the actor's registered key in DITS.
func TestCanonicalAgreesWithSubstrate(t *testing.T) {
	got, err := CanonicalEventJSON(canonicalVectorEvent())
	if err != nil {
		t.Fatalf("CanonicalEventJSON: %v", err)
	}
	if string(got) != canonicalVector {
		t.Errorf("canonical mismatch:\n got %s\nwant %s", got, canonicalVector)
	}
}
