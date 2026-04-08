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
func Classify(snap collector.Snapshot, spawnRate float64) ThreatLevel {
	mem := snap.System.MemPercent()
	cpu := snap.System.CPUPercent
	swapUsed := snap.System.SwapUsedBytes

	// Score-based: multiple moderate signals can escalate together
	score := 0

	// Memory pressure
	switch {
	case mem > config.MemCritPct:
		score += 3
	case mem > config.MemHotPct:
		score += 2
	case mem > config.MemWarmPct:
		score += 1
	}

	// CPU pressure
	switch {
	case cpu > config.CPUCritPct:
		score += 2
	case cpu > config.CPUWarmPct:
		score += 1
	}

	// Swap — macOS uses some swap normally, only worry when it's significant
	switch {
	case swapUsed > config.SwapCritBytes:
		score += 3
	case swapUsed > config.SwapHotBytes:
		score += 2
	case swapUsed > config.SwapWarmBytes:
		score += 1
	}

	// Spawn rate
	if spawnRate > config.SpawnRateEscalation {
		score += 1
	}

	switch {
	case score >= config.ScoreMeltdown:
		return ThreatMeltdown
	case score >= config.ScoreHot:
		return ThreatHot
	case score >= config.ScoreWarm:
		return ThreatWarm
	default:
		return ThreatCool
	}
}
