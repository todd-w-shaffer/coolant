package model

import (
	"strings"
	"testing"
	"time"

	"github.com/toddwshaffer/coolant/thermal/internal/collector"
	"github.com/toddwshaffer/coolant/thermal/internal/config"
)

func TestFirstUpdateSeedsState(t *testing.T) {
	s := NewAppState()
	now := time.Now()
	snap := testSnap(t, withTime(now))
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

	procs1 := []collector.ProcessInfo{
		{PID: 100, TypeCode: "N"},
		{PID: 101, TypeCode: "S"},
		{PID: 102, TypeCode: "V"},
	}
	s.Update(testSnap(t, withTime(now), withProcs(procs1)))

	if s.LastSpawns() != 0 {
		t.Errorf("LastSpawns after first tick = %d, want 0", s.LastSpawns())
	}

	procs2 := []collector.ProcessInfo{
		{PID: 100, TypeCode: "N"},
		{PID: 102, TypeCode: "V"},
		{PID: 103, TypeCode: "S"},
		{PID: 104, TypeCode: "G"},
	}
	s.Update(testSnap(t, withTime(now.Add(time.Second)), withProcs(procs2)))

	if s.LastSpawns() != 2 {
		t.Errorf("LastSpawns = %d, want 2 (PIDs 103, 104)", s.LastSpawns())
	}
	if s.LastDeaths() != 1 {
		t.Errorf("LastDeaths = %d, want 1 (PID 101)", s.LastDeaths())
	}
}

func TestTypeCountsPopulated(t *testing.T) {
	s := NewAppState()
	procs := []collector.ProcessInfo{
		{PID: 1, TypeCode: "V"},
		{PID: 2, TypeCode: "V"},
		{PID: 3, TypeCode: "N"},
		{PID: 4, TypeCode: "S"},
	}
	s.Update(testSnap(t, withProcs(procs)))

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
	procs := []collector.ProcessInfo{
		{PID: 1, TypeCode: "V"},  // node (vitest shows as node)
		{PID: 2, TypeCode: "T"},  // build
		{PID: 3, TypeCode: "B"},  // build
		{PID: 4, TypeCode: "N"},  // node
		{PID: 5, TypeCode: "G"},  // shell (grep)
		{PID: 6, TypeCode: "S"},  // shell
		{PID: 7, TypeCode: "GO"}, // go
		{PID: 8, TypeCode: "RS"}, // rust
	}
	s.Update(testSnap(t, withProcs(procs)))

	expected := map[string]int{
		"node":  2, // V + N
		"build": 2, // T + B
		"shell": 2, // G + S
		"go":    1, // GO
		"rust":  1, // RS
	}
	for cat, want := range expected {
		if got := s.CategoryCounts[cat]; got != want {
			t.Errorf("CategoryCounts[%s] = %d, want %d", cat, got, want)
		}
	}
}

func TestCategoryCountsIncludeSwift(t *testing.T) {
	s := NewAppState()
	procs := []collector.ProcessInfo{
		{PID: 1, TypeCode: "SW"}, // swift
		{PID: 2, TypeCode: "SW"}, // swift
		{PID: 3, TypeCode: "N"},  // node
	}
	s.Update(testSnap(t, withProcs(procs)))

	if got := s.CategoryCounts["swift"]; got != 2 {
		t.Errorf("CategoryCounts[swift] = %d, want 2", got)
	}
}

func TestTypeCountsClearedBetweenUpdates(t *testing.T) {
	s := NewAppState()
	now := time.Now()

	procs1 := []collector.ProcessInfo{{PID: 1, TypeCode: "V"}}
	s.Update(testSnap(t, withTime(now), withProcs(procs1)))
	if s.TypeCounts["V"] != 1 {
		t.Fatalf("TypeCounts[V] = %d, want 1", s.TypeCounts["V"])
	}

	procs2 := []collector.ProcessInfo{{PID: 2, TypeCode: "S"}}
	s.Update(testSnap(t, withTime(now.Add(time.Second)), withProcs(procs2)))

	if got := s.TypeCounts["V"]; got != 0 {
		t.Errorf("TypeCounts[V] after second tick = %d, want 0 (cleared)", got)
	}
}

func TestOnlineOfflineTransitions(t *testing.T) {
	s := NewAppState()
	now := time.Now()

	s.Update(testSnap(t, withTime(now), withOnline(true)))
	if !s.Online {
		t.Error("should be online after online snapshot")
	}

	s.Update(testSnap(t, withTime(now.Add(time.Second)), withOnline(false)))
	if s.Online {
		t.Error("should be offline after offline snapshot")
	}
	if s.OfflineSince.IsZero() {
		t.Error("OfflineSince should be set when going offline")
	}

	s.Update(testSnap(t, withTime(now.Add(5*time.Second)), withOnline(false)))
	if s.OfflineDuration < 4*time.Second {
		t.Errorf("OfflineDuration = %v, want >= 4s", s.OfflineDuration)
	}

	s.Update(testSnap(t, withTime(now.Add(10*time.Second)), withOnline(true)))
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

	s.Update(testSnap(t, withTime(now)))

	procs := make([]collector.ProcessInfo, config.SpawnBurstThreshold)
	for i := range procs {
		procs[i] = collector.ProcessInfo{PID: 200 + i, TypeCode: "N"}
	}
	s.Update(testSnap(t, withTime(now.Add(time.Second)), withProcs(procs)))

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
	snap := testSnap(t, withSessions([]collector.SessionTree{
		{RootPID: 1},
		{RootPID: 2},
		{RootPID: 3},
	}))
	s.Update(snap)

	if s.SessionCount != 3 {
		t.Errorf("SessionCount = %d, want 3", s.SessionCount)
	}
}

func TestIsIdle(t *testing.T) {
	s := NewAppState()
	now := time.Now()

	s.Update(testSnap(t, withTime(now)))
	if !s.IsIdle() {
		t.Error("IsIdle should be true with no sessions")
	}

	s.Update(testSnap(t, withTime(now.Add(time.Second)),
		withSessions([]collector.SessionTree{{RootPID: 1}})))
	if s.IsIdle() {
		t.Error("IsIdle should be false with sessions")
	}
}

func TestOnlineLogTracksPerTick(t *testing.T) {
	s := NewAppState()
	now := time.Now()

	s.Update(testSnap(t, withTime(now), withOnline(true)))
	s.Update(testSnap(t, withTime(now.Add(time.Second)), withOnline(false)))
	s.Update(testSnap(t, withTime(now.Add(2*time.Second)), withOnline(true)))

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

func TestAgentCountSplitsFreshAndStale(t *testing.T) {
	s := NewAppState()

	// Start two agents at different times
	old := collector.GateEvent{
		Timestamp: time.Now().Add(-10 * time.Minute),
		Event:     collector.EventAgentStart,
		AgentID:   "old-agent-0001",
		AgentType: "researcher",
	}
	fresh := collector.GateEvent{
		Timestamp: time.Now(),
		Event:     collector.EventAgentStart,
		AgentID:   "fresh-agent-0002",
		AgentType: "coder",
	}
	s.HandleEvent(old)
	s.HandleEvent(fresh)

	if s.AgentCount() != 2 {
		t.Fatalf("AgentCount = %d, want 2", s.AgentCount())
	}
	if s.FreshAgentCount() != 1 {
		t.Errorf("FreshAgentCount = %d, want 1", s.FreshAgentCount())
	}
	if s.StaleAgentCount() != 1 {
		t.Errorf("StaleAgentCount = %d, want 1", s.StaleAgentCount())
	}
}

func TestAgentStopClearsStaleAgent(t *testing.T) {
	s := NewAppState()

	// Start a stale agent
	s.HandleEvent(collector.GateEvent{
		Timestamp: time.Now().Add(-10 * time.Minute),
		Event:     collector.EventAgentStart,
		AgentID:   "stale-0001",
		AgentType: "researcher",
	})
	if s.StaleAgentCount() != 1 {
		t.Fatalf("StaleAgentCount = %d, want 1", s.StaleAgentCount())
	}

	// Stop it — should clear completely
	s.HandleEvent(collector.GateEvent{
		Timestamp: time.Now(),
		Event:     collector.EventAgentStop,
		AgentID:   "stale-0001",
		AgentType: "researcher",
	})
	if s.AgentCount() != 0 {
		t.Errorf("AgentCount = %d, want 0 after stop", s.AgentCount())
	}
	if s.StaleAgentCount() != 0 {
		t.Errorf("StaleAgentCount = %d, want 0 after stop", s.StaleAgentCount())
	}
}

func TestPurgeStaleAgents(t *testing.T) {
	s := NewAppState()

	// Two stale, one fresh
	s.HandleEvent(collector.GateEvent{
		Timestamp: time.Now().Add(-10 * time.Minute),
		Event:     collector.EventAgentStart,
		AgentID:   "stale-001",
		AgentType: "researcher",
	})
	s.HandleEvent(collector.GateEvent{
		Timestamp: time.Now().Add(-10 * time.Minute),
		Event:     collector.EventAgentStart,
		AgentID:   "stale-002",
		AgentType: "reviewer",
	})
	s.HandleEvent(collector.GateEvent{
		Timestamp: time.Now(),
		Event:     collector.EventAgentStart,
		AgentID:   "fresh-001",
		AgentType: "coder",
	})

	if s.StaleAgentCount() != 2 {
		t.Fatalf("StaleAgentCount = %d, want 2", s.StaleAgentCount())
	}

	s.PurgeStaleAgents()

	if s.AgentCount() != 1 {
		t.Errorf("AgentCount after purge = %d, want 1", s.AgentCount())
	}
	if s.StaleAgentCount() != 0 {
		t.Errorf("StaleAgentCount after purge = %d, want 0", s.StaleAgentCount())
	}
	if s.FreshAgentCount() != 1 {
		t.Errorf("FreshAgentCount after purge = %d, want 1", s.FreshAgentCount())
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
