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

// Classify determines threat level from a snapshot and spawn rate.
func Classify(snap *collector.Snapshot, spawnRate float64) ThreatLevel {
	mem := snap.System.MemPercent()
	cpu := snap.System.CPUPercent
	swapUsed := snap.System.SwapUsedBytes

	// Score-based: multiple moderate signals can escalate together
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
