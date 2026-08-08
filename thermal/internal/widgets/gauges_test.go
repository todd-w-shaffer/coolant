package widgets

import (
	"math"
	"testing"
	"time"

	"github.com/toddwshaffer/coolant/thermal/internal/collector"
	"github.com/toddwshaffer/coolant/thermal/internal/model"
)

// seedGaugesAt creates a Gauges at a given CPU% and width, pumps ticks until
// the spring settles, and returns the ready-to-test Gauges plus its AppState.
func seedGaugesAt(t *testing.T, cpu float64, width, ticks int) (*Gauges, *model.AppState, *collector.Snapshot) {
	t.Helper()
	g := NewGauges(testTheme, testAnim)
	g.SetSize(width, 6)

	state := model.NewAppState()
	snap := collector.Snapshot{
		System: collector.SystemStats{
			CPUPercent:    cpu,
			MemUsedBytes:  8 << 30,
			MemTotalBytes: 16 << 30,
			NCPUs:         8,
		},
		Online:    true,
		Timestamp: time.Now(),
	}
	state.Update(snap)
	g.Update(state)
	for i := 0; i < ticks; i++ {
		g.AnimTick()
	}
	return g, state, &snap
}

// seedGauges creates a Gauges with enough state to produce View output.
func seedGauges(t *testing.T) *Gauges {
	t.Helper()
	g, _, _ := seedGaugesAt(t, 25, 120, 5)
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

func TestPeakDecaySnapsUp(t *testing.T) {
	g, _, _ := seedGaugesAt(t, 80, 120, 60)

	if g.peaks[0] < 70 {
		t.Errorf("peak[CPU] = %v, want >= 70 after 80%% CPU spike", g.peaks[0])
	}
}

func TestPeakDecayDecaysOverTime(t *testing.T) {
	g, state, snap := seedGaugesAt(t, 90, 20, 60)
	peakAfterSpike := g.peaks[0]

	// Drop CPU to 0 and pump enough ticks to flush the visible window
	snap.System.CPUPercent = 0
	state.Update(*snap)
	g.Update(state)
	for i := 0; i < 120; i++ {
		g.AnimTick()
	}

	if g.peaks[0] >= peakAfterSpike {
		t.Errorf("peak should decay: before=%v, after=%v", peakAfterSpike, g.peaks[0])
	}
}

func TestPeakDecayRateMatchesProfile(t *testing.T) {
	g, state, snap := seedGaugesAt(t, 95, 10, 60)

	// Drop to 0, flush the visible window so windowPeak falls to ~0
	snap.System.CPUPercent = 0
	state.Update(*snap)
	g.Update(state)
	for i := 0; i < 120; i++ {
		g.AnimTick()
	}

	// Now the peak is decaying freely (windowPeak ≈ 0, well below peaks[0])
	peakBefore := g.peaks[0]
	g.AnimTick()
	peakAfter := g.peaks[0]

	if peakBefore < 0.1 {
		t.Skipf("peak already decayed too far to measure: %v", peakBefore)
	}

	ratio := peakAfter / peakBefore
	expected := testAnim.PeakDecayRate
	if math.Abs(ratio-expected) > 0.01 {
		t.Errorf("decay ratio = %v, want ~%v (PeakDecayRate)", ratio, expected)
	}
}

func TestViewLinesNilWhenNoState(t *testing.T) {
	g := NewGauges(testTheme, testAnim)
	g.SetSize(120, 6)
	if got := g.ViewLines(10); got != nil {
		t.Errorf("ViewLines with no state = %d lines, want nil", len(got))
	}
}

// TestDefaultVisibleSparklineSet pins the cold-start visible set so a
// future refactor that changes the default trips a clear signal. CPU,
// MEM, Token are visible; Decomp ("SWAP") is available via toggle but
// hidden by default.
func TestDefaultVisibleSparklineSet(t *testing.T) {
	g := NewGauges(testTheme, testAnim)
	wantVisible := map[SparklineSlot]bool{
		SlotCPU:    true,
		SlotMEM:    true,
		SlotDecomp: false,
		SlotToken:  true,
		SlotPretty: false,
	}
	for slot, want := range wantVisible {
		if got := g.IsVisible(slot); got != want {
			t.Errorf("IsVisible(%v) = %v, want %v", slot, got, want)
		}
	}
	if got := g.VisibleCount(); got != MaxVisibleSparklines {
		t.Errorf("default VisibleCount = %d, want %d", got, MaxVisibleSparklines)
	}
}

// TestToggleOffFreesSlot verifies a visible slot can be hidden and the
// visible count drops by one.
func TestToggleOffFreesSlot(t *testing.T) {
	g := NewGauges(testTheme, testAnim)
	before := g.VisibleCount()
	g.ToggleVisible(SlotCPU)
	if g.IsVisible(SlotCPU) {
		t.Fatalf("after toggle, SlotCPU still visible")
	}
	if got := g.VisibleCount(); got != before-1 {
		t.Errorf("VisibleCount after toggle off = %d, want %d", got, before-1)
	}
}

// TestToggleOnRoundTrip verifies toggle off + toggle on returns to the
// original visible state.
func TestToggleOnRoundTrip(t *testing.T) {
	g := NewGauges(testTheme, testAnim)
	g.ToggleVisible(SlotCPU)
	g.ToggleVisible(SlotCPU)
	if !g.IsVisible(SlotCPU) {
		t.Errorf("after toggle-off + toggle-on, SlotCPU should be visible again")
	}
	if got := g.VisibleCount(); got != MaxVisibleSparklines {
		t.Errorf("VisibleCount after round-trip = %d, want %d", got, MaxVisibleSparklines)
	}
}

// TestToggleOnWhenFullIsNoOp is the load-bearing invariant: with 3 slots
// visible, toggling on the 4th MUST NOT change state. The user has to
// free a slot first.
func TestToggleOnWhenFullIsNoOp(t *testing.T) {
	g := NewGauges(testTheme, testAnim)
	// Default visible set is {CPU, MEM, Token}; Decomp is hidden.
	if g.IsVisible(SlotDecomp) {
		t.Fatalf("test precondition: SlotDecomp should start hidden")
	}
	if g.VisibleCount() != MaxVisibleSparklines {
		t.Fatalf("test precondition: VisibleCount = %d, want %d", g.VisibleCount(), MaxVisibleSparklines)
	}

	g.ToggleVisible(SlotDecomp)
	if g.IsVisible(SlotDecomp) {
		t.Errorf("ToggleVisible(SlotDecomp) succeeded when stage was full — invariant violated")
	}
	if g.VisibleCount() != MaxVisibleSparklines {
		t.Errorf("VisibleCount after failed toggle-on = %d, want %d", g.VisibleCount(), MaxVisibleSparklines)
	}
}

// TestToggleOutOfRangeSlotIsNoOp guards against panics on slot indices
// outside [0, NumSparklineSlots).
func TestToggleOutOfRangeSlotIsNoOp(t *testing.T) {
	g := NewGauges(testTheme, testAnim)
	before := g.VisibleCount()
	g.ToggleVisible(SparklineSlot(-1))
	g.ToggleVisible(SparklineSlot(NumSparklineSlots))
	g.ToggleVisible(SparklineSlot(99))
	if g.VisibleCount() != before {
		t.Errorf("out-of-range toggles changed VisibleCount: %d → %d", before, g.VisibleCount())
	}
	if g.IsVisible(SparklineSlot(-1)) || g.IsVisible(SparklineSlot(99)) {
		t.Errorf("IsVisible should return false for out-of-range slots")
	}
}
