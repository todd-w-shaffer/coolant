package widgets

import (
	"image/color"
	"math"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/harmonica"

	"github.com/toddwshaffer/coolant/thermal/internal/anim"
	"github.com/toddwshaffer/coolant/thermal/internal/config"
	"github.com/toddwshaffer/coolant/thermal/internal/theme"
)

// lcdRestVelEps is the temperature-spring velocity threshold for AtRest
// (units/tick). Below it the eased digit won't move another integer step.
const lcdRestVelEps = 0.25

// SegmentReadout renders the 7-segment-style temperature number for the
// headline. It owns value + level, with spring-smoothed easing between
// numeric positions to eliminate boundary flicker on EMA jitter.
type SegmentReadout struct {
	value int // 0..99, clamped
	level int // 0..4

	// Spring-smoothed numeric position. rawTarget is the latest value passed
	// to Update (clamped to 0..99); tempPos/tempVel are advanced by AnimTick
	// via a critically-damped spring. The displayed int (s.value) lags the
	// raw target by ~200ms, eliminating boundary flicker on EMA jitter.
	tempSpring harmonica.Spring
	tempPos    float64
	tempVel    float64
	rawTarget  float64
	seeded     bool

	theme *theme.Theme
	anim  *anim.Profile
}

// NewSegmentReadout constructs a SegmentReadout wired to the given theme.
func NewSegmentReadout(th *theme.Theme, ap *anim.Profile) *SegmentReadout {
	return &SegmentReadout{
		theme:      th,
		anim:       ap,
		tempSpring: harmonica.NewSpring(harmonica.FPS(config.AnimFPS), config.LCDSpringFreq, config.LCDSpringDamping),
	}
}

// Update sets the temperature value and the overall thermal level.
func (s *SegmentReadout) Update(value, level int) {
	if level < 0 {
		level = 0
	}
	if level > 4 {
		level = 4
	}
	if value < 0 {
		value = 0
	}
	if value > 99 {
		value = 99
	}
	s.rawTarget = float64(value)
	if !s.seeded {
		// First Update: jump spring state and committed value to target so
		// cold start displays immediately with no easing from zero.
		s.tempPos = s.rawTarget
		s.tempVel = 0
		s.value = value
		s.seeded = true
	}
	s.level = level
}

// AnimTick advances the spring toward the raw target, then updates the
// displayed integer with boundary hysteresis.
func (s *SegmentReadout) AnimTick() {
	if s.seeded {
		s.tempPos, s.tempVel = s.tempSpring.Update(s.tempPos, s.tempVel, s.rawTarget)
		diff := s.tempPos - float64(s.value)
		switch {
		case math.Abs(diff) > 1.0:
			// Big jump: snap to rounded spring position so large deltas
			// don't take ~100 ticks to converge one integer at a time.
			s.value = int(math.Round(s.tempPos))
		case diff >= 0.5+config.LCDBoundaryHyst:
			s.value++
		case diff <= -(0.5 + config.LCDBoundaryHyst):
			s.value--
		}
	}
}

// AtRest reports whether the temperature spring has settled at its target —
// the eased position is within half a unit of the raw target (so the displayed
// integer matches and won't tick further) and velocity is negligible. The
// model's tick-stop consults this: an Update that moves rawTarget drives the
// gap large and AtRest false, so the frozen tick wakes to ease the LCD to the
// new reading instead of latching at the stale one. Unseeded reads at rest
// (nothing displayed yet).
func (s *SegmentReadout) AtRest() bool {
	if !s.seeded {
		return true
	}
	return math.Abs(s.tempPos-s.rawTarget) < 0.5 && math.Abs(s.tempVel) < lcdRestVelEps
}

// Render paints both braille rows as three per-digit styled spans.
func (s *SegmentReadout) Render(bg color.Color) (top, bot string, visWidth int) {
	// Three per-digit styled spans so cells stay byte-identical when only
	// part of the readout changes. Tens span is colored by its band anchor
	// (value/10 * 10) so it's stable across any in-band ones change. Degree
	// is colored by threat level so it only repaints on a band transition.
	tens := s.value / 10
	ones := s.value % 10
	tensTop, tensBot := digitToBraille(segmentDigits[tens])
	onesTop, onesBot := digitToBraille(segmentDigits[ones])
	degTop, degBot := degreeToBraille()

	tensFg := s.theme.OverallTemperatureFg(tens * 10)
	onesFg := s.theme.OverallTemperatureFg(s.value)
	degFg := s.theme.OverallLevelFg(s.level)

	bgStyle := lipgloss.NewStyle().Background(bg)
	gap := bgStyle.Render(" ")
	top = paintFg(bgStyle, tensFg, string(tensTop[:])) + gap + paintFg(bgStyle, onesFg, string(onesTop[:])) + gap + paintFg(bgStyle, degFg, string(degTop[:]))
	bot = paintFg(bgStyle, tensFg, string(tensBot[:])) + gap + paintFg(bgStyle, onesFg, string(onesBot[:])) + gap + paintFg(bgStyle, degFg, string(degBot[:]))
	visWidth = 10
	return
}

// paintFg composes a truecolor fg ANSI escape with a lipgloss bg style.
// Placed alongside Render so the fg precomputation survives lipgloss's
// internal ANSI resolution.
func paintFg(bgStyle lipgloss.Style, fgEsc, content string) string {
	var sb strings.Builder
	sb.Grow(len(content)*4 + len(fgEsc) + 8)
	sb.WriteString(fgEsc)
	sb.WriteString(content)
	sb.WriteString("\033[39m")
	return bgStyle.Render(sb.String())
}
