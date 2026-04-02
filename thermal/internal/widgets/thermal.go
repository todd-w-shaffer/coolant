package widgets

import "github.com/charmbracelet/lipgloss"

// Per-category thermal thresholds: how many procs before it gets warm/hot.
// Encodes danger — 3 test procs is warm, 3 search procs is ice cold.
var catThresholds = map[string][2]int{
	"test":   {2, 4},   // warm at 2, hot at 4 — each ~1GB
	"build":  {3, 6},   // warm at 3, hot at 6 — ~300MB each
	"run":    {4, 8},   // warm at 4, hot at 8 — variable weight
	"search": {10, 25}, // warm at 10, hot at 25 — lightweight
	"shell":  {15, 40}, // warm at 15, hot at 40 — ephemeral
}

// thermalLevel pairs foreground and background colors for a heat level.
type thermalLevel struct {
	fg lipgloss.Color
	bg lipgloss.Color
}

// thermalGradient: 5 levels from invisible to glowing.
var thermalGradient = []thermalLevel{
	{lipgloss.Color("236"), lipgloss.Color("233")}, // cold: nearly invisible
	{lipgloss.Color("240"), lipgloss.Color("234")}, // cool: barely there
	{lipgloss.Color("180"), lipgloss.Color("235")}, // warm: dim amber text
	{lipgloss.Color("214"), lipgloss.Color("236")}, // hot: orange, readable
	{lipgloss.Color("196"), lipgloss.Color("52")},  // critical: bright red on dark red
}

// thermalLevelFor returns 0-4 based on count vs category thresholds.
func thermalLevelFor(catName string, count int) int {
	thresh, ok := catThresholds[catName]
	if !ok {
		thresh = [2]int{10, 25}
	}
	warm := thresh[0]
	hot := thresh[1]

	if count == 0 {
		return 0
	}

	switch {
	case count >= hot:
		return 4
	case count >= (warm+hot)/2:
		return 3
	case count >= warm:
		return 2
	default:
		return 1
	}
}
