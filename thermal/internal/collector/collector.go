package collector

import (
	"context"
	"sync"
	"time"
)

// Run starts two decoupled collector loops:
//   - Fast loop (interval): CPU, memory, swap, procs — drives sparklines.
//   - Slow loop (1s): network reachability — online/offline state changes
//     are measured in minutes, not milliseconds.
//
// Both loops send Snapshots to ch. The fast loop carries the last-known
// online state so every snapshot is complete.
func Run(ch chan<- Snapshot, interval time.Duration, done <-chan struct{}) {
	defer close(ch)

	var (
		mu     sync.Mutex
		online bool // last-known network state
	)

	// Slow loop: network check at 1s
	go func() {
		netTicker := time.NewTicker(1 * time.Second)
		defer netTicker.Stop()
		for {
			select {
			case <-done:
				return
			case <-netTicker.C:
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				result := CheckOnline(ctx)
				cancel()
				mu.Lock()
				online = result
				mu.Unlock()
			}
		}
	}()

	// Fast loop: system stats + procs
	fastTicker := time.NewTicker(interval)
	defer fastTicker.Stop()

	for {
		select {
		case <-done:
			return
		case <-fastTicker.C:
			snap := collectFast()
			mu.Lock()
			snap.Online = online
			mu.Unlock()
			select {
			case ch <- snap:
			case <-done:
				return
			}
		}
	}
}

// collectFast gathers system stats and process trees (no network check).
func collectFast() Snapshot {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	now := time.Now()

	type sysResult struct {
		stats SystemStats
		err   error
	}
	type procResult struct {
		sessions []SessionTree
		allProcs []ProcessInfo
		err      error
	}

	sysCh := make(chan sysResult, 1)
	procCh := make(chan procResult, 1)

	go func() {
		stats, err := CollectSystem(ctx)
		sysCh <- sysResult{stats, err}
	}()
	go func() {
		sessions, allProcs, err := CollectProcs(ctx)
		procCh <- procResult{sessions, allProcs, err}
	}()

	snap := Snapshot{
		Timestamp: now,
	}

	select {
	case r := <-sysCh:
		if r.err == nil {
			snap.System = r.stats
		} else {
			snap.CollectErrs = append(snap.CollectErrs, "system: "+r.err.Error())
		}
	case <-ctx.Done():
		return snap
	}

	select {
	case r := <-procCh:
		if r.err == nil {
			snap.Sessions = r.sessions
			snap.AllProcs = r.allProcs
		} else {
			snap.CollectErrs = append(snap.CollectErrs, "procs: "+r.err.Error())
		}
	case <-ctx.Done():
		return snap
	}

	return snap
}
