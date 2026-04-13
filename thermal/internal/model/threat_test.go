package model

import (
	"testing"

	"github.com/toddwshaffer/coolant/thermal/internal/collector"
	"github.com/toddwshaffer/coolant/thermal/internal/config"
)

func TestClassifyAllZero(t *testing.T) {
	snap := testSnap(t)
	if got := Classify(&snap, 0); got != ThreatCool {
		t.Errorf("all-zero snapshot: got %v, want ThreatCool", got)
	}
}

func TestClassifyHighMemoryAlone(t *testing.T) {
	cases := []struct {
		name      string
		memPct    float64
		wantMin   ThreatLevel
		spawnRate float64
	}{
		{"mem at warm pct alone", float64(config.C.Memory.WarmPct) + 1, ThreatWarm, 0},
		{"mem at hot pct alone → warm (score=2)", float64(config.C.Memory.HotPct) + 1, ThreatWarm, 0},
		{"mem at crit pct alone → hot (score=3)", float64(config.C.Memory.CritPct) + 1, ThreatHot, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			snap := testSnap(t, withMem(pctToBytes(tc.memPct), testMemTotal))
			got := Classify(&snap, tc.spawnRate)
			if got < tc.wantMin {
				t.Errorf("mem %.0f%%: got %v, want at least %v", tc.memPct, got, tc.wantMin)
			}
		})
	}
}

func TestClassifyHighCPUAlone(t *testing.T) {
	cases := []struct {
		name    string
		cpu     float64
		wantMin ThreatLevel
	}{
		{"warm CPU", float64(config.C.CPU.WarmPct) + 1, ThreatWarm},
		{"crit CPU", float64(config.C.CPU.CritPct) + 1, ThreatWarm}, // score=2, still warm
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			snap := testSnap(t, withCPU(tc.cpu))
			got := Classify(&snap, 0)
			if got < tc.wantMin {
				t.Errorf("CPU %.0f%%: got %v, want at least %v", tc.cpu, got, tc.wantMin)
			}
		})
	}
}

func TestClassifyHighSwapAlone(t *testing.T) {
	cases := []struct {
		name    string
		swap    int64
		wantMin ThreatLevel
	}{
		{"warm swap", config.C.Swap.WarmBytes + 1, ThreatWarm},
		{"hot swap", config.C.Swap.HotBytes + 1, ThreatWarm},  // score=2
		{"crit swap", config.C.Swap.CritBytes + 1, ThreatHot}, // score=3
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			snap := testSnap(t, withSwap(tc.swap))
			got := Classify(&snap, 0)
			if got < tc.wantMin {
				t.Errorf("swap %d: got %v, want at least %v", tc.swap, got, tc.wantMin)
			}
		})
	}
}

func TestClassifyMultipleModerateSignalsCombine(t *testing.T) {
	// Warm mem (score 1) + warm CPU (score 1) + warm swap (score 1) = score 3 → Hot
	memUsed := pctToBytes(float64(config.C.Memory.WarmPct) + 1)
	snap := testSnap(t,
		withCPU(float64(config.C.CPU.WarmPct)+1),
		withMem(memUsed, testMemTotal),
		withSwap(config.C.Swap.WarmBytes+1),
	)
	got := Classify(&snap, 0)
	if got != ThreatHot {
		t.Errorf("three moderate signals: got %v, want ThreatHot (score=3)", got)
	}
}

func TestClassifyBoundaryAtWarm(t *testing.T) {
	// Exactly at MemWarmPct should NOT trigger (uses >)
	memUsed := pctToBytes(float64(config.C.Memory.WarmPct))
	snap := testSnap(t, withMem(memUsed, testMemTotal))
	got := Classify(&snap, 0)
	if got != ThreatCool {
		t.Errorf("exactly at MemWarmPct: got %v, want ThreatCool", got)
	}
}

func TestClassifySpawnRateAddsToScore(t *testing.T) {
	// warm mem + warm CPU (score 2) + spawn = 3 → hot
	memUsed := pctToBytes(float64(config.C.Memory.WarmPct) + 1)
	snap := testSnap(t, withCPU(float64(config.C.CPU.WarmPct)+1), withMem(memUsed, testMemTotal))

	withoutSpawn := Classify(&snap, 0)
	withSpawnRate := Classify(&snap, config.C.Spawn.RateEscalation+1)

	if withSpawnRate <= withoutSpawn {
		t.Errorf("spawn rate should escalate: without=%v, with=%v", withoutSpawn, withSpawnRate)
	}
}

func TestClassifyMeltdown(t *testing.T) {
	// crit mem (3) + crit CPU (2) = 5 → meltdown
	memUsed := pctToBytes(float64(config.C.Memory.CritPct) + 1)
	snap := testSnap(t, withCPU(float64(config.C.CPU.CritPct)+1), withMem(memUsed, testMemTotal))
	got := Classify(&snap, 0)
	if got != ThreatMeltdown {
		t.Errorf("crit mem + crit CPU: got %v, want ThreatMeltdown", got)
	}
}

func TestCompositeHeatMatchesClassify(t *testing.T) {
	cases := []struct {
		name     string
		snap     collector.Snapshot
		spawn    float64
		wantBand ThreatLevel
	}{
		{"idle", testSnap(t), 0, ThreatCool},
		{"warm mem+cpu+swap", testSnap(t,
			withCPU(float64(config.C.CPU.WarmPct)+1),
			withMem(pctToBytes(float64(config.C.Memory.WarmPct)+1), testMemTotal),
			withSwap(config.C.Swap.WarmBytes+1),
		), 0, ThreatHot},
		{"crit combo", testSnap(t,
			withCPU(float64(config.C.CPU.CritPct)+1),
			withMem(pctToBytes(float64(config.C.Memory.CritPct)+1), testMemTotal),
		), 0, ThreatMeltdown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			scalar := CompositeHeatFor(&tc.snap, tc.spawn)
			if scalar < 0 || scalar > 1 {
				t.Fatalf("scalar %v out of [0,1]", scalar)
			}
			got := Classify(&tc.snap, tc.spawn)
			if got != tc.wantBand {
				t.Fatalf("Classify=%v want %v", got, tc.wantBand)
			}
		})
	}
}

func TestCompositeHeatClamp(t *testing.T) {
	snap := testSnap(t, withCPU(-100))
	v := CompositeHeatFor(&snap, 0)
	if v < 0 || v > 1 {
		t.Fatalf("scalar %v not clamped to [0,1]", v)
	}
	if v := CompositeHeatFor(nil, 0); v != 0 {
		t.Errorf("nil snapshot: got %v, want 0", v)
	}
}

func TestAppStateCompositeHeat(t *testing.T) {
	s := NewAppState()
	if got := s.CompositeHeat(); got != 0 {
		t.Errorf("uninit AppState: got %v, want 0", got)
	}
	snap := testSnap(t, withCPU(float64(config.C.CPU.CritPct)+1))
	s.Current = &snap
	if got := s.CompositeHeat(); got <= 0 {
		t.Errorf("CPU-hot snapshot: got %v, want > 0", got)
	}
}

func TestThreatLevelString(t *testing.T) {
	cases := []struct {
		level ThreatLevel
		want  string
	}{
		{ThreatCool, "COOL"},
		{ThreatWarm, "WARM"},
		{ThreatHot, "HOT"},
		{ThreatMeltdown, "MELTDOWN"},
		{ThreatLevel(99), "UNKNOWN"},
	}
	for _, tc := range cases {
		if got := tc.level.String(); got != tc.want {
			t.Errorf("ThreatLevel(%d).String() = %q, want %q", tc.level, got, tc.want)
		}
	}
}
