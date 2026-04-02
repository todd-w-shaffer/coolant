package widgets

import (
	"fmt"
	"strings"
)

// SparkThresholds defines the warn/crit boundaries for per-dot coloring.
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

		// Zero value = minimum green dot (always show something)
		if v == 0 {
			bar = brailleBars[1]
			color = sparkGreen
		}

		sb.WriteString(color)
		sb.WriteRune(bar)
		sb.WriteString(sparkReset)
	}

	return sb.String()
}

// RenderSparklineCompact renders the most recent `width` data points.
// No downsampling — each dot is one real sample. The rightmost dot
// always represents the most recent value, matching the percentage display.
func RenderSparklineCompact(data []float64, width int, maxOverride float64, thresh *SparkThresholds) string {
	return RenderSparkline(data, width, maxOverride, thresh)
}

// RenderSparklineWithMask renders a sparkline where offline ticks become
// rainbow dots instead of severity bars. The mask indicates online (true)
// or offline (false) for each data point. Seamless transitions — rainbow
// dots sit inline with real data in the timeline.
func RenderSparklineWithMask(data []float64, online []bool, width int, maxOverride float64, thresh *SparkThresholds, tick int) string {
	if width <= 0 {
		return ""
	}

	// Align data and mask to same length (use shorter)
	n := len(data)
	if len(online) < n {
		n = len(online)
	}
	if n == 0 {
		return RenderSparkline(data, width, maxOverride, thresh)
	}

	// Trim both to visible window from the end
	startData := 0
	startMask := 0
	if n > width {
		startData = len(data) - width
		startMask = len(online) - width
		n = width
	} else {
		startData = len(data) - n
		startMask = len(online) - n
	}

	visibleData := data[startData:]
	visibleMask := online[startMask:]

	// Find peak for auto-scaling (only online values)
	peak := maxOverride
	if peak <= 0 {
		for i, v := range visibleData {
			if i < len(visibleMask) && visibleMask[i] && v > peak {
				peak = v
			}
		}
	}
	if peak <= 0 {
		peak = 1
	}

	var sb strings.Builder

	// Right-align padding
	pad := width - len(visibleData)
	if pad > 0 {
		sb.WriteString(strings.Repeat(" ", pad))
	}

	for i, v := range visibleData {
		isOnline := true
		if i < len(visibleMask) {
			isOnline = visibleMask[i]
		}

		if !isOnline {
			// Offline: static rainbow confetti — random but stable per position
			ch, color := rainbowChar(i)
			sb.WriteString(color)
			sb.WriteRune(ch)
			sb.WriteString(sparkReset)
			continue
		}

		// Online: severity dot
		if v < 0 {
			v = 0
		}

		var bar rune
		var color string

		if v == 0 {
			bar = brailleBars[1]
			color = sparkGreen
		} else if thresh == nil || v < thresh.Warn {
			bar = brailleBars[1]
			color = sparkGreen
		} else if v < thresh.Crit {
			bar = brailleBars[2]
			color = sparkYellow
		} else {
			bar = brailleBars[3]
			color = sparkRed
		}

		sb.WriteString(color)
		sb.WriteRune(bar)
		sb.WriteString(sparkReset)
	}

	return sb.String()
}

// Rainbow colors for offline mode sparklines.
var rainbowColors = []string{
	"\033[31m", // red
	"\033[33m", // yellow
	"\033[32m", // green
	"\033[36m", // cyan
	"\033[34m", // blue
	"\033[35m", // magenta
}

// Random braille patterns for offline mode — irreverent dot positions.
// Left column dots: 1(1), 2(2), 3(4), 7(64). Mix and match freely.
var funBraille = []rune{
	0x2800 + 64,         // ⡀ bottom only
	0x2800 + 1,          // ⠁ top only
	0x2800 + 2,          // ⠂ second from top
	0x2800 + 4,          // ⠄ third from top
	0x2800 + 64 + 1,     // ⡁ top + bottom
	0x2800 + 2 + 4,      // ⠆ middle two
	0x2800 + 1 + 4,      // ⠅ top + third
	0x2800 + 1 + 64,     // ⡁ top + bottom
	0x2800 + 2 + 64,     // ⡂ second + bottom
	0x2800 + 1 + 2,      // ⠃ top two
	0x2800 + 4 + 64,     // ⡄ third + bottom
	0x2800 + 1 + 4 + 64, // ⡅ top + third + bottom
	0x2800 + 2 + 4 + 64, // ⡆ middle two + bottom
	0x2800 + 1 + 2 + 64, // ⡃ top two + bottom
}

// rainbowChar picks a random braille pattern and rainbow color for position i.
// Deterministic per position — once generated, it stays put. No animation.
func rainbowChar(i int) (rune, string) {
	patIdx := (i*7 + i*i*3) % len(funBraille)
	colorIdx := (i*3 + i*i) % len(rainbowColors)
	return funBraille[patIdx], rainbowColors[colorIdx]
}

// FormatFixedWidth formats a value with a fixed total width, right-aligned.
func FormatFixedWidth(format string, width int, args ...interface{}) string {
	s := fmt.Sprintf(format, args...)
	if len(s) >= width {
		return s[:width]
	}
	return strings.Repeat(" ", width-len(s)) + s
}
