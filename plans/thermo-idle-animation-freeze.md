# thermo idle animation freeze (drive idle repaints to ~0)

## Goal
When the dashboard is calm, stop re-arming the animation tick entirely so
breathing/sparkline motion freezes (still rendered, just static), and
bubbletea's identical-frame suppression drops terminal writes to ~0. Resume
instantly on activity. Separately, deadband the jittery metric readouts so a
±1–2% wiggle doesn't force a redraw. This is the *opposite* lever from the
deferred adaptive-burst Phase 2 — slow DOWN when quiet, not burst up.

## Diagnosis
- **Hypothesis:** the always-on tick (breathing phase + sparkline history push,
  advanced every frame in `layout.AnimTick`) is what produces ~15 distinct
  frames/sec when nothing is happening. Phase 1 already proved animation (not
  data) drives per-frame redraw (`framerate_diag_test.go`: 15/15 distinct at
  steady state). bubbletea writes nothing when a frame is byte-identical to the
  prior (`cursed_renderer.go:281`, verified). So if the tick stops while calm,
  distinct frames → ~0 → writes → ~0.
- **Falsifiable test (lands FIRST in Phase 1, gates the rest):** extend
  `framerate_diag_test.go` with a calm scenario — push one steady snapshot, no
  agents, then drive a loop that advances `AnimTick` **only while
  `state.IsCalm()` is false** (mirroring main's gated re-arm). Assert: distinct
  frames over 1 simulated calm second == 0; then inject activity (an agent / a
  metric jump past deadband) and assert distinct frames climb back toward
  `AnimFPS`. If freezing does NOT drop distinct frames to ~0, the calm predicate
  or the gate is wrong — stop and re-explore before wiring main.
- **Test result:** (to record when run — red before implementation: today the
  loop churns 15/15 even when calm because nothing gates the tick).

## Non-goals
- **No adaptive-burst Phase 2** (the `plans/thermo-adaptive-framerate.md` idea):
  no variable burst rate, no per-widget `IsSettling`. Different direction.
- **No change to what breathing looks like** — it stays rendered at its frozen
  phase; we only stop advancing it. (Operator likes the look.)
- **Deadband applies to CPU% only** (operator's explicit choice, overriding the
  default-include-all-peers recommendation). MEM/GPU/SWAP % text stays raw.
  Rationale for the asymmetry: CPU% is the one metric observed to jitter at idle;
  MEM/GPU/SWAP are effectively flat when calm, so they neither flicker the text
  nor block calm in practice. Token counters are excluded too (monotonic — a
  deadband would wrongly suppress real growth). **Risk accepted:** if a non-CPU
  metric turns out to jitter at idle in some setup, it would cause occasional
  idle flushes and briefly delay freeze (transient, not broken) — extending the
  deadband is parked as a follow-up.
- **No freeze while a battery warning/meltdown pulse is active** — that pulse is
  an alert, not decoration; calm must exclude it (see Failure modes).
- **No `tea.WithFPS` change** — the 60fps flush cap is independent of our tick
  and harmless (identical frames flush as no-ops).

## Files to touch
**Phase 1 — freeze the tick when calm (the core win):**
- `thermal/internal/model/state.go` — add `IsCalm() bool`: no active agents
  (`AgentCount()==0`, :409 — stale agents are a subset of `activeRecords`, so
  this covers them; no separate `StaleAgentCount` walk on the hot path) AND the
  per-frame-animated signals have been stable for ≥K consecutive snapshots
  (K chosen ≥ spring-settle ≈ 6 frames, so springs are at rest by the time calm
  asserts — avoids freezing a gauge mid-ease) AND not battery-alerting.
  **"Stable" = the five sparkline sources** that scroll on every AnimTick
  (`gauges.go:155-159`: CPU%, MEM%, decompression delta, and the two
  token-per-sec rates), compared at display granularity. These — not an
  arbitrary CPU/MEM/GPU/SWAP list — are the values whose sparklines would lie
  (frozen waveform while data streams) if the freeze engaged mid-motion. GPU and
  swap are intentionally excluded: they render as static text with no per-frame
  scroll, so the snapshot path repaints them fine even while frozen. Track the
  stability counter in `Update` (:129). Reuse existing `AgentCount` /
  `AgentStaleThreshold` (:33) — do not invent new agent bookkeeping.
- `thermal/cmd/thermal/main.go` — add `animating bool` to the model struct.
  `animTickMsg` handler (:336): advance `AnimTick`, then if `IsCalm()` set
  `animating=false` and return `m, nil` (stop); else re-arm `animTick()`.
  `snapshotMsg` (:282) and `gateEventMsg` handlers: if `!m.animating &&
  !IsCalm()` set `animating=true` and re-arm `animTick()` once. The bool makes
  re-arm idempotent across the two wake sources — the verified fix for the
  double-arm/frame-doubling hazard (`tea.go:858`). Value receiver persists the
  flag as long as we return `m`.
- `thermal/internal/layout/framerate_diag_test.go` — add the calm-freeze
  load-test described in Diagnosis (gated `AnimTick` loop; distinct≈0 when calm,
  churns on activity + wake).
- `thermal/internal/model/state_test.go` — unit-test `IsCalm()` transitions:
  agents present → not calm; agent goes away + metrics settle K frames → calm;
  metric jump → not calm again; battery alert → not calm.

**Phase 2 — deadband the CPU% readout (operator's second ask, CPU%-only scope):**
- `thermal/internal/model/state.go` — compute a committed *display* CPU% once on
  `Update`: hold the last displayed value; update it only when raw CPU% moves
  ≥ `DisplayDeadbandPct` (default 3) from the **last displayed** value, OR a
  max-staleness timeout fires (force-refresh every ~2s) to defeat slow-drift
  stall. Raw CPU% stays untouched for threat level / stats — display-only layer.
  This committed value also feeds the calm stability check above (single source).
- `thermal/internal/widgets/rates.go` — read the committed display CPU%
  (`:84` currently `int(snap.System.CPUPercent)`); MEM/GPU/SWAP at `:85-89` stay
  raw.
- `thermal/internal/widgets/gauges.go` — feed the CPU gauge spring target from
  the committed CPU% so a sub-deadband wiggle doesn't perturb its spring (keeps
  it at rest → keeps calm reachable; bar/text stay consistent). Other gauge
  targets unchanged.
- `thermal/internal/config/tuning.go` — add `DisplayDeadbandPct` (3). (No
  `DisplayMaxStaleSec` — comparing against the last *displayed* value already
  defeats slow drift, and a forced periodic refresh would re-introduce the
  flicker the deadband removes.)
- `thermal/internal/layout/horizontal.go` — `idleView` renders a second CPU%
  readout (`:559`); switch it to the deadbanded value too, else the idle screen
  flickers and repaints ~6.7×/sec while the tick is frozen. (Added during the
  review pass — it's a missed CPU display path, not new scope.)
- *Deliberately NOT deadbanded:* the LCD temperature pressure term
  (`model/temperature.go:22`) keeps using raw CPU. It's a spring-eased focal
  readout that freezes with the tick when calm (no flicker, no repaint), it
  doesn't gate calm, and dampening the headline temperature is outside the
  CPU%-readout scope.

## Failure modes to anticipate
- **Double-armed tick → doubled/tripled frame rate.** Two wake sources
  (snapshot + event) both re-arming. Mitigated by the `animating` bool armed
  only on the false→true transition; `animTickMsg` is the sole disarm. Asserted
  by a test that wakes via both paths and counts one timer's worth of frames.
- **Stuck frozen (never wakes).** If the wake predicate is wrong the strip
  freezes permanently. Snapshots arrive every 150ms regardless and re-check
  calm, so the wake path is exercised constantly — but the load-test must prove
  a metric jump and an agent-start both re-arm within ~1 tick.
- **Slow-drift deadband stall.** A value creeping <3%/step under the band never
  updates. Mitigated by comparing to last *displayed* value + max-staleness
  force-refresh. Unit-tested with a monotonic slow ramp.
- **Freeze mid-spring-ease → gauge bar stuck off-target.** Mitigated by K-frame
  stability (≥ spring settle) in the calm predicate; never freeze the frame the
  target last changed.
- **Battery alert pulse frozen.** A low-battery warning/meltdown breath is an
  alert; freezing it hides danger. Calm predicate explicitly excludes the
  battery-alert state.
- **Cursor blink defeats identical-frame suppression** (`viewEquals` compares
  cursor blink/pos). Verify thermo runs cursor-hidden in alt-screen; if a
  blinking cursor is emitted, frozen frames would still flush. Confirm before
  claiming zero writes.
- **Widget tests that loop `AnimTick` directly** still pass — the freeze lives
  in main's re-arm decision, not inside `AnimTick`, which stays callable and
  unchanged. (Verified: breathedots/headline/gauges tests call the widget method
  directly.)
- **Sparkline shows a time gap after a long calm period** — scroll resumes from
  the frozen frame, compressing idle time out of the history. Acceptable
  (arguably better — idle isn't interesting); noted, not fixed.

## Done criteria
- Load-test green: distinct frames over 1 simulated calm second == 0; activity
  (agent or metric jump) re-arms and frames climb back toward `AnimFPS`.
- `IsCalm()` unit tests green across the transition matrix incl. battery-alert
  exclusion and slow-drift deadband.
- Manual: run `thermo` live; with no agents and flat metrics the breathing
  visibly stops (still drawn), GPU/CPU drops further than the 15fps baseline;
  starting an agent or a real CPU spike resumes motion within ~1 tick.
- CPU% (and MEM/GPU/SWAP) text no longer flickers on ±1–2% jitter but still
  updates on a real move and at least every ~2s.
- `cd thermal && go test ./...` green (report count). `bats tests/` green.
- Cursor-hidden confirmed (or frozen-frame writes explained).

## Parking lot
- If a non-CPU metric (MEM/GPU/SWAP) turns out to jitter at idle on some setup,
  extend the display deadband to it (operator chose CPU%-only scope for now).
