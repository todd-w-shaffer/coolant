package collector

import (
	"strconv"
	"strings"
	"time"
)

// parseBattery parses `pmset -g batt` output into the battery fields
// on SystemStats. Handles all six observed formats; sets
// BatteryPresent=false when no battery line is found (desktop Mac).
func parseBattery(pmset string, stats *SystemStats) {
	// Find the InternalBattery line — pmset indents it with whitespace.
	var battLine string
	for _, line := range strings.Split(pmset, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "-InternalBattery") {
			battLine = trimmed
			break
		}
	}
	if battLine == "" {
		return
	}

	// Split on tab to isolate the status portion after the battery ID.
	// Format: "-InternalBattery-0 (id=NNN)\tNN%; state; time remaining ..."
	tabIdx := strings.Index(battLine, "\t")
	if tabIdx < 0 {
		return
	}
	status := strings.TrimSpace(battLine[tabIdx+1:])
	if status == "" {
		return
	}

	// Parse percent — first token before ';'
	parts := strings.SplitN(status, ";", 2)
	if len(parts) < 2 {
		return
	}
	pctStr := strings.TrimSpace(parts[0])
	pctStr = strings.TrimSuffix(pctStr, "%")
	pct, err := strconv.ParseFloat(pctStr, 64)
	if err != nil {
		return
	}

	stats.BatteryPresent = true
	stats.BatteryPercent = pct

	// Remaining text after percent: " discharging; 2:57 remaining present: true"
	rest := strings.TrimSpace(parts[1])

	// Detect state from keywords in the rest string.
	switch {
	case strings.Contains(rest, "not charging"):
		stats.BatteryState = BatteryACNotCharging
	case strings.Contains(rest, "finishing charge"):
		stats.BatteryState = BatteryFinishing
	case strings.Contains(rest, "discharging"):
		stats.BatteryState = BatteryDischarging
	case strings.Contains(rest, "charging"):
		stats.BatteryState = BatteryCharging
	case strings.Contains(rest, "charged"):
		stats.BatteryState = BatteryCharged
	default:
		stats.BatteryState = BatteryUnknown
	}

	// Parse time remaining — look for "H:MM remaining".
	if strings.Contains(rest, "(no estimate)") {
		stats.BatteryTimeRemaining = 0
		return
	}
	// Scan for the H:MM pattern in semicolon-delimited fields.
	for _, field := range strings.Split(rest, ";") {
		field = strings.TrimSpace(field)
		if !strings.Contains(field, "remaining") {
			continue
		}
		// Field looks like "2:57 remaining present: true" or "0:00 remaining present: true"
		timeStr := strings.TrimSpace(strings.SplitN(field, "remaining", 2)[0])
		colonIdx := strings.Index(timeStr, ":")
		if colonIdx < 0 {
			break
		}
		hours, err := strconv.Atoi(timeStr[:colonIdx])
		if err != nil {
			break
		}
		mins, err := strconv.Atoi(timeStr[colonIdx+1:])
		if err != nil {
			break
		}
		stats.BatteryTimeRemaining = time.Duration(hours)*time.Hour + time.Duration(mins)*time.Minute
		return
	}
}
