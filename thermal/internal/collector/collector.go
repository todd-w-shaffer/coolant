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

// SlowCollector gathers a slow-changing stat group via subprocesses. Used for
// the fixed-1s swap+vm_stat group and the slow-cadence battery group.
type SlowCollector func(ctx context.Context) SystemStats

// GPUCollector reads GPU utilization %. Runs on the adaptive GPU cadence.
// ok is false on a read/parse failure, so the scheduler keeps the last value
// and retries instead of treating the flake as a genuine idle reading.
type GPUCollector func(ctx context.Context) (pct float64, ok bool)

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
// OTELView is optional — when non-nil it enables the live-throughput fan-in
// inside TokenCollector. Nil keeps the transcript-only behavior that matches
// the bind-failure-degrade path.
type RunConfig struct {
	FastCollect    FastCollector
	SlowCollect    SlowCollector // swap + vm_stat, every base tick (fixed 1s)
	GPUCollect     GPUCollector  // GPU %, adaptive cadence (backs off when flat)
	BatteryCollect SlowCollector // battery fields, slow cadence (BatteryInterval)
	ProcCollect    ProcSampler
	NetCheck       NetChecker
	OTELView       OTELTokenView
}

// DefaultRunConfig returns production dependencies. OTELView stays nil
// so callers can attach the CC adapter post-construction (it lives in
// a different package the collector intentionally doesn't import).
func DefaultRunConfig() RunConfig {
	// Pre-allocate reusable maps for process collection (cleared each tick).
	// The collector is driven only from the slow loop, so reuse is safe.
	pc := &ProcCollector{
		children: make(map[int][]int, 512),
		byPID:    make(map[int]rawProc, 512),
	}
	return RunConfig{
		FastCollect:    collectFast,
		SlowCollect:    CollectSwapVM,
		GPUCollect:     CollectGPU,
		BatteryCollect: CollectBattery,
		ProcCollect:    collectProcs(pc),
		NetCheck:       CheckOnline,
	}
}

// Run starts two decoupled collector loops:
//   - Fast loop (interval): CPU (cgo, no subprocess) — drives the CPU sparkline.
//   - Slow loop (1s base tick): swap/vm_stat + token tail + process-tree scan
//     every tick; network, GPU, and battery on their own longer adaptive
//     cadence (see slowSchedule). Cached results merge into each fast Snapshot.
//
// Both loops send Snapshots to ch. The fast loop carries last-known slow stats
// and online state so every snapshot is complete. otelView may be nil — in
// that case the collector runs in transcript-only mode (identical to the
// behavior when the OTEL receiver fails to bind).
func Run(ch chan<- Snapshot, interval time.Duration, done <-chan struct{}, otelView OTELTokenView) {
	cfg := DefaultRunConfig()
	cfg.OTELView = otelView
	RunWith(ch, interval, done, cfg)
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
	var tcOpts []TokenCollectorOption
	if cfg.OTELView != nil {
		tcOpts = append(tcOpts, WithOTELView(cfg.OTELView))
	}
	tokenCollector := NewTokenCollector(tcOpts...)

	// Nil sub-collectors (tests exercising only a subset) become no-ops, so the
	// always-run base group (swap/vm_stat + token tail + procs) stays
	// unconditional. The adaptive sources (net/GPU/battery) are run-when-due, so
	// a nil one is simply never due.
	slowCollect := cfg.SlowCollect
	if slowCollect == nil {
		slowCollect = func(context.Context) SystemStats { return SystemStats{} }
	}
	procCollect := cfg.ProcCollect
	if procCollect == nil {
		procCollect = func(context.Context) ProcSample { return ProcSample{} }
	}

	// Slow loop: ticks at SlowInterval (1s). The base group (swap/vm_stat,
	// tokens, procs) runs every tick; network, GPU, and battery poll on their
	// own longer adaptive cadence (see slowSchedule) so an idle laptop forks
	// ioreg/pmset/net far less often. Per-field cache merge: each source
	// refreshes only its own fields, so sources skipped this tick keep their
	// last value.
	go func() {
		ticker := time.NewTicker(config.SlowInterval)
		defer ticker.Stop()
		sched := newSlowSchedule()
		localOnline := false // goroutine-local mirror that drives the net cadence
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				now := time.Now()
				netDue, gpuDue, battDue := sched.due(now, localOnline)
				runNet := netDue && cfg.NetCheck != nil
				runGPU := gpuDue && cfg.GPUCollect != nil
				runBatt := battDue && cfg.BatteryCollect != nil

				ctx, cancel := context.WithTimeout(context.Background(), config.CollectTimeout)

				var (
					netResult    bool
					swapVM       SystemStats
					gpuResult    float64
					gpuOK        bool
					battResult   SystemStats
					tokensResult TokenStats
					procResult   ProcSample
				)
				var wg sync.WaitGroup

				// wg.Add(1) immediately before each go — the count can't desync
				// from the goroutines the way a precomputed total would.
				wg.Add(1)
				go func() {
					defer wg.Done()
					swapVM = slowCollect(ctx)
				}()
				wg.Add(1)
				go func() {
					defer wg.Done()
					tokensResult = tokenCollector.Tick(time.Now())
				}()
				wg.Add(1)
				go func() {
					defer wg.Done()
					procResult = procCollect(ctx)
				}()
				if runNet {
					wg.Add(1)
					go func() {
						defer wg.Done()
						netCtx, netCancel := context.WithTimeout(ctx, config.NetCheckTimeout)
						netResult = cfg.NetCheck(netCtx)
						netCancel()
					}()
				}
				if runGPU {
					wg.Add(1)
					go func() {
						defer wg.Done()
						gpuResult, gpuOK = cfg.GPUCollect(ctx)
					}()
				}
				if runBatt {
					wg.Add(1)
					go func() {
						defer wg.Done()
						battResult = cfg.BatteryCollect(ctx)
					}()
				}
				wg.Wait()
				cancel()

				mu.Lock()
				// swap/vm_stat refresh every tick (fixed cadence — the
				// decompression counter is a per-tick delta).
				slow.SwapUsedBytes = swapVM.SwapUsedBytes
				slow.SwapTotalBytes = swapVM.SwapTotalBytes
				slow.MemUsedBytes = swapVM.MemUsedBytes
				slow.Decompressions = swapVM.Decompressions
				if runNet {
					online = netResult
					localOnline = netResult
				}
				if runGPU && gpuOK {
					slow.GPUPercent = gpuResult
				}
				if runBatt {
					slow.BatteryPresent = battResult.BatteryPresent
					slow.BatteryPercent = battResult.BatteryPercent
					slow.BatteryState = battResult.BatteryState
					slow.BatteryTimeRemaining = battResult.BatteryTimeRemaining
				}
				tokens = tokensResult
				procs = procResult
				procSeq++
				lastSlowSuccess = now
				mu.Unlock()

				// Record runs and re-derive the GPU cadence from its reading.
				// (slowSchedule is goroutine-local, so no lock needed.)
				if runNet {
					sched.markNet(now)
				}
				if runGPU && gpuOK {
					// Only a successful read advances the GPU cadence. A failed
					// read leaves lastGPU unmoved, so GPU stays due and retries on
					// the next base tick instead of backing off on a flake.
					sched.markGPU(now, gpuResult)
				}
				if runBatt {
					sched.markBattery(now)
				}
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
