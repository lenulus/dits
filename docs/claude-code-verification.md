# Claude Code Integration — Verification Plan

This document captures the verification steps for the `dits-mcp` ↔ Claude
Code integration. Steps marked **automated** run in CI / `go test`. Steps
marked **manual** require a real Claude Code session and are run before a
release tag.

## 1. Build (automated)

```
make build
```

**Pass criteria:** `bin/dits`, `bin/dits-server`, and `bin/dits-mcp` all
produced without errors.

## 2. Unit + smoke tests (automated)

```
go test ./...
```

**Pass criteria:** all packages green. The load-bearing test is
`internal/mcp/server_test.go`, which:

- Spins up an in-process `mcp-go` client against a temp DITS project.
- Exercises `initialize` → `tools/call` for `dits_work_list` (empty),
  `dits_work_create`, `dits_work_list` (non-empty), `dits_meta_show`,
  and the `dits_sync` no-remote error path.
- Confirms read-only annotations are carried through to `tools/list`.

If this test fails, the MCP wiring is broken regardless of what the
binary does manually.

## 3. Cross-platform release artifacts (automated)

```
make release
ls dist/
```

**Pass criteria:** six archives — `linux-{amd64,arm64}`,
`darwin-{amd64,arm64}` as `.tar.gz`, `windows-{amd64,arm64}` as `.zip` —
plus a `SHA256SUMS` file. Each archive contains all three binaries
(`dits`, `dits-server`, `dits-mcp`).

The same target runs in `.github/workflows/release.yml` on `v*` tag
push and uploads the artifacts as release assets.

## 4. Local MCP smoke test (manual, ~2 minutes)

Drive `dits-mcp` directly over stdio to confirm the binary works without
involving Claude Code.

```bash
TMP=$(mktemp -d) && cd "$TMP"
dits init --project SMOKE
dits work create --title "smoke item" --kind task

{
  printf '%s\n' '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"smoke","version":"0.0.1"}}}'
  printf '%s\n' '{"jsonrpc":"2.0","method":"notifications/initialized"}'
  printf '%s\n' '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"dits_work_list","arguments":{"ready":true}}}'
  printf '%s\n' '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"dits_meta_show","arguments":{}}}'
} | dits-mcp --project "$TMP"
```

**Pass criteria:** the `id:1` response carries `serverInfo.name = "dits-mcp"`,
the `id:2` response includes the `SMOKE-1` item, and `id:3` returns the
full meta config JSON.

Alternative: `npx @modelcontextprotocol/inspector dits-mcp --project $(pwd)`
for an interactive UI.

## 5. Sync round-trip (manual, ~3 minutes)

Verify the only network-touching tool, `dits_sync`, completes a full
push/pull cycle against a local `dits-server`.

```bash
# terminal 1
dits-server -addr 127.0.0.1:18765 \
            -db /tmp/dits-verify.db \
            -blobs /tmp/dits-verify-blobs \
            -project SMOKE \
            -signature-mode ignore

# terminal 2
TMP=$(mktemp -d) && cd "$TMP"
dits init --project SMOKE
dits work create --title "sync test" --kind task
dits remote set http://127.0.0.1:18765

{
  printf '%s\n' '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"verify","version":"0.0.1"}}}'
  printf '%s\n' '{"jsonrpc":"2.0","method":"notifications/initialized"}'
  printf '%s\n' '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"dits_sync","arguments":{}}}'
} | dits-mcp --project "$TMP"
```

**Pass criteria:** the `dits_sync` response is a JSON `SyncResult` with
`pushed_events ≥ 1` and no `isError: true`. A second invocation should
report `pushed_events: 0`.

## 6. End-to-end with Claude Code (manual, ~10 minutes)

Verify the full skill → subagent → MCP tool path inside a real Claude
Code session.

**Setup**

1. Copy or symlink the integration bundle:
   ```bash
   cp -r integrations/claude-code/skills/* ~/.claude/skills/
   cp -r integrations/claude-code/agents/* ~/.claude/agents/
   ```
2. Add the MCP server to `~/.claude.json` (merge the snippet from
   `integrations/claude-code/mcp.example.json` into your `mcpServers`,
   pointing `--project` at a real DITS project).
3. Restart Claude Code.

**Run**

In the project directory, ahead of starting Claude Code:

```bash
dits work create --kind execution --title "verify the integration" --body "Create a file called HELLO and write 'world' into it."
```

In Claude Code, prompt: **"work the queue"**.

**Pass criteria:**

- Claude calls `dits_work_list` (or the dits-worker subagent does),
  finds the ready execution task, leases it, starts an attempt,
  performs the file edit, checkpoints, completes the attempt, and
  releases the lease.
- `dits work show <id>` after the session shows the full event sequence:
  `lease-acquired → attempt-started → progress-checkpoint* →
  attempt-completed → lease-released`.

## 7. Multi-agent worker + judge (manual, ~15 minutes)

Verify the eval/retry branch with two cooperating sessions.

**Setup**: same as step 6, but open two Claude Code windows in the same
project. Configure one to use the `dits-worker` subagent, the other to
use `dits-judge`.

**Run**

```bash
dits work create --kind execution --title "needs review" --body "..."
```

1. In the worker window: "work the queue". Worker leases, executes,
   completes, then emits `eval-request`.
2. In the judge window: "review pending evals". Judge calls
   `dits_work_list` filtered to items with pending evals, inspects via
   `dits_work_show`, and emits `eval-complete` with `verdict: fail`.
3. Back to the worker window: confirm the worker (via the work-loop
   skill's retry branch) detects the failed eval, starts a new attempt,
   and tries again.

**Pass criteria:** the work item's event log contains at least two
`attempt-started` events with an `eval-complete{verdict:fail}` between
them.

## What's covered where

| Step | Type | Runs in CI? | Required pre-release? |
|---|---|---|---|
| 1. Build | automated | yes | yes |
| 2. Unit + smoke | automated | yes | yes |
| 3. Release artifacts | automated | yes (release workflow) | yes |
| 4. MCP smoke (stdio) | manual | no | yes |
| 5. Sync round-trip | manual | no | yes |
| 6. Claude Code E2E | manual | no | yes (per integration release) |
| 7. Multi-agent | manual | no | recommended |
