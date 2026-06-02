package collector

import "testing"

// computePercent and (*cpuSampler).sample are the pure, cgo-free core of the
// CPU sampler. These tests exercise the sleep/wake hardening: a zero delta,
// a non-monotonic (wrapped/reset) counter, and a failed mach read must all
// HOLD the last reading instead of emitting a false 0% or a garbage 100%.

func TestComputePercentFirstSample(t *testing.T) {
	// No previous snapshot (total == 0) → 0, and it's a real result.
	pct, ok := computePercent(cpuTicks{}, cpuTicks{User: 150, Idle: 150}, 99)
	if !ok || pct != 0 {
		t.Errorf("first sample = (%v, %v), want (0, true)", pct, ok)
	}
}

func TestComputePercentNormal(t *testing.T) {
	prev := cpuTicks{User: 100, Idle: 100} // total 200, busy 100
	cur := cpuTicks{User: 150, Idle: 150}  // total 300, busy 150 → delta 50/100
	pct, ok := computePercent(prev, cur, 0)
	if !ok || pct != 50 {
		t.Errorf("normal delta = (%v, %v), want (50, true)", pct, ok)
	}
}

func TestComputePercentZeroDeltaHolds(t *testing.T) {
	prev := cpuTicks{User: 100, Idle: 100}
	cur := prev // ticks unchanged between reads (common at 150ms)
	pct, ok := computePercent(prev, cur, 37)
	if ok || pct != 37 {
		t.Errorf("zero delta = (%v, %v), want (37, false) — hold last", pct, ok)
	}
}

func TestComputePercentBackwardsHolds(t *testing.T) {
	// Counter went backwards (sleep/wake, CPU offline/online). Must NOT wrap to
	// a huge unsigned delta and emit garbage — hold the last reading.
	prev := cpuTicks{User: 300, Idle: 300} // total 600
	cur := cpuTicks{User: 100, Idle: 100}  // total 200 < prev
	pct, ok := computePercent(prev, cur, 42)
	if ok || pct != 42 {
		t.Errorf("backwards delta = (%v, %v), want (42, false) — hold last", pct, ok)
	}
}

func TestComputePercentClampsAt100(t *testing.T) {
	prev := cpuTicks{Idle: 100}           // total 100, busy 0
	cur := cpuTicks{User: 200, Idle: 100} // total 300, busy 200 → 200/200 = 100%
	pct, ok := computePercent(prev, cur, 0)
	if !ok || pct != 100 {
		t.Errorf("full-busy delta = (%v, %v), want (100, true)", pct, ok)
	}
}

func TestSamplerFailedReadDoesNotPoisonPrev(t *testing.T) {
	s := &cpuSampler{}
	s.sample(cpuTicks{User: 100, Idle: 100}, true) // first → 0, prev set
	if got := s.sample(cpuTicks{User: 150, Idle: 150}, true); got != 50 {
		t.Fatalf("setup: second sample = %v, want 50", got)
	}
	// Failed mach read: hold last, and crucially do NOT store the zero ticks
	// as prev (that would make the NEXT read diff against 0 and return a false 0).
	if got := s.sample(cpuTicks{}, false); got != 50 {
		t.Errorf("failed read = %v, want 50 (held)", got)
	}
	// Next good read must diff against the last GOOD snapshot, not the failed zero.
	if got := s.sample(cpuTicks{User: 200, Idle: 200}, true); got != 50 {
		t.Errorf("post-failure read = %v, want 50 (prev not poisoned)", got)
	}
}

func TestSamplerZeroDeltaHolds(t *testing.T) {
	s := &cpuSampler{}
	s.sample(cpuTicks{User: 100, Idle: 100}, true) // first → 0
	s.sample(cpuTicks{User: 150, Idle: 150}, true) // → 50
	if got := s.sample(cpuTicks{User: 150, Idle: 150}, true); got != 50 {
		t.Errorf("unchanged ticks = %v, want 50 (held)", got)
	}
}
