package layout

import (
	"testing"
	"time"

	"github.com/toddwshaffer/coolant/thermal/internal/collector"
	"github.com/toddwshaffer/coolant/thermal/internal/config"
	"github.com/toddwshaffer/coolant/thermal/internal/keys"
)

// TestDistinctFramesPerSecond is the Layer-1 falsifiable test for the
// thermo-adaptive-framerate plan. It proves the diagnosis: in a fully
// steady state (one snapshot pushed, then NO further data), the dashboard
// still emits a visually-distinct frame on (nearly) every animation tick,
// because breathing / tidal / KITT phase accumulators and gauge springs
// advance every AnimTick regardless of data freshness.
//
// It drives AnimTick()+View() for exactly AnimFPS iterations (= 1 simulated
// second) and counts how many frames differ from the prior frame. The count
// is the per-second terminal-repaint pressure that drives GPU load.
//
// This is a measurement harness, not a behavioral assertion — it only fails
// if the steady-state frame production collapses to a near-static stream,
// which would mean the diagnosis is wrong and the plan is built on sand.
func TestDistinctFramesPerSecond(t *testing.T) {
	h := NewHorizontal(testTheme, testAnim, keys.Default())
	h.SetSize(120, 10)

	// One steady-state snapshot: active session, fixed metrics. After this
	// we never call Update again — any frame churn is pure animation.
	snap := collector.Snapshot{
		System: collector.SystemStats{
			CPUPercent:    25,
			MemUsedBytes:  8 << 30,
			MemTotalBytes: 16 << 30,
			GPUPercent:    15,
			NCPUs:         8,
		},
		Sessions: []collector.SessionTree{{RootPID: 1, RootComm: "claude"}},
		AllProcs: []collector.ProcessInfo{
			{PID: 2, PPID: 1, TypeCode: "S", Comm: "bash"},
		},
		Online:    true,
		Timestamp: time.Now(),
	}
	h.State().Update(snap)
	h.Update(h.State())

	frames := config.AnimFPS // one simulated second
	prev := h.View()
	distinct := 0
	for i := 0; i < frames; i++ {
		h.AnimTick()
		cur := h.View()
		if cur != prev {
			distinct++
		}
		prev = cur
	}

	t.Logf("AnimFPS=%d: %d/%d distinct frames in 1 simulated second (steady state)",
		config.AnimFPS, distinct, frames)

	// Diagnosis sanity floor: a steady dashboard should churn most frames.
	// If it doesn't, the animation isn't the cost driver and the plan needs
	// rethinking. Threshold is deliberately loose (half the frames) — the
	// exact ratio is recorded via t.Logf for the before/after comparison.
	if distinct < frames/2 {
		t.Fatalf("steady-state churn %d/%d below half — diagnosis (animation drives "+
			"per-frame redraw) may be wrong; re-explore before lowering AnimFPS",
			distinct, frames)
	}
}

// calmSteadySnap is an agent-free, fixed-metric snapshot used to drive the
// dashboard into the calm state for the freeze tests below.
func calmSteadySnap() collector.Snapshot {
	return collector.Snapshot{
		System: collector.SystemStats{
			CPUPercent:    5,
			MemUsedBytes:  8 << 30,
			MemTotalBytes: 16 << 30,
			GPUPercent:    0,
			NCPUs:         8,
		},
		Sessions:  []collector.SessionTree{{RootPID: 1, RootComm: "claude"}},
		Online:    true,
		Timestamp: time.Now(),
	}
}

// driveCalmGatedSecond mirrors main's gated tick: it advances AnimTick ONLY
// while the state is not calm, for one simulated second, and returns the
// number of frames whose View() differed from the prior frame.
func driveCalmGatedSecond(h *Horizontal) int {
	prev := h.View()
	distinct := 0
	for i := 0; i < config.AnimFPS; i++ {
		if !h.State().IsCalm() {
			h.AnimTick()
		}
		cur := h.View()
		if cur != prev {
			distinct++
		}
		prev = cur
	}
	return distinct
}

// TestIdleFreezeStopsFrameProduction is the Phase-1 falsifiable load-test:
// once the dashboard is calm (no agents, flat metrics), gating AnimTick on
// !IsCalm() produces ZERO distinct frames — animation is frozen, so
// bubbletea's identical-frame suppression drives terminal writes to zero.
func TestIdleFreezeStopsFrameProduction(t *testing.T) {
	h := NewHorizontal(testTheme, testAnim, keys.Default())
	h.SetSize(120, 10)

	// Wind down to calm: identical agent-free snapshots, letting springs ease
	// (AnimTick) while still not-calm, exactly as production does.
	for i := 0; i < config.CalmStableSnapshots+1; i++ {
		h.State().Update(calmSteadySnap())
		h.Update(h.State())
		h.AnimTick()
	}
	if !h.State().IsCalm() {
		t.Fatalf("expected calm after priming %d steady snapshots", config.CalmStableSnapshots+1)
	}

	distinct := driveCalmGatedSecond(h)
	t.Logf("calm steady state: %d/%d distinct frames (gated)", distinct, config.AnimFPS)
	if distinct != 0 {
		t.Errorf("calm: %d/%d distinct frames, want 0 (animation should be frozen)", distinct, config.AnimFPS)
	}
}

// TestActivityWakesFrameProduction proves the wake path: after calm, a CPU
// spike resets the stability window, IsCalm() goes false, and the gated loop
// resumes producing frames.
func TestActivityWakesFrameProduction(t *testing.T) {
	h := NewHorizontal(testTheme, testAnim, keys.Default())
	h.SetSize(120, 10)

	for i := 0; i < config.CalmStableSnapshots+1; i++ {
		h.State().Update(calmSteadySnap())
		h.Update(h.State())
		h.AnimTick()
	}
	if !h.State().IsCalm() {
		t.Fatalf("expected calm after priming")
	}

	// Inject activity: a CPU spike.
	busy := calmSteadySnap()
	busy.System.CPUPercent = 85
	h.State().Update(busy)
	h.Update(h.State())
	if h.State().IsCalm() {
		t.Fatal("expected NOT calm immediately after a CPU spike")
	}

	distinct := driveCalmGatedSecond(h)
	t.Logf("after activity: %d/%d distinct frames (gated)", distinct, config.AnimFPS)
	if distinct < config.AnimFPS/2 {
		t.Errorf("after activity: %d/%d distinct frames, want > half (animation should resume)", distinct, config.AnimFPS)
	}
}
