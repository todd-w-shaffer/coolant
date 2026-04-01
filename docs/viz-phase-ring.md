# Phase Ring

**Replaces:** Nothing (net-new, not a pane).

**Unique signal:** System state classification over time. Every other visualization shows raw metrics (counts, rates, types). The phase ring is the only one that computes a *derived state* — it classifies each tick into a phase (calm/ramping/exploding/cooling) based on multi-signal analysis, then displays the trajectory as a rolling sequence of colored dots. It's the 30,000-foot view. You glance at it and see "green green green yellow yellow red red red red yellow green" — the story of the last minute in one line.

**Form factor:** Not a tmux pane. This is an inline widget that lives in the bottom status row alongside the breakdown pane, or in the tmux status bar itself.

---

## Layout

**Position:** Inline in the bottom full-span row, right-aligned after the breakdown chart. Alternatively, rendered as a tmux status-right component.

**Visual:**

```
  N ████████████████████████████░░░░░░  41     ● ● ● ● ● ● ● ● ● ● ● ● ● ● ●
  G ██████████████░░░░░░░░░░░░░░░░░░░  22     ▲ phase ring (last 15 ticks)
  ...
  total: 82    types: 5                        CALM
```

Or as tmux status bar:

```
coolant │ ● ● ● ● ● ● ● ● ● ● │ CALM
```

The ring is 15-30 characters wide (configurable). Each dot is a colored `●` (U+25CF) or `⬤` (U+2B24). One dot per tick.

## Phase Classification

Each tick, the system is classified into one of four phases based on current metrics:

| Phase | Color | Condition |
|-------|-------|-----------|
| CALM | Green (`\033[32m`) | total < TOTAL_WARN AND spawns < SPAWN_WARN AND net < NET_WARN |
| RAMPING | Yellow (`\033[33m`) | spawns >= SPAWN_WARN OR net >= NET_WARN (but below CRIT) |
| EXPLODING | Red (`\033[31m`) | spawns >= SPAWN_CRIT OR net >= NET_CRIT OR total >= TOTAL_CRIT |
| COOLING | Cyan (`\033[36m`) | total > TOTAL_WARN AND net < 0 (dying off but still elevated) |

### Priority

If multiple conditions are true, highest severity wins: EXPLODING > RAMPING > COOLING > CALM.

### Edge case

COOLING requires that the system was recently in RAMPING or EXPLODING (net negative implies die-off). If net is negative but total is low, it's just CALM.

## Rendering Rules

### Ring buffer

Maintain a ring buffer of the last N phase values (N = configurable width, default 20).

### Dot rendering

Each phase value maps to a colored `●`:

```bash
case "$phase" in
  calm)      printf '\033[32m●\033[0m' ;;
  ramping)   printf '\033[33m●\033[0m' ;;
  exploding) printf '\033[31m●\033[0m' ;;
  cooling)   printf '\033[36m●\033[0m' ;;
esac
```

Dots are printed left to right with a space between each: `● ● ● ● ●`. Newest tick is rightmost.

### State label

After the ring, print the current phase name in its color, uppercase: `CALM`, `RAMPING`, `EXPLODING`, `COOLING`.

## Integration Options

### Option A: Inline in breakdown pane

The breakdown pane (`breakdown.sh`) renders the bar chart and footer. The phase ring can be appended to the footer line:

```
  total: 82    types: 5    ● ● ● ● ● ● ● ● ● ● CALM
```

This requires the phase ring logic to live inside breakdown.sh (or a shared function in common.sh).

### Option B: Standalone widget

A separate tiny script (`phase-ring.sh`) that can be:
- Run in its own minimal pane (1-2 rows tall, full width)
- Called by `tmux set status-right` to embed in the status bar
- Sourced by other scripts that want to render it inline

### Option C: tmux status bar

```bash
# In cc-viz.sh launcher:
tmux set -t cc-viz status-right "#(bash $SCRIPT_DIR/phase-ring.sh --inline)"
```

The `--inline` flag makes it output a single line without cursor control, suitable for tmux status interpolation.

**Recommended:** Option B with both `--inline` mode (for tmux status / embedding) and standalone mode (for its own pane). Build it as a standalone script with an `--inline` flag.

---

## Implementation Constraints

1. **Buffered rendering** (only if running as a pane, not needed for `--inline` mode).
2. **Process substitution** for tail loop.
3. **Source cc-viz/common.sh** for thresholds and JSONL parsing.
4. **Bash 3.2 compatible.**
5. **Compact output** — this widget must fit in a single line when in inline mode. No box drawing, no multi-row layout.

### Computing metrics

Each tick, parse the JSONL line to get:
- `total` = `_count`
- `spawns` = count of procs with `age == 0`
- `deaths` and `net` via `compute_deltas` from common.sh

Then classify using the phase table above.

---

## Data Source

Tail `$CC_VIZ_DATA`.

## File

`cc-viz/phase-ring.sh` — executable, `#!/usr/bin/env bash`, sources `common.sh`. Supports `--inline` flag for single-line output mode.
