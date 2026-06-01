# thermo power-efficiency roadmap (cool to the touch on a laptop)

## Status (2026-06-01)
- **Phase 1 shipped** — `4eb8a79` (GPU shell wrapper dropped; `ps` parse in place).
- **Phase 2 shipped** — `dc79b99` (`ps` scan moved 150ms → 1s slow loop; spawn/death
  rate gated on `ProcSeq` freshness).
- **Phase 3 shipped** — `90cec59` (per-source adaptive cadence: battery 20s, GPU
  15s flat/1s active, network 12s/3s; vm_stat+swap fixed 1s).
- The whole **collector tier (1-3) is done** and on branch `token-counter-on-main`
  (not pushed). All green: 18/18 Go packages, 286/286 bats.
- **Phases 4-7 deferred** to a later session. Phase 4 needs a re-plan first —
  see the ⚠ note on Phase 4 below.

## Goal
Cut the idle CPU/power the always-on thermal strip burns. It currently holds
~5–7% of a core steadily even when the machine is 95% idle (measured below),
doing repeated work — a 15 Hz render wakeup, a 6.6 Hz `ps` fork+exec, a 1 Hz
subprocess storm — only to confirm nothing changed. Ship the fix as
independently-landable phases, cheapest/safest first.

Findings backing every claim: `docs/_drafts/efficiency/` — `00-roadmap.md`
(synthesis) and `01`–`05` (per-dimension, file:line-cited this session).

## Diagnosis  *(perf plan — hypothesis confirmed before scoping)*
- **Hypothesis:** thermo's idle CPU is dominated by three always-on loops that
  run regardless of activity: the 15 Hz bubbletea anim tick (`main.go:336-338`),
  the 150 ms `ps` fork+exec (`procs.go:134`), and the 1 s subprocess fan-out
  (`ioreg`/`pmset`/`vm_stat`/`sysctl`, `system.go:104-119`).
- **Falsifiable test:** measure (a) a running thermo's idle CPU, (b) per-call
  cost of `ps`/`ioreg`/`pmset`, (c) whether the anim tick re-arms unconditionally.
- **Test result (run this session):**
  - Running thermo at 95% system-idle: **~5–7% of a core instantaneous,
    16.3% lifetime avg** (`top`/`ps`, pid 23238). Real, measurable idle cost.
  - `ps -Ao …` ~20 ms wall × 6.6/s; `bash|grep|ioreg` ~17.5 ms; `pmset` ~6 ms
    (`01`/`02` measured tables).
  - Anim tick re-arms unconditionally — `case animTickMsg: … return m, animTick()`
    at `main.go:336-338`, no calm gate (post-revert `11cde8c`). Confirmed.
  - bubbletea v2: returning `(m, nil)` does NOT quit (`tea.go:693-695,724`);
    re-arm is NOT idempotent — stacked ticks double the rate (#436). Verified.
  Hypothesis confirmed; scoping the fix.

## Phases (each independently shippable; commit per phase)

**Phase 1 — easy collector wins, no scheduler.** (`01` F1, `02` F5)
- `system.go:113` — call `ioreg -r -d 1 -c AGXAccelerator` directly; drop the
  `bash -c "… | grep …"` wrapper. `parseGPU` (`system.go:179-193`) already
  regex-filters, so no parser change. Removes 2 forks/sec.
- `procs.go:148` — replace `strings.Split(string(psOutput), "\n")` with an
  in-place `[]byte` `\n` scan in `buildTrees`; kills the proc loop's last big
  per-tick allocation. Existing `buildTrees` tests cover it.

**Phase 2 — decouple the `ps` scan from the 150 ms CPU cadence.** (`02` F2)
- `collector.go` — move `pc.Collect` out of `collectFast` and into the existing
  1 s slow-loop `wg` fan-out (alongside `SlowCollect`/`NetCheck`/token tick);
  cache `[]SessionTree`/`[]ProcessInfo` as mutex-guarded last-known fields and
  merge into each fast snapshot, exactly as `slow`/`online`/`tokens` already are
  (`collector.go:139-149`). CPU mach sampling keeps 150 ms. `ps` fork+execs drop
  ~85% (6.6→1/s). Eyeball spawn/death-rate EMA in `--demo` (slower cadence
  widens the rate window — arguably more correct, confirm it reads right).

**Phase 3 — per-source adaptive cadence scheduler.** (`01` F2/F3/F4, `02` F4)
- New per-source ticker abstraction replacing the single shared 1 s `netTicker`
  (`collector.go:80`). Add interval constants to `config/tuning.go`.
  - Battery → 15–30 s (once-per-minute value, highest-latency probe). (`01` F3)
  - GPU → adaptive: back off to 10–30 s when reads stay below a low threshold,
    snap to 1 s on a crossing. Hysteresis + a hard max-interval cap so a spike
    is never invisible >N s. Thread the last value into the scheduler (the
    structural fix — the cadence decision needs the value). (`01` F2)
  - Network → 10–15 s when last-known-online, fast retry (2–3 s) on failure.
    (`01` F4)
  - Battery-aware: widen CPU + proc tickers on `BatteryDischarging` (state
    already in-snapshot, `types.go:30`). (`02` F4)
  - **vm_stat + swap stay fixed at 1 s** (see Non-goals).

**Phase 4 — event-driven anim tick-stop (the idle battery prize). ⚠ NEEDS RE-PLAN
before implementing.** (`05` F4)
- **Why re-plan (found 2026-06-01 while scoping P4):** the planned
  `AllSparklinesFlat()` predicate is INSUFFICIENT — idle byte-stability has motion
  sources beyond sparklines, so gating the tick-stop on `IsCalm() &&
  AllSparklinesFlat()` would freeze them mid-animation (same class as the v1
  revert):
  - **KITT highscore scanner.** `breathedots.go:98` advances `staleSweep +=
    KITTSweepRate` on EVERY AnimTick, unconditionally. Completed agents
    (`CompletedAgentCount`) are NOT in `IsCalm`'s `AgentCount()` check
    (`state.go:262` vs `:580`), so with completed dots on screen + flat metrics,
    `IsCalm()` is true while the KITT scan animates every frame. Tick-stop →
    frozen KITT sweep.
  - **Peak-marker decay.** `gauges.go:224` decays `peaks[i] *= decayRate` each
    tick after a spike, moving the rendered marker until it settles.
  - A hand-enumerated "everything flat" predicate must catch every source
    (springs, reveal, scroll, peak decay, KITT scan, breath, LCD temp, battery
    pulse). Miss one → freeze. This is the trap the `05` findings warned about.
- **The fix: merge P4 with P5.** The robust settle signal is ACTUAL byte-stability
  of the composed frame — the render signature P5 builds. Build the signature
  first, gate the tick-stop on `IsCalm() && signature unchanged for K frames`,
  and reuse the same signature for memoization. Then the v1 machinery applies
  cleanly (verified intact at revert `11cde8c`):
- Re-introduce the reverted machinery, gated on the *settle* signal (render
  signature stable), NOT the weaker `IsCalm()` alone:
  - `main.go:336` `animTickMsg`: when settled, return `m, nil` (stop re-arming);
    else `m, animTick()`.
  - `wakeCmd` re-arm from `snapshotMsg`/`gateEventMsg` (`main.go:282,296`), with
    the `animating bool` idempotency guard (bubbletea ticks don't dedupe, #436).
  - The 150 ms snapshot stream stays the unconditional wake source → can't latch.
  - Breath freeze already shipped (precondition).

**Phase 5 — compositor frame memoization + fold in `zone.Scan`.** (`03` F3/F4)
- `layout/horizontal.go:200` / `main.go:353` — hash the display-granularity
  inputs (the set `calmSignals` already computes, `state.go:223-232`) + anim
  phase counters + ALL mode/overlay/hover/filter/agent-spring/KITT/LCD-ghost/
  notification flags; on a hit, return the cached *scanned* string and skip both
  the rebuild and `zone.Scan`. Do it at the layout boundary, not per-widget.
  (lipgloss `Style` reuse + rendered-string caching verified sound.)

**Phase 6 — active-frame allocation cleanup (helps when busy).** (`03` F1/F2/F5/F6)
- `headline.go` — cache invariant `lipgloss.Style` structs (bgPad, cat-cell per
  gradient level, rail text/pad) instead of per-cell `NewStyle().Render()`
  (~40–60% of active allocs); `bloomedBgPad`/`truecolorBg` (`:142-171`) →
  manual base-10 int format instead of `fmt.Sprintf`.
- `sparkline.go` — pooled braille builders via the existing `SparkBufs`.
- `rates.go` — cache the four fixed-color ANSI prefixes instead of `ColorText`
  per call.

**Phase 7 — cgo/mach/IOKit subprocess elimination (research-gated).** (`01` F6)
- Begins with a cgo smoke-test confirming values match the subprocess output on
  arm64+amd64, then migrates, matching the `cpu_darwin.go` idiom (cache
  `mach_host_self()` once; `CFRelease` IOKit objects each sample):
  - vm_stat → `host_statistics64(HOST_VM_INFO64)` → `vm_statistics64` (verified
    `<mach/mach_host.h>`, `<mach/host_info.h>`, `<mach/vm_statistics.h>`).
  - swap → `sysctlbyname("vm.swapusage")` → `struct xsw_usage` (`<sys/sysctl.h>`).
  - battery → `IOPSCopyPowerSourcesInfo`/`IOPSGetPowerSourceDescription` with
    `kIOPSCurrentCapacityKey`/`kIOPSMaxCapacityKey`/`kIOPSIsChargingKey`/
    `kIOPSTimeToEmptyKey` (verified `<IOKit/ps/IOPowerSources.h>`,`IOPSKeys.h`).
  - **GPU stays on the subprocess** (see Non-goals).
  Removes ~3 forks/sec. Note: if Phase 3's battery cadence already cut battery
  forks ~95%, the battery cgo migration's marginal value drops — reassess its
  worth at this point rather than doing it reflexively.

## Non-goals
- **GPU is NOT migrated to cgo (Phase 7 excludes it while vm_stat/swap/battery
  are included).** Verified this session: there is no public macOS API for GPU
  "Device Utilization %"; the only route is the same private `AGXAccelerator`
  IORegistry path `ioreg` already walks, so cgo trades one fragile undocumented
  surface for another with zero stability gain. Phase 3's adaptive cadence
  already captures ~95% of GPU's cost at far less risk — that's the safe split.
- **vm_stat + swap stay on a fixed 1 s cadence (excluded from Phase 3's adaptive
  treatment).** The SWAP sparkline shows a *per-tick decompression delta*
  (`system.go:63-77`); changing the interval silently re-bases the delta's time
  axis and corrupts the sparkline's meaning. They're also the two cheapest
  probes (~1 ms), so adaptive cadence buys ~nothing. Safe because they share no
  state/queue/ordering with the adaptive sources — each slow-loop metric is an
  independent last-known field merged under the same mutex (`collector.go:139-149`).
- **No throttle on activity rates (warm/cool/net) or counters (tok/bill).** Owned
  by the separate `plans/thermo-readout-throttle.md`; deadbanding rates/counters
  hides real signal.
- **Token transcript scan is NOT reworked** beyond an optional stat-before-open
  guard (parking lot) — it's already offset-based, discovery-throttled, and
  near-zero when idle (`04`, verified optimal).
- **No render-pipeline re-architecture beyond memoization.** No `lipgloss.Canvas`
  (breaks bubblezone, per `thermal/CLAUDE.md`).
- **No FPS reduction or focus/blur gating** as a substitute for the settle-gated
  tick-stop — those are coarser and don't reach true idle quiescence.

## Failure modes to anticipate
- **Tick re-arm stacking → doubled frame rate.** bubbletea ticks aren't
  idempotent (verified, #436); two wake sources double-arm without the
  `animating bool` guard. Phase 4 must restore it.
- **`AllSparklinesFlat` incomplete → reintroduces the v1 freeze.** Must cover
  every visible slot, the full visible span, AND the reveal-not-finished case.
  The two `framerate_diag_test.go` tests (startup reveal, token scroll) are the
  regression contract — they must stay green.
- **Memo key incomplete → stale frozen screen (Phase 5).** Miss one input
  (overlay/hover/filter/agent spring/KITT phase/LCD ghost/notification) and the
  screen freezes wrong. Conservative explicit signature at the layout boundary.
- **Adaptive cadence flapping (Phase 3).** No hysteresis on the GPU threshold →
  oscillation. Add a deadband; cap the max interval so a transient is never
  hidden longer than N s.
- **Decoupled proc cadence shifts spawn/death-rate EMA window (Phase 2).**
  Validate in `--demo`.
- **cgo correctness (Phase 7).** `mach_host_self()` leaks a send right per call
  if not cached; IOKit CF objects must be `CFRelease`d each sample; both arches
  need testing. Smoke-test gates the phase.
- **Liveness / freeze latch (Phase 4).** Guaranteed only because the 150 ms
  snapshot stream is unconditional; any future snapshot back-off must preserve a
  wake edge — that's why it's parked, not in scope.

## Done criteria
- Idle thermo CPU measurably lower than the ~5–7%/core baseline (re-measure with
  `top` at calm-but-online; target a clear drop after Phases 1–4).
- Phase 1: GPU path forks only `ioreg`; proc parse allocates no full-output copy;
  all collector tests green.
- Phase 2: `ps` runs ≤1×/s; CPU sparkline still samples at 150 ms; tests green.
- Phase 3: battery/GPU/network on adaptive intervals with hysteresis + max cap;
  vm_stat/swap unchanged at 1 s; new cadence tests green.
- Phase 4: at idle-settled the anim tick stops re-arming and wakes within one
  snapshot on activity; both existing framerate tests green + a new
  settled-stops-then-wakes test.
- Phase 5: a calm/online frame collapses to a hash compare (no rebuild, no
  `zone.Scan`); a new memo-correctness test proves every tracked input
  invalidates the cache.
- Phase 6: headline active-frame allocs down ~40–60% (the added bench is the
  guard); tests green.
- Phase 7 (if pursued): vm_stat/swap/battery sourced in-process on both arches,
  values match subprocess output within tolerance; GPU still subprocess.
- Each phase: `cd thermal && go test ./...` green (report count); `bats tests/`
  green where touched.

## Parking lot
- `parseVMStatField` (`system.go:157`) still does `strings.Split(string(vmstat),…)`
  and re-splits the blob 4×/tick (the pattern Phase 1 migrated procs.go away
  from). **Covered by Phase 7** — the `host_statistics64` cgo migration deletes
  vm_stat text parsing entirely, so optimizing the parser now is throwaway. Left
  as-is deliberately; if Phase 7 is dropped, fold a `bytes.Lines` single-pass
  rewrite here instead. (Surfaced by Phase 1 `/code-review`.)
- Token scan stat-before-open guard — skip open+seek when a file didn't grow
  (`04` F5). Easy, small.
- Calm-aware snapshot back-off below the post-Phase-4 6.6 Hz idle floor (`05` F5
  / `02` F3) — only after Phase 4; must preserve an unconditional wake edge;
  needs a model→collector signal. Separate spec.
- Calm-gated proc backoff (`02` F3) — same model→collector signal; revisit if
  profiling still shows warmth at idle after Phases 1–4.
- Timer-coalescing tolerance (Apple guidance) — Go's `time.Timer` has no
  tolerance API; revisit only if a cgo timer path is ever justified.
