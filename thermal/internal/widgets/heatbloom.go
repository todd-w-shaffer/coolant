// HeatBloom renders a left-anchored thermographic accent layer behind the
// headline strip. It exposes per-cell background colors via BgAt that the
// headline consumes in place of its solid iconBg when painting the left
// zone. The bloom respects BloomRightBoundary to guarantee zero visual
// contribution to the right-anchored cluster at any heat or breath phase.
package widgets

import (
	"image/color"
	"math"

	"github.com/charmbracelet/harmonica"
	colorful "github.com/lucasb-eyer/go-colorful"

	"github.com/toddwshaffer/coolant/thermal/internal/anim"
	"github.com/toddwshaffer/coolant/thermal/internal/config"
	"github.com/toddwshaffer/coolant/thermal/internal/model"
	"github.com/toddwshaffer/coolant/thermal/internal/theme"
)

// bloomRestEps is the heat-spring rest threshold for AtRest (applied to both
// the position gap and the velocity). It's a heuristic, not a precise
// render-resolution bound: the byte-stability settle gate backstops it, so it
// only has to read false on a genuine target move (a large position gap) and
// true once the spring has effectively converged.
const bloomRestEps = 0.005

// HeatBloom is a background-only widget with the standard coolant
// lifecycle (SetSize, Update, AnimTick, View/BgAt). It holds no text,
// renders nothing by itself, and does not advance state on Update alone —
// breath and spring both step on AnimTick at config.AnimFPS cadence.
type HeatBloom struct {
	theme   *theme.Theme
	profile *anim.Profile

	width, height int

	// heatTarget is the latest scalar fed by Update; heat eases toward it
	// through the spring. heatVel is the spring's velocity accumulator.
	heatTarget float64
	heat       float64
	heatVel    float64
	spring     harmonica.Spring
	// primed is false until the first Update lands. The first target is
	// snapped (heat = target, vel = 0) so the spring starts at rest —
	// otherwise the boot ramp from 0 → first-snapshot heat would overshoot
	// and paint a flashbulb of the hottest LUT entry across the bloom zone.
	primed bool

	// breathePhase accumulates radians modulo 2π. Period is heat-dependent.
	breathePhase float64
}

// NewHeatBloom constructs the widget with the given theme and profile.
func NewHeatBloom(th *theme.Theme, ap *anim.Profile) *HeatBloom {
	return &HeatBloom{
		theme:   th,
		profile: ap,
		spring:  harmonica.NewSpring(harmonica.FPS(config.AnimFPS), ap.BloomSpringFreq, ap.BloomSpringDamping),
	}
}

// SetSize sets the left-zone dimensions the bloom renders into.
func (b *HeatBloom) SetSize(w, h int) {
	if w < 0 {
		w = 0
	}
	if h < 0 {
		h = 0
	}
	b.width = w
	b.height = h
}

// Update captures the latest composite-heat scalar from AppState as the
// spring target. Safe with nil state (resets target to 0).
func (b *HeatBloom) Update(s *model.AppState) {
	if s == nil {
		b.setTarget(0)
		return
	}
	b.setTarget(s.CompositeHeat())
}

// AtRest reports whether the heat spring has settled at its target — position
// and velocity both within bloomRestEps. The model's tick-stop consults this:
// once the bloom is at rest (and the breathe oscillator is frozen by calm) the
// bloom contributes no per-tick motion, and a later Update that moves the
// target drives AtRest false so the frozen tick wakes to ease the new heat.
// The byte-stability gate backstops the settle side, so this only has to read
// false on a genuine target move (a large position-vs-target gap).
func (b *HeatBloom) AtRest() bool {
	return math.Abs(b.heat-b.heatTarget) < bloomRestEps && math.Abs(b.heatVel) < bloomRestEps
}

// setTarget assigns the spring target, snapping heat to it on the very
// first call so the boot frame starts at rest instead of ramping from 0.
func (b *HeatBloom) setTarget(t float64) {
	b.heatTarget = t
	if !b.primed {
		b.heat = t
		b.heatVel = 0
		b.primed = true
	}
}

// AnimTick advances the spring toward heatTarget and steps the breathe
// oscillator at a heat-dependent rate. When calm, the breathe oscillator
// freezes in place (the heat spring still settles) — the bloom becomes static
// decoration so the composed frame goes byte-stable and bubbletea suppresses it.
func (b *HeatBloom) AnimTick(calm bool) {
	b.heat, b.heatVel = b.spring.Update(b.heat, b.heatVel, b.heatTarget)
	// Underdamped spring (BloomSpringDamping=0.9) intentionally overshoots
	// for "alive" feel, but heat > 1 saturates bloomLUTIndex to the
	// hottest ramp entry and pushes alphaAt's opacity envelope past max —
	// a flashbulb frame on every spike. Clamp keeps the spring's character
	// while capping the visual extreme.
	if b.heat < 0 {
		b.heat = 0
	} else if b.heat > 1 {
		b.heat = 1
	}

	if calm {
		return // freeze the breathe oscillator at its current phase
	}

	periodSec := b.profile.BloomBreatheSecCool +
		(b.profile.BloomBreatheSecHot-b.profile.BloomBreatheSecCool)*b.heat
	if periodSec <= 0 {
		periodSec = b.profile.BloomBreatheSecCool
	}
	step := 2 * math.Pi / (periodSec * float64(config.AnimFPS))
	b.breathePhase += step
	if b.breathePhase > 2*math.Pi {
		b.breathePhase -= 2 * math.Pi
	}
}

// BgAt returns the bloom's background color at (col, row). Cells past
// BloomRightBoundary, out-of-bounds cells, and zero-alpha interior cells
// return the fallback unchanged — the right-cluster bleed guard.
func (b *HeatBloom) BgAt(col, row int, fallback color.Color) color.Color {
	if b.width == 0 || b.height == 0 || col < 0 || row < 0 || row >= b.height {
		return fallback
	}
	if float64(col) >= config.BloomRightBoundary*float64(b.width) {
		return fallback
	}
	alpha := b.alphaAt(col, row)
	if alpha <= 0 {
		return fallback
	}
	bloomC := b.theme.BloomColor(b.heat, b.radialAt(col, row))
	return blendColors(bloomC, fallback, alpha)
}

// alphaAt returns the bloom alpha [0,1] at a cell, combining gaussian
// radial falloff, the breath-scaled ellipse size, and the heat-modulated
// opacity envelope. Zero-sized widgets return 0.
func (b *HeatBloom) alphaAt(col, row int) float64 {
	if b.width == 0 || b.height == 0 {
		return 0
	}
	w := float64(b.width)
	h := float64(b.height)

	breath := (math.Sin(b.breathePhase) + 1) * 0.5

	amp := b.profile.BloomScaleAmpCool +
		(b.profile.BloomScaleAmpHot-b.profile.BloomScaleAmpCool)*b.heat
	scale := 1.0 + amp*(2*breath-1) // oscillates in [1-amp, 1+amp]

	oMin := b.profile.BloomOpacityMinCool +
		(b.profile.BloomOpacityMinHot-b.profile.BloomOpacityMinCool)*b.heat
	oMax := b.profile.BloomOpacityMaxCool +
		(b.profile.BloomOpacityMaxHot-b.profile.BloomOpacityMaxCool)*b.heat
	envelope := oMin + (oMax-oMin)*breath

	nx := (float64(col) + 0.5) / w
	ny := (float64(row) + 0.5) / h
	dx := (nx - config.BloomAnchorX) / (config.BloomRadiusX * scale)
	dy := (ny - config.BloomAnchorY) / (config.BloomRadiusY * scale)
	dist := math.Sqrt(dx*dx + dy*dy)
	if dist >= 1.0 {
		return 0
	}
	return envelope * math.Pow(1-dist, config.BloomFalloffExp)
}

// radialAt returns the radial distance [0,1] from the bloom anchor in
// unscaled ellipse coordinates. Used to look up the color ramp edge
// position — decoupled from breath scale so color stays still while
// only alpha shimmers.
func (b *HeatBloom) radialAt(col, row int) float64 {
	if b.width == 0 || b.height == 0 {
		return 1
	}
	nx := (float64(col) + 0.5) / float64(b.width)
	ny := (float64(row) + 0.5) / float64(b.height)
	dx := (nx - config.BloomAnchorX) / config.BloomRadiusX
	dy := (ny - config.BloomAnchorY) / config.BloomRadiusY
	dist := math.Sqrt(dx*dx + dy*dy)
	if dist > 1 {
		return 1
	}
	return dist
}

// blendColors composites fg over bg with the given alpha in [0,1].
func blendColors(fg colorful.Color, bg color.Color, alpha float64) color.Color {
	if alpha >= 1 {
		r, g, bl := fg.RGB255()
		return color.RGBA{R: r, G: g, B: bl, A: 255}
	}
	if alpha <= 0 {
		return bg
	}
	br, bgr, bb, _ := bg.RGBA()
	fr, fg8, fb := fg.RGB255()
	outR := uint8(float64(fr)*alpha + float64(br>>8)*(1-alpha))
	outG := uint8(float64(fg8)*alpha + float64(bgr>>8)*(1-alpha))
	outB := uint8(float64(fb)*alpha + float64(bb>>8)*(1-alpha))
	return color.RGBA{R: outR, G: outG, B: outB, A: 255}
}
