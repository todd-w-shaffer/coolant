# Phase 1 Implementation Plans

This directory contains three sequential plans that together implement the Phase 1 design from `docs/superpowers/specs/2026-04-11-coolant-oss-bsl-foundation-design.md`.

## The three plans

| # | File | Scope | Execution mode | Why split |
|---|---|---|---|---|
| 1a | `2026-04-11-phase1a-foundation.md` | Licensing files, repo reorganization, `pkg/collector` lift | **Serial, inline** | Mechanical file moves; no parallelism safe when touching import paths |
| 1b | `2026-04-11-phase1b-telemetry.md` | `coolant-emit` CLI, six Phase 1 hooks, `/commit` trailer | **Serial bootstrap then parallel fan-out** — ideal for subagent-driven-development | TDD-heavy; 7 tasks can run simultaneously once `coolant-emit` lands |
| 1c | `2026-04-11-phase1c-presentation.md` | Two Grafana dashboards, docs sweep | **Light parallel** | Independent JSONs + Markdown writing |

## Dependency order

```
1a (foundation)  →  1b (telemetry)  →  1c (presentation)
```

**Do not start 1b before 1a is fully green and committed.** The repo layout and module path collapse must be complete before any new source is written. Likewise, 1c dashboards have nothing to visualize until 1b emits metrics.

## Recommended execution cadence

- **Plan 1a:** inline in one focused session. No subagents needed — the moves are mechanical and benefit from a single cursor.
- **Plan 1b:** launch with `superpowers:subagent-driven-development`. One serial bootstrap task (coolant-emit), then seven parallel subagents for the hooks and trailer, then one serial merge task to wire everything into `hooks/hooks.json`.
- **Plan 1c:** inline or two quick parallel subagents (one per dashboard) plus a docs task.

## Commit conventions

All commits within these plans use the `/commit` skill — never raw `git commit`. The skill generates the `Recipe:` and `Changes:` blocks from conversation context. Plan steps that say "Commit" mean "invoke `/commit`".

## Testing conventions

- **Go code:** direct assertions with `*testing.T`, no framework. Table-driven where behavior fans out. Run: `go test ./...`
- **Bash hooks:** bats-core, one `.bats` file per hook, temp-dir isolated via existing `tests/test_helper.bash`. Run: `bats tests/`
- **TDD strict:** red → green → refactor. No implementation commits without tests.
