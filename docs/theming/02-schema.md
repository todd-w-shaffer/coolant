# Theme Schema

The canonical definition of what a Theme contains. This drives the Go struct, palette design, and migration mapping.

## Theme struct (Go pseudocode)

```go
package theme

import (
    "image/color"
    colorful "github.com/lucasb-eyer/go-colorful"
)

// Theme defines the complete visual palette for the thermal dashboard.
type Theme struct {
    Name string // "classic", "iron", "mono", "frost"

    // ── Severity gradient (sparklines) ──────────────────────
    // Three anchor colors blended in HCL space for sparkline coloring.
    // severityColor() interpolates: low → mid below warn, mid → high above warn.
    GradientLow  colorful.Color // e.g. green (#22c55e)
    GradientMid  colorful.Color // e.g. yellow (#eab308)
    GradientHigh colorful.Color // e.g. red (#ef4444)

    // Pre-computed LUTs (generated at theme load from the above anchors)
    lowMidLUT  [101]string // ANSI escapes: GradientLow → GradientMid
    midHighLUT [101]string // ANSI escapes: GradientMid → GradientHigh
    lowANSI    string      // cached: truecolorFg(GradientLow)
    highANSI   string      // cached: truecolorFg(GradientHigh)

    // ── Overall thermal gradient (headline bar) ─────────────
    // 5 levels: cold (no data) → cool → warm → hot → critical
    OverallGradient [5]ThermalLevel

    // ── Category thermal gradient (category boxes) ──────────
    // 5 levels, same structure, different colors (amber tones by default)
    CategoryGradient [5]ThermalLevel

    // ── Threat colors (alerts, status indicators) ───────────
    ThreatColors [4]color.Color // indexed by ThreatLevel (Cool/Warm/Hot/Meltdown)

    // ── Session phase colors (session diamonds) ─────────────
    SessionPhase SessionPhaseColors

    // ── Gauge dot colors (sparkline row indicators) ─────────
    GaugeDots [4]GaugeDotColor // CPU, MEM, COMP, GPU

    // ── Accent color (breathing agent icons) ────────────────
    AccentR, AccentG, AccentB float64 // base RGB for breathing modulation

    // ── Offline / special states ────────────────────────────
    OfflineFg color.Color   // quip text color when offline
    OfflineBg color.Color   // background when offline
    OfflineSparkColors []colorful.Color // rainbow replacement for offline sparklines

    // ── Chrome (UI frame, help text, dim elements) ──────────
    DimColor  color.Color // gray — muted/inactive text
    HelpColor color.Color // help text descriptions

    // ── Semantic colors (process types & categories) ────────
    // Optional overrides — nil entries fall back to defaults.
    // Most themes leave these alone.
    TypeColors     map[string]color.Color // process type codes → colors
    CategoryColors map[string]color.Color // category names → colors

    // ── Rate display colors ─────────────────────────────────
    SpawnColor color.Color // warm/spawn rate text
    DeathColor color.Color // cool/death rate text
    NetColor   color.Color // net rate text
}

// ThermalLevel pairs foreground and background for a heat level.
type ThermalLevel struct {
    Fg color.Color
    Bg color.Color
}

// SessionPhaseColors holds the 5 escalation states.
type SessionPhaseColors struct {
    Idle      color.Color // no activity
    Active    color.Color // shells below threshold
    Language  color.Color // runtime category detected
    Build     color.Color // build tools active
    Explosion color.Color // shell count above threshold
}

// GaugeDotColor defines a colored dot indicator for sparkline rows.
type GaugeDotColor struct {
    Char      string      // glyph (usually "●")
    Color     color.Color // lipgloss color for styled rendering
    // Derived at load time:
    ANSI      string      // raw ANSI color escape
    Formatted string      // pre-computed: ANSI + Char + reset + space
}
```

## Field justification

| Field | Why it exists | What if we omit it |
|-------|---------------|-------------------|
| `GradientLow/Mid/High` | Drives all sparkline coloring across 5 gauges | Sparklines hardcoded to green/yellow/red |
| `OverallGradient` | Headline bar bg/fg at each threat level | Headline bar un-themeable |
| `CategoryGradient` | Category boxes in headline (separate from overall) | Category boxes un-themeable |
| `ThreatColors` | Alert messages, status indicators | Alerts un-themeable |
| `SessionPhase` | Session diamond colors in headline | Session diamonds un-themeable |
| `GaugeDots` | CPU/MEM/COMP/GPU sparkline markers | Gauge dots un-themeable |
| `AccentR/G/B` | Agent breathing hexagons | Breathing always Anthropic orange |
| `OfflineFg/Bg` | Offline state headline | Offline state un-themeable |
| `OfflineSparkColors` | Offline sparkline rainbow | Rainbow always 6-color ANSI |
| `DimColor` | All dim/muted text across UI | Dim color un-themeable |
| `HelpColor` | Help overlay text descriptions | Help text un-themeable |
| `TypeColors` | Process type glyphs (optional) | Acceptable — most themes share |
| `CategoryColors` | Category glyphs (optional) | Acceptable — most themes share |
| `SpawnColor/DeathColor/NetColor` | Rate display in bottom bar | Rate colors un-themeable |

## What's NOT in the theme

- **Thresholds** (warn/crit percentages) — operational, not aesthetic
- **Timing** (animation FPS, decay rates) — behavioral, not visual
- **Glyphs** (⬡, ⬢, ●, ■, etc.) — structural, could be future work
- **Layout** — compositor decisions, not palette
- **Braille font bitmaps** — structural, never changes

## Initialization

```go
// Init pre-computes LUTs and derived ANSI strings.
// Called once at theme load. ~50μs.
func (t *Theme) Init() {
    for i := 0; i <= 100; i++ {
        ratio := float64(i) / 100.0
        t.lowMidLUT[i] = truecolorFg(t.GradientLow.BlendHcl(t.GradientMid, ratio).Clamped())
        t.midHighLUT[i] = truecolorFg(t.GradientMid.BlendHcl(t.GradientHigh, ratio).Clamped())
    }
    t.lowANSI = truecolorFg(t.GradientLow)
    t.highANSI = truecolorFg(t.GradientHigh)

    for i := range t.GaugeDots {
        d := &t.GaugeDots[i]
        d.ANSI = truecolorFgFromColor(d.Color)
        d.Formatted = d.ANSI + d.Char + "\033[0m "
    }
}
```

## STUB: External theme file format

When we add user-defined themes (Phase 6), the file format will be TOML:

```toml
name = "cyberpunk"

[gradient]
low = "#00ff41"
mid = "#ffb000"
high = "#ff0040"

[overall]
# ... 5 levels with fg/bg pairs

[accent]
rgb = "#e87348"  # Anthropic orange

# ... etc
```

**To be fully designed when we reach Phase 6.** The Go struct is the source of truth; the TOML maps onto it.
