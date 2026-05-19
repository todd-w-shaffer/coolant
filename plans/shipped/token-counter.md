# Token counter

## Goal
Surface live token throughput + prompt cache hit % on the rates widget, sourced by tailing Claude Code's per-session transcript JSONL files. One row in the existing rates strip — no new widget, no per-agent breakdown.

## Non-goals
- Dedicated widget or sparkline (defer to phase 2 if signal proves useful)
- Per-agent / per-session breakdown
- Cost math ($/min, model-pricing tables)
- Historical persistence across coolant restarts
- Anything that meaningfully raises CPU baseline — coolant exists to keep machines cool

## Phases

**Phase 0 — Extract shared JSONL tailer.** Land as its own commit before tokens.go exists.
- `thermal/internal/collector/jsonl_tail.go` (new) — generic file-tailer: offset tracking, partial-line skip, truncation reset. Takes a path + per-line callback (`func([]byte) error`).
- `thermal/internal/collector/jsonl_tail_test.go` (new) — fixtures for offset advance, partial-line resilience, truncation reset.
- `thermal/internal/collector/events.go` — refactor `TailEvents` to use the new tailer. Behavior unchanged.

**Phase 1 — Token collector + widget integration.**
- `thermal/internal/collector/tokens.go` (new) — multi-file session discovery (`~/.claude/projects/*/*.jsonl`), per-file offset map, dedupe-by-`message.id`, EMA rate using `RateSmoothAlpha`. Uses the Phase 0 tailer.
- `thermal/internal/collector/tokens_test.go` (new) — table-driven: dedupe (two rows sharing one `message.id` → counted once), cache-hit formula, cold-start offset seeding.
- `thermal/internal/collector/types.go` — add `TokenStats` struct + field on `Snapshot`.
- `thermal/internal/collector/collector.go` — third goroutine in `Run`, publishes via existing mutex pattern in the slow loop.
- `thermal/internal/widgets/rates.go` — append one fragment to the existing rates line: `tok N/s · cache N%`, after `net:+N/s` and before the Desktop/Chrome indicators. Self-suppress when `TokenStats` is zero.
- `thermal/internal/demo/demov2.go` — synthetic token-rate oscillation driven by existing phase state (don't introduce new randomness; reuse phase ticks).

## Failure modes to anticipate
- **Double-counting on multi-block responses.** Confirmed against a live transcript: when a response has both `thinking` and `tool_use` blocks, two consecutive rows share the same `message.id` and carry **identical** usage. Must dedupe on `message.id` per-file, not naively sum. Without this, every tool-using turn counts 2-3×.
- **Cold-start spike.** First scan of an existing session file would replay the entire history as "just happened." Must seed `offsets[path] = file size at discovery time` so we only count new activity, not historical. The widget should show zero tok/s until the next message lands.
- **File discovery: encoded cwd.** Sessions live at `~/.claude/projects/<dash-encoded-cwd>/<session-uuid>.jsonl`. "Active" = mtime within the last 60s. Glob all project dirs, not just the current one (agents can spawn outside cwd).
- **Partial-line reads.** If we read while Claude Code is mid-flush, the trailing line may be incomplete JSON. Skip lines that fail to parse, do **not** advance the offset past them.
- **File rotation/truncation.** Sessions don't rotate, but if a file's size shrinks below our stored offset (manual cleanup, disk issue), reset offset to 0 and re-seed.
- **Permission / missing dir.** `~/.claude/projects/` may not exist on a fresh machine. Collector returns zero stats silently — never crash the dashboard.
- **CPU baseline.** Slow loop at 1s with O(active files) re-opens per tick. If >20 active sessions get glob'd, this could matter. Mitigation: cap scan to 32 most-recently-modified files; rely on mtime filter to keep the working set small.

## Done criteria
- Phase 0 ships and `events.go` still passes its existing tests using the new tailer (no behavior change)
- Rates line renders `tok N/s · cache N%` fragment when at least one active session file exists (mtime < 60s)
- Rates line **self-suppresses the token fragment** when `TokenStats` is zero (no `tok 0/s · cache 0%` placeholder)
- Cache-hit ratio uses the canonical formula: `cache_read / (input + cache_creation + cache_read)`
- Dedupe-by-message-id verified by table-driven test (replay a fixture with two rows sharing one `message.id` → counts once)
- `--demo` mode shows synthetic but plausible numbers (rate oscillates with the session phase, cache hit ~85%)
- `go test ./...` green; `bats tests/` still green
- Manual smoke: run `./bin/thermal` during a live Claude Code session, watch the line update as messages stream

## Parking lot
(empty)
