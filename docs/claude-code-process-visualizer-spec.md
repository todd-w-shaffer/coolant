# Claude Code Subprocess Visualizer

A set of ANSI terminal scripts designed to run in separate tmux panes, all consuming the same process snapshot data, each rendering a different view. The goal: detect the moment when parallel agents start closing out coding tasks and spawn a flood of subprocesses, before your page file balloons and your machine locks up.

---

## Shared Data Contract

Every pane reads from the same source. Each tick (1 second), the data source produces a snapshot of all currently alive subprocesses spawned by Claude Code.

### Data Format

A JSONL file (append-only) written to a known path, e.g. `/tmp/cc-procs.jsonl`. One line per tick:

```json
{"tick":0,"ts":"2026-03-31T10:00:00Z","procs":[{"pid":1234,"type":"N","age":0},{"pid":1235,"type":"G","age":2}]}
{"tick":1,"ts":"2026-03-31T10:00:01Z","procs":[{"pid":1234,"type":"N","age":1},{"pid":1235,"type":"G","age":3},{"pid":1240,"type":"V","age":0}]}
```

Each process entry:

- `pid`: the OS process ID
- `type`: a single-character label derived from the command name. Mapping: `N` = node, `G` = grep, `V` = vitest, `S` = sed, `R` = ripgrep, `F` = find, `C` = cat, `P` = python, `T` = tsc/typescript compiler, `X` = anything else.
- `age`: seconds since this process first appeared (0 means it spawned this tick)

### Derived Signals

Each pane can compute these from consecutive ticks:

- **spawns**: processes present in tick N with `age == 0` (or present in tick N but absent in tick N-1)
- **deaths**: processes present in tick N-1 but absent in tick N
- **net**: spawns minus deaths
- **total**: length of the `procs` array

### Implementation Options

**Option A (recommended): Shared collector daemon.** A single script does the `ps` scrape, writes JSONL. All panes tail the file. This guarantees all views see identical snapshots.

**Option B: Independent scraping.** Each pane runs its own `ps` loop. Simpler (no coordination), but snapshots may differ slightly between panes. Fine for most purposes.

### Collector Script (if using Option A)

This is the data producer. It runs in its own tmux pane or as a background process. Every second it:

1. Runs `ps --ppid <claude_code_pid> -o pid,comm,etimes --no-headers` (or walks `/proc/<pid>/task` for the full process tree)
2. Maps each `comm` value to a single-character type label
3. Computes `age` by tracking first-seen time per PID in memory
4. Writes one JSON line to `/tmp/cc-procs.jsonl`
5. Optionally caps the file at N lines (ring buffer) so it doesn't grow forever

The `claude_code_pid` should be passed as an argument or detected automatically (e.g. `pgrep -f "claude"` or similar).

---

## Prompt 1: Skyline Pane

### Purpose

Show total live process count as a vertical stack of type labels, one column per tick, scrolling left to right. This is the "shape" view. You glance at it and the silhouette tells you whether things are calm or exploding. The height of the column IS the signal. You perceive slope change instantly without reading anything.

### Rendering Rules

- Terminal width determines visible columns. Measure with `tput cols` on startup and on SIGWINCH.
- Each column is 3 characters wide: the type letter, a space, and a column separator. So a terminal 120 chars wide fits 40 columns of history.
- Maintain a ring buffer of the last N snapshots (where N = number of visible columns).
- Every tick: push the new snapshot onto the ring buffer, drop the oldest if full, redraw.
- Each column is rendered bottom-to-top. The process types within a column are sorted in a stable order (alphabetical by type label, or by PID, pick one and be consistent). This means a persistent `N` process stays in the same vertical slot across ticks, which helps your eye track continuity.
- Empty space above the stack is blank.
- The tallest the skyline can get is the terminal height minus 2 (one row for the separator line, one for the count footer).
- If a tick has more processes than available rows, truncate from the top and show a `+` or `^` indicator meaning "there are more above."

### Color

- Each type letter gets a fixed ANSI color. Suggested: `N`=green, `G`=yellow, `V`=red, `S`=cyan, `R`=magenta, `F`=blue, `C`=white, `P`=bright yellow, `T`=bright cyan, `X`=gray.
- The separator line is dim white.
- The count footer shows the number in bold, and changes color at thresholds: white below 20, yellow 20-49, red 50+.

### Footer Row

Below the separator, show the total process count for each column, right-aligned within the 3-char cell. This is the numeric version of the height.

### Example Output (showing ~10 columns of a 40-column display)

```
                              G  N
                           N  V  G
                        G  G  N  V
                     N  N  N  G  N
                  N  G  G  G  N  G
               G  G  V  V  V  G  N
      N  N  N  N  N  N  N  N  N  N
N  N  G  G  G  G  G  G  G  G  G  G
G  G  V  V  V  V  V  V  V  V  V  V
──────────────────────────────────────
 3  4  5  6  6  7  8  9 10 12 14 16
```

### Redraw Strategy

Use ANSI escape codes to move cursor to top-left and redraw in place (no flicker). Specifically: `\033[H` to home cursor, then overwrite each line. Pad lines with spaces to clear any leftover characters from the previous frame.

### Input

Tail `/tmp/cc-procs.jsonl` and parse each new line. On each new line, update the ring buffer and redraw.

---

## Prompt 2: Spawn Rate Pane

### Purpose

Show the rate of process creation and destruction over time as a sparkline-style display. This is the "derivative" view. It answers: how fast are new processes appearing? The skyline shows the integral (total count), this pane shows the differential (rate of change). This is your early warning system.

### Rendering Rules

Three sparkline rows, each spanning the terminal width, scrolling left in sync with the skyline:

1. **Spawns per tick** (top row): how many new processes appeared this tick (`age == 0`).
2. **Deaths per tick** (middle row): how many processes disappeared since last tick.
3. **Net per tick** (bottom row): spawns minus deaths.

Each cell in the sparkline is a single Unicode block character representing the value. Use the standard sparkline blocks: `▁▂▃▄▅▆▇█`. Map the value to one of these 8 levels based on a configurable scale. For example, if max expected spawns/sec is 40, then each block represents 5 units. Values exceeding the scale get `█` (clamped).

### Layout

```
 +spawns  ▁▁▁▂▂▃▃▅▅▆▇█▇█▇█████████████
 -deaths  ▁▁▁▁▁▁▂▂▂▃▃▅▃▅▃▅▅▅▅▅▆▆▆▇▇▇▇
  net     ▁▁▁▁▂▂▂▃▃▄▅█▅█▅██████████████

  spawns: 14/s  deaths: 6/s  net: +8/s  peak_net: +22/s
```

The labels on the left are fixed-width (10 chars). The sparkline fills the remaining terminal width.

The summary line at the bottom shows current-tick values and the peak net value seen during this session.

### Color

- Spawns row: green when low (below threshold A), yellow when moderate, red when high (above threshold B).
- Deaths row: always dim white or gray (deaths are expected; they're not the alarm).
- Net row: green when low, yellow when moderate, red when high. Color is per-character based on the value that character represents.
- Summary line: the `net` value uses the same color thresholds.

### Thresholds (configurable via env vars or args)

- `SPAWN_WARN=10` (spawns/sec to go yellow)
- `SPAWN_CRIT=20` (spawns/sec to go red)
- `NET_WARN=5` (net/sec to go yellow)
- `NET_CRIT=15` (net/sec to go red)

### Input

Same as skyline: tail `/tmp/cc-procs.jsonl`. Compute deltas between consecutive ticks.

---

## Prompt 3: Process Type Breakdown Pane

### Purpose

Show a horizontal bar chart of currently alive processes, grouped by type. Answers: "is it all vitest, or is it grep that's going nuts?" Updates every tick.

### Rendering Rules

One row per process type. Only show types that currently have at least one process. Sorted by count, descending. The bar is made of `█` (full block) characters. The bar length is proportional to the count, scaled so the longest bar fills the available width (terminal width minus the label and count columns).

### Layout

```
  N ████████████████████████████░░░░░░  41
  G ██████████████░░░░░░░░░░░░░░░░░░░  22
  V ████████░░░░░░░░░░░░░░░░░░░░░░░░░  11
  S ████░░░░░░░░░░░░░░░░░░░░░░░░░░░░░   5
  R ██░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░   3

  total: 82    types: 5
```

The `░` (light shade) characters fill the remainder of the bar to the max width, giving a "progress bar" feel so you can see the proportion at a glance. The count is right-aligned at the end.

### Color

Each type letter and its bar use the same color as the skyline pane (same color mapping). This creates visual continuity across panes.

### Footer

Show total process count and number of distinct types. Color the total using the same thresholds as the skyline footer.

### Behavior

This pane redraws in place every tick. When a process type dies off completely, its row disappears. When a new type appears, it gets inserted in sorted order. The bars rescale every tick (the longest bar always fills the width), so you see relative proportions, not absolute scale. Absolute scale is what the skyline is for.

### Input

Same JSONL tail. Group current tick's procs by type, count each group.

---

## Prompt 4: Alert Log Pane

### Purpose

A scrolling text log that fires timestamped entries when configurable thresholds are crossed. This is the pane you wire into your notification system later. It's also the historical record: the other three panes show "right now," this one shows "what happened."

### Rules

This pane does NOT redraw in place. It appends lines and scrolls naturally, like a normal log. Let the terminal handle scrollback.

### Alert Conditions (each independently configurable)

1. **WARN spawn rate**: triggered when spawns/sec exceeds `SPAWN_WARN` for 1 tick.
2. **CRIT spawn rate**: triggered when spawns/sec exceeds `SPAWN_CRIT` for 1 tick.
3. **WARN net rate**: triggered when net/sec exceeds `NET_WARN` for `NET_SUSTAIN` consecutive ticks (default 3). A single spike is normal; sustained growth is the danger signal.
4. **CRIT net rate**: triggered when net/sec exceeds `NET_CRIT` for 1 tick.
5. **Total count threshold**: triggered when total live processes exceeds `TOTAL_WARN` (default 50) or `TOTAL_CRIT` (default 100).
6. **Type explosion**: triggered when any single process type's count exceeds `TYPE_CRIT` (default 30). Log which type.
7. **Calm restored**: triggered when all metrics drop below WARN levels after having been in WARN or CRIT. Useful for knowing when a storm passed.

### Output Format

```
14:32:07 INFO  total:12 spawns:2/s net:+1/s
14:32:08 WARN  spawn rate 14/s exceeds threshold (10/s)
14:32:09 CRIT  spawn rate 23/s exceeds threshold (20/s)
14:32:09 CRIT  type N count 31 exceeds threshold (30)
14:32:10 CRIT  net rate +18/s sustained for 3 ticks
14:32:15 INFO  calm restored, all metrics below WARN
```

### Color

- `INFO` lines: dim white/gray
- `WARN` lines: yellow, the word WARN is bold yellow
- `CRIT` lines: red, the word CRIT is bold red, and optionally the entire line background is dim red for maximum visibility

### Periodic Summary

Every 30 seconds (configurable), emit a summary `INFO` line even if no alerts fired, showing current total, spawn rate, and net rate. This confirms the pane is alive and gives a baseline pulse.

### Input

Same JSONL tail. Compute all metrics per tick, check thresholds, emit lines.

---

## Prompt 5: Integration and Launcher

### Purpose

A single launcher script that ties everything together. It:

1. Starts the data collector (if using the shared collector approach).
2. Creates a tmux session with a 2x2 grid layout.
3. Launches each pane script in its designated quadrant.
4. Handles cleanup on exit (kill collector, remove temp files).

### Layout

```
tmux layout:

┌─────────────────────────┬─────────────────────────┐
│                         │                         │
│   Pane 1: Skyline       │   Pane 2: Spawn Rate    │
│                         │                         │
├─────────────────────────┼─────────────────────────┤
│                         │                         │
│   Pane 3: Breakdown     │   Pane 4: Alert Log     │
│                         │                         │
└─────────────────────────┴─────────────────────────┘
```

### Script Responsibilities

The launcher script (`cc-viz.sh`) should:

1. Accept `--pid <PID>` to specify the Claude Code parent process, or auto-detect it.
2. Accept `--collector-only` to just run the data collector without tmux (for debugging).
3. Accept `--demo` to run with synthetic data (random process spawning/dying) so you can test the visualization without Claude Code running.
4. Set up env vars for thresholds, passing them to all child scripts.
5. Create the tmux session: `cc-viz`
6. Split into 4 panes with the 2x2 layout.
7. Start the collector in the background (writing to `/tmp/cc-procs.jsonl`).
8. Send the appropriate script command to each pane.
9. Trap EXIT/INT/TERM to kill the collector and clean up `/tmp/cc-procs.jsonl`.

### Synthetic Data Mode (`--demo`)

For testing without Claude Code, the collector generates fake data that simulates the "calm then explosion" pattern:

- Ticks 0-30: baseline, 3-8 processes, mostly N and G, occasional V.
- Ticks 30-45: ramp up, agents start finishing, spawn rate increases linearly.
- Ticks 45-60: explosion, 20-40 new spawns per tick, all types active.
- Ticks 60-75: plateau, high count but spawn rate stabilizes.
- Ticks 75-90: die-off, processes completing, death rate exceeds spawn rate.
- Ticks 90+: loop back to baseline.

This gives you a repeating ~90-second cycle that exercises all the views and triggers all the alert thresholds.

### File Layout

```
cc-viz/
  cc-viz.sh          # launcher (this prompt)
  collector.sh       # data collector daemon
  skyline.sh         # pane 1
  spawnrate.sh       # pane 2
  breakdown.sh       # pane 3
  alertlog.sh        # pane 4
  common.sh          # shared functions (JSONL parsing, color codes, type mapping)
```

`common.sh` is sourced by all four pane scripts. It contains:

- ANSI color code variables
- The type-label-to-color mapping
- A function to parse a JSONL line into shell variables
- Threshold defaults (overridable by env vars)
- A function to compute spawns/deaths/net from two consecutive snapshots

---

## Implementation Notes

### Language

All scripts are bash. Use `jq` for JSON parsing (it's fast enough for 1 tick/sec). Use `tput` for terminal dimensions and basic cursor control. Use raw ANSI escape codes for colors and cursor positioning within the render loop (faster than calling `tput` repeatedly).

### Dependencies

- `bash` 4+
- `jq`
- `tmux`
- `ps` (procps)
- A terminal that supports Unicode block characters (virtually all modern terminals)

### Performance

The render loop for each pane should complete well under 100ms. The bottleneck is the `ps` scrape in the collector, which should also be well under 100ms even with hundreds of processes. At 1 tick/sec this is very comfortable.

### Testing

Build and test each pane independently first using `--demo` mode or a pre-recorded JSONL file. Pipe a static file through `tail -f` to simulate live data:

```bash
# Record real data
./collector.sh --pid 12345 > /tmp/cc-procs-recording.jsonl

# Replay later
cat /tmp/cc-procs-recording.jsonl | while IFS= read -r line; do
  echo "$line" >> /tmp/cc-procs.jsonl
  sleep 1
done
```
