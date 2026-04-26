# Go thermal dashboard — design reference

Pull this in when working on widgets, rendering, or animation internals. The conventions that prevent compilation errors (bubbletea v2 API, lipgloss v2 types) live in `thermal/CLAUDE.md` — this doc covers *how the subsystems work*.

## Keyboard shortcuts

- `h` — toggle help overlay (replaces sparklines with plain-language explainers)
- `c` — collapse/expand the notification bar (top row with [i] install, etc.)
- `q` / `ctrl+c` — quit

## UI layers

- **Notification bar** (top, collapsible via `c`) — transient chrome: install CTA, future version alerts
- **Rates bar** (bottom, always visible) — system stats with colored dots + spawn/death/net + permanent `[h] help`
- **Status messages** — loaded from `thermal/internal/model/data/messages.csv` via `go:embed`, prefixed with `:: `

## Sparklines

Double-resolution braille: both columns of each character pack two time samples (left=N, right=N+1), doubling visible history. Raw data is linearly interpolated (midpoint insertion) before rendering to turn step-function transitions into visible ramps. Shared helpers `prepareSparkData()`/`prepareSparkMask()` handle zero-padding, interpolation, and visible window slicing.

**Height** is proportional (auto-scaled to visible window peak). **Color** encodes severity via go-colorful `BlendHcl()` (green->yellow->red, 24-bit truecolor). Values below 2% of peak render as invisible (noise floor).

**Edge fades** dim the outermost 3 characters on both left and right edges (35%->60%->82% brightness ramp) so data fades in on entry and fades out on exit. Uses `dimmedFg()` which blends severity color toward black via Lab space.

## Gauges

Harmonica spring physics (critically damped, 30fps) for smooth numeric readout easing. **Sparklines scroll at animation rate (30fps)**, not collector rate: each `AnimTick` pushes the spring-interpolated value into a per-gauge `renderHistory` buffer, and sparklines render from this buffer. This decouples scroll speed from data collection. Peak per gauge tracks the visible render-history window only with fast decay (0.982/tick at 30fps, ~1.3s half-life) — spikes that scroll off screen release the scale within ~1s.

## Render architecture

Collector samples at 150ms, animation tick runs at 30fps (~33ms). Springs interpolate between data arrivals (~4.5 frames per sample). `snapshotMsg` updates spring targets; `animTickMsg` advances springs AND pushes interpolated values into render history, driving sparkline scroll. The two rates are fully decoupled via separate bubbletea messages.

## CPU sampling

Caches the mach host port (avoids port leak) and holds the last computed CPU% when tick deltas are zero (avoids false 0% gaps at 150ms).

## Stats engine

`internal/stats/` persists cross-session agent aggregates that the in-memory ring buffer can't answer ("how many agents this month?", "all-time peak concurrent?"). The on-disk shape is `~/.coolant/stats.json` — distinct from `$TMPDIR/coolant-$USER.events.jsonl` because macOS may purge `$TMPDIR` on reboot.

**Durability split.** JSONL is the streaming source of truth; the cache is durable history. Several fields are *primary* — they cannot be reconstructed from a post-rotation JSONL and must survive every cache-discard / migration path: `records`, `daily` buckets, `first_seen`, `by_type`, `by_project`. Lifetime totals are *always* `sum(daily)` computed on demand — never stored separately, eliminating cache-vs-live drift.

**Schema gate (virtual chop).** `Aggregator.Fold` drops events whose `Schema` falls outside `[1, MaxKnownSchema]`. Pre-versioning events (no schema field, parsed as 0) are silently skipped; future schema-N events on an old binary are also skipped. The JSONL is never rewritten — old and new envelopes coexist line-by-line.

**Delta-merge checkpoint dance.** Each Aggregator tracks a `baseline` Snapshot (the on-disk state at last load/checkpoint). At Checkpoint time: re-read disk → compute `delta = current - baseline` → per-key additive merge for `byType`/`byProject`/`daily` → max-merge `records` (newest-`At` tiebreak) → fsync tempfile → rename → fsync parent dir → adopt as new baseline. This handles two thermos checkpointing concurrently without losing increments.

**Concurrency.** `sync.RWMutex` guards all mutable state. `Fold` and `Checkpoint` take the write lock; `Snapshot` takes the read lock. A process-local `sync.Mutex` (`processLock`) serializes Checkpoint within a binary; cross-process `flock` is deferred — the delta-merge math is correct under single-binary concurrency.

**Stale prune.** `Checkpoint` calls `pruneStale(now)` to drop `agentStarts`/`agentMeta` entries older than 24h. Mirrors the bash `_compute_agent_duration` cutoff for agents that crash without emitting `agent.stop`.

**Wiring.** `cmd/thermal/main.go::newModel` constructs the aggregator via `productionStatsConfig()` and calls `AppState.AttachAggregator`. `AppState.HandleEvent` fan-outs to `aggregator.Fold` when attached (nil-safe for tests). A 30s checkpoint goroutine in `Init` final-flushes on graceful shutdown via the `checkpointDone` channel `main()` blocks on post-Run, so process exit can't race the fsync.

**Hidden subcommand.** `thermo statsdump` (dispatched via first-arg matching in `cmd/thermal/main.go` BEFORE `flag.Parse`) folds the JSONL once into a fresh aggregator and dumps the snapshot as JSON — dev/debug tool, distinct from the future user-facing `thermo stats` (separate spec).
