# Demo GIF Recording Process

How to re-record the per-theme demo GIFs for the README after visual changes to the dashboard.

## Prerequisites

```bash
brew install charmbracelet/tap/vhs   # scriptable terminal recorder
brew install gifsicle                 # GIF optimization (lossy compression)
```

## Pipeline

### 1. Build the binary

```bash
cd thermal && go build -o ../bin/thermo ./cmd/thermal/
```

### 2. Record with VHS

Four tapes, one per theme: `demo-classic.tape`, `demo-iron.tape`, `demo-mono.tape`, `demo-frappe.tape`. Shared settings:

| Setting    | Value | Why                                                    |
|------------|-------|--------------------------------------------------------|
| FontSize   | 16    | Larger glyphs rendered at 2x for crisp downscale       |
| Width      | 1920  | 2x display width — browser downscales for crisp text   |
| Height     | 190   | Fits double-height heat bar + swap row at FontSize 16  |
| Padding    | 0     | No wasted space                                        |
| Framerate  | 15    | Balances smooth animation with file size               |

Each tape hides the terminal, launches `./bin/thermo --demo --theme=<name>`, presses `c` (compact mode), records for 20s, then quits.

```bash
for theme in classic iron mono frappe; do
  vhs assets/demo-$theme.tape
done
```

### 3. Optimize file size

```bash
for theme in classic iron mono frappe; do
  gifsicle -O3 --lossy=120 assets/thermal-$theme.gif -o assets/thermal-$theme.gif
done
```

`--lossy=120` cuts ~20% without visible artifacts on terminal content. Higher values (150+) start smearing braille dots. Expect 5–8MB per GIF.

## Why 2x rendering

GIFs are limited to 256 colors per frame. At 1x (960px), braille dots and text look blocky. Rendering at 1920px with a larger FontSize means each glyph has more source pixels. When GitHub's README viewer downscales to display width (~960px), the extra detail is preserved as effective subpixel resolution. The tradeoff is file size.

The FontSize must scale with Width — at 1920px, FontSize 16 produces roughly the same terminal column/row count as FontSize 8 at 960px.

## Adjusting height

If the dashboard layout changes (new rows, different braille font size), Height may need tuning:

1. Set Height generous (e.g., 300)
2. Record, extract a peak-activity frame: `gifsicle output.gif '#250' -o /tmp/peak.gif`
3. Read the frame to check where content ends vs dead space
4. Reduce Height by ~20px, re-record, re-check
5. Iterate until peak fills height with minimal bottom margin — get it right at the source, no gifsicle cropping

At current FontSize 16 / Width 1920, dashboard content occupies ~175–185px with the double-height heat bar.

## Demo narrative arc

The `--demo` flag runs a scripted scenario (see `thermal/internal/demo/demov2.go`):

| Phase     | Ticks  | Time      | What happens                              |
|-----------|--------|-----------|-------------------------------------------|
| Calm      | 0-8    | 0-2s      | 1 agent idle                              |
| Spawn     | 8-16   | 2-4s      | Agents 2 and 3 appear (breathedots)       |
| Language  | 16-28  | 4-7s      | Node + Go processes spawn                 |
| Build     | 28-38  | 7-9.5s    | tsc + esbuild join                        |
| Shell     | 38-60  | 9.5-15s   | Shell explosion to 80+ procs, stats pin   |
| Cooldown  | 60-80  | 15-20s    | Everything winds down, agents die         |

## File inventory

- `assets/demo-<theme>.tape` — VHS tape per theme (classic, iron, mono, frappe)
- `assets/thermal-<theme>.gif` — output GIFs referenced by README.md
- `assets/DEMO-RECORDING.md` — this file
- `thermal/internal/demo/demov2.go` — scripted narrative data generator
- `thermal/internal/demo/demov2_test.go` — TestNarrativeArc validates the arc
