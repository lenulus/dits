# Getting Started with DITS

This guide walks through setting up a DITS project, creating and managing issues, and syncing between multiple clients.

## Prerequisites

- Go 1.21+
- Make

## Build

```bash
git clone https://github.com/lenulus/pf.git
cd pf
make build
```

This produces `bin/dits` (CLI) and `bin/dits-server` (sync server).

## 1. Initialize a Project

```bash
mkdir my-project && cd my-project
dits init --project DEMO
```

This creates a `.dits/` directory containing:
- `config.json` — project key, node ID, actor ID, server URL
- `identity.json` — Ed25519 keypair (private, do not share)
- `dits.db` — SQLite database (events, materialized issues, meta)

Check your identity:

```bash
dits identity show
```

## 2. Create Issues

```bash
dits issue create --title "Fix authentication" --body "Login fails on Safari"
dits issue create --title "Add dark mode" --type task
```

Issues get a shared ID (`DEMO-1`, `DEMO-2`) and a canonical ID (`iss_01...`). Both can be used to reference issues.

## 3. Work with Issues

```bash
# List open issues
dits issue list

# Show details
dits issue show DEMO-1

# Add a comment
dits issue comment DEMO-1 --body "Reproduced on Chrome too"

# Change status
dits issue status DEMO-1 in_progress

# Close and reopen
dits issue close DEMO-1
dits issue reopen DEMO-1
```

## 4. Labels and Metadata

Labels are defined in the project's meta configuration and validated when applied:

```bash
# Add labels to the project
dits meta label add --slug bug --name "Bug" --color "#ff0000"
dits meta label add --slug feature --name "Feature"

# Apply labels to issues
dits issue create --title "Fix crash" --label bug
dits issue label-add DEMO-1 feature
dits issue label-remove DEMO-1 bug

# View meta configuration
dits meta show
```

## 5. Relations and Assignments

```bash
# Link issues
dits issue link DEMO-2 blocks DEMO-1
dits issue unlink DEMO-2 blocks DEMO-1

# Assign
dits issue assign DEMO-1 actor_bob
dits issue unassign DEMO-1 actor_bob
```

## 6. Attachments

Files are stored in a content-addressed blob store. Metadata syncs with events; blobs sync separately.

```bash
# Attach a file
dits issue attach DEMO-1 screenshot.png

# List attachments
dits issue attachments DEMO-1

# Remove an attachment
dits issue detach DEMO-1 att_01...
```

## 7. Private Overlay

Annotations and private labels are local-only and never sync:

```bash
# Add a personal note
dits issue annotate DEMO-1 notes "discuss with Alice on Monday"

# Add a private label
dits issue private-label-add DEMO-1 my-sprint

# View (shows in `issue show` under "Local (not synced)")
dits issue show DEMO-1

# Clean up
dits issue annotate-delete DEMO-1 notes
dits issue private-label-remove DEMO-1 my-sprint
```

## 8. Syncing

### Start the server

```bash
dits-server --project DEMO --addr :8484
```

The server stores events, assigns shared IDs, and serves blobs. It uses SQLite by default.

### Configure and sync a client

```bash
dits remote set http://localhost:8484
dits sync
```

### Two-client workflow

```bash
# Terminal 1: Server
dits-server --project DEMO --addr :8484

# Terminal 2: Client A
mkdir clientA && cd clientA
dits init --project DEMO
dits remote set http://localhost:8484
dits issue create --title "From client A"
dits sync

# Terminal 3: Client B
mkdir clientB && cd clientB
dits init --project DEMO
dits remote set http://localhost:8484
dits sync                          # Gets A's issue
dits issue comment DEMO-1 --body "Seen from B"
dits sync                          # Pushes comment

# Back in Terminal 2
dits sync                          # Gets B's comment
dits issue show DEMO-1             # Shows the comment
```

## 9. JSON Output

For scripting and integrations:

```bash
dits issue list --json
dits issue show DEMO-1 --json
```

## 10. Server Query API

The server exposes a read-only JSON API:

```bash
# List issues
curl http://localhost:8484/api/v1/issues

# Filter by status
curl http://localhost:8484/api/v1/issues?status=open

# Get a single issue
curl http://localhost:8484/api/v1/issues/DEMO-1

# Get meta configuration
curl http://localhost:8484/api/v1/meta
```

## What's Next

- Read the [Architecture Guide](architecture.md) to understand how events, the DAG, and sync work
- See the [CLI Reference](cli-reference.md) for every command and flag
- Check the [API Reference](api-reference.md) for server endpoint details
- Review the [Specification](spec.md) for the formal protocol and schema definitions
