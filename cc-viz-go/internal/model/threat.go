package model

import "github.com/toddwshaffer/coolant/cc-viz-go/internal/collector"

// ThreatLevel represents the system thermal state.
type ThreatLevel int

const (
	ThreatCool     ThreatLevel = iota // green — everything's fine
	ThreatWarm                        // yellow — elevated, worth watching
	ThreatHot                         // orange — approaching limits
	ThreatMeltdown                    // red — swap active, degradation underway
)

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

// Swap thresholds tuned for macOS, which proactively swaps even with free memory.
// Some swap is normal — only escalate when it's growing fast or large.
const (
	swapWarm = 2 << 30  // 2GB — macOS baseline noise
	swapHot  = 8 << 30  // 8GB — real pressure
	swapCrit = 20 << 30 // 20GB — meltdown territory
)

// Classify determines threat level from a snapshot and spawn rate.
func Classify(snap collector.Snapshot, spawnRate float64) ThreatLevel {
	mem := snap.System.MemPercent()
	cpu := snap.System.CPUPercent
	swapUsed := snap.System.SwapUsedBytes

	// Score-based: multiple moderate signals can escalate together
	score := 0

	// Memory pressure
	switch {
	case mem > 90:
		score += 3
	case mem > 80:
		score += 2
	case mem > 65:
		score += 1
	}

	// CPU pressure
	switch {
	case cpu > 90:
		score += 2
	case cpu > 75:
		score += 1
	}

	// Swap — macOS uses some swap normally, only worry when it's significant
	switch {
	case swapUsed > int64(swapCrit):
		score += 3
	case swapUsed > int64(swapHot):
		score += 2
	case swapUsed > int64(swapWarm):
		score += 1
	}

	// Spawn rate
	if spawnRate > 10 {
		score += 1
	}

	switch {
	case score >= 5:
		return ThreatMeltdown
	case score >= 3:
		return ThreatHot
	case score >= 1:
		return ThreatWarm
	default:
		return ThreatCool
	}
}
