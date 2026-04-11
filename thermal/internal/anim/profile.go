// Package anim defines pluggable animation profiles for the thermal dashboard.
// Profiles control motion parameters (speed, brightness, spring physics) while
// themes control color. The two compose orthogonally.
package anim

// Profile bundles all animation constants into a single tunable unit.
type Profile struct {
	Name string

	// -- Tidal wave (active agent dots) --
	TidalPhaseStep   float64 // phase advance per tick
	TidalWaveMix     float64 // wave weight in brightness blend
	TidalBreathMix   float64 // individual breath weight
	TidalBrightFloor float64 // minimum brightness
	TidalPhaseSpread float64 // rad between adjacent dots

	// -- Glyph thresholds --
	GlyphFilledThresh float64 // wave > this → filled glyph
	GlyphMidThresh    float64 // wave > this → mid glyph

	// -- KITT scanner (stale/completed dots) --
	KITTSweepRate    float64 // sweep position per tick
	KITTAmbient      float64 // floor brightness at edges
	KITTPeak         float64 // peak brightness contribution
	KITTSigmaSq      float64 // gaussian width
	KITTSingleBright float64 // single-dot fallback brightness

	// -- Breathing (base dot animation) --
	BreathePhaseStep float64 // phase advance per tick
	BreatheStaleRate float64 // multiplier for stale dots
	BreatheStaleDim  float64 // brightness multiplier for stale dots

	// -- Spring physics (shared: dots + gauges) --
	SpringFreq    float64
	SpringDamping float64

	// -- Gauge --
	PeakDecayRate float64
}
