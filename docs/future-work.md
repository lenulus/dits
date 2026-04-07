# Future Work

Speculative items, not committed scope. Captured here so they aren't lost
between sessions and so anyone picking them up has enough context to make
the same trade-offs we'd make.

## Alternate frontends over `internal/workops`

`internal/workops` was extracted specifically so multiple frontends can
share one backend. Today there are two: the `dits` Cobra CLI
(`internal/cli/`) and the `dits-mcp` MCP server (`internal/mcp/`). Both
are thin shims — `internal/workops/workops_test.go` pins the contract
independently of either.

Two frontends worth considering:

### TUI (`cmd/dits-tui`)

A `bubbletea`-based terminal UI showing the ready queue, current
attempts, recent events, and pending evals — the same surfaces the
work-loop skill walks through, but for humans driving DITS directly.

- **Why:** The CLI is great for scripting and automation; the MCP server
  is great for Claude Code; neither is great for a human who wants an
  always-on dashboard of "what's claimed, what's blocked, what's waiting
  on me."
- **Shape:** A single binary that opens the local project via
  `workops.Open()`, polls (or watches) the reducer for changes, and
  renders panes. Same lease/start/checkpoint operations the work-loop
  uses, bound to keystrokes.
- **What to reuse:** all of `internal/workops`. Nothing else from
  `internal/cli` should leak in — the TUI is a peer to the CLI, not a
  layer on top of it.
- **Open questions:** how to handle live updates without busy-polling
  SQLite (the store has no pub/sub); whether to ship as a separate
  binary or as a `dits tui` subcommand.

### Web UI (`cmd/dits-web` or extending `dits-server`)

A read-mostly browser view served either by a new binary against a
local checkout, or as a set of HTML routes added to `dits-server` for
the multi-actor case.

- **Why:** Stakeholders who don't run a terminal still want to see
  pending evals, retain/discard outcomes, and the event log of a given
  work item.
- **Shape:** Extend the existing `internal/server` v2 query API with
  HTML routes, or build a small SPA against the JSON endpoints.
- **What to reuse:** `internal/workops` for any mutation paths,
  `internal/store` for queries (the v2 API already does this).
- **Open question:** does a web UI imply server-side identity / auth?
  Today DITS identity is per-actor and tied to the local `.dits/`
  directory. A multi-user web UI changes that model — worth thinking
  through before building.

In all cases the test discipline is the same: anything that mutates
state goes through `workops` and the existing `workops_test.go` pins
it; frontend code stays I/O-only and ships its own thin smoke test
along the lines of `internal/mcp/server_test.go`.

## Emit DITS state to a TODO-style file

A file like `TODO.md` (or `.dits/TODO.md`, or wherever the user wants)
that materializes the current ready/in-progress/blocked work items into
a flat checklist a human can skim, grep, or feed back into Claude Code
as plain context.

### Why

- Many editors and tools (VS Code's outline, GitHub previews, plain
  `grep`, every LLM that doesn't have an MCP client wired up) understand
  Markdown checklists trivially. They don't speak the DITS event log.
- A committed `TODO.md` makes the project state visible in PR diffs and
  on GitHub without anyone needing the `dits` binary installed.
- It gives Claude Code (and other LLM tools) a zero-tool fallback path:
  even without the MCP server registered, they can read `TODO.md` and
  reason about what needs doing.

### Shape

A new `dits` subcommand and a corresponding `workops` method:

```
dits export todo [--out TODO.md] [--include-closed] [--group-by kind]
```

Backed by `workops.ExportTodo(ctx, opts) ([]byte, error)` so the same
export is callable from CLI, MCP (`dits_export_todo`), and any future
TUI/web frontend.

Output sketch:

```markdown
# DITS — TODO

_Generated from .dits/ at 2026-04-07T04:13:05Z. Do not edit by hand —
run `dits export todo` to regenerate._

## Ready

- [ ] **PROJ-12** Fix the auth middleware regression `task` `priority:high`
- [ ] **PROJ-15** Investigate flaky sync test `investigation`

## In progress

- [ ] **PROJ-9** Refactor reducer for execution kind `task` — claimed by
      `actor_3e5e066445f40eb8`, attempt active for 14m

## Blocked

- [ ] **PROJ-7** Migrate to v3 schema `task` — _waiting on legal sign-off_

## Pending eval

- [ ] **PROJ-11** Ship integration bundle `execution` — eval requested,
      no judge yet
```

### Update modes

Two reasonable modes, pick one or expose both:

1. **One-shot (`dits export todo`)** — write or rewrite the file on
   demand. Trivial. Suitable for `make`, pre-commit hooks, CI.
2. **Watch / hook (`dits export todo --watch`)** — re-emit on every
   event append. Implementable as a small loop polling
   `GetEventsForWorkItem` with a high-water mark, or by hooking into
   `workops.AppendAndMaterialize` directly via a callback registry.

Mode 2 is much nicer in practice but introduces a long-running process;
mode 1 covers most use cases (commit hook, manual refresh, CI artifact)
and should land first.

### Things to think about

- **Determinism:** the output must be byte-stable for a given DITS
  state, otherwise it churns in PR diffs. Sort sections by shared ID
  ascending; format timestamps in UTC with a fixed precision; resolve
  actor IDs to a stable display form.
- **Sections vs. priorities:** `--group-by kind` vs. `--group-by status`
  vs. `--group-by priority`. The example above mixes the three; in
  practice users probably want one grouping with optional badges.
- **Round-trip is out of scope.** This is one-way — DITS → markdown.
  Editing `TODO.md` and trying to fold changes back into the event log
  is a different (much harder) project; explicitly don't promise it.
- **Who owns the file?** If `TODO.md` lives at the repo root and is
  committed, the project needs a convention (gitignore? committed and
  regenerated by hook? CI-generated artifact?). Document the chosen
  convention in `integrations/` or in the subcommand help.
- **Skill integration:** add a one-line entry to
  `integrations/claude-code/skills/dits-work-loop.md` so that when the
  MCP server isn't available, Claude can still read `TODO.md` as a
  fallback context source.
