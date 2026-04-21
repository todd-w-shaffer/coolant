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
		if b.stats.BatteryPercent < config.BatteryMeltdownPct {
			b.meltdownPhase += meltdownPhaseStep
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

	// Meltdown brightness multiplier (1.0 = no pulse).
	meltdown := 1.0
	if b.meltdownPhase != 0 {
		meltdown = 0.6 + 0.4*(math.Sin(b.meltdownPhase)+1)/2
	}
	modFg := sevFg
	if meltdown < 1.0 {
		modFg = modulateBrightness(sevFg, meltdown)
	}
	fgStyle := lipgloss.NewStyle().Foreground(modFg).Background(bg)

	tL, tC, tR, bL, bC, bR := brailleBattery(b.stats.BatteryPercent)
	topGauge := string([]rune{tL, tC, tR})
	botGauge := string([]rune{bL, bC, bR})
	pctText := fmt.Sprintf("%03d%%", int(b.stats.BatteryPercent))

	// Top row: always battery shape + percent (stable across all states).
	topContent := fgStyle.Render(topGauge) + fgStyle.Render(" "+pctText)
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
		botContent = fgStyle.Render(botGauge) + bgPad(bg, 1) + boltStyle.Render("⚡")
		botVis = 3 + 1 + 2 // battery + space + ⚡

	case b.stats.BatteryTimeRemaining > 0:
		timeText := formatRemaining(b.stats.BatteryTimeRemaining)
		botContent = fgStyle.Render(botGauge) + fgStyle.Render(" "+timeText)
		botVis = 4 + len(timeText) // battery + space + timeText

	default:
		// No estimate yet — just the battery shape.
		botContent = fgStyle.Render(botGauge)
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
// Wall: dots 7+8 (bottom row of top char) — the casing's top edge.
// Nipple: dots 1+4 (top row of top char) — the terminal, floating above
// the wall with a visible gap at low fill. The .:.  silhouette emerges
// from the gap between nipple and wall in the center char.
var (
	battWallBits        = leftBits[1] | rightBits[1] // ⣀ — wall top edge
	battNippleBits rune = 0x01 | 0x08                // ⠉ — terminal at top of center char
)

// brailleBattery returns six braille runes (3 top, 3 bottom) forming a
// battery shape. The center top char includes the nipple terminal, side
// top chars include the wall top edge. Fill rises bottom-to-top with
// charge level. Reuses levelSplit/leftBits/rightBits from sparkline.go.
func brailleBattery(pct float64) (topL, topC, topR, botL, botC, botR rune) {
	level := int(pct*8.0/100.0 + 0.5)
	if level < 0 {
		level = 0
	}
	// Cap at 7 so the side top chars never fully fill — the nipple
	// always pokes one row above the sides, preserving the battery
	// silhouette at every charge level. The percent text carries precision.
	if level > 7 {
		level = 7
	}
	b, t := levelSplit(level)

	fill := leftBits[t] | rightBits[t]
	botFill := leftBits[b] | rightBits[b]

	topL = 0x2800 | fill | battWallBits
	topC = 0x2800 | fill | battWallBits | battNippleBits
	topR = 0x2800 | fill | battWallBits
	botL = 0x2800 | botFill
	botC = 0x2800 | botFill
	botR = 0x2800 | botFill
	return
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
