package widgets

import (
	"fmt"
	"image/color"
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

// Edge fade: rightmost N characters fade in, leftmost N fade out.
const fadeChars = 3

// Brightness ramp per character from edge inward.
// Index 0 = outermost char (dimmest), last = transition to full brightness.
var fadeRamp = [fadeChars]float64{0.35, 0.60, 0.82}

// edgeFade returns the brightness multiplier for a character at charIdx
// in a sparkline of numChars width. Returns 1.0 for characters in the
// middle, <1.0 for characters near either edge.
func edgeFade(charIdx, numChars int) float64 {
	// Left edge fade-out
	if charIdx < fadeChars {
		return fadeRamp[charIdx]
	}
	// Right edge fade-in
	fromRight := numChars - 1 - charIdx
	if fromRight < fadeChars {
		return fadeRamp[fromRight]
	}
	return 1.0
}

// ── Motion trail: comet-tail when a row ends ────────────────
const trailChars = 8 // how many characters the trail extends

// Brightness per trail character (index 0 = right after last real dot).
// Floor at 0.25 — never darker than a typical dark terminal background.
var trailRamp = [trailChars]float64{0.90, 0.75, 0.60, 0.48, 0.40, 0.34, 0.28, 0.25}

// trailState tracks the comet trail for one braille row (top or bottom).
type trailState struct {
	lastBits  rune
	lastColor colorful.Color
	age       int
}

// renderTrailedChar writes one braille character with comet-trail logic.
// Same or growing pattern → full brightness. Shrinking pattern → trail
// the previous (taller) pattern until it fades, then show current.
func renderTrailedChar(sb *strings.Builder, ts *trailState, bits rune, color colorful.Color, edgeAlpha float64) {
	isSubset := ts.lastBits != 0 && bits != ts.lastBits && bits&^ts.lastBits == 0

	if isSubset && ts.age < trailChars {
		// Pattern shrank (or went empty) — trail the original pattern
		alpha := trailRamp[ts.age] * edgeAlpha
		sb.WriteString(dimmedFg(ts.lastColor, alpha))
		sb.WriteRune(0x2800 | ts.lastBits)
		sb.WriteString(sparkReset)
		ts.age++
	} else if bits != 0 {
		// Has dots, not shrinking (or trail expired) — full brightness
		ts.lastBits = bits
		ts.lastColor = color
		ts.age = 0
		if edgeAlpha < 1.0 {
			sb.WriteString(dimmedFg(color, edgeAlpha))
		} else {
			sb.WriteString(truecolorFg(color))
		}
		sb.WriteRune(0x2800 | bits)
		sb.WriteString(sparkReset)
	} else {
		// Empty, no trail
		ts.lastBits = 0
		sb.WriteRune(' ')
	}
}

// rainbowEntry holds a pre-rendered offline rainbow character.
type rainbowEntry struct {
	ch    rune
	color string
}

// severityColorful returns the perceptual gradient color for a value.
// Green→yellow below warn, yellow→red between warn and crit, red above crit.
func severityColorful(v float64, thresh *SparkThresholds) colorful.Color {
	if thresh == nil {
		return gradGreen
	}
	switch {
	case v >= thresh.Crit:
		return gradRed
	case v >= thresh.Warn:
		ratio := (v - thresh.Warn) / (thresh.Crit - thresh.Warn)
		return gradYellow.BlendHcl(gradRed, ratio).Clamped()
	default:
		if thresh.Warn <= 0 {
			return gradGreen
		}
		ratio := v / thresh.Warn
		return gradGreen.BlendHcl(gradYellow, ratio).Clamped()
	}
}

// severityColor returns a truecolor ANSI escape for a value relative to
// thresholds. Interpolates green→yellow below warn, yellow→red between
// warn and crit, solid red above crit. Produces smooth per-dot gradients.
func severityColor(v float64, thresh *SparkThresholds) string {
	return truecolorFg(severityColorful(v, thresh))
}

// dimmedFg blends a color toward black by alpha (0=black, 1=full brightness)
// and returns a truecolor ANSI foreground escape.
func dimmedFg(c colorful.Color, alpha float64) string {
	black, _ := colorful.MakeColor(color.Black)
	dimmed := black.BlendLab(c, alpha).Clamped()
	return truecolorFg(dimmed)
}

// truecolorFg emits \033[38;2;R;G;Bm for 24-bit foreground color.
func truecolorFg(c colorful.Color) string {
	r, g, b := c.RGB255()
	return fmt.Sprintf("\033[38;2;%d;%d;%dm", r, g, b)
}

// Braille dot layout (2 columns × 4 rows):
//
//	Left  Right
//	 1     4    (top)
//	 2     5
//	 3     6
//	 7     8    (bottom)
//
// Each column encodes one sample, bottom-up. Two samples per character
// doubles horizontal resolution — ~240 samples across a full terminal
// instead of ~120.

// Left column bit patterns for levels 0–4 (bottom-up).
var leftBits = [5]rune{
	0x00,                      // 0: empty
	0x40,                      // 1: dot 7
	0x40 | 0x04,               // 2: dots 7+3
	0x40 | 0x04 | 0x02,        // 3: dots 7+3+2
	0x40 | 0x04 | 0x02 | 0x01, // 4: dots 7+3+2+1 (full)
}

// Right column bit patterns for levels 0–4 (bottom-up).
var rightBits = [5]rune{
	0x00,                      // 0: empty
	0x80,                      // 1: dot 8
	0x80 | 0x20,               // 2: dots 8+6
	0x80 | 0x20 | 0x10,        // 3: dots 8+6+5
	0x80 | 0x20 | 0x10 | 0x08, // 4: dots 8+6+5+4 (full)
}

// maxLevels is the number of filled braille levels per column (0–8).
// Two stacked braille characters give 8 vertical dots: bottom char (1–4),
// top char (5–8). This yields ~12.5% granularity with a fixed max of 100.
const maxLevels = 8

// levelSplit splits a 0–8 level into bottom (0–4) and top (0–4) braille levels.
func levelSplit(level int) (bottom, top int) {
	if level <= 4 {
		return level, 0
	}
	return 4, level - 4
}

// valueToLevel maps a value to a braille height level (0–8).
// Values below 2% of peak render as 0 (invisible noise floor).
// Values between 2–5% render as level 1 (faint dot, visible with dim color).
func valueToLevel(v, peak float64) int {
	if v <= 0 || v < peak*0.02 {
		return 0
	}
	level := int((v / peak) * float64(maxLevels))
	if level > maxLevels {
		level = maxLevels
	}
	if level < 1 {
		level = 1
	}
	return level
}

// interpolateData inserts linear midpoints between consecutive samples,
// doubling resolution. Turns step-function transitions into visible ramps.
// Result length: 2*len(data) - 1 (or original for len < 2).
func interpolateData(data []float64) []float64 {
	if len(data) < 2 {
		return data
	}
	out := make([]float64, 2*len(data)-1)
	for i, v := range data {
		out[i*2] = v
		if i > 0 {
			out[i*2-1] = (data[i-1] + v) / 2
		}
	}
	return out
}

// interpolateMask expands each bool to 2 entries (real + midpoint copy).
// Midpoints inherit the state of the left (older) neighbor.
func interpolateMask(mask []bool) []bool {
	if len(mask) < 2 {
		return mask
	}
	out := make([]bool, 2*len(mask)-1)
	for i, v := range mask {
		out[i*2] = v
		if i > 0 {
			out[i*2-1] = mask[i-1]
		}
	}
	return out
}

// prepareSparkData pads raw data to fill the sparkline width, interpolates
// midpoints for smooth ramps, and returns the visible window of width*2 samples.
func prepareSparkData(data []float64, width int) []float64 {
	minRaw := width + 1
	if len(data) < minRaw {
		padded := make([]float64, minRaw)
		copy(padded[minRaw-len(data):], data)
		data = padded
	}
	interp := interpolateData(data)
	need := width * 2
	if len(interp) > need {
		interp = interp[len(interp)-need:]
	}
	return interp
}

// prepareSparkMask pads and interpolates an online/offline mask in lockstep
// with prepareSparkData. Padding entries are marked online (zero-value empty braille).
func prepareSparkMask(mask []bool, width int) []bool {
	minRaw := width + 1
	if len(mask) < minRaw {
		padded := make([]bool, minRaw)
		for i := 0; i < minRaw-len(mask); i++ {
			padded[i] = true
		}
		copy(padded[minRaw-len(mask):], mask)
		mask = padded
	}
	interp := interpolateMask(mask)
	need := width * 2
	if len(interp) > need {
		interp = interp[len(interp)-need:]
	}
	return interp
}

// SparkPair holds the top and bottom rows of a 2-row stacked sparkline.
type SparkPair struct {
	Top    string
	Bottom string
}

// RenderSparkline renders a 2-row double-resolution braille sparkline. Each
// character packs two samples (left + right columns), and two vertically
// stacked characters give 8 levels per column (~12.5% granularity at max 100).
// HEIGHT is proportional, COLOR encodes severity.
// Edge fades dim the outermost characters for smooth data entry and exit.
func RenderSparkline(data []float64, width int, maxOverride float64, thresh *SparkThresholds) SparkPair {
	if width <= 0 {
		return SparkPair{}
	}

	visible := prepareSparkData(data, width)

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

	numChars := (len(visible) + 1) / 2
	var top, bot strings.Builder

	// Trail state per row
	var topTrail, botTrail trailState

	for i := 0; i < len(visible); i += 2 {
		charIdx := i / 2

		vL := visible[i]
		if vL < 0 {
			vL = 0
		}
		levL := valueToLevel(vL, peak)

		var vR float64
		var levR int
		if i+1 < len(visible) {
			vR = visible[i+1]
			if vR < 0 {
				vR = 0
			}
			levR = valueToLevel(vR, peak)
		}

		realBotL, realTopL := levelSplit(levL)
		realBotR, realTopR := levelSplit(levR)

		edgeAlpha := edgeFade(charIdx, numChars)
		colorVal := vL
		if vR > colorVal {
			colorVal = vR
		}
		color := severityColorful(colorVal, thresh)

		topBits := leftBits[realTopL] | rightBits[realTopR]
		botBits := leftBits[realBotL] | rightBits[realBotR]

		renderTrailedChar(&bot, &botTrail, botBits, color, edgeAlpha)
		renderTrailedChar(&top, &topTrail, topBits, color, edgeAlpha)
	}

	return SparkPair{Top: top.String(), Bottom: bot.String()}
}

// RenderSparklineWithMask renders a 2-row sparkline where offline ticks become
// rainbow dots. Two samples per character, two stacked characters per column.
// Edge fades dim the outermost characters for smooth data entry and exit.
func RenderSparklineWithMask(data []float64, online []bool, width int, maxOverride float64, thresh *SparkThresholds, tick int) SparkPair {
	if width <= 0 {
		return SparkPair{}
	}

	// Align data and mask to same length (use shorter)
	n := len(data)
	if len(online) < n {
		n = len(online)
	}
	if n == 0 {
		return RenderSparkline(data, width, maxOverride, thresh)
	}

	// Trim to aligned length, then pad+interpolate+window via shared helpers
	alignedData := data[len(data)-n:]
	alignedMask := online[len(online)-n:]
	visibleData := prepareSparkData(alignedData, width)
	visibleMask := prepareSparkMask(alignedMask, width)

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

	numChars := (len(visibleData) + 1) / 2
	var top, bot strings.Builder

	// Trail state per row
	var topTrail, botTrail trailState

	for i := 0; i < len(visibleData); i += 2 {
		charIdx := i / 2

		// Left sample
		vL := visibleData[i]
		onL := true
		if i < len(visibleMask) {
			onL = visibleMask[i]
		}

		// Right sample (may not exist)
		var vR float64
		onR := true
		hasRight := i+1 < len(visibleData)
		if hasRight {
			vR = visibleData[i+1]
			if i+1 < len(visibleMask) {
				onR = visibleMask[i+1]
			}
		}

		// Both offline → rainbow character (bottom only, top blank)
		if !onL && (!hasRight || !onR) {
			ch, color := rainbowChar(charIdx)
			bot.WriteString(color)
			bot.WriteRune(ch)
			bot.WriteString(sparkReset)
			top.WriteRune(' ')
			// Kill trails across offline gaps
			topTrail = trailState{}
			botTrail = trailState{}
			continue
		}

		// Mixed online/offline or both online
		colorVal := 0.0
		var levL, levR int

		if onL {
			if vL < 0 {
				vL = 0
			}
			levL = valueToLevel(vL, peak)
			colorVal = vL
		}

		if hasRight && onR {
			if vR < 0 {
				vR = 0
			}
			levR = valueToLevel(vR, peak)
			if vR > colorVal {
				colorVal = vR
			}
		}

		// Offline side gets a filler pattern (bottom row only)
		var offlineLBits, offlineRBits rune
		if !onL && hasRight && onR {
			offlineLBits = leftBits[1+int(rune(i/2)*5)%3]
		}
		if hasRight && !onR {
			offlineRBits = rightBits[1+int(rune(i/2)*3)%3]
		}

		realBotL, realTopL := levelSplit(levL)
		realBotR, realTopR := levelSplit(levR)

		edgeAlpha := edgeFade(charIdx, numChars)
		color := severityColorful(colorVal, thresh)

		topBits := leftBits[realTopL] | rightBits[realTopR]
		botBits := leftBits[realBotL] | rightBits[realBotR] | offlineLBits | offlineRBits

		renderTrailedChar(&bot, &botTrail, botBits, color, edgeAlpha)
		renderTrailedChar(&top, &topTrail, topBits, color, edgeAlpha)
	}

	return SparkPair{Top: top.String(), Bottom: bot.String()}
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
