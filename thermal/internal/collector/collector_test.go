package collector

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// fakeFast returns a CPU-only snapshot with the given CPU%.
func fakeFast(cpu float64) FastCollector {
	return func() Snapshot {
		return Snapshot{
			Timestamp: time.Now(),
			System:    SystemStats{CPUPercent: cpu},
		}
	}
}

// fakeProc returns a ProcSampler that records its call count and yields a
// recognizable marker (DesktopRunning + one session) so a test can confirm the
// proc data reached a snapshot.
func fakeProc(calls *atomic.Int32) ProcSampler {
	return func(ctx context.Context) ProcSample {
		calls.Add(1)
		return ProcSample{DesktopRunning: true, Sessions: []SessionTree{{RootPID: 1}}}
	}
}

// fakeSlow returns fixed SystemStats.
func fakeSlow(mem int64) SlowCollector {
	return func(ctx context.Context) SystemStats {
		return SystemStats{MemUsedBytes: mem}
	}
}

// fakeNet returns a fixed online state.
func fakeNet(online bool) NetChecker {
	return func(ctx context.Context) bool { return online }
}

func TestRunWith_DeliversSnapshots(t *testing.T) {
	snapCh := make(chan Snapshot, 16)
	done := make(chan struct{})

	cfg := RunConfig{
		FastCollect: fakeFast(42.0),
		SlowCollect: fakeSlow(1024),
		NetCheck:    fakeNet(true),
	}

	go RunWith(snapCh, 10*time.Millisecond, done, cfg)

	var snaps []Snapshot
	timeout := time.After(200 * time.Millisecond)
	for len(snaps) < 3 {
		select {
		case s, ok := <-snapCh:
			if !ok {
				t.Fatal("snapCh closed before 3 snapshots")
			}
			snaps = append(snaps, s)
		case <-timeout:
			t.Fatalf("timed out waiting for snapshots, got %d", len(snaps))
		}
	}
	close(done)

	for i, s := range snaps {
		if s.System.CPUPercent != 42.0 {
			t.Errorf("snap[%d] CPU = %v, want 42.0", i, s.System.CPUPercent)
		}
	}
}

func TestRunWith_MergesSlowStats(t *testing.T) {
	snapCh := make(chan Snapshot, 16)
	done := make(chan struct{})

	cfg := RunConfig{
		FastCollect: fakeFast(10.0),
		SlowCollect: fakeSlow(8 * 1024 * 1024 * 1024), // 8GB
		NetCheck:    fakeNet(true),
	}

	go RunWith(snapCh, 10*time.Millisecond, done, cfg)

	timeout := time.After(3 * time.Second)
	var found bool
	for !found {
		select {
		case s, ok := <-snapCh:
			if !ok {
				t.Fatal("snapCh closed before slow stats merged")
			}
			if s.System.MemUsedBytes != 0 && s.Online {
				found = true
			}
		case <-timeout:
			t.Fatal("timed out waiting for slow stats to merge")
		}
	}
	close(done)
}

func TestRunWith_CollectErrsFlowThrough(t *testing.T) {
	snapCh := make(chan Snapshot, 16)
	done := make(chan struct{})

	cfg := RunConfig{
		FastCollect: func() Snapshot {
			return Snapshot{Timestamp: time.Now(), CollectErrs: []string{"procs: timeout"}}
		},
		SlowCollect: fakeSlow(0),
		NetCheck:    fakeNet(true),
	}

	go RunWith(snapCh, 10*time.Millisecond, done, cfg)

	timeout := time.After(200 * time.Millisecond)
	select {
	case s, ok := <-snapCh:
		if !ok {
			t.Fatal("snapCh closed unexpectedly")
		}
		if len(s.CollectErrs) == 0 {
			t.Error("expected CollectErrs to be populated")
		}
	case <-timeout:
		t.Fatal("timed out waiting for snapshot")
	}
	close(done)
}

func TestRunWith_TracksSlowAge(t *testing.T) {
	snapCh := make(chan Snapshot, 16)
	done := make(chan struct{})

	cfg := RunConfig{
		FastCollect: fakeFast(10.0),
		SlowCollect: fakeSlow(1024),
		NetCheck:    fakeNet(true),
	}

	go RunWith(snapCh, 10*time.Millisecond, done, cfg)

	// First snapshot: SlowAge == 0 (no slow data yet — zero value is the sentinel)
	timeout := time.After(200 * time.Millisecond)
	select {
	case s := <-snapCh:
		if s.SlowAge != 0 {
			t.Errorf("first snapshot SlowAge = %v, want 0 (no slow data yet)", s.SlowAge)
		}
	case <-timeout:
		t.Fatal("timed out")
	}

	// After slow loop runs (~1s), SlowAge should be positive but small
	timeout2 := time.After(3 * time.Second)
	var found bool
	for !found {
		select {
		case s, ok := <-snapCh:
			if !ok {
				t.Fatal("snapCh closed before slow loop ran")
			}
			if s.SlowAge > 0 && s.SlowAge < 500*time.Millisecond {
				found = true
			}
		case <-timeout2:
			t.Fatal("timed out waiting for SlowAge to reflect slow loop completion")
		}
	}
	close(done)
}

func TestRunWith_SlowLoopRunsConcurrently(t *testing.T) {
	snapCh := make(chan Snapshot, 16)
	done := make(chan struct{})

	// Track peak concurrency: both collectors increment inflight on entry,
	// decrement on exit. If they run concurrently, peak reaches 2.
	var inflight atomic.Int32
	var peak atomic.Int32
	gate := make(chan struct{}) // blocks both collectors until both are inflight

	cfg := RunConfig{
		FastCollect: fakeFast(1.0),
		SlowCollect: func(ctx context.Context) SystemStats {
			cur := inflight.Add(1)
			for cur > peak.Load() {
				peak.Store(cur)
			}
			<-gate // wait for signal
			inflight.Add(-1)
			return SystemStats{MemUsedBytes: 999}
		},
		NetCheck: func(ctx context.Context) bool {
			cur := inflight.Add(1)
			for cur > peak.Load() {
				peak.Store(cur)
			}
			<-gate // wait for signal
			inflight.Add(-1)
			return true
		},
	}

	go RunWith(snapCh, 10*time.Millisecond, done, cfg)

	// Wait until both collectors are inflight (peak == 2), then release them.
	timeout := time.After(5 * time.Second)
	for peak.Load() < 2 {
		select {
		case <-timeout:
			t.Fatalf("timed out waiting for concurrent execution, peak inflight = %d", peak.Load())
		default:
			time.Sleep(time.Millisecond)
		}
	}
	close(gate)

	// Drain until we see merged results
	for {
		select {
		case s, ok := <-snapCh:
			if !ok {
				t.Fatal("snapCh closed before slow stats merged")
			}
			if s.System.MemUsedBytes == 999 && s.Online {
				close(done)
				return
			}
		case <-timeout:
			t.Fatal("timed out waiting for merged slow stats")
		}
	}
}

// TestRunWith_ProcsCollectedOnSlowCadence proves the process-tree scan is
// decoupled from the fast CPU cadence: proc data still reaches every snapshot
// (merged from the slow-loop cache, like the other slow stats), but the proc
// collector runs on the slow cadence (~1s) while the fast CPU collector runs
// on the injected fast interval — so proc forks happen far less often.
func TestRunWith_ProcsCollectedOnSlowCadence(t *testing.T) {
	snapCh := make(chan Snapshot, 64)
	done := make(chan struct{})
	defer close(done)

	var fastCalls, procCalls atomic.Int32
	cfg := RunConfig{
		FastCollect: func() Snapshot {
			fastCalls.Add(1)
			return Snapshot{Timestamp: time.Now(), System: SystemStats{CPUPercent: 5}}
		},
		SlowCollect: fakeSlow(0),
		NetCheck:    fakeNet(true),
		ProcCollect: fakeProc(&procCalls),
	}

	go RunWith(snapCh, 10*time.Millisecond, done, cfg)

	// Wait until a snapshot carries the merged proc marker — proves proc data
	// reaches snapshots though collected on the slow loop.
	timeout := time.After(3 * time.Second)
	var merged bool
	for !merged {
		select {
		case s, ok := <-snapCh:
			if !ok {
				t.Fatal("snapCh closed before proc data merged")
			}
			if s.DesktopRunning && len(s.Sessions) == 1 {
				merged = true
			}
		case <-timeout:
			t.Fatal("timed out waiting for proc data to merge into snapshot")
		}
	}

	// By the time the 1s slow loop fired once, the 10ms fast loop fired ~100×.
	// Assert the cadence is genuinely decoupled (fast >> proc).
	fc, pc := fastCalls.Load(), procCalls.Load()
	if pc == 0 {
		t.Fatal("proc collector never ran")
	}
	if fc < pc*5 {
		t.Errorf("procs not decoupled from fast cadence: fastCalls=%d procCalls=%d (want fast >> proc)", fc, pc)
	}
}

func TestRunWith_ClosesChannelOnDone(t *testing.T) {
	snapCh := make(chan Snapshot, 16)
	done := make(chan struct{})

	cfg := RunConfig{
		FastCollect: fakeFast(1.0),
		SlowCollect: fakeSlow(0),
		NetCheck:    fakeNet(false),
	}

	go RunWith(snapCh, 10*time.Millisecond, done, cfg)

	time.Sleep(50 * time.Millisecond)
	close(done)

	timeout := time.After(time.Second)
	for {
		select {
		case _, ok := <-snapCh:
			if !ok {
				return // success
			}
		case <-timeout:
			t.Fatal("snapCh not closed after done signal")
		}
	}
}
