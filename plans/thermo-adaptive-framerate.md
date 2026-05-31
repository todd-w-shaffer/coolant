# thermo adaptive / lowered frame rate (GPU load)

## Goal
Cut the rate at which thermo produces visually-distinct frames so each session
drives far fewer terminal repaints/sec, reducing GPU load on the user's machine.
The backend stays bubbletea v2 — it is not the problem.

## Diagnosis
- **Hypothesis:** the fixed 30fps animation tick is the GPU driver. `animTick()`
  reschedules unconditionally every `config.AnimInterval` (≈33ms) — `cmd/thermal/main.go:160-164`
  and `:336-338`. Every `AnimTick` advances at least one phase accumulator by a
  nonzero step (breathing `breathedots.go:91-99`, heat-bloom breath `heatbloom.go:110`,
  battery breath/pulse `battery.go:51-60`, gauge spring + sparkline history push
  `gauges.go:162-176`), so `View()` output differs on every frame. bubbletea flushes
  whenever the view changes, capped at 60fps (`tea.go:1372-1400`, `defaultFPS=60`).
  30 < 60 → no coalescing → ~30 distinct frames/sec → ~30 repaints/sec, per session.
  Data only changes at 1Hz (slow) / 6.7Hz (fast) (`config/tuning.go:10-11`), so 30fps
  is far above what the data warrants — the surplus is pure animation.
- **Falsifiable test (gates Phase 1, run before any other Phase-1 edit).** Three
  measurement layers, mechanism → outcome:
  - **Layer 1 — frame production (deterministic, zero noise).** Throwaway
    `internal/layout/*_test.go` harness: build a `Horizontal` via the existing
    `seedActive`-style setup (`horizontal_test.go:17-35`), push ONE steady-state
    snapshot, then loop `h.AnimTick()`+`h.View()` for `AnimFPS` iterations (= 1 sim
    second), counting frames whose `View()` differs from the prior. **Predict:** ≈30
    distinct/sec at 30fps, scaling ~linearly (≈15 at 15fps). This is the clean causal
    proof the cause changed.
  - **Layer 2 — terminal + thermo process CPU (proximate cost; likely the clearest
    real signal per research — CPU frame-rebuild dominates, not GPU).** `ps`/`top`
    sampled on the terminal-emulator PID and the thermo PID over the window.
  - **Layer 3 — system GPU utilization (the target outcome).** Poll the SAME source
    coolant trusts — `ioreg -r -d 1 -c AGXAccelerator | grep 'Device Utilization'`
    (`system.go:113`, regex `"Device Utilization %"=(\d+)` at `system.go:179`, lands in
    `SystemStats.GPUPercent`) — once/sec over the window via a throwaway shell loop,
    averaged. **No sudo** (confirmed: bare `ioreg` returns `"Device Utilization %"=N`).
    Note this is whole-system GPU, so it captures the *terminal's* repaint cost (the
    thing we care about), not thermo's own ~0 GPU. Expected noisy on a small strip
    (Ghostty's per-repaint GPU is near-zero); do NOT over-index — layers 1-2 carry the
    proof if GPU% sits in the noise. `sudo powermetrics --samplers gpu_power` is an
    optional cross-check only if ioreg is too coarse.
  - **Controlled run:** two builds (30fps vs lowered), **interleaved A/B/A/B** to beat
    thermal/background drift; held constant = same Ghostty window + size, **on AC**
    (battery changes GPU clocking), screen on, no other GPU apps; **steady state, NOT
    `--demo`** (demo's scripted data churn isn't the animation-only floor); ~60s/window,
    a few trials. Confound: the running thermo instance also polls ioreg every 1s, but
    identically in A and B, so it cancels in the delta. To isolate one variable, the
    A/B subject can be a SECOND thermo strip in another pane while a quiet reference
    sampler watches GPU%.
- **Test result (2026-05-31):**
  - **Layer 1 (deterministic):** 30/30 distinct frames per simulated second at 30fps in
    full steady state (zero data changes). Diagnosis CONFIRMED — animation, not data,
    drives per-frame redraw. (`internal/layout/framerate_diag_test.go`.)
  - **Layer 2 (frame-production CPU, accumulated CPU-time over 30s window, 3s warmup,
    A/B/A/B under PTY):** 30fps = 3.70% & 3.93%; 15fps = 2.30% & 2.36%. **~37% process-CPU
    reduction at 15fps, dead repeatable.** (Method matters: a 15s window with no warmup
    was within noise — too short; 30s+warmup gives a clean repeatable signal. Instantaneous
    `ps %cpu` reads 0 and is useless here.) Absolute CPU is modest — thermo is a light
    one-line strip — so the bigger prize is whole-system terminal-repaint work, which only
    shows with a visible window (Layer 3, operator).
  - **Layer 3 (terminal GPU%):** PTY runs don't drive a visible window, so terminal
    repaint GPU isn't captured headlessly. Operator script at `/tmp/gpu-ab.sh` runs the
    A/B in a real Ghostty window sampling ioreg. Per research, Ghostty per-repaint GPU is
    near-zero so this is expected in the noise; Layers 1+2 carry the proof.
  - **Decision: base rate = 15fps.** Grounded on Layer 1 (deterministic 30→15 halving of
    frame production) + the design constraint that 15fps keeps the fastest breath (2.2s hot)
    at 33 frames/cycle (smooth) while 12fps risks visible stepping. NOT grounded on Layer-2
    CPU (measurement failed — see above).
- **Key finding that shapes the fix:** breathing, the tidal wave, and the KITT sweep
  are period-based accumulators that **never settle** (`breathedots.go:91-99`,
  `heatbloom.go:110`, `battery.go:51`) — only the harmonica springs reach rest
  (~0.2-0.4s). So the steady-state floor can never drop to zero; **lowering the base
  rate is the dominant GPU lever**, and adaptive bursting (Phase 2) only buys smoother
  spring easing on top — it does not lower the steady-state cost.

## Non-goals
- **No rendering-backend swap** (notcurses/tcell/ratatui). Research confirmed bubbletea
  v2 already does cell-diff rendering + a 60fps flush cap; GPU cost is driven by repaint
  *frequency*, which is set by our frame-production rate, not the library. Swapping
  backends moves PTY bytes, not GPU load.
- **No Rust port / shared-core work.** (For the record: the only true Go+Rust shared
  render core is notcurses via C FFI, but Go has no maintained binding; the pragmatic
  port path is ratatui+crossterm on the Rust side — equivalent diff rendering, no
  shared core needed. Out of scope here.)
- **Phase 2 (adaptive burst) is optional** and gated on a Phase-1 decision — see below.
  Safe to defer because breathing pins the steady-state floor regardless, so Phase 1
  alone delivers the GPU reduction; Phase 2 is spring-easing polish.

## Files to touch
**Phase 1 — measure, pick a base rate, lower the floor + decouple the sparkline window:**
- **Measure-first (gating, before the constant edit):** run the distinct-frames/sec
  harness at 30fps and at candidate rates (12 / 15 / 20), and sample `./bin/thermo --demo`
  process CPU at each. Pick the base rate from the numbers + demo feel — do NOT hardcode
  a value until measured.
- `internal/config/tuning.go` — lower `AnimFPS` from 30 to the measured-and-chosen base
  rate. Single source of truth: all four springs build via
  `harmonica.NewSpring(harmonica.FPS(config.AnimFPS), …)` (`segmentreadout.go:42`,
  `breathedots.go:46`, `heatbloom.go:52`, `gauges.go:95`), so lowering it auto-retunes
  every spring to the same wall-clock easing with fewer frames — no per-widget edits.
  Two other `AnimFPS`-derived steps auto-scale correctly too: heat-bloom breath step
  (`heatbloom.go:109`) and battery meltdown step (`battery.go:18`).
- `internal/widgets/gauges.go` — `MaxRenderHistory` (`:38`, used `:190`) is a raw
  600-frame cap ("~20s at 30fps"). At a lower fps the same 600 frames = a longer
  window and slower scroll. Convert the cap to a duration-derived value
  (`windowSeconds * AnimFPS`) so the displayed time window and scroll speed stay
  invariant to the rate. Add the `windowSeconds` constant to `config/tuning.go`.
- `internal/layout/framerate_diag_test.go` *(new, throwaway)* — the distinct-frames-per-
  second harness from the falsifiable test. Kept as a regression guard or removed at
  ship (operator's call — will ask, per repo no-delete rule).

**Phase 2 — adaptive spring-settle burst (DECIDE AFTER PHASE 1 — do not build until then):**
- `internal/widgets/gauges.go`, `segmentreadout.go`, `heatbloom.go`, `breathedots.go`
  — add an `IsSettling() bool` per widget: epsilon check on returned spring velocity +
  |pos-target| (harmonica exposes no rest helper — `spring.go:216` returns only
  `(pos, vel)`; caller compares against own epsilon).
- `internal/layout/horizontal.go` — `AnimTick()` (`:195-198`) returns an aggregate
  `settling bool` (OR of widget `IsSettling()`).
- `cmd/thermal/main.go` — `animTickMsg` handler (`:336-338`) chooses the next
  `tea.Tick` interval: burst rate while `settling`, base rate otherwise. `tea.Tick`
  takes the duration per-call (`commands.go:154-164`), so a varying interval is the
  correct primitive (NOT `tea.Every`, which snaps to wall-clock boundaries).

## Failure modes to anticipate
- **Variable-dt breaks fixed-dt springs (Phase 2).** Springs are built for dt=`1/AnimFPS`
  (`harmonica.FPS(AnimFPS)`). If the wall-clock tick interval varies (burst↔base), a
  spring tuned for one dt eases at the wrong speed at the other. Phase 2 must keep spring
  *simulation* time fixed (always step springs as if at the burst dt) while only the
  wall-clock emit rate varies — or pass real elapsed dt. This is the hard part and the
  reason Phase 2 is separable.
- **Sparkline window/scroll change (Phase 1).** Already mitigated by the duration-derived
  cap above; if missed, the sparkline silently shows a different time span and scrolls slower.
- **Breathing looks choppy at too-low a base rate.** Heat-bloom breath periods are 2.2s
  (hot) – 6s (cool) (`config/tuning.go:247-248`); at 15fps that's 33–90 frames/cycle
  (smooth). Going below ~10fps risks visible stepping on the fastest (hot) breath.
- **`WithFPS` interaction.** thermo never sets `tea.WithFPS` (`main.go:608`), so the flush
  cap is 60. Keep it ≥ the burst rate so the flush goroutine never clips motion frames.
- **Demo/preview tools.** `bin/swatch --animate` and `--demo` (`demo/`) read the same
  constants; verify animations still look right there, not just live.
- **Tests asserting frame-count-based behavior.** Any test that hardcodes 30fps timing
  or 600-frame history could break — run the full Go suite.

## Done criteria
- Empirical before/after recorded across all three layers: distinct-frames/sec ≈ AnimFPS
  at both rates (Layer 1), a measured drop in terminal + thermo process CPU (Layer 2),
  and system GPU active-residency delta logged (Layer 3, even if within noise).
- `AnimFPS` lowered; all spring easing visually verified unchanged in wall-clock feel
  (`bin/swatch --animate`, `./bin/thermo --demo`).
- Sparkline displayed time window + scroll speed unchanged from the 30fps baseline
  (duration-derived cap).
- `cd thermal && go test ./...` green (report pass count).
- `bats tests/` green (bash layer untouched, but the cycle runs the full suite).
- Phase 2: if approved, adaptive burst verified — springs ease at burst rate, steady
  state holds at base rate, no dt-induced easing speed change.

## Parking lot
(empty)
