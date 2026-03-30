# DITS — Distributed Issue Tracking System

DITS is a local-first, event-sourced distributed issue tracker. It combines append-only history, coordinated identity, schema evolution, and causal correctness into a system that works offline and syncs through a lightweight server.

## Key Features

- **Local-first** — create and edit issues offline, sync when ready
- **Event-sourced** — every change is an immutable event in a DAG
- **Deterministic conflict resolution** — concurrent edits converge automatically
- **Human-friendly IDs** — `PROJ-123` shared IDs alongside stable canonical IDs
- **Structured metadata** — versioned labels, workflows, issue types with validation
- **Content-addressed attachments** — files stored separately from event history, deduped by SHA256
- **Ed25519 signatures** — every event cryptographically signed
- **Private overlay** — local annotations and labels that never sync
- **JSON output** — `--json` flag for scripting and integrations

## Quick Start

```bash
# Build
make build

# Initialize a project
dits init --project MYPROJ

# Create issues
dits issue create --title "Fix login bug" --body "Fails on Safari"
dits issue create --title "Add dark mode" --label feature

# List and show
dits issue list
dits issue show MYPROJ-1

# Comment, close, reopen
dits issue comment MYPROJ-1 --body "Reproduced on Chrome"
dits issue close MYPROJ-1
dits issue reopen MYPROJ-1

# Sync with a server
dits-server --project MYPROJ --addr :8484 &
dits remote set http://localhost:8484
dits sync
```

## Architecture

DITS has three layers:

| Layer | Purpose | Synced? |
|-------|---------|---------|
| **Data Plane** | Append-only event DAG (issue history) | Yes |
| **Control Plane** | Versioned meta config (labels, workflows) | Yes |
| **Private Layer** | Local annotations and private labels | No |

Events form a DAG via parent references. Issues are materialized by reducing events in causal order (Kahn's algorithm). Conflicts resolve deterministically: causal order > timestamp > lexical event ID.

See [docs/architecture.md](docs/architecture.md) for full details.

## Documentation

- [Getting Started](docs/getting-started.md) — hands-on tutorial
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
  server/            HTTP server (chi)
  blob/              Content-addressed blob store
  crypto/            Ed25519 signing and verification
  cli/               Cobra CLI commands
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
