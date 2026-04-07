# DITS + Claude Code integration

This directory contains the bits you need to drive DITS from a Claude Code
session:

- **`dits-mcp`** — a Model Context Protocol (MCP) server that exposes DITS
  work-item operations as tools. It lives in `cmd/dits-mcp/` and is built
  alongside `dits` and `dits-server` (`make build`).
- **`skills/`** — Claude Code skills that teach the agent *when* and *how*
  to use the MCP tools.
- **`agents/`** — purpose-built subagents (`dits-worker`, `dits-judge`,
  `dits-investigator`) with restricted tool surfaces.
- **`mcp.example.json`** — a ready-to-paste config snippet for your
  `~/.claude.json`.

For verification steps before tagging a release, see
[`docs/claude-code-verification.md`](../../docs/claude-code-verification.md).

## Install

### 1. Get the `dits-mcp` binary

Either download a prebuilt release for your platform from the project's
GitHub Releases page, or build it from source:

```sh
make build   # produces bin/dits, bin/dits-server, bin/dits-mcp
sudo install bin/dits-mcp /usr/local/bin/
```

Prebuilt releases ship the three binaries (`dits`, `dits-server`,
`dits-mcp`) for `linux/{amd64,arm64}`, `darwin/{amd64,arm64}`, and
`windows/{amd64,arm64}`.

### 2. Register the MCP server with Claude Code

Merge the contents of `mcp.example.json` into your `~/.claude.json`
(create the file if it doesn't exist). Replace the project path with
the DITS-tracked repository you want Claude to work in.

```json
{
  "mcpServers": {
    "dits": {
      "command": "dits-mcp",
      "args": ["--project", "/absolute/path/to/your/repo"]
    }
  }
}
```

### 3. Copy or symlink the skills and agents

```sh
mkdir -p ~/.claude/skills ~/.claude/agents
cp integrations/claude-code/skills/*.md ~/.claude/skills/
cp integrations/claude-code/agents/*.md ~/.claude/agents/
```

You can also keep them repo-local in `.claude/skills` and
`.claude/agents`.

## Operating model

`dits-mcp` works against a local `.dits/` checkout, exactly like `git`
works against a local repo. There is no "remote mode." Syncing against a
`dits-server` remote is an explicit action via the `dits_sync` tool, which
shares the `internal/workops.Sync` code path with `dits sync` on the CLI.

## Logging and debugging

When Claude Code's tool calls misbehave the MCP server's log file is the
fastest place to look. `dits-mcp` writes structured logs to a file by
default — never to stdout/stderr, because those are reserved for the
JSON-RPC protocol.

**Default log location:** `$XDG_STATE_HOME/dits/mcp.log` (or
`~/.dits/mcp.log` if `XDG_STATE_HOME` is unset).

**Flags** (set via the `args` field in `mcp.example.json`):

| Flag | Default | Notes |
|---|---|---|
| `--log-level` | `info` | One of `error`, `warn`, `info`, `debug`, `trace`. |
| `--log-format` | `json` | `json` (machine-friendly, `jq`-able) or `text`. |
| `--log-file` | _default path above_ | Absolute path; pass `-` to log to stderr (only safe outside of MCP/Claude Code). |

**Cranking up logging when something breaks:**

```json
{
  "mcpServers": {
    "dits": {
      "command": "dits-mcp",
      "args": [
        "--project", "/absolute/path/to/your/repo",
        "--log-level", "debug",
        "--log-file", "/tmp/dits-mcp.log"
      ]
    }
  }
}
```

Then `tail -f /tmp/dits-mcp.log` (optionally piped through `jq`) shows
every tool invocation in real time. Each call is bracketed by a `tool
start` and `tool ok`/`tool failed` line carrying the same `request_id`,
which is also threaded through the underlying `workops` event-emission
lines and (if a sync fires) the `dits-server` log. A single
`grep request_id=…` reconstructs an entire round-trip across all three.

The `dits` CLI and `dits-server` accept the same `--log-level` and
`--log-format` flags (the CLI logs to stderr by default; the server logs
to stdout). See [`docs/observability-plan.md`](../../docs/observability-plan.md)
for the field schema and level discipline.

## Tool surface

Read-only tools (marked with the MCP `readOnlyHint` annotation so Claude
Code's permission UI can auto-allow them):

`dits_work_list`, `dits_work_show`, `dits_work_events`, `dits_meta_show`,
`dits_identity_show`.

Mutating tools (gated by the usual permission prompt):

`dits_work_create`, `dits_work_lease`, `dits_work_lease_release`,
`dits_work_start`, `dits_work_checkpoint`, `dits_work_complete`,
`dits_work_fail`, `dits_work_observe`, `dits_work_finding`,
`dits_work_attach`, `dits_work_eval_request`, `dits_work_eval_complete`,
`dits_work_retain`, `dits_work_discard`, `dits_work_handoff`,
`dits_work_link`, `dits_work_status`, `dits_work_assign`,
`dits_work_comment`, `dits_work_block`, `dits_work_unblock`,
`dits_work_close`, `dits_work_reopen`, `dits_sync`.

All tool results are structured JSON.
