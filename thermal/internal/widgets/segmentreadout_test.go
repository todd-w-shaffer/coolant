package widgets

import (
	"image/color"
	"math"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/toddwshaffer/coolant/thermal/internal/anim"
	"github.com/toddwshaffer/coolant/thermal/internal/theme"
)

func newTestSegment(t *testing.T) *SegmentReadout {
	t.Helper()
	th := theme.Classic()
	th.Init()
	return NewSegmentReadout(th, anim.Default())
}

// TestSegmentReadout_RenderShape — both rows render, contain braille runes,
// and visible width is the constant 10.
func TestSegmentReadout_RenderShape(t *testing.T) {
	s := newTestSegment(t)
	s.Update(87, 3)
	top, bot, w := s.Render(color.RGBA{0, 0, 0, 255})

	if w != 10 {
		t.Errorf("visWidth=%d, want 10", w)
	}
	if ansi.StringWidth(top) != 10 {
		t.Errorf("top visual width=%d, want 10 (stripped: %q)", ansi.StringWidth(top), ansi.Strip(top))
	}
	if ansi.StringWidth(bot) != 10 {
		t.Errorf("bot visual width=%d, want 10 (stripped: %q)", ansi.StringWidth(bot), ansi.Strip(bot))
	}
	// Top row must contain the head rune of each selected digit (8, 7).
	for _, d := range []int{8, 7} {
		wantTop, _ := digitToBraille(segmentDigits[d])
		if !strings.ContainsRune(top, wantTop[0]) {
			t.Errorf("top does not contain digit %d head rune %#x", d, wantTop[0])
		}
	}
}

// TestSegmentReadout_ValueColors — different values produce different ANSI
// output (proves the continuous gradient LUT is consulted).
func TestSegmentReadout_ValueColors(t *testing.T) {
	s := newTestSegment(t)
	s.Update(15, 1)
	cool, _, _ := s.Render(color.RGBA{0, 0, 0, 255})
	s.Update(90, 4)
	melt, _, _ := s.Render(color.RGBA{0, 0, 0, 255})
	if cool == melt {
		t.Errorf("cool and meltdown values render identical; gradient not consulted")
	}
}

// TestSegmentReadout_GradientContinuous — two values in the same threat band
// but at different positions render distinct colors. If they match the
// readout snapped to discrete levels instead of a continuous LUT.
func TestSegmentReadout_GradientContinuous(t *testing.T) {
	s := newTestSegment(t)
	s.Update(30, 2) // warm low
	low, _, _ := s.Render(color.RGBA{0, 0, 0, 255})
	s.Update(54, 2) // warm high
	high, _, _ := s.Render(color.RGBA{0, 0, 0, 255})
	if low == high {
		t.Errorf("two in-band values render identical; gradient is snapping")
	}
}

// TestSegmentReadout_LevelClamp — out-of-range levels clamp without panicking.
func TestSegmentReadout_LevelClamp(t *testing.T) {
	s := newTestSegment(t)
	s.Update(50, -5)
	s.Render(color.RGBA{0, 0, 0, 255})
	s.Update(50, 99)
	s.Render(color.RGBA{0, 0, 0, 255})
}

// TestSegmentReadout_NoGhostOnFirstUpdate — the very first Update has no
// prior value, so it renders the current value without a ghost.
func TestSegmentReadout_NoGhostOnFirstUpdate(t *testing.T) {
	s := newTestSegment(t)
	s.Update(50, 2)
	top, _, _ := s.Render(color.RGBA{0, 0, 0, 255})
	want5, _ := digitToBraille(segmentDigits[5])
	if !strings.ContainsRune(top, want5[0]) {
		t.Errorf("first update should render current value; no digit '5' head rune found")
	}
}

// TestSegmentReadout_GhostShowsPrevValue — on value change the ghost frame
// shows the previous value, not the new one. The new value "arrives" after
// the ghost window expires.
func TestSegmentReadout_GhostShowsPrevValue(t *testing.T) {
	s := newTestSegment(t)
	bg := color.RGBA{0, 0, 0, 255}
	s.Update(50, 2)
	_, _, _ = s.Render(bg)
	s.Update(60, 2)
	top, _, _ := s.Render(bg)

	want5, _ := digitToBraille(segmentDigits[5])
	want6, _ := digitToBraille(segmentDigits[6])
	if !strings.ContainsRune(top, want5[0]) {
		t.Errorf("ghost frame should contain prev digit '5' head rune")
	}
	if strings.ContainsRune(top, want6[0]) {
		t.Errorf("ghost frame should NOT contain new digit '6' head rune yet")
	}
}

// TestSegmentReadout_GhostDimmerThanNormal — the ghost frame color differs
// from the post-ghost frame color (dimmed variant of the prev value).
func TestSegmentReadout_GhostDimmerThanNormal(t *testing.T) {
	s := newTestSegment(t)
	bg := color.RGBA{0, 0, 0, 255}
	s.Update(50, 2)
	_, _, _ = s.Render(bg)
	s.Update(60, 2)
	ghost, _, _ := s.Render(bg)
	for i := 0; i < 10; i++ {
		s.AnimTick()
	}
	post, _, _ := s.Render(bg)
	if ghost == post {
		t.Errorf("ghost and post-ghost frames should differ")
	}
}

// TestSegmentReadout_RapidOscillationSettles — snapshots arrive at a
// faster cadence than the ghost window, so Update is called repeatedly
// with small value deltas. The readout must still spend most of its time
// showing the CURRENT value, not permanently ghosting the previous one.
// Reproduces the live-demo bug where the LCD never settled.
func TestSegmentReadout_RapidOscillationSettles(t *testing.T) {
	s := newTestSegment(t)
	bg := color.RGBA{0, 0, 0, 255}
	// Establish baseline.
	s.Update(50, 2)
	_, _, _ = s.Render(bg)

	// 20 snapshots, each one AnimTick apart, each bumping value by 1 or 2
	// points — the natural jitter of a smoothed live signal. The readout
	// must show the CURRENT value in at least half the renders; otherwise
	// the ghost trail is stuck permanently re-arming.
	// A "current hit" means the ones-digit head rune of v appears at its
	// expected slot (position 4 — after the tens digit + gap). Substring
	// containment isn't sufficient: the tens digit stays at 5 across the
	// oscillation, and ghost frames rendering a prev value with the same
	// tens digit would falsely register.
	currentHits := 0
	for i := 0; i < 20; i++ {
		v := 50 + (i % 3) // 50, 51, 52, 50, 51, ... — jitter below the ghost threshold
		s.Update(v, 2)
		top, _, _ := s.Render(bg)
		stripped := []rune(ansi.Strip(top))
		onesHead, _ := digitToBraille(segmentDigits[v%10])
		if len(stripped) > 4 && stripped[4] == onesHead[0] {
			currentHits++
		}
		s.AnimTick()
	}
	if currentHits < 10 {
		t.Errorf("rapid oscillation: current value shown in only %d/20 renders — ghost is stuck", currentHits)
	}
}

// TestSegmentReadout_OfflineOnlineCycle — a long offline gap (no updates)
// followed by resuming with a different value must not trap the readout
// in permanent ghost mode. After the ghost window completes the current
// value must render.
func TestSegmentReadout_OfflineOnlineCycle(t *testing.T) {
	s := newTestSegment(t)
	bg := color.RGBA{0, 0, 0, 255}
	s.Update(50, 2)
	_, _, _ = s.Render(bg)
	// Simulate offline window: no Update calls, only AnimTicks.
	for i := 0; i < 10; i++ {
		s.AnimTick()
	}
	// Resume online with a new value.
	s.Update(85, 3)
	// Burn through ghost window.
	for i := 0; i < ghostTickCount+1; i++ {
		s.AnimTick()
	}
	// Now a few normal snapshots with stable value.
	s.Update(85, 3)
	top, _, _ := s.Render(bg)
	want8, _ := digitToBraille(segmentDigits[8])
	want5, _ := digitToBraille(segmentDigits[5])
	if !strings.ContainsRune(top, want8[0]) || !strings.ContainsRune(top, want5[0]) {
		t.Errorf("after offline→online cycle, readout should show 85 head runes; got %q", ansi.Strip(top))
	}
}

// TestSegmentReadout_GhostExpires — after enough AnimTicks the new value
// takes over.
func TestSegmentReadout_GhostExpires(t *testing.T) {
	s := newTestSegment(t)
	bg := color.RGBA{0, 0, 0, 255}
	s.Update(50, 2)
	_, _, _ = s.Render(bg)
	s.Update(60, 2)
	for i := 0; i < 10; i++ {
		s.AnimTick()
	}
	top, _, _ := s.Render(bg)
	want6, _ := digitToBraille(segmentDigits[6])
	if !strings.ContainsRune(top, want6[0]) {
		t.Errorf("after ghost expires, render should show new digit '6'")
	}
}

// TestSegmentReadout_FlashOnUpwardTransition — bumping the level upward
// arms a 1-tick inverted frame. The inverted frame differs from the
// steady-state frame.
func TestSegmentReadout_FlashOnUpwardTransition(t *testing.T) {
	s := newTestSegment(t)
	bg := color.RGBA{0, 0, 0, 255}
	s.Update(50, 1)
	_, _, _ = s.Render(bg)
	s.Update(50, 2) // upward: value unchanged (so no ghost), level up → flash
	flash, _, _ := s.Render(bg)
	s.AnimTick()
	post, _, _ := s.Render(bg)
	if flash == post {
		t.Errorf("flash frame should differ from post-flash frame")
	}
}

// TestSegmentReadout_NoFlashOnDownwardTransition — downward level moves
// don't pop; the render is identical before and after AnimTick.
func TestSegmentReadout_NoFlashOnDownwardTransition(t *testing.T) {
	s := newTestSegment(t)
	bg := color.RGBA{0, 0, 0, 255}
	s.Update(50, 3)
	_, _, _ = s.Render(bg)
	s.Update(50, 1) // downward
	pre, _, _ := s.Render(bg)
	s.AnimTick()
	post, _, _ := s.Render(bg)
	if pre != post {
		t.Errorf("downward transition should not flash; frames differ")
	}
}

// TestSegmentReadout_MeltdownPulseModulates — varying pulse scale across
// the sine cycle produces distinct rendered outputs (sanity that the
// modulation actually reaches the ANSI escape).
func TestSegmentReadout_MeltdownPulseModulates(t *testing.T) {
	s := newTestSegment(t)
	bg := color.RGBA{0, 0, 0, 255}
	s.Update(90, 4)
	seen := map[string]bool{}
	for _, ph := range []float64{0, 1, 2, 3, 4, 5} {
		pulse := 0.6 + 0.4*((math.Sin(ph)+1)/2)
		top, _, _ := s.RenderWithPulse(bg, pulse)
		seen[top] = true
	}
	if len(seen) < 2 {
		t.Errorf("pulse modulation produced %d distinct outputs, want >=2", len(seen))
	}
}
