package model

import (
	"fmt"

	"github.com/toddwshaffer/coolant/thermal/internal/config"
)

const (
	GB = 1 << 30
	MB = 1 << 20
)

// WeightClass maps process type codes to estimated memory usage in bytes.
// V1: hardcoded heuristics. V2 will learn from observed RSS.
var WeightClass = map[string]int64{
	"V": 1 * GB,   // vitest/jest — V8 heap per worker
	"N": 1 * GB,   // node/deno/bun — general runtime
	"T": 300 * MB, // tsc — TypeScript compiler
	"B": 300 * MB, // bundlers/linters/compilers
	"P": 200 * MB, // python/ruby/java/docker
	"C": 50 * MB,  // git/curl/cat — utilities
	"G": 20 * MB,  // grep/ag
	"R": 20 * MB,  // ripgrep
	"F": 20 * MB,  // find/fd
	"S": 20 * MB,  // bash/sh/zsh/sed/awk
	"X": 50 * MB,  // unknown
}

// HeadroomInfo summarizes projected memory state.
type HeadroomInfo struct {
	MemAvailBytes int64  // total - used (before swap)
	EstCommitted  int64  // sum of weight class estimates for Claude procs
	HeadroomBytes int64  // available - committed (can be negative)
	HeavyProcs    int    // count of HEAVY weight class procs (V, N)
	HeavyEstimate int64  // just the heavy proc estimate
	Warning       string // human-readable warning, empty if fine
}

// EstimateHeadroom computes projected memory commitment from type counts and system stats.
func EstimateHeadroom(typeCounts map[string]int, memUsed, memTotal int64) HeadroomInfo {
	avail := memTotal - memUsed

	var estCommitted int64
	var heavyCount int
	var heavyEst int64

	for typeCode, count := range typeCounts {
		weight, ok := WeightClass[typeCode]
		if !ok {
			weight = WeightClass["X"]
		}
		est := weight * int64(count)
		estCommitted += est

		if typeCode == "V" || typeCode == "N" {
			heavyCount += count
			heavyEst += est
		}
	}

	headroom := avail - estCommitted

	info := HeadroomInfo{
		MemAvailBytes: avail,
		EstCommitted:  estCommitted,
		HeadroomBytes: headroom,
		HeavyProcs:    heavyCount,
		HeavyEstimate: heavyEst,
	}

	// Generate warning if headroom is tight
	switch {
	case headroom < 0:
		info.Warning = fmt.Sprintf("!! over-committed by ~%s", FormatBytes(-headroom))
	case headroom < config.HeadroomCritBytes*GB:
		info.Warning = fmt.Sprintf("!! headroom ~%s before swap", FormatBytes(headroom))
	case headroom < config.HeadroomWarnBytes*GB:
		info.Warning = fmt.Sprintf("headroom ~%s", FormatBytes(headroom))
	}

	return info
}

// FormatBytes returns a human-readable byte size.
func FormatBytes(b int64) string {
	switch {
	case b >= GB:
		return fmt.Sprintf("%.1fGB", float64(b)/float64(GB))
	case b >= MB:
		return fmt.Sprintf("%dMB", b/MB)
	default:
		return fmt.Sprintf("%dKB", b/1024)
	}
}
