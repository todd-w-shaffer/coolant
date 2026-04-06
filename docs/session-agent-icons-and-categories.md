# Session arc: agent icons, session row, and process-based categories

**Date:** 2026-04-05

## What we built

A single session that started with "add breathing agent icons to the headline bar" and ended with a complete replacement of the dashboard's category system. 14 commits across the thermal dashboard.

## The arc

### Breathing agent icons

The first feature: teal diamond icons (◆) in the headline bar, one per active subagent. Each icon breathes independently via sine-wave brightness oscillation driven by harmonica springs. Icons fade in on agent spawn and fade out on death. The icon count tracks `AgentCount` from JSONL `agent.start`/`agent.stop` events, not process tree session count.

This required extracting `BreatheDots` as a shared animation type — spring-driven fade-in/out with sine-wave breathing, reusable by any widget that needs animated dot indicators.

### The session row

Below the stats line, a hierarchical view of what's running:

```
⊙ Chrome  ⌬ ●●●◆◆·· [08]  +1
```

Each Code session (⌬) shows its descendant processes as category-typed glyphs, with a fixed-width count. Idle sessions collapse to a dim `+N` trailer. Desktop (⊞) and Chrome bridge (⊙) are detected separately from CLI sessions — the collector filters out Electron helper processes that were inflating the session count from 2 to 11.

### The ghost Claude investigation

A user screenshot showed 10 dim diamonds where only 2 CLI sessions existed. Investigation revealed the collector's `strings.Contains(name, "claude")` was matching Claude Desktop's Electron process tree (Claude Helper, Claude Helper (Renderer), chrome-native-host, etc.). Fix: match CLI binaries via `HasPrefix` on basename, skip anything with `Claude.app` in the path. The chrome-native-host (browser extension bridge) persists after Desktop closes, so it gets its own detection flag.

### Process-based categories

The original 5 activity categories (test, build, run, search, shell) were fiction. "Test" never lit up because vitest shows as `node` in `ps`. "Search" barely fired because grep is too ephemeral. The categories described what Claude was *doing* but `ps` only reports what *binary* is running.

Replaced with a process-based system:
- **Fixed** (always visible): `build`, `shell` — with counts, right-anchored
- **Dynamic** (appear when active): `node`, `go`, `python`, `rust` — label only, no count, pop in to the left

Dynamic runtimes warm at threshold 1 (any presence triggers amber) with a slow EMA decay (~6s linger) so they stay visible long enough to register. Fixed categories stay cold at low counts since a few shell processes is normal. The thermal gradient was shifted warmer at level 1 (dim amber instead of invisible gray) to make the "pop-in" moment noticeable.

### The live-fire sequence

Three screenshots captured the full escalation during a parallel agent run:

1. **Three agent diamonds breathing** — calm, agents just spawned
2. **`node` flashes amber** — JS runtime spinning up, build/shell still quiet
3. **Red wall** — `build:003` and `shell:083` both critical, CPU 71%, MEM 69%, compressor at 8K

The dynamic runtime popping first acts as an early warning before the fixed categories escalate. The whole story plays out in a tmux strip without opening Activity Monitor.

### Sparkline resize fix

A bug surfaced mid-session: changing font size caused a panic (`slice bounds out of range [:471] with capacity 401`). The `SparkBufs` interpolation buffers were allocated once at the initial terminal width and never grew. Fix: `SparkBufs.grow()` method with a `lastWidth` cache, called from `prepareSparkDataBuf`/`prepareSparkMaskBuf`. Two tests reproduce the exact panic scenario.

## Design decisions

- **Agent icons use ◆, sessions use ⌬** — diamonds are for active agents (breathing, in the headline), deltas are for session markers (in the session row). Different glyphs for different concepts.
- **Runtimes show label only, no count** — the signal is binary (present or not). The thermal color conveys intensity. Counts are noise for dynamic boxes.
- **Per-category EMA decay** — `smoothMapPerKey` uses `FixedCategories` to select alpha. Fixed categories at 0.15 (snappy), runtimes at 0.05 (linger ~6s). This lets runtimes fade slowly enough to read while build/shell respond quickly to actual changes.
- **Fixed categories right-anchored** — build and shell never move when dynamic runtimes appear/disappear. The quip cell absorbs width changes. Nothing jumps.
- **TDD enforced for all functional code** — CLAUDE.md updated to require red-green-refactor for bash, Go, or anything else. No exceptions. This was prompted by the BreatheDots type being written without tests.

## Commits (14)

1. Add breathing agent icons to headline bar
2. Hierarchical session row, agent tracking, and sparkline resize fix
3. Collapse idle sessions to a dim +N trailer
4. Exclude Claude Desktop from session count, add Desktop indicator
5. Split Desktop and Chrome bridge into separate indicators
6. Reorder session row: static presence left, dynamic Code right
7. Replace activity categories with process-based dynamic system
8. Warm runtime thresholds and amber gradient for visible pop-in
9. Right-anchor fixed categories, dynamic runtimes pop in leftward
10. Slow EMA decay so dynamic runtime boxes linger visibly
11. Keep fixed categories cold at low counts, amber only for runtimes
12. Strip counts from runtime boxes and add slow decay for lingering
13-14. Tuning commits (gradient, threshold, decay adjustments)
