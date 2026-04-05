package model

import (
	"strings"
	"testing"
	"time"

	"github.com/toddwshaffer/coolant/thermal/internal/collector"
	"github.com/toddwshaffer/coolant/thermal/internal/config"
)

// baseSnap returns a minimal valid snapshot at the given time.
func baseSnap(t *testing.T, ts time.Time) collector.Snapshot {
	t.Helper()
	return collector.Snapshot{
		System: collector.SystemStats{
			MemTotalBytes: 16 * int64(GB),
		},
		Online:    true,
		Timestamp: ts,
	}
}

// snapWithProcs returns a snapshot with the given processes.
func snapWithProcs(t *testing.T, ts time.Time, procs []collector.ProcessInfo) collector.Snapshot {
	t.Helper()
	snap := baseSnap(t, ts)
	snap.AllProcs = procs
	snap.Sessions = []collector.SessionTree{{RootPID: 1, Descendants: procs}}
	return snap
}

// overcommittedSnap returns a snapshot with heavy procs and high memory usage.
func overcommittedSnap(t *testing.T, ts time.Time) collector.Snapshot {
	t.Helper()
	procs := []collector.ProcessInfo{
		{PID: 1, TypeCode: "V"},
		{PID: 2, TypeCode: "V"},
		{PID: 3, TypeCode: "V"},
		{PID: 4, TypeCode: "N"},
		{PID: 5, TypeCode: "N"},
	}
	snap := snapWithProcs(t, ts, procs)
	snap.System.MemUsedBytes = 14 * int64(GB)
	snap.System.MemTotalBytes = 16 * int64(GB)
	return snap
}

func TestFirstUpdateSeedsState(t *testing.T) {
	s := NewAppState()
	now := time.Now()
	snap := baseSnap(t, now)
	s.Update(snap)

	if s.Current == nil {
		t.Fatal("Current is nil after first Update")
	}
	if s.History.Len() != 1 {
		t.Errorf("History.Len = %d, want 1", s.History.Len())
	}
	if s.ThreatLevel != ThreatCool {
		t.Errorf("ThreatLevel = %v, want ThreatCool for empty snapshot", s.ThreatLevel)
	}
	if !s.Online {
		t.Error("Online should be true for online snapshot")
	}
}

func TestSpawnDeathDeltasAcrossTwoUpdates(t *testing.T) {
	s := NewAppState()
	now := time.Now()

	// First tick: 3 processes
	procs1 := []collector.ProcessInfo{
		{PID: 100, TypeCode: "N"},
		{PID: 101, TypeCode: "S"},
		{PID: 102, TypeCode: "V"},
	}
	s.Update(snapWithProcs(t, now, procs1))

	// After first tick, no spawn/death yet (no previous PIDs to compare)
	if s.LastSpawns() != 0 {
		t.Errorf("LastSpawns after first tick = %d, want 0", s.LastSpawns())
	}

	// Second tick: PID 101 died, PID 103 and 104 spawned
	procs2 := []collector.ProcessInfo{
		{PID: 100, TypeCode: "N"},
		{PID: 102, TypeCode: "V"},
		{PID: 103, TypeCode: "S"},
		{PID: 104, TypeCode: "G"},
	}
	s.Update(snapWithProcs(t, now.Add(time.Second), procs2))

	if s.LastSpawns() != 2 {
		t.Errorf("LastSpawns = %d, want 2 (PIDs 103, 104)", s.LastSpawns())
	}
	if s.LastDeaths() != 1 {
		t.Errorf("LastDeaths = %d, want 1 (PID 101)", s.LastDeaths())
	}
}

func TestTypeCountsPopulated(t *testing.T) {
	s := NewAppState()
	now := time.Now()
	procs := []collector.ProcessInfo{
		{PID: 1, TypeCode: "V"},
		{PID: 2, TypeCode: "V"},
		{PID: 3, TypeCode: "N"},
		{PID: 4, TypeCode: "S"},
	}
	s.Update(snapWithProcs(t, now, procs))

	if s.TypeCounts["V"] != 2 {
		t.Errorf("TypeCounts[V] = %d, want 2", s.TypeCounts["V"])
	}
	if s.TypeCounts["N"] != 1 {
		t.Errorf("TypeCounts[N] = %d, want 1", s.TypeCounts["N"])
	}
	if s.TypeCounts["S"] != 1 {
		t.Errorf("TypeCounts[S] = %d, want 1", s.TypeCounts["S"])
	}
}

func TestCategoryCountsPopulated(t *testing.T) {
	s := NewAppState()
	now := time.Now()
	procs := []collector.ProcessInfo{
		{PID: 1, TypeCode: "V"}, // test
		{PID: 2, TypeCode: "T"}, // build
		{PID: 3, TypeCode: "B"}, // build
		{PID: 4, TypeCode: "N"}, // run
		{PID: 5, TypeCode: "G"}, // search
		{PID: 6, TypeCode: "S"}, // shell
	}
	s.Update(snapWithProcs(t, now, procs))

	expected := map[string]int{
		"test":   1,
		"build":  2,
		"run":    1,
		"search": 1,
		"shell":  1,
	}
	for cat, want := range expected {
		if got := s.CategoryCounts[cat]; got != want {
			t.Errorf("CategoryCounts[%s] = %d, want %d", cat, got, want)
		}
	}
}

func TestTypeCountsClearedBetweenUpdates(t *testing.T) {
	s := NewAppState()
	now := time.Now()

	// First tick has V procs
	procs1 := []collector.ProcessInfo{{PID: 1, TypeCode: "V"}}
	s.Update(snapWithProcs(t, now, procs1))
	if s.TypeCounts["V"] != 1 {
		t.Fatalf("TypeCounts[V] = %d, want 1", s.TypeCounts["V"])
	}

	// Second tick has no V procs, only S
	procs2 := []collector.ProcessInfo{{PID: 2, TypeCode: "S"}}
	s.Update(snapWithProcs(t, now.Add(time.Second), procs2))

	if got := s.TypeCounts["V"]; got != 0 {
		t.Errorf("TypeCounts[V] after second tick = %d, want 0 (cleared)", got)
	}
}

func TestOnlineOfflineTransitions(t *testing.T) {
	s := NewAppState()
	now := time.Now()

	// Start online
	snap1 := baseSnap(t, now)
	snap1.Online = true
	s.Update(snap1)
	if !s.Online {
		t.Error("should be online after online snapshot")
	}

	// Go offline
	snap2 := baseSnap(t, now.Add(time.Second))
	snap2.Online = false
	s.Update(snap2)
	if s.Online {
		t.Error("should be offline after offline snapshot")
	}
	if s.OfflineSince.IsZero() {
		t.Error("OfflineSince should be set when going offline")
	}

	// Stay offline — duration should increase
	snap3 := baseSnap(t, now.Add(5*time.Second))
	snap3.Online = false
	s.Update(snap3)
	if s.OfflineDuration < 4*time.Second {
		t.Errorf("OfflineDuration = %v, want >= 4s", s.OfflineDuration)
	}

	// Come back online
	snap4 := baseSnap(t, now.Add(10*time.Second))
	snap4.Online = true
	s.Update(snap4)
	if !s.Online {
		t.Error("should be online after reconnect")
	}
	if s.OfflineDuration != 0 {
		t.Errorf("OfflineDuration = %v, want 0 after reconnect", s.OfflineDuration)
	}
}

func TestAlertOnSpawnBurst(t *testing.T) {
	s := NewAppState()
	now := time.Now()

	// First tick: no procs
	s.Update(baseSnap(t, now))

	// Second tick: burst of procs exceeding threshold
	procs := make([]collector.ProcessInfo, config.SpawnBurstThreshold)
	for i := range procs {
		procs[i] = collector.ProcessInfo{PID: 200 + i, TypeCode: "N"}
	}
	s.Update(snapWithProcs(t, now.Add(time.Second), procs))

	found := false
	for i := 0; i < s.Alerts.Len(); i++ {
		if strings.Contains(s.Alerts.At(i).Message, "spawn burst") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected spawn burst alert, none found")
	}
}

func TestAlertOnHeadroomWarning(t *testing.T) {
	s := NewAppState()
	now := time.Now()

	s.Update(overcommittedSnap(t, now))

	found := false
	for i := 0; i < s.Alerts.Len(); i++ {
		msg := s.Alerts.At(i).Message
		if strings.Contains(msg, "over-committed") || strings.Contains(msg, "headroom") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected headroom warning alert, none found")
	}
}

func TestAlertDeduplication(t *testing.T) {
	s := NewAppState()
	now := time.Now()

	s.Update(overcommittedSnap(t, now))
	alertsAfterFirst := s.Alerts.Len()

	s.Update(overcommittedSnap(t, now.Add(time.Second)))
	alertsAfterSecond := s.Alerts.Len()

	// Count headroom-related alerts
	headroomCount := 0
	for i := 0; i < s.Alerts.Len(); i++ {
		msg := s.Alerts.At(i).Message
		if strings.Contains(msg, "over-committed") || strings.Contains(msg, "headroom") {
			headroomCount++
		}
	}

	if headroomCount > 1 {
		t.Errorf("headroom alerts = %d, want 1 (should be deduplicated). alerts after first=%d, after second=%d",
			headroomCount, alertsAfterFirst, alertsAfterSecond)
	}
}

func TestSessionCountTracked(t *testing.T) {
	s := NewAppState()
	now := time.Now()

	snap := baseSnap(t, now)
	snap.Sessions = []collector.SessionTree{
		{RootPID: 1},
		{RootPID: 2},
		{RootPID: 3},
	}
	s.Update(snap)

	if s.SessionCount != 3 {
		t.Errorf("SessionCount = %d, want 3", s.SessionCount)
	}
}

func TestIsIdle(t *testing.T) {
	s := NewAppState()
	now := time.Now()

	// No sessions → idle
	s.Update(baseSnap(t, now))
	if !s.IsIdle() {
		t.Error("IsIdle should be true with no sessions")
	}

	// With sessions → not idle
	snap := baseSnap(t, now.Add(time.Second))
	snap.Sessions = []collector.SessionTree{{RootPID: 1}}
	s.Update(snap)
	if s.IsIdle() {
		t.Error("IsIdle should be false with sessions")
	}
}

func TestOnlineLogTracksPerTick(t *testing.T) {
	s := NewAppState()
	now := time.Now()

	snap1 := baseSnap(t, now)
	snap1.Online = true
	s.Update(snap1)

	snap2 := baseSnap(t, now.Add(time.Second))
	snap2.Online = false
	s.Update(snap2)

	snap3 := baseSnap(t, now.Add(2*time.Second))
	snap3.Online = true
	s.Update(snap3)

	if s.OnlineLog.Len() != 3 {
		t.Fatalf("OnlineLog.Len = %d, want 3", s.OnlineLog.Len())
	}
	got := s.OnlineLog.Slice()
	want := []bool{true, false, true}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("OnlineLog[%d] = %v, want %v", i, got[i], w)
		}
	}
}

func TestHandleEventAgentStart(t *testing.T) {
	s := NewAppState()
	ev := collector.GateEvent{
		Timestamp: time.Now(),
		Event:     collector.EventAgentStart,
		AgentType: "subagent",
		AgentID:   "abcdef1234567890",
	}
	s.HandleEvent(ev)

	if !s.PluginActive {
		t.Error("PluginActive should be true after agent.start event")
	}
	if s.Alerts.Len() != 1 {
		t.Fatalf("Alerts.Len = %d, want 1", s.Alerts.Len())
	}
	msg := s.Alerts.At(0).Message
	if !strings.Contains(msg, "subagent") {
		t.Errorf("alert message = %q, want to contain agent type", msg)
	}
	if !strings.Contains(msg, "abcdef12") {
		t.Errorf("alert message = %q, want truncated agent ID", msg)
	}
}

func TestHandleEventGateSuppress(t *testing.T) {
	s := NewAppState()
	ev := collector.GateEvent{
		Timestamp: time.Now(),
		Event:     collector.EventGateSuppress,
		Command:   "tsc",
		Reason:    "parallel mode",
	}
	s.HandleEvent(ev)

	if s.Alerts.Len() != 1 {
		t.Fatalf("Alerts.Len = %d, want 1", s.Alerts.Len())
	}
	msg := s.Alerts.At(0).Message
	if !strings.Contains(msg, "tsc") || !strings.Contains(msg, "suppressed") {
		t.Errorf("alert message = %q, want gate suppress details", msg)
	}
}

func TestShortID(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"abcdef1234567890", "abcdef12"},
		{"short", "short"},
		{"12345678", "12345678"},
		{"", ""},
	}
	for _, tc := range cases {
		if got := shortID(tc.input); got != tc.want {
			t.Errorf("shortID(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
