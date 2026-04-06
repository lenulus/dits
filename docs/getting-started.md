# Getting Started

## Build

```bash
git clone <repo>
cd pf
make build
```

This produces `bin/dits` (CLI) and `bin/dits-server` (server).

## Initialize a Project

```bash
mkdir my-project && cd my-project
dits init --project DEMO
```

This creates `.dits/` containing:
- `config.json` — project key, node ID, actor ID
- `identity.json` — Ed25519 keypair
- `dits.db` — SQLite database
- `blobs/` — content-addressed artifact store

## Create Work Items

```bash
# Simple task
dits work create --title "Set up CI pipeline"

# Issue with description
dits work create --title "Login fails on Safari" --kind issue --body "500 error on POST /auth"

# Execution work item
dits work create --title "Deploy v2.1" --kind execution
```

Default kind is `task`. Available kinds: `task`, `issue`, `investigation`, `plan`, `decision`, `execution`, `handoff`, `eval`.

## List and View

```bash
dits work list
dits work list --kind issue
dits work list --all        # include closed
dits work show DEMO-1
dits work show DEMO-1 --json
```

## Comments, Labels, Status

```bash
dits work comment DEMO-1 --body "Reproduced on Chrome too"

# Add a label (must exist in meta first)
dits meta label add --slug bug --name Bug --color "#ff0000"
dits work label-add DEMO-1 bug

# Change status
dits work status DEMO-1 in_progress
dits work close DEMO-1
dits work reopen DEMO-1
```

## Assignments and Relations

```bash
dits work assign DEMO-1 actor_abc123
dits work unassign DEMO-1 actor_abc123

dits work link DEMO-1 blocks DEMO-2
dits work unlink DEMO-1 blocks DEMO-2
```

## Artifacts

```bash
dits work attach DEMO-1 screenshot.png
dits work show DEMO-1    # artifacts listed in output
dits work detach DEMO-1 art_01...
```

## Coordination: Leasing and Execution

This is the agent-native workflow. An agent leases a work item, starts an execution attempt, checkpoints progress, and completes (or fails).

```bash
# Create an execution work item
dits work create --title "Run integration tests" --kind execution

# Lease it (prevents other actors from claiming)
dits work lease DEMO-1

# Start an attempt
dits work start DEMO-1

# Record progress
dits work checkpoint DEMO-1 --summary "Unit tests passed" --progress 0.5
dits work checkpoint DEMO-1 --summary "Integration tests passed" --progress 1.0

# Complete the attempt
dits work complete DEMO-1 --summary "All tests green"

# Or fail it (with retry signal)
# dits work fail DEMO-1 --error "Timeout on DB connection" --retryable

# Release the lease when done
dits work lease-release DEMO-1
```

## Evidence and Findings

For investigation work items, record observations and findings:

```bash
dits work create --title "Investigate memory spike" --kind investigation

dits work observe DEMO-2 --summary "RSS grew 500MB in 2 hours"
dits work observe DEMO-2 --summary "Correlates with batch job schedule"

dits work finding DEMO-2 --statement "Memory leak in batch processor" --confidence 0.8
```

## Blocking and Unblocking

```bash
dits work block DEMO-1 --reason "Waiting for database migration"
dits work unblock DEMO-1 --reason "Migration complete"
```

## Plans and Handoffs

```bash
dits work plan DEMO-1 --summary "Two-phase rollout" --plan "Phase 1: canary. Phase 2: full rollout."
dits work handoff DEMO-1 --to actor_bob --context "Ready for review"
dits work review DEMO-1 --scope "Code review of deploy script"
```

## Private Overlay (Local Only)

Annotations and private labels are local-only and never synced:

```bash
dits work annotate DEMO-1 priority-reason "Blocking release"
dits work annotations DEMO-1
dits work annotate-delete DEMO-1 priority-reason

dits work private-label-add DEMO-1 my-focus
dits work private-label-remove DEMO-1 my-focus
```

Note: These commands are available under both `dits work` and `dits issue`.

## Sync with a Server

Start the server:

```bash
dits-server --project DEMO --addr :8484 &
```

From the client:

```bash
dits remote set http://localhost:8484
dits sync
```

Sync pushes local events, pulls remote events, and transfers missing artifact blobs in both directions.

### Two-Client Sync

```bash
# Client A
mkdir client-a && cd client-a
dits init --project DEMO
dits work create --title "Task from A" --kind task
dits sync --server http://localhost:8484

# Client B
mkdir client-b && cd client-b
dits init --project DEMO
dits sync --server http://localhost:8484
dits work list    # sees A's task
dits work comment DEMO-1 --body "Got it!"
dits sync

# Client A syncs again
cd ../client-a
dits sync
dits work show DEMO-1    # sees B's comment
```

## Meta Configuration

View and modify the project's structured vocabulary:

```bash
dits meta show

# Labels
dits meta label add --slug feature --name Feature --color "#00ff00"
dits meta label list

# Work kinds
dits meta type add --slug epic --name Epic --workflow default
dits meta type list

# Workflows
dits meta workflow list
dits meta workflow show execution
```

## Query API

With the server running, query work items programmatically:

```bash
# All work items
curl http://localhost:8484/api/v2/work

# Ready for work (open, unleased, unblocked)
curl http://localhost:8484/api/v2/work?ready=true

# Events for a work item
curl http://localhost:8484/api/v2/work/DEMO-1/events

# Artifacts filtered by type
curl http://localhost:8484/api/v2/work/DEMO-1/artifacts?type=log

# Cross-item event stream
curl http://localhost:8484/api/v2/events?type=work.checkpointed
```

## Identity

```bash
dits identity show
```

Displays your actor ID (derived from Ed25519 public key), public key, and node ID.

## JSON Output

Most commands support `--json` for scripting:

```bash
dits work list --json
dits work show DEMO-1 --json
```
