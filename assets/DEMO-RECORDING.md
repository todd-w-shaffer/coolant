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

VHS output is unoptimized. Use gifsicle with lossy compression to bring it down to ~3.5MB:

```bash
gifsicle -O3 --lossy=80 assets/thermal-demo.gif -o assets/thermal-demo.gif
```

`--lossy=80` is aggressive enough to cut ~20% without visible artifacts on terminal content.

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

## Adjusting height

If the dashboard layout changes (new rows, different braille font size), the Height may need tuning:

1. Set Height to something generous (e.g., 300)
2. Record, extract a peak-activity frame: `gifsicle output.gif '#300' -o /tmp/peak.gif`
3. Measure where content ends (view frame, look for dead space)
4. Set Height to that value + small margin
5. Re-record and verify — no gifsicle cropping needed

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

- `assets/demo-thermal.tape` — VHS tape (source of truth for recording settings)
- `assets/thermal-demo.gif` — output GIF referenced by README.md line 29
