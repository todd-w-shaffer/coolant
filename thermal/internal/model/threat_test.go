package model

import (
	"testing"

	"github.com/toddwshaffer/coolant/thermal/internal/collector"
	"github.com/toddwshaffer/coolant/thermal/internal/config"
)

// makeSnap builds a snapshot with specified system stats for Classify testing.
func makeSnap(t *testing.T, cpuPct float64, memUsed, memTotal, swapUsed int64) collector.Snapshot {
	t.Helper()
	return collector.Snapshot{
		System: collector.SystemStats{
			CPUPercent:    cpuPct,
			MemUsedBytes:  memUsed,
			MemTotalBytes: memTotal,
			SwapUsedBytes: swapUsed,
		},
	}
}

func TestClassifyAllZero(t *testing.T) {
	snap := makeSnap(t, 0, 0, 16*GB, 0)
	if got := Classify(snap, 0); got != ThreatCool {
		t.Errorf("all-zero snapshot: got %v, want ThreatCool", got)
	}
}

func TestClassifyHighMemoryAlone(t *testing.T) {
	cases := []struct {
		name      string
		memPct    float64 // will compute memUsed from this
		wantMin   ThreatLevel
		spawnRate float64
	}{
		{"mem at warm pct alone", config.MemWarmPct + 1, ThreatWarm, 0},
		{"mem at hot pct alone → warm (score=2)", config.MemHotPct + 1, ThreatWarm, 0},
		{"mem at crit pct alone → hot (score=3)", config.MemCritPct + 1, ThreatHot, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			memTotal := int64(16 * GB)
			memUsed := int64(tc.memPct / 100.0 * float64(memTotal))
			snap := makeSnap(t, 0, memUsed, memTotal, 0)
			got := Classify(snap, tc.spawnRate)
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
		{"warm CPU", config.CPUWarmPct + 1, ThreatWarm},
		{"crit CPU", config.CPUCritPct + 1, ThreatWarm}, // score=2, still warm
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			snap := makeSnap(t, tc.cpu, 0, 16*GB, 0)
			got := Classify(snap, 0)
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
		{"warm swap", int64(config.SwapWarmBytes) + 1, ThreatWarm},
		{"hot swap", int64(config.SwapHotBytes) + 1, ThreatWarm},  // score=2
		{"crit swap", int64(config.SwapCritBytes) + 1, ThreatHot}, // score=3
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			snap := makeSnap(t, 0, 0, 16*GB, tc.swap)
			got := Classify(snap, 0)
			if got < tc.wantMin {
				t.Errorf("swap %d: got %v, want at least %v", tc.swap, got, tc.wantMin)
			}
		})
	}
}

func TestClassifyMultipleModerateSignalsCombine(t *testing.T) {
	// Warm mem (score 1) + warm CPU (score 1) + warm swap (score 1) = score 3 → Hot
	memTotal := int64(16 * GB)
	memUsed := int64((config.MemWarmPct + 1) / 100.0 * float64(memTotal))
	snap := makeSnap(t, config.CPUWarmPct+1, memUsed, memTotal, int64(config.SwapWarmBytes)+1)
	got := Classify(snap, 0)
	if got != ThreatHot {
		t.Errorf("three moderate signals: got %v, want ThreatHot (score=3)", got)
	}
}

func TestClassifyBoundaryAtWarm(t *testing.T) {
	// Exactly at MemWarmPct should NOT trigger (uses >)
	memTotal := int64(16 * GB)
	memUsed := int64(config.MemWarmPct * float64(memTotal) / 100.0)
	snap := makeSnap(t, 0, memUsed, memTotal, 0)
	got := Classify(snap, 0)
	if got != ThreatCool {
		t.Errorf("exactly at MemWarmPct: got %v, want ThreatCool", got)
	}
}

func TestClassifySpawnRateAddsToScore(t *testing.T) {
	// Warm mem (score 1) + spawn rate escalation (score 1) = score 2 → warm
	// Without spawn rate it would be score 1 → warm too, but let's verify
	// the score contribution: warm mem + warm CPU (score 2) + spawn = 3 → hot
	memTotal := int64(16 * GB)
	memUsed := int64((config.MemWarmPct + 1) / 100.0 * float64(memTotal))
	snap := makeSnap(t, config.CPUWarmPct+1, memUsed, memTotal, 0)

	withoutSpawn := Classify(snap, 0)
	withSpawn := Classify(snap, config.SpawnRateEscalation+1)

	if withSpawn <= withoutSpawn {
		t.Errorf("spawn rate should escalate: without=%v, with=%v", withoutSpawn, withSpawn)
	}
}

func TestClassifyMeltdown(t *testing.T) {
	// crit mem (3) + crit CPU (2) = 5 → meltdown
	memTotal := int64(16 * GB)
	memUsed := int64((config.MemCritPct + 1) / 100.0 * float64(memTotal))
	snap := makeSnap(t, config.CPUCritPct+1, memUsed, memTotal, 0)
	got := Classify(snap, 0)
	if got != ThreatMeltdown {
		t.Errorf("crit mem + crit CPU: got %v, want ThreatMeltdown", got)
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
