# Debug: monitor.sh rendering issues

## Context

We added two features to the monitor tonight:

1. **Burst detection** (`sense_activity` / `sense_push` in `agents.sh`) — detects agent-like activity when SubagentStart/Stop hooks don't fire (e.g., Explore agents). Uses fast-tick polling (300ms) between 2s render cycles. Dual-signal: subtree CPU elevated + bash child count. Rolling window with debounce.

2. **Process tree formatting** (render_tree in `monitor.sh`) — attempted to fix column alignment and path readability.

Both features have passing tests (`bats tests/` — 70/70 green). The burst detection logic is solid. The rendering is broken.

## What's broken

### Process tree rendering (`render_tree()` around line 186-261)

The tree is visually garbled:
- Columns (PID, CPU%, RSS, command) don't align across rows
- Tree connectors (├─ └─ │) shift column positions because they're multi-byte UTF-8 but single-cell width, and ANSI color codes add invisible bytes
- The `printf` field width counts bytes, not display cells — so rows with tree prefixes misalign against the root row
- Commands still show long garbled paths sometimes

### What was tried (and failed)

1. Changed PID from `%d` (variable width) to `%6d` (fixed right-aligned)
2. Changed CPU from `%5s%%` to `%5.1f%%`
3. Changed RSS from `%5dM` to `%6s` with conditional GB formatting
4. Added `gsub()` calls to shorten paths — **`[^/]` in regex crashes macOS nawk** (interprets `/` as regex delimiter inside bracket class). Fixed with `match()` approach but the path shortening is still fragile.
5. Multiple iterations of format string tweaks — each fix shifted other columns
6. **Two-phase render_tree rewrite** (2026-03-31) — separated tree prefix (ANSI + box-drawing) from data columns via `sprintf` into a `data` var, then combined with `printf "%s%s\n", tree_pfx, data`. Root row used `sprintf` with BLD/RST around PID; child rows used a plain `sprintf("%6d  %5.1f%%  %6s  %s", ...)`. Tests passed (70/70) and the `echo "q" | bash scripts/monitor.sh` smoke test looked aligned in piped output, but **in a live tmux pane the columns were still garbled** — the two-phase approach didn't fix the visible rendering. Reverted.
7. **Alternate screen buffer (`tput smcup`/`rmcup`)** (2026-03-31) — added `tput smcup` on entry and `tput rmcup` in cleanup to prevent scrollback pollution (stacked headers when scrolling up). The idea was sound (htop/vim do this), but **did not fix the scrollback stacking in practice** — the repeated COOLANT/AGENTS headers still appeared when scrolling. Reverted.

### Root cause

The fundamental problem: **tree prefix (`pfx`) contains ANSI escape codes and multi-byte UTF-8 box-drawing characters**. When you do `printf "%s%s├─%s%6d ..."`, the `%s` fields for DIM/pfx/RST add invisible bytes that printf doesn't know about. There's no way to align columns with a single printf when the prefix has variable invisible content.

### Recommended approach (tried — see #6 above, did not fix visible rendering)

**Separate the tree prefix from the data columns.** Two-phase rendering:

1. Build the tree prefix string (indentation + connector) with ANSI codes
2. Build the data columns (PID, CPU, RSS, CMD) as a fixed-width formatted string independently
3. Concatenate: `prefix + data`

This way the data columns are always the same width regardless of tree depth. The tree prefix just pushes everything right, but the columns stay internally aligned.

Something like:
```awk
# Phase 1: tree prefix (variable width, includes ANSI)
if (islst == -1) tree_pfx = "  "
else if (islst == 1) tree_pfx = DIM pfx "└─" RST
else tree_pfx = DIM pfx "├─" RST

# Phase 2: data columns (fixed width, no tree influence)
data = sprintf("%6d  %5.1f%%  %6s  %s", p, cpu[p]+0, mem_str, command[p])

# Combine
printf "%s%s\n", tree_pfx, data
```

### Events spam (fixed with debounce, but needs validation)

The `SENSED_DEBOUNCE=6` constant requires 6 consecutive fast ticks (~2s) in the new state before logging. This prevents the rapid on/off/on/off cycling seen in screenshots. The user also wants **grouped/summarized events** rather than per-tick noise — think "30 processes spawned" rather than "sensed/cleared/sensed/cleared".

## Files involved

- `scripts/monitor.sh` — `render_tree()` (lines 186-261), `main()` fast-tick loop (lines 496-530), `pressure_badge()` (lines 104-120), header rendering (lines 312-330)
- `scripts/agents.sh` — `sense_activity()` (lines 216-232), `sense_push()` (lines 234-261)
- `tests/agents.bats` — 19 tests including 6 for burst detection

## What's working

- All 70 tests pass
- Burst detection logic (sense_activity, sense_push, rolling window) is correct
- Subtree CPU summing (not just claude PID) is correct
- Fast-tick polling architecture works
- The sparkline charts, agent gauge, pressure badge all render fine
- Events debounce is in place

## What to do

1. Fix `render_tree()` using the two-phase approach above
2. Smoke test: `echo "q" | bash scripts/monitor.sh --refresh 1` and visually verify alignment
3. Run `bats tests/` to confirm nothing broke
4. Consider: should events show process spawn/exit counts rather than sensed state transitions?

## Useful commands

```bash
# Run monitor
bash scripts/monitor.sh

# Run tests
bats tests/
bats tests/agents.bats

# Test render_tree awk in isolation (use PID of your claude session)
ALL_PROCS=$(ps -Ao pid=,ppid=,%cpu=,rss=,args= 2>/dev/null)
# paste the awk block from render_tree() and test against $ALL_PROCS

# Watch for nawk compatibility — avoid [^/] in regex, test ternary in printf args
```
