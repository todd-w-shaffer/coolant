# Extensible Tool Gating System — Implementation Report

**Date:** 2026-04-04
**Status:** Shipped (Steps 1-6 of 8)

## Problem

The project that inspired Coolant — a Node/TypeScript codebase — experiences CPU meltdowns from two sources:

1. **Unbounded test runners.** `vitest` spawns one worker per CPU core by default. With 5 parallel agents each triggering test runs, that's 5 × 10 = 50 vitest workers competing for 10 cores.
2. **Repeated type-checking.** `tsc --noEmit` runs on every file edit. With 8 agents each editing 20 files, that's 160 tsc invocations during the writing phase alone.

Coolant v1 suppressed tsc via a PostToolUse hook on Edit/Write, but only for one tool in one language. The gating system needed to be extensible across ecosystems.

## Design Decisions

### Command matching over ecosystem detection

The adversarial review killed the original plan of auto-detecting ecosystems by sniffing for `package.json`, `Cargo.toml`, etc. The insight: **the command IS the signal**. If the Bash call is `cargo clippy`, we know it's Rust without checking any config files. Zero-config, always correct, no polyglot monorepo edge cases.

### PreToolUse over PostToolUse

Claude Code's hook system supports `PreToolUse` hooks that fire before a tool executes. This enables:
- **Suppress** (`permissionDecision: "deny"`) — block the command entirely
- **Rewrite** (`updatedInput`) — inject flags like `--maxConcurrency 2` before execution

PostToolUse can only react after the damage is done. PreToolUse prevents it.

### JSONL as the universal event bus

Bash writes structured events to `$TMPDIR/coolant-$USER.events.jsonl`. Go tails this file at 500ms intervals. This follows the project's core principle: bash for plumbing, Go for visualization, structured data at the boundary.

### Single thin dispatcher

One `gate.sh` script handles all languages via a case statement. No registry files, no ecosystem detection, no profile system. Under 90 lines.

## What Was Built

### Bash layer

| File | Change | Purpose |
|------|--------|---------|
| `scripts/common.sh` | Modified | Added `COOLANT_EVENTS` path, `coolant_event()` for JSONL output, `_json_field()` and `_nested_command()` for jq-free JSON parsing, `_json_escape()` for safe value interpolation |
| `scripts/gate.sh` | **New** | PreToolUse hook — pattern-matches commands across 5 ecosystems, suppresses during parallel mode, handles transparent wrappers (`npx`, `env`, `command`, path prefixes) |
| `scripts/agent-start.sh` | Modified | Reads hook stdin, extracts agent metadata, emits `agent.start` and `parallel.engaged` JSONL events |
| `scripts/agent-stop.sh` | Modified | Reads hook stdin, emits `agent.stop` and `parallel.disengaged` JSONL events |
| `hooks/hooks.json` | Modified | Added `PreToolUse` section for Bash → `gate.sh` |

### Go layer

| File | Change | Purpose |
|------|--------|---------|
| `thermal/internal/collector/events.go` | **New** | `GateEvent` struct, event name constants, poll-based JSONL tailer with truncation recovery |
| `thermal/internal/model/state.go` | Modified | `HandleEvent()` maps gate/agent/parallel events to severity-colored alerts |
| `thermal/cmd/thermal/main.go` | Modified | Wired event channel into bubbletea Init/Update loop |
| `thermal/internal/config/tuning.go` | Modified | Added `EventInterval` constant (500ms) |

### Tests

| File | Tests | Coverage |
|------|-------|----------|
| `tests/common.bats` | 12 | JSONL events, JSON parsing, escaping |
| `tests/gate.bats` | 29 | All ecosystems, wrappers, edge cases |
| `tests/agent-start.bats` | 9 | Counter + JSONL events |
| `tests/agent-stop.bats` | 9 | Counter + JSONL events |
| `thermal/internal/collector/events_test.go` | 4 | Tailer: read, missing file, malformed, truncation |
| **Total** | **113 bats + 4 Go** | |

### Gated tools by ecosystem

| Ecosystem | Gated commands |
|-----------|---------------|
| TypeScript/Node | `tsc`, `vitest`, `jest`, `eslint`, `prettier`, `webpack`, `esbuild`, `vite build` |
| Rust | `cargo build`, `cargo test`, `cargo clippy`, `cargo check` |
| Go | `go build`, `go test`, `go vet` |
| Python | `pytest`, `mypy`, `pylint`, `ruff` |
| Java | `gradle`, `mvn`, `javac` |

All matched regardless of wrapper (`npx tsc`, `env vitest`, `command cargo build`) or path prefix (`/usr/local/bin/tsc`).

## Security Hardening

The implementation went through a security review that identified and fixed three issues:

1. **JSON injection via backslash** — Added `_json_escape()` that escapes backslashes before quotes. Applied to all values interpolated into JSONL.

2. **Symlink attacks on `/tmp`** — Moved state files from `/tmp/` to `$TMPDIR` (macOS per-user `/var/folders/.../T/`). Not world-accessible, eliminates symlink pre-creation by other users.

3. **Pattern match bypass via wrappers** — `npx tsc`, `env vitest`, `/usr/local/bin/tsc` all bypassed first-word matching. Added transparent prefix stripping for `npx`, `env`, `command`, `nice`, `time`, `sudo` and path-component stripping.

Accepted risks (low impact):
- Env var override of state paths (by design for testability)
- Lockfile TOCTOU (advisory mechanism, idempotent operations)
- JSONL injection by same-user process (cosmetic dashboard impact)

## What's Next (Stubbed, Not Built)

### Step 7: Concurrency capping

Uses `updatedInput` to rewrite commands with concurrency limits:
- `vitest run` → `vitest run --maxConcurrency 2`
- `cargo test` → `cargo test -j 2`
- `go test` → `go test -parallel 2`
- `pytest` → `pytest -n 2`

Cap value from `/tmp/coolant-$USER.cap`, set by `toggle.sh` or auto-computed from thermal state.

### Step 8: Debounce

Hash command → check last run time → skip if within window. Known limitation: under parallel agents, sources always changing, so debounce rarely fires. Useful for single-agent repeated builds.

## Process Notes

- TDD discipline held for Steps 1-4 (red-green-refactor). The security hardening pass broke this — fixes were written before tests. This was noted and corrected in project memory.
- The adversarial agent review (end-user perspective) was valuable for killing bad ideas early. The ecosystem detection plan and the three-behavior-at-once scope were both correctly identified as premature.
- The PreToolUse `updatedInput` capability was confirmed via Claude Code documentation search, which unblocked the concurrency capping design.
