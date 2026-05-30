package signing

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

// Event is Pilot's minimal mirror of the DITS domain.Event, carrying exactly
// the fields that participate in the signature. Pilot cannot import
// internal/domain, so this is an independent type whose JSON shape matches what
// dits_event_submit unmarshals back into domain.Event (snake_case tags, see
// internal/domain/event.go).
//
// Only the fields CanonicalEventJSON covers are signature-relevant; the rest
// (none here) would be cosmetic. EmittedBy is modelled as an opaque
// json.RawMessage so Pilot can round-trip it without re-declaring its shape.
type Event struct {
	ID             string          `json:"id"`
	WorkItemID     string          `json:"work_item_id"`
	Type           string          `json:"type"`
	ParentEventIDs []string        `json:"parent_event_ids"`
	MetaVersion    uint64          `json:"meta_version"`
	ActorID        string          `json:"actor_id"`
	Timestamp      time.Time       `json:"timestamp"`
	Payload        json.RawMessage `json:"payload"`
	EmittedBy      json.RawMessage `json:"emitted_by,omitempty"`
	Signature      []byte          `json:"signature,omitempty"`
}

// CanonicalEventJSON produces the deterministic JSON an event is signed over.
// It is a byte-for-byte reimplementation of internal/crypto.CanonicalEventJSON
// (Pilot may not import internal/crypto). The two encodings MUST agree or a
// Pilot-signed event would fail dits_event_submit's verification.
//
// Invariants mirrored from the substrate:
//   - excludes the signature field;
//   - renames type→"event_type";
//   - sorts top-level keys;
//   - formats the timestamp as UTC "2006-01-02T15:04:05.999999999Z";
//   - omits emitted_by when nil.
//
// A canonicalization-agreement test lives in canonical_test.go (pinned to a
// literal that internal/crypto separately asserts it reproduces).
func CanonicalEventJSON(e *Event) ([]byte, error) {
	m := map[string]any{
		"actor_id":         e.ActorID,
		"event_type":       e.Type,
		"id":               e.ID,
		"work_item_id":     e.WorkItemID,
		"meta_version":     e.MetaVersion,
		"parent_event_ids": e.ParentEventIDs,
		"payload":          json.RawMessage(e.Payload),
		"timestamp":        e.Timestamp.UTC().Format("2006-01-02T15:04:05.999999999Z"),
	}
	if len(e.EmittedBy) > 0 {
		m["emitted_by"] = json.RawMessage(e.EmittedBy)
	}

	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	buf := []byte("{")
	for i, k := range keys {
		if i > 0 {
			buf = append(buf, ',')
		}
		keyJSON, _ := json.Marshal(k)
		buf = append(buf, keyJSON...)
		buf = append(buf, ':')

		valJSON, err := json.Marshal(m[k])
		if err != nil {
			return nil, fmt.Errorf("marshaling %s: %w", k, err)
		}
		buf = append(buf, valJSON...)
	}
	buf = append(buf, '}')
	return buf, nil
}
