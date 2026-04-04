package widgets

import (
	"image/color"

	"charm.land/lipgloss/v2"
	"github.com/toddwshaffer/coolant/thermal/internal/config"
)

// thermalLevel pairs foreground and background colors for a heat level.
type thermalLevel struct {
	fg color.Color
	bg color.Color
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
	thresh, ok := config.CatThresholds[catName]
	if !ok {
		thresh = config.CatThresholdDefault
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
