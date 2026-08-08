package widgets

import (
	"math"
	"strings"
	"testing"

	"github.com/toddwshaffer/coolant/thermal/internal/theme"
)

// ── SeverityColor (via theme) ─────────────────────────────────

func TestSeverityColorNilThresh(t *testing.T) {
	th := testTheme
	got := th.SeverityColor(50, nil)
	wantGreen := th.SeverityColor(0, nil)
	if got != wantGreen {
		t.Errorf("SeverityColor with nil thresh = %q, want green %q", got, wantGreen)
	}
}

func TestSeverityColorBelowWarn(t *testing.T) {
	th := testTheme
	thresh := &theme.SparkThresholds{Warn: 70, Crit: 90}
	got := th.SeverityColor(30, thresh)
	green := th.SeverityColor(0, nil)
	red := th.SeverityColor(100, thresh)
	if got == red {
		t.Error("SeverityColor(30, {70,90}) should not be red")
	}
	if got == green {
		t.Error("SeverityColor(30, {70,90}) should not be pure green")
	}
	if got == "" {
		t.Error("SeverityColor(30, {70,90}) returned empty string")
	}
}

func TestSeverityColorAboveCrit(t *testing.T) {
	th := testTheme
	thresh := &theme.SparkThresholds{Warn: 70, Crit: 90}
	got := th.SeverityColor(95, thresh)
	red := th.SeverityColor(100, thresh)
	if got != red {
		t.Errorf("SeverityColor(95, {70,90}) = %q, want red %q", got, red)
	}
}

func TestSeverityColorBetweenWarnAndCrit(t *testing.T) {
	th := testTheme
	thresh := &theme.SparkThresholds{Warn: 60, Crit: 80}
	got := th.SeverityColor(70, thresh)
	green := th.SeverityColor(0, nil)
	red := th.SeverityColor(100, thresh)
	if got == green {
		t.Error("SeverityColor(70, {60,80}) should not be green")
	}
	if got == red {
		t.Error("SeverityColor(70, {60,80}) should not be pure red (below crit)")
	}
	if got == "" {
		t.Error("SeverityColor(70, {60,80}) returned empty string")
	}
}

func TestSeverityColorZeroWarn(t *testing.T) {
	th := testTheme
	thresh := &theme.SparkThresholds{Warn: 0, Crit: 100}
	got := th.SeverityColor(-5, thresh)
	green := th.SeverityColor(0, nil)
	if got != green {
		t.Errorf("SeverityColor(-5, {0,100}) = %q, want green", got)
	}
}

// ── valueToLevel ──────────────────────────────────────────────

func TestValueToLevelZero(t *testing.T) {
	// A present 0% sample renders the baseline dot, not a blank cell — blank
	// is reserved for absent (NaN) samples. See plans/cpu-sparkline-gaps.md.
	if got := valueToLevel(0, 100, false); got != 1 {
		t.Errorf("valueToLevel(0, 100, false) = %d, want 1 (baseline dot)", got)
	}
}

func TestValueToLevelNegative(t *testing.T) {
	// Negatives (e.g. spring undershoot) clamp to the baseline dot, not blank.
	if got := valueToLevel(-5, 100, false); got != 1 {
		t.Errorf("valueToLevel(-5, 100, false) = %d, want 1 (baseline dot)", got)
	}
}

func TestValueToLevelNaN(t *testing.T) {
	// NaN is the single "absent / no data" sentinel — it alone renders blank.
	if got := valueToLevel(math.NaN(), 100, false); got != 0 {
		t.Errorf("valueToLevel(NaN, 100, false) = %d, want 0 (blank)", got)
	}
}

func TestValueToLevelSubFloor(t *testing.T) {
	// A present sub-1% sample sits at the baseline dot rather than vanishing —
	// height-suppression of jitter is kept (it's at the floor), the hole is gone.
	if got := valueToLevel(0.5, 100, false); got != 1 {
		t.Errorf("valueToLevel(0.5, 100, false) = %d, want 1 (baseline, not blank)", got)
	}
}

func TestValueToLevelSmallPositive(t *testing.T) {
	// Any present non-negative value maps to at least the baseline dot — there
	// is no percentage cutoff (the old 2% noise floor is gone).
	got := valueToLevel(2.5, 100, false)
	if got < 1 {
		t.Errorf("valueToLevel(2.5, 100, false) = %d, want >= 1", got)
	}
}

func TestValueToLevelFull(t *testing.T) {
	// peak == value → maxLevels (8)
	if got := valueToLevel(100, 100, false); got != maxLevels {
		t.Errorf("valueToLevel(100, 100, false) = %d, want %d", got, maxLevels)
	}
}

func TestValueToLevelClampAbovePeak(t *testing.T) {
	// value > peak → still capped at maxLevels
	if got := valueToLevel(200, 100, false); got != maxLevels {
		t.Errorf("valueToLevel(200, 100, false) = %d, want %d (clamped)", got, maxLevels)
	}
}

func TestValueToLevelProportional(t *testing.T) {
	// 50% of peak → level 4 (50% of 8)
	if got := valueToLevel(50, 100, false); got != 4 {
		t.Errorf("valueToLevel(50, 100, false) = %d, want 4", got)
	}
}

// TestValueToLevelLogScale pins the log1p height curve at representative
// inputs against a crit-anchored peak (4000). The whole point of log mode
// is that bursts spanning orders of magnitude get distinguishable heights
// instead of all clipping at the top — verify that 1, 100, 1000, 4000,
// 10000 all map to different (and monotonically increasing) levels.
//
// valueToLevel under logScale=true expects the caller to pre-transform
// peak via math.Log1p (per-render hoist optimization).
func TestValueToLevelLogScale(t *testing.T) {
	const rawPeak = 4000.0
	peak := math.Log1p(rawPeak)
	cases := []struct {
		v       float64
		wantMin int
		wantMax int
	}{
		{0, 1, 1},     // present zero → baseline dot (blank is NaN-only now)
		{0.5, 1, 1},   // below log floor → still a present sample → baseline dot
		{1, 1, 1},     // log1p(1)/log1p(4000) ≈ 8% → clamped up to 1
		{10, 2, 3},    // log1p(10)/log1p(4000) ≈ 29% → level 2
		{100, 4, 5},   // log1p(100)/log1p(4000) ≈ 56% → level 4
		{1000, 6, 7},  // log1p(1000)/log1p(4000) ≈ 83% → level 6
		{4000, 8, 8},  // crit: full height
		{10000, 8, 8}, // above crit: clamped
	}
	for _, tc := range cases {
		got := valueToLevel(tc.v, peak, true)
		if got < tc.wantMin || got > tc.wantMax {
			t.Errorf("valueToLevel(%v, log1p(%v), true) = %d, want %d..%d", tc.v, rawPeak, got, tc.wantMin, tc.wantMax)
		}
	}
}

// TestValueToLevelLogMonotonic verifies that log mode preserves ordering:
// for any v1 < v2 (both above the floor), level(v1) ≤ level(v2). Catches
// any regression where the transform inverts the curve.
func TestValueToLevelLogMonotonic(t *testing.T) {
	peak := math.Log1p(4000.0)
	values := []float64{1, 5, 25, 100, 500, 1500, 3000, 4000}
	prev := 0
	for _, v := range values {
		got := valueToLevel(v, peak, true)
		if got < prev {
			t.Errorf("log scale not monotonic: v=%v → %d but prior value gave %d", v, got, prev)
		}
		prev = got
	}
}

// ── levelSplit ────────────────────────────────────────────────

func TestLevelSplit(t *testing.T) {
	tests := []struct {
		level   int
		wantBot int
		wantTop int
	}{
		{0, 0, 0},
		{1, 1, 0},
		{2, 2, 0},
		{3, 3, 0},
		{4, 4, 0},
		{5, 4, 1},
		{6, 4, 2},
		{7, 4, 3},
		{8, 4, 4},
	}
	for _, tt := range tests {
		bot, top := levelSplit(tt.level)
		if bot != tt.wantBot || top != tt.wantTop {
			t.Errorf("levelSplit(%d) = (%d, %d), want (%d, %d)",
				tt.level, bot, top, tt.wantBot, tt.wantTop)
		}
	}
}

// ── prepareSparkDataBuf ───────────────────────────────────────

func TestPrepareSparkDataBufPadsShort(t *testing.T) {
	data := []float64{42}
	width := 5
	buf := NewSparkBufs(width)
	got := prepareSparkDataBuf(data, width, buf)
	if len(got) != width*2 {
		t.Errorf("len = %d, want %d", len(got), width*2)
	}
}

func TestPrepareSparkDataBufTruncatesLong(t *testing.T) {
	// This is the critical panic-fix test: data longer than width+1
	// must be truncated before interpolation, not cause an index panic.
	width := 5
	buf := NewSparkBufs(width)
	data := make([]float64, 100) // far more than width+1=6
	for i := range data {
		data[i] = float64(i)
	}
	got := prepareSparkDataBuf(data, width, buf)
	if len(got) != width*2 {
		t.Errorf("len = %d, want %d", len(got), width*2)
	}
	// Most recent value (99) should be the last element.
	if got[len(got)-1] != 99 {
		t.Errorf("last element = %f, want 99", got[len(got)-1])
	}
}

func TestPrepareSparkDataBufTruncatesExactlyWidthPlus2(t *testing.T) {
	// Edge case: exactly one more than minRaw (width+1).
	width := 5
	buf := NewSparkBufs(width)
	data := make([]float64, width+2)
	for i := range data {
		data[i] = float64(i + 1)
	}
	got := prepareSparkDataBuf(data, width, buf)
	if len(got) != width*2 {
		t.Errorf("len = %d, want %d", len(got), width*2)
	}
}

// ── prepareSparkMaskBuf ───────────────────────────────────────

func TestPrepareSparkMaskBufTruncatesLong(t *testing.T) {
	width := 5
	buf := NewSparkBufs(width)
	mask := make([]bool, 100)
	for i := range mask {
		mask[i] = true
	}
	mask[99] = false // last entry offline
	got := prepareSparkMaskBuf(mask, width, buf)
	if len(got) != width*2 {
		t.Errorf("len = %d, want %d", len(got), width*2)
	}
	// Last element should be false (the offline marker).
	if got[len(got)-1] != false {
		t.Errorf("last mask element = %v, want false", got[len(got)-1])
	}
}

func TestPrepareSparkMaskBufPadsFalseAsOnline(t *testing.T) {
	// Padded entries should be true (online) so they render as empty braille.
	width := 5
	buf := NewSparkBufs(width)
	mask := []bool{false} // single offline entry
	got := prepareSparkMaskBuf(mask, width, buf)
	if len(got) != width*2 {
		t.Errorf("len = %d, want %d", len(got), width*2)
	}
	// First entries are padding — should be true (online).
	if !got[0] {
		t.Errorf("padded mask[0] = false, want true (online padding)")
	}
}

// ── SparkBufs resize on width growth ──────────────────────────

func TestSparkBufsWidthGrowthNoPanic(t *testing.T) {
	// Simulate terminal resize: buffer allocated at small width, then used at larger width.
	// This is the exact scenario that caused the [:471] with capacity 401 panic.
	smallWidth := 80
	buf := NewSparkBufs(smallWidth)

	data := make([]float64, 300)
	for i := range data {
		data[i] = float64(i)
	}

	largeWidth := 235 // typical wide terminal after font size decrease
	// This must not panic
	got := prepareSparkDataBuf(data, largeWidth, buf)
	if len(got) != largeWidth*2 {
		t.Errorf("len = %d, want %d", len(got), largeWidth*2)
	}
}

func TestSparkBufsMaskWidthGrowthNoPanic(t *testing.T) {
	smallWidth := 80
	buf := NewSparkBufs(smallWidth)

	mask := make([]bool, 300)
	for i := range mask {
		mask[i] = true
	}

	largeWidth := 235
	got := prepareSparkMaskBuf(mask, largeWidth, buf)
	if len(got) != largeWidth*2 {
		t.Errorf("len = %d, want %d", len(got), largeWidth*2)
	}
}

// ── RenderSparkline ────────────────────────────────

func TestRenderSparklineOfflineRainbow(t *testing.T) {
	width := 10
	buf := NewSparkBufs(width)
	data := make([]float64, width+1)
	online := make([]bool, width+1)
	// All offline (false).
	for i := range data {
		data[i] = 50
	}
	pair := RenderSparkline(data, online, width, 100, nil, 0, buf, testTheme, false)

	// Bottom row should contain rainbow ANSI escape sequences (e.g., \033[31m).
	if !strings.Contains(pair.Bottom, "\033[3") {
		t.Errorf("offline sparkline Bottom missing rainbow ANSI escapes: %q", pair.Bottom)
	}
}

func TestRenderSparklineAllOnline(t *testing.T) {
	width := 10
	buf := NewSparkBufs(width)
	data := make([]float64, width+1)
	online := make([]bool, width+1)
	for i := range data {
		data[i] = float64(i * 10)
		online[i] = true
	}
	pair := RenderSparkline(data, online, width, 100, nil, 0, buf, testTheme, false)
	if len(pair.Bottom) == 0 {
		t.Error("all-online sparkline Bottom is empty")
	}
}

func TestRenderSparklineWidthZero(t *testing.T) {
	buf := NewSparkBufs(1) // minimal buf
	pair := RenderSparkline([]float64{1, 2}, []bool{true, true}, 0, 100, nil, 0, buf, testTheme, false)
	if pair.Top != "" || pair.Bottom != "" {
		t.Errorf("width=0 should produce empty pair, got Top=%q Bottom=%q", pair.Top, pair.Bottom)
	}
}

// TestRenderSparklinePresentZeroBaseline is the core regression for
// plans/cpu-sparkline-gaps.md: a full buffer of present 0% samples must render
// a solid baseline of braille dots, NOT a row of blank cells. The old behavior
// (value <= floor → level 0 → space) produced the "gaps flowing through".
func TestRenderSparklinePresentZeroBaseline(t *testing.T) {
	width := 10
	buf := NewSparkBufs(width)
	data := make([]float64, width+1) // all 0.0 — fully idle CPU
	online := make([]bool, width+1)
	for i := range online {
		online[i] = true
	}
	pair := RenderSparkline(data, online, width, 100, nil, 0, buf, testTheme, false)
	if strings.ContainsRune(pair.Bottom, ' ') {
		t.Errorf("present-zero sparkline has blank cells (gap) in Bottom: %q", pair.Bottom)
	}
}

// TestRenderSparklinePaddingBlank guards the other side: a not-yet-full buffer
// (startup) must keep its leading padding cells BLANK, not draw a false
// baseline. This is what the NaN padding sentinel buys — distinguishing "no
// data yet" from a real 0% reading.
func TestRenderSparklinePaddingBlank(t *testing.T) {
	width := 10
	buf := NewSparkBufs(width)
	data := []float64{42} // single real sample; the rest is padding
	online := []bool{true}
	pair := RenderSparkline(data, online, width, 100, nil, 0, buf, testTheme, false)
	first := []rune(pair.Bottom)
	if len(first) == 0 || first[0] != ' ' {
		t.Errorf("padding region should render blank; Bottom starts non-blank: %q", pair.Bottom)
	}
}

func TestPrepareSparkDataBufPadsNaN(t *testing.T) {
	// Padding must be NaN (the absent sentinel), not 0 — a 0 would render as a
	// baseline dot and produce a false leading line at startup.
	data := []float64{42}
	width := 5
	buf := NewSparkBufs(width)
	got := prepareSparkDataBuf(data, width, buf)
	if !math.IsNaN(got[0]) {
		t.Errorf("padded leading cell = %v, want NaN", got[0])
	}
}
