// Package collector gathers system stats and Claude process trees:
// the fast loop via cgo kernel reads, the slow loop via subprocess
// calls, and agent events via JSONL tailing.
package collector

import (
	"context"
	"sync"
	"time"

	"github.com/toddwshaffer/coolant/thermal/internal/config"
)

// ── Dependency injection for testability ───────────────────

// FastCollector gathers CPU + process trees (no subprocesses).
type FastCollector func(pc *ProcCollector) Snapshot

// SlowCollector gathers swap/vm_stat/GPU via subprocesses.
type SlowCollector func(ctx context.Context) SystemStats

// NetChecker probes API reachability.
type NetChecker func(ctx context.Context) bool

// RunConfig holds injectable dependencies for the collector loops.
type RunConfig struct {
	FastCollect FastCollector
	SlowCollect SlowCollector
	NetCheck    NetChecker
}

// DefaultRunConfig returns production dependencies.
func DefaultRunConfig() RunConfig {
	return RunConfig{
		FastCollect: collectFast,
		SlowCollect: CollectSlowStats,
		NetCheck:    CheckOnline,
	}
}

// Run starts two decoupled collector loops:
//   - Fast loop (interval): CPU + procs (cgo, no subprocess) — drives sparklines.
//   - Slow loop (1s): network reachability + swap/vm_stat/GPU (subprocess-heavy
//     stats that change slowly). Cached results merge into each fast-loop Snapshot.
//
// Both loops send Snapshots to ch. The fast loop carries last-known slow stats
// and online state so every snapshot is complete.
func Run(ch chan<- Snapshot, interval time.Duration, done <-chan struct{}) {
	RunWith(ch, interval, done, DefaultRunConfig())
}

// RunWith is the testable core of Run — same behavior, injectable dependencies.
func RunWith(ch chan<- Snapshot, interval time.Duration, done <-chan struct{}, cfg RunConfig) {
	defer close(ch)

	var (
		mu              sync.Mutex
		online          bool        // last-known network state
		slow            SystemStats // last-known subprocess-heavy stats
		lastSlowSuccess time.Time   // zero until first slow loop completes
	)

	// Slow loop: network + swap/vm_stat/GPU at 1s
	go func() {
		netTicker := time.NewTicker(config.SlowInterval)
		defer netTicker.Stop()
		for {
			select {
			case <-done:
				return
			case <-netTicker.C:
				ctx, cancel := context.WithTimeout(context.Background(), config.CollectTimeout)

				// Run network check and slow stats concurrently
				var netResult bool
				var statsResult SystemStats
				var wg sync.WaitGroup

				wg.Add(2)
				go func() {
					defer wg.Done()
					netCtx, netCancel := context.WithTimeout(ctx, config.NetCheckTimeout)
					netResult = cfg.NetCheck(netCtx)
					netCancel()
				}()
				go func() {
					defer wg.Done()
					statsResult = cfg.SlowCollect(ctx)
				}()
				wg.Wait()
				cancel()

				mu.Lock()
				online = netResult
				slow = statsResult
				lastSlowSuccess = time.Now()
				mu.Unlock()
			}
		}
	}()

	// Pre-allocate reusable maps for process collection (cleared each tick).
	procCollector := &ProcCollector{
		children: make(map[int][]int, 512),
		byPID:    make(map[int]rawProc, 512),
	}

	// Fast loop: CPU + procs (cgo)
	fastTicker := time.NewTicker(interval)
	defer fastTicker.Stop()

	for {
		select {
		case <-done:
			return
		case <-fastTicker.C:
			snap := cfg.FastCollect(procCollector)
			mu.Lock()
			snap.Online = online
			snap.System.MemUsedBytes = slow.MemUsedBytes
			snap.System.SwapUsedBytes = slow.SwapUsedBytes
			snap.System.SwapTotalBytes = slow.SwapTotalBytes
			snap.System.Decompressions = slow.Decompressions
			snap.System.GPUPercent = slow.GPUPercent
			snap.System.BatteryPresent = slow.BatteryPresent
			snap.System.BatteryPercent = slow.BatteryPercent
			snap.System.BatteryState = slow.BatteryState
			snap.System.BatteryTimeRemaining = slow.BatteryTimeRemaining
			if !lastSlowSuccess.IsZero() {
				snap.SlowAge = time.Since(lastSlowSuccess)
			}
			mu.Unlock()
			select {
			case ch <- snap:
			case <-done:
				return
			}
		}
	}
}

// collectFast gathers CPU stats and process trees. Both are direct
// kernel reads (cgo), no subprocesses.
func collectFast(pc *ProcCollector) Snapshot {
	snap := Snapshot{
		Timestamp: time.Now(),
		System:    CollectCPU(),
	}
	snap.Sessions, snap.AllProcs = pc.Collect()
	snap.DesktopRunning = pc.DesktopRunning
	snap.ChromeHostRunning = pc.ChromeHostRunning
	return snap
}
