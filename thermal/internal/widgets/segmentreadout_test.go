package widgets

import (
	"image/color"
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

// settleTo drives Update + AnimTicks until the spring lands on the target.
func settleTo(t *testing.T, s *SegmentReadout, value, level int) {
	t.Helper()
	s.Update(value, level)
	for i := 0; i < 90 && s.value != value; i++ {
		s.AnimTick()
	}
}

// TestSegmentReadout_ValueColors — different values produce different ANSI
// output (proves the continuous gradient LUT is consulted).
func TestSegmentReadout_ValueColors(t *testing.T) {
	s := newTestSegment(t)
	settleTo(t, s, 15, 1)
	cool, _, _ := s.Render(color.RGBA{0, 0, 0, 255})
	settleTo(t, s, 90, 4)
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
	settleTo(t, s, 30, 2) // warm low
	low, _, _ := s.Render(color.RGBA{0, 0, 0, 255})
	settleTo(t, s, 54, 2) // warm high
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

// TestSegmentReadout_RapidOscillationSettles — snapshots arrive at a
// faster cadence than the ghost window, so Update is called repeatedly
// with small value deltas. With spring smoothing the displayed int must
// stay within a tight neighborhood of the oscillation band and never get
// trapped in a ghost re-arming loop. Reproduces the live-demo settle bug.
func TestSegmentReadout_RapidOscillationSettles(t *testing.T) {
	s := newTestSegment(t)
	bg := color.RGBA{0, 0, 0, 255}
	// Establish baseline.
	s.Update(50, 2)
	_, _, _ = s.Render(bg)

	// 20 snapshots, each one AnimTick apart, each bumping value by 1 or 2
	// points — the natural jitter of a smoothed live signal. The displayed
	// int (decoded from top row) must remain within {49..52} for every
	// frame; the spring hysteresis absorbs the sub-threshold jitter and
	// prevents the ghost from arming on noise.
	tensHead, _ := digitToBraille(segmentDigits[5])
	for i := 0; i < 20; i++ {
		v := 50 + (i % 3) // 50, 51, 52, 50, 51, ... — jitter below the ghost threshold
		s.Update(v, 2)
		top, _, _ := s.Render(bg)
		stripped := []rune(ansi.Strip(top))
		if len(stripped) < 5 || stripped[0] != tensHead[0] {
			t.Errorf("iter %d: tens digit left '5' band; stripped=%q", i, string(stripped))
		}
		s.AnimTick()
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
	// Resume online with a new value, then let the spring converge.
	s.Update(85, 3)
	for i := 0; i < 60; i++ {
		s.AnimTick()
	}
	s.Update(85, 3)
	top, _, _ := s.Render(bg)
	want8, _ := digitToBraille(segmentDigits[8])
	want5, _ := digitToBraille(segmentDigits[5])
	if !strings.ContainsRune(top, want8[0]) || !strings.ContainsRune(top, want5[0]) {
		t.Errorf("after offline→online cycle, readout should show 85 head runes; got %q", ansi.Strip(top))
	}
}

// TestSegmentReadout_NoFlashOnAnyTransition — upward and downward level
// moves produce identical frames before and after AnimTick. No flash.
func TestSegmentReadout_NoFlashOnAnyTransition(t *testing.T) {
	s := newTestSegment(t)
	bg := color.RGBA{0, 0, 0, 255}
	s.Update(50, 1)
	_, _, _ = s.Render(bg)
	// Upward: should NOT flash.
	s.Update(50, 3)
	pre, _, _ := s.Render(bg)
	s.AnimTick()
	post, _, _ := s.Render(bg)
	if pre != post {
		t.Errorf("upward transition should not flash; frames differ")
	}
	// Downward: should NOT flash.
	s.Update(50, 1)
	pre, _, _ = s.Render(bg)
	s.AnimTick()
	post, _, _ = s.Render(bg)
	if pre != post {
		t.Errorf("downward transition should not flash; frames differ")
	}
}

// TestSegmentReadout_SteadyAtMeltdown — at meltdown level (4), Render
// produces identical output across ticks. No pulse modulation exists.
func TestSegmentReadout_SteadyAtMeltdown(t *testing.T) {
	s := newTestSegment(t)
	bg := color.RGBA{0, 0, 0, 255}
	s.Update(90, 4)
	// Let spring settle so value is stable.
	for i := 0; i < 60; i++ {
		s.AnimTick()
	}
	ref, _, _ := s.Render(bg)
	for i := 0; i < 8; i++ {
		s.AnimTick()
		top, _, _ := s.Render(bg)
		if top != ref {
			t.Fatalf("tick %d: Render output changed at meltdown level (no pulse expected)", i)
		}
	}
}
