package widgets

import (
	"image/color"

	"charm.land/lipgloss/v2"
	"github.com/toddwshaffer/coolant/thermal/internal/collector"
	"github.com/toddwshaffer/coolant/thermal/internal/config"
)

// thermalLevel pairs foreground and background colors for a heat level.
type thermalLevel struct {
	fg color.Color
	bg color.Color
}

// thermalGradient: 5 levels from invisible to glowing.
var thermalGradient = []thermalLevel{
	{lipgloss.Color("236"), lipgloss.Color("233")}, // cold: nearly invisible (zero count)
	{lipgloss.Color("180"), lipgloss.Color("234")}, // cool: dim amber — visible on appearance
	{lipgloss.Color("214"), lipgloss.Color("235")}, // warm: orange, clearly active
	{lipgloss.Color("208"), lipgloss.Color("236")}, // hot: bright orange
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

	// Fixed categories (build, shell) stay cold below warm — low counts are normal.
	// Dynamic runtimes get level 1 (amber) on any presence — their appearance is the signal.
	if count < warm {
		if collector.FixedCategories[catName] {
			return 0
		}
		return 1
	}

	switch {
	case count >= hot:
		return 4
	case count >= (warm+hot)/2:
		return 3
	default:
		return 2
	}
}
