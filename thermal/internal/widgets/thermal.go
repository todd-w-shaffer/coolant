package widgets

import (
	"github.com/toddwshaffer/coolant/thermal/internal/collector"
	"github.com/toddwshaffer/coolant/thermal/internal/config"
)

// thermalLevelFor returns 0-4 based on count vs category thresholds.
func thermalLevelFor(catName string, count int) int {
	thresh := config.C.CatThreshold(catName)
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
