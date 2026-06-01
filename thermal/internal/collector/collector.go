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

// FastCollector samples CPU via cgo (no subprocess). Runs on the fast loop.
type FastCollector func() Snapshot

// SlowCollector gathers swap/vm_stat/GPU via subprocesses.
type SlowCollector func(ctx context.Context) SystemStats

// NetChecker probes API reachability.
type NetChecker func(ctx context.Context) bool

// ProcSample is the process-tree result, collected on the slow cadence and
// merged into each fast snapshot like the other slow stats.
type ProcSample struct {
	Sessions          []SessionTree
	AllProcs          []ProcessInfo
	DesktopRunning    bool
	ChromeHostRunning bool
	Err               error
}

// ProcSampler gathers Claude process trees. Decoupled onto the slow loop — the
// tree changes on agent/build timescales, so re-scanning it at the fast CPU
// cadence forked `ps` ~6.6×/s for no gain.
type ProcSampler func(ctx context.Context) ProcSample

// RunConfig holds injectable dependencies for the collector loops.
// ProcCollect may be nil — the loop then delivers snapshots with no
// process-tree data (used by tests that only exercise the CPU/slow paths).
type RunConfig struct {
	FastCollect FastCollector
	SlowCollect SlowCollector
	ProcCollect ProcSampler
	NetCheck    NetChecker
}

// DefaultRunConfig returns production dependencies.
func DefaultRunConfig() RunConfig {
	// Pre-allocate reusable maps for process collection (cleared each tick).
	// The collector is driven only from the slow loop, so reuse is safe.
	pc := &ProcCollector{
		children: make(map[int][]int, 512),
		byPID:    make(map[int]rawProc, 512),
	}
	return RunConfig{
		FastCollect: collectFast,
		SlowCollect: CollectSlowStats,
		ProcCollect: collectProcs(pc),
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
		tokens          TokenStats  // last-known transcript token activity
		procs           ProcSample  // last-known process-tree scan (slow cadence)
		procSeq         uint64      // bumps each fresh proc scan so the model can dedup stale re-deliveries
		lastSlowSuccess time.Time   // zero until first slow loop completes
	)
	tokenCollector := NewTokenCollector()

	// A nil ProcCollect (tests that exercise only the CPU/slow paths) becomes a
	// no-op sampler, so the slow loop's fan-out stays unconditional instead of
	// re-deriving nil-ness across the WaitGroup count, the goroutine, and the merge.
	procCollect := cfg.ProcCollect
	if procCollect == nil {
		procCollect = func(context.Context) ProcSample { return ProcSample{} }
	}

	// Slow loop: network + swap/vm_stat/GPU + process-tree scan at 1s
	go func() {
		netTicker := time.NewTicker(config.SlowInterval)
		defer netTicker.Stop()
		for {
			select {
			case <-done:
				return
			case <-netTicker.C:
				ctx, cancel := context.WithTimeout(context.Background(), config.CollectTimeout)

				// Run network check, slow stats, token tail, and the process-tree
				// scan concurrently. Procs ride the slow loop now (was the fast
				// loop) — the tree changes too slowly to justify a 150ms `ps` fork.
				var netResult bool
				var statsResult SystemStats
				var tokensResult TokenStats
				var procResult ProcSample
				var wg sync.WaitGroup

				wg.Add(4)
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
				go func() {
					defer wg.Done()
					tokensResult = tokenCollector.Tick(time.Now())
				}()
				go func() {
					defer wg.Done()
					procResult = procCollect(ctx)
				}()
				wg.Wait()
				cancel()

				mu.Lock()
				online = netResult
				slow = statsResult
				tokens = tokensResult
				procs = procResult
				procSeq++
				lastSlowSuccess = time.Now()
				mu.Unlock()
			}
		}
	}()

	// Fast loop: CPU (cgo) only. Slow-cadence stats (swap/GPU/battery/procs)
	// merge in from the last-known cache below.
	fastTicker := time.NewTicker(interval)
	defer fastTicker.Stop()

	for {
		select {
		case <-done:
			return
		case <-fastTicker.C:
			snap := cfg.FastCollect()
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
			snap.Tokens = tokens
			// Process-tree data, collected on the slow loop.
			snap.Sessions = procs.Sessions
			snap.AllProcs = procs.AllProcs
			snap.DesktopRunning = procs.DesktopRunning
			snap.ChromeHostRunning = procs.ChromeHostRunning
			snap.ProcSeq = procSeq
			if procs.Err != nil {
				snap.CollectErrs = append(snap.CollectErrs, "procs: "+procs.Err.Error())
			}
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

// collectFast samples CPU via cgo (a single mach call, no subprocess). Process
// trees are gathered separately on the slow loop (see collectProcs) — they
// change too slowly to justify the fast cadence's `ps` fork rate.
func collectFast() Snapshot {
	return Snapshot{
		Timestamp: time.Now(),
		System:    CollectCPU(),
	}
}

// collectProcs wraps a ProcCollector into a ProcSampler. The collector reuses
// internal buffers across calls, so the returned sampler must be driven from a
// single goroutine (the slow loop) — never concurrently.
func collectProcs(pc *ProcCollector) ProcSampler {
	return func(ctx context.Context) ProcSample {
		sessions, allProcs, err := pc.Collect(ctx)
		return ProcSample{
			Sessions:          sessions,
			AllProcs:          allProcs,
			DesktopRunning:    pc.DesktopRunning,
			ChromeHostRunning: pc.ChromeHostRunning,
			Err:               err,
		}
	}
}
