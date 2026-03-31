# Coolant monitor UI exploration — from snapshot bars to sparkline charts

I have a bash TUI monitor (`scripts/monitor.sh`) for a Claude Code resource management plugin. It runs in a tmux pane and shows system gauges and process trees. I want to evolve its UI toward the density and interactivity of `bottom` (btm) while keeping its existing braille aesthetic.

## What we have now

Current monitor layout, top to bottom:

- Header: mode badge (idle/PARALLEL), agent count, threshold, timestamp
- SYSTEM section: three single-line braille fill bars — CPU, MEM, SWAP — each showing a snapshot percentage with inline stats (load avg, used/total, core count). A PRES (pressure) badge showing NORMAL/WARN/CRITICAL as a colored pill.
- PROCESSES section: tree view with box-drawing connectors (├─ └─ │), each node showing pid, cpu%, rss in MB, and truncated command. Subtree summary line at bottom with totals.
- EVENTS section: recent hook events from the coolant log, dimmed text
- Footer: refresh rate, quit hint, version
- Everything is full-width vertical stacking. No columns, no history, no interactivity beyond 'q' to quit.

## Where we want to go

btm-inspired, specific gaps to close:

1. **Time-series sparkline charts** replacing (or supplementing) the flat CPU bar. 60 seconds of history scrolling left on each refresh. This is the single biggest visual upgrade.
2. **Per-core breakdown with selectable focus.** A right-side legend listing each core with a live percentage. Selecting "All" overlays every core in distinct colors; selecting a single core isolates its trace.
3. **Multi-color overlaid traces** in "All" mode — each core gets its own color (red, green, yellow, magenta, etc.) on the same chart area.
4. **Box-drawing chart frames** — `┌─ CPU — 2.44 2.37 2.25 ─┐` with the chart inside, replacing the current flat section header + horizontal rule.
5. **Column layout** — chart on the left, core selector on the right. Requires computing column widths from terminal width.
6. **Data in the chart title** — load averages embedded in the top border, like btm does.

## What must be preserved

- The braille character vocabulary: `⠀ ⣀ ⣄ ⣆ ⣇ ⣧ ⣷ ⣿` for fill levels, `⠂` for empty, `⡇` as dim separators — this is the shared DNA with the Claude statusline
- Zone-based thermometer coloring: green (<50%), yellow (50-70%), red (>=70%)
- ANSI only — no 256-color, no truecolor. Bold, dim, and the basic 8 colors
- The process tree section — btm doesn't have this, it's coolant's unique value
- The pressure badge — good at-a-glance severity signal
- The event log — ties into the hook system
- Compact header with mode + agent count
- Dark background (Ghostty terminal), monospace font
- Buffered rendering (home-and-overwrite, no `clear` per frame)

## Constraints

- Pure bash 3.2 (macOS system bash). No `mapfile`, no associative arrays, no `|&`
- No external dependencies — only `sysctl`, `vm_stat`, `ps`, `awk`, `tput`, standard coreutils
- Must fit in a tmux pane (could be full-width, typically 40-60 rows tall)
- Refresh rate is configurable (default 2s) — history data must persist across refreshes
- State files live in `/tmp/coolant-$USER.*`

---

## Braille sparkline rendering — the math

Unicode braille characters (U+2800–U+28FF) encode a 2-wide × 4-tall dot grid in a single character cell. Each dot maps to a bit:

```
col 0    col 1
 ●(0x01)  ●(0x08)   row 0 (top)
 ●(0x02)  ●(0x10)   row 1
 ●(0x04)  ●(0x20)   row 2
 ●(0x40)  ●(0x80)   row 3 (bottom)
```

A character's codepoint = `0x2800 + (sum of active dot bits)`. This means:

- **Vertical resolution**: each character row represents 4 dot-rows. A 5-row chart = 20 dot-rows = 5% per dot at 0–100% scale.
- **Horizontal resolution**: each character column represents 2 data points. A 40-character-wide chart = 80 data points = 160 seconds of history at 2s refresh.

### Column-pair rendering

For each pair of adjacent data points `(v_left, v_right)` at chart column `x`:

1. Map values to dot-row space: `dot_y = value * (chart_rows * 4 - 1) / 100`
2. For each character row `r` (0 = top, chart_rows-1 = bottom):
   - The dot-rows this cell covers are `[(chart_rows - 1 - r) * 4 .. (chart_rows - 1 - r) * 4 + 3]`
   - For each of the 4 dot positions in the left column, light the dot if `dot_y_left >= that dot-row`
   - Same for right column with `dot_y_right`
3. OR the bits together → codepoint → emit the character

This gives a **filled-area** chart (like btm's default). For a **line-only** chart, only light the single dot-row closest to the value, not everything below it.

### Bash 3.2 implementation path

The per-character bit math is too slow in a bash loop for 40×5 = 200 cells per frame. The move is to do it in **one awk call** per frame:

```
printf '%s\n' "${HISTORY[@]}" | awk -v rows=5 -v width=40 '
  # read data points into array, compute bit patterns, emit braille
'
```

awk can do the arithmetic, build the codepoint, and emit UTF-8 bytes directly. The braille block is in the BMP, so UTF-8 encoding for codepoint `cp`:

```
byte1 = 0xE0 + rshift(cp, 12)
byte2 = 0x80 + and(rshift(cp, 6), 0x3F)
byte3 = 0x80 + and(cp, 0x3F)
```

awk's `printf "%c", n` works for ASCII but not multi-byte. On macOS awk (one true awk / nawk), we need `printf "\\%03o\\%03o\\%03o"` with the three bytes — and then pipe through `printf` or use the shell's `$'...'`. The cleanest path: awk outputs octal escape sequences, shell `printf` interprets them.

### Color zones in sparklines

The chart should color each column based on its value using the same thresholds as the current bars:

- Green: value < 50%
- Yellow: 50% ≤ value < 70%
- Red: value ≥ 70%

Since ANSI color codes apply to whole characters and each character spans two data points, use the **max** of the two column values to pick the color. This occasionally over-warns but never under-warns.

---

## Approach A: Sparkline strip (minimal surgery)

### Visual layout

```
 COOLANT  idle                               22:51:34
 agents: 0  threshold: 3.0%

 SYSTEM ──────────────────────────────────────────────
 CPU  ⣿⣿⣿⣿⣿⣿⠂⠂⠂⠂⠂⠂⠂⠂⠂⠂⠂⠂⠂⠂  29%  load 2.38 2.62 2.40  (8 cores)
      ⣀⣀⣠⣤⣶⣿⣿⣷⣦⣤⣀⣀⣠⣤⣶⣿⣿⣷⣦⣤⣀⣀⣠⣤⣶⣿⣷⣦⣤⣀⣀⣠⣤⣶⣿⣷⣦⣤   ← 1-row sparkline (60s)
 MEM  ⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⠂⠂⠂⠂⠂⠂⠂⠂  58%  9.4G / 16.0G
 SWAP ⣿⠂⠂⠂⠂⠂⠂⠂⠂⠂⠂⠂⠂⠂⠂⠂⠂⠂⠂⠂   0%  0M / 3300M
 PRES  NORMAL
```

A single braille row under the CPU bar. Each character = 2 data points, 4 vertical dot levels. With 30 characters that's 60 data points = 120 seconds of history at 2s refresh.

### What changes

- **New**: A `CPU_HISTORY` indexed array (bash 3.2 supports `arr[0]`, `arr[1]`...) holding the last N CPU percentages. Shifted left each tick.
- **New**: An awk function that takes the history array and emits one row of braille characters with zone coloring.
- **Modified**: `render()` gains one `printf` line between the CPU bar and MEM bar.
- **Everything else unchanged** — header, MEM/SWAP bars, process tree, events, footer.

### Data flow

```
read_cpu() → appends CPU_PCT to CPU_HISTORY[] → render calls sparkline_row()
```

History stored in a bash indexed array. No file I/O — lives in the process's memory across the main loop iterations. Lost on restart, which is fine for a 60-second window.

### Key challenges

1. **1-row resolution is only 4 levels** (0%, 25%, 50%, 75%, 100%). It's recognizable as a sparkline but coarse. Values 0–24% all look the same.
2. **No per-core breakdown** — aggregate CPU only.
3. **Single color per character** means the sparkline can't show distinct zones within one character cell.

### Verdict

Lowest risk, fastest to implement (<1 hour), immediately ships value. Natural stepping stone — the sparkline rendering awk can be promoted to multi-row later.

---

## Approach B: Boxed multi-row chart with core legend (btm layout)

### Visual layout

```
 COOLANT  idle                                          22:51:34
 agents: 0  threshold: 3.0%

 ┌─ CPU ── 2.38 2.62 2.40 ──────────────────────┐┌─────────┐
 │⣀⣀⣠⣤⣶⣿⣿⣷⣦⣤⣀⣀⣠⣤⣶⣿⣿⣷⣦⣤⣀⣀⣠⣤⣶⣿⣷⣦⣤⣀⣀⣠⣤⣶⣿⣷⣦⣤││ All  29%│
 │⣀⣀⣠⣤⣶⣿⣿⣷⣦⣤⣀⣀⣠⣤⣶⣿⣿⣷⣦⣤⣀⣀⣠⣤⣶⣿⣷⣦⣤⣀⣀⣠⣤⣶⣿⣷⣦⣤││ 0   42%│
 │⣀⣀⣠⣤⣶⣿⣿⣷⣦⣤⣀⣀⣠⣤⣶⣿⣿⣷⣦⣤⣀⣀⣠⣤⣶⣿⣷⣦⣤⣀⣀⣠⣤⣶⣿⣷⣦⣤││ 1   18%│
 │⣀⣀⣠⣤⣶⣿⣿⣷⣦⣤⣀⣀⣠⣤⣶⣿⣿⣷⣦⣤⣀⣀⣠⣤⣶⣿⣷⣦⣤⣀⣀⣠⣤⣶⣿⣷⣦⣤││ 2    5%│
 │⣀⣀⣠⣤⣶⣿⣿⣷⣦⣤⣀⣀⣠⣤⣶⣿⣿⣷⣦⣤⣀⣀⣠⣤⣶⣿⣷⣦⣤⣀⣀⣠⣤⣶⣿⣷⣦⣤││ 3   12%│
 │                              100%│  50%│   0%││ ...    │
 └───────────────────────────────────────────────┘└─────────┘
 ┌─ MEM ─ 9.4G/16.0G ─┐ ┌─ SWAP ─ 0M/3300M ─┐
 │ ⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⠂⠂  58% │ │ ⣿⠂⠂⠂⠂⠂⠂⠂⠂⠂  0%  │  PRES  NORMAL
 └─────────────────────┘ └────────────────────┘

 PROCESSES ───────────────────────────────────────────
   2015  0.0%  715M  claude —permission-mode bypassPermis
   ...

 EVENTS ──────────────────────────────────────────────
   no recent events
```

### What changes

**Chart area**: 5 braille rows × ~40 chars = 20 dot-rows vertical × 80 data points horizontal. Resolution: 5% per dot-row, 160 seconds of history. This is btm-quality density.

**Core legend**: Right-side panel, 10 chars wide. Lists "All" + each core (0–7) with current percentage. Highlighted row = selected trace. Arrow keys or number keys to select.

**Box frames**: `┌─┐│└─┘` around chart and legend. Title embedded in top border.

**MEM/SWAP**: Collapse into a pair of small boxed bars below the chart, side by side.

**Column layout math**: `chart_width = terminal_cols - legend_width - frame_chars - padding`. Legend is fixed at 10 chars. Frame chars = 4 (two boxes, two borders each side). So chart inner width = `cols - 16`.

### Data flow

```
Per-core history:
  top -l 1 -n 0 (or ps sampling trick) → core percentages → CORE_HISTORY_0[], CORE_HISTORY_1[], ...

Aggregate:
  CPU_HISTORY[] = mean of all core histories

Render:
  Selected == "All" → overlay all CORE_HISTORY_N[] with distinct colors
  Selected == N     → render CORE_HISTORY_N[] in green
```

### Per-core data: the hard part

macOS doesn't expose per-core utilization in sysctl. Options:

1. **`top -l 2 -n 0 -stats cpu`** — second sample gives system-wide CPU breakdown (user/sys/idle) but NOT per-core.
2. **`powermetrics --samplers cpu_power -n 1 -i 1000`** — gives per-core frequency and residency, but requires sudo. Non-starter.
3. **Synthetic per-core from ps** — sum cpu% of all processes, assume kernel distributes across cores. This is what btm actually does on macOS (via sysinfo crate, which reads host_processor_info Mach call). We can't do the Mach call from bash.
4. **`/usr/bin/sar -P ALL 1 1`** — not available by default on macOS.

**Realistic fallback**: Show aggregate CPU as the main trace. If we want multiple traces, overlay CPU + MEM + SWAP as three colored lines on the same chart — different data, same visual density.

### Key challenges

1. **Column layout** requires computing widths per-frame and padding/truncating. Not hard, but fiddly in bash.
2. **Box drawing + braille in the same line** — box characters are 1 cell wide but variable byte length. Printf field widths count bytes, not characters. Need to pad with spaces explicitly.
3. **Per-core data is effectively unavailable** without sudo or compiled helpers. The "core legend" design needs rethinking — could become a "trace legend" (CPU / MEM / SWAP) instead.
4. **Multi-color overlay** in a single braille character is impossible — ANSI colors the entire character. Two traces can only be distinguished if they don't overlap in the same cell. Where they overlap, use the higher value's color. This limits the visual utility to ~3 traces with distinct ranges.
5. **5-row chart eats 7 lines** (chart + top/bottom border). Combined with MEM/SWAP boxes, header, and process tree, easily 35+ lines. Tight in a half-screen tmux pane.

### Verdict

The full btm experience. Biggest visual impact, but per-core is a dead end without a compiled helper or sudo. Recommend pivoting "core legend" to "trace legend" (CPU/MEM/SWAP overlay). Implementation: 3–4 hours, mostly the awk chart renderer and column layout.

---

## Approach C: Stacked sparkline charts (vertical density, no columns)

### Visual layout

```
 COOLANT  idle                               22:51:34
 agents: 0  threshold: 3.0%

 ── CPU ── load 2.38 2.62 2.40 (8 cores) ── 29% ──
 ⣀⣀⣠⣤⣶⣿⣿⣷⣦⣤⣀⣀⣠⣤⣶⣿⣿⣷⣦⣤⣀⣀⣠⣤⣶⣿⣷⣦⣤⣀⣀⣠⣤⣶⣿⣷⣦⣤⣀⣀⣠⣤⣶⣿⣷⣦⣤
 ⣀⣀⣠⣤⣶⣿⣿⣷⣦⣤⣀⣀⣠⣤⣶⣿⣿⣷⣦⣤⣀⣀⣠⣤⣶⣿⣷⣦⣤⣀⣀⣠⣤⣶⣿⣷⣦⣤⣀⣀⣠⣤⣶⣿⣷⣦⣤
 ⣀⣀⣠⣤⣶⣿⣿⣷⣦⣤⣀⣀⣠⣤⣶⣿⣿⣷⣦⣤⣀⣀⣠⣤⣶⣿⣷⣦⣤⣀⣀⣠⣤⣶⣿⣷⣦⣤⣀⣀⣠⣤⣶⣿⣷⣦⣤

 ── MEM ── 9.4G / 16.0G ── 58% ────────────────────
 ⣀⣀⣠⣤⣶⣿⣿⣷⣦⣤⣀⣀⣠⣤⣶⣿⣿⣷⣦⣤⣀⣀⣠⣤⣶⣿⣷⣦⣤⣀⣀⣠⣤⣶⣿⣷⣦⣤⣀⣀⣠⣤⣶⣿⣷⣦⣤
 PRES  NORMAL

 PROCESSES ──────────────────────────────────────────
   2015  0.0%  715M  claude —permission-mode bypassPermis
   ...

 EVENTS ─────────────────────────────────────────────
   no recent events

 refresh 2s │ q: quit │ coolant v2
```

### What changes

**CPU chart**: 3 braille rows × full terminal width. 12 dot-rows = ~8% per dot-row resolution. At 80 cols, that's 160 data points = 320 seconds of history. Data embedded in the section header line.

**MEM chart**: 1 braille row sparkline — MEM is less volatile than CPU, so a trend line is enough.

**SWAP stays as a bar** (or gets folded into the MEM header) — SWAP barely changes on most machines.

**No column layout** — everything is full-width vertical stacking, same as current. No box frames, just the existing `── SECTION ──` style.

**No core legend** — aggregate only. Keeps the design honest about what data we can actually get.

### Data flow

```
CPU_HISTORY[]  — indexed array, shifted left each tick, up to terminal_width * 2 entries
MEM_HISTORY[]  — same structure

sparkline_chart $rows "${HISTORY[@]}" → emits $rows lines of braille
```

One shared awk renderer, parameterized by row count. CPU gets 3 rows, MEM gets 1 row.

### Storage: indexed arrays vs temp files

**Indexed arrays** (recommended): Bash 3.2 supports `arr[0]=val`, `${arr[@]}`, `${#arr[@]}`. A 160-element integer array is trivial. Shift with a for loop or `arr=("${arr[@]:1}" "$new_val")`.

**Temp files**: Write one value per line to `/tmp/coolant-$USER.cpu_history`. `tail -n 160` to trim. Survives restarts but adds file I/O per tick. Not worth it for ephemeral 60-second windows.

**Ring buffer in file**: Write index + values. Overcomplicated for bash.

Verdict: in-memory indexed arrays. Simple, fast, no I/O.

### Key challenges

1. **Full-width charts look great on wide terminals but waste space on narrow ones.** Need a minimum width check (say, 40 cols) and graceful degradation to the 1-row strip.
2. **3 rows of CPU + 1 row of MEM + section headers = 8 additional lines** vs current layout. Process tree might need truncation on short terminals.
3. **No interactivity** — this approach punts on per-core selection entirely. It's pure passive display.

### Verdict

Best balance of visual upgrade and implementation simplicity. Full-width means no column math. No per-core means no data collection problem. The 3-row CPU chart at full width is genuinely information-dense — 12 dot-rows × 160 columns = 1,920 data cells per frame. Implementation: 1.5–2 hours, almost entirely the awk sparkline renderer.

---

## Recommendation: C then B (incremental path)

**Ship Approach C first.** It delivers the single biggest visual upgrade (time-series CPU sparkline) with the least structural change. The full-width vertical layout means zero layout engine work — just slot the chart between the header and MEM bar.

**Build the awk chart renderer as a standalone function** — `sparkline_chart(rows, width, values[])`. This is the reusable core that all approaches need. Getting it right once in Approach C means Approach B's column layout just calls the same renderer with a narrower width.

**Graduate to Approach B when:**
- The chart renderer is proven and fast
- We have a real use for the legend panel (maybe trace selection, maybe process-specific sparklines)
- Terminal width detection is in place and tested

**Skip per-core** unless we later add a compiled sensor binary (the "compiled binary only if earned" philosophy from CLAUDE.md). The aggregate CPU sparkline is the 80/20 — per-core is cosmetic without a real per-core data source.

### Implementation sequence (Approach C)

1. **Red**: Write bats tests for history array management (shift, append, max-length trim)
2. **Green**: Implement `history_push()` / `history_trim()` helpers in common.sh or a new `sparkline.sh`
3. **Red**: Write a test that the awk renderer emits correct braille for known input values
4. **Green**: Implement the awk sparkline renderer
5. **Red**: Test zone coloring (green/yellow/red thresholds in sparkline output)
6. **Green**: Wire coloring into the awk renderer
7. **Refactor**: Extract renderer into a sourced function, integrate into `render()`, replace CPU snapshot bar with chart
8. **Manual smoke test**: run monitor, watch the sparkline fill in over 60 seconds
