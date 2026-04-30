# scripts/ — Bash hooks and plumbing

Bash layer: SessionStart / PreToolUse / Subagent hooks, reconciled counters, JSONL event emission, `upgrade.sh`. The bash↔Go seam is a JSONL event log — see "JSONL event bus" in root `CLAUDE.md`.

## Conventions

- All scripts must be bash 3.2 compatible (macOS system bash). No `mapfile`, no associative arrays, no `|&`.
- All scripts source `scripts/common.sh` for shared config paths (`COOLANT_LOCKFILE`, `COOLANT_COUNTER`, `COOLANT_LOG`, `COOLANT_EVENTS`, `COOLANT_AGENT_STARTS`, `COOLANT_DEGRADED_COUNT`, `COOLANT_THRESHOLD`).
- Hook scripts log human-readable events via `coolant_log "message"` and structured JSONL via `coolant_event '"key":"value"'`. `coolant_event` injects `"schema":1` into every envelope and serializes `>>` appends behind `coolant_lock` (mkdir-mutex). The lock is **NOT reentrant** — callers already holding `coolant_lock` MUST release it before invoking `coolant_event`, or they deadlock for ~1s and fall through to an unsynchronized write that bumps `$COOLANT_DEGRADED_COUNT`.
- JSON field extraction from hook stdin uses `_json_field` (top-level), `_nested_command` (tool_input.command), and `_extract_escaped` (extract + escape for re-emission into JSONL) — no jq dependency.
- Values interpolated into JSONL must pass through `_json_escape` to handle backslashes and quotes. Agent metadata extraction uses `_extract_agent_fields`, which calls `_extract_escaped` for each field.
- macOS system APIs: `sysctl`, `vm_stat`, `ps -Ao`, `ioreg` for sensors. No third-party tools.

## State files

State lives in `$TMPDIR/coolant-$USER.*` files — lockfile, counter, event log, agent-starts tsv (agent_id<TAB>epoch_s for duration_s computation on stop; 24h entries self-prune), version cache, `coolant-$USER.session` (the session-id sidecar written by `preflight.sh` on `SessionStart` and read by `_reconcile_counter` + Go's `TailEvents` to scope per-session counters), and `coolant-$USER.cc-otel.jsonl` (raw metric data points from thermo's embedded OTLP receiver — sibling to events.jsonl, consumed by `internal/otel/cc/` reconciliation). No databases, no config files at runtime. `$TMPDIR` is per-user on macOS (`/var/folders/.../T/`), avoiding `/tmp` symlink attacks. Durable state — cross-session aggregates and CC OTEL drift findings — lives in `~/.coolant/` (`stats.json`, `cc-otel-findings.jsonl`).

## Gate system

`gate.sh` is a PreToolUse hook on Bash with two behaviors. **Test runners** (vitest, jest, cargo test, go test, pytest) are always throttled with agent-count-adaptive concurrency limits: `cap = floor((cores - 2) / agents)`, min 1. **Build tools, linters, and type checkers** (tsc, eslint, cargo build, go vet, etc.) are blocked when user opts in via `/coolant`. Agent count is reconciled against JSONL event log (`_reconcile_counter` in common.sh) to prevent stale counters from orphaned agents. Handles wrappers (`npx`, `env`, `command`, path prefixes). See `docs/gate-system-report.md`.

### Terminology mapping

User-facing labels were relabeled (commit 9c751e3); JSONL event names and Go constants are unchanged.

| User-facing | JSONL event | Go constant | Trigger |
|-------------|-------------|-------------|---------|
| `throttled: <cmd> → <rewritten>` | `gate.cap` | `EventGateCap` | Test runners (automatic) |
| `blocked: <cmd> (parallel mode — /coolant to release)` | `gate.suppress` | `EventGateSuppress` | Build tools (opt-in via `/coolant`) |

The intel overlay uses "throttled"/"blocked" in the tools row. `docs/gate-system-report.md` still uses old "capped"/"suppressed" terminology — historical doc, left as-is.

## Testing

```bash
bats tests/                        # full suite
bats tests/toggle.bats             # single file
bats tests/ -f "reconcile"         # name pattern
```

`tests/test_helper.bash` provides `setup`/`teardown` that isolates all state to a temp directory — tests never touch real `/tmp/coolant-*` files. Tests set env vars (`COOLANT_LOCKFILE`, etc.) to point at the temp dir; scripts respect these via the defaults in `common.sh`.

One assertion per test, behavior-describing names (`agent-start auto-engages at threshold`). Install via `brew install bats-core`.
