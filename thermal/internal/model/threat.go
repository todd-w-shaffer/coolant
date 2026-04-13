package model

import (
	"github.com/toddwshaffer/coolant/thermal/internal/collector"
	"github.com/toddwshaffer/coolant/thermal/internal/config"
)

// ThreatLevel represents the system thermal state.
type ThreatLevel int

const (
	ThreatCool     ThreatLevel = iota // green — everything's fine
	ThreatWarm                        // yellow — elevated, worth watching
	ThreatHot                         // orange — approaching limits
	ThreatMeltdown                    // red — swap active, degradation underway
)

// String returns the human-readable name of the threat level.
func (t ThreatLevel) String() string {
	switch t {
	case ThreatCool:
		return "COOL"
	case ThreatWarm:
		return "WARM"
	case ThreatHot:
		return "HOT"
	case ThreatMeltdown:
		return "MELTDOWN"
	default:
		return "UNKNOWN"
	}
}

// compositeHeatRaw returns the unbounded integer score that drives both
// Classify's bucketing and the CompositeHeatFor scalar. Kept private so
// external callers funnel through one of the two supported views.
func compositeHeatRaw(snap *collector.Snapshot, spawnRate float64) int {
	if snap == nil {
		return 0
	}
	mem := snap.System.MemPercent()
	cpu := snap.System.CPUPercent
	swapUsed := snap.System.SwapUsedBytes

	score := 0

	// Memory pressure
	switch {
	case mem > float64(config.C.Memory.CritPct):
		score += 3
	case mem > float64(config.C.Memory.HotPct):
		score += 2
	case mem > float64(config.C.Memory.WarmPct):
		score += 1
	}

	// CPU pressure
	switch {
	case cpu > float64(config.C.CPU.CritPct):
		score += 2
	case cpu > float64(config.C.CPU.WarmPct):
		score += 1
	}

	// Swap — macOS uses some swap normally, only worry when it's significant
	switch {
	case swapUsed > config.C.Swap.CritBytes:
		score += 3
	case swapUsed > config.C.Swap.HotBytes:
		score += 2
	case swapUsed > config.C.Swap.WarmBytes:
		score += 1
	}

	// Spawn rate
	if spawnRate > config.C.Spawn.RateEscalation {
		score += 1
	}

	return score
}

// CompositeHeatFor returns the composite pressure scalar in [0.0, 1.0]
// derived from the same signals Classify uses. 0 = fully idle,
// 1 = score >= Meltdown threshold. The HeatBloom widget consumes this
// as a continuous heat target, while Classify buckets it for threat UI.
func CompositeHeatFor(snap *collector.Snapshot, spawnRate float64) float64 {
	if config.C.Score.Meltdown <= 0 {
		return 0
	}
	score := compositeHeatRaw(snap, spawnRate)
	scalar := float64(score) / float64(config.C.Score.Meltdown)
	if scalar < 0 {
		return 0
	}
	if scalar > 1 {
		return 1
	}
	return scalar
}

// Classify determines threat level from a snapshot and spawn rate.
func Classify(snap *collector.Snapshot, spawnRate float64) ThreatLevel {
	score := compositeHeatRaw(snap, spawnRate)
	switch {
	case score >= config.C.Score.Meltdown:
		return ThreatMeltdown
	case score >= config.C.Score.Hot:
		return ThreatHot
	case score >= config.C.Score.Warm:
		return ThreatWarm
	default:
		return ThreatCool
	}
}
