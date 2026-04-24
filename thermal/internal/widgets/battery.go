package widgets

import (
	"fmt"
	"image/color"
	"math"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/toddwshaffer/coolant/thermal/internal/anim"
	"github.com/toddwshaffer/coolant/thermal/internal/collector"
	"github.com/toddwshaffer/coolant/thermal/internal/config"
	"github.com/toddwshaffer/coolant/thermal/internal/theme"
)

// meltdownPhaseStep advances the battery meltdown/warning pulse phase per
// AnimTick for a 1 Hz oscillation at the project's AnimFPS cadence.
var meltdownPhaseStep = 2 * math.Pi / float64(config.AnimFPS)

// Battery renders a 2-row cell showing battery percent, state, and
// time remaining. Always visible on laptops; collapses to zero width
// on desktops where BatteryPresent is false. Online-path-only —
// offline mode does not render the cell (stale data would mislead).
type Battery struct {
	stats         collector.SystemStats
	phase         float64 // breath phase during charging
	meltdownPhase float64 // local meltdown pulse during <10% discharging
	theme         *theme.Theme
	anim          *anim.Profile
}

// NewBattery creates a battery widget bound to the given theme and
// animation profile.
func NewBattery(th *theme.Theme, ap *anim.Profile) *Battery {
	return &Battery{theme: th, anim: ap}
}

// SetSize is a no-op — the battery cell has a fixed width.
func (b *Battery) SetSize(w, height int) {}

// Update refreshes the battery stats from the latest collector snapshot.
func (b *Battery) Update(stats collector.SystemStats) {
	b.stats = stats
}

// AnimTick advances the appropriate phase accumulator for the current
// battery state: charging breath, discharging meltdown pulse, or neither.
func (b *Battery) AnimTick() {
	switch b.stats.BatteryState {
	case collector.BatteryCharging, collector.BatteryFinishing:
		b.phase += b.anim.BreathePhaseStep * config.BatteryBreathRate
		b.meltdownPhase = 0
	case collector.BatteryDischarging:
		b.phase = 0
		if b.stats.BatteryPercent < config.BatteryCritPct {
			var step float64
			if b.stats.BatteryPercent < config.BatteryMeltdownPct {
				step = meltdownPhaseStep // aggressive: 1 cycle/s
			} else {
				step = meltdownPhaseStep * config.BatteryWarnBreathRate // subtle: 1 cycle/2s
			}
			b.meltdownPhase += step
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
}

// ViewLines returns the top and bottom row strings and the cell's
// visible width. Returns ("", "", 0) when no battery is present.
//
// Layout is stable across all states — 3-wide battery shape always renders:
//
//	Discharging:  ⣀⣤⣀ 47%    Charging:  ⣀⣤⣀ 85%    No estimate:  ⣀⣤⣀ 47%
//	              ⣿⣿⣿ 3.0h              ⣿⣿⣿  ⚡                  ⣿⣿⣿
func (b *Battery) ViewLines(bg color.Color) (top, bot string, width int) {
	if !b.stats.BatteryPresent {
		return "", "", 0
	}

	w := config.BatteryCellWidth
	onAC := b.stats.BatteryState == collector.BatteryCharging ||
		b.stats.BatteryState == collector.BatteryFinishing ||
		b.stats.BatteryState == collector.BatteryCharged ||
		b.stats.BatteryState == collector.BatteryACNotCharging
	activeCharge := b.stats.BatteryState == collector.BatteryCharging ||
		b.stats.BatteryState == collector.BatteryFinishing

	sevFg := b.severityFg()

	// Brightness multiplier: warning breath (10-20%) or meltdown pulse (<10%).
	meltdown := 1.0
	if b.meltdownPhase != 0 {
		if b.stats.BatteryPercent < config.BatteryMeltdownPct {
			meltdown = config.BatteryMeltdownBreathFloor +
				(config.BatteryMeltdownBreathCeil-config.BatteryMeltdownBreathFloor)*sinNorm(b.meltdownPhase)
		} else {
			meltdown = config.BatteryWarnBreathFloor +
				(config.BatteryWarnBreathCeil-config.BatteryWarnBreathFloor)*sinNorm(b.meltdownPhase)
		}
	}
	// Gauge breathes with the meltdown/warning pulse; percent + time text
	// stay steady so the numbers remain legible while the battery itself
	// signals urgency.
	modFg := sevFg
	if meltdown < 1.0 {
		modFg = modulateBrightness(sevFg, meltdown)
	}
	gaugeStyle := lipgloss.NewStyle().Foreground(modFg).Background(bg)
	textStyle := lipgloss.NewStyle().Foreground(sevFg).Background(bg)

	tL, tC, tR, bL, bC, bR := brailleBattery(b.stats.BatteryPercent)
	topGauge := string([]rune{tL, tC, tR})
	botGauge := string([]rune{bL, bC, bR})
	pctText := fmt.Sprintf("%03d%%", int(b.stats.BatteryPercent))

	// Top row: always battery shape + percent (stable across all states).
	topContent := gaugeStyle.Render(topGauge) + textStyle.Render(" "+pctText)
	topVis := 4 + len(pctText) // battery=3 + space + pctText
	topPad := w - topVis
	if topPad < 0 {
		topPad = 0
	}

	// Bottom row: gauge + (time | ⚡ | blank).
	var botContent string
	var botVis int

	switch {
	case onAC:
		// ⚡ in the time slot — breath-animated when actively charging,
		// solid when AC-stable (charged, not-charging).
		boltFg := b.theme.OverallGradient[1].Fg
		if activeCharge {
			breath := config.BatteryChargeBreathFloor +
				(config.BatteryChargeBreathCeil-config.BatteryChargeBreathFloor)*sinNorm(b.phase)
			boltFg = modulateBrightness(boltFg, breath)
		}
		boltStyle := lipgloss.NewStyle().Foreground(boltFg).Background(bg)
		// ⚡ (U+26A1) renders as 2 cells in Ghostty/iTerm2.
		botContent = gaugeStyle.Render(botGauge) + bgPad(bg, 1) + boltStyle.Render("⚡")
		botVis = 3 + 1 + 2 // battery + space + ⚡

	case b.stats.BatteryTimeRemaining > 0:
		timeText := formatRemaining(b.stats.BatteryTimeRemaining)
		botContent = gaugeStyle.Render(botGauge) + textStyle.Render(" "+timeText)
		botVis = 4 + len(timeText) // battery + space + timeText

	default:
		// No estimate yet — just the battery shape.
		botContent = gaugeStyle.Render(botGauge)
		botVis = 3
	}

	botPad := w - botVis
	if botPad < 0 {
		botPad = 0
	}

	return topContent + bgPad(bg, topPad),
		botContent + bgPad(bg, botPad),
		w
}

// severityFg returns the foreground color for the current battery percent.
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

// Battery outline bits OR'd into the top row regardless of fill level.
// Nipple: dots 1+4 (top row of center top char) — the terminal.
// Shoulder: row 2 inner dots on side chars — the step-down from the
// narrow nipple to the wider body, closing the battery outline.
const (
	battNippleBits rune = 0x01 | 0x08 // ⠉ — terminal at top of center char
	battShoulderL  rune = 0x10        // dot 5 — right col row 2 of topL
	battShoulderR  rune = 0x02        // dot 2 — left col row 2 of topR
)

// Floor: the bottom-most dot row is structural, giving the battery a
// visible vessel floor at every fill level (including 0%). Excluded
// from the 22-dot fill budget.
const (
	battFloorL rune = 0x80        // dot 8 — right col row 4 of botL
	battFloorC rune = 0x40 | 0x80 // dots 7+8 — both cols row 4 of botC
	battFloorR rune = 0x40        // dot 7 — left col row 4 of botR
)

// Casing walls: outer columns always lit to form the battery outline.
// Bottom chars get full-height walls (4 rows). Top chars get short walls
// (rows 2-4) so the nipple terminal pokes above the casing on row 1.
var (
	casingL    = leftBits[4]  // full left column — bottom char left wall
	casingR    = rightBits[4] // full right column — bottom char right wall
	casingTopL = leftBits[3]  // rows 2-4 — top char left wall, below nipple
	casingTopR = rightBits[3] // rows 2-4 — top char right wall, below nipple
)

// Char index into the 6-rune battery tuple — used by batteryFillOrder
// to route each dot to the correct char without string keys.
const (
	bTopL = iota
	bTopC
	bTopR
	bBotL
	bBotC
	bBotR
)

// fillDot is one entry in the bottom-up, left-to-right dot stream.
type fillDot struct {
	char int
	bits rune
}

// batteryFillOrder is the 22-dot stream: each entry lights up the n-th
// fill dot as the battery charges. Rows are traversed bottom-up starting
// at row G (row H is structural floor); within each row, positions go
// left-to-right. The shoulder row (row B / top char row 2) has only
// two fillable positions — its side dots are permanent silhouette.
var batteryFillOrder = [22]fillDot{
	// Row G — bot char row 3
	{bBotL, 0x20}, {bBotC, 0x04}, {bBotC, 0x20}, {bBotR, 0x04},
	// Row F — bot char row 2
	{bBotL, 0x10}, {bBotC, 0x02}, {bBotC, 0x10}, {bBotR, 0x02},
	// Row E — bot char row 1
	{bBotL, 0x08}, {bBotC, 0x01}, {bBotC, 0x08}, {bBotR, 0x01},
	// Row D — top char row 4
	{bTopL, 0x80}, {bTopC, 0x40}, {bTopC, 0x80}, {bTopR, 0x40},
	// Row C — top char row 3
	{bTopL, 0x20}, {bTopC, 0x04}, {bTopC, 0x20}, {bTopR, 0x04},
	// Row B — top char row 2, shoulder row. Only center positions
	// fillable; side positions are permanent shoulder silhouette.
	{bTopC, 0x02}, {bTopC, 0x10},
}

// brailleBattery returns six braille runes (3 top, 3 bottom) forming a
// battery shape. Silhouette (casing, nipple, shoulder, floor) is always
// lit; the 22 remaining interior positions form a dot stream that fills
// bottom-up / left-to-right as `pct` rises. One fill dot ≈ 4.55% charge,
// giving ~22 discrete visible states across 0-100%. The final two dots
// (shoulder row centers) only light at 100% — a "crown" that makes the
// topped-off state unambiguous.
func brailleBattery(pct float64) (topL, topC, topR, botL, botC, botR rune) {
	n := int(pct*22.0/100.0 + 0.5)
	if n < 0 {
		n = 0
	}
	if n > 22 {
		n = 22
	}

	chars := [6]rune{
		bTopL: 0x2800 | casingTopL | battShoulderL,
		bTopC: 0x2800 | battNippleBits,
		bTopR: 0x2800 | casingTopR | battShoulderR,
		bBotL: 0x2800 | casingL | battFloorL,
		bBotC: 0x2800 | battFloorC,
		bBotR: 0x2800 | casingR | battFloorR,
	}
	for i := 0; i < n; i++ {
		d := batteryFillOrder[i]
		chars[d.char] |= d.bits
	}
	return chars[bTopL], chars[bTopC], chars[bTopR], chars[bBotL], chars[bBotC], chars[bBotR]
}

// formatRemaining renders a time.Duration as "X.Xh" (under 10 hours)
// or "Xh" (10+). Only called when d > 0.
func formatRemaining(d time.Duration) string {
	hours := d.Hours()
	if hours >= 9.95 {
		return fmt.Sprintf("%dhr", int(hours+0.5)) // "10hr" — 4 chars, matches "9.9h"
	}
	return fmt.Sprintf("%.1fh", hours)
}

// modulateBrightness scales a color's brightness by the given factor (0-1).
func modulateBrightness(c color.Color, factor float64) color.Color {
	r, g, b, a := c.RGBA()
	return color.NRGBA{
		R: uint8(float64(r>>8) * factor),
		G: uint8(float64(g>>8) * factor),
		B: uint8(float64(b>>8) * factor),
		A: uint8(a >> 8),
	}
}
