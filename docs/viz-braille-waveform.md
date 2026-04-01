# Braille Waveform

**Replaces:** Spawn Rate pane (spawnrate.sh)

**Why:** The sparkline blocks (`▁▂▃▄▅▆▇█`) give 8 levels of vertical resolution per character row. Braille characters give 4 dots per row × 2 columns per character = 8x the spatial resolution. A spawn/death trace at braille resolution looks like an actual oscilloscope signal — smooth curves instead of stepped blocks. And by overlaying both signals on the same character grid, you see the relationship between spawns and deaths as interwoven waveforms rather than separate rows you have to mentally correlate.

**Unique signal:** Overlaid spawn vs death waveforms at high resolution. The spawnrate pane showed three separate rows (spawns, deaths, net). This shows spawns and deaths on the same canvas so you see crossovers, divergence, and convergence as visual shapes — the moment deaths overtake spawns is a visible crossing of two traces, not a number to compute mentally.

---

## Layout

**Position:** Top-right quadrant of the 2x2 tmux grid (same slot spawnrate occupied).

**Structure:**

```
┌─ RATE ── +12/s  -4/s  net:+8 ──────────┐
│                                          │
│  ⠀⠀⠀⠀⠀⡀⣀⣠⣤⣶⣿⣿⣶⣤⣠⣀⡀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀  │  <- overlaid traces
│  ⠀⠀⠀⡀⣠⣶⣿⣿⣶⣤⣠⣀⡀⠀⠀⠀⠀⣀⣠⣤⣶⣿⣿⣶⣠⡀⠀⠀⠀  │
│  ⣿⣶⣤⣿⣿⣤⣠⣀⡀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⡀⣀⣠⣤⣶⣿⣿⣶⣤  │
│                                          │
└─ ● spawns  ● deaths ────────────────────┘
```

The chart area uses the existing `sparkline.sh` braille rendering approach from `scripts/sparkline.sh` in the coolant monitor — specifically the multi-dot-per-character encoding.

## Rendering Rules

### Braille encoding

Each Unicode braille character (U+2800 to U+28FF) is a 2-wide × 4-tall dot grid:

```
Dot positions:     Bit values:
(0,0) (1,0)        1    8
(0,1) (1,1)        2   16
(0,2) (1,2)        4   32
(0,3) (1,3)       64  128
```

Codepoint = 0x2800 + sum of set bits. UTF-8 encode as 3 bytes.

### Two traces on one canvas

- **Spawns trace** (green): dots set based on spawn count scaled to chart height
- **Deaths trace** (red): dots set based on death count scaled to same chart height
- When both traces occupy the same braille character, set both bits. Color the character based on which trace is dominant at that position (higher value wins), or use yellow for overlap.

### Scaling

- Chart height in rows is configurable (default: pane height minus 3 for header + legend + border).
- Total vertical resolution = chart_rows × 4 dots.
- Auto-scale: the peak value seen in the visible window determines the vertical range. If peak is 40 spawns/s, each dot represents 40 / (rows × 4) spawns.
- Each character column represents one tick (2 data points per character column, same as the monitor's sparkline approach — left column = tick N, right column = tick N+1).

### Scrolling

- Ring buffer of spawn and death values, sized to `chart_width × 2` (two data points per character).
- New ticks push from the right, oldest fall off the left.

### Header

Single-line header inside the box: current spawn rate, death rate, and net. Compact format: `+12/s  -4/s  net:+8`. Net is colored green (positive calm), yellow (positive warn), red (positive crit). Use threshold vars from common.sh.

### Legend

Bottom line: `● spawns  ● deaths` with green and red dots respectively.

---

## Implementation Constraints

1. **Buffered rendering.** `frame=$(render)`, `tput home`, `printf '%s' "$frame"`, `tput ed`.
2. **Cache tput cols/lines before subshell.** `COLS=$(tput cols)` and `ROWS=$(tput lines)` before `frame=$(render)`.
3. **Process substitution.** `while read ... done < <(tail -n 0 -f ...)`.
4. **Source cc-viz/common.sh.**
5. **Bash 3.2 compatible.**
6. **Reuse braille encoding from scripts/sparkline.sh** — the bit-packing and UTF-8 encoding logic is already proven. Either source it or port the relevant functions. The monitor's `sparkline_chart` and `multitrace_chart` functions are the reference implementation.

### Computing deltas

Use `compute_deltas` from common.sh to get `_spawns` and `_deaths` per tick. Also count spawns directly as `age == 0` processes in the current snapshot.

---

## Data Source

Tail `$CC_VIZ_DATA`. Compute spawns (age==0) and deaths (PIDs absent from current tick) per tick.

## File

`cc-viz/waveform.sh` — executable, `#!/usr/bin/env bash`, sources `common.sh`.
