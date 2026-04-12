package widgets

import (
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
	if got := valueToLevel(0, 100); got != 0 {
		t.Errorf("valueToLevel(0, 100) = %d, want 0", got)
	}
}

func TestValueToLevelNegative(t *testing.T) {
	if got := valueToLevel(-5, 100); got != 0 {
		t.Errorf("valueToLevel(-5, 100) = %d, want 0", got)
	}
}

func TestValueToLevelNoiseFloor(t *testing.T) {
	// 1.9% of 100 = 1.9, below 2% threshold → 0
	if got := valueToLevel(1.9, 100); got != 0 {
		t.Errorf("valueToLevel(1.9, 100) = %d, want 0 (noise floor)", got)
	}
}

func TestValueToLevelAboveNoiseFloor(t *testing.T) {
	// 2.5% of 100 = 2.5, above 2% threshold → at least 1
	got := valueToLevel(2.5, 100)
	if got < 1 {
		t.Errorf("valueToLevel(2.5, 100) = %d, want >= 1", got)
	}
}

func TestValueToLevelFull(t *testing.T) {
	// peak == value → maxLevels (8)
	if got := valueToLevel(100, 100); got != maxLevels {
		t.Errorf("valueToLevel(100, 100) = %d, want %d", got, maxLevels)
	}
}

func TestValueToLevelClampAbovePeak(t *testing.T) {
	// value > peak → still capped at maxLevels
	if got := valueToLevel(200, 100); got != maxLevels {
		t.Errorf("valueToLevel(200, 100) = %d, want %d (clamped)", got, maxLevels)
	}
}

func TestValueToLevelProportional(t *testing.T) {
	// 50% of peak → level 4 (50% of 8)
	if got := valueToLevel(50, 100); got != 4 {
		t.Errorf("valueToLevel(50, 100) = %d, want 4", got)
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
