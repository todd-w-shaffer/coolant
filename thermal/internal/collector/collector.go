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
	// 5s deadline protects against hanging syscalls or DNS — any collector
	// goroutine that outlives this context gets cancelled via exec.CommandContext.
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
	netCh := make(chan bool, 1)

	go func() {
		stats, err := CollectSystem(ctx)
		sysCh <- sysResult{stats, err}
	}()
	go func() {
		sessions, allProcs, err := CollectProcs(ctx)
		procCh <- procResult{sessions, allProcs, err}
	}()
	go func() {
		netCh <- CheckOnline(ctx)
	}()

	snap := Snapshot{
		Timestamp: now,
	}

	// Receive with timeout — if a collector hangs past the deadline,
	// return a partial snapshot rather than deadlocking.
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

	select {
	case online := <-netCh:
		snap.Online = online
	case <-ctx.Done():
		return snap
	}

	return snap
}
