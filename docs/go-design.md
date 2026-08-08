# Go thermal dashboard — design reference

Pull this in when working on widgets, rendering, or animation internals. The conventions that prevent compilation errors (bubbletea v2 API, lipgloss v2 types) live in `thermal/CLAUDE.md` — this doc covers *how the subsystems work*.

## Keyboard shortcuts

- `h` — toggle help overlay (replaces sparklines with plain-language explainers)
- `i` — toggle session intel overlay
- `c` — collapse/expand the notification bar (top row with [i] install, etc.)
- `x` — purge stale agents
- `[` / `]` / `\` — prev / next / clear category filter
- `m` — toggle mouse capture
- `1` / `2` / `3` / `4` / `5` — toggle CPU / MEM / SWAP / TOK / PRTY sparkline visibility. The row holds at most 3 visible at a time; toggle-on when full is a silent no-op.
- `q` / `ctrl+c` — quit

Single source of truth for all key bindings is `internal/keys/KeyMap`. The help overlay's key-binding lines (`layout/horizontal.go:helpView`) generate from `KeyMap.FullHelp()` plus `SparklineToggles()` so new bindings surface automatically.

## UI layers

- **Notification bar** (top, collapsible via `c`) — transient chrome: install CTA, future version alerts
- **Rates bar** (bottom, always visible) — system stats with colored dots + spawn/death/net + permanent `[h] help`
- **Status messages** — loaded from `thermal/internal/model/data/messages.csv` via `go:embed`, prefixed with `:: `

## Sparklines

Double-resolution braille: both columns of each character pack two time samples (left=N, right=N+1), doubling visible history. Raw data is linearly interpolated (midpoint insertion) before rendering to turn step-function transitions into visible ramps. Shared helpers `prepareSparkData()`/`prepareSparkMask()` handle zero-padding, interpolation, and visible window slicing.

**Height** is proportional (auto-scaled to visible window peak). **Color** encodes severity via go-colorful `BlendHcl()` (green->yellow->red, 24-bit truecolor). Values below 2% of peak render as invisible (noise floor).

**Edge fades** dim the outermost 3 characters on both left and right edges (35%->60%->82% brightness ramp) so data fades in on entry and fades out on exit. Uses `dimmedFg()` which blends severity color toward black via Lab space.

## Gauges

Harmonica spring physics (critically damped, 30fps) for smooth numeric readout easing. **Sparklines scroll at animation rate (30fps)**, not collector rate: each `AnimTick` pushes the spring-interpolated value into a per-gauge `renderHistory` buffer, and sparklines render from this buffer. This decouples scroll speed from data collection.

**Per-slot scale strategy** — not uniform across slots:
- **CPU, MEM**: fixed max=100 (percentage signals).
- **Decomp (SWAP label)**: autoscaled via `g.peaks[slot]` — visible-window peak with fast decay (0.982/tick at 30fps, ~1.3s half-life). Decomp rates span many orders of magnitude, so autoscale is the right call.
- **Token, Pretty**: fixed max at the warn threshold (`TokenSparkThresh().Warn`, default 1000). The autoscale would shrink the scale to small bursts and re-amplify low values to full height — reads as the bar "growing" during idle when the underlying signal is flat. Heavy bursts clip past 100% but the color logic already signals red at crit so the "this is a lot" signal isn't lost.

## Sparkline visibility

5 slot constants (CPU, MEM, Decomp, Token, Pretty) in `widgets/gauges.go`; at most `MaxVisibleSparklines=3` render at any time. `Gauges.ToggleVisible(slot)` enforces the cap centrally. Hidden slots skip `RenderSparkline` upstream (never post-filter marker-bearing strings — bubblezone policy), but their springs + `renderHistory` keep updating so toggling on doesn't show an empty graph. Default visible set is CPU + MEM + Token; SWAP and PRTY are opt-in via the `1`-`5` keys.

## Render architecture

Collector samples at 150ms, animation tick runs at 30fps (~33ms). Springs interpolate between data arrivals (~4.5 frames per sample). `snapshotMsg` updates spring targets; `animTickMsg` advances springs AND pushes interpolated values into render history, driving sparkline scroll. The two rates are fully decoupled via separate bubbletea messages.

## CPU sampling

Caches the mach host port (avoids port leak) and holds the last computed CPU% when tick deltas are zero (avoids false 0% gaps at 150ms).

## Stats engine

`internal/stats/` persists cross-session agent aggregates that the in-memory ring buffer can't answer ("how many agents this month?", "all-time peak concurrent?"). The on-disk shape is `~/.coolant/stats.json` — distinct from `$TMPDIR/coolant-$USER.events.jsonl` because macOS may purge `$TMPDIR` on reboot.

**Durability split.** JSONL is the streaming source of truth; the cache is durable history. Several fields are *primary* — they cannot be reconstructed from a post-rotation JSONL and must survive every cache-discard / migration path: `records`, `daily` buckets, `first_seen`, `by_type`, `by_project`. Lifetime `by_type` / `by_project` are stored alongside the per-day `ByTypeDay` / `ByProjectDay` maps so a future retention policy aging out daily buckets cannot orphan lifetime totals; a totals-only drift guard at Snapshot time bumps the degraded counter once per Aggregator instance if the two diverge.

**Schema gate (virtual chop).** `Aggregator.Fold` drops events whose `Schema` falls outside `[1, MaxKnownSchema]`. Pre-versioning events (no schema field, parsed as 0) are silently skipped; future schema-N events on an old binary are also skipped. The JSONL is never rewritten — old and new envelopes coexist line-by-line.

**Delta-merge checkpoint dance.** Each Aggregator tracks a `baseline` Snapshot (the on-disk state at last load/checkpoint). At Checkpoint time: re-read disk → compute `delta = current - baseline` → per-key additive merge for `byType`/`byProject`/`daily` → max-merge `records` (newest-`At` tiebreak) → fsync tempfile → rename → fsync parent dir → adopt as new baseline. This handles two thermos checkpointing concurrently without losing increments.

**Concurrency.** `sync.RWMutex` guards all mutable state. `Fold` and `Checkpoint` take the write lock; `Snapshot` takes the read lock. A process-local `sync.Mutex` (`processLock`) serializes Checkpoint within a binary; cross-process `flock` is deferred — the delta-merge math is correct under single-binary concurrency.

**Stale prune.** `Checkpoint` calls `pruneStale(now)` to drop `agentStarts`/`agentMeta` entries older than 24h. Mirrors the bash `_compute_agent_duration` cutoff for agents that crash without emitting `agent.stop`.

**Wiring.** `cmd/thermal/main.go::newModel` constructs the aggregator via `productionStatsConfig()` and calls `AppState.AttachAggregator`. `AppState.HandleEvent` fan-outs to `aggregator.Fold` when attached (nil-safe for tests). A 30s checkpoint goroutine in `Init` final-flushes on graceful shutdown via the `checkpointDone` channel `main()` blocks on post-Run, so process exit can't race the fsync.

**Subcommands.** Both `thermo stats` (user-facing summary, formatted text or `--json`) and `thermo statsdump` (hidden dev/debug raw JSON) are dispatched via first-arg matching in `cmd/thermal/main.go` BEFORE `flag.Parse`. Both share `foldSnapshot` in `cmd/thermal/stats.go` — load cache via `stats.New(cfg)` (read-only), replay JSONL on top in-memory. Neither calls `Aggregator.Checkpoint()` — the dashboard's 30s checkpoint goroutine is the sole writer.
