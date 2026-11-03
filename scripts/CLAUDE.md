# scripts/ — Bash hooks and plumbing

Bash layer: SessionStart / PreToolUse / Subagent hooks, the work-queue door shim (`coolant`), reconciled counters, JSONL event emission, `upgrade.sh`. The bash↔Go seam is a JSONL event log — see "JSONL event bus" in root `CLAUDE.md`.

## Conventions

- All scripts must be bash 3.2 compatible (macOS system bash). No `mapfile`, no associative arrays, no `|&`.
- All scripts source `scripts/common.sh` for shared config paths (`COOLANT_COUNTER`, `COOLANT_LOG`, `COOLANT_EVENTS`, `COOLANT_AGENT_STARTS`, `COOLANT_DEGRADED_COUNT`).
- Hook scripts log human-readable events via `coolant_log "message"` and structured JSONL via `coolant_event '"key":"value"'`. `coolant_event` injects `"schema":1` into every envelope and serializes `>>` appends behind `coolant_lock` (mkdir-mutex). The lock is **NOT reentrant** — callers already holding `coolant_lock` MUST release it before invoking `coolant_event`, or they deadlock for ~1s and fall through to an unsynchronized write that bumps `$COOLANT_DEGRADED_COUNT`.
- JSON field extraction from hook stdin uses `_json_field` (top-level), `_nested_command` (tool_input.command), and `_extract_escaped` (extract + escape for re-emission into JSONL) — no jq dependency.
- Values interpolated into JSONL must pass through `_json_escape` to handle backslashes and quotes. Agent metadata extraction uses `_extract_agent_fields`, which calls `_extract_escaped` for each field.
- macOS system APIs: `sysctl`, `vm_stat`, `ps -Ao`, `ioreg` for sensors. No third-party tools.

## State files

State lives in `$TMPDIR/coolant-$USER.*` files — counter, event log, agent-starts tsv (agent_id<TAB>epoch_s for duration_s computation on stop; 24h entries self-prune), version cache, `coolant-$USER.session` (the session-id sidecar written by `preflight.sh` on `SessionStart` and read by `_reconcile_counter` + Go's `TailEvents` to scope per-session counters), and `coolant-$USER.cc-otel.jsonl` (raw metric data points from thermo's embedded OTLP receiver — sibling to events.jsonl, consumed by `internal/otel/cc/` reconciliation). No databases, no config files at runtime. `$TMPDIR` is per-user on macOS (`/var/folders/.../T/`), avoiding `/tmp` symlink attacks. Durable state — cross-session aggregates and CC OTEL drift findings — lives in `~/.coolant/` (`stats.json`, `cc-otel-findings.jsonl`). The work queue lives there too, deliberately not in `$TMPDIR` (a context with `TMPDIR` unset would split it): `~/.coolant/queue/` (tickets) and `~/.coolant/runs.jsonl` (one record per job).

## Work queue door (`scripts/coolant`)

`scripts/coolant` is the machine-wide work queue's door: `coolant run --class <c> -- <cmd>`, `coolant queue [--json]` and `coolant queue weights [--json|--toml]`. The queue itself is Go (`thermo run` / `thermo queue`, package `thermal/internal/workq`); the shim only finds a thermo that answers `--door-version` with `1` (`$COOLANT_THERMO` alone when set, else the checkout's `bin/thermo` resolved through symlinks, else `thermo` on PATH) and execs it. The probe is a flag so a thermo without the door fails `flag.Parse` and exits at once; a bare verb would start its TUI inside a git hook. With no door-capable thermo, `run` warns once and execs the command unqueued; a job never fails because the queue is missing. Interface contract for callers (flags, env, exit codes, record fields): `docs/work-queue.md`.

## Gate system

`gate.sh` is a PreToolUse hook on Bash that **routes** typed heavy commands into the work queue: it rewrites `<cmd>` to `<door> run --class <class> -- <cmd>`, where `<door>` is `scripts/coolant` beside the hook (`$COOLANT_DOOR` overrides; quoted when the path needs it). It never caps and never denies — admission is the queue's job. The door is a **prefix**, so `2>&1` and pipes into read-only filters stay outside it and apply as typed (`vitest run 2>&1 | tail -5` queues only vitest). Because the rewrite answers `"allow"` — which skips the permission prompt for the whole line (deny and ask rules still apply; permission rules match the rewritten command) — the gate routes only a **lone** heavy command: `lone_command` declines `;`, `&&`, `||`, a background `&`, any redirect but `2>&1`, and a pipe into anything but `tail`/`head`/`grep`/`wc`/`cat`. A declined line runs through the normal permission flow, unqueued, logged `unrouted: <cmd> (not a lone command)`. Wrappers (`npx`, `env`, `command`, `nice`, `time`, `sudo`) and leading assignments (`CI=1 vitest run`) go to the door with the command — the door applies leading `NAME=value` words as env(1) does. The gate skips plain-valued assignments (and `env`'s) only to find the binary; a quoted value (`FOO='a b' vitest`) is not recognized, so that command runs unrouted. Class mapping: vitest/jest/pytest → `files` when the gated segment names a file-shaped word (`*.test.*`, `*.spec.*`, `*.ts`, `*.py`, …; a directory like `test/` is still a whole suite), else `suite`; cargo/go/swift/xcodebuild test → `suite`; tsc/mypy → `typecheck`; eslint/ruff/pylint/swiftlint/cargo clippy|check/go vet → `lint`; esbuild/webpack/tailwindcss/vite build/cargo|go|swift build/xcodebuild build|archive|analyze/gradle/mvn/javac → `build`. prettier is not gated (it is not heavy).

The gate understands exactly one piece of shell grammar: **quoting** (an operator inside an argument, `-t 'a|b'`, is not a pipeline). `first_operator_offset` uses it only to scope the files-vs-suite check to the gated segment, so `vitest run | grep a.test.ts` is still a suite. The parse check runs once, after the binary is known to be heavy and before classifying.

Everything past that grammar — command substitution, backticks, subshells, embedded newlines, any backslash (`_nested_command` hands over the raw JSON string and the rewrite re-escapes it, so `a\.b` would return as `a\\.b`), and the truncated remnant left by a double-quoted argument — is **refused**, not guessed: `route_parseable` returns non-zero, the gate logs `unrouted: <cmd> (unparseable shell)` and exits without rewriting. Rewriting wrongly runs a different command than the one typed, which costs more than an unqueued one; keep new grammar rules on that side of the trade. The wrapper flag-skip loop must `break` when no space remains — `${rest#* }` is a no-op at that point, so a bare-flag wrapper (`sudo -v`) would otherwise spin until Claude Code's hook timeout. See `docs/gate-system-report.md` (historical).

### Terminology mapping

| User-facing | JSONL event | Go constant | Trigger |
|-------------|-------------|-------------|---------|
| `routed: <cmd> → <class>` | `gate.route` | `EventGateRoute` | Typed heavy commands (automatic) |
| `throttled: <cmd> → <rewritten>` | `gate.cap` | `EventGateCap` | Not emitted; parsed from older logs |
| `blocked: <cmd> (parallel mode, retired)` | `gate.suppress` | `EventGateSuppress` | Not emitted; parsed from older logs |

The intel overlay's tools row counts `routed`. `docs/gate-system-report.md` uses old "capped"/"suppressed" terminology — historical doc, left as-is.

## Linting

Run `shellcheck <file>` on any bash script you edit or create before reporting the task complete — `.sh`, `.bash`, `.bats`, and the extensionless hooks under `.githooks/` and `.claude/hooks/`. shellcheck catches quoting, globbing, and variable-scope bugs that grep+Edit misses; the highest-stakes bash (commit classifier, audit-review gates) lives extensionless so it won't be caught by any LSP — manual lint is the only safety net.

## Testing

```bash
bats tests/                        # full suite
bats tests/gate.bats               # single file
bats tests/ -f "reconcile"         # name pattern
```

Run bats with `< /dev/null` from an agent harness: several hook tests call `agent-start.sh` with no stdin, and when bats inherits an open pipe the hook's `cat` blocks forever.

`tests/test_helper.bash` provides `setup`/`teardown` that isolates all state to a temp directory — tests never touch real `/tmp/coolant-*` files. Tests set env vars (`COOLANT_COUNTER`, etc.) to point at the temp dir; scripts respect these via the defaults in `common.sh`.

One assertion per test, behavior-describing names (`gate routes tsc as typecheck`). Install via `brew install bats-core`.
