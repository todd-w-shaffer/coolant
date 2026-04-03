package widgets

import (
	"fmt"
	"strings"

	colorful "github.com/lucasb-eyer/go-colorful"
)

// SparkThresholds defines the warn/crit boundaries for per-dot coloring.
type SparkThresholds struct {
	Warn float64
	Crit float64
}

// ANSI reset and dim (still needed for offline/special states).
const (
	sparkDim   = "\033[2;37m"
	sparkReset = "\033[0m"
)

// Gradient anchor colors for severity interpolation (HCL perceptual space).
var (
	gradGreen  = mustHex("#22c55e")
	gradYellow = mustHex("#eab308")
	gradRed    = mustHex("#ef4444")
)

func mustHex(hex string) colorful.Color {
	c, _ := colorful.Hex(hex)
	return c
}

// severityColor returns a truecolor ANSI escape for a value relative to
// thresholds. Interpolates green→yellow below warn, yellow→red between
// warn and crit, solid red above crit. Produces smooth per-dot gradients.
func severityColor(v float64, thresh *SparkThresholds) string {
	if thresh == nil {
		return truecolorFg(gradGreen)
	}

	var c colorful.Color
	switch {
	case v >= thresh.Crit:
		c = gradRed
	case v >= thresh.Warn:
		ratio := (v - thresh.Warn) / (thresh.Crit - thresh.Warn)
		c = gradYellow.BlendHcl(gradRed, ratio).Clamped()
	default:
		if thresh.Warn <= 0 {
			c = gradGreen
		} else {
			ratio := v / thresh.Warn
			c = gradGreen.BlendHcl(gradYellow, ratio).Clamped()
		}
	}

	return truecolorFg(c)
}

// truecolorFg emits \033[38;2;R;G;Bm for 24-bit foreground color.
func truecolorFg(c colorful.Color) string {
	r, g, b := c.RGB255()
	return fmt.Sprintf("\033[38;2;%d;%d;%dm", r, g, b)
}

// Braille codepoints for bars, bottom-up. 4 filled levels, left column only.
// Left column dots bottom to top: 7(0x40), 3(0x04), 2(0x02), 1(0x01)
// Color handles additional resolution — height stays single-width.
var brailleBars = []rune{
	0x2800,        // 0: empty
	0x2800 + 0x40, // 1: ⡀  (dot 7)
	0x2800 + 0x44, // 2: ⡄  (dots 7+3)
	0x2800 + 0x46, // 3: ⡆  (dots 7+3+2)
	0x2800 + 0x47, // 4: ⡇  (dots 7+3+2+1, full left column)
}

// maxLevels is the number of filled braille levels (1–4).
const maxLevels = 4

// RenderSparkline renders a braille sparkline where HEIGHT is proportional to
// the value (auto-scaled to the visible window's peak) and COLOR encodes
// severity. This gives EKG-style motion even at idle — small fluctuations
// become visible instead of collapsing into a single severity bucket.
func RenderSparkline(data []float64, width int, maxOverride float64, thresh *SparkThresholds) string {
	if width <= 0 {
		return ""
	}

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

	// Right-align
	pad := width - len(visible)
	if pad > 0 {
		sb.WriteString(strings.Repeat(" ", pad))
	}

	for _, v := range visible {
		if v < 0 {
			v = 0
		}

		// Height: proportional to peak, 1–5 (non-zero values always at least 1)
		level := int((v / peak) * float64(maxLevels))
		if level > maxLevels {
			level = maxLevels
		}
		if level < 1 && v > 0 {
			level = 1 // non-zero always visible
		}

		bar := brailleBars[level]

		sb.WriteString(severityColor(v, thresh))
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

		// Online: proportional height, severity color
		if v < 0 {
			v = 0
		}

		level := int((v / peak) * float64(maxLevels))
		if level > maxLevels {
			level = maxLevels
		}
		if level < 1 && v > 0 {
			level = 1
		}

		bar := brailleBars[level]

		sb.WriteString(severityColor(v, thresh))
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
