# Swatch Renderer — The Fitting Room

A standalone tool for iterating on palettes without booting the full dashboard.

## Purpose

When designing themes, you need fast visual feedback: "does this amber work against that dark red background at 1-row height in braille?" Running `--demo` every time is too slow and shows too much. The swatch renderer shows exactly the themed elements, in a single terminal view, for one theme at a time.

## What it renders

One screen, approximately 80×12, containing:

```
┌─ Severity Gradient ──────────────────────────────────────────────────────┐
│ ⣀⣤⣶⣿⣿⣶⣤⣀  (braille sparkline ramp from 0% to 100%, colored by theme gradient) │
│ ⣀⣤⣶⣿⣿⣶⣤⣀  (bottom row of same)                                                │
├─ Overall Gradient ───────────────────────────────────────────────────────┤
│ [    COOL    ] [    WARM    ] [     HOT    ] [  CRITICAL  ] [  OFFLINE  ]│
├─ Category Gradient ──────────────────────────────────────────────────────┤
│ [build:000] [build:001] [build:003] [build:005] [build:010]             │
├─ Threat Colors ──────────────────────────────────────────────────────────┤
│ ● COOL  ● WARM  ● HOT  ● MELTDOWN                                     │
├─ Session Diamonds ───────────────────────────────────────────────────────┤
│ ⌬ idle  ⌬ active  ⌬ language  ⌬ build  ⌬ explosion                     │
├─ Gauge Dots ─────────────────────────────────────────────────────────────┤
│ ● CPU  ● MEM  ● COMP  ● GPU                                            │
├─ Agent Icons ────────────────────────────────────────────────────────────┤
│ ⬡ ⬢ ⬡ ⬢ ⬡  (accent color at 5 brightness levels: 0.2, 0.4, 0.6, 0.8, 1.0) │
├─ Offline Sparkline ──────────────────────────────────────────────────────┤
│ (rainbow or themed scatter dots)                                         │
├─ Rates ──────────────────────────────────────────────────────────────────┤
│ warm:+004/s  cool:-003/s  net:+001/s                                    │
├─ Chrome ─────────────────────────────────────────────────────────────────┤
│ Dim text  Help text  [h] help                                           │
└──────────────────────────────────────────────────────────────────────────┘
```

## Implementation

### Location

`thermal/cmd/swatch/main.go` — standalone binary alongside `brailletext` and `thermal`.

### Interface

```bash
go run ./cmd/swatch                    # default (classic) theme
go run ./cmd/swatch --theme iron       # specific built-in theme
go run ./cmd/swatch --theme frost      # another
go run ./cmd/swatch --all              # render all built-in themes stacked vertically
```

### Architecture

- Imports `internal/theme` for palette definitions
- Renders directly to stdout (no bubbletea, no interactivity)
- Each section is a function that takes a `*theme.Theme` and returns a string
- Uses the same `truecolorFg()` and braille rendering from widgets (or duplicates the small helpers)

### Key design choices

1. **No bubbletea.** This is a print-and-exit tool. No event loop, no model.
2. **Reuse sparkline rendering** for the gradient ramp — generate synthetic data from 0→100 and render with the theme's gradient.
3. **Labels for each section** so screenshots are self-documenting.
4. **`--all` mode** for side-by-side comparison screenshots.

## STUB: Detailed implementation

The swatch renderer is straightforward enough that it doesn't need a full spec. Build it during Phase 2 when the Theme struct exists. The key constraint is: it must use the same rendering paths as the real widgets (same `severityColor()`, same `thermalGradient` lookup) so what you see in the swatch is what you get in the dashboard.

## Screenshotting workflow

```bash
# Render swatch, capture with freeze
go run ./cmd/swatch --theme iron | freeze -o assets/swatch-iron.png

# Or compare all themes
go run ./cmd/swatch --all | freeze -o assets/swatch-all.png
```

This feeds directly into the existing marketing asset pipeline (freeze for stills, vhs for animated demos).
