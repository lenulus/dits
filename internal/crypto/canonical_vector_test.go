package crypto

import (
	"testing"
	"time"

	"github.com/lenulus/pf/internal/domain"
)

// canonicalVector is the pinned canonical-JSON encoding of canonicalVectorEvent.
// It is the shared contract between this package and Pilot's independent
// reimplementation (internal/pilot/signing.CanonicalEventJSON), which Pilot
// cannot import. Both sides assert they reproduce THIS exact byte string, so a
// Pilot-signed event verifies against internal/crypto. If the canonical format
// ever changes, update this literal here AND in
// internal/pilot/signing/canonical_test.go in lockstep.
const canonicalVector = `{"actor_id":"actor_pilot_test","event_type":"work.commented","id":"evt_canon_vector","meta_version":7,"parent_event_ids":["evt_parent_a","evt_parent_b"],"payload":{"body":"canon vector"},"timestamp":"2026-05-30T12:34:56.789Z","work_item_id":"wrk_canon_vector"}`

// canonicalVectorEvent builds the fixed event the vector pins. Pilot mirrors
// this event shape in its own test.
func canonicalVectorEvent() *domain.Event {
	ts, _ := time.Parse(time.RFC3339Nano, "2026-05-30T12:34:56.789Z")
	return &domain.Event{
		ID:             "evt_canon_vector",
		WorkItemID:     "wrk_canon_vector",
		Type:           domain.EventWorkCommented,
		ParentEventIDs: []domain.EventID{"evt_parent_a", "evt_parent_b"},
		MetaVersion:    7,
		ActorID:        "actor_pilot_test",
		Timestamp:      ts,
		Payload:        domain.MustMarshalPayload(domain.CommentPayload{Body: "canon vector"}),
	}
}

// TestCanonicalEventJSONVector pins internal/crypto's canonicalization to the
// shared vector. Its Pilot-side twin asserts the same literal, proving the two
// independent implementations agree without sharing code.
func TestCanonicalEventJSONVector(t *testing.T) {
	got, err := CanonicalEventJSON(canonicalVectorEvent())
	if err != nil {
		t.Fatalf("CanonicalEventJSON: %v", err)
	}
	if string(got) != canonicalVector {
		t.Errorf("canonical mismatch:\n got %s\nwant %s", got, canonicalVector)
	}
}
