// Package widgets provides the individual UI components for the thermal
// dashboard: sparklines, gauges, headline bar, rates, alerts, and animations.
package widgets

import (
	"math"
	"strings"

	"github.com/toddwshaffer/coolant/thermal/internal/config"
	"github.com/toddwshaffer/coolant/thermal/internal/theme"
)

// ANSI reset and dim (still needed for offline/special states).
const (
	sparkReset = "\033[0m"
)

// renderBrailleChar writes one braille character with a pre-computed ANSI color,
// or a space if the pattern is empty.
func renderBrailleChar(sb *strings.Builder, bits rune, ansiColor string) {
	if bits != 0 {
		sb.WriteString(ansiColor)
		sb.WriteRune(0x2800 | bits)
		sb.WriteString(sparkReset)
	} else {
		sb.WriteRune(' ')
	}
}

// rainbowEntry holds a pre-rendered offline rainbow character.
type rainbowEntry struct {
	ch    rune
	color string
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

// valueToLevel maps a value to a braille height level (1–8) for any present
// sample, or 0 (blank) for an absent one. peak is the height-ratio denominator;
// when logScale=true, the CALLER must pass math.Log1p(rawPeak) so the per-column
// loop doesn't redo the invariant transform on every column.
//
// Level 0 — a blank cell — is reserved for an ABSENT sample, signalled by NaN
// (the padding sentinel for a not-yet-full buffer). A present sample, even a
// genuine 0 or a tiny fraction of peak, floors to level 1 (the baseline dot):
// it's real data, not a hole. This is what stopped the "gaps flowing through"
// the CPU sparkline — a genuinely-idle 0% reading is the baseline, not a missing
// cell. See plans/cpu-sparkline-gaps.md.
func valueToLevel(v, peak float64, logScale bool) int {
	if math.IsNaN(v) {
		return 0 // absent sample → blank cell
	}
	if v < 0 {
		v = 0 // spring undershoot etc. → baseline, not blank
	}
	if logScale {
		v = math.Log1p(v)
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

// SparkPair holds the top and bottom rows of a 2-row stacked sparkline.
type SparkPair struct {
	Top    string
	Bottom string
}

// Sparkline threshold accessors — read from config.C at call time so
// user config overrides take effect (Load runs after package init).
func CPUSparkThresh() theme.SparkThresholds {
	return theme.SparkThresholds{Warn: config.C.Sparklines.CPUWarn, Crit: config.C.Sparklines.CPUCrit}
}
func MemSparkThresh() theme.SparkThresholds {
	return theme.SparkThresholds{Warn: config.C.Sparklines.MemWarn, Crit: config.C.Sparklines.MemCrit}
}
func DecompSparkThresh() theme.SparkThresholds {
	return theme.SparkThresholds{Warn: config.C.Sparklines.DecompWarn, Crit: config.C.Sparklines.DecompCrit}
}
func SwapSparkThresh() theme.SparkThresholds {
	return theme.SparkThresholds{Warn: config.C.Sparklines.SwapWarnGB, Crit: config.C.Sparklines.SwapCritGB}
}
func GPUSparkThresh() theme.SparkThresholds {
	return theme.SparkThresholds{Warn: config.C.Sparklines.GPUWarn, Crit: config.C.Sparklines.GPUCrit}
}
func TokenSparkThresh() theme.SparkThresholds {
	return theme.SparkThresholds{Warn: config.C.Sparklines.TokenWarn, Crit: config.C.Sparklines.TokenCrit}
}

// SparkBufs holds reusable interpolation buffers to avoid per-frame allocations.
// Allocate once via NewSparkBufs and pass to RenderSparkline.
type SparkBufs struct {
	interpData []float64 // reused by prepareSparkDataBuf
	interpMask []bool    // reused by prepareSparkMaskBuf
	padData    []float64 // reused for padding in prepareSparkDataBuf
	padMask    []bool    // reused for padding in prepareSparkMaskBuf
	lastWidth  int       // cached width to skip redundant grow checks
}

// NewSparkBufs creates reusable buffers sized for the given sparkline width.
func NewSparkBufs(maxWidth int) *SparkBufs {
	b := &SparkBufs{}
	b.grow(maxWidth)
	return b
}

// grow reallocates buffers if width exceeds current capacity.
func (b *SparkBufs) grow(width int) {
	if width <= b.lastWidth {
		return
	}
	b.lastWidth = width
	interpCap := 2*(width+1) - 1
	padCap := width + 1
	if cap(b.interpData) < interpCap {
		b.interpData = make([]float64, 0, interpCap)
		b.interpMask = make([]bool, 0, interpCap)
	}
	if cap(b.padData) < padCap {
		b.padData = make([]float64, 0, padCap)
		b.padMask = make([]bool, 0, padCap)
	}
}

// prepareSparkDataBuf is like prepareSparkData but reuses buf.interpData and
// buf.padData to avoid allocations. The returned slice is only valid until the
// next call.
func prepareSparkDataBuf(data []float64, width int, buf *SparkBufs) []float64 {
	buf.grow(width)
	minRaw := width + 1
	src := data
	if len(data) < minRaw {
		padded := buf.padData[:minRaw]
		gap := minRaw - len(data)
		for i := 0; i < gap; i++ {
			padded[i] = math.NaN() // absent sentinel — renders blank, never baseline
		}
		copy(padded[gap:], data)
		src = padded
	} else if len(data) > minRaw {
		// Truncate to the most recent minRaw elements before interpolating
		src = data[len(data)-minRaw:]
	}

	// Interpolate into buf.interpData
	if len(src) < 2 {
		return src
	}
	outLen := 2*len(src) - 1
	interp := buf.interpData[:outLen]
	for i, v := range src {
		interp[i*2] = v
		if i > 0 {
			interp[i*2-1] = (src[i-1] + v) / 2
		}
	}

	need := width * 2
	if len(interp) > need {
		interp = interp[len(interp)-need:]
	}
	return interp
}

// prepareSparkMaskBuf is like prepareSparkMask but reuses buf.interpMask and
// buf.padMask to avoid allocations. The returned slice is only valid until the
// next call.
func prepareSparkMaskBuf(mask []bool, width int, buf *SparkBufs) []bool {
	buf.grow(width)
	minRaw := width + 1
	src := mask
	if len(mask) < minRaw {
		padded := buf.padMask[:minRaw]
		gap := minRaw - len(mask)
		for i := 0; i < gap; i++ {
			padded[i] = true
		}
		copy(padded[gap:], mask)
		src = padded
	} else if len(mask) > minRaw {
		src = mask[len(mask)-minRaw:]
	}

	// Interpolate into buf.interpMask
	if len(src) < 2 {
		return src
	}
	outLen := 2*len(src) - 1
	interp := buf.interpMask[:outLen]
	for i, v := range src {
		interp[i*2] = v
		if i > 0 {
			interp[i*2-1] = src[i-1]
		}
	}

	need := width * 2
	if len(interp) > need {
		interp = interp[len(interp)-need:]
	}
	return interp
}

// RenderSparkline renders a 2-row sparkline where offline ticks become
// rainbow dots, reusing pre-allocated interpolation buffers to avoid per-frame
// allocations. Two samples per character, two stacked characters per column.
// When dimmed is true, colors render via the theme's dim LUTs so the sparkline
// recedes behind an overlay but stays visible as live motion.
func RenderSparkline(data []float64, online []bool, width int, maxOverride float64, thresh *theme.SparkThresholds, tick int, buf *SparkBufs, th *theme.Theme, dimmed bool) SparkPair {
	return renderSparklineCore(data, online, width, maxOverride, thresh, tick, buf, th, dimmed, false)
}

// RenderSparklineLog renders with log1p height scaling — values spanning
// multiple orders of magnitude stay distinguishable by height rather than
// all clipping at the top. Color computation remains on raw values so
// severity thresholds keep their absolute meaning (warn at warn, crit at
// crit). Useful for signals like token throughput where a typical burst
// (~500 io/s) and a heavy burst (~5000 io/s) both need to read as
// "meaningful but distinguishable" rather than both pinning at full height.
func RenderSparklineLog(data []float64, online []bool, width int, maxOverride float64, thresh *theme.SparkThresholds, tick int, buf *SparkBufs, th *theme.Theme, dimmed bool) SparkPair {
	return renderSparklineCore(data, online, width, maxOverride, thresh, tick, buf, th, dimmed, true)
}

// renderSparklineCore is the implementation for sparkline rendering.
// When mask is nil, all samples are treated as online (no rainbow/offline logic).
// buf must be non-nil — callers allocate once via NewSparkBufs and reuse.
// logScale controls whether per-column heights use a log1p transform;
// colorVal stays raw so severity thresholds fire at absolute values.
func renderSparklineCore(data []float64, mask []bool, width int, maxOverride float64, thresh *theme.SparkThresholds, tick int, buf *SparkBufs, th *theme.Theme, dimmed, logScale bool) SparkPair {
	if width <= 0 {
		return SparkPair{}
	}

	hasMask := mask != nil

	// Align data and mask to same length when mask is present
	alignedData := data
	alignedMask := mask
	if hasMask {
		n := len(data)
		if len(mask) < n {
			n = len(mask)
		}
		if n == 0 {
			hasMask = false
			alignedData = data
		} else {
			alignedData = data[len(data)-n:]
			alignedMask = mask[len(mask)-n:]
		}
	}

	// Prepare visible data (and mask) via pad+interpolate+window
	visibleData := prepareSparkDataBuf(alignedData, width, buf)
	var visibleMask []bool
	if hasMask {
		visibleMask = prepareSparkMaskBuf(alignedMask, width, buf)
	}

	// Find peak for auto-scaling
	peak := maxOverride
	if peak <= 0 {
		for i, v := range visibleData {
			// When masked, only count online values for peak detection
			if hasMask && i < len(visibleMask) && !visibleMask[i] {
				continue
			}
			if v > peak {
				peak = v
			}
		}
	}
	if peak <= 0 {
		peak = 1
	}

	// Hoist log1p(peak) out of the per-column loop — peak is invariant
	// within a single render, no point recomputing it for every column.
	// valueToLevel's contract under logScale=true expects this pre-transform.
	peakForRatio := peak
	if logScale {
		peakForRatio = math.Log1p(peak)
	}

	var top, bot strings.Builder

	for i := 0; i < len(visibleData); i += 2 {
		charIdx := i / 2

		// Left sample
		vL := visibleData[i]
		onL := true
		if hasMask && i < len(visibleMask) {
			onL = visibleMask[i]
		}

		// Right sample (may not exist)
		var vR float64
		onR := true
		hasRight := i+1 < len(visibleData)
		if hasRight {
			vR = visibleData[i+1]
			if hasMask && i+1 < len(visibleMask) {
				onR = visibleMask[i+1]
			}
		}

		// Both offline → rainbow character (bottom only, top blank)
		if hasMask && !onL && (!hasRight || !onR) {
			ch, color := rainbowChar(charIdx, th.OfflineSparkANSI)
			bot.WriteString(color)
			bot.WriteRune(ch)
			bot.WriteString(sparkReset)
			top.WriteRune(' ')
			continue
		}

		// Compute levels for online samples
		colorVal := 0.0
		var levL, levR int

		// NaN = absent sample (padding). valueToLevel already maps NaN→0, so the
		// skipped block leaves levL/levR at 0 (blank) regardless; the guard here
		// earns its keep by keeping NaN out of colorVal and the severity LUT.
		// Don't remove either NaN check thinking it duplicates the other.
		if onL && !math.IsNaN(vL) {
			if vL < 0 {
				vL = 0
			}
			levL = valueToLevel(vL, peakForRatio, logScale)
			colorVal = vL
		}

		if hasRight && onR && !math.IsNaN(vR) {
			if vR < 0 {
				vR = 0
			}
			levR = valueToLevel(vR, peakForRatio, logScale)
			if vR > colorVal {
				colorVal = vR
			}
		}

		// Offline side gets a filler pattern (bottom row only)
		var offlineLBits, offlineRBits rune
		if hasMask {
			if !onL && hasRight && onR {
				offlineLBits = leftBits[1+int(rune(charIdx)*5)%3]
			}
			if hasRight && !onR {
				offlineRBits = rightBits[1+int(rune(charIdx)*3)%3]
			}
		}

		realBotL, realTopL := levelSplit(levL)
		realBotR, realTopR := levelSplit(levR)

		var ansi string
		if dimmed {
			ansi = th.SeverityColorDimmed(colorVal, thresh)
		} else {
			ansi = th.SeverityColor(colorVal, thresh)
		}

		topBits := leftBits[realTopL] | rightBits[realTopR]
		botBits := leftBits[realBotL] | rightBits[realBotR] | offlineLBits | offlineRBits

		renderBrailleChar(&bot, botBits, ansi)
		renderBrailleChar(&top, topBits, ansi)
	}

	return SparkPair{Top: top.String(), Bottom: bot.String()}
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
func rainbowChar(i int, offlineColors []string) (rune, string) {
	patIdx := (i*7 + i*i*3) % len(funBraille)
	colorIdx := (i*3 + i*i) % len(offlineColors)
	return funBraille[patIdx], offlineColors[colorIdx]
}
