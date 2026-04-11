package widgets

import (
	"testing"
	"time"

	"github.com/toddwshaffer/coolant/thermal/internal/collector"
	"github.com/toddwshaffer/coolant/thermal/internal/model"
)

// seedGauges creates a Gauges with enough state to produce View output.
func seedGauges(t *testing.T) *Gauges {
	t.Helper()
	g := NewGauges(testTheme, testAnim)
	g.SetSize(120, 6)

	state := model.NewAppState()
	snap := collector.Snapshot{
		System: collector.SystemStats{
			CPUPercent:    25,
			MemUsedBytes:  8 << 30,
			MemTotalBytes: 16 << 30,
			NCPUs:         8,
		},
		Online:    true,
		Timestamp: time.Now(),
	}
	state.Update(snap)
	g.Update(state)
	// Pump a few anim ticks so renderHistory is populated
	for i := 0; i < 5; i++ {
		g.AnimTick()
	}
	return g
}

func TestViewLinesNilWhenTooShort(t *testing.T) {
	g := seedGauges(t)
	for _, avail := range []int{0, 1, 2} {
		if got := g.ViewLines(avail); got != nil {
			t.Errorf("ViewLines(%d) = %d lines, want nil", avail, len(got))
		}
	}
}

func TestViewLinesCPUOnlyAtHeight3(t *testing.T) {
	g := seedGauges(t)
	got := g.ViewLines(3)
	if len(got) != 2 {
		t.Errorf("ViewLines(3) = %d lines, want 2 (CPU only)", len(got))
	}
}

func TestViewLinesCPUAndMemAtHeight5(t *testing.T) {
	g := seedGauges(t)
	got := g.ViewLines(5)
	if len(got) != 4 {
		t.Errorf("ViewLines(5) = %d lines, want 4 (CPU+MEM)", len(got))
	}
}

func TestViewLinesFullAtHeight6(t *testing.T) {
	g := seedGauges(t)
	got := g.ViewLines(6)
	if len(got) != 6 {
		t.Errorf("ViewLines(6) = %d lines, want 6 (all gauges)", len(got))
	}
}

func TestViewLinesFullAtHeight10(t *testing.T) {
	g := seedGauges(t)
	got := g.ViewLines(10)
	if len(got) != 6 {
		t.Errorf("ViewLines(10) = %d lines, want 6 (all gauges, no padding)", len(got))
	}
}

func TestViewLinesNilWhenNoState(t *testing.T) {
	g := NewGauges(testTheme, testAnim)
	g.SetSize(120, 6)
	if got := g.ViewLines(10); got != nil {
		t.Errorf("ViewLines with no state = %d lines, want nil", len(got))
	}
}
