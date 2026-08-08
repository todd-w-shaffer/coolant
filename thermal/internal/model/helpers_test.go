package model

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/toddwshaffer/coolant/thermal/internal/collector"
)

const testMemTotal int64 = 16 * GB

// testSnap builds a snapshot with full control over system stats, timestamp,
// and online status. All model test files share this single builder.
func testSnap(t *testing.T, opts ...snapOpt) collector.Snapshot {
	t.Helper()
	snap := collector.Snapshot{
		System: collector.SystemStats{
			MemTotalBytes: testMemTotal,
		},
		Online:    true,
		Timestamp: time.Now(),
	}
	for _, o := range opts {
		o(&snap)
	}
	return snap
}

type snapOpt func(*collector.Snapshot)

func withCPU(pct float64) snapOpt {
	return func(s *collector.Snapshot) { s.System.CPUPercent = pct }
}

func withMem(used, total int64) snapOpt {
	return func(s *collector.Snapshot) {
		s.System.MemUsedBytes = used
		s.System.MemTotalBytes = total
	}
}

func withSwap(used int64) snapOpt {
	return func(s *collector.Snapshot) { s.System.SwapUsedBytes = used }
}

func withTime(ts time.Time) snapOpt {
	return func(s *collector.Snapshot) { s.Timestamp = ts }
}

func withOnline(online bool) snapOpt {
	return func(s *collector.Snapshot) { s.Online = online }
}

// testProcSeq hands each withProcs call a distinct, monotonically increasing
// ProcSeq — a withProcs snapshot models a *new* process sample, which the
// collector now tags so the model can tell a fresh sample from the ~6 stale
// re-deliveries that ride each 150ms fast snapshot between 1s proc scans.
var testProcSeq atomic.Uint64

func withProcs(procs []collector.ProcessInfo) snapOpt {
	return func(s *collector.Snapshot) {
		s.AllProcs = procs
		s.Sessions = []collector.SessionTree{{RootPID: 1, Descendants: procs}}
		s.ProcSeq = testProcSeq.Add(1)
	}
}

// withProcSeq overrides the ProcSeq (apply after withProcs) to simulate a stale
// re-delivery of the same proc sample on a later fast tick.
func withProcSeq(seq uint64) snapOpt {
	return func(s *collector.Snapshot) { s.ProcSeq = seq }
}

func withSessions(sessions []collector.SessionTree) snapOpt {
	return func(s *collector.Snapshot) { s.Sessions = sessions }
}

// overcommittedSnap returns a snapshot with 3xV + 2xN heavy procs and 14GB used.
func overcommittedSnap(t *testing.T, ts time.Time) collector.Snapshot {
	t.Helper()
	return testSnap(t, withTime(ts),
		withProcs([]collector.ProcessInfo{
			{PID: 1, TypeCode: "V"},
			{PID: 2, TypeCode: "V"},
			{PID: 3, TypeCode: "V"},
			{PID: 4, TypeCode: "N"},
			{PID: 5, TypeCode: "N"},
		}),
		withMem(14*int64(GB), testMemTotal),
	)
}

// pctToBytes converts a memory percentage to bytes relative to testMemTotal.
// Exists as a function (not inline arithmetic) to prevent Go constant folding
// from rejecting float64→int64 conversions at compile time.
func pctToBytes(pct float64) int64 {
	return int64(pct / 100.0 * float64(testMemTotal))
}
