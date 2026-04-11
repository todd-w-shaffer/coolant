# Animation Axis — Design Spec

A pluggable animation profile system, independent of the theme system. Theme controls color, animation controls motion. They compose orthogonally.

## Why profiles, not raw flags

We evaluated four approaches:

1. **Single abstract axis** (`--animation pulse`): Forces fake sparkline modes — sparklines have zero animation tunables today. Dishonest UX.
2. **Two axes** (`--dot-animation tidal --spark-animation smooth`): One axis is empty (sparklines are pure functions of data). Premature abstraction.
3. **Hybrid** (`--animation` silently only affects dots): Naming confusion. Users who notice sparklines didn't change will file bugs.
4. **Named profiles** (`--animation calm`): Bundles coherent constants. Single flag, honest scope, extensible without API change.

Profiles win. They follow the proven theme registry pattern.

## Current animation surfaces

### BreatheDots (15+ tunables, high pluggability value)

Three modes branched in `Render()`:
- **Tidal wave** (active dots): `TidalPhaseStep`, `TidalWaveMix`, `TidalBreathMix`, `TidalBrightFloor`, `GlyphFilledThresh`, `GlyphMidThresh`, phase offset (hardcoded 1.5 rad/dot)
- **KITT scanner** (stale or completed): `KITTSweepRate`, `KITTAmbient`, `KITTPeak`, `KITTSigmaSq`, `KITTSingleBright`
- **Breathing** (base): `BreathePhaseStep`, `BreatheStaleRate`, `BreatheStaleDim`, `BreatheFadeEps`

Plus spring physics: `SpringFreq`, `SpringDamping` (shared with gauges).

### Gauges (3 tunables, low pluggability value)

- Spring params (`SpringFreq`, `SpringDamping`) — shared with BreatheDots
- Peak decay (`PeakDecayRate`)
- History cap (`MaxRenderHistory`)

Spring params are the only ones that affect perceived animation "style." Peak decay and history cap are functional, not aesthetic.

### Sparklines (0 tunables)

Pure function of data. No frame state, no animation parameters. The scroll illusion comes from gauges pushing to `renderHistory` each AnimTick. Offline rainbow is a deterministic hash, not animated.

**If sparklines ever gain animation tunables** (scroll easing, fade-in curves, alternative braille encodings), profiles grow a field. No API change.

## Architecture

### Package: `internal/anim/`

Mirror the theme system exactly:

```
internal/anim/
├── profile.go    # AnimProfile struct + helpers
├── default.go    # Default profile (current constants)
├── calm.go       # Slower rates, narrower brightness, softer transitions
├── intense.go    # Faster rates, sharper glyph transitions, wider brightness
└── registry.go   # Registry map, Get(), Names()
```

### AnimProfile struct

```go
type AnimProfile struct {
    Name string

    // -- Tidal wave (active agent dots) --
    TidalPhaseStep    float64 // phase advance per tick
    TidalWaveMix      float64 // wave weight in brightness blend
    TidalBreathMix    float64 // individual breath weight
    TidalBrightFloor  float64 // minimum brightness
    TidalPhaseSpread  float64 // rad between adjacent dots (currently hardcoded 1.5)
    GlyphFilledThresh float64 // wave > this → filled glyph
    GlyphMidThresh    float64 // wave > this → mid glyph

    // -- KITT scanner (stale/completed dots) --
    KITTSweepRate    float64 // sweep position per tick
    KITTAmbient      float64 // floor brightness at edges
    KITTPeak         float64 // peak brightness contribution
    KITTSigmaSq      float64 // gaussian width
    KITTSingleBright float64 // single-dot fallback brightness

    // -- Breathing (base dot animation) --
    BreathePhaseStep float64 // phase advance per tick
    BreatheStaleRate float64 // multiplier for stale dots
    BreatheStaleDim  float64 // brightness multiplier for stale dots

    // -- Spring physics (shared: dots + gauges) --
    SpringFreq    float64
    SpringDamping float64

    // -- Gauge --
    PeakDecayRate float64
}
```

### Resolution

Same cascade as themes: `--animation` flag > `COOLANT_ANIMATION` env > `"default"`.

### Wiring

`AnimProfile` passed alongside `*theme.Theme` to widget constructors:

```go
// layout
func NewHorizontal(th *theme.Theme, ap *anim.AnimProfile) *Horizontal

// widgets
func NewBreatheDots(th *theme.Theme, ap *anim.AnimProfile) *BreatheDots
func NewGauges(th *theme.Theme, ap *anim.AnimProfile) *Gauges
```

BreatheDots replaces all `config.KITT*`, `config.Tidal*`, `config.Breathe*`, `config.Glyph*` reads with `ap.*` field reads. Gauges replaces `config.SpringFreq`, `config.SpringDamping`, `config.PeakDecayRate`.

The constants stay in `config/tuning.go` as documentation and fallback defaults. The `Default()` profile constructor reads from them so there's exactly one source of truth for the shipped defaults.

### Refactoring BreatheDots.Render()

The 3-branch brightness dispatch (tidal, KITT, dim-breathe) stays structurally identical — profiles only change the constants feeding each branch. The branches select behavior based on dot state (active, stale, dying), which is orthogonal to animation style.

**No strategy interface needed for the initial ship.** The branches are state selection, not style selection. All three modes exist in every profile — profiles just tune how fast/bright/sharp each mode runs.

If a future profile wanted a fundamentally different animation *kind* (e.g., heartbeat that replaces tidal wave entirely), that's when we'd extract a `DotAnimator` interface. Not now.

## Ship plan

### Profile candidates

| Profile | Personality | Key differences from default |
|---------|-------------|------------------------------|
| `default` | Current behavior | Baseline — all existing constants |
| `calm` | Meditative, slow | TidalPhaseStep halved, wider TidalBrightFloor (0.7), KITTSweepRate halved, softer gaussian (KITTSigmaSq 1.5) |
| `intense` | Urgent, sharp | TidalPhaseStep doubled, narrower GlyphMidThresh (0.25), faster KITTSweepRate (0.08), tighter gaussian (KITTSigmaSq 0.4) |

### Swatch integration

`cmd/swatch/` gains an `--animation` flag. In `--animate` mode (future, specced in backlog), it runs a brief bubbletea loop showing agent dots with the selected profile. For now, swatch just prints the profile name and its constants.

## Resolved questions (2026-04-10)

1. **`Default()` reads from config/tuning.go.** Single source of truth. `internal/anim` imports `internal/config` — same direction as every widget.

2. **`Animatable` interface deferred.** Only 2 widgets tick today. Revisit when a third appears.

3. **Highscore mode is orthogonal.** `--kitt-highscore` selects which dots get KITT; the profile tunes how KITT behaves. No interaction, no special casing.

4. **`TidalPhaseSpread` promoted to config + profile.** Was the only animation constant not in tuning.go — an oversight, not a design choice. Now in `config.TidalPhaseSpread` and `Profile.TidalPhaseSpread`.

5. **Spring params shared.** Visual coherence is a feature. If needed later, `GaugeSpringFreq`/`DotSpringFreq` overrides are a backward-compatible addition.

6. **Three profiles shipped: `default`, `calm`, `intense`.** Proves the range. Community profiles via the registry pattern later.
