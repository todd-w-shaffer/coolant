package anim

import (
	"testing"

	"github.com/toddwshaffer/coolant/thermal/internal/config"
)

func mustGet(t *testing.T, name string) *Profile {
	t.Helper()
	p, err := Get(name)
	if err != nil {
		t.Fatalf("Get(%q): %v", name, err)
	}
	return p
}

func TestGetDefault(t *testing.T) {
	p := mustGet(t, "default")
	if p.Name != "default" {
		t.Errorf("Name = %q, want \"default\"", p.Name)
	}
}

func TestDefaultReadsFromConfig(t *testing.T) {
	p := mustGet(t, "default")

	tests := []struct {
		name string
		got  float64
		want float64
	}{
		{"TidalPhaseStep", p.TidalPhaseStep, config.TidalPhaseStep},
		{"TidalWaveMix", p.TidalWaveMix, config.TidalWaveMix},
		{"TidalBreathMix", p.TidalBreathMix, config.TidalBreathMix},
		{"TidalBrightFloor", p.TidalBrightFloor, config.TidalBrightFloor},
		{"TidalPhaseSpread", p.TidalPhaseSpread, config.TidalPhaseSpread},
		{"GlyphFilledThresh", p.GlyphFilledThresh, config.GlyphFilledThresh},
		{"GlyphMidThresh", p.GlyphMidThresh, config.GlyphMidThresh},
		{"KITTSweepRate", p.KITTSweepRate, config.KITTSweepRate},
		{"KITTAmbient", p.KITTAmbient, config.KITTAmbient},
		{"KITTPeak", p.KITTPeak, config.KITTPeak},
		{"KITTSigmaSq", p.KITTSigmaSq, config.KITTSigmaSq},
		{"KITTSingleBright", p.KITTSingleBright, config.KITTSingleBright},
		{"BreathePhaseStep", p.BreathePhaseStep, config.BreathePhaseStep},
		{"BreatheStaleRate", p.BreatheStaleRate, config.BreatheStaleRate},
		{"BreatheStaleDim", p.BreatheStaleDim, config.BreatheStaleDim},
		{"SpringFreq", p.SpringFreq, config.SpringFreq},
		{"SpringDamping", p.SpringDamping, config.SpringDamping},
		{"PeakDecayRate", p.PeakDecayRate, config.PeakDecayRate},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s = %v, want %v", tt.name, tt.got, tt.want)
			}
		})
	}
}

func TestGetNonexistent(t *testing.T) {
	_, err := Get("nonexistent")
	if err == nil {
		t.Error("Get(\"nonexistent\") should return error")
	}
}

func TestNamesSorted(t *testing.T) {
	names := Names()
	if len(names) == 0 {
		t.Fatal("Names() returned empty slice")
	}
	for i := 1; i < len(names); i++ {
		if names[i] < names[i-1] {
			t.Errorf("Names() not sorted: %q before %q", names[i-1], names[i])
		}
	}
}

func TestNamesContainsDefault(t *testing.T) {
	names := Names()
	found := false
	for _, n := range names {
		if n == "default" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Names() = %v, missing \"default\"", names)
	}
}

func TestCalmSlowerThanDefault(t *testing.T) {
	calm := mustGet(t, "calm")
	def := mustGet(t, "default")

	tests := []struct {
		name        string
		calm, def   float64
		wantSmaller bool // true = calm should be smaller (slower)
	}{
		// Speed: calm is slower
		{"TidalPhaseStep", calm.TidalPhaseStep, def.TidalPhaseStep, true},
		{"KITTSweepRate", calm.KITTSweepRate, def.KITTSweepRate, true},
		{"BreathePhaseStep", calm.BreathePhaseStep, def.BreathePhaseStep, true},
		{"SpringFreq", calm.SpringFreq, def.SpringFreq, true},

		// Brightness/contrast: calm is softer (higher floors, wider gaussians)
		{"TidalBrightFloor", calm.TidalBrightFloor, def.TidalBrightFloor, false}, // higher = softer
		{"TidalPhaseSpread", calm.TidalPhaseSpread, def.TidalPhaseSpread, false}, // wider = lazier
		{"KITTAmbient", calm.KITTAmbient, def.KITTAmbient, false},                // brighter edges
		{"KITTSigmaSq", calm.KITTSigmaSq, def.KITTSigmaSq, false},                // wider gaussian

		// Peak: calm has gentler peak, slower decay
		{"KITTPeak", calm.KITTPeak, def.KITTPeak, true},                 // gentler peak
		{"PeakDecayRate", calm.PeakDecayRate, def.PeakDecayRate, false}, // closer to 1 = slower decay
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantSmaller && tt.calm >= tt.def {
				t.Errorf("calm.%s (%v) should be < default (%v)", tt.name, tt.calm, tt.def)
			}
			if !tt.wantSmaller && tt.calm <= tt.def {
				t.Errorf("calm.%s (%v) should be > default (%v)", tt.name, tt.calm, tt.def)
			}
		})
	}
}

func TestIntenseFasterThanDefault(t *testing.T) {
	intense := mustGet(t, "intense")
	def := mustGet(t, "default")

	tests := []struct {
		name         string
		intense, def float64
		wantLarger   bool // true = intense should be larger (faster)
	}{
		// Speed: intense is faster
		{"TidalPhaseStep", intense.TidalPhaseStep, def.TidalPhaseStep, true},
		{"KITTSweepRate", intense.KITTSweepRate, def.KITTSweepRate, true},
		{"BreathePhaseStep", intense.BreathePhaseStep, def.BreathePhaseStep, true},
		{"SpringFreq", intense.SpringFreq, def.SpringFreq, true},
		{"KITTPeak", intense.KITTPeak, def.KITTPeak, true}, // brighter peak
		{"KITTSingleBright", intense.KITTSingleBright, def.KITTSingleBright, true},

		// Contrast: intense is sharper (lower floors, tighter gaussians)
		{"TidalBrightFloor", intense.TidalBrightFloor, def.TidalBrightFloor, false}, // lower = higher contrast
		{"TidalPhaseSpread", intense.TidalPhaseSpread, def.TidalPhaseSpread, false}, // tighter = more dots lit
		{"KITTSigmaSq", intense.KITTSigmaSq, def.KITTSigmaSq, false},                // tighter gaussian
		{"KITTAmbient", intense.KITTAmbient, def.KITTAmbient, false},                // darker edges
		{"GlyphMidThresh", intense.GlyphMidThresh, def.GlyphMidThresh, false},       // narrower mid zone
		{"SpringDamping", intense.SpringDamping, def.SpringDamping, false},          // underdamped = overshoot

		// Decay: intense peaks decay faster (closer to 0)
		{"PeakDecayRate", intense.PeakDecayRate, def.PeakDecayRate, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantLarger && tt.intense <= tt.def {
				t.Errorf("intense.%s (%v) should be > default (%v)", tt.name, tt.intense, tt.def)
			}
			if !tt.wantLarger && tt.intense >= tt.def {
				t.Errorf("intense.%s (%v) should be < default (%v)", tt.name, tt.intense, tt.def)
			}
		})
	}
}

func TestCalmUnchangedFieldsMatchDefault(t *testing.T) {
	calm := mustGet(t, "calm")
	def := mustGet(t, "default")

	tests := []struct {
		name string
		calm float64
		def  float64
	}{
		{"TidalWaveMix", calm.TidalWaveMix, def.TidalWaveMix},
		{"TidalBreathMix", calm.TidalBreathMix, def.TidalBreathMix},
		{"GlyphFilledThresh", calm.GlyphFilledThresh, def.GlyphFilledThresh},
		{"GlyphMidThresh", calm.GlyphMidThresh, def.GlyphMidThresh},
		{"KITTSingleBright", calm.KITTSingleBright, def.KITTSingleBright},
		{"BreatheStaleRate", calm.BreatheStaleRate, def.BreatheStaleRate},
		{"BreatheStaleDim", calm.BreatheStaleDim, def.BreatheStaleDim},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.calm != tt.def {
				t.Errorf("calm.%s (%v) should equal default (%v)", tt.name, tt.calm, tt.def)
			}
		})
	}
}

func TestIntenseUnchangedFieldsMatchDefault(t *testing.T) {
	intense := mustGet(t, "intense")
	def := mustGet(t, "default")

	tests := []struct {
		name    string
		intense float64
		def     float64
	}{
		{"TidalWaveMix", intense.TidalWaveMix, def.TidalWaveMix},
		{"TidalBreathMix", intense.TidalBreathMix, def.TidalBreathMix},
		{"GlyphFilledThresh", intense.GlyphFilledThresh, def.GlyphFilledThresh},
		{"BreatheStaleRate", intense.BreatheStaleRate, def.BreatheStaleRate},
		{"BreatheStaleDim", intense.BreatheStaleDim, def.BreatheStaleDim},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.intense != tt.def {
				t.Errorf("intense.%s (%v) should equal default (%v)", tt.name, tt.intense, tt.def)
			}
		})
	}
}

func TestNamesContainsAllThree(t *testing.T) {
	names := Names()
	want := map[string]bool{"default": false, "calm": false, "intense": false}
	for _, n := range names {
		if _, ok := want[n]; ok {
			want[n] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("Names() missing %q, got %v", name, names)
		}
	}
}
