package widgets

import (
	"image/color"
	"math"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/toddwshaffer/coolant/thermal/internal/anim"
	"github.com/toddwshaffer/coolant/thermal/internal/theme"
)

// renderedTemp decodes the displayed integer from both braille rows. Each
// digit occupies 3 cells per row × 2 rows = 6 runes; the full 6-tuple is
// unique per digit, so comparing all 6 runes disambiguates glyphs that
// share a head rune (e.g. 3/6/8/9 all start with .####. in row 0).
func renderedTemp(t *testing.T, top, bot string) int {
	t.Helper()
	topR := []rune(ansi.Strip(top))
	botR := []rune(ansi.Strip(bot))
	if len(topR) < 9 || len(botR) < 9 {
		t.Fatalf("stripped rows too short: top=%q bot=%q", string(topR), string(botR))
	}
	decode := func(off int) int {
		tr := [3]rune{topR[off], topR[off+1], topR[off+2]}
		br := [3]rune{botR[off], botR[off+1], botR[off+2]}
		for d := 0; d < 10; d++ {
			dt, db := digitToBraille(segmentDigits[d])
			if dt == tr && db == br {
				return d
			}
		}
		return -1
	}
	tens := decode(0)
	ones := decode(4)
	if tens < 0 || ones < 0 {
		return -1
	}
	return tens*10 + ones
}

func newSmoothingSegment(t *testing.T) *SegmentReadout {
	t.Helper()
	th := theme.Classic()
	th.Init()
	return NewSegmentReadout(th, anim.Default())
}

func renderInt(t *testing.T, s *SegmentReadout) int {
	t.Helper()
	top, bot, _ := s.Render(color.RGBA{0, 0, 0, 255})
	return renderedTemp(t, top, bot)
}

// TestSegmentReadout_Smoothing_SpringEasing — after seeding then jumping,
// the displayed int lags initially and monotonically converges to target.
func TestSegmentReadout_Smoothing_SpringEasing(t *testing.T) {
	s := newSmoothingSegment(t)

	s.Update(50, 2)
	if v := renderInt(t, s); v != 50 {
		t.Fatalf("post-seed render: got %d, want 50", v)
	}

	s.Update(70, 2)
	// Immediately after Update (no AnimTick yet), displayed value must still be 50.
	if v := renderInt(t, s); v != 50 {
		t.Errorf("pre-tick after jump: got %d, want 50 (spring should not advance on Update)", v)
	}

	prev := 50
	const ticks = 60
	for i := 1; i <= ticks; i++ {
		s.AnimTick()
		v := renderInt(t, s)
		if v < prev {
			t.Errorf("tick %d: displayed %d regressed below prev %d (not monotonic)", i, v, prev)
		}
		prev = v
	}
	if prev != 70 {
		t.Errorf("after %d ticks: displayed %d, want 70 (did not converge)", ticks, prev)
	}
}

// TestSegmentReadout_Smoothing_SeededJumpOnFirstUpdate — first Update
// displays the target immediately, no easing from zero.
func TestSegmentReadout_Smoothing_SeededJumpOnFirstUpdate(t *testing.T) {
	s := newSmoothingSegment(t)
	s.Update(85, 3)
	if v := renderInt(t, s); v != 85 {
		t.Errorf("first update: got %d, want 85 (should jump to target)", v)
	}
}

// TestSegmentReadout_Smoothing_BoundaryHysteresisHoldsOnJitter — tiny
// alternating updates must not cause the displayed value to oscillate.
func TestSegmentReadout_Smoothing_BoundaryHysteresisHoldsOnJitter(t *testing.T) {
	s := newSmoothingSegment(t)
	s.Update(50, 2)
	s.AnimTick()
	_ = renderInt(t, s)

	prev := 50
	transitions := 0
	for i := 0; i < 20; i++ {
		v := 50
		if i%2 == 0 {
			v = 51
		}
		s.Update(v, 2)
		s.AnimTick()
		disp := renderInt(t, s)
		if disp < 49 || disp > 51 {
			t.Errorf("jitter iter %d: displayed %d out of {49,50,51}", i, disp)
		}
		if disp != prev {
			transitions++
			prev = disp
		}
	}
	if transitions > 2 {
		t.Errorf("jitter transitions=%d, want ≤2 (hysteresis not eating jitter)", transitions)
	}
}

// TestSegmentReadout_Smoothing_OfflineHoldLastValue — no Update calls for
// many ticks; displayed value holds the last seeded value, never blanks.
func TestSegmentReadout_Smoothing_OfflineHoldLastValue(t *testing.T) {
	s := newSmoothingSegment(t)
	s.Update(65, 2)
	for i := 0; i < 60; i++ {
		s.AnimTick()
		if v := renderInt(t, s); v != 65 {
			t.Errorf("offline tick %d: displayed %d, want 65", i, v)
			break
		}
	}
}

// TestSegmentReadout_Smoothing_LargeJumpCompletes — a big jump converges
// to the target after enough ticks.
func TestSegmentReadout_Smoothing_LargeJumpCompletes(t *testing.T) {
	s := newSmoothingSegment(t)
	s.Update(20, 1)
	s.Update(85, 3)
	for i := 0; i < 60; i++ {
		s.AnimTick()
	}
	if v := renderInt(t, s); v != 85 {
		t.Errorf("after large jump + 60 ticks: displayed %d, want 85", v)
	}
}

// TestSegmentReadout_Smoothing_MeltdownPulseIndependent — pulse modulation
// still produces distinct outputs at a stable smoothed value.
func TestSegmentReadout_Smoothing_MeltdownPulseIndependent(t *testing.T) {
	s := newSmoothingSegment(t)
	bg := color.RGBA{0, 0, 0, 255}
	s.Update(90, 4)
	// Let spring settle.
	for i := 0; i < 60; i++ {
		s.AnimTick()
	}
	seen := map[string]bool{}
	for _, ph := range []float64{0, 1, 2, 3, 4, 5} {
		pulse := 0.6 + 0.4*((math.Sin(ph)+1)/2)
		top, _, _ := s.RenderWithPulse(bg, pulse)
		seen[top] = true
	}
	if len(seen) < 2 {
		t.Errorf("pulse modulation at stable value produced %d distinct outputs, want ≥2", len(seen))
	}
}
