# JSONL Event Schema

The coolant event log at `$TMPDIR/coolant-$USER.events.jsonl` is an append-only JSONL file written by bash hooks and tailed by the Go thermal dashboard. Each line is a self-contained JSON object with a `ts` and `event` field.

The log accumulates indefinitely (no rotation). At typical usage (~40 events/day) it grows ~2MB/year. The OS cleans `$TMPDIR` on reboot, but the log survives across thermo restarts within a boot cycle.

## Common fields

Every event has:

| Field | Type | Description |
|-------|------|-------------|
| `ts` | string | ISO 8601 timestamp (UTC) |
| `schema` | int | Envelope shape version. Currently `1`. Pure-additive field changes preserve the version; renames or removals bump it. |
| `event` | string | Event type (see below) |

### Session-scoped vs global events

Events split into two contracts based on whether they belong to a
specific Claude Code session:

| Scope | Events | Filter behavior |
|-------|--------|----------------|
| Session-scoped | `agent.start`, `agent.stop`, `session.start`, `session.end`, `counter.underflow` | Filtered to the current session's `session_id` by both the bash awk reconciler and the Go tailer (`isSessionScoped` in `internal/collector/events.go`) |
| Global | `gate.suppress`, `gate.cap`, `parallel.engaged`, `parallel.disengaged`, `counter.reset`, `preflight.warn` | Pass-through; consumed regardless of session |

The current session id is written to `$TMPDIR/coolant-$USER.session`
by `preflight.sh` on `SessionStart` and read by both filters. When
the sidecar is missing (degraded fallback), session-scoped events
pass through unfiltered — matches pre-spec behavior.

### Envelope versioning

`schema:1` is a **shape contract**, not a deployment marker — any
emitter producing the documented envelope shape may set it. Events
from before this field was introduced remain in the log unmodified;
the Go aggregator's schema gate (`internal/stats`) filters them at
parse time so versioned and unversioned events coexist without
truncation.

### Write serialization

Bash hooks emit through `coolant_event()` in `scripts/common.sh`,
which serializes `>>` appends behind `coolant_lock` (a non-reentrant
mkdir-mutex). The lock prevents parallel hooks from splicing large
payloads when an unsynchronized append would exceed the kernel's
atomic-write threshold. **All callers of `coolant_event` MUST NOT
already hold `coolant_lock`** — that path deadlocks for ~1s and
falls through to an unsynchronized write.

On lock-acquisition timeout, the event is still emitted (signal
preservation > silent drop) and a single newline is appended to
`$COOLANT_DEGRADED_COUNT` — an out-of-band counter file the
aggregator reads via `wc -l` to surface degraded-write totals.

## Event types

### `agent.start`

Emitted by `SubagentStart` hook when Claude Code spawns a subagent.

| Field | Type | Description |
|-------|------|-------------|
| `session_id` | string | Claude Code session UUID |
| `agent_id` | string | Unique subagent identifier |
| `agent_type` | string | Agent type: `Explore`, `general-purpose`, `Plan`, `code-reviewer`, custom agent names |
| `cwd` | string | Full working directory path of the spawning session |
| `project` | string | Basename of `cwd` — the project folder name (e.g. `coolant`) |
| `agent_count` | int | Total active agents after this start |

### `agent.stop`

Emitted by `SubagentStop` hook when a subagent terminates.

| Field | Type | Description |
|-------|------|-------------|
| `session_id` | string | Claude Code session UUID |
| `agent_id` | string | Unique subagent identifier (matches the `agent.start`) |
| `agent_type` | string | Agent type (same as start) |
| `cwd` | string | Full working directory path of the spawning session |
| `project` | string | Basename of `cwd` — the project folder name |
| `permission_mode` | string | Permission mode (same enum as start) |
| `transcript_path` | string | Absolute path to the subagent's conversation log (`.jsonl` in `~/.claude/projects/`) |
| `agent_count` | int | Total active agents after this stop |

### `gate.suppress`

Emitted when a build/lint tool is blocked during parallel mode (`/coolant` engaged).

| Field | Type | Description |
|-------|------|-------------|
| `tool` | string | Always `Bash` |
| `command` | string | The suppressed command |
| `reason` | string | Always `parallel_mode` |

### `gate.cap`

Emitted when a test runner's concurrency is capped based on active agent count.

| Field | Type | Description |
|-------|------|-------------|
| `tool` | string | Always `Bash` |
| `command` | string | Original command |
| `rewritten` | string | Command with concurrency flag injected (e.g. `-parallel 6`) |

### `session.start`

Emitted by the `SessionStart` (`startup` matcher only) hook at the
top of `preflight.sh`, BEFORE `counter.reset`. Lifecycle anchor for
explicit session-duration math (`longest_session_s`). Emitted only on
genuine session creation — `resume` / `clear` / `compact` do not fire
it. Idempotent at the aggregator: a duplicate `session.start` with
the same `session_id` keeps the earliest start timestamp.

| Field | Type | Description |
|-------|------|-------------|
| `session_id` | string | Claude Code session UUID |

### `counter.reset`

Emitted at session start (`SessionStart` hook) to establish a baseline
for agent count reconciliation. Pinned to fire AFTER `session.start`
so consumers folding the JSONL see the lifecycle anchor first. The
aggregator's `EventCounterReset` fold is a no-op (sessions are keyed
on `session_id`, not on counter epochs).

No additional fields.

### `session.end`

Emitted by the `SessionEnd` hook (matcher `.*`) via
`scripts/session-end.sh`. Closes a session for `longest_session_s`
math. Kill -9 / SIGKILL / OS reboot do NOT fire `SessionEnd`; the
aggregator's 8h staleness sweep in `Snapshot` closes those sessions
using last-observed activity as the implicit end timestamp.

| Field | Type | Description |
|-------|------|-------------|
| `session_id` | string | Claude Code session UUID |

### `counter.underflow`

Diagnostic event emitted by `agent-stop.sh` when the active-agent
counter would go negative (i.e. more `agent.stop` events than
`agent.start`). The counter is still floored at zero on disk; this
event surfaces the under-count signal for diagnosis without masking
it. Emitted AFTER the standard `agent.stop` line, after
`coolant_unlock`. Not a user-visible alert in v1 — folded as a
no-op by the aggregator and consumed only via JSONL inspection.

| Field | Type | Description |
|-------|------|-------------|
| `session_id` | string | Claude Code session UUID |
| `raw` | int | The pre-floor counter value (e.g. `-1`) |

### `parallel.disengaged`

Emitted when the last agent stops and the parallel mode lockfile is auto-removed.

| Field | Type | Description |
|-------|------|-------------|
| `agent_count` | int | Always `0` |

### `preflight.warn`

Emitted when the session-start preflight detects a missing `.claude` exclusion in test config.

| Field | Type | Description |
|-------|------|-------------|
| `check` | string | Check name (e.g. `worktree_exclude`) |
| `runner` | string | Test runner name (e.g. `vitest`) |
| `config` | string | Config file path that triggered the warning |

## Queryable dimensions

The schema supports these analytical cuts without any code changes:

| Question | How to derive |
|----------|---------------|
| Agents per day | Count `agent.start` events grouped by date |
| Agent duration | Join `agent.start` and `agent.stop` on `agent_id`, compute `ts` delta |
| Agents per project | Group `agent.start` by `project` |
| Agent type distribution | Group `agent.start` by `agent_type` |
| Peak concurrency | Max `agent_count` across all `agent.start` events |
| Throttle frequency | Count `gate.cap` events per day |
| Suppressed builds | Count `gate.suppress` events |
| Permission mode usage | Group by `permission_mode` |
| Ghost ratio | `agent.stop` count with no matching `agent.start` (orphan stops) |
| Agent transcript lookup | Use `transcript_path` from `agent.stop` to read the full conversation |

## Data sources comparison

| | Coolant event log | Claude Code transcripts |
|---|---|---|
| Location | `$TMPDIR/coolant-$USER.events.jsonl` | `~/.claude/projects/*/*.jsonl` |
| What it captures | Agent lifecycle, gate events, concurrency | Full conversation, tool calls, model output |
| Retention | Unbounded (OS cleans on reboot) | 30 days (pruned by Claude Code) |
| Who reads it | Thermo event tailer, future analytics | `/insights` command |
| Weight | ~170 bytes/event | KBs–MBs per session |
| Bridge | `transcript_path` on `agent.stop` points into the transcript store |

## Not yet captured

Fields available in hook stdin but not currently extracted:

| Field | Available on | Why it's useful |
|-------|-------------|-----------------|
| `team_name` | Not exposed by hooks | Team affinity for multi-agent coordination |
| Parent agent ID | Not exposed by hooks | Nested delegation chains |
| `isolation` (worktree) | Not exposed by hooks | Whether agent got its own worktree |
| Exit reason | Not exposed by hooks | Completed vs errored vs cancelled |
| Model | Not exposed by hooks | Which model (sonnet/opus/haiku) the agent ran on |

These are blocked on Claude Code exposing the fields in hook input, not on coolant implementation.
