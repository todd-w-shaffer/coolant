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

// ghostTickCount is how many animation ticks the previous value lingers on
// screen (dimmed) after a value change. At 30fps this yields ~167ms — long
// enough to read as a trail, short enough not to delay the new reading.
const ghostTickCount = 5

// ghostDeltaThreshold is the minimum value change that arms the ghost
// trail. Small jitter from EMA smoothing (1–2 points) must NOT trigger a
// ghost, otherwise the readout re-arms every snapshot and is stuck
// permanently in the dimmed prev-value state.
const ghostDeltaThreshold = 3

// SegmentReadout renders the 7-segment-style temperature number for the
// headline. It owns value + level + a sub-second ghost-trail state machine
// and a single-tick threshold flash that fires on upward level transitions.
type SegmentReadout struct {
	value      int // 0..99, clamped
	prevValue  int
	ghostTicks int

	level      int // 0..4
	prevLevel  int
	hasLevel   bool
	flashTicks int

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

// Update sets the temperature value and the overall thermal level. Arms
// two animations: a ghost trail on value change (previous value lingers
// dimmed) and a 1-tick inverted flash on a monotonic upward level move.
// Downward transitions are silent — relief shouldn't pop.
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
	// Ghost-trail arming was removed: spring smoothing now does the easing
	// the ghost was approximating, and the dim→snap transition fought the
	// spring's continuous walk. ghostTicks/prevValue/ghost render branch
	// remain as dead code paths pending a post-merge cleanup pass.
	if s.hasLevel && level > s.prevLevel {
		s.flashTicks = 1
	}
	s.level = level
	s.prevLevel = level
	s.hasLevel = true
}

// AnimTick advances the spring toward the raw target, then updates the
// displayed integer with boundary hysteresis (ghost may fire when the
// displayed int crosses ghostDeltaThreshold). Flash/ghost countdowns tick
// last so a freshly-armed ghost gets its full window.
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
	if s.ghostTicks > 0 {
		s.ghostTicks--
	}
	if s.flashTicks > 0 {
		s.flashTicks--
	}
}

// Render paints both braille rows, honoring the active animation state in
// priority order: ghost trail > threshold flash > steady state.
func (s *SegmentReadout) Render(bg color.Color) (top, bot string, visWidth int) {
	if s.ghostTicks > 0 {
		rawTop, rawBot, w := RenderTemperature(s.prevValue)
		fgEsc := s.theme.OverallTemperatureFgDimmed(s.prevValue)
		bgStyle := lipgloss.NewStyle().Background(bg)
		return paintFg(bgStyle, fgEsc, rawTop), paintFg(bgStyle, fgEsc, rawBot), w
	}
	if s.flashTicks > 0 {
		// Inverted frame: swap the level's fg/bg for one render tick.
		levelFg := s.theme.OverallGradient[s.level].Fg
		levelBg := s.theme.OverallGradient[s.level].Bg
		style := lipgloss.NewStyle().Foreground(levelBg).Background(levelFg)
		rawTop, rawBot, w := RenderTemperature(s.value)
		return style.Render(rawTop), style.Render(rawBot), w
	}
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
