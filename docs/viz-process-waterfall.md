# Process Lifetime Waterfall

**Replaces:** Nothing (net-new visualization).

**Unique signal:** Individual process lifetimes. Every other pane deals in aggregates — total counts, rates, type breakdowns. This is the only view that tracks *individual* processes from birth to death. The visual shape answers a question nothing else can: "are these long-lived workers or rapid churn?" Many short bars = churn. Few long bars = stable workers. A mix = agents spawning tools that do real work.

---

## Layout

**Position:** Bottom-left quadrant of the 2x2 tmux grid (replaces breakdown's old slot; breakdown moves to the full-span bottom row).

**Structure:**

```
┌─ WATERFALL ── 14 alive ─────────────────┐
│ N ████████████████████████████████░░░░░░ │  <- 32s old, dimming
│ G ██████████████████████░░░░░░░░░░░░░░░ │  <- 21s old
│ V ████████████████░░░░░░░░░░░░░░░░░░░░░ │  <- 16s old
│ N █████████████░░░░░░░░░░░░░░░░░░░░░░░░ │  <- 13s old
│ G ██████████░░░░░░░░░░░░░░░░░░░░░░░░░░░ │  <- 10s old
│ N ███████░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░ │  <- 7s old
│ R █████░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░ │  <- 5s old
│ X ███░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░ │  <- 3s old
│ N █░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░ │  <- new spawn (bright)
│                                          │
│   (3 more above)                         │  <- overflow indicator
└──────────────────────────────────────────┘
```

## Rendering Rules

### Swim lanes

Each row represents one currently alive process. Rows are sorted by age, oldest at top, newest at bottom.

### Bar rendering

- The bar extends from the left edge, with length proportional to the process's age.
- Scale: the oldest process in the current snapshot sets 100% width. All other bars are proportional to it.
- **Filled portion** (`█`): colored by the process type (same colors as common.sh).
- **Empty portion** (`░`): dim gray fill to the right edge.

### Age-based color intensity

The bar's color shifts from bright to dim as the process ages:

| Age | Intensity | Effect |
|-----|-----------|--------|
| 0-2s | Bright + bold | `\033[1m` + type color — new spawn, visually pops |
| 3-10s | Normal | Standard type color |
| 11-30s | Dim | `\033[2m` + type color |
| 30s+ | Very dim | `\033[2m` + gray — been around forever, not interesting |

This creates a natural gradient: the bottom of the waterfall glows bright (new spawns), the top fades (old workers). Your eye is drawn to the action.

### Row label

Each row starts with a 4-char label: 2-space indent + type letter + space. The type letter uses the type's color from common.sh.

### Process lifecycle

- **Spawn:** Process appears in the JSONL snapshot with `age == 0`. A new row is inserted at the bottom of the waterfall.
- **Alive:** Process continues appearing in subsequent snapshots. Its bar grows rightward each tick (age increases).
- **Death:** Process disappears from the snapshot. Its row is removed immediately — no fade-out, no ghost. The gap closes.

### Overflow

If there are more alive processes than rows available (pane height minus 2 for header + border):
- Show the newest N processes (most recent spawns at the bottom).
- Display an overflow indicator at the top: `(K more above)` in dim text.
- This prioritizes showing new activity over old stable workers.

### Header

Single-line header inside the box: `WATERFALL ── N alive` where N is the current total process count, colored by threshold (white/yellow/red using `threshold_color` from common.sh).

---

## Implementation Constraints

1. **Buffered rendering.** `frame=$(render)`, `tput home`, `printf '%s' "$frame"`, `tput ed`.
2. **Cache tput cols/lines before subshell.**
3. **Process substitution.** `while read ... done < <(tail -n 0 -f ...)`.
4. **Source cc-viz/common.sh.**
5. **Bash 3.2 compatible.**
6. **No state across ticks needed** — each frame renders purely from the current snapshot's `procs` array (pid, type, age). No ring buffer, no history tracking. The JSONL data already contains everything (each proc has its `age` field).

### Parsing

Use `eval "$(parse_jsonl_line "$line")"` to get `_proc_pids`, `_proc_types`, `_proc_ages`, `_count`. Sort by age descending (oldest first) for display ordering.

---

## Data Source

Tail `$CC_VIZ_DATA`. Each JSONL line has `procs` array with `pid`, `type`, `age` per process.

## File

`cc-viz/waterfall.sh` — executable, `#!/usr/bin/env bash`, sources `common.sh`.
