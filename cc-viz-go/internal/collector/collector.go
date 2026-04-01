package collector

import (
	"context"
	"time"
)

// Run starts the collector loop, sending Snapshots to ch at the given interval.
// It collects system stats and process trees concurrently.
func Run(ch chan<- Snapshot, interval time.Duration, done <-chan struct{}) {
	defer close(ch)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			snap := collect()
			select {
			case ch <- snap:
			case <-done:
				return
			}
		}
	}
}

func collect() Snapshot {
	ctx := context.Background()
	now := time.Now()

	// Collect system stats and process tree concurrently
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

	sysRes := <-sysCh
	procRes := <-procCh

	snap := Snapshot{
		Timestamp: now,
	}

	if sysRes.err == nil {
		snap.System = sysRes.stats
	}
	if procRes.err == nil {
		snap.Sessions = procRes.sessions
		snap.AllProcs = procRes.allProcs
	}

	return snap
}
