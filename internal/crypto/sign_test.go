package crypto

import (
	"testing"
	"time"

	"github.com/lenulus/pf/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSignAndVerify(t *testing.T) {
	id, err := GenerateIdentity()
	require.NoError(t, err)

	privKey, err := id.PrivKey()
	require.NoError(t, err)
	pubKey, err := id.PubKey()
	require.NoError(t, err)

	e := &domain.Event{
		ID:             "evt_test001",
		WorkItemID:     "wrk_test001",
		Type:           domain.EventWorkCreated,
		ParentEventIDs: nil,
		MetaVersion:    1,
		ActorID:        id.ActorID,
		Timestamp:      time.Date(2026, 3, 29, 10, 0, 0, 0, time.UTC),
		Payload:        domain.MustMarshalPayload(domain.WorkCreatedPayload{Title: "Test", Kind: "task"}),
	}

	// Sign.
	require.NoError(t, SignEvent(e, privKey))
	assert.NotEmpty(t, e.Signature)

	// Verify.
	valid, err := VerifyEvent(e, pubKey)
	require.NoError(t, err)
	assert.True(t, valid)

	// Tamper and verify fails.
	e.Payload = domain.MustMarshalPayload(domain.WorkCreatedPayload{Title: "Tampered", Kind: "task"})
	valid, err = VerifyEvent(e, pubKey)
	require.NoError(t, err)
	assert.False(t, valid)
}

func TestSignAndVerify_DifferentKey(t *testing.T) {
	id1, _ := GenerateIdentity()
	id2, _ := GenerateIdentity()

	privKey1, _ := id1.PrivKey()
	pubKey2, _ := id2.PubKey()

	e := &domain.Event{
		ID:         "evt_test002",
		WorkItemID: "wrk_test002",
		Type:       domain.EventWorkCommented,
		ActorID:    id1.ActorID,
		Timestamp:  time.Now().UTC(),
		Payload:    domain.MustMarshalPayload(domain.CommentPayload{Body: "Hello"}),
	}

	require.NoError(t, SignEvent(e, privKey1))

	// Wrong key should fail.
	valid, _ := VerifyEvent(e, pubKey2)
	assert.False(t, valid)
}

func TestCanonicalJSON_Deterministic(t *testing.T) {
	e := &domain.Event{
		ID:             "evt_test003",
		WorkItemID:     "wrk_test003",
		Type:           domain.EventWorkCreated,
		ParentEventIDs: []domain.EventID{"evt_parent1"},
		MetaVersion:    5,
		ActorID:        "actor_abc",
		Timestamp:      time.Date(2026, 1, 15, 8, 30, 0, 0, time.UTC),
		Payload:        domain.MustMarshalPayload(domain.WorkCreatedPayload{Title: "Test", Kind: "task"}),
	}

	json1, err := CanonicalEventJSON(e)
	require.NoError(t, err)

	json2, err := CanonicalEventJSON(e)
	require.NoError(t, err)

	assert.Equal(t, json1, json2, "canonical JSON should be deterministic")
}

func TestVerifyEvent_NoSignature(t *testing.T) {
	id, _ := GenerateIdentity()
	pubKey, _ := id.PubKey()

	e := &domain.Event{
		ID:         "evt_test004",
		WorkItemID: "wrk_test004",
		Type:       domain.EventWorkCreated,
		Payload:    domain.MustMarshalPayload(domain.WorkCreatedPayload{Title: "Test", Kind: "task"}),
	}

	_, err := VerifyEvent(e, pubKey)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no signature")
}

func TestGenerateIdentity(t *testing.T) {
	id, err := GenerateIdentity()
	require.NoError(t, err)
	assert.NotEmpty(t, id.ActorID)
	assert.NotEmpty(t, id.PublicKey)
	assert.NotEmpty(t, id.PrivateKey)
	assert.Contains(t, string(id.ActorID), "actor_")
}
