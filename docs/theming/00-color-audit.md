# Color Audit — Every Hardcoded Color in thermal/

Canonical inventory of every color definition, organized by file. This is the "before" picture — every line here must be accounted for in the migration mapping.

## ui/colors.go (the current "center of gravity")

| Line | Symbol | Value | What it colors | Semantic role |
|------|--------|-------|----------------|---------------|
| 14 | `DimColor` | `"8"` (gray) | Muted/inactive text | Chrome — dim |
| 15 | `CyanColor` | `"6"` (cyan) | Offline accents, cool state | Chrome — accent |
| 46 | `GaugeDots[0]` | `"\033[37m"` / `"7"` (white) | CPU sparkline dot | Gauge — CPU |
| 47 | `GaugeDots[1]` | `"\033[36m"` / `"6"` (cyan) | MEM sparkline dot | Gauge — MEM |
| 48 | `GaugeDots[2]` | `"\033[35m"` / `"5"` (magenta) | Compressor sparkline dot | Gauge — COMP |
| 49 | `GaugeDots[3]` | `"\033[32m"` / `"2"` (green) | GPU sparkline dot | Gauge — GPU |
| 64-76 | `TypeColor` | 12 entries | Process type codes (N,G,V,S,R,F,C,P,T,GO,RS,SW,X) | Semantic — type identity |
| 80-88 | `CategoryColor` | 7 entries | Process categories (build,shell,node,go,python,rust,swift) | Semantic — category identity |
| 124 | `ThreatColor[Cool]` | `"2"` (green) | Cool threat indicators | Threat — cool |
| 125 | `ThreatColor[Warm]` | `"3"` (yellow) | Warm threat indicators | Threat — warm |
| 126 | `ThreatColor[Hot]` | `"208"` (orange) | Hot threat indicators | Threat — hot |
| 127 | `ThreatColor[Meltdown]` | `"1"` (red) | Meltdown threat indicators | Threat — critical |

## widgets/sparkline.go

| Line | Symbol | Value | What it colors | Semantic role |
|------|--------|-------|----------------|---------------|
| 26 | `gradGreen` | `#22c55e` | Gradient anchor: low severity | Gradient — low |
| 27 | `gradYellow` | `#eab308` | Gradient anchor: mid severity | Gradient — mid |
| 28 | `gradRed` | `#ef4444` | Gradient anchor: high severity | Gradient — high |
| 36-37 | `greenYellowANSILUT[101]` | computed | Interpolated green→yellow | Gradient — derived |
| 37 | `yellowRedANSILUT[101]` | computed | Interpolated yellow→red | Gradient — derived |
| 444-451 | `rainbowColors[6]` | ANSI 31-35 | Offline sparkline dots | Offline — playful |
| 455-470 | `funBraille[14]` | braille patterns | Offline sparkline shapes | Offline — playful |

## widgets/thermal.go

| Line | Symbol | Value | What it colors | Semantic role |
|------|--------|-------|----------------|---------------|
| 19 | `thermalGradient[0]` | fg:`236` bg:`233` | Cold category box | CatGradient — cold |
| 20 | `thermalGradient[1]` | fg:`180` bg:`234` | Cool category box (dim amber) | CatGradient — cool |
| 21 | `thermalGradient[2]` | fg:`214` bg:`235` | Warm category box (orange) | CatGradient — warm |
| 22 | `thermalGradient[3]` | fg:`208` bg:`236` | Hot category box (bright orange) | CatGradient — hot |
| 23 | `thermalGradient[4]` | fg:`196` bg:`52` | Critical category box (red on dark red) | CatGradient — critical |

## widgets/headline.go

| Line | Symbol | Value | What it colors | Semantic role |
|------|--------|-------|----------------|---------------|
| 17 | `overallGradient[0]` | fg:`236` bg:`233` | Cold overall bar | OverallGradient — cold |
| 18 | `overallGradient[1]` | fg:`2` bg:`233` | Cool overall bar (green) | OverallGradient — cool |
| 19 | `overallGradient[2]` | fg:`3` bg:`234` | Warm overall bar (yellow) | OverallGradient — warm |
| 20 | `overallGradient[3]` | fg:`208` bg:`235` | Hot overall bar (orange) | OverallGradient — hot |
| 21 | `overallGradient[4]` | fg:`196` bg:`52` | Critical overall bar (red) | OverallGradient — critical |
| 140 | inline | bg:`67` | Offline overall bar background | Offline — bg |
| 158-159 | inline | fg:`#000000`, bg:`67` | Offline quip text | Offline — fg+bg |

## widgets/rates.go

| Line | Symbol | Value | What it colors | Semantic role |
|------|--------|-------|----------------|---------------|
| 112 | inline | `"208"` | Spawn rate text (warm:+004/s) | Rate — spawns |
| 114 | ref | `CyanColor` ("6") | Death rate text (cool:-003/s) | Rate — deaths |
| 116 | inline | `"7"` | Net rate text (net:+001/s) | Rate — net |
| 188 | `phaseRed` | `"196"` | Session diamond: shell explosion | SessionPhase — explosion |
| 189 | `phaseOrange` | `"208"` | Session diamond: build phase | SessionPhase — build |
| 190 | `phaseYellow` | `"3"` | Session diamond: language phase | SessionPhase — language |
| 191 | `phaseGreen` | `"2"` | Session diamond: active/shells | SessionPhase — active |
| 192 | `phaseIdle` | `"245"` | Session diamond: idle | SessionPhase — idle |

## widgets/breathedots.go

| Line | Symbol | Value | What it colors | Semantic role |
|------|--------|-------|----------------|---------------|
| 132-135 | inline | `#RRGGBB` from config constants | Agent hexagon breathing color | Accent — agent icon |

## config/tuning.go (color-adjacent constants)

| Line | Symbol | Value | Purpose |
|------|--------|-------|---------|
| 52 | `BreatheBaseR` | `232.0` | Agent icon base red (Anthropic orange) |
| 53 | `BreatheBaseG` | `115.0` | Agent icon base green |
| 54 | `BreatheBaseB` | `74.0` | Agent icon base blue |
| 55 | `BreatheStaleDim` | `0.35` | Stale dot brightness multiplier |

## layout/horizontal.go

| Line | Symbol | Value | What it colors | Semantic role |
|------|--------|-------|----------------|---------------|
| 120 | inline | `"250"` | Help text descriptions | Chrome — help |
| 124 | inline | `"245"`, `"2"`, `"3"`, `"208"`, `"196"` | Help view session diamond examples | Chrome — help (mirrors SessionPhase) |

## Summary counts

- **Total unique color definitions:** ~45
- **Files with hardcoded colors:** 7
- **Duplicated values:** `"208"` appears 6 times, `"196"` 4 times, `"2"` 4 times, `"3"` 3 times
- **Two separate 5-level gradients:** `thermalGradient` (category) and `overallGradient` (headline) — intentionally different
- **Pre-computed LUTs:** 2 arrays × 101 entries (must regenerate per theme)
