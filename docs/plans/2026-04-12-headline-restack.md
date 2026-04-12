# Headline Restack: Right-Aligned 2-Row Cluster

**Date:** 2026-04-12
**Worktree:** `.claude/worktrees/segment-readout`
**Owner file:** `thermal/internal/widgets/headline.go`

## Goal

Rearrange the double-height headline from a "top row does everything, bottom row is LCD tail + bg fill" shape into a horizontally-composed 2-row cluster:

```
┌─────────────┬─────────────┬───────────────┬────────┐
│             │ sessions ⌬  │ build:NNN     │        │
│  runtimes   │             │               │ LCD    │
│ (dynamic)   │ agents ⬡    │ shell:NNN     │ 048°   │
└─────────────┴─────────────┴───────────────┴────────┘
```

- **Leftmost:** dynamic runtime cells (grow/shrink into empty space)
- **Middle-left stack:** sessions (top) / agents (bottom), pure iconography
- **Middle-right stack:** `build:NNN` (top) / `shell:NNN` (bottom)
- **Far right:** LCD segment readout (2-row)
- **Quip:** lives in the leftmost region alongside/above runtimes (decision below)
- **Offline:** collapses to 1-row (quip only)

## Architecture

### Output contract (unchanged)
`Headline.ViewLines() []string` must still return exactly 2 lines of equal visible width when online (even with no LCD), and 2 lines offline (bottom row bg-filled) per `TestHeadline_ViewLinesAlwaysTwoRows`. This no-reflow contract is enforced by `horizontal.go` which just appends `ViewLines()...`.

### Current shape
Today `buildOverallCell` is a single monolithic function that renders the "overall cell" (quip + LCD + agents + sessions) as a single chunk of width `overallWidth`, then concatenates `dynamicCells` and `fixedCells` flat after it. The LCD's bottom row is painted on the second line via `tempBot`, everything else on the second line is bg-fill.

### Target shape
Compose four stack fragments independently; each fragment returns `(top string, bot string, visWidth int)`. Then `ViewLines` joins them with a single-space bg divider and stacks the `top`s into line 0, `bot`s into line 1.

```
frag = (top, bot, visWidth)

leftFrag  := h.renderLeftFrag(quip, dynamic, fg, iconBg, leftWidth)
midLFrag  := h.renderSessionsAgentsStack(sessions, agents, iconBg)
midRFrag  := h.renderBuildShellStack(fixed, h.state.SmoothedCats)
rightFrag := h.renderLCDFrag(iconBg, pulseScale)   // "" when !Online

top := leftFrag.top + bgSpace + midLFrag.top + bgSpace + midRFrag.top + bgSpace + rightFrag.top
bot := leftFrag.bot + bgSpace + midLFrag.bot + bgSpace + midRFrag.bot + bgSpace + rightFrag.bot
```

Width math runs right-to-left: compute each right-cluster fragment's own width first, subtract from `h.width`, hand remainder to `leftFrag` which owns quip truncation + bg pad. This way runtimes appearing/disappearing only ever changes the quip-pad portion.

### New helpers (proposed signatures, all in `headline.go`)

```go
// rowPair is a 2-row rendered fragment at a fixed visible width.
// visWidth counts cells (post-ANSI), so layers can pad/align.
type rowPair struct {
    top, bot string
    visWidth int
}

// renderSessionsAgentsStack returns a 2-row cell with session diamonds on the
// top row and agent hex glyphs on the bottom. Both rows share iconBg so the
// cell reads as one rectangle. Empty sessions or agents still emit a bg-filled
// row of the same width so the rectangle never collapses asymmetrically.
func (h *Headline) renderSessionsAgentsStack(
    sessions []collector.SessionTree,
    agentsStr string, agentsWidth int,
    iconBg color.Color,
) rowPair

// renderBuildShellStack returns "build:NNN" / "shell:NNN" stacked, each
// thermal-colored per renderCatCell rules, at a consistent cell width
// (fixedCellWidth). If renderCatCell's per-cell bg differs between build
// and shell, the stack's width is still the max of the two and trailing
// padding on the shorter row uses the row's own bg.
func (h *Headline) renderBuildShellStack(
    smoothed map[string]float64,
) rowPair

// renderLCDFrag wraps SegmentReadout.RenderWithPulse. Returns zero-visWidth
// fragment offline or when readout is hidden.
func (h *Headline) renderLCDFrag(iconBg color.Color, pulseScale float64) rowPair

// renderLeftFrag renders dynamic runtime cells plus quip, filling leftWidth.
// Runtimes on top row (or bottom — decision below); quip fills remaining
// space on whichever row it's assigned to; the other row is bg-filled.
func (h *Headline) renderLeftFrag(
    quip string, fg, bg color.Color,
    dynamic []collector.Category, smoothed map[string]float64,
    leftWidth int,
) rowPair
```

### Quip decision — flagged

Two viable placements:

- **(A) Quip above runtimes on the top row.** Runtime cells are bottom-row, quip fills top-row of the left region. Matches the "sessions above agents" vertical rhythm. Risk: runtime cells are thermal-colored (per `renderCatCell`) and sit adjacent to the plain-bg quip zone — bg seam will be visible.
- **(B) Quip fills both rows of a bg-padded zone; runtimes sit to the right of quip, both rows tall (top row just the label centered, bottom row bg).** Keeps runtimes visually "cells" but wastes half the runtime cell height.

**Recommendation: (A).** Runtimes already have their own per-cell bg; the bg seam between quip zone and runtimes already exists today between the overall cell and the first runtime cell, so this is a wash. Top-row quip preserves today's reading order. Engineer should visually confirm in `--demo` and swap to (B) if the bg seam bothers.

### Offline collapse

`ViewLines` top-of-function check: when `!state.Online`, short-circuit to a 1-row-of-content implementation that matches today's offline top line (quip with offline bg), and return a 2-line slice `[top, bgFill]` to honor the no-reflow contract. No stacks, no LCD, no runtimes (runtimes are empty offline anyway since there's no data).

## Task Breakdown (TDD, one feature per cycle)

Each task: write failing test → minimal impl → `go test ./...` → `go build -o ../bin/thermo ./cmd/thermal/` from `thermal/` AND `go build -o /Users/toddwshaffer/Desktop/apps/coolant/bin/thermo ./cmd/thermal/` → eyeball `./bin/thermo --demo` → invoke `/commit` skill.

---

### Task 1 — Introduce `rowPair` + refactor LCD rendering into `renderLCDFrag`

Pure refactor, no visible change. Isolates the LCD so later tasks can move it to the far right without touching readout code.

- **Test:** `TestRenderLCDFrag_OfflineZeroWidth` — `h.state.Online = false`, call `renderLCDFrag(...)` — expect `visWidth == 0`, `top == "" && bot == ""`.
- **Test:** `TestRenderLCDFrag_OnlineTwoRowsEqualWidth` — online fixture, `ansi.StringWidth(top) == ansi.StringWidth(bot) == visWidth`.
- **Impl:** Extract `tempTop/tempBot/tempVisWidth` assembly into `renderLCDFrag`.
- **Verify:** Existing `TestHeadline_ViewLinesTwoRowsWhenOnline` still green. No visible diff in demo.
- **Commit:** "refactor: extract LCD rendering into rowPair fragment".

### Task 2 — `renderSessionsAgentsStack` (sessions on top of agents)

Build the stack helper but DON'T wire it into `ViewLines` yet. Keep today's side-by-side rendering in `buildOverallCell`.

- **Test:** `TestRenderSessionsAgentsStack_WidthMaxOfRows` — construct with 3 sessions and 2 agents, assert `visWidth == max(sessionVisWidth, agentVisWidth)` and both top/bot have that ansi width.
- **Test:** `TestRenderSessionsAgentsStack_EmptySessionsStillFillsRow` — zero sessions, 3 agents — top row must be bg-filled to agent width, not empty.
- **Test:** `TestRenderSessionsAgentsStack_SharedBg` — decode bg ansi on both rows, must match so cell reads as one rectangle.
- **Impl:** Call `renderSessionDiamonds` + `h.agents.Render(...)`, pad shorter row with `lipgloss.Style.Background(iconBg).Render(strings.Repeat(" ", diff))`.
- **Verify:** Only unit tests; demo unchanged.
- **Commit:** "feat(headline): add sessions/agents stack fragment".

### Task 3 — `renderBuildShellStack` (build on top of shell)

- **Test:** `TestRenderBuildShellStack_BothRowsFixedWidth` — `visWidth == fixedCellWidth`, both rows equal width.
- **Test:** `TestRenderBuildShellStack_TopIsBuildBotIsShell` — `ansi.Strip(top)` contains `build:`, `ansi.Strip(bot)` contains `shell:`.
- **Test:** `TestRenderBuildShellStack_ThermalColorIndependent` — set smoothed build count to hot-threshold and shell count to zero; assert top row and bot row bg escape sequences differ (each row keeps its own thermal color — they do NOT need to share a bg since each is a discrete cell).
- **Impl:** Call existing `renderCatCell(buildCat, smoothed, fixedCellWidth, th)` for top, same for shell on bot.
- **Verify:** Unit only.
- **Commit:** "feat(headline): add build/shell stack fragment".

### Task 4 — Wire all four fragments into `ViewLines`, restructure layout

The big step. Replaces the body of `ViewLines`/`buildOverallCell`.

- **Test:** `TestHeadline_LCDOnFarRight` — online fixture, 120 wide; compute LCD visWidth; assert `ansi.Strip(lines[0])` ends with the LCD's top-row content (no trailing bg-padded runtimes).
- **Test:** `TestHeadline_SessionsAboveAgents` — online fixture; strip ansi; look at a fixed column range where sessions/agents cluster sits; top row contains `⌬` and bot row contains a hex glyph rune.
- **Test:** `TestHeadline_BuildAboveShell` — assert `build:` on top row and `shell:` on bot row at the same approximate column.
- **Test:** existing `TestHeadline_ViewLinesTwoRowsWhenOnline`, `TestHeadline_ViewLinesAlwaysTwoRows`, `TestHeadline_TwoRowPreservesTopContent` still green. Note: `TwoRowPreservesTopContent` asserts `build` AND `shell` on the TOP row — **this test must be updated** since `shell` moves to the bottom row. Adjust to assert `build` on top and `shell` on bot.
- **Test:** `TestHeadline_RuntimeCellsAppearOnLeft` — put `node` in smoothed, assert its cell position precedes the sessions cluster.
- **Impl:** Rewrite `ViewLines`:
  1. Compute right-cluster widths (LCD + mid-right + mid-left + 3 dividers).
  2. Hand remainder to `renderLeftFrag`.
  3. Glue with `bgStyle.Render(" ")` between fragments.
  4. Offline path: short-circuit to 1-row-of-content + bg-fill bot row.
- **Verify:** `./bin/thermo --demo` — confirm stacks render and LCD is far right. Confirm stale-agent KITT still sweeps. Confirm meltdown pulse still modulates (existing `TestHeadline_MeltdownPulseDrivesModulation`).
- **Commit:** "feat(headline): restack into right-aligned 2-row cluster".

### Task 5 — Offline collapse polish

After Task 4 offline is "works but ugly". Tighten.

- **Test:** `TestHeadline_OfflineNoLCDNoStacks` — set Online=false, strip both lines, assert no `build:` / `shell:` / LCD glyphs on either row; top contains offline quip.
- **Impl:** Confirm the short-circuit already omits everything; adjust if stacks leaked through.
- **Verify:** Demo in offline cycle; confirm single line of content, no seam flicker.
- **Commit:** "feat(headline): collapse offline mode to quip-only row".

### Task 6 — Narrow-terminal defensive sizing

- **Test:** `TestHeadline_NarrowTerminalDoesNotPanic` — `SetSize(40, 2)`, online fixture, render, assert no panic and width >= 0.
- **Test:** `TestHeadline_NarrowTerminalDropsRuntimesFirst` — at 40 cols with node+go visible, assert LCD and stacks are still rendered (right-anchor wins) and runtimes are elided if needed.
- **Impl:** Clamp `leftWidth` to 0; when 0, skip `renderLeftFrag` entirely.
- **Verify:** `COLUMNS=40 ./bin/thermo --demo` (or resize manually).
- **Commit:** "feat(headline): drop runtimes first on narrow terminals".

### Task 7 — Golden test (deferred to backlog)

**Deferred 2026-04-12** until the headline layout stabilizes enough that a frozen frame is worth more than the iteration friction a golden creates. Enriched analysis, fixture proposal, tick-sensitivity prework, and ready-to-lock checklist live in `docs/backlog/headline-golden.md`. Pick it up from there.

## Edge Cases (flag for implementing engineer)

1. **Offline collapse** — Today offline still emits 2 lines with bot bg-fill. Must preserve (layout depends on constant height).
2. **Empty sessions** — stack's top row must be bg-filled to agent width, not empty string, else the cell becomes L-shaped.
3. **Empty agents** — symmetric: stack's bot row must be bg-filled to session width.
4. **Empty sessions AND agents** — stack collapses to visWidth=0; omit the whole fragment + its divider in `ViewLines`.
5. **Terminal width too small** — if right-cluster width alone exceeds `h.width`, drop runtimes first, then quip, then sessions/agents stack (keep LCD + build/shell as the most-informative pair). Simplest defensive cap: if `leftWidth < 0`, set to 0 and skip `renderLeftFrag`.
6. **Quip truncation** with LCD on far right — `maxQuip = leftWidth - 2` (room for a leading space + trailing space before the stack divider). Rune-count truncation via `truncRunes`, not byte slicing.
7. **Golden test invalidation** — No existing headline golden, but `thermal_levels.golden` still valid (uses `renderCatCell` directly, untouched). If Task 7 is done, regenerate.
8. **`horizontal.go`** — does `h.headline.SetSize(w, 2)` and `ViewLines()...`; unchanged. Do NOT adjust.
9. **Demo fixture** — `thermal/internal/demo/demov2.go` drives via snapshots; as long as `Headline.Update` contract holds, demo Just Works — but eyeballing is still required (CLAUDE.md).
10. **`TestHeadline_TwoRowPreservesTopContent`** — WILL fail because it asserts `shell` on top row. This is expected; fix the test in Task 4 to assert `build` on top and `shell` on bot (structural contract changed intentionally).

## Gotchas

- **lipgloss v2 bg fills** — `lipgloss.NewStyle().Background(bg).Render(strings.Repeat(" ", n))` is the pattern; don't fall back to raw spaces or the bg seam shows. When composing two pre-styled fragments, inserting a bg divider requires the divider's bg to match both sides — use `iconBg` consistently across every 2-row stacked cell.
- **Rune-counted truncation** — `utf8.RuneCountInString` / `truncRunes` (already in `headline.go`). Offline messages contain em-dashes (3 bytes, 1 cell). Do not use `len(quip)` for pad math.
- **Consistent `iconBg` across stack rows** — sessions and agents must share bg so the cell reads as one rectangle. `renderCatCell`'s thermal bg is independent per cell; build vs shell having different thermal bgs is fine because each row is its own styled cell, but the divider spaces between stacks must use `iconBg` (the overall thermal bg), not any cell's bg.
- **Visible-width vs byte-length** — `ansi.StringWidth` (from `github.com/charmbracelet/x/ansi`) is the authority; the `visWidth int` in `rowPair` must match. BreatheDots glyphs `⬡⏣⬢` and session diamond `⌬` are all 1 cell each — no surprise double-width.
- **SegmentReadout's precomputed fg ANSI** — `paintFg` in `segmentreadout.go` injects raw escape sequences; never re-wrap the LCD output in a style that calls `Foreground()`, or the inner escape will survive inside lipgloss's output but the outer one will override fg. Treat LCD fragment as opaque — only compose bg-padding adjacent to it, never nested around it.
- **Meltdown pulse** — phase lives at the Headline level (`h.pulsePhase`); don't duplicate oscillators in the new stack helpers. Pass `pulseScale` only to the LCD fragment.
- **`Headline.Update` runs LCD update only when Online** — preserve this; moving the LCD to far right doesn't change when it's updated.
- **Commits** — use `/commit` skill (not raw `git commit`). Each task ends with a `/commit` invocation; the skill generates the Recipe + Changes body.
- **Build paths** — `cd thermal && go build -o ../bin/thermo ./cmd/thermal/` AND `go build -o /Users/toddwshaffer/Desktop/apps/coolant/bin/thermo ./cmd/thermal/` so live-demo stays in sync (per CLAUDE.md).
