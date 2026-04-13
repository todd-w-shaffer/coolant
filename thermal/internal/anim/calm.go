package anim

import "github.com/toddwshaffer/coolant/thermal/internal/config"

// Calm returns a meditative animation profile — slower rates, wider brightness
// floors, softer gaussian. Everything breathes gently.
func Calm() *Profile {
	return &Profile{
		Name: "calm",

		TidalPhaseStep:   config.TidalPhaseStep * 0.5,   // half speed wave
		TidalWaveMix:     config.TidalWaveMix,           // same blend ratio
		TidalBreathMix:   config.TidalBreathMix,         // same blend ratio
		TidalBrightFloor: 0.7,                           // higher floor = softer contrast
		TidalPhaseSpread: config.TidalPhaseSpread * 1.2, // wider spread = lazier wave

		GlyphFilledThresh: config.GlyphFilledThresh,
		GlyphMidThresh:    config.GlyphMidThresh,

		KITTSweepRate:    config.KITTSweepRate * 0.5, // half speed scanner
		KITTAmbient:      0.25,                       // brighter edges (softer falloff)
		KITTPeak:         0.75,                       // gentler peak
		KITTSigmaSq:      1.5,                        // wider gaussian = diffuse spotlight
		KITTSingleBright: config.KITTSingleBright,

		BreathePhaseStep: config.BreathePhaseStep * 0.7, // slower breathing
		BreatheStaleRate: config.BreatheStaleRate,
		BreatheStaleDim:  config.BreatheStaleDim,

		SpringFreq:    3.5, // slower spring settle
		SpringDamping: 1.0, // still critically damped

		PeakDecayRate: 0.990, // slower peak decay (~2.3s half-life at 30fps)

		BloomBreatheSecCool: config.BloomBreatheSecCool * 1.25, // longer breath
		BloomBreatheSecHot:  config.BloomBreatheSecHot * 1.25,
		BloomScaleAmpCool:   config.BloomScaleAmpCool * 0.75, // softer swell
		BloomScaleAmpHot:    config.BloomScaleAmpHot * 0.75,
		BloomOpacityMinCool: config.BloomOpacityMinCool,
		BloomOpacityMaxCool: config.BloomOpacityMaxCool * 0.9, // gentler peak
		BloomOpacityMinHot:  config.BloomOpacityMinHot,
		BloomOpacityMaxHot:  config.BloomOpacityMaxHot * 0.9,
		BloomSpringFreq:     config.BloomSpringFreq * 0.7, // lazier heat tracking
		BloomSpringDamping:  config.BloomSpringDamping,
	}
}
