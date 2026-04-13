package widgets

import (
	"image/color"
	"time"

	colorful "github.com/lucasb-eyer/go-colorful"

	"github.com/toddwshaffer/coolant/thermal/internal/theme"
)

// railState tracks the directional-rail ember state for one build/shell
// cell. The rail signals "incoming heat" at peakLevel, and when the live
// level drops below peak it eases back toward iconBg over emberDuration
// via a time-based decay — so the animation is framerate-independent.
type railState struct {
	currentLevel int       // level driven by the smoothed count right now
	peakLevel    int       // highest level still held during ember decay
	decayStart   time.Time // when the live level first dropped below peak (zero = not decaying)
}

// update folds a new live level into rs at wall time `now`, collapsing the
// peak once the full emberDuration has elapsed past its decayStart. A
// rising level snaps peak up and clears any in-flight decay.
func (rs *railState) update(level int, now time.Time, emberDuration time.Duration) {
	rs.currentLevel = level
	switch {
	case level >= rs.peakLevel:
		// Warming (or matching) — clamp peak to current level, no decay.
		rs.peakLevel = level
		rs.decayStart = time.Time{}
	case rs.decayStart.IsZero():
		// First frame after a drop — arm the decay timer.
		rs.decayStart = now
	default:
		// Mid-decay — check whether the ember has fully cooled.
		if now.Sub(rs.decayStart) >= emberDuration {
			rs.peakLevel = level
			rs.decayStart = time.Time{}
		}
	}
}

// decayAt returns the [0,1] decay scalar for rs at wall time now. 1.0 =
// rail at full peak color; 0.0 = rail collapsed to iconBg.
func (rs *railState) decayAt(now time.Time, emberDuration time.Duration) float64 {
	if rs.decayStart.IsZero() {
		return 1.0
	}
	elapsed := now.Sub(rs.decayStart)
	if elapsed <= 0 {
		return 1.0
	}
	if elapsed >= emberDuration {
		return 0.0
	}
	return 1.0 - float64(elapsed)/float64(emberDuration)
}

// railColor returns the rail fg color for CategoryGradient[level] HCL-blended
// toward iconBg by (1 - decay). decay=1 returns the full-strength Fg;
// decay=0 returns iconBg exactly (endpoint precision is required by tests
// so idle rails are visually invisible against the pinned backdrop).
func railColor(th *theme.Theme, level int, iconBg color.Color, decay float64) color.Color {
	// Level 0 is the "cold" category gradient stop; painting it would
	// leave a dim rail visible even when no build/shell activity exists.
	// Collapse to iconBg so idle cells read as pure backdrop.
	if level == 0 {
		return iconBg
	}
	if decay >= 1.0 {
		return th.CategoryGradient[level].Fg
	}
	if decay <= 0.0 {
		return iconBg
	}
	fg := colorfulFromColor(th.CategoryGradient[level].Fg)
	bg := colorfulFromColor(iconBg)
	blended := bg.BlendHcl(fg, decay).Clamped()
	r, g, b := blended.RGB255()
	return color.RGBA{R: r, G: g, B: b, A: 0xFF}
}

// colorfulFromColor is a local mirror of theme.colorfulFromColor so the
// rail helper doesn't pull an unexported import dependency.
func colorfulFromColor(c color.Color) colorful.Color {
	r, g, b, _ := c.RGBA()
	return colorful.Color{
		R: float64(r) / 65535.0,
		G: float64(g) / 65535.0,
		B: float64(b) / 65535.0,
	}
}
