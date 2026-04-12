package widgets

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/toddwshaffer/coolant/thermal/internal/anim"
	"github.com/toddwshaffer/coolant/thermal/internal/theme"
)

// ghostTickCount is how many animation ticks the previous value lingers on
// screen (dimmed) after a value change. At 30fps this yields ~167ms — long
// enough to read as a trail, short enough not to delay the new reading.
const ghostTickCount = 5

// SegmentReadout renders the 7-segment-style temperature number for the
// headline. It owns value + level + a sub-second ghost-trail state machine
// and a single-tick threshold flash that fires on upward level transitions.
type SegmentReadout struct {
	value      int // 0..99, clamped
	prevValue  int
	hasPrev    bool
	ghostTicks int

	level      int // 0..4
	prevLevel  int
	hasLevel   bool
	flashTicks int

	theme *theme.Theme
	anim  *anim.Profile
}

// NewSegmentReadout constructs a SegmentReadout wired to the given theme.
func NewSegmentReadout(th *theme.Theme, ap *anim.Profile) *SegmentReadout {
	return &SegmentReadout{theme: th, anim: ap}
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
	if s.hasPrev && value != s.value {
		s.prevValue = s.value
		s.ghostTicks = ghostTickCount
	}
	if s.hasLevel && level > s.prevLevel {
		s.flashTicks = 1
	}
	s.value = value
	s.level = level
	s.prevLevel = level
	s.hasPrev = true
	s.hasLevel = true
}

// AnimTick advances ghost and flash countdowns; called once per frame.
func (s *SegmentReadout) AnimTick() {
	if s.ghostTicks > 0 {
		s.ghostTicks--
	}
	if s.flashTicks > 0 {
		s.flashTicks--
	}
}

// Render paints the readout on the supplied bg with no pulse modulation.
// Equivalent to RenderWithPulse(bg, 1.0); kept for callers that don't
// participate in the phase-locked meltdown pulse.
func (s *SegmentReadout) Render(bg color.Color) (top, bot string, visWidth int) {
	return s.RenderWithPulse(bg, 1.0)
}

// RenderWithPulse paints both braille rows, honoring the active animation
// state in priority order: ghost trail > threshold flash > pulse-modulated
// steady state. pulseScale is the brightness multiplier (1.0 = full).
func (s *SegmentReadout) RenderWithPulse(bg color.Color, pulseScale float64) (top, bot string, visWidth int) {
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
	rawTop, rawBot, w := RenderTemperature(s.value)
	fgEsc := s.theme.OverallTemperaturePulsedFg(s.value, pulseScale)
	bgStyle := lipgloss.NewStyle().Background(bg)
	return paintFg(bgStyle, fgEsc, rawTop), paintFg(bgStyle, fgEsc, rawBot), w
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
