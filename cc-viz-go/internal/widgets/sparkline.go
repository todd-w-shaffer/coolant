package widgets

import "strings"

// Block characters for sparklines, 8 levels from empty to full.
var sparkBlocks = []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

// RenderSparkline renders a sparkline string from float64 values.
// Values are normalized to [0, max]. The sparkline is right-aligned:
// if len(data) < width, empty space pads the left.
// maxOverride > 0 forces the scale; otherwise auto-scales to peak.
func RenderSparkline(data []float64, width int, maxOverride float64) string {
	if width <= 0 {
		return ""
	}

	// Trim to visible window
	visible := data
	if len(visible) > width {
		visible = visible[len(visible)-width:]
	}

	// Find peak for auto-scaling
	peak := maxOverride
	if peak <= 0 {
		for _, v := range visible {
			if v > peak {
				peak = v
			}
		}
	}
	if peak <= 0 {
		peak = 1
	}

	var sb strings.Builder

	// Right-align: pad left with spaces
	pad := width - len(visible)
	if pad > 0 {
		sb.WriteString(strings.Repeat(" ", pad))
	}

	for _, v := range visible {
		if v < 0 {
			v = 0
		}
		norm := v / peak
		if norm > 1 {
			norm = 1
		}
		idx := int(norm * 7)
		if idx > 7 {
			idx = 7
		}
		sb.WriteRune(sparkBlocks[idx])
	}

	return sb.String()
}
