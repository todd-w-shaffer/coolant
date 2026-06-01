# thermo idle animation freeze (v2 — freeze the breath, not the tick)

## Goal
When the dashboard is idle, freeze ONLY the decorative breathing animation
(heatbloom breath + battery charging breath) and leave the animation tick
running. Sparklines fill/scroll/finish-bursts normally; bubbletea's
identical-frame suppression then drives idle terminal repaints toward zero.
This replaces the shipped-but-wrong v1 approach (stop the whole tick), which
froze the sparklines too and broke the startup reveal and token scrolling.

## Diagnosis
- **Why v1 was wrong:** v1 stopped re-arming the tick when calm, freezing
  EVERYTHING — including the sparkline scroll. So the startup fill-in reveal
  froze half-drawn and the token sparkline froze mid-scroll on every lull
  (operator confirmed both, live).
- **The real mechanism (verified):** bubbletea v2 writes nothing when a frame
  is byte-identical to the last (`cursed_renderer.go:281` early-return;
  `viewEquals` compares Content/modes/cursor, `:790-830`). A flat sparkline
  scrolling renders identically frame-to-frame, so it already costs zero
  repaints — it only repaints while showing real changing content (the reveal,
  a token burst), which is exactly when you want it. **Suppression holds only
  if the ENTIRE composed view is byte-stable**, so any single forever-animating
  widget defeats it for the whole frame.
- **The only forever-oscillators in the idle (non-overlay) view** (enumerated):
  - `heatbloom.go` breath — `sin(breathePhase)`, advances every AnimTick
    regardless of heat; never settles. This is the "breathing strip" the
    operator sees. **FREEZE when idle.**
  - `battery.go:51` charging breath — animates every frame while charging.
    **FREEZE when idle** (decoration). The discharging-low alert pulse
    (`:55-62`) must stay live, but `IsCalm` is already false then
    (`batteryAlerting`), so gating on `IsCalm` keeps it running for free.
  - LCD temp (`segmentreadout.go`) renders the hysteresis-stable integer
    `value` (`:92-104`), but its target is `OverallTemperature` =
    `max(rawCPU, mem)` (`temperature.go:22`); a ±2% raw-CPU jitter flips the
    integer across boundaries every snapshot → repaint. **Fix: feed the
    deadbanded CPU (`displayCPU`) so the target is stable and the integer
    holds.** (Not a freeze; it self-quiesces once the input is stable.)
  - Everything else self-quiesces: gauges springs settle + flat sparkline
    renders identically (`gauges.go`), peak decay clamps to flat, build/shell
    ember is wall-clock and settles, breathedots renders nothing at zero agents.
  - No per-second time-derived text in the always-on view (session uptime is in
    the on-demand intel overlay only, `horizontal.go:351`).
- **Falsifiable load-test (lands first in Phase 2, gates the GPU claim):** drive
  a `Horizontal` to filled + settled + calm, then over 1 sim-second of
  **sub-3%-CPU-jitter snapshots + AnimTicks** assert the composed `View()` is
  byte-identical every frame (distinct == 0). Separately assert a fresh widget
  at idle DOES churn while the sparkline fills (reveal not frozen), and a token
  burst churns (token scroll not frozen). If idle distinct > 0, an un-frozen
  oscillator remains — enumerate and freeze it; the test is the safety net for
  "did we miss one."
- **Test result:** (record when run — pre-freeze, idle distinct ≈ AnimFPS
  because the breath animates; post-freeze, expect 0.)

## Non-goals
- **No stopping the tick** — the tick runs at the (already-lowered) 15fps
  always; the GPU win comes from suppression of byte-identical frames, not from
  halting the loop. (The residual cost is a per-frame View+diff, low-single-digit
  CPU per prior art; the operator's priority is GPU, which goes to ~0 at idle.)
- **No freezing the sparklines** (CPU/MEM/decomp/token/pretty scroll) — they must
  keep filling/scrolling; they self-quiesce when flat. This is the v1 bug.
- **No deadband beyond CPU%** — MEM/GPU/SWAP stay raw (operator's CPU%-only
  scope); they don't jitter enough at idle to flip a rendered value.
- **No touching the 30→15fps base-rate work** — already shipped, good.

## Files to touch
**Phase 1 — revert v1's tick-stop, restore normal animation (fixes the two bugs):**
- `cmd/thermal/main.go` — remove the `animating` field, `wakeCmd`, the
  `animTickMsg` freeze-and-don't-rearm branch, and the `tea.Batch(...,
  m.wakeCmd())` in the snapshot/event handlers. `animTickMsg` returns
  `m, animTick()` unconditionally; snapshot/event return their plain `waitFor*`
  cmd. (Tick always re-arms — back to pre-v1 behavior, so sparklines fill and
  token bursts scroll normally.)
- `internal/layout/framerate_diag_test.go` — rewrite the v1 calm-gated tests to
  the new contract (see Phase 2 load-test); in Phase 1 assert the two
  regressions are fixed: at idle a fresh widget's sparkline still fills
  (distinct > 0 during fill) and a token burst churns.

**Phase 2 — freeze the breath + stabilize the LCD (restores the GPU win):**
- `internal/widgets/headline.go` — `AnimTick()` reads `h.state.IsCalm()` once
  and passes it to the breath sub-widgets' AnimTick.
- `internal/widgets/heatbloom.go` — `AnimTick(calm bool)`: skip the
  `breathePhase` advance when calm (freeze the bloom at its current alpha).
- `internal/widgets/battery.go` — `AnimTick(calm bool)`: skip the charging-breath
  phase advance when calm; the discharging-low meltdown/warn pulse is unaffected
  (calm is already false when `batteryAlerting`).
- `internal/model/temperature.go` — `OverallTemperature` pressure term uses
  `s.DisplayCPUPercent()` (deadbanded) instead of raw `CPUPercent`, so the LCD
  integer doesn't flip under sub-3% jitter.
- `internal/widgets/heatbloom_test.go`, `internal/widgets/battery_test.go` — add
  tests: AnimTick freezes the breath phase when calm, advances when not; battery
  discharging-low pulse still advances regardless.
- `internal/layout/framerate_diag_test.go` — land the idle-byte-stable load-test
  (distinct == 0 under sub-3% CPU jitter with breath frozen).

**Keep (verified independent of the revert):** the CPU% deadband
(`model/state.go` `updateDisplayCPU`/`DisplayCPUPercent`/`displayCPU`/`cpuSeeded`,
`config.DisplayDeadbandPct`, the `rates.go`/`gauges.go`/`horizontal.go` reads)
and the `IsCalm` predicate + calm tracking — `IsCalm` is reused unchanged as the
breath-freeze signal (it already means "no agents + the per-frame sparkline
sources are flat + not battery-alerting", which is exactly "freezing the breath
will make the frame static").

## Failure modes to anticipate
- **Missed oscillator → idle not byte-stable → no GPU win.** The load-test
  (distinct == 0 at idle under jitter) is the safety net; if it fails, something
  in the composed view still animates — enumerate and freeze it.
- **Froze something that should animate** (sparkline fill, token burst) — caught
  by the Phase-1 "sparkline fills at idle / burst churns" assertions.
- **Battery alert frozen.** Must NOT freeze the discharging-low pulse. Gating on
  `IsCalm` covers it (calm is false when `batteryAlerting`); test both states.
- **Threading `calm` via AnimTick signature change** — only `headline` calls the
  battery/heatbloom AnimTick; update those call sites, keep `layout` calling
  `headline.AnimTick()` no-arg.
- **Freeze too eager at start/transition** — `CalmStableSnapshots` (~5 snapshots)
  keeps the breath alive briefly after activity stops, so it settles to a rest
  frame rather than freezing mid-pulse (prior-art gotcha).
- **A future per-second clock in the always-on view** would silently defeat
  suppression even with animation frozen — verified none today (uptime is
  overlay-only); flag if one is added.
- **Resize / mouse-mode toggle** force a one-off repaint (expected, not per-frame).

## Done criteria
- Load-test green: idle composed `View()` byte-stable (distinct == 0 over 1s)
  under sub-3% CPU jitter with breath frozen; sparkline fills at idle and token
  burst churns (both distinct > 0).
- Widget tests green: heatbloom/battery freeze breath when calm, advance when
  not; battery discharging-low pulse advances regardless.
- Manual: startup reveal plays fully; token sparkline scrolls smoothly through
  lulls (no pauses); at true idle the breathing stops (static decoration) and
  GPU drops; activity resumes the breath within a tick.
- The two operator-reported regressions (startup freeze, token pause) are gone.
- `cd thermal && go test ./...` green (report count); `bats tests/` green.

## Parking lot
- v1 commits (3f67d69 freeze mechanism, 8f50400 deadband) are local, not pushed.
  Phase 1 reverts the 3f67d69 tick-stop machinery; the 8f50400 deadband is kept
  and extended (temperature.go). Decide at implementation whether to revert-
  forward or reset+recommit for clean history.
