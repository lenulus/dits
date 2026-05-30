package domain

import (
	"crypto/rand"
	"encoding/hex"
)

// ACK is a generic two-party commitment lifecycle: a Specifier files a
// commitment and the Specifier and Builder each independently stand behind it
// (or reject it). The shape is not methodology-specific — any methodology that
// wants a "commitment + bilateral acceptance" flow can use these events. The
// methodology decides which work kinds support ACK and what counts as a
// material amendment; DITS core owns only the lifecycle semantics below.

// AckState is one side's stance on a filed commitment. Values are stored as
// the prototype's lowercase tokens so they serialise cleanly to JSON for any
// frontend (see design-prototype views.jsx).
type AckState string

const (
	AckPending  AckState = "pending"
	AckAccepted AckState = "accepted"
	AckRejected AckState = "rejected"
)

// AckSide names which party an accept/reject/clear applies to. "both" is used
// by the auto-clear emitted on a material amendment.
const (
	AckSideSpecifier = "specifier"
	AckSideBuilder   = "builder"
	AckSideBoth      = "both"
)

// AckRollup is the derived alignment of a commitment's two sides. Tokens match
// the prototype's rollup vocabulary and the Portfolio "alignment" filter.
type AckRollup string

const (
	RollupAligned          AckRollup = "aligned"
	RollupBuilderPending   AckRollup = "builder_pending"
	RollupSpecifierPending AckRollup = "specifier_pending"
	RollupBothPending      AckRollup = "both_pending"
	RollupRejected         AckRollup = "rejected"
)

// ComputeAckRollup derives the alignment of a commitment from its two sides.
// Pure and deterministic; mirrors the prototype's ackRollup (views.jsx:61–68).
func ComputeAckRollup(specifier, builder AckState) AckRollup {
	if specifier == AckRejected || builder == AckRejected {
		return RollupRejected
	}
	if specifier == AckAccepted && builder == AckAccepted {
		return RollupAligned
	}
	if specifier != AckAccepted && builder != AckAccepted {
		return RollupBothPending
	}
	if specifier != AckAccepted {
		return RollupSpecifierPending
	}
	return RollupBuilderPending
}

// Amendment types carried by work.ack_amended.
const (
	AmendmentScopeChange    = "scope_change"
	AmendmentTimelineChange = "timeline_change"
	AmendmentTargetChange   = "target_change"
	AmendmentClarification  = "clarification"
)

// IsMaterialAmendment reports whether an amendment of this type clears a
// commitment once either side has accepted. A clarification never clears.
func IsMaterialAmendment(amendmentType string) bool {
	switch amendmentType {
	case AmendmentScopeChange, AmendmentTimelineChange, AmendmentTargetChange:
		return true
	default:
		return false
	}
}

// IsValidAmendmentType reports whether the amendment type is one of the four
// recognised kinds. Used by tier-1 validation.
func IsValidAmendmentType(amendmentType string) bool {
	switch amendmentType {
	case AmendmentScopeChange, AmendmentTimelineChange, AmendmentTargetChange, AmendmentClarification:
		return true
	default:
		return false
	}
}

// NewAckID returns a fresh opaque identifier for a filed commitment.
func NewAckID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return "ack_" + hex.EncodeToString(b[:])
}
