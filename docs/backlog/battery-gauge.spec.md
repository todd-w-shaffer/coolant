# Spec: battery gauge on the headline strip

**Status:** shipped
**Seeded:** 2026-04-21

## Context

The thermal dashboard already reads like a cockpit — CPU/MEM sparklines,
process categories, agent dots, the LCD temperature readout. What it
does *not* tell you is a signal every laptop-on-battery user needs while
in flow state: "how long until this machine dies?" In a full-screen
terminal with the dashboard as a bottom strip, the macOS menu bar is
occluded, so battery state is invisible unless the user Mission-Controls
out of their workspace. The dashboard is the natural place to surface it.

The payoff: a glance at the strip answers *"bruh, time to close the
laptop and walk home from the coffee shop?"* without breaking focus. The
cell uses the existing breathing animation vocabulary for "charging in
progress" so it reads as part of the dashboard's motion language rather
than a bolted-on widget.

## Scope

**In:**
- `pmset -g batt` sampled in the existing SlowCollector 1s loop.
- Parser returns: percent (0–100), state enum, time-remaining (duration,
  zero if unknown).
- New `Battery` widget — 2-row cell, rightmost on the headline.
- Charging state breathes using the same sine phase source as BreatheDots.
- Always-visible cell regardless of power source.
- Severity-colored fill-bar glyph on battery; solid lightning bolt on AC.

**Out (explicit non-goals):**
- Historical battery plotting / sparkline. The headline cell is
  instantaneous only.
- Low-battery sound or system alerts. Visual only.
- Alert log entries ("battery low!") — the severity color *is* the alert.
- Multi-battery systems (external packs). macOS laptops have one
  internal battery; the spec assumes one.
- Windows / Linux power sources. macOS-only, matching the rest of
  Coolant's system-stats approach.
- Persisting battery state between runs. Ephemeral.
- **Statusline integration.** The braille statusline
  (`claude-statusline/`) is deliberately not in scope — ever. Thermo
  is the system-wide dashboard; statusline is per-session surface.
  Battery belongs on the system dashboard, not per-session chrome.

## Resolved open questions

### Q1 — Where does battery data come from?

**`pmset -g batt`.** Evidence: native macOS, no dependencies, same
subprocess cost as `ioreg` for GPU already used in
`collector/system.go:113`. Output format verified on the target machine:

```
Now drawing from 'Battery Power'
 -InternalBattery-0 (id=21692515)	47%; discharging; 2:57 remaining present: true
```

Other observed formats (documented in `man pmset` and confirmed across
Apple Silicon machines):

- `Now drawing from 'AC Power'` (plugged in)
- `100%; charged; 0:00 remaining present: true` (full, plugged)
- `85%; charging; 0:42 remaining present: true` (charging)
- `85%; AC attached; not charging; 0:00 remaining present: true`
  (optimized-charging hold at 80%, cold battery, or 100%)
- `47%; discharging; (no estimate)` (first ~60s after unplug, battery
  calculating its time-remaining estimate)
- `85%; finishing charge; 0:02 remaining present: true` (near 100%)

Parser handles all six. `time-remaining` is parsed as `H:MM`, mapped to
`time.Duration`, and reports `0` when `(no estimate)` or absent.

### Q2 — Where does the subprocess run?

**Slow loop, alongside swap/vm_stat/GPU.** Evidence:
`collector/system.go:95-140` already fans out three goroutines at 1s
cadence via the shared `ch chan result` pattern. Adding a fourth goroutine
for `pmset` follows the existing pattern exactly. Fast loop (150ms) would
be wasteful — battery percent drifts on the order of minutes.

### Q3 — How does `BatteryStats` live on `SystemStats`?

**Inline fields, not a nested struct.** Evidence: all existing slow-loop
fields (`SwapUsedBytes`, `GPUPercent`, etc.) are inlined directly on
`SystemStats` (`collector/types.go:6-16`). Keep the pattern. Four new
fields:

```go
BatteryPresent       bool          // false on desktops / Mac Studio
BatteryPercent       float64       // 0-100
BatteryState         BatteryState  // enum: Unknown, Discharging, Charging, Charged, ACNotCharging
BatteryTimeRemaining time.Duration // 0 when (no estimate) or AC-stable
```

`BatteryState` is a new int-typed enum in `types.go`. Stringer methods
not needed — widget code switches on the enum directly.

### Q4 — How is "no battery hardware" handled?

**`BatteryPresent = false` hides the cell entirely.** No fallback text,
no "AC" badge. Evidence: dynamic runtime cells already use the
appear/disappear model (`headline.go:383-388`), but fixed cells
(build/shell) are always on. Battery is semantically optional hardware,
so appear/disappear is correct. On desktop Macs the cell's width
collapses to zero and the rest of the strip occupies the freed space
naturally through the existing `appendFrag` + `rightVis` layout math.

### Q5 — What's the cell's visual design?

**Two rows, 7 cells wide, right-anchored content. The fuel-level
indicator is a single-column braille gauge — one cell wide, both rows
tall — reusing the existing sparkline braille primitives:**

```
 ⠛ 87%      ← top braille char (level 7-of-8, top half) + "87%"
 ⣿ 2h14m    ← bottom braille char (full) + time-remaining
```

Braille gives 8 vertical dots per column across two stacked characters,
exactly matching the ~12.5% granularity we want. Reusing
`widgets/sparkline.go:47-76` the mapping is:

```go
// percent → 0-8 braille level
level := int(pct * 8.0 / 100.0 + 0.5)
if level > 8 { level = 8 }
bottom, top := levelSplit(level)      // existing helper
// fill BOTH columns (left + right) so the bar looks solid, not a single thread
topChar    := 0x2800 | leftBits[top]    | rightBits[top]
bottomChar := 0x2800 | leftBits[bottom] | rightBits[bottom]
```

This reads as a real analog fuel gauge rather than a one-off block
glyph, and it speaks the dashboard's existing braille vocabulary
(sparkline, statusline, brailletext). No new font or bitmap — the
battery widget is in the same `widgets` package as `sparkline.go` so
`levelSplit` / `leftBits` / `rightBits` are directly accessible.

On AC (charging):
```
 ⚡ 85%
   ~42m
```

On AC (stable — charged, finishing, or not-charging):
```
 ⚡
   —
```

When charging, the braille column is *replaced* by the ⚡ bolt on the
top row (bot row left cell becomes iconBg-padded space) — the gauge
metaphor is less useful when electrons are incoming than the state
indicator. (Follow-up idea noted in observations: during `Charging`,
animate the braille dots themselves filling bottom-to-top in sync with
the breath phase. Out of scope for v1; revisit if the simple bolt
reads flat.)

Severity color applies to both the braille gauge and the percent
digits; the ⚡ uses `theme.AccentR/G/B * breath`:

- `>50%` → `theme.OverallGradient[1].Fg` (calm / green)
- `20–50%` → `theme.OverallGradient[2].Fg` (warm / amber)
- `<20%` → `theme.OverallGradient[3].Fg` (hot / red)

Re-use of `OverallGradient` (not a new battery LUT) keeps the strip on
the same color language; "red" means the same thing everywhere.

### Q6 — How does charging breathe?

**Brightness modulated by `sinNorm(b.phase)` on the ⚡ glyph, where
`b.phase` advances by `anim.BreathePhaseStep` per AnimTick.** Evidence:
`widgets/breathedots.go:76-92` already owns the canonical breathing
phase source. The battery widget owns its own phase accumulator (not
shared with BreatheDots' per-dot phases) so the breath reads as a single
steady pulse on one glyph rather than mimicking the multi-dot tidal
stagger.

Charging breath: brightness ∈ `[0.4, 1.0]` (floor matches
`TidalBrightFloor`). AC-stable (charged, not-charging, or full):
constant brightness 1.0, no phase advance, no motion.

### Q7 — AC attached but not charging — solid or breath?

**Solid.** Evidence: confirmed with user — breath should mean "actively
pulling electrons." `charging` and `finishing charge` breathe;
`charged`, `AC attached; not charging`, and an unknown-but-plugged
state render solid. The enum branch is the single source of truth for
breath vs solid.

### Q8 — Time-remaining format on the bottom row

**`Xh YYm` when ≥ 1h, `YYm` when < 1h, `—` when unknown or AC-stable.**
`—` (em-dash) reads as "no data" unambiguously and matches the "nothing
to show here" pattern used by empty-state cells elsewhere in the strip.
`(no estimate)` within the first 60s after unplug collapses to `—` until
pmset provides a number.

### Q9a — Theme-specific severity colors?

**Yes, by construction.** `theme.OverallGradient` is a `[5]ThermalLevel`
array (0-indexed: cold/calm/warm/hot/crit). The battery widget uses
indices `1/2/3` for green/amber/red buckets — the same pattern every
other heat-aware widget uses (LCD, heat bloom, gauge peaks). Each
theme populates all five slots (asserted by `theme_test.go`).

**Caveat — monochrome themes:** `theme/mono.go`'s `OverallGradient` is
brightness-only (amber at three levels, no hue separation). On mono,
the battery cell will read as three brightness bands of the same
color, which is correct behavior for that theme — the color *language*
is preserved, only the palette's expressive range differs. No special
handling; it emerges from the existing indirection.

### Q9b — Should battery contribute to OverallTemperature?

**No — battery is runway, not thermal stress.** Feeding a 5% battery
into the thermal score would mean a cool-CPU system reports meltdown,
which breaks the mental model that "heat = work the machine is doing."
Battery and thermal are orthogonal axes; the cell's color carries the
severity locally.

**However**: at critical (<10%) the cell should throb using the same
`pulseScale` math the LCD meltdown pulse uses
(`headline.go:95-104`). This pulls attention to the runway signal
without polluting the heat metric. New threshold constant:

```go
const BatteryMeltdownPct = 10.0 // below this, cell pulses locally
```

The pulse is cell-local: when `BatteryPercent < BatteryMeltdownPct`
*and* the battery is discharging (not charging-at-crit), the battery
widget advances its own meltdown phase each AnimTick and multiplies
both the braille gauge and percent-text foregrounds by
`0.6 + 0.4*(sin(phase)+1)/2` — same envelope as the LCD. Independent
of the headline-wide meltdown phase so they don't fight each other on
a CPU-meltdown + battery-crit simultaneous state (they'd both be
pulsing, but at their own rhythms, which reads as "two urgent
problems" rather than one synchronized blink).

### Q9c — Where does the cell sit on the strip?

**Rightmost, after LCD.** The existing `appendFrag` sequence in
`headline.go:413-415` is:

```go
appendFrag(sessAgents)
appendFrag(buildShell)
appendFrag(lcd)
```

Append `battery` last so the strip reads left-to-right:

```
[quip + runtimes]  [sessions/agents] [build/shell] [LCD °] [battery]
```

This pushes the existing cluster left, satisfying the user's visual
request. `leftCombined = h.width - rightVis - headlineRightMargin`
already subtracts the right-cluster width from available left space, so
the runtime/quip zone auto-shrinks.

## Implementation plan

### 1. Collector (Go)

**New file `thermal/internal/collector/battery.go`:**

```go
type BatteryState int

const (
    BatteryUnknown BatteryState = iota
    BatteryDischarging
    BatteryCharging
    BatteryCharged        // 100%, plugged
    BatteryACNotCharging  // optimized-hold, cold, etc.
    BatteryFinishing      // "finishing charge"
)

// parseBattery parses `pmset -g batt` output into the battery fields
// on SystemStats. Handles all six observed formats; sets
// BatteryPresent=false when no battery line is found (desktop Mac).
func parseBattery(pmset string, stats *SystemStats) { ... }
```

The parser is pure (string in, mutates stats). Tested in isolation with
the six canonical format strings plus malformed inputs.

**Change to `collector/types.go`:** four new fields on `SystemStats`
(Q3), plus the `BatteryState` enum (exported).

**Change to `collector/system.go`:** extend `CollectSlowStats` to launch
a fourth goroutine:

```go
ch := make(chan result, 4)
// ... existing three ...
go func() {
    out, _ := execCmd(ctx, "pmset", "-g", "batt")
    ch <- result{"battery", out}
}()
for i := 0; i < 4; i++ {
    r := <-ch
    switch r.key {
    // ... existing cases ...
    case "battery":
        parseBattery(r.val, &stats)
    }
}
```

**Change to `collector/collector.go`:** propagate the new fields when
the fast loop merges the cached slow stats (`collector.go:119-123`):

```go
snap.System.BatteryPresent = slow.BatteryPresent
snap.System.BatteryPercent = slow.BatteryPercent
snap.System.BatteryState = slow.BatteryState
snap.System.BatteryTimeRemaining = slow.BatteryTimeRemaining
```

### 2. Config constants

**New constants in `thermal/internal/config/tuning.go`:**

```go
// Battery severity thresholds (percent remaining).
const (
    BatteryMeltdownPct = 10.0 // below this, cell pulses locally (crit throb)
    BatteryCritPct     = 20.0 // red below this
    BatteryWarnPct     = 50.0 // amber below this, green above
)

// Battery cell rendering.
const (
    BatteryCellWidth         = 7   // cells wide for the 2-row cell
    BatteryChargeBreathFloor = 0.4 // brightness floor during charging breath
)
```

No new anim.Profile fields — charging breath reuses `BreathePhaseStep`
which is already owned by the profile and tuned per calm/default/intense.

### 3. Widget

**New file `thermal/internal/widgets/battery.go`:**

```go
type Battery struct {
    stats         collector.SystemStats
    phase         float64  // breath phase during charging
    meltdownPhase float64  // local meltdown pulse during <10% discharging
    theme         *theme.Theme
    anim          *anim.Profile
}

func NewBattery(th *theme.Theme, ap *anim.Profile) *Battery
func (b *Battery) SetSize(w, height int)          // no-op; fixed width
func (b *Battery) Update(stats collector.SystemStats)
func (b *Battery) AnimTick()                       // advance relevant phase
func (b *Battery) ViewLines(bg color.Color) (top, bot string, width int)
```

`ViewLines` returns empty strings + width 0 when
`!stats.BatteryPresent`, letting the headline's `appendFrag` skip it.

Rendering contract: top row right-pads to `BatteryCellWidth` with
`bgPad(bg, n)`; bot row right-pads likewise. Mirrors the
`renderRailCell` / LCD fragment pattern — iconBg backdrop, foreground
colors from theme gradient.

Braille fuel-gauge rendering — reuses sparkline primitives (no new
glyph tables):

```go
// brailleGauge returns the top and bottom braille runes for a 0-100
// percent reading, filling both left and right columns for a solid-bar
// look. Reuses levelSplit/leftBits/rightBits from sparkline.go.
func brailleGauge(pct float64) (top, bot rune) {
    level := int(pct*8.0/100.0 + 0.5)
    if level < 0 { level = 0 }
    if level > 8 { level = 8 }
    b, t := levelSplit(level)
    return 0x2800 | leftBits[t] | rightBits[t],
           0x2800 | leftBits[b] | rightBits[b]
}
```

Battery widget lives in the same `widgets` package as `sparkline.go`,
so `leftBits` / `rightBits` / `levelSplit` are directly accessible
without export.

Severity → color helper reuses `theme.OverallGradient` (no new LUT):

```go
func (b *Battery) severityFg() color.Color {
    switch {
    case b.stats.BatteryPercent < config.BatteryCritPct:
        return b.theme.OverallGradient[3].Fg
    case b.stats.BatteryPercent < config.BatteryWarnPct:
        return b.theme.OverallGradient[2].Fg
    default:
        return b.theme.OverallGradient[1].Fg
    }
}
```

AnimTick — advance the right phase for the current state:

```go
switch b.stats.BatteryState {
case collector.BatteryCharging, collector.BatteryFinishing:
    b.phase += b.anim.BreathePhaseStep
    b.meltdownPhase = 0
case collector.BatteryDischarging:
    b.phase = 0
    if b.stats.BatteryPercent < config.BatteryMeltdownPct {
        b.meltdownPhase += meltdownPhaseStep // reuse headline.go const
        if b.meltdownPhase > 2*math.Pi {
            b.meltdownPhase -= 2 * math.Pi
        }
    } else {
        b.meltdownPhase = 0
    }
default:
    b.phase = 0
    b.meltdownPhase = 0
}
```

Brightness composition in `ViewLines`:

```go
// Charging breath on ⚡
if charging {
    breath := config.BatteryChargeBreathFloor +
        (1-config.BatteryChargeBreathFloor)*sinNorm(b.phase)
    // apply to ⚡ via AccentR/G/B * breath
}
// Meltdown pulse on braille + digits
meltdown := 1.0
if b.meltdownPhase != 0 {
    meltdown = 0.6 + 0.4*(math.Sin(b.meltdownPhase)+1)/2
}
// final fg = severityFg * meltdown
```

The `meltdownPhaseStep` package-level var is already defined at
`headline.go:22` (it's `var`, not `const`, because it references
`config.AnimFPS`); the battery widget reads it directly since both
live in the `widgets` package.

### 4. Headline integration

**Change to `thermal/internal/widgets/headline.go`:**

Add `battery *Battery` field to `Headline`; construct in `NewHeadline`;
call `battery.AnimTick()` in `AnimTick`.

**Nil-guard in `Update`** — `Headline.Update(state)` already handles
`state == nil` at line 69, but does NOT guard `state.Current == nil`
at the top level (other widgets like
`renderSessionDiamonds` guard it inline at line 375). Battery must
follow the same discipline:

```go
func (h *Headline) Update(state *model.AppState) {
    h.state = state
    if state == nil { /* existing reset */ return }
    // ... existing body ...
    if state.Current != nil {
        h.battery.Update(state.Current.System)
    }
    h.bloom.Update(state)
}
```

This prevents a nil-pointer panic on the very first frame before the
collector has emitted any snapshot.

**Offline mode** — `offlineViewLines()` renders a 1-row strip and does
not call any `appendFrag` chain. Battery is online-path-only; when
`state.Online == false`, the cell does not render (stale data would be
misleading anyway). No code change needed in the offline path — battery
simply never gets a chance to emit. Document this in a code comment on
the battery ViewLines method.

**Insert a fourth `appendFrag` after `lcd`:**

```go
bTop, bBot, bW := h.battery.ViewLines(iconBg)
battery := rowPair{top: bTop, bot: bBot, visWidth: bW}
// ...
appendFrag(sessAgents)
appendFrag(buildShell)
appendFrag(lcd)
appendFrag(battery)
```

No other layout changes required — `rightVis` accounting, `absorbWidth`,
and right-margin padding all already handle arbitrary fragment counts.

**`rebuildBotRight` signature update** — current signature
(`headline.go:508`):

```go
func (h *Headline) rebuildBotRight(buildShell, lcd rowPair, divider string) string
```

New signature:

```go
func (h *Headline) rebuildBotRight(buildShell, lcd, battery rowPair, divider string) string
```

Body gains one trailing `write(battery)` after `write(lcd)`. Exactly
one caller (`headline.go:493`) must be updated to pass the new
fragment.

## Tests (TDD — failing first)

Red-green-refactor per project convention. Four test files; each test
fails before its implementation lands.

### 4a. `collector/battery_test.go`

Table-driven parser cases — one per observed pmset format plus edge
cases:

1. **DischargingWithEstimate** — canonical `47%; discharging; 2:57
   remaining`. Assert all four fields.
2. **ChargingWithEstimate** — `85%; charging; 0:42 remaining`.
3. **Charged** — `100%; charged; 0:00 remaining`. Assert
   `BatteryTimeRemaining == 0`, state == `BatteryCharged`.
4. **ACNotCharging** — `85%; AC attached; not charging; 0:00
   remaining`. Assert state == `BatteryACNotCharging`.
5. **NoEstimate** — `47%; discharging; (no estimate)`. Assert percent
   parsed, `BatteryTimeRemaining == 0`, state == `BatteryDischarging`.
6. **FinishingCharge** — `99%; finishing charge; 0:02 remaining`.
   Assert state == `BatteryFinishing`.
7. **NoBatteryLine** — input with only `Now drawing from 'AC Power'`
   and no `InternalBattery` line (desktop Mac). Assert
   `BatteryPresent == false`, all other fields zero-valued.
8. **MalformedInput** — empty string, random text, truncated lines.
   Parser must not panic; `BatteryPresent == false`.

9. **LeadingWhitespaceTolerance** — pmset emits the battery line with a
   leading tab and variable indentation across macOS versions. Test
   inputs: `"\t -InternalBattery-0 …"`, `"  -InternalBattery-0 …"`,
   and the canonical `" -InternalBattery-0 …"`. All three must parse
   identically. Locks the whitespace contract into tests so a future
   macOS output change fails loudly instead of silently hiding the
   cell.

### 4b. `collector/system_test.go`

Integration test that `CollectSlowStats` wires the battery goroutine
through. Injectable `execCmd` would be ideal but existing GPU/swap
tests use real subprocess calls — follow the same pattern. One test:
`TestCollectSlowStats_IncludesBattery` — asserts that on a laptop,
`stats.BatteryPercent > 0 && stats.BatteryPercent <= 100`. Skipped on
desktop with `if !present { t.Skip() }`.

### 4c. `widgets/battery_test.go`

Widget behavior tests:

1. **HiddenWhenAbsent** — `BatteryPresent=false`, `ViewLines` returns
   `"", "", 0`.
2. **DischargingRender** — 47%, `BatteryDischarging`, 2h57m remaining.
   Top row's first cell is the top braille char for level 4 (47% →
   level 4 of 8), bot row's first cell is the bottom braille char for
   level 4 (fully filled). Percent text `47%` follows on top row,
   `2h57m` on bot. Severity color matches amber (`<50%` bucket).

2a. **BrailleLevelMapping** — table test across pct values: 0→(0,0),
    12→(0,1), 25→(0,2), 50→(0,4), 63→(1,4), 87→(3,4), 100→(4,4) — the
    `(top, bottom)` pairs returned by the braille helper. Documents
    the 8-level quantization explicitly so a later tuning change has
    to break a test to happen.
3. **SeverityBucketGreen** — 80% → green fg. Bucket boundary tested.
4. **SeverityBucketAmber** — 35% → amber fg.
5. **SeverityBucketRed** — 10% → red fg.
6. **SeverityBucketBoundaryExact** — 20% and 50% exact boundary
   behavior (`< critPct` vs `< warnPct`).
7. **ChargingShowsBolt** — `BatteryCharging` state, top row contains
   `⚡`, no braille gauge.
8. **ChargedShowsSolidBolt** — `BatteryCharged`, top row contains
   `⚡`, AnimTick does not advance phase (breath phase stays 0).
9. **ChargingAnimTickAdvancesPhase** — `BatteryCharging`, one
   AnimTick advances `b.phase` by `BreathePhaseStep`. Stable for all
   profiles.
10. **ChargingBrightnessOscillates** — 30 AnimTicks on charging;
    brightness values span `[0.4, 1.0]` and are not monotonic.
11. **NoEstimateRendersDash** — `BatteryTimeRemaining == 0` while
    discharging, bot row is `—` (not `0h00m`).
12. **FixedCellWidth** — for any state, `width == BatteryCellWidth`
    when present, `0` when absent.
13. **MeltdownPulseAtCrit** — 5% discharging; across 30 AnimTicks the
    rendered brightness multiplier spans a detectable range (min
    ≤ 0.7, max ≥ 0.95). At 12% discharging, no pulse — brightness
    stays flat within ε.
14. **MeltdownPulseIndependentOfHeadline** — battery widget's
    meltdown phase advances on its own AnimTick calls, never driven
    by an external headline phase. Two AnimTicks with the widget's
    battery <10% produce monotonically advancing phase values
    regardless of any headline-level state passed elsewhere. Documents
    the intent of phase-independence.
15. **ChargingAboveCritDoesNotPulse** — 8% charging (plugged, rapidly
    refilling): meltdown-pulse suppressed because we're no longer in
    "walk home" territory. Cell shows the charging bolt + breath, not
    crit pulse. (Runway is the *discharging* slope; plugged-at-low is
    fine.)

### 4d. `widgets/headline_test.go` (regression sweep)

Add cases that verify:

1. Battery fragment appears rightmost when present.
2. Battery absence collapses right cluster width correctly (desktop
   simulation — the default `fixtureSnapshot()` in `golden_test.go:23`
   already produces a `SystemStats{}` with `BatteryPresent=false`, so
   existing tests stay green by default).
3. Existing LCD/buildShell/sessAgents layout unchanged when battery
   absent.
4. Right-margin and offline-mode rendering unaffected.

**Pre-existing test that MUST be updated:**
`TestHeadline_RightMarginProtectsLCDDegreeGlyph`
(`headline_test.go:558-584`) asserts the rightmost non-margin rune is
the LCD's degree-sign glyph. When battery is present, the rightmost
rune becomes a filled braille cell (⣿ or similar). The test must be
updated to either:
- (preferred) seed the fixture with `BatteryPresent=false` so it
  continues to test the LCD-on-far-right invariant unchanged, OR
- split into two tests: one with battery absent (asserts degree sign),
  one with battery present (asserts braille cell rightmost pre-margin).

The first option is cleaner — the test's name targets the LCD, so
battery-absent is the correct scope.

**Golden fixtures:** `golden_test.go` exposes `UPDATE_GOLDEN=1` to
re-record. The existing golden list
(`golden_test.go:150-166`) captures rails, breathedots, bloom rows,
etc. — none are full-headline goldens, so no re-capture is actually
required. Adding battery only requires net-new widget goldens (if any
are added in this spec's scope — optional, a behavior-assertion test
is sufficient for v1).

## Visual acceptance

Run `./bin/thermo` on battery, confirm:

1. Cell appears rightmost on the headline strip. Sessions/agents,
   build/shell, and LCD all pushed left from their pre-change positions.
2. Braille fuel gauge visually maps to percent (obvious by comparing to
   `pmset -g batt` in a sibling terminal).
3. Color transitions: at 51% green, at 49% amber, at 21% amber, at
   19% red.
4. Time-remaining reads clearly at the glance cadence (1s update).
5. At <10% discharging, the entire cell pulses on the same envelope as
   the LCD meltdown pulse — impossible to miss from peripheral vision.
   Independent of CPU/headline meltdown state.

Plug in AC, confirm:

6. Cell content flips to `⚡NN%` on the top row.
7. Breath rhythm matches BreatheDots' base breathing cadence (they're
   driven by the same `BreathePhaseStep`).

Wait for 100% / optimized-hold:

8. Breath stops; bolt renders solid at full brightness.

Run `./bin/thermo --demo`: the demo's `SystemStats{}` literal in
`demo/demov2.go:280` gains synthetic battery fields so the cell renders
identically to live mode. Default injection (required — without it,
`--demo` hides the cell on all hardware):

```go
SystemStats{
    // ... existing fields ...
    BatteryPresent:       true,
    BatteryPercent:       65,
    BatteryState:         collector.BatteryDischarging,
    BatteryTimeRemaining: 3 * time.Hour,
}
```

Scripted variations (percent drain across phases, charging plug-in
event, crit-pulse moment) are optional polish — ship with the static
defaults first; add motion in a follow-up if the demo GIF feels flat.

Run on a desktop Mac (or simulate via `BatteryPresent=false` override
env var): cell disappears, rest of strip re-flows naturally.

## Where the change lands (file inventory)

New files:
- `thermal/internal/collector/battery.go`
- `thermal/internal/collector/battery_test.go`
- `thermal/internal/widgets/battery.go`
- `thermal/internal/widgets/battery_test.go`

Modified files:
- `thermal/internal/collector/types.go` — four new `SystemStats`
  fields + `BatteryState` enum.
- `thermal/internal/collector/system.go` — fourth goroutine in
  `CollectSlowStats`, new case in the result switch.
- `thermal/internal/collector/system_test.go` — add live-integration
  case (laptop only).
- `thermal/internal/collector/collector.go` — propagate four new
  fields in the fast-loop merge (`collector.go:119-123`).
- `thermal/internal/config/tuning.go` — five new constants.
- `thermal/internal/widgets/headline.go` — `battery` field on
  `Headline`, construction, Update/AnimTick wiring, fourth
  `appendFrag`, `rebuildBotRight` signature update.
- `thermal/internal/widgets/headline_test.go` or golden fixtures —
  updated to include the new cell.
- `thermal/internal/demo/demov2.go` (line ~280) — add battery fields
  to the `SystemStats{}` literal; required for `--demo` to show the
  cell.
- `thermal/internal/widgets/headline_test.go` —
  `TestHeadline_RightMarginProtectsLCDDegreeGlyph` must be seeded with
  `BatteryPresent=false` (or split; see test plan 4d).

No changes needed to:
- `model/state.go` — battery is pure-system-stat, not an AppState
  derivation. No threat-level formula changes; the severity color is
  rendered directly from raw percent.
- `anim/profile.go` — charging breath reuses `BreathePhaseStep`.
- `theme/*` — severity colors reuse `OverallGradient`.

## Remaining open questions for the human

1. **Config opt-out.** Worth a `COOLANT_BATTERY=0` env override for
   users who don't want it? My default is no — the cell is
   non-disruptive, collapses to zero on desktops, and adds one
   subprocess call per second with ~0 perceptible cost. Opt-out can be
   added later if anyone asks.

## Verification

```bash
cd thermal && go test ./collector/... ./widgets/... ./config/...
cd thermal && go test ./...
cd thermal && go build -o ../bin/thermo ./cmd/thermal/
./bin/thermo                # live, on battery
./bin/thermo --demo         # scripted narrative
```

Commit via `/commit` (never raw `git commit`).
