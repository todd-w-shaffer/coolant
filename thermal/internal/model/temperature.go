package model

import "math"

// OverallTemperature maps the system's current threat level and resource
// pressure to a 0–99 integer for the segment-LCD readout.
//
// Bands are disjoint so the integer alone communicates threat state:
//
//	Cool      5..29
//	Warm     30..54
//	Hot      55..79
//	Meltdown 80..99
//
// Pressure (max of CPU%, MEM%) slides the value within its band. A band base
// dominates a pressure offset, so cross-band movement is unambiguous even
// when pressure temporarily swings.
func OverallTemperature(s *AppState) int {
	if s == nil || s.Current == nil {
		return 0
	}
	// Use the deadbanded CPU% (not raw) so sub-band jitter doesn't flip the
	// displayed temperature integer every snapshot, which would defeat the
	// idle frame suppression (the LCD renders the integer, and a ±2% raw-CPU
	// wiggle can cross a temperature boundary).
	pressure := math.Max(s.DisplayCPUPercent(), s.Current.System.MemPercent())
	if pressure < 0 {
		pressure = 0
	}
	if pressure > 100 {
		pressure = 100
	}

	var base, span float64
	switch s.ThreatLevel {
	case ThreatMeltdown:
		base, span = 80, 19
	case ThreatHot:
		base, span = 55, 24
	case ThreatWarm:
		base, span = 30, 24
	default: // ThreatCool
		base, span = 5, 24
	}

	v := base + pressure*span/100
	if v < 0 {
		v = 0
	}
	if v > 99 {
		v = 99
	}
	return int(math.Round(v))
}
