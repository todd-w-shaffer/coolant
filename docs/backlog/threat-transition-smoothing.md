# Spec stub: smooth threat-level transitions (color blend + time hysteresis)

**Status:** backlog / stub
**Seeded:** 2026-04-12

## Problem

The headline thermal bar snaps between threat states (COOL → WARM → HOT → MELTDOWN) with zero transition. Two problems compound:

1. **Visual jolt** — a single frame flips the entire palette. No fade, no decay, no interpolation. The bar goes from cool cyan to warm amber in one 150ms repaint, which reads as broken rather than intentional.
2. **No time tolerance** — a 1ms spike past a threshold flips state immediately. A transient CPU burst or single sampled metric blip can promote the bar to HOT for one frame and back. No hysteresis, no dwell requirement, no EMA gate on state changes.

The Iron palette already demonstrates the target feel: within a single severity it fades smoothly through the blackbody gradient (purple → magenta → amber). The state-level transitions should echo that internal continuity.

## Proposed approach

Two orthogonal mechanisms:

**A. Color transition (visual blend)**
- When threat state changes, don't swap palettes instantly. Ease the rendered color from old-state color to new-state color over N frames (candidate: 300–600ms, tunable in `anim.Profile`).
- Implementation: track `currentThreat` + `targetThreat` + `transitionProgress` in `AppState`. Use HCL interpolation (already used for severity gradients in `theme.Init()` LUTs) to blend.
- Harmonica spring could drive progress for organic easing (already a project dependency).

**B. Time hysteresis (state-change gate)**
- A threshold crossing must persist for M consecutive samples (or EMA-smoothed value must cross, not raw) before the state actually changes.
- Asymmetric dwell: escalating (COOL → HOT) can be faster than de-escalating (HOT → COOL) so danger shows quickly but doesn't flicker back and forth at the boundary.
- Candidate: 3–5 samples to escalate, 8–12 to de-escalate. Tune via `config/tuning.go`.

Both should live in `model/threat.go` (extend `Classify`) and `model/state.go` (hold transition state), consumed by widgets that color-key off threat level.

## Open questions

- Does the blend apply to every widget that color-keys off threat, or only the headline? (Likely only the headline — other widgets use severity-gradient-per-value, which already blends continuously.)
- How do we handle MELTDOWN pulse animation during a transition *into* MELTDOWN — does the pulse ramp up with the blend, or kick in at the end?
- Interaction with the segment readout ghost trail: if the trail lags the current value, should its color lag the transition too? Probably yes, for visual coherence.
- EMA vs. N-sample dwell for hysteresis — EMA is already ubiquitous in the codebase, might be the more consistent choice.
- Does this require a new `Profile` tunable or hardcoded constants? Probably `Profile` so Calm and Intense can differ.

## References

- `thermal/internal/model/threat.go` — current `Classify` (hard cutover)
- `thermal/internal/model/state.go` — `AppState`, rolling history, smoothed counts
- `thermal/internal/theme/theme.go` — `Theme.Init()` HCL blend LUT pattern to reuse
- `thermal/internal/anim/profile.go` — tunables live here
- `thermal/internal/widgets/headline.go` — primary consumer
- Memory: `reference_hcl_dim_pattern.md` — existing blend-via-HCL precedent
