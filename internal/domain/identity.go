package domain

import (
	"crypto/rand"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"
)

// CanonicalID is a globally unique, client-generated issue identifier.
// Format: iss_<ULID>
type CanonicalID string

// EventID is a globally unique, client-generated event identifier.
// Format: evt_<ULID>
type EventID string

// SharedID is a server-assigned, human-friendly issue identifier.
// Format: PROJ-1423
type SharedID string

// ActorID identifies a user/agent in the system.
type ActorID string

// MetaVersion is a monotonically increasing version for meta configuration.
type MetaVersion uint64

// NodeID identifies a DITS instance (client or server).
type NodeID string

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

func NewCanonicalID() CanonicalID {
	return CanonicalID(fmt.Sprintf("iss_%s", newULID().String()))
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

func (id CanonicalID) String() string { return string(id) }
func (id EventID) String() string     { return string(id) }
func (id SharedID) String() string    { return string(id) }
func (id ActorID) String() string     { return string(id) }
func (id NodeID) String() string      { return string(id) }
