// Package mcp is Pilot's typed client wrapper over the DITS MCP tool
// surface. It is the ONLY channel through which Pilot reaches the DITS
// substrate (see implementation-plan-v2 §2 topology and §8.1). Methods are
// named after the Pilot operations they back, each mapping to one MCP tool
// call.
//
// Phase-4 skeleton: an interface plus a stub implementation whose methods
// return ErrNotImplemented / zero values. No real MCP transport yet.
package mcp

import (
	"context"
	"errors"
)

// ErrNotImplemented is returned by the skeleton client until the real MCP
// transport lands.
var ErrNotImplemented = errors.New("pilot/mcp: not implemented")

// WorkItem is a minimal placeholder for a DITS work item as seen over MCP.
// TODO(phase 5): replace with the typed projection of the MCP work payload.
type WorkItem struct {
	ID    string
	Title string
}

// Indicators is a placeholder for the four RE indicator values returned by
// the indicators_get tool (or computed Pilot-side; see §10.3).
// TODO(phase 6): flesh out per projections package.
type Indicators struct{}

// Client is the typed surface Pilot code uses to talk to DITS over MCP.
// Every method corresponds to a single MCP tool call. The set mirrors the
// Pilot operations enumerated in the plan (§7 MCP surface, §8.2 identity).
type Client interface {
	// Work item queries.
	WorkList(ctx context.Context) ([]WorkItem, error)
	WorkGet(ctx context.Context, id string) (WorkItem, error)

	// Classification and role binding (Phase 3 substrate primitives).
	Classify(ctx context.Context, id, taxonomy, value string) error
	BindRole(ctx context.Context, id, role, actor string) error

	// ACK lifecycle (Phase 2 events).
	AckFile(ctx context.Context, id, role string) error
	AckAccept(ctx context.Context, id, role string) error

	// Indicators / projections.
	IndicatorsGet(ctx context.Context) (Indicators, error)

	// Meta bundle application (Phase 6 first-run).
	MetaApply(ctx context.Context, json []byte) error

	// Identity bridge — register a custodial/self-sovereign public key.
	ActorRegister(ctx context.Context, actor string, pubKey []byte) error
}

// stubClient is the Phase-4 no-op implementation of Client.
type stubClient struct{}

// NewStub returns a Client whose methods are not yet wired to MCP.
// TODO(phase 5): replace with a constructor that dials the dits-mcp
// endpoint (stdio or HTTP) and performs the MCP handshake.
func NewStub() Client { return stubClient{} }

func (stubClient) WorkList(context.Context) ([]WorkItem, error) {
	return nil, ErrNotImplemented
}

func (stubClient) WorkGet(context.Context, string) (WorkItem, error) {
	return WorkItem{}, ErrNotImplemented
}

func (stubClient) Classify(context.Context, string, string, string) error {
	return ErrNotImplemented
}

func (stubClient) BindRole(context.Context, string, string, string) error {
	return ErrNotImplemented
}

func (stubClient) AckFile(context.Context, string, string) error {
	return ErrNotImplemented
}

func (stubClient) AckAccept(context.Context, string, string) error {
	return ErrNotImplemented
}

func (stubClient) IndicatorsGet(context.Context) (Indicators, error) {
	return Indicators{}, ErrNotImplemented
}

func (stubClient) MetaApply(context.Context, []byte) error {
	return ErrNotImplemented
}

func (stubClient) ActorRegister(context.Context, string, []byte) error {
	return ErrNotImplemented
}
