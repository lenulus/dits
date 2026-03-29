package crypto

import (
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/lenulus/pf/internal/domain"
)

// SignEvent signs an event using the given private key.
// The signature covers the canonical JSON of the event (all fields except signature, sorted keys).
func SignEvent(e *domain.Event, privKey ed25519.PrivateKey) error {
	canonical, err := CanonicalEventJSON(e)
	if err != nil {
		return fmt.Errorf("computing canonical JSON: %w", err)
	}
	e.Signature = ed25519.Sign(privKey, canonical)
	return nil
}

// VerifyEvent verifies an event's signature using the given public key.
func VerifyEvent(e *domain.Event, pubKey ed25519.PublicKey) (bool, error) {
	if len(e.Signature) == 0 {
		return false, fmt.Errorf("event has no signature")
	}
	canonical, err := CanonicalEventJSON(e)
	if err != nil {
		return false, fmt.Errorf("computing canonical JSON: %w", err)
	}
	return ed25519.Verify(pubKey, canonical, e.Signature), nil
}

// CanonicalEventJSON produces a deterministic JSON representation of an event
// for signing/verification. Excludes the signature field, sorts keys.
func CanonicalEventJSON(e *domain.Event) ([]byte, error) {
	// Build a map with sorted keys, excluding signature.
	m := map[string]any{
		"actor_id":         string(e.ActorID),
		"event_type":       string(e.Type),
		"id":               string(e.ID),
		"issue_id":         string(e.IssueID),
		"meta_version":     e.MetaVersion,
		"parent_event_ids": e.ParentEventIDs,
		"payload":          json.RawMessage(e.Payload),
		"timestamp":        e.Timestamp.UTC().Format("2006-01-02T15:04:05.999999999Z"),
	}

	// Sort keys for deterministic output.
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Build ordered JSON manually for determinism.
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
