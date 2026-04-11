package anim

import "github.com/toddwshaffer/coolant/thermal/internal/config"

// Intense returns an urgent, sharp animation profile — faster rates, tighter
// gaussians, snappier springs. Everything reacts immediately.
func Intense() *Profile {
	return &Profile{
		Name: "intense",

		TidalPhaseStep:   config.TidalPhaseStep * 2.0,   // double speed wave
		TidalWaveMix:     config.TidalWaveMix,           // same blend ratio
		TidalBreathMix:   config.TidalBreathMix,         // same blend ratio
		TidalBrightFloor: 0.3,                           // lower floor = higher contrast
		TidalPhaseSpread: config.TidalPhaseSpread * 0.8, // tighter spread = more dots lit at once

		GlyphFilledThresh: config.GlyphFilledThresh,
		GlyphMidThresh:    0.25, // narrower mid zone = sharper glyph transitions

		KITTSweepRate:    config.KITTSweepRate * 2.0,    // double speed scanner
		KITTAmbient:      0.08,                          // darker edges (sharper falloff)
		KITTPeak:         0.92,                          // brighter peak
		KITTSigmaSq:      0.4,                           // tight gaussian = sharp spotlight
		KITTSingleBright: config.KITTSingleBright * 1.2, // slightly brighter single dot

		BreathePhaseStep: config.BreathePhaseStep * 1.5, // faster breathing
		BreatheStaleRate: config.BreatheStaleRate,
		BreatheStaleDim:  config.BreatheStaleDim,

		SpringFreq:    7.0, // snappier spring
		SpringDamping: 0.8, // slightly underdamped — subtle overshoot

		PeakDecayRate: 0.970, // faster peak decay (~0.75s half-life at 30fps)
	}
}
