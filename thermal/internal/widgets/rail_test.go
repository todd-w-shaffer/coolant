package widgets

import (
	"image/color"
	"testing"
	"time"

	"github.com/toddwshaffer/coolant/thermal/internal/theme"
)

// colorsApproxEqual compares two color.Color values via their 8-bit RGB
// components, tolerating ±1 per channel to absorb rounding noise across
// the palette→colorful→RGB round-trip in HCL blending.
func colorsApproxEqual(a, b color.Color) bool {
	ar, ag, ab, _ := a.RGBA()
	br, bg, bb, _ := b.RGBA()
	diff := func(x, y uint32) int {
		d := int(x>>8) - int(y>>8)
		if d < 0 {
			return -d
		}
		return d
	}
	return diff(ar, br) <= 1 && diff(ag, bg) <= 1 && diff(ab, bb) <= 1
}

// TestRailColor_FullDecayIsGradientFg — decay=1.0 must return the
// CategoryGradient[level].Fg exactly so warming cells read as the
// category's heat color at peak.
func TestRailColor_FullDecayIsGradientFg(t *testing.T) {
	th := theme.Classic()
	iconBg := th.OverallGradient[1].Bg
	// Start at level 1 — level 0 is the idle override that collapses to
	// iconBg so a cold cell reads as pure backdrop; endpoint precision
	// for decay==1.0 only applies to warming levels.
	for level := 1; level < 5; level++ {
		got := railColor(th, level, iconBg, 1.0)
		want := th.CategoryGradient[level].Fg
		if !colorsApproxEqual(got, want) {
			gr, gg, gb, _ := got.RGBA()
			wr, wg, wb, _ := want.RGBA()
			t.Errorf("level=%d decay=1: got rgb(%d,%d,%d), want rgb(%d,%d,%d)",
				level, gr>>8, gg>>8, gb>>8, wr>>8, wg>>8, wb>>8)
		}
	}
}

// TestRailColor_IdleLevelCollapsesToIconBg — level 0 is the cold idle
// stop; the rail must paint iconBg regardless of decay so a cell with
// no activity renders as pure backdrop (no dim-cold rail leaking out).
func TestRailColor_IdleLevelCollapsesToIconBg(t *testing.T) {
	th := theme.Classic()
	iconBg := th.OverallGradient[1].Bg
	for _, decay := range []float64{0.0, 0.5, 1.0} {
		got := railColor(th, 0, iconBg, decay)
		if !colorsApproxEqual(got, iconBg) {
			gr, gg, gb, _ := got.RGBA()
			ir, ig, ib, _ := iconBg.RGBA()
			t.Errorf("level=0 decay=%f: got rgb(%d,%d,%d), want rgb(%d,%d,%d)",
				decay, gr>>8, gg>>8, gb>>8, ir>>8, ig>>8, ib>>8)
		}
	}
}

// TestRailColor_ZeroDecayIsIconBg — decay=0.0 must collapse to the
// pinned headline bg so a fully cooled rail blends invisibly with the
// cell backdrop.
func TestRailColor_ZeroDecayIsIconBg(t *testing.T) {
	th := theme.Classic()
	iconBg := th.OverallGradient[1].Bg
	for level := 0; level < 5; level++ {
		got := railColor(th, level, iconBg, 0.0)
		if !colorsApproxEqual(got, iconBg) {
			gr, gg, gb, _ := got.RGBA()
			ir, ig, ib, _ := iconBg.RGBA()
			t.Errorf("level=%d decay=0: got rgb(%d,%d,%d), want rgb(%d,%d,%d)",
				level, gr>>8, gg>>8, gb>>8, ir>>8, ig>>8, ib>>8)
		}
	}
}

// TestRailState_HoldsPeakDuringDecay — after level drops below peak,
// peakLevel must persist for the full ember duration before collapsing.
func TestRailState_HoldsPeakDuringDecay(t *testing.T) {
	rs := railState{}
	t0 := time.Date(2026, 4, 13, 12, 0, 0, 0, time.UTC)

	rs.update(3, t0, 2*time.Second)
	if rs.peakLevel != 3 {
		t.Fatalf("after climb to 3: peakLevel=%d want 3", rs.peakLevel)
	}

	// Drop to 0 — peak should hold.
	rs.update(0, t0.Add(10*time.Millisecond), 2*time.Second)
	if rs.peakLevel != 3 {
		t.Errorf("immediately after drop: peakLevel=%d want 3", rs.peakLevel)
	}

	// Halfway through decay — peak still held, decay progressing.
	d := rs.decayAt(t0.Add(1*time.Second+10*time.Millisecond), 2*time.Second)
	if d <= 0 || d >= 1 {
		t.Errorf("mid-decay: decay=%f want in (0,1)", d)
	}
	if rs.peakLevel != 3 {
		t.Errorf("mid-decay: peakLevel=%d want 3", rs.peakLevel)
	}

	// Just before ember elapses — peak still held.
	rs.update(0, t0.Add(1990*time.Millisecond), 2*time.Second)
	if rs.peakLevel != 3 {
		t.Errorf("just before elapse: peakLevel=%d want 3", rs.peakLevel)
	}

	// After full duration — decay saturates and peak collapses to current.
	rs.update(0, t0.Add(2500*time.Millisecond), 2*time.Second)
	if rs.peakLevel != 0 {
		t.Errorf("after elapse: peakLevel=%d want 0", rs.peakLevel)
	}
}

// TestRailState_UpwardLevelResetsImmediately — a rising level must
// reset the peak without any decay lag.
func TestRailState_UpwardLevelResetsImmediately(t *testing.T) {
	rs := railState{}
	t0 := time.Date(2026, 4, 13, 12, 0, 0, 0, time.UTC)

	rs.update(2, t0, 2*time.Second)
	// Drop, then climb above the previous peak immediately.
	rs.update(0, t0.Add(100*time.Millisecond), 2*time.Second)
	rs.update(4, t0.Add(200*time.Millisecond), 2*time.Second)

	if rs.peakLevel != 4 {
		t.Errorf("after climb past peak: peakLevel=%d want 4", rs.peakLevel)
	}
	if rs.currentLevel != 4 {
		t.Errorf("after climb: currentLevel=%d want 4", rs.currentLevel)
	}
	// Since we're at peak, decay should read as 1.0 (full heat).
	d := rs.decayAt(t0.Add(300*time.Millisecond), 2*time.Second)
	if d != 1.0 {
		t.Errorf("at peak: decay=%f want 1.0", d)
	}
}
