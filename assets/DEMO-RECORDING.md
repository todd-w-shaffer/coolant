# Demo GIF Recording Process

How to re-record `thermal-demo.gif` for the README after visual changes to the dashboard.

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

The tape file is `assets/demo-thermal.tape`. Key settings:

| Setting    | Value | Why                                                    |
|------------|-------|--------------------------------------------------------|
| FontSize   | 16    | Larger glyphs rendered at 2x for crisp downscale       |
| Width      | 1920  | 2x display width — browser downscales for crisp text   |
| Height     | 150   | Tight fit around dashboard content at FontSize 16      |
| Padding    | 0     | No wasted space                                        |
| Framerate  | 15    | Balances smooth animation with file size (~5.4MB)      |

The tape hides the terminal, launches `./bin/thermo --demo`, presses `c` (compact mode), records for 20s, then quits.

```bash
vhs assets/demo-thermal.tape
```

This produces `assets/thermal-demo.gif`.

### 3. Optimize file size

VHS output is unoptimized. Use gifsicle with lossy compression:

```bash
gifsicle -O3 --lossy=120 assets/thermal-demo.gif -o assets/thermal-demo.gif
```

`--lossy=120` cuts ~20% without visible artifacts on terminal content. Higher values (150+) start smearing braille dots. Expect ~5.4MB final at current settings — the 2x resolution and color transitions (green→red headline) make this inherently larger than a 1x GIF.

### 4. Verify

Extract frames at key narrative moments and eyeball them:

```bash
gifsicle assets/thermal-demo.gif '#50'  -o /tmp/early.gif   # calm phase
gifsicle assets/thermal-demo.gif '#250' -o /tmp/mid.gif     # language/build ramp
gifsicle assets/thermal-demo.gif '#400' -o /tmp/peak.gif    # shell explosion
```

Check that:
- Content fills the frame height (no large dead space at bottom)
- Braille gauges render cleanly (no compression smear)
- Narrative arc is visible: calm → ramp → peak → cooldown

## Why 2x rendering

GIFs are limited to 256 colors per frame. At 1x (960px), braille dots and text look blocky. Rendering at 1920px with a larger FontSize means each glyph has more source pixels. When GitHub's README viewer downscales to display width (~960px), the extra detail is preserved as effective subpixel resolution. The tradeoff is file size (~5.4MB vs ~3.5MB at 1x).

The FontSize must scale with Width — at 1920px, FontSize 16 produces roughly the same terminal column/row count as FontSize 8 at 960px. If you shrink FontSize without increasing Width, you get more columns but each glyph has fewer pixels, defeating the purpose.

## Adjusting height

If the dashboard layout changes (new rows, different braille font size), the Height may need tuning:

1. Set Height to something generous (e.g., 300)
2. Record, extract a peak-activity frame: `gifsicle output.gif '#250' -o /tmp/peak.gif`
3. Read the frame to check where content ends vs dead space
4. Reduce Height by ~20px, re-record, re-check
5. Iterate until peak frame fills the height with minimal bottom margin
6. No gifsicle cropping needed — get it right at the source

At current FontSize 16 / Width 1920, the dashboard content occupies ~130-140px. Height 150 gives a small breathing margin.

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

## Full one-shot reproduction

```bash
cd /path/to/coolant
cd thermal && go build -o ../bin/thermo ./cmd/thermal/ && cd ..
vhs assets/demo-thermal.tape
gifsicle -O3 --lossy=120 assets/thermal-demo.gif -o assets/thermal-demo.gif
```

That's the whole pipeline. The tape file has all the settings baked in.

## File inventory

- `assets/demo-thermal.tape` — VHS tape (source of truth for recording settings)
- `assets/thermal-demo.gif` — output GIF referenced by README.md line 29
- `assets/DEMO-RECORDING.md` — this file
- `thermal/internal/demo/demov2.go` — scripted narrative data generator
- `thermal/internal/demo/demov2_test.go` — TestNarrativeArc validates the arc
