package anim

import "github.com/toddwshaffer/coolant/thermal/internal/config"

// Default returns the baseline animation profile — all values read from
// config/tuning.go so there is exactly one source of truth for shipped defaults.
func Default() *Profile {
	return &Profile{
		Name: "default",

		TidalPhaseStep:   config.TidalPhaseStep,
		TidalWaveMix:     config.TidalWaveMix,
		TidalBreathMix:   config.TidalBreathMix,
		TidalBrightFloor: config.TidalBrightFloor,
		TidalPhaseSpread: config.TidalPhaseSpread,

		GlyphFilledThresh: config.GlyphFilledThresh,
		GlyphMidThresh:    config.GlyphMidThresh,

		KITTSweepRate:    config.KITTSweepRate,
		KITTAmbient:      config.KITTAmbient,
		KITTPeak:         config.KITTPeak,
		KITTSigmaSq:      config.KITTSigmaSq,
		KITTSingleBright: config.KITTSingleBright,

		BreathePhaseStep: config.BreathePhaseStep,
		BreatheStaleRate: config.BreatheStaleRate,
		BreatheStaleDim:  config.BreatheStaleDim,

		SpringFreq:    config.SpringFreq,
		SpringDamping: config.SpringDamping,

		PeakDecayRate: config.PeakDecayRate,

		BloomBreatheSecCool: config.BloomBreatheSecCool,
		BloomBreatheSecHot:  config.BloomBreatheSecHot,
		BloomScaleAmpCool:   config.BloomScaleAmpCool,
		BloomScaleAmpHot:    config.BloomScaleAmpHot,
		BloomOpacityMinCool: config.BloomOpacityMinCool,
		BloomOpacityMaxCool: config.BloomOpacityMaxCool,
		BloomOpacityMinHot:  config.BloomOpacityMinHot,
		BloomOpacityMaxHot:  config.BloomOpacityMaxHot,
		BloomSpringFreq:     config.BloomSpringFreq,
		BloomSpringDamping:  config.BloomSpringDamping,
	}
}
