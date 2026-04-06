# CLI Reference

## Global

```
dits [command]
```

DITS is a local-first, event-sourced distributed coordination system.

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

## dits work

Primary command tree for managing work items.

### dits work create

```
dits work create --title <title> [flags]
```

| Flag | Short | Required | Default | Description |
|------|-------|----------|---------|-------------|
| `--title` | `-t` | Yes | | Work item title |
| `--body` | `-b` | No | | Description |
| `--label` | `-l` | No | | Labels (comma-separated or repeated) |
| `--kind` | `-k` | No | `task` | Work kind (task, issue, investigation, execution, plan, decision, handoff, eval) |

Kind is validated against meta config.

### dits work list

```
dits work list [flags]
```

Aliases: `ls`

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--status` | `-s` | | Filter by status |
| `--kind` | `-k` | | Filter by kind |
| `--all` | `-a` | `false` | Include closed work items |
| `--json` | | `false` | Output as JSON |

### dits work show

```
dits work show <id> [flags]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--json` | `false` | Output as JSON |

Accepts shared ID (`PROJ-1`) or canonical ID (`wrk_01...`). Displays full work item details including coordination state, artifacts, attempts, checkpoints, observations, findings, comments, and local overlay data.

### dits work comment

```
dits work comment <id> --body <text>
```

### dits work close

```
dits work close <id>
```

### dits work reopen

```
dits work reopen <id>
```

### dits work status

```
dits work status <id> <status>
```

Status is validated against the meta workflow. Available statuses depend on the work item's kind and its associated workflow.

### dits work label-add / label-remove

```
dits work label-add <id> <label-slug>
dits work label-remove <id> <label-slug>
```

### dits work assign / unassign

```
dits work assign <id> <actor-id>
dits work unassign <id> <actor-id>
```

### dits work link / unlink

```
dits work link <id> <relation-type> <target-id>
dits work unlink <id> <relation-type> <target-id>
```

Relation type is validated against meta config. Default types: `blocks`, `blocked_by`, `depends_on`, `parent_of`, `child_of`, `relates_to`, `duplicates`, `derived_from`, `supersedes`.

### dits work attach / detach

```
dits work attach <id> <file-path>
dits work detach <id> <artifact-id>
```

Computes SHA256 hash, stores blob locally, creates `artifact_added` event. Max file size: 50MB.

---

## Coordination Commands

### dits work lease

```
dits work lease <id>
```

Leases a work item. Duration determined by meta lease policy for the work kind (default 300s). Fails if already leased.

### dits work lease-release

```
dits work lease-release <id> [--reason <text>]
```

### dits work start

```
dits work start <id>
```

Starts a new execution attempt. Generates a unique AttemptID. Display-order attempt numbers are derived during materialization, not assigned by the client.

### dits work complete

```
dits work complete <id> [--summary <text>]
```

Completes the current execution attempt. Requires an active attempt.

### dits work fail

```
dits work fail <id> --error <msg> [--retryable]
```

Fails the current execution attempt. `--retryable` signals that a new attempt is appropriate.

### dits work checkpoint

```
dits work checkpoint <id> --summary <text> [--progress <float>]
```

Records a checkpoint on the current attempt. Progress is 0.0 - 1.0.

### dits work block / unblock

```
dits work block <id> --reason <text>
dits work unblock <id> [--reason <text>]
```

### dits work observe

```
dits work observe <id> --summary <text>
```

Records an observation (unstructured note about something noticed).

### dits work finding

```
dits work finding <id> --statement <text> [--confidence <float>]
```

Records a structured finding (epistemic assertion). Confidence is 0.0 - 1.0, default 0.5.

### dits work plan

```
dits work plan <id> --summary <text> --plan <text>
```

Proposes a plan for the work item.

### dits work handoff

```
dits work handoff <id> --to <actor-id> --context <text>
```

Hands off the work item to another actor.

### dits work review

```
dits work review <id> --scope <text>
```

Requests a review of the work item (human judgment).

### dits work eval-request

```
dits work eval-request <id> --scope <text> [--subject <ref>] [--subject-kind <kind>] [--rubric-ref <ref>]
```

Requests a machine evaluation. Subject ref can be a content hash, work item ID, or artifact ID. Subject kind: work_item, artifact, attempt, finding, plan.

### dits work eval-complete

```
dits work eval-complete <id> --eval-id <id> --verdict <pass|fail|partial> [--subject <ref>] [--subject-kind <kind>] [--rubric-ref <ref>] [--summary <text>] [--metrics <json>] [--metrics-file <path>]
```

Completes an eval with a machine-generated verdict. Metrics can be inline JSON or a file path.

### dits work retain

```
dits work retain <id> --subject <ref> --subject-kind <kind> [--reason <text>] [--eval-ref <eval-id>]
```

Marks an output (attempt, artifact) as the retained/accepted result. Optionally references the eval that informed this decision.

### dits work discard

```
dits work discard <id> --subject <ref> --subject-kind <kind> [--reason <text>]
```

Marks an output as discarded/superseded. If the discarded ref is the currently retained output, clears the retained ref.

---

## dits issue

Alias for `dits work` commands. `dits issue create` defaults to `--kind issue`. All subcommands from v1 are preserved: create, list, show, comment, close, reopen, status, label-add, label-remove, assign, unassign, link, unlink, attach, attachments, detach, annotate, annotations, annotate-delete, private-label-add, private-label-remove.

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

Displays project key, version, work kinds, workflows, labels, priorities, artifact types, evidence types, relation types, and policies.

### dits meta label add / remove / list

```
dits meta label add --slug <slug> [--name <name>] [--color <hex>]
dits meta label remove <slug>
dits meta label list
```

### dits meta type add / list

```
dits meta type add --slug <slug> [--name <name>] [--workflow <slug>]
dits meta type list
```

Adds/lists work kinds. Default workflow: `default`.

### dits meta workflow show / list

```
dits meta workflow show [slug]
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
