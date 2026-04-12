package widgets

import (
	"image/color"

	"charm.land/lipgloss/v2"
	"github.com/toddwshaffer/coolant/thermal/internal/anim"
	"github.com/toddwshaffer/coolant/thermal/internal/theme"
)

// SegmentReadout renders the 7-segment-style temperature number for the
// headline. It owns value + level, and paints braille rows with theme
// foreground on a caller-supplied background (usually the headline bg).
type SegmentReadout struct {
	value int // 0..99, clamped
	level int // 0..4, index into Theme.OverallGradient
	theme *theme.Theme
	anim  *anim.Profile
}

// NewSegmentReadout constructs a SegmentReadout wired to the given theme.
func NewSegmentReadout(th *theme.Theme, ap *anim.Profile) *SegmentReadout {
	return &SegmentReadout{theme: th, anim: ap}
}

// Update sets the temperature value and the overall thermal level used to
// pick a foreground from Theme.OverallGradient.
func (s *SegmentReadout) Update(value, level int) {
	s.value = value
	if level < 0 {
		level = 0
	}
	if level > 4 {
		level = 4
	}
	s.level = level
}

// Render paints both braille rows with the level's theme foreground on the
// supplied background and returns them with the fixed visible width (12).
func (s *SegmentReadout) Render(bg color.Color) (top, bot string, visWidth int) {
	rawTop, rawBot, w := RenderTemperature(s.value)
	fg := s.theme.OverallGradient[s.level].Fg
	style := lipgloss.NewStyle().Foreground(fg).Background(bg)
	return style.Render(rawTop), style.Render(rawBot), w
}
