# Headline golden test (deferred from 2026-04-12 restack plan)

**Status:** backlog / deferred
**Seeded:** 2026-04-12
**Origin:** Task 7 of `docs/plans/2026-04-12-headline-restack.md` — deferred until the headline layout stabilizes enough that a frozen frame is worth more than it costs.

## Why this is deferred, not dropped

Goldens are anti-iteration. Locking one now while the headline is still moving around (ribbon polish, LCD tuning, stack tweaks) would either (a) break on every iteration and train reflexive regeneration — which defeats the purpose — or (b) subtly discourage visual iteration because regeneration is friction. Pick it back up when the layout feels locked and regressions start mattering more than changes.

## Ready-to-lock checklist

Before picking this up, confirm:

- [ ] Right-cluster column math is stable (LCD position, build/shell stack, sessions/agents stack aren't moving)
- [ ] Ghost ribbon absorption behavior is final (both partial-absorb with `needRibbonSep` and full-absorb paths)
- [ ] `SegmentReadout` has a known settled rest state at a deterministic tick count (see §Tick-sensitivity below)
- [ ] No planned changes to Classic palette, `messages.csv`, or `AnimFPS` in the near term

If any of those are still moving, defer again.

## Which frame(s) to freeze

`ViewLines` has three branching paths around the ghost ribbon (headline.go:361–394):

1. **Partial absorb** — `absorbWidth > 0 && needRibbonSep` — `activeWidth < sessionWidth` AND both `ghostWidth > 0` and `activeWidth > 0`. Hits the richest branching (ribbon separator logic + `rebuildBotRight` + ghost-right-anchor with partial extension).
2. **Full absorb** — `absorbWidth > 0 && !needRibbonSep` — `activeWidth == 0`, all ghosts extend under sessions. Primary target of 5ee4cc0.
3. **No absorb** — `absorbWidth == 0` — actives ≥ sessions, normal `rightBot.String()` path.

**Recommendation:** two goldens. `headline_partial_absorb.golden` for branch coverage (path 1), `headline_full_absorb.golden` to pin 5ee4cc0's fix (path 2). Path 3 is the simplest and lowest-risk; skip unless it breaks later.

## Proposed fixture (partial absorb)

```go
func captureHeadline() string {
    th := testTheme; ap := testAnim
    h := NewHeadline(th, ap)
    h.SetSize(120, 2)

    state := fixtureState()
    // 3 sessions → sessionWidth ≈ 5 (3 glyphs + 2 bg separators)
    state.Current.Sessions = []collector.SessionTree{
        {RootPID: 1000, RootComm: "claude", Descendants: []collector.ProcessInfo{{TypeCode:"N",Comm:"node"}}},
        {RootPID: 2000, RootComm: "claude", Descendants: []collector.ProcessInfo{{TypeCode:"N",Comm:"node"}, {TypeCode:"N",Comm:"node"}}},
        {RootPID: 3000, RootComm: "claude", Descendants: []collector.ProcessInfo{{TypeCode:"S",Comm:"bash"}}},
    }
    state.SmoothedCats["node"]  = 5  // runtime on top-row left
    state.SmoothedCats["go"]    = 2
    state.SmoothedCats["build"] = 3  // non-zero build/shell stack
    state.SmoothedCats["shell"] = 5

    h.Update(state)
    h.agents.SetTarget(5)      // 2 active + 3 stale
    h.agents.SetStaleCount(3)
    for i := 0; i < settledTickCount; i++ { h.AnimTick() }

    return h.View() + "\n"
}
```

For **full absorb**: same fixture but `SetTarget(3)` + `SetStaleCount(3)` → `activeWidth == 0`, all 3 ghosts extend under sessions.

## Invariants this locks in

A regression in any of these visibly breaks the golden:

- LCD pinned to the right edge (no trailing bg-padded runtimes after it)
- `build:NNN` on top row, `shell:NNN` on bot row, stacked at second-from-right
- Sessions-over-actives stack right-anchored, shared `iconBg`
- Runtime cells on top-row left, not bot
- Ghost ribbon continuity — partial absorb case exercises `needRibbonSep` between ghost trail and active glyphs
- 2-row equal visible width (subsumed by byte-exact golden match)

## Non-invariants — deliberately NOT tested

- Meltdown pulse (avoid it — `CPUPercent: 42.5` keeps threat non-meltdown so `pulseScale == 1.0` is tick-stable)
- Narrow-terminal sizing (Task 6 of the plan covers this via unit tests)
- Theme variations (other themes aren't part of `TestClassicMatchesGolden`)

## Tick-sensitivity prework

`SegmentReadout` has internal ghost trail + flash countdowns. 60 ticks is the convention in `captureBreatheDots`/`captureGauges`, but **verify** before adopting:

1. Instrument the readout to log when countdowns expire
2. Render every N ticks for N=0..120 and diff consecutive outputs; find the first tick T where output stops changing
3. Use `settledTickCount = T + small_buffer` so refactors that shave a tick or two don't invalidate

If the readout never settles (continuously animating even at rest), the golden is coupled to exact tick count and any AnimTick refactor breaks it. In that case, either (a) add a "freeze" mode to the readout for test rendering, or (b) accept the coupling and document it loudly.

## Fragility callouts

The golden becomes a tripwire for:

- **`internal/model/data/messages.csv` edits** — `state.StableQuip()` pulls from this. Correct behavior (quip text in-frame), but a foot-gun for contributors adding quips.
- **Classic palette edits** — any hex change in `theme/classic.go` invalidates. Correct, but means palette tuning requires regeneration.
- **`AnimFPS` or meltdown phase step** — our fixture avoids meltdown so this is mostly moot, but if anything drives pulse into the capture, this becomes load-bearing.
- **Any `renderCatCell` padding math change** — cell widths cascade into right-cluster column positions.

Call these out in a comment on `captureHeadline` so the regeneration path is obvious when they change.

## Implementation steps (when picking up)

1. Verify ready-to-lock checklist
2. Do the tick-sensitivity prework; pin `settledTickCount`
3. Add `captureHeadline` helper(s) to `thermal/internal/widgets/golden_test.go`
4. Register in `TestCaptureGoldenFiles` under `UPDATE_GOLDEN=1`
5. Register in `TestClassicMatchesGolden` table for byte-match assertion
6. Run `UPDATE_GOLDEN=1 go test ./internal/widgets/ -run TestCaptureGoldenFiles` to generate
7. Visually inspect the generated golden (`cat testdata/headline*.golden` and render it in a terminal) — don't ship a capture you haven't eyeballed
8. `/commit` with recipe describing the frame(s) chosen and why

## References

- Origin plan: `docs/plans/2026-04-12-headline-restack.md` Task 7
- Restack merge commit: `6412237`
- Ghost ribbon fix (the 5ee4cc0 behavior full-absorb golden would pin): `5ee4cc0`
- Existing capture pattern: `thermal/internal/widgets/golden_test.go` — `captureBreatheDots`, `captureGauges`, `captureRatesLine`
- Headline under test: `thermal/internal/widgets/headline.go` — especially `ViewLines` 245–400
