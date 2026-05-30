package sqlite_test

import (
	"context"
	"testing"

	"github.com/lenulus/pf/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUpsertWorkItem_RESubstrateCollections pins that the RE substrate / ACK
// collections (Classifications, RoleBindings, Acks) survive a store
// round-trip. This regression test exists because Phase 1/2 only asserted on
// the in-memory reduce result, so the missing persistence path was invisible
// until the Phase 3 read tools surfaced empty collections.
func TestUpsertWorkItem_RESubstrateCollections(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	wi := &domain.WorkItem{
		ID:     "wrk_re",
		Kind:   "milestone",
		Title:  "recurring billing",
		Status: "open",
		Classifications: []domain.Classification{
			{TaxonomySlug: "product", NodeSlug: "commerce/checkout/payments"},
			{TaxonomySlug: "org", NodeSlug: "acme/platform/payments-team"},
		},
		RoleBindings: []domain.RoleBinding{
			{RoleSlug: "specifier", Actor: "actor_spec"},
			{RoleSlug: "builder", Actor: "actor_build"},
		},
		Acks: []domain.Ack{{
			AckID:        "ack_1",
			ScopeSummary: "ship v1",
			Specifier:    domain.AckAccepted,
			Builder:      domain.AckPending,
		}},
	}
	require.NoError(t, db.UpsertWorkItem(ctx, wi))

	got, err := db.GetWorkItem(ctx, "wrk_re")
	require.NoError(t, err)
	require.NotNil(t, got)

	require.Len(t, got.Classifications, 2)
	assert.Equal(t, "product", got.Classifications[0].TaxonomySlug)
	assert.Equal(t, "commerce/checkout/payments", got.Classifications[0].NodeSlug)

	require.Len(t, got.RoleBindings, 2)
	assert.Equal(t, "specifier", got.RoleBindings[0].RoleSlug)
	assert.Equal(t, domain.ActorID("actor_build"), got.RoleBindings[1].Actor)

	require.Len(t, got.Acks, 1)
	assert.Equal(t, "ack_1", got.Acks[0].AckID)
	assert.Equal(t, domain.AckAccepted, got.Acks[0].Specifier)
	assert.Equal(t, domain.RollupBuilderPending, got.Acks[0].Rollup())
}

// TestUpsertWorkItem_EmptyRECollectionsRoundTrip pins that a work item with no
// RE collections reads back as empty (non-nil) slices, not garbage.
func TestUpsertWorkItem_EmptyRECollectionsRoundTrip(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	wi := &domain.WorkItem{ID: "wrk_plain", Kind: "task", Title: "plain", Status: "open"}
	require.NoError(t, db.UpsertWorkItem(ctx, wi))

	got, err := db.GetWorkItem(ctx, "wrk_plain")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Empty(t, got.Acks)
	assert.Empty(t, got.Classifications)
	assert.Empty(t, got.RoleBindings)
}
