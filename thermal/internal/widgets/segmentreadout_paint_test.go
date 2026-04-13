package widgets

import (
	"image/color"
	"strings"
	"testing"

	"github.com/toddwshaffer/coolant/thermal/internal/anim"
	"github.com/toddwshaffer/coolant/thermal/internal/theme"
)

func newPaintSegment(t *testing.T) *SegmentReadout {
	t.Helper()
	th := theme.Classic()
	th.Init()
	return NewSegmentReadout(th, anim.Default())
}

// settle drives Update + enough AnimTicks for the spring to converge so the
// displayed int matches the requested target.
func settle(t *testing.T, s *SegmentReadout, value, level int) {
	t.Helper()
	s.Update(value, level)
	for i := 0; i < 90 && s.value != value; i++ {
		s.AnimTick()
	}
	if s.value != value {
		t.Fatalf("spring failed to converge to %d (stuck at %d)", value, s.value)
	}
	// One more tick to consume any pending level-up flash so Render hits the
	// steady-state branch (the flash branch is a whole-line invert and doesn't
	// participate in per-digit span caching).
	s.AnimTick()
}

// expectedTensSpan reconstructs the byte sequence the tens digit's styled
// fragment SHOULD have if the readout is rendered as three per-digit spans
// and the tens digit is colored by its band-anchor (value/10*10).
func expectedTensSpan(th *theme.Theme, value int) string {
	band := (value / 10) * 10
	digit := value / 10
	top, _ := digitToBraille(segmentDigits[digit])
	fg := th.OverallTemperaturePulsedFg(band, 1.0)
	return fg + string(top[:]) + "\033[39m"
}

// expectedDegreeSpan reconstructs the byte sequence the degree glyph's
// styled fragment SHOULD have if it's colored by the threat level.
func expectedDegreeSpan(th *theme.Theme, level int) string {
	top, _ := degreeToBraille()
	fg := th.OverallLevelFg(level)
	return fg + string(top[:]) + "\033[39m"
}

// TestSegmentReadout_TensSpanByteIdenticalWithinBand — the tens span byte
// sequence must appear identically in 14 and 15 (same band). This is the
// "1 doesn't blink when ones changes" invariant the user asked for.
func TestSegmentReadout_TensSpanByteIdenticalWithinBand(t *testing.T) {
	s := newPaintSegment(t)
	bg := color.RGBA{0, 0, 0, 255}

	settle(t, s, 14, 1)
	top14, _, _ := s.Render(bg)

	settle(t, s, 15, 1)
	top15, _, _ := s.Render(bg)

	want := expectedTensSpan(s.theme, 14)
	if !strings.Contains(top14, want) {
		t.Errorf("render of 14 missing expected tens span")
	}
	if !strings.Contains(top15, want) {
		t.Errorf("render of 15 missing expected tens span — tens digit is repainting on every value change")
	}
}

// TestSegmentReadout_TensSpanChangesAcrossBandBoundary — when crossing a
// tens boundary (19→20) the tens span MUST change. Sanity that we're not
// over-caching.
func TestSegmentReadout_TensSpanChangesAcrossBandBoundary(t *testing.T) {
	s := newPaintSegment(t)
	bg := color.RGBA{0, 0, 0, 255}

	settle(t, s, 19, 1)
	span19 := expectedTensSpan(s.theme, 19)
	top19, _, _ := s.Render(bg)
	if !strings.Contains(top19, span19) {
		t.Fatalf("setup: render of 19 missing its tens span")
	}

	settle(t, s, 20, 1)
	if strings.Contains(top19, expectedTensSpan(s.theme, 20)) {
		t.Errorf("setup invalid: 19 contains 20's tens span")
	}
	top20, _, _ := s.Render(bg)
	if strings.Contains(top20, span19) {
		t.Errorf("19→20: tens span did not change despite band crossing")
	}
}

// TestSegmentReadout_DegreeSpanByteIdenticalWithinLevel — degree glyph's
// styled fragment is identical for two values inside the same threat level.
func TestSegmentReadout_DegreeSpanByteIdenticalWithinLevel(t *testing.T) {
	s := newPaintSegment(t)
	bg := color.RGBA{0, 0, 0, 255}

	settle(t, s, 30, 2)
	top30, _, _ := s.Render(bg)

	settle(t, s, 45, 2)
	top45, _, _ := s.Render(bg)

	want := expectedDegreeSpan(s.theme, 2)
	if !strings.Contains(top30, want) {
		t.Errorf("render of 30 missing expected degree span (level=2)")
	}
	if !strings.Contains(top45, want) {
		t.Errorf("render of 45 missing expected degree span — degree repainting within same level")
	}
}

// TestSegmentReadout_DegreeSpanChangesAcrossLevels — degree glyph's styled
// fragment differs when the level changes.
func TestSegmentReadout_DegreeSpanChangesAcrossLevels(t *testing.T) {
	s := newPaintSegment(t)
	bg := color.RGBA{0, 0, 0, 255}

	settle(t, s, 30, 1)
	deg1 := expectedDegreeSpan(s.theme, 1)

	settle(t, s, 30, 3)
	top30hot, _, _ := s.Render(bg)

	if strings.Contains(top30hot, deg1) {
		t.Errorf("level 1→3: degree span did not change")
	}
	want := expectedDegreeSpan(s.theme, 3)
	if !strings.Contains(top30hot, want) {
		t.Errorf("render at level 3 missing level-3 degree span")
	}
}

// TestSegmentReadout_DegreeNotPulsedAtMeltdown — at meltdown, varying
// pulseScale must NOT change the degree span (degree color is level-derived,
// not pulse-modulated). Digits may still pulse.
func TestSegmentReadout_DegreeNotPulsedAtMeltdown(t *testing.T) {
	s := newPaintSegment(t)
	bg := color.RGBA{0, 0, 0, 255}
	settle(t, s, 90, 4)

	want := expectedDegreeSpan(s.theme, 4)
	for _, p := range []float64{0.4, 0.7, 1.0} {
		top, _, _ := s.RenderWithPulse(bg, p)
		if !strings.Contains(top, want) {
			t.Errorf("pulse=%.2f: degree span changed (degree should be pulse-independent)", p)
		}
	}
}

// TestSegmentReadout_GhostNeverArms — ghost-trail logic is removed; even on
// a large jump the ghostTicks counter must remain zero throughout.
func TestSegmentReadout_GhostNeverArms(t *testing.T) {
	s := newPaintSegment(t)
	s.Update(20, 1)
	s.AnimTick()
	s.Update(80, 3) // big jump
	for i := 0; i < 30; i++ {
		s.AnimTick()
		if s.ghostTicks > 0 {
			t.Fatalf("tick %d: ghost armed (ticks=%d) — arming should be removed", i, s.ghostTicks)
		}
	}
}
