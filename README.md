# DITS — Distributed Coordination Substrate

DITS is a local-first, event-sourced distributed coordination system for humans and agents. Work items replace issues as the primary object. The event vocabulary covers lifecycle, execution, evidence, planning, and handoffs. Agents coordinate by leasing work, checkpointing progress, recording findings, and handing off between actors.

## Key Features

- **Work items, not tickets** — durable coordination objects with a `kind` (task, issue, investigation, execution, plan, decision, handoff, artifact_review)
- **Agent-native coordination** — first-class events for leasing work, checkpointing, recording evidence, proposing plans, and handing off
- **Local-first** — create and edit offline, sync when ready
- **Event-sourced** — every change is an immutable event in a DAG
- **Deterministic conflict resolution** — concurrent edits converge automatically
- **Structured provenance** — two-layer model: EmittedBy (who created the event) and ProducedBy (who produced the content)
- **Artifacts over attachments** — files have types, semantic roles, and producer metadata
- **Human-friendly IDs** — `PROJ-123` shared IDs alongside stable `wrk_<ULID>` canonical IDs
- **Ed25519 signatures** — every event cryptographically signed
- **Machine query surfaces** — `ready=true` answers "what should I do next?"

## Quick Start

```bash
# Build
make build

# Initialize a project
dits init --project MYPROJ

# Create work items
dits work create --title "Deploy service" --kind execution
dits work create --title "Fix login bug" --kind issue

# List and show
dits work list
dits work show MYPROJ-1

# Coordination: lease, execute, checkpoint, complete
dits work lease MYPROJ-1
dits work start MYPROJ-1
dits work checkpoint MYPROJ-1 --summary "Step 1 done" --progress 0.5
dits work complete MYPROJ-1

# Evidence and findings
dits work observe MYPROJ-2 --summary "CPU spike at 10:00"
dits work finding MYPROJ-2 --statement "Memory leak in service X" --confidence 0.8

# Sync with a server
dits-server --project MYPROJ --addr :8484 &
dits remote set http://localhost:8484
dits sync
```

## Architecture

DITS has four layers:

| Layer | Purpose | Synced? |
|-------|---------|---------|
| **Data Plane** | Append-only event DAG (work item history) | Yes |
| **Control Plane** | Versioned meta config (kinds, workflows, policies) | Yes |
| **Coordination** | Leases, attempts, checkpoints (materialized from events) | Yes |
| **Private Layer** | Local annotations and private labels | No |

Events form a DAG via parent references. Work items are materialized by reducing events in causal order (Kahn's algorithm). Conflicts resolve deterministically: causal order > timestamp > lexical event ID.

See [docs/architecture.md](docs/architecture.md) for full details.

## Documentation

- [Getting Started](docs/getting-started.md) — hands-on tutorial
- [Use Cases](docs/use-cases.md) — agent harness integration, human workflows, platform operations
- [Architecture](docs/architecture.md) — internals and design decisions
- [CLI Reference](docs/cli-reference.md) — every command and flag
- [API Reference](docs/api-reference.md) — server HTTP endpoints
- [Specification](docs/spec.md) — formal event schema, sync protocol, identity model

## Project Structure

```
cmd/
  dits/              CLI binary
  dits-server/       Server binary
internal/
  domain/            Core types, events, DAG, reducer, validation
  store/sqlite/      SQLite storage with migrations
  sync/              Sync protocol and engine
  server/            HTTP server (chi) with v2 query API
  blob/              Content-addressed blob store
  crypto/            Ed25519 signing and verification
  cli/               Cobra CLI commands (work + issue)
  project/           .dits/ directory management
```

## Building

```bash
make build          # Build both binaries to bin/
make test           # Run all tests
make vet            # Run go vet
```

Requires Go 1.21+ (uses `log/slog`). No CGo — SQLite via `modernc.org/sqlite`.

## License

See LICENSE file.
