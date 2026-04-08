// Package collector gathers system stats and Claude process trees
// via cgo, subprocess calls, and JSONL event tailing.
package collector

import (
	"context"
	"sync"
	"time"

	"github.com/toddwshaffer/coolant/thermal/internal/config"
)

// ── Dependency injection for testability ───────────────────

// FastCollector gathers CPU + process trees (no subprocesses for CPU).
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
//   - Fast loop (interval): CPU (cgo, no subprocess) + procs — drives sparklines.
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

	// Fast loop: CPU (cgo) + procs
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

// collectFast gathers CPU stats (cgo, no subprocess) and process trees.
func collectFast(pc *ProcCollector) Snapshot {
	ctx, cancel := context.WithTimeout(context.Background(), config.CollectTimeout)
	defer cancel()
	now := time.Now()

	type procResult struct {
		sessions []SessionTree
		allProcs []ProcessInfo
		err      error
	}

	procCh := make(chan procResult, 1)

	go func() {
		sessions, allProcs, err := pc.Collect(ctx)
		procCh <- procResult{sessions, allProcs, err}
	}()

	// CPU collection is synchronous — it's a single cgo call, no subprocess
	stats := CollectCPU()

	snap := Snapshot{
		Timestamp: now,
		System:    stats,
	}

	select {
	case r := <-procCh:
		if r.err == nil {
			snap.Sessions = r.sessions
			snap.AllProcs = r.allProcs
			snap.DesktopRunning = pc.DesktopRunning
			snap.ChromeHostRunning = pc.ChromeHostRunning
		} else {
			snap.CollectErrs = append(snap.CollectErrs, "procs: "+r.err.Error())
		}
	case <-ctx.Done():
		return snap
	}

	return snap
}
