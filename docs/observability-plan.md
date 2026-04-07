# Observability Plan — dits-mcp / workops / dits-server

Status: **proposed**, not implemented. Owner: next session.

## Why

Verifications #6 (end-to-end Claude Code) and #7 (multi-agent worker +
judge) from `docs/claude-code-verification.md` are the first time the
full stack — Claude Code → MCP → workops → store → server — runs in
anger. Today there is **no logging** in `cmd/dits-mcp`,
`internal/mcp/`, or `internal/workops/`. When something misbehaves the
only diagnostic surface is `dits work show <id>` after the fact and
whatever `dits-server` happens to log on its side.

That's not enough to debug an interleaving multi-actor workflow. This
plan closes the gap with the minimum viable observability needed to
make the verification loop tractable, then layers on the nice-to-haves
for everyday development.

## Log levels

We use the standard `slog` levels. Keep the discipline tight — if every
tool call is `INFO`, nothing is.

| Level | Used for | Example |
|---|---|---|
| `ERROR` | Operation failed; user / Claude needs to know. | `lease failed`, `sync failed`, `event validation rejected` |
| `WARN` | Unexpected but recoverable; suspicious state. | `lease expired before release`, `blob download failed (continuing)`, `meta version mismatch` |
| `INFO` | One line per externally-visible operation: tool calls, sync requests, lease/start/complete. The default level in production. | `tool dits_work_lease`, `lease acquired`, `attempt completed` |
| `DEBUG` | Internal state transitions: event built, signed, appended, reduced; head computation; reducer pass. Off by default. | `event appended`, `reduced 14 events`, `resolved shared id` |
| `TRACE` (slog level -8) | Wire-level: full event payloads, full request/response JSON, raw SQL. Almost never on. | `mcp request: {…}`, `event payload: {…}` |

The CLI exposes `--log-level` (default `info`) and `--log-format`
(`text` for humans, `json` for tooling). `dits-mcp` reads the same
flags.

**stdio is sacred for `dits-mcp`** — the MCP protocol speaks JSON-RPC
over stdout/stderr, so logs must go to a file by default
(`~/.dits/mcp.log`) or to a `--log-file` path. We never write logs to
stdout or stderr from `dits-mcp` unless explicitly told to.

## Cross-cutting: structured context

Every log line in this plan carries these fields when they apply:

- `actor_id` — who is doing the thing
- `work_item` — `wrk_xxx` ID (canonical), and `shared_id` when known
- `event_id` — when an event was emitted
- `attempt_id`, `lease_id`, `eval_id` — when relevant
- `tool` — for MCP tool calls
- `request_id` — for MCP / HTTP requests, threaded through to the bottom of the stack
- `dur_ms` — for any operation we time
- `err` — `slog.Any("err", err)` so the wrapped chain is preserved

This is the part that pays compounding interest: a `grep work_item=wrk_abc`
across worker.log + judge.log + server.log reconstructs an entire
multi-session interleaving in seconds.

---

## Phase 1 — Minimum viable (blocks verifications #6 / #7)

### 1. Logger setup in `cmd/dits-mcp` and `cmd/dits` ✦ **must-have**

- New `internal/logging` package: `New(level, format, sink) *slog.Logger`
  + a `Parse(level string)` helper. Wraps `slog.NewJSONHandler` /
  `slog.NewTextHandler`.
- `cmd/dits-mcp/main.go`: add `--log-level`, `--log-format`,
  `--log-file` flags. Default file = `$XDG_STATE_HOME/dits/mcp.log` (or
  `~/.dits/mcp.log` if XDG isn't set). Default level `info`. Pass the
  logger into `mcp.Serve(cfg, logger)`.
- `cmd/dits/main.go`: same flags, default sink stderr. CLI is
  human-facing so default format `text`, default level `info`.
- Log one `INFO` line at startup for both binaries:
  `mcp_server_start version=… project_root=… log_level=…`. Solves the
  "wait, which project was that talking to?" footgun in five characters
  of grep.

**Acceptance:** `dits-mcp --log-level=debug --log-file=/tmp/mcp.log`
runs, the file exists after a single tool call, contains a startup
line with the resolved project root.

### 2. Tool-level middleware logging in `internal/mcp` ✦ **must-have**

- Thread the logger through `Config` → `Serve` → `withOps`.
- `withOps` wraps every handler with: `INFO` "tool start" before, `INFO`
  "tool ok" or `ERROR` "tool failed" after, with `tool`, `dur_ms`,
  `request_id` (generate a ULID per call), and `err` on failure.
- Log **arg keys only**, not arg values, by default. At `DEBUG`, log
  redacted args (`title`, `id`, etc. — never `body`, never `payload`).
  At `TRACE`, log the full request JSON.
- Tools that mutate state also log their result IDs (`work_item`,
  `event_id`, `attempt_id`) at `INFO` so a single grep on the log
  reconstructs the call → result mapping.

**Acceptance:** `tail -f ~/.dits/mcp.log | jq -r 'select(.tool)
| "\(.level) \(.tool) \(.dur_ms)ms \(.err // \"ok\")"'` produces a
useful per-call timeline during a Claude Code session.

### 3. Workops event-emission logging ✦ **must-have**

- Add a logger field to `WorkOps`. Default: `slog.Default()`. Set by
  `workops.Open(opts)` (opts struct so we don't break the existing
  no-arg form — keep `Open()` as a thin wrapper).
- `INFO` log at the moment each event is appended-and-materialized:
  one line per event with `type`, `work_item`, `event_id`, `actor_id`,
  and any kind-specific IDs (`lease_id` for lease, `attempt_id` for
  start/checkpoint/complete, `eval_id` for eval).
- `WARN` for the recoverable branches today silenced as `continue`:
  `Sync` swallows blob download failures (`internal/workops/sync.go`)
  — log them with the hash and the underlying error.
- `ERROR` for unexpected failures inside `AppendAndMaterialize`
  (validation, signing, append, reduce). These already return errors;
  we just want to log them at the boundary too so a flat
  `grep level=ERROR` is sufficient triage.

**Acceptance:** running the work-loop test scenario from #6 produces a
log file where `grep work_item=<id>` shows the canonical sequence
`work-created → work-leased → execution-started → checkpointed* →
execution-completed → lease-released`, one line per event.

### 4. Project-root logging at startup ✦ **must-have, trivial**

Already covered as a sub-bullet of (1) but calling it out separately
because it's the single highest-value-per-byte item: log the resolved
`project_root`, the resolved actor ID, and the project key on every
binary startup. Saves more debugging time than anything else on this
list.

---

## Phase 2 — Recommended (run before verification #7 if possible)

### 5. Structured-error wrapping in workops ✦ **should-have**

Today's error chain looks like:
`"sync failed: ingesting events: inserting event evt_xyz: database is locked"`

Readable, but a flat string with no fields to query. Replace the deep
wrappers with `slog.Error(...)` calls at the boundary plus a simpler
returned error, so the log carries `err`, `event_id`, `work_item`,
`step` as discrete fields.

**Scope:** ~15 call sites in workops, mechanical.

**Why phase 2 not phase 1:** the flat strings already work for human
debugging; structured fields are nice-to-have for log queries. Skip if
short on time.

### 6. `dits-server` actor identity in sync log ✦ **should-have**

`internal/server/server.go:88` logs `node_id` but not `actor_id`.
For #7 (worker + judge), `node_id` is per-machine and useless for
distinguishing two Claude Code sessions on the same laptop. Add
`actor_id` from `req.ActorID`. One-line change.

### 7. Request ID threading ✦ **should-have**

Generate a ULID per MCP tool call (already in #2) and pass it through
`context.Context` to workops and the store. Server side: read the
existing chi `RequestID` middleware value into the slog handler.
End result: a single ID lets you grep across `mcp.log` → `dits.log` →
`server.log` for one Claude Code action.

**Implementation note:** a `ContextHandler` wrapping `slog.Handler`
that pulls `request_id` from `ctx.Value` is the cleanest way; add it
to `internal/logging`.

---

## Phase 3 — Nice-to-haves (post-verification, ongoing dev)

### 8. `dits events <id> --follow` ✦ **nice-to-have**

A subcommand that tails the event log for one work item, polling the
store every ~250ms with a high-water mark on `event_id`. Cheap to
build (~50 lines), invaluable when debugging a multi-session
interleaving in real time.

Same logic exposed as an MCP tool `dits_work_events_tail` is *not*
worth it — Claude Code already polls via repeated calls.

### 9. Lease metrics in workops result structs ✦ **nice-to-have**

`LeaseResult` already returns `LeaseExpiresAt`. Add `lease_dur_held_ms`
to `LeaseRelease`'s result so we can log how long every lease was held.
Surfaces stuck/abandoned-lease patterns at a glance.

### 10. SQLite slow-query log ✦ **nice-to-have**

Wrap the `*sql.DB` with a thin proxy that times each query and logs at
`WARN` if it exceeds a threshold (say 100ms). The MaxOpenConns(1) ghost
from the SQLITE_BUSY fix could come back here if we ever add another
hot path; cheap insurance.

### 11. Audit log mode ✦ **nice-to-have, future**

A separate sink that records every mutating tool call to a JSONL file
intended for archival / compliance, distinct from the operational log.
Out of scope until someone actually needs it; flagged because the
plumbing in #2 already gives us 90% of what's needed.

---

## Order of work

```
Phase 1   (blocks #6 / #7)
├── 1. logger setup + flags                     ~half day
├── 4. (rolled into 1)                          —
├── 2. mcp middleware + tool logs               ~half day
└── 3. workops event-emission logs              ~half day

Phase 2   (recommended before #7 multi-session)
├── 6. server actor_id in sync log              5 minutes
├── 7. request ID threading                     ~2 hours
└── 5. structured error wrapping in workops     ~half day

Phase 3   (when convenient)
├── 8. dits events --follow                     ~2 hours
├── 9. lease duration in result structs         ~30 minutes
├── 10. SQLite slow-query proxy                 ~half day
└── 11. audit log mode                          deferred
```

Phase 1 is one focused day's work and is the minimum to make
verifications #6 and #7 actually debuggable. Phase 2 (specifically #6
and #7 from this plan, not from the verification plan — naming
collision, sorry) is what makes a multi-session debug session
tractable.

## Verification

- Phase 1 success criterion: re-run verification #6 with `dits-mcp
  --log-level=debug --log-file=/tmp/mcp.log`. After the session, a
  human (or Claude) reading only `/tmp/mcp.log` and `dits-server`'s
  output can reconstruct the full lease → start → checkpoint →
  complete → release sequence without inspecting the SQLite store.
- Phase 2 success criterion: in verification #7, a single
  `request_id` grep across worker MCP log + judge MCP log + server log
  reconstructs one round-trip cleanly.

## Out of scope

- Metrics (Prometheus, OpenTelemetry, etc.). Logs first; metrics when
  the operational pattern is clearer.
- Distributed tracing. Same reasoning — premature with one machine,
  one process per session.
- Log shipping / aggregation (Loki, Datadog). Local files are correct
  until DITS itself is run multi-host.
