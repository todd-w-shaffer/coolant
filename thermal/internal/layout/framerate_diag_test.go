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
			// A charging battery so the battery cell renders — this puts the
			// charging-breath oscillator in-frame, so the byte-stable load-test
			// actually exercises its freeze (not just the battery unit test).
			BatteryPresent: true,
			BatteryState:   collector.BatteryCharging,
			BatteryPercent: 80,
		},
		Sessions:  []collector.SessionTree{{RootPID: 1, RootComm: "claude"}},
		Online:    true,
		Timestamp: time.Now(),
	}
}

// driveSecond advances AnimTick+View for one simulated second and returns the
// count of frames whose View() differed from the prior. The v2 approach never
// stops the tick — idle quiescence comes from freezing the breath + bubbletea's
// identical-frame suppression (Phase 2), not from halting the loop.
func driveSecond(h *Horizontal) int {
	prev := h.View()
	distinct := 0
	for i := 0; i < config.AnimFPS; i++ {
		h.AnimTick()
		cur := h.View()
		if cur != prev {
			distinct++
		}
		prev = cur
	}
	return distinct
}

// TestStartupRevealAnimatesAtIdle guards the v1 regression: the sparkline must
// keep filling in at idle (the startup reveal), NOT freeze half-drawn. A fresh
// widget has an empty renderHistory, so each AnimTick that appends a sample
// changes the rendered sparkline — frames churn even with no agents and flat
// metrics, because the tick is never stopped.
func TestStartupRevealAnimatesAtIdle(t *testing.T) {
	h := NewHorizontal(testTheme, testAnim, keys.Default())
	h.SetSize(120, 10)
	h.State().Update(calmSteadySnap())
	h.Update(h.State())

	distinct := driveSecond(h)
	t.Logf("startup reveal at idle: %d/%d distinct frames", distinct, config.AnimFPS)
	if distinct < config.AnimFPS/2 {
		t.Errorf("startup reveal: %d/%d distinct frames, want > half (sparkline must fill, not freeze)", distinct, config.AnimFPS)
	}
}

// TestIdleByteStableUnderJitter is the v2 Phase-2 load-test. Once the dashboard
// is filled, settled, and calm with the breath frozen, the entire composed
// View must be byte-identical frame-to-frame EVEN as CPU jitters within the
// display deadband — so bubbletea suppresses the frame and idle terminal
// repaints fall to zero. If ANY widget still animates at idle (breath not
// frozen, LCD temp flipping on raw-CPU jitter, …), distinct > 0 and this fails.
func TestIdleByteStableUnderJitter(t *testing.T) {
	h := NewHorizontal(testTheme, testAnim, keys.Default())
	h.SetSize(120, 10)

	// Fill the sparklines and let springs settle while flat metrics latch calm.
	for i := 0; i < config.MaxRenderHistory+config.CalmStableSnapshots+5; i++ {
		h.State().Update(calmSteadySnap())
		h.Update(h.State())
		h.AnimTick()
	}
	if !h.State().IsCalm() {
		t.Fatalf("expected calm after fill+settle")
	}

	// Drive a second of frames with sub-deadband CPU jitter (all within
	// DisplayDeadbandPct of the baseline 5), updating each frame.
	jitter := []float64{4, 6, 5, 7, 4, 6}
	prev := h.View()
	distinct := 0
	for i := 0; i < config.AnimFPS; i++ {
		snap := calmSteadySnap()
		snap.System.CPUPercent = jitter[i%len(jitter)]
		h.State().Update(snap)
		h.Update(h.State())
		h.AnimTick()
		cur := h.View()
		if cur != prev {
			distinct++
		}
		prev = cur
	}
	t.Logf("idle under jitter: %d/%d distinct frames", distinct, config.AnimFPS)
	if distinct != 0 {
		t.Errorf("idle under jitter: %d/%d distinct frames, want 0 (breath frozen + deadband → byte-stable)", distinct, config.AnimFPS)
	}
}

// TestTokenActivityAnimatesAtIdle guards the other v1 regression: a streaming
// token sparkline must keep scrolling, not freeze mid-scroll during a lull.
func TestTokenActivityAnimatesAtIdle(t *testing.T) {
	h := NewHorizontal(testTheme, testAnim, keys.Default())
	h.SetSize(120, 10)
	// Establish a filled, settled window first.
	for i := 0; i < config.AnimFPS; i++ {
		h.State().Update(calmSteadySnap())
		h.Update(h.State())
		h.AnimTick()
	}
	// Now stream tokens.
	burst := calmSteadySnap()
	burst.Tokens.IOTokensPerSec = 1500
	h.State().Update(burst)
	h.Update(h.State())

	distinct := driveSecond(h)
	t.Logf("token activity at idle: %d/%d distinct frames", distinct, config.AnimFPS)
	if distinct == 0 {
		t.Errorf("token activity: 0 distinct frames, want > 0 (token sparkline must scroll)")
	}
}
