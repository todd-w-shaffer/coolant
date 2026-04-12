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
// and visible width is the constant 12.
func TestSegmentReadout_RenderShape(t *testing.T) {
	s := newTestSegment(t)
	s.Update(87, 3)
	top, bot, w := s.Render(color.RGBA{0, 0, 0, 255})

	if w != 12 {
		t.Errorf("visWidth=%d, want 12", w)
	}
	if ansi.StringWidth(top) != 12 {
		t.Errorf("top visual width=%d, want 12 (stripped: %q)", ansi.StringWidth(top), ansi.Strip(top))
	}
	if ansi.StringWidth(bot) != 12 {
		t.Errorf("bot visual width=%d, want 12 (stripped: %q)", ansi.StringWidth(bot), ansi.Strip(bot))
	}
	// Top row must contain the head rune of each selected digit (0, 8, 7).
	for _, d := range []int{0, 8, 7} {
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
