package collector

import (
	"context"
	"testing"
	"time"

	"github.com/toddwshaffer/coolant/thermal/internal/config"
)

// fakeFast returns a fixed snapshot with the given CPU%.
func fakeFast(cpu float64) FastCollector {
	return func(pc *ProcCollector) Snapshot {
		return Snapshot{
			Timestamp: time.Now(),
			System:    SystemStats{CPUPercent: cpu},
		}
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
		FastCollect: func(pc *ProcCollector) Snapshot {
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

	slowDelay := 200 * time.Millisecond

	cfg := RunConfig{
		FastCollect: fakeFast(1.0),
		SlowCollect: func(ctx context.Context) SystemStats {
			time.Sleep(slowDelay)
			return SystemStats{MemUsedBytes: 999}
		},
		NetCheck: func(ctx context.Context) bool {
			time.Sleep(slowDelay)
			return true
		},
	}

	go RunWith(snapCh, 10*time.Millisecond, done, cfg)

	// Wait for slow stats to merge (online=true AND mem=999)
	start := time.Now()
	timeout := time.After(3 * time.Second)
	var found bool
	for !found {
		select {
		case s, ok := <-snapCh:
			if !ok {
				t.Fatal("snapCh closed before slow stats merged")
			}
			if s.System.MemUsedBytes == 999 && s.Online {
				found = true
			}
		case <-timeout:
			t.Fatal("timed out waiting for concurrent slow stats")
		}
	}
	elapsed := time.Since(start)
	close(done)

	// The slow loop fires on config.SlowInterval (1s) ticker, then runs both
	// collectors concurrently. If sequential, total >= SlowInterval + 2*slowDelay.
	// If concurrent, total ~= SlowInterval + 1*slowDelay.
	seqMinimum := time.Duration(config.SlowInterval) + 2*slowDelay
	if elapsed >= seqMinimum {
		t.Errorf("slow loop took %v (>= %v), suggesting sequential execution (want concurrent)", elapsed, seqMinimum)
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
