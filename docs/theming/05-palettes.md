# Reference Palettes

Four built-in themes. Each defined with rationale, all field values, and visual character notes.

---

## 1. Classic (default — backward compatible)

**Character:** The current look. Traffic-light severity. Anthropic orange accents. Familiar, readable, proven.

**When to use:** You don't think about themes. This is what ships out of the box.

```
Name: "classic"

Gradient:
  Low:  #22c55e  (green)
  Mid:  #eab308  (yellow)
  High: #ef4444  (red)

OverallGradient:
  [0] cold:     fg:236  bg:233  (invisible)
  [1] cool:     fg:2    bg:233  (green on dark)
  [2] warm:     fg:3    bg:234  (yellow)
  [3] hot:      fg:208  bg:235  (orange)
  [4] critical: fg:196  bg:52   (red on dark red)

CategoryGradient:
  [0] cold:     fg:236  bg:233  (invisible)
  [1] cool:     fg:180  bg:234  (dim amber)
  [2] warm:     fg:214  bg:235  (orange)
  [3] hot:      fg:208  bg:236  (bright orange)
  [4] critical: fg:196  bg:52   (red on dark red)

ThreatColors:
  Cool:     2    (green)
  Warm:     3    (yellow)
  Hot:      208  (orange)
  Meltdown: 1    (red)

SessionPhase:
  Idle:      245  (gray)
  Active:    2    (green)
  Language:  3    (yellow)
  Build:     208  (orange)
  Explosion: 196  (red)

GaugeDots:
  CPU:  white (7)  — ●
  MEM:  cyan (6)   — ●
  COMP: magenta (5) — ●
  GPU:  green (2)  — ●

Accent: RGB(232, 115, 74) — Anthropic orange

Offline:
  Fg: #000000
  Bg: 67 (steel blue)
  SparkColors: [red, yellow, green, cyan, blue, magenta] (ANSI rainbow)

Chrome:
  Dim: 8 (gray)
  Help: 250 (light gray)

Rates:
  Spawn: 208 (orange)
  Death: 6 (cyan)
  Net:   7 (white)
```

---

## 2. Iron (FLIR thermal camera)

**Character:** Blackbody radiation palette. Deep purple-blacks through amber to white-hot. What an actual thermal camera sees. Scientific, dramatic, distinctive. No green anywhere.

**When to use:** You want the dashboard to feel like looking through a thermal imager. The metaphor is literal — you're watching heat.

**Design notes:**
- The gradient avoids green/yellow entirely — goes from deep indigo through magenta to amber to white
- Backgrounds are true black (#000) with subtle purple tinting at higher levels
- The "cold" state is invisible (as in classic), but "cool" is a deep indigo instead of green
- Hottest state goes white-on-amber — like actual incandescence

```
Name: "iron"

Gradient:
  Low:  #1a0533  (deep indigo — almost invisible)
  Mid:  #c2185b  (hot magenta-pink)
  High: #ffcc02  (incandescent amber-white)

OverallGradient:
  [0] cold:     fg:233  bg:232  (invisible)
  [1] cool:     fg:55   bg:232  (deep purple on black)
  [2] warm:     fg:162  bg:233  (magenta)
  [3] hot:      fg:208  bg:234  (amber)
  [4] critical: fg:229  bg:130  (white-yellow on burnt orange)

CategoryGradient:
  [0] cold:     fg:233  bg:232
  [1] cool:     fg:55   bg:232  (deep purple)
  [2] warm:     fg:133  bg:233  (orchid)
  [3] hot:      fg:208  bg:234  (amber)
  [4] critical: fg:229  bg:130  (white on burnt orange)

ThreatColors:
  Cool:     55   (deep purple)
  Warm:     162  (hot pink)
  Hot:      208  (amber)
  Meltdown: 229  (bright yellow-white)

SessionPhase:
  Idle:      236  (near-black)
  Active:    55   (deep purple)
  Language:  133  (orchid)
  Build:     208  (amber)
  Explosion: 229  (white-hot)

GaugeDots:
  CPU:  STUB — needs testing in swatch renderer
  MEM:  STUB
  COMP: STUB
  GPU:  STUB

Accent: RGB(200, 80, 120) — deep rose (iron-theme appropriate)

Offline:
  Fg: STUB
  Bg: STUB
  SparkColors: STUB — probably muted purples/magentas

Chrome:
  Dim: 237 (very dark gray — iron themes are darker)
  Help: 245

Rates:
  Spawn: 208 (amber — matches hot)
  Death: 55 (purple — matches cool)
  Net: 245 (gray)
```

**STUB: Gauge dots, offline colors, and exact gradient hex values need tuning in the swatch renderer. The values above are starting points based on FLIR "iron" palette research. The key constraint: must be readable at 1-char height in braille.**

---

## 3. Mono (single-hue, brightness-only)

**Character:** Everything in one color family. Heat communicated through brightness and saturation only. Minimal, elegant, accessibility-friendly. No color-coding — only luminance carries information.

**When to use:** Colorblind users. Terminal purists. Anyone who finds the rainbow distracting. People who want their dashboard to look like a CRT.

**Design notes:**
- Base hue is warm white/amber (not pure white, to avoid eye strain)
- Cold = nearly invisible, hot = blazing bright
- No color distinction between categories — rely on labels and glyphs
- Accent is the same hue at peak brightness
- This theme proves the system works without color as an information channel

```
Name: "mono"

Gradient:
  Low:  #3d3225  (dim amber-brown)
  Mid:  #b8956a  (warm medium)
  High: #ffe4b5  (bright warm white — moccasin)

OverallGradient:
  [0] cold:     fg:236  bg:232
  [1] cool:     fg:137  bg:232  (dim khaki)
  [2] warm:     fg:179  bg:233  (medium amber)
  [3] hot:      fg:222  bg:234  (bright gold)
  [4] critical: fg:230  bg:94   (white on deep amber)

CategoryGradient:
  Same as OverallGradient — no distinction in mono

ThreatColors:
  Cool:     137  (dim)
  Warm:     179  (medium)
  Hot:      222  (bright)
  Meltdown: 230  (blazing)

SessionPhase:
  Idle:      236
  Active:    137
  Language:  179
  Build:     222
  Explosion: 230

GaugeDots:
  CPU:  STUB — all same hue at different brightnesses?
  MEM:  STUB — or use glyph differentiation instead of color?
  COMP: STUB
  GPU:  STUB

Accent: RGB(200, 170, 120) — warm amber

Offline:
  Fg: 236
  Bg: 232
  SparkColors: STUB — single hue at varying brightness

Chrome:
  Dim: 239
  Help: 246

Rates:
  Spawn: 222
  Death: 137
  Net: 179
```

**STUB: The mono theme's biggest challenge is gauge dot differentiation. Without color, the 4 gauges (CPU/MEM/COMP/GPU) need different glyphs or positional cues. Options: (a) use different unicode dots (●, ◆, ▲, ■), (b) accept that mono users read labels not dots, (c) use 4 brightness levels of the same hue. Needs swatch testing.**

---

## 4. Frost (cold→hot temperature literal)

**Character:** Blue ice through white to warm amber. The "weather map" palette. Cold is literally cold-colored. Hot is literally warm-colored. Intuitive for anyone who's seen a weather forecast.

**When to use:** The traffic-light feels artificial. You want the color temperature to match the thermal metaphor directly.

**Design notes:**
- Blue end is true cold — deep navy, ice blue
- White/neutral midpoint at the "comfortable" zone
- Warm end is amber, not red — red is reserved for true emergency (meltdown only)
- The transition through white creates a natural "zero crossing" at the midpoint

```
Name: "frost"

Gradient:
  Low:  #1e3a5f  (deep navy blue)
  Mid:  #e8e8e8  (near-white — neutral zone)
  High: #e85d26  (warm amber-orange)

OverallGradient:
  [0] cold:     fg:233  bg:232
  [1] cool:     fg:25   bg:232  (navy blue)
  [2] warm:     fg:252  bg:233  (white/silver — neutral)
  [3] hot:      fg:208  bg:234  (amber)
  [4] critical: fg:196  bg:52   (red — shared emergency color)

CategoryGradient:
  [0] cold:     fg:233  bg:232
  [1] cool:     fg:25   bg:232
  [2] warm:     fg:110  bg:233  (steel blue — mid transition)
  [3] hot:      fg:208  bg:234
  [4] critical: fg:196  bg:52

ThreatColors:
  Cool:     25   (navy)
  Warm:     252  (silver-white)
  Hot:      208  (amber)
  Meltdown: 196  (red)

SessionPhase:
  Idle:      236
  Active:    25   (navy)
  Language:  110  (steel blue)
  Build:     208  (amber)
  Explosion: 196  (red)

GaugeDots:
  STUB — needs swatch testing against blue-to-amber gradient

Accent: RGB(100, 140, 200) — ice blue

Offline:
  Fg: STUB
  Bg: STUB
  SparkColors: STUB — icy scatter?

Chrome:
  Dim: 238
  Help: 248

Rates:
  Spawn: 208 (amber)
  Death: 25 (navy)
  Net: 252 (white)
```

**STUB: The frost theme has a potential readability issue: the "warm" state (white/silver text) might wash out on light-ish terminal backgrounds. This is acceptable given our dark-terminal-only decision (D12), but worth verifying in the swatch renderer. The gradient through white is the signature — if it doesn't work visually, shift the midpoint to light steel blue.**

---

## Palette validation checklist (for each theme)

Before a theme ships, verify in the swatch renderer:

- [ ] All 5 overall gradient levels are visually distinct at 1-row height
- [ ] All 5 category gradient levels are visually distinct in compact cells
- [ ] Sparkline severity ramp shows smooth gradient with no banding artifacts
- [ ] Gauge dots are distinguishable from each other (critical for mono)
- [ ] Breathing dots are visible at min and max brightness on the overall gradient bg
- [ ] Session diamonds are distinguishable at all 5 phases
- [ ] Offline state is clearly different from active states
- [ ] Help text is readable against the dashboard background
- [ ] Dim text is readable but clearly subordinate
- [ ] Rate colors (spawn/death/net) are distinguishable from each other
- [ ] Screenshot at 1x and 2x resolution both look good (retina displays)
