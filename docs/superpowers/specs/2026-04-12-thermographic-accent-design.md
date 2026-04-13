# Thermographic accent layer — design

**Date:** 2026-04-12
**Status:** design, pending user approval
**Brainstorm artifacts:** `.superpowers/brainstorm/25258-1776041636/content/`

## Summary

Add a soft, left-flowing thermographic bloom behind the headline strip's text content. Always breathing at a slow baseline; swells brighter, wider, and faster as composite system heat rises. Purely atmospheric — text legibility is unchanged. Sparklines, gauges, rates, agent dots, alerts, and the entire right-anchored cluster (SESS/BLD stack, agents count, double-height LCD) are untouched.

## Goal

Elevate the dashboard's visual identity from "TUI" to "instrument" with a single focused addition. The headline currently snaps between threat states and carries no atmospheric layer; the thermographic bloom introduces continuous, organic motion that reads as heat without requiring raster graphics or any terminal-capability branching.

## Non-goals

- **No raster graphics.** Kitty/Sixel protocols are out of scope. Rendering is pure ANSI truecolor halfblocks. Terminal portability is unconditional.
- **No right-cluster involvement.** The bloom never visually touches or crosses into the right cluster. Not at baseline, not at meltdown. The right cluster's visual real estate is inviolable.
- **No data-legibility impact.** Text on the headline remains fully readable at all heat levels. The bloom is a background layer only.
- **No changes below the headline strip.** Sparklines, gauges, rates, agent dots, alerts are out of scope.
- **No quip replacement.** The freed top-row real estate stays blank in this spec; filling it is backlogged as `headline-top-row-content.md`.

## Architecture

### New widget

`thermal/internal/widgets/heatbloom.go` — a new widget with the standard coolant widget interface (`SetSize`, `Update`, `View() string`).

### Integration point

Rendered as a layer underneath the headline's text output in `thermal/internal/widgets/headline.go`. The headline's existing text composition is unchanged; the bloom's output is composed *behind* it using lipgloss background layering. If lipgloss v2 does not support z-layering directly, the bloom writes the background cells and the headline's text uses only foreground color + transparent background, allowing the bloom's cells to show through.

### Dependencies consumed

- `*theme.Theme` — palette-aware color ramp. The bloom's HCL color interpolation flows through the existing `theme.Init()` LUT pre-computation pattern. All built-in themes (Classic, Iron, Mono, Frappé) produce correct bloom colors without per-theme bloom code.
- `*anim.Profile` — baseline breathe period, amplitude, opacity range, meltdown-multiplier tunables. Calm/Default/Intense profiles get orthogonal motion parameters per the existing pattern.
- A continuous `CompositeHeat()` scalar exposed on `model.AppState` (see Data binding).

### Rendering strategy

ANSI truecolor halfblocks (`▀` upper-half, `▄` lower-half) composed via lipgloss. Each cell carries two stacked "pixels" — upper foreground color, lower background color (or inverse). This gives 2x vertical resolution within the headline's row height.

For each cell in the bloom's bounding box:
1. Compute the cell's normalized position `(x, y)` within the bloom ellipse.
2. Compute radial distance from the bloom anchor (`x=0.08, y=0.5` in normalized bar coordinates).
3. Apply the HCL-interpolated color ramp (see Visual spec) indexed by current heat level, with alpha falloff based on radial distance.
4. Alpha-blend against the active theme's background color.
5. Emit the final RGB truecolor escape sequence.

No terminal-capability branching. Works identically on Ghostty, Kitty, WezTerm, iTerm2, tmux passthrough, or ssh.

## Visual spec

### Geometry

- **Anchor:** `x = 0.08`, `y = 0.5` in normalized bar coordinates (near left edge, vertical center).
- **Size:** approximately `0.5 × bar_width` wide, `1.0 × bar_height` tall (ellipse).
- **Right fade boundary:** bloom alpha must reach 0 by `x = 0.65` of bar width. The right cluster starts at approximately `x = 0.80` — a 15% buffer zone ensures no visual bleed under any animation phase.
- **Falloff:** inner core at maximum alpha, fading to transparent by 88% of the ellipse's radial extent. Soft edge, no hard termination.

### Color ramp (palette-aware)

The ramp is defined in theme-neutral semantic terms; each theme provides the concrete colors. Example mapping for Frappé:

| heat | inner core | midband | outer fade |
|------|------------|---------|------------|
| 0.00 | `blue` | `green` | transparent |
| 0.33 | `yellow` | `peach` | transparent |
| 0.66 | `peach` | `red` | transparent |
| 1.00 | `red` (max saturation) | `maroon` | transparent |

Transitions between rows are HCL-interpolated through a pre-computed LUT (101 entries, consistent with `theme.SeverityColor`'s existing pattern). No hard cutovers between heat levels — the bloom's color always reflects the continuous heat scalar.

### Motion (M1 — "hot breath")

Single baseline breathing gesture that intensifies as heat rises. No distinct meltdown mode; the same gesture just cranks its parameters.

| parameter | heat=0.0 | heat=1.0 |
|-----------|----------|----------|
| breathe period | 4.5s | 1.6s |
| scale amplitude | ±2% | ±8% |
| opacity range | 0.72–0.88 | 0.85–1.00 |

Linear interpolation between endpoints. All three parameters live in `anim.Profile` with Calm/Default/Intense variants.

### Transitions

When the heat scalar changes, bloom parameters ease to their new target via harmonica spring (already a project dependency, already used by gauges). No jerky snaps. Spring stiffness and damping also live in `anim.Profile`.

## Data binding

### New method

```go
// CompositeHeat returns the weighted CPU/MEM/SWAP signal in [0.0, 1.0].
// Same composite that drives Classify, exposed as a continuous scalar
// rather than bucketed threat level.
func (s *AppState) CompositeHeat() float64
```

### Implementation

Refactor `model.Classify`:
1. Extract the scoring logic currently inside `Classify` into an unexported helper `compositeHeatScore(snap, spawnRate) float64`.
2. `Classify` calls the helper and buckets the scalar into `ThreatCool/Warm/Hot/Meltdown` using the existing thresholds.
3. `CompositeHeat` calls the same helper and returns the scalar unmodified.

This preserves existing threat-classification behavior exactly and surfaces the continuous value the bloom needs.

### Consumer

Heatbloom widget pulls the scalar once per tick and drives its spring targets. No coupling to the LCD value, the headline text, or any other widget — the bloom is a self-contained rendering concern that reads one `float64` per frame.

## Error handling

- **Theme missing bloom colors:** each theme must provide the bloom ramp's four semantic color slots. If a theme is registered without them, `theme.Registry` fails to register and the program exits with a clear error at startup. No runtime fallbacks — misconfiguration is a build-time bug.
- **Bar width too small:** if the headline's bounding box is narrower than some minimum (candidate: 40 columns), the bloom renders at reduced size but never below 0 width. No crash, no panic.
- **Composite heat NaN/Inf:** clamped to `[0.0, 1.0]` at the `CompositeHeat` boundary. Asserted via test.

## Testing

### Golden captures (`widgets/heatbloom_test.go`)

Four frozen frames in `testdata/`:
1. `heatbloom_heat_0_phase_0.golden` — idle baseline, breathe phase = 0
2. `heatbloom_heat_0.5_phase_half.golden` — warm, breathe phase = π/2 (mid-swell)
3. `heatbloom_heat_1_phase_0.golden` — meltdown, breathe phase = 0
4. `heatbloom_right_boundary.golden` — meltdown, with cursor-position marker at the right-cluster boundary column; asserts bloom alpha = 0 at that column (guards against right-side bleed in future changes)

### Unit tests

- `TestCompositeHeatMatchesClassify` — at each threshold boundary (the exact values used by `Classify`'s bucketing), `CompositeHeat()` produces a scalar that rounds to the same bucket. Guards against the two functions drifting apart during future refactors.
- `TestCompositeHeatClamp` — input variants that could produce NaN/Inf (divide-by-zero, negative CPU sample) all clamp to `[0.0, 1.0]`.
- `TestBloomRespectsRightBoundary` — given a 120-column bar, asserts every cell at column index ≥ `0.65 × 120 = 78` has zero bloom contribution at `heat = 1.0`.

### Manual QA

Launch `./bin/thermo --demo` and verify:
- Bloom visible, breathing slowly in baseline
- Bloom intensifies (faster breath, wider, brighter) as demo drives heat up
- No visible bleed into the right cluster at any heat level
- No flicker or tearing during rapid heat transitions
- All four themes (Classic/Iron/Mono/Frappé) produce coherent-looking blooms

## Open questions

- **Exact anchor tuning.** `x=0.08` is the mockup value; may need empirical adjustment once rendered in terminal to feel correctly "left-anchored but breathing" rather than stuck in the corner. Resolve during implementation, not now.
- **Right-boundary exact value.** 0.65 is a soft target; actual right cluster start position depends on headline rendering, which varies with terminal width. Implementation may parameterize this as a function of the headline's measured right-cluster extent rather than a constant.

Both are small calibration concerns that fall out of normal iteration during implementation. Neither blocks the design.

## Out of scope / follow-up

- `docs/backlog/headline-top-row-content.md` — what fills the freed top-row real estate (separate brainstorm required).
- `docs/backlog/threat-transition-smoothing.md` — continuous color transitions on other headline elements. Complementary to this spec but independent.
- Raster rendering (Kitty/Sixel) — archived with `rasterm-graphics` stubs; not pursued.
