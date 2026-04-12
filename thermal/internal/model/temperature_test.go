package model

import (
	"testing"
)

// TestOverallTemperature_NilSafe — zero value inputs map to zero output.
func TestOverallTemperature_NilSafe(t *testing.T) {
	if got := OverallTemperature(nil); got != 0 {
		t.Errorf("OverallTemperature(nil): got %d, want 0", got)
	}
	s := NewAppState()
	if got := OverallTemperature(s); got != 0 {
		t.Errorf("OverallTemperature(no snapshot): got %d, want 0", got)
	}
}

// TestOverallTemperature_Bands — within each threat band the number sits in
// a pinned numeric window so cross-band movement is unambiguous. Pressure
// (max CPU, MEM) drifts the number inside the band.
func TestOverallTemperature_Bands(t *testing.T) {
	cases := []struct {
		name   string
		level  ThreatLevel
		cpu    float64
		memPct float64
		wantLo int
		wantHi int
	}{
		{"cool idle", ThreatCool, 0, 0, 5, 8},
		{"cool mid pressure", ThreatCool, 50, 10, 15, 22},
		{"cool high pressure", ThreatCool, 95, 20, 27, 30},
		{"warm idle", ThreatWarm, 0, 0, 30, 33},
		{"warm mid", ThreatWarm, 50, 30, 40, 47},
		{"warm high", ThreatWarm, 80, 60, 48, 54},
		{"hot idle", ThreatHot, 0, 0, 55, 58},
		{"hot mid", ThreatHot, 60, 40, 65, 72},
		{"hot high", ThreatHot, 95, 85, 77, 79},
		{"meltdown idle", ThreatMeltdown, 0, 0, 80, 83},
		{"meltdown mid", ThreatMeltdown, 60, 50, 88, 94},
		{"meltdown pegged", ThreatMeltdown, 100, 95, 97, 99},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := buildTempState(t, tc.level, tc.cpu, tc.memPct)
			got := OverallTemperature(s)
			if got < tc.wantLo || got > tc.wantHi {
				t.Errorf("level=%v cpu=%.0f mem=%.0f: got %d, want in [%d,%d]",
					tc.level, tc.cpu, tc.memPct, got, tc.wantLo, tc.wantHi)
			}
		})
	}
}

// TestOverallTemperature_MonotonicInPressure — at fixed threat level, higher
// pressure produces higher (or equal) output. No non-monotonic dips.
func TestOverallTemperature_MonotonicInPressure(t *testing.T) {
	for _, level := range []ThreatLevel{ThreatCool, ThreatWarm, ThreatHot, ThreatMeltdown} {
		prev := -1
		for p := 0; p <= 100; p += 10 {
			s := buildTempState(t, level, float64(p), float64(p))
			got := OverallTemperature(s)
			if got < prev {
				t.Errorf("level=%v: monotonicity broke at pressure=%d: %d < %d", level, p, got, prev)
			}
			prev = got
		}
	}
}

// TestOverallTemperature_BandsDisjoint — maximum output of each band is
// strictly less than the minimum output of the next band, so a viewer can
// read the threat level off the number alone.
func TestOverallTemperature_BandsDisjoint(t *testing.T) {
	coolMax := OverallTemperature(buildTempState(t, ThreatCool, 100, 100))
	warmMin := OverallTemperature(buildTempState(t, ThreatWarm, 0, 0))
	warmMax := OverallTemperature(buildTempState(t, ThreatWarm, 100, 100))
	hotMin := OverallTemperature(buildTempState(t, ThreatHot, 0, 0))
	hotMax := OverallTemperature(buildTempState(t, ThreatHot, 100, 100))
	meltMin := OverallTemperature(buildTempState(t, ThreatMeltdown, 0, 0))

	if coolMax >= warmMin {
		t.Errorf("cool max %d >= warm min %d", coolMax, warmMin)
	}
	if warmMax >= hotMin {
		t.Errorf("warm max %d >= hot min %d", warmMax, hotMin)
	}
	if hotMax >= meltMin {
		t.Errorf("hot max %d >= meltdown min %d", hotMax, meltMin)
	}
}

// TestOverallTemperature_Clamp — output stays in [0,99].
func TestOverallTemperature_Clamp(t *testing.T) {
	s := buildTempState(t, ThreatMeltdown, 999, 999)
	if got := OverallTemperature(s); got < 0 || got > 99 {
		t.Errorf("extreme inputs: got %d, want 0..99", got)
	}
	s2 := buildTempState(t, ThreatCool, -100, -100)
	if got := OverallTemperature(s2); got < 0 {
		t.Errorf("negative pressure: got %d, want >= 0", got)
	}
}

func buildTempState(t *testing.T, level ThreatLevel, cpu, memPct float64) *AppState {
	t.Helper()
	s := NewAppState()
	snap := testSnap(t, withCPU(cpu), withMem(pctToBytes(memPct), testMemTotal))
	s.Current = &snap
	s.ThreatLevel = level
	return s
}
