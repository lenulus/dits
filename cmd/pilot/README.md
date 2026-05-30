# Pilot — the Radical Execution web frontend for DITS

Pilot is a reference frontend for the **Radical Execution (RE)** methodology
on the DITS substrate. It is a separate binary that talks to DITS **only over
MCP** — it never imports DITS-internal packages (enforced by
`TestForbiddenImports`). See
[`docs/proposals/radical-execution-implementation-plan-v2.md`](../../docs/proposals/radical-execution-implementation-plan-v2.md)
for the design and
[`docs/proposals/re-implementation-status.md`](../../docs/proposals/re-implementation-status.md)
for what's implemented vs. deferred.

```
browser ──HTTP──▶ pilot ──stdio MCP──▶ dits-mcp ──in-proc──▶ DITS substrate
```

## Build

```bash
make build          # builds dits, dits-server, dits-mcp, and pilot into ./bin
# or just Pilot:
make build-pilot
```

This README assumes `./bin` is on your `PATH` (or prefix the commands with
`./bin/`).

## Run (end-to-end)

### 1. Create a DITS project

`dits init` takes the **project key** (`-p`) and creates a `.dits/` in the
current directory:

```bash
mkdir my-portfolio && cd my-portfolio
dits init -p ACME
```

### 2. Apply the RE methodology bundle

`pilot init` loads the embedded RE meta bundle (work kinds, workflows, roles,
role constraints, taxonomy skeletons) into the project via the
`dits_meta_apply` MCP tool. Idempotent. `--project` here is the **path** to the
project directory:

```bash
pilot init --project /path/to/my-portfolio
```

> Naming note: `dits init -p ACME` takes the project **key**; every
> `--project` flag on `pilot` and `dits-mcp` is the project **directory path**.

### 3. Seed some work (optional)

```bash
cd /path/to/my-portfolio
dits work create --kind milestone --title "Recurring billing v1"
dits work create --kind milestone --title "Customer audit log export"
dits work status ACME-1 in_flight
dits work status ACME-2 shipped
```

Classification, role bindings, and ACKs are RE-specific and are emitted via
the MCP tools (`dits_work_classify`, `dits_work_role_bind`, `dits_ack_file`,
…) rather than the base CLI.

### 4. Serve the UI

```bash
pilot serve --project /path/to/my-portfolio --addr :7070
```

Open <http://localhost:7070> — it lands on **For you** (the Attention view).

Run the RE scheduler alongside the server (auto-creates outcome assessments
for shipped milestones, marks past-SLA assessments overdue, escalates idle
DecisionBlocks):

```bash
pilot serve --project /path/to/my-portfolio --schedule --schedule-interval 60s
```

Without `--project`, `pilot serve` uses a stub MCP client and renders empty
state — useful for previewing the chrome without a live substrate.

## Routes (§9.2)

| Section | Routes |
|---|---|
| Workspace | `/for-you` (default), `/leadership`, `/portfolio`, `/ack`, `/rfcs`, `/log` |
| Methodology | `/decisions`, `/outcomes` |
| Substrate | `/roles`, `/taxonomies`, `/events` |
| External | `/roadmap` (public, unauthenticated — committed milestones only) |

## Flags

**`pilot serve`**

| Flag | Default | Meaning |
|---|---|---|
| `--addr` | `:7070` | HTTP listen address |
| `--project` | _(empty)_ | DITS project directory; empty → stub client (empty state) |
| `--mcp-command` | `dits-mcp` | command that launches the DITS MCP server |
| `--schedule` | `false` | run the RE scheduler in-process |
| `--schedule-interval` | `60s` | scheduler cycle interval |

**`pilot init`** — `--project` (required), `--mcp-command`.

## Status & limits

Read path (all 12 views over live MCP data) and the methodology automation
(scheduler, four leading indicators) are implemented and verified end-to-end.
Still stubbed / deferred (see the status doc): OAuth + custodial signing
(needs a `dits_event_submit` tool), the inline-mutation write path, and the
`review_request` MCP tool. For now, mutations route through `dits-mcp` and are
signed as the project actor.
