package widgets

import (
	"testing"

	"github.com/toddwshaffer/coolant/thermal/internal/collector"
)

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
