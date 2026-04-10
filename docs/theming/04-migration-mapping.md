# Migration Mapping — Surgical Refactor Guide

Every current color callsite mapped to its target theme field. This is the checklist for Phase 3 (consolidation refactor). Each line is one mechanical change.

## Key

- **Source**: current file:line and symbol
- **Target**: theme field that replaces it
- **Action**: what to do (replace, remove, derive)
- **Notes**: gotchas or dependencies

---

## ui/colors.go

| Source | Target | Action | Notes |
|--------|--------|--------|-------|
| `DimColor` (L14) | `theme.DimColor` | Replace with theme lookup | Used by `DimText()` — need theme-aware helper or pass color |
| `CyanColor` (L15) | `theme.DeathColor` (for rates), `theme.OfflineFg` (for idle) | Split | Currently overloaded: used for both "cool/death" rate display and offline accents |
| `GaugeDots` init (L40-60) | `theme.GaugeDots` | Remove init(), derive from theme | Pre-computed Formatted strings must regenerate per theme |
| `TypeColor` map (L63-77) | `theme.TypeColors` (optional override) | Keep as default, allow override | Most themes won't touch this |
| `CategoryColor` map (L80-88) | `theme.CategoryColors` (optional override) | Keep as default, allow override | Used by CategoryGlyphFormatted init |
| `CategoryGlyphFormatted` init (L111-120) | Regenerate from theme | Move to theme.Init() | Depends on CategoryColors |
| `ThreatColor` map (L123-128) | `theme.ThreatColors` | Replace | Direct indexed access by ThreatLevel |

## widgets/sparkline.go

| Source | Target | Action | Notes |
|--------|--------|--------|-------|
| `gradGreen` (L26) | `theme.GradientLow` | Remove package var | |
| `gradYellow` (L27) | `theme.GradientMid` | Remove package var | |
| `gradRed` (L28) | `theme.GradientHigh` | Remove package var | |
| `greenYellowANSILUT` (L36) | `theme.lowMidLUT` | Remove package var, read from theme | |
| `yellowRedANSILUT` (L37) | `theme.midHighLUT` | Remove package var, read from theme | |
| `gradGreenANSI` (L38) | `theme.lowANSI` | Remove package var | |
| `gradRedANSI` (L39) | `theme.highANSI` | Remove package var | |
| `init()` LUT generation (L42-50) | `theme.Init()` | Move to theme initialization | |
| `severityColor()` (L90-107) | Add `*Theme` parameter | Must propagate through all callers | **Biggest ripple** — called from gauges.go, rates.go |
| `rainbowColors` (L444-451) | `theme.OfflineSparkColors` | Replace | Need ANSI escape conversion |
| `rainbowChar()` (L474-478) | Update to use theme colors | Index into `theme.OfflineSparkColors` | |

### severityColor() call chain (ripple analysis)

```
severityColor() ← called from:
  └── renderSparklineCore() (sparkline.go:431)
        └── RenderSparkline() (sparkline.go:308)
              └── Gauges.View() (gauges.go:219)
  └── Rates.View() (rates.go:83, 91, 98, 105)
```

**Options for threading theme through:**
1. Add `*Theme` parameter to `severityColor()` — 2 callers to update
2. Make `severityColor` a method on Theme — cleaner, but changes the package boundary
3. Store theme on Gauges and Rates widgets — they already have state

**Resolution: Option 3.** Gauges and Rates already store `*model.AppState`. Add `theme *Theme` field set via constructor. They call `theme.SeverityColor(v, thresh)` instead of the package function.

## widgets/thermal.go

| Source | Target | Action | Notes |
|--------|--------|--------|-------|
| `thermalGradient` var (L18-24) | `theme.CategoryGradient` | Remove package var | |
| `thermalLevelFor()` (L27-56) | Keep as-is (threshold logic, not color) | No change | Returns index, not color |
| Usage in `renderCatCell()` (headline.go:78) | `theme.CategoryGradient[level]` | Update lookup | |

## widgets/headline.go

| Source | Target | Action | Notes |
|--------|--------|--------|-------|
| `overallGradient` var (L16-22) | `theme.OverallGradient` | Remove package var | |
| Offline bg `"67"` (L140) | `theme.OfflineBg` | Replace inline | |
| Offline fg/bg `"#000000"`/`"67"` (L158-159) | `theme.OfflineFg` / `theme.OfflineBg` | Replace inline | |
| `threatToThermal()` (L262-275) | Keep as-is (maps ThreatLevel to gradient index) | No change | Pure logic |
| Usage: `overallGradient[overallLevel]` (L143, L164) | `theme.OverallGradient[overallLevel]` | Update lookup | |

**Headline needs theme reference.** Add `theme *Theme` field, set via constructor.

## widgets/rates.go

| Source | Target | Action | Notes |
|--------|--------|--------|-------|
| `phaseRed` (L188) | `theme.SessionPhase.Explosion` | Remove package var | |
| `phaseOrange` (L189) | `theme.SessionPhase.Build` | Remove package var | |
| `phaseYellow` (L190) | `theme.SessionPhase.Language` | Remove package var | |
| `phaseGreen` (L191) | `theme.SessionPhase.Active` | Remove package var | |
| `phaseIdle` (L192) | `theme.SessionPhase.Idle` | Remove package var | |
| Spawn color `"208"` (L112) | `theme.SpawnColor` | Replace inline | |
| Death color `CyanColor` (L114) | `theme.DeathColor` | Replace inline | |
| Net color `"7"` (L116) | `theme.NetColor` | Replace inline | |
| `sessionPhaseColor()` (L197-224) | Read from `theme.SessionPhase` | Thread theme through | |

**Rates needs theme reference.** Same pattern as Headline.

## widgets/breathedots.go

| Source | Target | Action | Notes |
|--------|--------|--------|-------|
| `config.BreatheBaseR` usage (L132) | `theme.AccentR` | Replace config ref | |
| `config.BreatheBaseG` usage (L133) | `theme.AccentG` | Replace config ref | |
| `config.BreatheBaseB` usage (L134) | `theme.AccentB` | Replace config ref | |

**BreatheDots needs theme reference.** Set via constructor or passed through Headline (which owns it).

## layout/horizontal.go

| Source | Target | Action | Notes |
|--------|--------|--------|-------|
| Help text `"250"` (L120) | `theme.HelpColor` | Replace | |
| Session diamond colors in helpView (L124) | Read from `theme.SessionPhase.*` | Replace hardcoded strings | Must match runtime colors |
| Idle view `CyanColor` references (L143, L153) | Keep or map to theme | Minor — idle view styling | |

**Layout already holds all widgets — add `theme *Theme` field.** Pass to widget constructors.

## config/tuning.go

| Source | Target | Action | Notes |
|--------|--------|--------|-------|
| `BreatheBaseR/G/B` (L52-54) | Move to theme.AccentR/G/B | Remove from config | Config keeps non-color constants |
| `BreatheStaleDim` (L57) | Keep in config (behavioral, not aesthetic) | No change | Controls brightness multiplier |

## widgets/alerts.go

| Source | Target | Action | Notes |
|--------|--------|--------|-------|
| `ui.ThreatColor[alert.Level]` (L45) | `theme.ThreatColors[alert.Level]` | Replace | Alerts needs theme reference |

---

## Constructor changes summary

Widgets that need `*Theme` added to their constructor:

| Widget | Constructor | Current params | Add |
|--------|------------|----------------|-----|
| `Headline` | `NewHeadline()` | none | `theme *Theme` |
| `Gauges` | `NewGauges()` | none | `theme *Theme` |
| `Rates` | `NewRates()` | none | `theme *Theme` |
| `Alerts` | `NewAlerts()` | none | `theme *Theme` |
| `BreatheDots` | `NewBreatheDots()` | none | `theme *Theme` (or via Headline) |
| `Horizontal` | `NewHorizontal()` | none | `theme *Theme` |

**BreatheDots** is owned by Headline. Two options:
1. Headline passes theme to BreatheDots constructor
2. BreatheDots.Render() takes accent RGB as parameters (already somewhat functional)

**Resolution: Option 1** — simpler, consistent pattern.

## Execution order

1. Create `internal/theme/` package with Theme struct and Classic palette
2. Update widget constructors to accept `*Theme` (compile errors guide you)
3. Replace package-level color vars with theme field reads — one file at a time
4. Remove orphaned package vars
5. Run golden tests to verify Classic theme produces identical output
