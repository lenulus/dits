# CLI Reference

## Global

```
dits [command]
```

DITS is a local-first, event-sourced distributed issue tracker.

---

## dits init

Initialize a new DITS project in the current directory.

```
dits init --project <KEY>
```

| Flag | Short | Required | Description |
|------|-------|----------|-------------|
| `--project` | `-p` | Yes | Project key (e.g., MYPROJ). Uppercased automatically. |

Creates `.dits/` with `config.json`, `identity.json` (Ed25519 keypair), `dits.db`, and default meta config.

---

## dits issue

### dits issue create

```
dits issue create --title <title> [flags]
```

| Flag | Short | Required | Default | Description |
|------|-------|----------|---------|-------------|
| `--title` | `-t` | Yes | | Issue title |
| `--body` | `-b` | No | | Issue description |
| `--label` | `-l` | No | | Labels (comma-separated or repeated) |
| `--type` | | No | `task` | Issue type slug |

Labels are validated against meta config. Type must exist in meta.

### dits issue list

```
dits issue list [flags]
```

Aliases: `ls`

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--status` | `-s` | | Filter by status |
| `--all` | `-a` | `false` | Include closed issues |
| `--json` | | `false` | Output as JSON |

### dits issue show

```
dits issue show <issue-id> [flags]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--json` | `false` | Output as JSON |

Accepts shared ID (`PROJ-1`) or canonical ID (`iss_01...`). Displays issue details, labels, assignees, relations, attachments, comments, and local overlay data.

### dits issue comment

```
dits issue comment <issue-id> --body <text>
```

| Flag | Short | Required | Description |
|------|-------|----------|-------------|
| `--body` | `-b` | Yes | Comment text |

### dits issue close

```
dits issue close <issue-id>
```

### dits issue reopen

```
dits issue reopen <issue-id>
```

### dits issue status

```
dits issue status <issue-id> <status>
```

Status is validated against the meta workflow. Available statuses by default: `open`, `in_progress`, `closed`.

### dits issue label-add

```
dits issue label-add <issue-id> <label-slug>
```

Label must exist in meta config.

### dits issue label-remove

```
dits issue label-remove <issue-id> <label-slug>
```

### dits issue assign

```
dits issue assign <issue-id> <actor-id>
```

### dits issue unassign

```
dits issue unassign <issue-id> <actor-id>
```

### dits issue link

```
dits issue link <issue-id> <relation-type> <target-issue-id>
```

Relation type is freeform (e.g., `blocks`, `relates_to`, `duplicates`).

### dits issue unlink

```
dits issue unlink <issue-id> <relation-type> <target-issue-id>
```

### dits issue attach

```
dits issue attach <issue-id> <file-path>
```

Computes SHA256 hash, stores blob locally, creates `attachment_added` event. Max file size: 50MB.

### dits issue attachments

```
dits issue attachments <issue-id>
```

Lists all attachments with ID, filename, size, MIME type, and content hash.

### dits issue detach

```
dits issue detach <issue-id> <attachment-id>
```

### dits issue annotate

```
dits issue annotate <issue-id> <key> <value>
```

Sets a local annotation (key-value pair). Not synced.

### dits issue annotations

```
dits issue annotations <issue-id>
```

### dits issue annotate-delete

```
dits issue annotate-delete <issue-id> <key>
```

### dits issue private-label-add

```
dits issue private-label-add <issue-id> <label>
```

Private labels are local-only and not synced. No validation against meta.

### dits issue private-label-remove

```
dits issue private-label-remove <issue-id> <label>
```

---

## dits sync

```
dits sync [flags]
```

| Flag | Short | Description |
|------|-------|-------------|
| `--server` | `-s` | Server URL (overrides config) |

Pushes local events to server, pulls remote events, transfers missing blobs in both directions. On first use with `--server`, saves the URL to config.

---

## dits remote

### dits remote set

```
dits remote set <url>
```

### dits remote show

```
dits remote show
```

---

## dits meta

### dits meta show

```
dits meta show
```

Displays project key, version, labels, workflows, issue types, and priorities.

### dits meta label add

```
dits meta label add --slug <slug> [flags]
```

| Flag | Required | Description |
|------|----------|-------------|
| `--slug` | Yes | Label identifier |
| `--name` | No | Display name (defaults to slug) |
| `--color` | No | Hex color code |

### dits meta label remove

```
dits meta label remove <slug>
```

### dits meta label list

```
dits meta label list
```

Aliases: `ls`

### dits meta type add

```
dits meta type add --slug <slug> [flags]
```

| Flag | Required | Default | Description |
|------|----------|---------|-------------|
| `--slug` | Yes | | Type identifier |
| `--name` | No | slug | Display name |
| `--workflow` | No | `default` | Associated workflow |

### dits meta type list

```
dits meta type list
```

### dits meta workflow show

```
dits meta workflow show [slug]
```

Defaults to `default` workflow. Shows statuses and transitions.

### dits meta workflow list

```
dits meta workflow list
```

---

## dits identity

### dits identity show

```
dits identity show
```

Displays actor ID, public key, and node ID.

---

## dits-server

```
dits-server [flags]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--addr` | `:8484` | Listen address |
| `--db` | `dits-server.db` | SQLite database path |
| `--blobs` | `./blobs` | Blob storage directory |
| `--project` | (required) | Project key |

Graceful shutdown on SIGINT/SIGTERM with 10-second drain timeout.
