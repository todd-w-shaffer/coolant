package config

import (
	"math"
	"testing"
)

func TestTidalPhaseSpreadExists(t *testing.T) {
	// TidalPhaseSpread was previously hardcoded inline (1.5 rad/dot).
	// Verify it exists as a named constant and is positive.
	if TidalPhaseSpread <= 0 {
		t.Errorf("TidalPhaseSpread = %v, want > 0", TidalPhaseSpread)
	}
}

// TestAnimationWallClockInvariance guards the property that motivated the
// AnimFPS reduction: the per-tick animation steps are DERIVED from AnimFPS so
// the wall-clock speed of breathing / tidal wave / KITT sweep / peak-decay
// stays constant when AnimFPS changes. A raw per-tick constant would silently
// run 2x slow when AnimFPS halves — the regression this test exists to catch.
func TestAnimationWallClockInvariance(t *testing.T) {
	fps := float64(AnimFPS)

	// Breathing: full cycle should be ~1.5s regardless of AnimFPS.
	if got := 2 * math.Pi / (BreathePhaseStep * fps); math.Abs(got-1.5) > 0.01 {
		t.Errorf("breathe cycle = %.3fs, want 1.5s (BreathePhaseStep not AnimFPS-derived?)", got)
	}
	// Tidal wave: full wave ~14s.
	if got := 2 * math.Pi / (TidalPhaseStep * fps); math.Abs(got-14.0) > 0.1 {
		t.Errorf("tidal wave = %.3fs, want 14s", got)
	}
	// KITT sweep: 0.75 sweep-units/sec.
	if got := KITTSweepRate * fps; math.Abs(got-0.75) > 0.001 {
		t.Errorf("KITT sweep = %.4f/s, want 0.75/s", got)
	}
	// Peak decay: half-life ~1.27s → per-second factor 0.5^(1/1.27) ≈ 0.578.
	wantPerSec := math.Pow(0.5, 1.0/1.27)
	if got := math.Pow(PeakDecayRate, fps); math.Abs(got-wantPerSec) > 0.01 {
		t.Errorf("peak decay = %.4f/s, want %.4f/s (~1.27s half-life)", got, wantPerSec)
	}
	// Sparkline window: 20s of frames.
	if MaxRenderHistory != 20*AnimFPS {
		t.Errorf("MaxRenderHistory = %d, want %d (20s*AnimFPS)", MaxRenderHistory, 20*AnimFPS)
	}
}

// TestPeakDecayForHalfLifeOrdering guards the calm > default > intense decay
// ordering through the half-life derivation (calm decays slowest → rate closest
// to 1; intense fastest → smallest rate).
func TestPeakDecayForHalfLifeOrdering(t *testing.T) {
	intense := PeakDecayForHalfLife(0.75)
	def := PeakDecayForHalfLife(peakDecayHalfLifeSec)
	calm := PeakDecayForHalfLife(2.3)
	if !(intense < def && def < calm) {
		t.Errorf("want intense(%.4f) < default(%.4f) < calm(%.4f)", intense, def, calm)
	}
}
