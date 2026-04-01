# Heatmap Spectrogram

**Replaces:** Skyline pane (skyline.sh)

**Why:** The skyline encodes count as column height, which wastes vertical space during calm periods and clips during explosions. The heatmap encodes count as color intensity on a fixed grid — it scales to any pane size without clipping or wasted space, and reveals per-type temporal patterns that the skyline buries in a stacked column.

**Unique signal:** Which process type is hot *when*. The skyline tells you "a lot of stuff is running." The heatmap tells you "node went hot at tick 40, grep followed at tick 43, vitest exploded at tick 45." Lane patterns are instantly readable — synchronized bands mean correlated activity, staggered bands mean cascading spawns.

---

## Layout

**Position:** Top-left quadrant of the 2x2 tmux grid (same slot the skyline occupied).

**Grid structure:** Rows are process types (one per type that has appeared during the session, sorted alphabetically). Columns are time ticks, scrolling left. Each cell is a single background-colored space character — no text content.

```
  N ██████████████████████████████████████████████
  G ██████████████████████████████████████████████
  V ██████████████████████████████████████████████
  S ██████████████████████████████████████████████
  R ██████████████████████████████████████████████
  F ██████████████████████████████████████████████
```

The left margin is 4 chars (2 space indent + type letter + space). The remaining terminal width is all heatmap cells, one per tick.

## Rendering Rules

### Cell color

Each cell represents the count of processes of that type at that tick. Map count to one of 6 intensity levels using ANSI 256-color or 24-bit background colors:

| Count | Intensity | Color approach |
|-------|-----------|---------------|
| 0 | Black (empty) | Default background |
| 1 | Dim | `\033[48;5;232m` (gray 1) |
| 2-3 | Low | `\033[48;5;235m` (gray 4) |
| 4-7 | Medium | `\033[48;5;130m` (dim orange) |
| 8-15 | High | `\033[48;5;208m` (orange) |
| 16+ | White-hot | `\033[48;5;196m` (bright red) → `\033[48;5;231m` (white) |

Use the type's foreground color from common.sh for the row label only. The cells themselves are pure background — a space character with ANSI background set.

### Row management

- Only show types that have appeared at least once during the session (sticky — once a type shows up, its lane persists even if count drops to zero, so the grid doesn't jump).
- Sort rows alphabetically by type letter (stable layout).
- Max rows = number of distinct types seen (typically 8-10, well within any pane height).

### Scrolling

- Maintain a ring buffer of snapshots, one per tick, sized to available columns (terminal width minus label margin).
- Each tick: push new snapshot, drop oldest, redraw.

### Header

None. The type labels on the left are sufficient. Every pixel of width goes to the heatmap.

---

## Implementation Constraints

These are hard-won lessons from earlier cc-viz work:

1. **Buffered rendering.** Capture entire frame into a variable via `frame=$(render)`, then `tput home && printf '%s' "$frame" && tput ed`. Never `clear` per frame.

2. **Cache tput cols.** Call `COLS=$(tput cols)` *before* the `$(render)` subshell. Inside the subshell, `tput cols` can't read the terminal fd.

3. **Process substitution.** Use `while read line; do ... done < <(tail -n 0 -f "$CC_VIZ_DATA")` not `tail | while`. The pipe creates a subshell that loses state between iterations.

4. **Source cc-viz/common.sh** (not scripts/common.sh). Use `SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"` then `source "$SCRIPT_DIR/common.sh"`.

5. **Bash 3.2 compatible.** No `mapfile`, no associative arrays, no `|&`. Use parallel plain arrays or space-delimited strings.

6. **Parse JSONL via common.sh.** Use `eval "$(parse_jsonl_line "$line")"` to get `_proc_types`, `_proc_ages`, `_proc_pids`, `_count`.

---

## Data Source

Same as all cc-viz panes: tail `$CC_VIZ_DATA` (default `/tmp/cc-procs.jsonl`). Each line is a tick snapshot with a `procs` array. Group by type letter, count per type per tick.

## File

`cc-viz/heatmap.sh` — executable, `#!/usr/bin/env bash`, sources `common.sh`.
