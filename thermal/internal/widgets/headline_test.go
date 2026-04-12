package widgets

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/toddwshaffer/coolant/thermal/internal/anim"
	"github.com/toddwshaffer/coolant/thermal/internal/collector"
	"github.com/toddwshaffer/coolant/thermal/internal/model"
	"github.com/toddwshaffer/coolant/thermal/internal/theme"
)

// TestHeadline_ViewLinesTwoRowsWhenOnline — online+active state returns two
// lines of equal visible width, so the headline paints a 2-row strip.
func TestHeadline_ViewLinesTwoRowsWhenOnline(t *testing.T) {
	th := theme.Classic()
	th.Init()
	h := NewHeadline(th, anim.Default())
	h.SetSize(120, 2)
	h.Update(fixtureState())

	lines := h.ViewLines()
	if len(lines) != 2 {
		t.Fatalf("online active: got %d line(s), want 2", len(lines))
	}
	w0 := ansi.StringWidth(lines[0])
	w1 := ansi.StringWidth(lines[1])
	if w0 != w1 {
		t.Errorf("row widths differ: top=%d bot=%d", w0, w1)
	}
	if w0 == 0 {
		t.Errorf("top row empty")
	}
}

// TestHeadline_ViewLinesOneRowWhenOffline — offline fallback stays 1 row so
// the offline path is untouched.
func TestHeadline_ViewLinesOneRowWhenOffline(t *testing.T) {
	th := theme.Classic()
	th.Init()
	h := NewHeadline(th, anim.Default())
	h.SetSize(120, 2)
	state := fixtureState()
	state.Online = false
	h.Update(state)

	lines := h.ViewLines()
	if len(lines) != 1 {
		t.Errorf("offline: got %d line(s), want 1", len(lines))
	}
}

// TestHeadline_TwoRowPreservesTopContent — the 2-row growth is additive. The
// top row must still contain the quip text and the fixed category labels
// that the 1-row headline shows today. If this fails the refactor removed
// user-visible content.
func TestHeadline_TwoRowPreservesTopContent(t *testing.T) {
	th := theme.Classic()
	th.Init()
	h := NewHeadline(th, anim.Default())
	h.SetSize(120, 2)
	h.Update(fixtureState())

	lines := h.ViewLines()
	top := ansi.Strip(lines[0])
	for _, want := range []string{"build", "shell"} {
		if !strings.Contains(top, want) {
			t.Errorf("top row missing category label %q:\n%s", want, top)
		}
	}
}

// TestHeadline_MeltdownPulseDrivesModulation — at meltdown, successive
// AnimTicks must change the rendered output. This proves the pulse phase
// is owned at Headline (single oscillator) and actually reaches the
// segment readout's fg color.
func TestHeadline_MeltdownPulseDrivesModulation(t *testing.T) {
	th := theme.Classic()
	th.Init()
	h := NewHeadline(th, anim.Default())
	h.SetSize(120, 2)

	state := fixtureState()
	state.ThreatLevel = model.ThreatMeltdown
	h.Update(state)

	frames := make([]string, 0, 8)
	for i := 0; i < 8; i++ {
		lines := h.ViewLines()
		frames = append(frames, lines[0])
		h.AnimTick()
	}
	distinct := map[string]bool{}
	for _, f := range frames {
		distinct[f] = true
	}
	if len(distinct) < 2 {
		t.Errorf("meltdown pulse produced %d distinct top frames across 8 ticks, want >=2", len(distinct))
	}
}

func TestVisibleCategoriesFixedAlwaysPresent(t *testing.T) {
	smoothed := map[string]float64{} // all zero
	got := visibleCategories(smoothed)
	// build and shell must appear even with zero counts
	found := map[string]bool{}
	for _, cat := range got {
		found[cat.Name] = true
	}
	if !found["build"] {
		t.Error("build should always be visible")
	}
	if !found["shell"] {
		t.Error("shell should always be visible")
	}
}

func TestVisibleCategoriesDynamicAppearsWhenNonZero(t *testing.T) {
	smoothed := map[string]float64{"node": 5.0}
	got := visibleCategories(smoothed)
	found := map[string]bool{}
	for _, cat := range got {
		found[cat.Name] = true
	}
	if !found["node"] {
		t.Error("node should be visible when count > 0")
	}
}

func TestVisibleCategoriesDynamicHiddenWhenZero(t *testing.T) {
	smoothed := map[string]float64{} // no go
	got := visibleCategories(smoothed)
	for _, cat := range got {
		if cat.Name == "go" {
			t.Error("go should not be visible when count is zero")
		}
	}
}

func TestVisibleCategoriesPreservesOrder(t *testing.T) {
	smoothed := map[string]float64{"node": 5.0, "go": 2.0, "rust": 1.0}
	got := visibleCategories(smoothed)
	// Should follow collector.Categories order
	prevOrder := -1
	for _, cat := range got {
		for _, ref := range collector.Categories {
			if ref.Name == cat.Name {
				if ref.Order < prevOrder {
					t.Errorf("category %q (order %d) appeared after order %d", cat.Name, ref.Order, prevOrder)
				}
				prevOrder = ref.Order
			}
		}
	}
}
