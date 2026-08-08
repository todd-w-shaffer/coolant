# CPU sparkline gaps

## Goal
Stop the occasional blank gaps of varying length that flow leftward through the
CPU sparkline. A present data point — even 0% — should render as the baseline
dot, never as a hole.

## Diagnosis  *(confirmed empirically, this session)*
- **Hypothesis:** the gaps are genuine `0%` CPU readings rendered as blank cells,
  not a sampling failure. A fully-idle 150ms interval makes `busyDelta == 0`
  (`cpu_darwin.go:88-89`) so the sampled value is exactly `0`; the kernel's CPU-tick
  counter refreshes coarser than 150ms, so that `0` is *held* across several
  reads, and the render side turns any value `<= 0` or `< peak*0.02` into a literal
  space.
- **Mechanism (verified):**
  - `valueToLevel` returns level **0** for `v <= 0` and (linear) `v < peak*0.02`
    (`sparkline.go:89,97`).
  - The CPU slot's `peak` is a fixed **100** (`gauges.go:308`), so its floor is a
    hard **2%** — anything in `[0%, 2%)` is level 0.
  - Level 0 renders as a literal space (`renderBrailleChar`, `sparkline.go:26`).
  - The sparkline draws from the spring-smoothed history at 15fps, so one idle dip
    parks the spring in the `[0,2%)` dead zone for a *variable* number of frames →
    a blank run of varying length scrolling left.
- **Falsifiable test (run):** sampled the real CPU path at 150ms for ~300 reads.
  Result: the only sub-2% values were **exactly `0.0000%`, in consecutive runs**;
  `host_statistics` failures = **0**, counter-backwards = **0**, but kernel
  ticks-unchanged = **150/300 (50%)**, often in runs of 5–6. Confirms genuine-zero
  + coarse-cadence hold, and rules out the unchecked-return / wrap artifacts as the
  cause of the everyday gap.

## Non-goals
- **Changing the CPU sample rate.** 150ms oversamples the kernel ~2×, but the read
  is a near-free in-process mach call, the faster rate buys spike responsiveness,
  and slowing it would *widen* the held-zero gaps. Out of scope.
- **The offline/network rainbow path** (`sparkline.go:356-363`) — that's the
  legitimate "source offline" rendering and stays as-is.

  *Peer-set decision (confirmed): phase 1 applies to all 5 sparkline slots.* The
  render core is shared by CPU/MEM/Decomp/Token/Pretty. The fix removes interior
  holes for every slot — correct uniformly, since a present data point should never
  be a hole on any of them. The visible consequence is that idle Token/Pretty
  throughput (0 io/s) renders a flat baseline instead of blank — accepted.

## Approach
Give the renderer one unambiguous "no data" signal instead of conflating it with
"zero value":
1. **Pad with `NaN`, not `0`** (`prepareSparkDataBuf`, `sparkline.go:183`). NaN is
   the single "absent" sentinel — distinct from a real 0% and immune to spring
   undershoot (a negative sentinel would collide with spring overshoot below 0).
2. **Render core blanks only absent/offline cells** (`renderSparklineCore`): a
   `NaN` sample → space (as padding does today); a finite present sample → braille
   dot. Offline (mask=false) → rainbow, unchanged. NaN never wins peak detection
   (`v > peak` is false for NaN), so autoscale (Decomp) is unaffected.
3. **`valueToLevel` floor → baseline, not blank**: drop the early `return 0`
   branches; a present value maps to **minimum level 1** (the existing `level < 1
   → 1` clamp). Height-suppression of jitter is preserved (low values sit at the
   baseline dot); only the *hole* goes away.

## Files to touch
- `thermal/internal/widgets/sparkline.go` — NaN padding in `prepareSparkDataBuf`;
  NaN-absent branch + present→min-level-1 in `renderSparklineCore`; drop the
  `return 0` floor branches in `valueToLevel`.
- `thermal/internal/widgets/sparkline_test.go` — update `TestValueToLevelZero`,
  `TestValueToLevelNegative`, `TestValueToLevelNoiseFloor` (now expect level 1) and
  `TestPrepareSparkDataBufPadsShort` (now pads NaN); add: present-0-renders-baseline
  (no space in a full buffer), NaN-padding-renders-blank, offline-still-rainbow.
- **(phase 2)** `thermal/internal/collector/cpu_darwin.go` — check the
  `host_statistics` return; compute the delta in signed space (hold last value on
  `<= 0`); extract a pure `computePercent(prev, cur, last)` so the guards are
  testable without cgo.
- **(phase 2)** `thermal/internal/collector/cpu_darwin_test.go` *(new)* — scripted
  tick sequences: zero-delta → hold; failed read → hold (not 0); backwards/wrap →
  hold (not 100).

## Failure modes to anticipate
- **Startup baseline-line regression** — padding must stay blank; if NaN isn't
  threaded through interpolation, the first ~5s would show a false flat baseline.
  Test the short-buffer case explicitly.
- **Interpolation boundary** — the midpoint `(NaN + v)/2` is NaN, so the
  padding↔data boundary cell reads as absent (blank). Intended; assert it.
- **Offline precedence** — a real-but-offline cell must still render rainbow, not a
  baseline dot. Check the NaN branch sits beside, not over, the offline branch.
- **Peak detection with NaN** — confirm NaN never becomes the autoscale peak for
  Decomp (`maxOverride <= 0` path), or every cell would normalize to NaN.
- **Width / bubblezone** — a baseline dot and a space are both 1 cell, so swapping
  them can't shift the layout; verify no `lipgloss.Width` math regresses anyway.
- **Token/Pretty idle look changes** — flat baseline instead of blank when idle
  (the peer-set consequence). Acceptable per decision A, but eyeball it in `--demo`.

## Done criteria
- A full-width CPU history containing a `0` renders a baseline braille dot at that
  column, not a space (new test, red→green).
- A short/startup buffer still renders leading cells blank (new test).
- Offline cells still render the rainbow pattern (existing test stays green).
- `go test ./...` green in `thermal/`; bats unaffected.
- Visual confirmation in `./bin/thermo --demo` (and live): no interior holes in the
  CPU sparkline through an idle→busy→idle cycle.

## Parking lot
(empty)
