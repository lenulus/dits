package domain

import (
	"crypto/rand"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"
)

// WorkItemID is a globally unique, client-generated work item identifier.
// Format: wrk_<ULID>
type WorkItemID string

// EventID is a globally unique, client-generated event identifier.
// Format: evt_<ULID>
type EventID string

// SharedID is a server-assigned, human-friendly work item identifier.
// Format: PROJ-<N>
type SharedID string

// ActorID identifies a user/agent in the system.
// Derived from Ed25519 public key.
type ActorID string

// MetaVersion is a monotonically increasing version for meta configuration.
type MetaVersion uint64

// NodeID identifies a DITS instance (client or server).
type NodeID string

// ArtifactID is a globally unique, client-generated artifact identifier.
// Format: art_<ULID>
type ArtifactID string

// LeaseID is a globally unique, client-generated lease identifier.
// Format: lea_<ULID>
type LeaseID string

// AttemptID is a globally unique, client-generated execution attempt identifier.
// Format: atp_<ULID>
type AttemptID string

// ReviewID is a globally unique, client-generated review identifier.
// Format: rev_<ULID>
type ReviewID string

// HandoffID is a globally unique, client-generated handoff identifier.
// Format: hof_<ULID>
type HandoffID string

var (
	entropyMu sync.Mutex
	entropy   = ulid.Monotonic(rand.Reader, 0)
)

func newULID() ulid.ULID {
	entropyMu.Lock()
	defer entropyMu.Unlock()
	id, err := ulid.New(ulid.Timestamp(time.Now()), entropy)
	if err != nil {
		// Monotonic entropy exhausted; reset.
		entropy = ulid.Monotonic(rand.Reader, 0)
		id = ulid.MustNew(ulid.Timestamp(time.Now()), entropy)
	}
	return id
}

func NewWorkItemID() WorkItemID {
	return WorkItemID(fmt.Sprintf("wrk_%s", newULID().String()))
}

func NewEventID() EventID {
	return EventID(fmt.Sprintf("evt_%s", newULID().String()))
}

func NewNodeID() NodeID {
	return NodeID(fmt.Sprintf("node_%s", newULID().String()))
}

func NewActorID(source io.Reader) ActorID {
	id := ulid.MustNew(ulid.Timestamp(time.Now()), source)
	return ActorID(fmt.Sprintf("actor_%s", id.String()))
}

func NewArtifactID() ArtifactID {
	return ArtifactID(fmt.Sprintf("art_%s", newULID().String()))
}

func NewLeaseID() LeaseID {
	return LeaseID(fmt.Sprintf("lea_%s", newULID().String()))
}

func NewAttemptID() AttemptID {
	return AttemptID(fmt.Sprintf("atp_%s", newULID().String()))
}

func NewReviewID() ReviewID {
	return ReviewID(fmt.Sprintf("rev_%s", newULID().String()))
}

func NewHandoffID() HandoffID {
	return HandoffID(fmt.Sprintf("hof_%s", newULID().String()))
}

func (id WorkItemID) String() string { return string(id) }
func (id EventID) String() string    { return string(id) }
func (id SharedID) String() string   { return string(id) }
func (id ActorID) String() string    { return string(id) }
func (id NodeID) String() string     { return string(id) }
func (id ArtifactID) String() string { return string(id) }
func (id LeaseID) String() string    { return string(id) }
func (id AttemptID) String() string  { return string(id) }
func (id ReviewID) String() string   { return string(id) }
func (id HandoffID) String() string  { return string(id) }
