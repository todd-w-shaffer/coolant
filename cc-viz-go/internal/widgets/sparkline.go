package widgets

import (
	"fmt"
	"strings"
)

// SparkThresholds define the warn/crit boundaries for per-dot coloring.
type SparkThresholds struct {
	Warn float64
	Crit float64
}

// ANSI color codes.
const (
	sparkGreen  = "\033[32m"
	sparkYellow = "\033[33m"
	sparkRed    = "\033[31m"
	sparkDim    = "\033[2;37m"
	sparkReset  = "\033[0m"
)

// Braille codepoints for left-column-only bars, bottom-up.
// Left column dots bottom to top: 7(64), 3(4), 2(2), 1(1)
var brailleBars = []rune{
	0x2800,      // 0 dots: empty
	0x2800 + 64, // 1 dot:  ⡀  (dot 7)
	0x2800 + 68, // 2 dots: ⡄  (dots 7+3)
	0x2800 + 70, // 3 dots: ⡆  (dots 7+3+2)
}

// RenderSparkline renders a braille sparkline where bar HEIGHT encodes severity.
// 1 dot high = green (below warn), 2 dots = yellow (warn-crit), 3 dots = red (above crit).
// Each braille cell is one time sample. Right-aligned with left padding.
func RenderSparkline(data []float64, width int, maxOverride float64, thresh *SparkThresholds) string {
	if width <= 0 {
		return ""
	}

	visible := data
	if len(visible) > width {
		visible = visible[len(visible)-width:]
	}

	var sb strings.Builder

	// Right-align
	pad := width - len(visible)
	if pad > 0 {
		sb.WriteString(strings.Repeat(" ", pad))
	}

	for _, v := range visible {
		if v < 0 {
			v = 0
		}

		// Classify severity → bar height + color
		var bar rune
		var color string

		if thresh == nil || v < thresh.Warn {
			bar = brailleBars[1] // 1 dot
			color = sparkGreen
		} else if v < thresh.Crit {
			bar = brailleBars[2] // 2 dots
			color = sparkYellow
		} else {
			bar = brailleBars[3] // 3 dots
			color = sparkRed
		}

		// Zero value = empty
		if v == 0 {
			bar = brailleBars[0]
			color = sparkDim
		}

		sb.WriteString(color)
		sb.WriteRune(bar)
		sb.WriteString(sparkReset)
	}

	return sb.String()
}

// RenderSparklineCompact downsamples data to fit width, then renders.
func RenderSparklineCompact(data []float64, width int, maxOverride float64, thresh *SparkThresholds) string {
	if width <= 0 || len(data) == 0 {
		return ""
	}

	// Downsample by averaging if data is wider than available space
	if len(data) > width {
		binSize := len(data) / width
		var downsampled []float64
		for i := 0; i < width; i++ {
			start := i * binSize
			end := start + binSize
			if end > len(data) {
				end = len(data)
			}
			sum := 0.0
			for _, v := range data[start:end] {
				sum += v
			}
			downsampled = append(downsampled, sum/float64(end-start))
		}
		data = downsampled
	}

	return RenderSparkline(data, width, maxOverride, thresh)
}

// FormatFixedWidth formats a value with a fixed total width, right-aligned.
func FormatFixedWidth(format string, width int, args ...interface{}) string {
	s := fmt.Sprintf(format, args...)
	if len(s) >= width {
		return s[:width]
	}
	return strings.Repeat(" ", width-len(s)) + s
}
