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

// TestSegmentReadout_LevelColors — different levels produce different ANSI
// output (proves the level is actually consulted).
func TestSegmentReadout_LevelColors(t *testing.T) {
	s := newTestSegment(t)
	s.Update(50, 1)
	cool, _, _ := s.Render(color.RGBA{0, 0, 0, 255})
	s.Update(50, 4)
	melt, _, _ := s.Render(color.RGBA{0, 0, 0, 255})
	if cool == melt {
		t.Errorf("cool and meltdown renders identical; levels not influencing output")
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
