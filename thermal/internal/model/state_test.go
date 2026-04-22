package model

import (
	"os"
	"path/filepath"
	"strconv"
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

	procs := make([]collector.ProcessInfo, config.C.Spawn.BurstThreshold)
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

func TestAgentStartCreatesRecord(t *testing.T) {
	s := NewAppState()
	ev := collector.GateEvent{
		Timestamp: time.Now(),
		Event:     collector.EventAgentStart,
		AgentID:   "abc123",
		SessionID: "sess-001",
		AgentType: "researcher",
		Project:   "/home/user/project",
		Cwd:       "/home/user/project/src",
	}
	s.HandleEvent(ev)

	if s.AgentCount() != 1 {
		t.Fatalf("AgentCount = %d, want 1", s.AgentCount())
	}

	records := s.ActiveRecords()
	if len(records) != 1 {
		t.Fatalf("ActiveRecords len = %d, want 1", len(records))
	}
	rec := records[0]
	if rec.AgentID != "abc123" {
		t.Errorf("AgentID = %q, want %q", rec.AgentID, "abc123")
	}
	if rec.SessionID != "sess-001" {
		t.Errorf("SessionID = %q, want %q", rec.SessionID, "sess-001")
	}
	if rec.AgentType != "researcher" {
		t.Errorf("AgentType = %q, want %q", rec.AgentType, "researcher")
	}
	if rec.Project != "/home/user/project" {
		t.Errorf("Project = %q, want %q", rec.Project, "/home/user/project")
	}
	if rec.Cwd != "/home/user/project/src" {
		t.Errorf("Cwd = %q, want %q", rec.Cwd, "/home/user/project/src")
	}
	if rec.StartTime.IsZero() {
		t.Error("StartTime should not be zero")
	}
}

func TestAgentStopCompletesRecord(t *testing.T) {
	s := NewAppState()
	startTime := time.Now()
	stopTime := startTime.Add(30 * time.Second)

	s.HandleEvent(collector.GateEvent{
		Timestamp: startTime, Event: collector.EventAgentStart,
		AgentID: "rec-001", SessionID: "sess-1", AgentType: "coder",
		Project: "/proj", Cwd: "/proj/src",
	})
	s.HandleEvent(collector.GateEvent{
		Timestamp: stopTime, Event: collector.EventAgentStop,
		AgentID: "rec-001", AgentType: "coder",
		PermissionMode: "auto", TranscriptPath: "/tmp/transcript.jsonl",
	})

	if len(s.ActiveRecords()) != 0 {
		t.Errorf("ActiveRecords len = %d, want 0 after stop", len(s.ActiveRecords()))
	}

	completed := s.CompletedRecords()
	if len(completed) != 1 {
		t.Fatalf("CompletedRecords len = %d, want 1", len(completed))
	}
	rec := completed[0]
	if rec.Duration != 30*time.Second {
		t.Errorf("Duration = %v, want 30s", rec.Duration)
	}
	if rec.StopTime != stopTime {
		t.Errorf("StopTime = %v, want %v", rec.StopTime, stopTime)
	}
	if rec.PermissionMode != "auto" {
		t.Errorf("PermissionMode = %q, want %q", rec.PermissionMode, "auto")
	}
	if rec.TranscriptPath != "/tmp/transcript.jsonl" {
		t.Errorf("TranscriptPath = %q, want %q", rec.TranscriptPath, "/tmp/transcript.jsonl")
	}
	if s.CompletedAgentCount() != 1 {
		t.Errorf("CompletedAgentCount = %d, want 1", s.CompletedAgentCount())
	}
}

func TestPurgeStaleAgentsPushesRecords(t *testing.T) {
	s := NewAppState()

	// Two stale, one fresh
	s.HandleEvent(collector.GateEvent{
		Timestamp: time.Now().Add(-10 * time.Minute),
		Event:     collector.EventAgentStart,
		AgentID:   "stale-001", AgentType: "researcher",
	})
	s.HandleEvent(collector.GateEvent{
		Timestamp: time.Now().Add(-10 * time.Minute),
		Event:     collector.EventAgentStart,
		AgentID:   "stale-002", AgentType: "reviewer",
	})
	s.HandleEvent(collector.GateEvent{
		Timestamp: time.Now(),
		Event:     collector.EventAgentStart,
		AgentID:   "fresh-001", AgentType: "coder",
	})

	s.PurgeStaleAgents()

	// Purged records should appear in completed with Purged: true
	completed := s.CompletedRecords()
	if len(completed) != 2 {
		t.Fatalf("CompletedRecords len = %d, want 2 after purge", len(completed))
	}
	for _, rec := range completed {
		if !rec.Purged {
			t.Errorf("record %q should have Purged=true", rec.AgentID)
		}
		if rec.StopTime.IsZero() {
			t.Errorf("record %q should have StopTime set to purge cutoff", rec.AgentID)
		}
	}

	// Purge should NOT increment totalCompleted (avoids phantom KITT dots)
	if s.CompletedAgentCount() != 0 {
		t.Errorf("CompletedAgentCount = %d, want 0 (purge doesn't count)", s.CompletedAgentCount())
	}

	// Fresh agent still active
	if s.AgentCount() != 1 {
		t.Errorf("AgentCount = %d, want 1", s.AgentCount())
	}
}

func TestRingBufferEvictsOldest(t *testing.T) {
	s := NewAppState()
	// Push MaxAgentRecords + 1 completed records, verify oldest is evicted
	for i := 0; i < config.MaxAgentRecords+1; i++ {
		id := "agent-" + strconv.Itoa(i)
		start := time.Now()
		s.HandleEvent(collector.GateEvent{
			Timestamp: start, Event: collector.EventAgentStart,
			AgentID: id, AgentType: "worker",
		})
		s.HandleEvent(collector.GateEvent{
			Timestamp: start.Add(time.Second), Event: collector.EventAgentStop,
			AgentID: id, AgentType: "worker",
		})
	}

	completed := s.CompletedRecords()
	if len(completed) != config.MaxAgentRecords {
		t.Fatalf("CompletedRecords len = %d, want %d", len(completed), config.MaxAgentRecords)
	}
	// Oldest should be agent-1 (agent-0 was evicted)
	if completed[0].AgentID != "agent-1" {
		t.Errorf("oldest record = %q, want %q (agent-0 should be evicted)", completed[0].AgentID, "agent-1")
	}
	// Counter should still be monotonic
	if s.CompletedAgentCount() != config.MaxAgentRecords+1 {
		t.Errorf("CompletedAgentCount = %d, want %d", s.CompletedAgentCount(), config.MaxAgentRecords+1)
	}
}

func TestLookupAgent(t *testing.T) {
	s := NewAppState()
	start := time.Now()

	// Start two agents
	s.HandleEvent(collector.GateEvent{
		Timestamp: start, Event: collector.EventAgentStart,
		AgentID: "active-1", AgentType: "coder",
	})
	s.HandleEvent(collector.GateEvent{
		Timestamp: start, Event: collector.EventAgentStart,
		AgentID: "done-1", AgentType: "reviewer",
	})
	// Stop one
	s.HandleEvent(collector.GateEvent{
		Timestamp: start.Add(10 * time.Second), Event: collector.EventAgentStop,
		AgentID: "done-1", AgentType: "reviewer",
	})

	// Look up the active one — should have zero StopTime
	rec := s.LookupAgent("active-1")
	if rec == nil {
		t.Fatal("LookupAgent(active-1) returned nil")
	}
	if !rec.StopTime.IsZero() {
		t.Errorf("active agent StopTime = %v, want zero", rec.StopTime)
	}

	// Look up the completed one — should have non-zero Duration
	rec = s.LookupAgent("done-1")
	if rec == nil {
		t.Fatal("LookupAgent(done-1) returned nil")
	}
	if rec.Duration != 10*time.Second {
		t.Errorf("completed agent Duration = %v, want 10s", rec.Duration)
	}

	// Look up nonexistent — should be nil
	if s.LookupAgent("nope") != nil {
		t.Error("LookupAgent(nope) should return nil")
	}
}

func TestCounterResetClearsActiveRecords(t *testing.T) {
	s := NewAppState()
	start := time.Now()

	// Start 2 agents
	s.HandleEvent(collector.GateEvent{
		Timestamp: start, Event: collector.EventAgentStart,
		AgentID: "a1", AgentType: "coder",
	})
	s.HandleEvent(collector.GateEvent{
		Timestamp: start, Event: collector.EventAgentStart,
		AgentID: "a2", AgentType: "reviewer",
	})

	if s.AgentCount() != 2 {
		t.Fatalf("AgentCount = %d, want 2 before reset", s.AgentCount())
	}

	// counter.reset
	resetTime := start.Add(time.Minute)
	s.HandleEvent(collector.GateEvent{
		Timestamp: resetTime, Event: collector.EventCounterReset,
	})

	if s.AgentCount() != 0 {
		t.Errorf("AgentCount = %d, want 0 after reset", s.AgentCount())
	}

	completed := s.CompletedRecords()
	if len(completed) != 2 {
		t.Fatalf("CompletedRecords len = %d, want 2 after reset", len(completed))
	}
	for _, rec := range completed {
		if !rec.Purged {
			t.Errorf("record %q should have Purged=true after reset", rec.AgentID)
		}
		if rec.StopTime != resetTime {
			t.Errorf("record %q StopTime = %v, want reset timestamp", rec.AgentID, rec.StopTime)
		}
	}

	// Purge from reset should NOT increment totalCompleted
	if s.CompletedAgentCount() != 0 {
		t.Errorf("CompletedAgentCount = %d, want 0 (reset doesn't count)", s.CompletedAgentCount())
	}
}

func TestOrphanStopCreatesRecord(t *testing.T) {
	s := NewAppState()

	// Stop with no prior start — orphan
	s.HandleEvent(collector.GateEvent{
		Timestamp: time.Now(), Event: collector.EventAgentStop,
		AgentID: "orphan-1", AgentType: "ghost",
		SessionID: "sess-x", Project: "/proj",
		PermissionMode: "auto", TranscriptPath: "/tmp/t.jsonl",
	})

	completed := s.CompletedRecords()
	if len(completed) != 1 {
		t.Fatalf("CompletedRecords len = %d, want 1", len(completed))
	}
	rec := completed[0]
	if !rec.Orphan {
		t.Error("record should have Orphan=true")
	}
	if rec.Duration != 0 {
		t.Errorf("orphan Duration = %v, want 0 (no start to compute from)", rec.Duration)
	}
	if rec.PermissionMode != "auto" {
		t.Errorf("PermissionMode = %q, want %q", rec.PermissionMode, "auto")
	}
	if s.OrphanStopCount() != 1 {
		t.Errorf("OrphanStopCount = %d, want 1", s.OrphanStopCount())
	}
	if s.CompletedAgentCount() != 1 {
		t.Errorf("CompletedAgentCount = %d, want 1 (orphans count)", s.CompletedAgentCount())
	}

	// Normal start+stop should not affect orphan count
	s.HandleEvent(collector.GateEvent{
		Timestamp: time.Now(), Event: collector.EventAgentStart,
		AgentID: "normal-1", AgentType: "coder",
	})
	s.HandleEvent(collector.GateEvent{
		Timestamp: time.Now(), Event: collector.EventAgentStop,
		AgentID: "normal-1", AgentType: "coder",
	})
	if s.OrphanStopCount() != 1 {
		t.Errorf("OrphanStopCount = %d after normal stop, want 1", s.OrphanStopCount())
	}
}

func TestPeakConcurrency(t *testing.T) {
	s := NewAppState()
	start := time.Now()

	// Start 3 agents
	for i := 0; i < 3; i++ {
		s.HandleEvent(collector.GateEvent{
			Timestamp: start.Add(time.Duration(i) * time.Second),
			Event:     collector.EventAgentStart,
			AgentID:   "peak-" + strconv.Itoa(i), AgentType: "worker",
		})
	}
	if s.PeakConcurrency() != 3 {
		t.Errorf("PeakConcurrency = %d, want 3", s.PeakConcurrency())
	}

	// Stop 2, start 1 — peak should still be 3 (high-water mark)
	s.HandleEvent(collector.GateEvent{
		Timestamp: start.Add(5 * time.Second), Event: collector.EventAgentStop,
		AgentID: "peak-0", AgentType: "worker",
	})
	s.HandleEvent(collector.GateEvent{
		Timestamp: start.Add(6 * time.Second), Event: collector.EventAgentStop,
		AgentID: "peak-1", AgentType: "worker",
	})
	s.HandleEvent(collector.GateEvent{
		Timestamp: start.Add(7 * time.Second), Event: collector.EventAgentStart,
		AgentID: "peak-3", AgentType: "worker",
	})
	if s.PeakConcurrency() != 3 {
		t.Errorf("PeakConcurrency after partial stop = %d, want 3", s.PeakConcurrency())
	}
}

func TestBurstDetection(t *testing.T) {
	s := NewAppState()
	start := time.Now()

	// Burst 0: 3 agents within 3 seconds
	for i := 0; i < 3; i++ {
		s.HandleEvent(collector.GateEvent{
			Timestamp: start.Add(time.Duration(i) * time.Second),
			Event:     collector.EventAgentStart,
			AgentID:   "b0-" + strconv.Itoa(i), AgentType: "worker",
		})
	}

	// Gap > BurstGapThreshold, then burst 1: 2 agents
	for i := 0; i < 2; i++ {
		s.HandleEvent(collector.GateEvent{
			Timestamp: start.Add(6*time.Second + time.Duration(i)*time.Second),
			Event:     collector.EventAgentStart,
			AgentID:   "b1-" + strconv.Itoa(i), AgentType: "worker",
		})
	}

	// Stop all and check BurstIDs via completed records
	for i := 0; i < 3; i++ {
		s.HandleEvent(collector.GateEvent{
			Timestamp: start.Add(10 * time.Second), Event: collector.EventAgentStop,
			AgentID: "b0-" + strconv.Itoa(i), AgentType: "worker",
		})
	}
	for i := 0; i < 2; i++ {
		s.HandleEvent(collector.GateEvent{
			Timestamp: start.Add(10 * time.Second), Event: collector.EventAgentStop,
			AgentID: "b1-" + strconv.Itoa(i), AgentType: "worker",
		})
	}

	completed := s.CompletedRecords()
	for _, rec := range completed {
		switch {
		case rec.AgentID[:2] == "b0":
			if rec.BurstID != 0 {
				t.Errorf("agent %q BurstID = %d, want 0", rec.AgentID, rec.BurstID)
			}
		case rec.AgentID[:2] == "b1":
			if rec.BurstID != 1 {
				t.Errorf("agent %q BurstID = %d, want 1", rec.AgentID, rec.BurstID)
			}
		}
	}
}

func TestTranscriptStat(t *testing.T) {
	s := NewAppState()
	start := time.Now()

	// Create a temp file with known content
	dir := t.TempDir()
	path := dir + "/transcript.jsonl"
	data := make([]byte, 1234)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Start + stop with transcript path pointing to the temp file
	s.HandleEvent(collector.GateEvent{
		Timestamp: start, Event: collector.EventAgentStart,
		AgentID: "t1", AgentType: "coder",
	})
	s.HandleEvent(collector.GateEvent{
		Timestamp: start.Add(time.Second), Event: collector.EventAgentStop,
		AgentID: "t1", AgentType: "coder", TranscriptPath: path,
	})

	completed := s.CompletedRecords()
	if len(completed) != 1 {
		t.Fatalf("CompletedRecords len = %d, want 1", len(completed))
	}
	if completed[0].TranscriptBytes != 1234 {
		t.Errorf("TranscriptBytes = %d, want 1234", completed[0].TranscriptBytes)
	}

	// Stop with nonexistent path — TranscriptBytes should be 0
	s.HandleEvent(collector.GateEvent{
		Timestamp: start, Event: collector.EventAgentStart,
		AgentID: "t2", AgentType: "reviewer",
	})
	s.HandleEvent(collector.GateEvent{
		Timestamp: start.Add(time.Second), Event: collector.EventAgentStop,
		AgentID: "t2", AgentType: "reviewer", TranscriptPath: "/nonexistent/path.jsonl",
	})

	completed = s.CompletedRecords()
	if len(completed) != 2 {
		t.Fatalf("CompletedRecords len = %d, want 2", len(completed))
	}
	// Newest is last
	if completed[1].TranscriptBytes != 0 {
		t.Errorf("TranscriptBytes for missing file = %d, want 0", completed[1].TranscriptBytes)
	}
}

func TestOrphanTranscriptStat(t *testing.T) {
	s := NewAppState()

	dir := t.TempDir()
	path := dir + "/orphan-transcript.jsonl"
	data := make([]byte, 567)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Orphan stop with transcript path
	s.HandleEvent(collector.GateEvent{
		Timestamp: time.Now(), Event: collector.EventAgentStop,
		AgentID: "orphan-t", AgentType: "ghost", TranscriptPath: path,
	})

	completed := s.CompletedRecords()
	if len(completed) != 1 {
		t.Fatalf("CompletedRecords len = %d, want 1", len(completed))
	}
	if completed[0].TranscriptBytes != 567 {
		t.Errorf("orphan TranscriptBytes = %d, want 567", completed[0].TranscriptBytes)
	}
}

func TestGateEventBuffer(t *testing.T) {
	s := NewAppState()
	now := time.Now()

	// Push 3 cap events and 1 suppress
	for i := 0; i < 3; i++ {
		s.HandleEvent(collector.GateEvent{
			Timestamp: now.Add(time.Duration(i) * time.Second),
			Event:     collector.EventGateCap,
			Command:   "vitest", Rewritten: "vitest --maxWorkers=2",
		})
	}
	s.HandleEvent(collector.GateEvent{
		Timestamp: now.Add(4 * time.Second),
		Event:     collector.EventGateSuppress,
		Command:   "tsc", Reason: "parallel mode",
	})

	events := s.GateEvents()
	if len(events) != 4 {
		t.Fatalf("GateEvents len = %d, want 4", len(events))
	}
	if s.GateCapCount() != 3 {
		t.Errorf("GateCapCount = %d, want 3", s.GateCapCount())
	}
	if s.GateSuppressCount() != 1 {
		t.Errorf("GateSuppressCount = %d, want 1", s.GateSuppressCount())
	}
}

func TestCounterDrift(t *testing.T) {
	s := NewAppState()
	now := time.Now()

	// Start an agent with AgentCount=1 — Go also has 1 active record → drift 0
	s.HandleEvent(collector.GateEvent{
		Timestamp: now, Event: collector.EventAgentStart,
		AgentID: "d1", AgentType: "coder", AgentCount: 1,
	})
	if s.CounterDrift() != 0 {
		t.Errorf("drift after start = %d, want 0", s.CounterDrift())
	}

	// Orphan stop with AgentCount=2 — Go removes nothing (no match), bash says 2
	// Go has 1 active → drift = 2 - 1 = 1
	s.HandleEvent(collector.GateEvent{
		Timestamp: now.Add(time.Second), Event: collector.EventAgentStop,
		AgentID: "unknown", AgentType: "ghost", AgentCount: 2,
	})
	if s.CounterDrift() != 1 {
		t.Errorf("drift after orphan stop = %d, want 1", s.CounterDrift())
	}

	// Start another agent with AgentCount=2 — Go now has 2 active → drift 0
	s.HandleEvent(collector.GateEvent{
		Timestamp: now.Add(2 * time.Second), Event: collector.EventAgentStart,
		AgentID: "d2", AgentType: "reviewer", AgentCount: 2,
	})
	if s.CounterDrift() != 0 {
		t.Errorf("drift after second start = %d, want 0", s.CounterDrift())
	}
}

func TestGateEventBufferEviction(t *testing.T) {
	s := NewAppState()
	now := time.Now()

	// Push MaxGateEvents + 1 cap events — oldest should be evicted
	for i := 0; i < config.MaxGateEvents+1; i++ {
		s.HandleEvent(collector.GateEvent{
			Timestamp: now.Add(time.Duration(i) * time.Second),
			Event:     collector.EventGateCap,
			Command:   "vitest-" + strconv.Itoa(i),
		})
	}

	events := s.GateEvents()
	if len(events) != config.MaxGateEvents {
		t.Fatalf("GateEvents len = %d, want %d", len(events), config.MaxGateEvents)
	}
	// Oldest should be vitest-1 (vitest-0 evicted)
	if events[0].Command != "vitest-1" {
		t.Errorf("oldest gate event = %q, want %q", events[0].Command, "vitest-1")
	}
	if s.GateCapCount() != config.MaxGateEvents {
		t.Errorf("GateCapCount = %d, want %d", s.GateCapCount(), config.MaxGateEvents)
	}
}

func TestBurstGapBoundary(t *testing.T) {
	s := NewAppState()
	start := time.Now()

	// First agent
	s.HandleEvent(collector.GateEvent{
		Timestamp: start, Event: collector.EventAgentStart,
		AgentID: "exact-0", AgentType: "worker",
	})
	// Second agent at exactly BurstGapThreshold — should stay in same burst (> not >=)
	s.HandleEvent(collector.GateEvent{
		Timestamp: start.Add(config.BurstGapThreshold), Event: collector.EventAgentStart,
		AgentID: "exact-1", AgentType: "worker",
	})
	// Third agent 1ns past threshold — new burst
	s.HandleEvent(collector.GateEvent{
		Timestamp: start.Add(config.BurstGapThreshold + config.BurstGapThreshold + 1),
		Event:     collector.EventAgentStart,
		AgentID:   "over-0", AgentType: "worker",
	})

	// Stop all to check BurstIDs
	for _, id := range []string{"exact-0", "exact-1", "over-0"} {
		s.HandleEvent(collector.GateEvent{
			Timestamp: start.Add(10 * time.Second), Event: collector.EventAgentStop,
			AgentID: id, AgentType: "worker",
		})
	}

	completed := s.CompletedRecords()
	for _, rec := range completed {
		switch rec.AgentID {
		case "exact-0", "exact-1":
			if rec.BurstID != 0 {
				t.Errorf("agent %q BurstID = %d, want 0 (within threshold)", rec.AgentID, rec.BurstID)
			}
		case "over-0":
			if rec.BurstID != 1 {
				t.Errorf("agent %q BurstID = %d, want 1 (past threshold)", rec.AgentID, rec.BurstID)
			}
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
	if !strings.Contains(msg, "tsc") {
		t.Errorf("alert message = %q, want command name present", msg)
	}
}

func TestHandleEventCounterReset(t *testing.T) {
	s := NewAppState()
	ev := collector.GateEvent{
		Timestamp: time.Now(),
		Event:     collector.EventCounterReset,
	}
	s.HandleEvent(ev)

	if s.Alerts.Len() != 1 {
		t.Fatalf("Alerts.Len = %d, want 1", s.Alerts.Len())
	}
	alert := s.Alerts.At(0)
	if alert.Message != "session reset" {
		t.Errorf("alert message = %q, want %q", alert.Message, "session reset")
	}
	if alert.Level != ThreatCool {
		t.Errorf("alert level = %v, want ThreatCool", alert.Level)
	}
}

func TestHandleEventPreflightWarn(t *testing.T) {
	s := NewAppState()
	ev := collector.GateEvent{
		Timestamp: time.Now(),
		Event:     collector.EventPreflightWarn,
		Reason:    "missing worktree exclusion",
	}
	s.HandleEvent(ev)

	if s.Alerts.Len() != 1 {
		t.Fatalf("Alerts.Len = %d, want 1", s.Alerts.Len())
	}
	alert := s.Alerts.At(0)
	wantMsg := "preflight: missing worktree exclusion"
	if alert.Message != wantMsg {
		t.Errorf("alert message = %q, want %q", alert.Message, wantMsg)
	}
	if alert.Level != ThreatWarm {
		t.Errorf("alert level = %v, want ThreatWarm", alert.Level)
	}
}

func TestHandleEventGateCap(t *testing.T) {
	s := NewAppState()
	ev := collector.GateEvent{
		Timestamp: time.Now(),
		Event:     collector.EventGateCap,
		Command:   "vitest --pool=threads",
		Rewritten: "vitest --pool=threads --maxWorkers=2",
	}
	s.HandleEvent(ev)

	if s.Alerts.Len() != 1 {
		t.Fatalf("Alerts.Len = %d, want 1", s.Alerts.Len())
	}
	alert := s.Alerts.At(0)
	if !strings.Contains(alert.Message, "vitest") {
		t.Errorf("alert message = %q, want command name present", alert.Message)
	}
	if alert.Level != ThreatWarm {
		t.Errorf("alert level = %v, want ThreatWarm", alert.Level)
	}
}

func TestHandleEventAgentStopAlert(t *testing.T) {
	s := NewAppState()
	ev := collector.GateEvent{
		Timestamp: time.Now(),
		Event:     collector.EventAgentStop,
		AgentID:   "agent-9876543210",
		AgentType: "researcher",
	}
	s.HandleEvent(ev)

	if s.Alerts.Len() != 1 {
		t.Fatalf("Alerts.Len = %d, want 1", s.Alerts.Len())
	}
	alert := s.Alerts.At(0)
	if !strings.Contains(alert.Message, "stopped") {
		t.Errorf("alert message = %q, want 'stopped' substring", alert.Message)
	}
	if !strings.Contains(alert.Message, "agent-98") {
		t.Errorf("alert message = %q, want truncated agent ID", alert.Message)
	}
	if alert.Level != ThreatCool {
		t.Errorf("alert level = %v, want ThreatCool", alert.Level)
	}
}

func TestHandleEventParallelEngaged(t *testing.T) {
	s := NewAppState()
	ev := collector.GateEvent{
		Timestamp: time.Now(),
		Event:     collector.EventParallelEngaged,
	}
	s.HandleEvent(ev)

	if s.Alerts.Len() != 1 {
		t.Fatalf("Alerts.Len = %d, want 1", s.Alerts.Len())
	}
	alert := s.Alerts.At(0)
	if alert.Message != "parallel mode engaged" {
		t.Errorf("alert message = %q, want %q", alert.Message, "parallel mode engaged")
	}
	if alert.Level != ThreatHot {
		t.Errorf("alert level = %v, want ThreatHot", alert.Level)
	}
}

func TestCompletedAgentsIncrementsOnStop(t *testing.T) {
	s := NewAppState()

	// Start two agents
	s.HandleEvent(collector.GateEvent{
		Timestamp: time.Now(), Event: collector.EventAgentStart,
		AgentID: "a1", AgentType: "researcher",
	})
	s.HandleEvent(collector.GateEvent{
		Timestamp: time.Now(), Event: collector.EventAgentStart,
		AgentID: "a2", AgentType: "coder",
	})

	if s.CompletedAgentCount() != 0 {
		t.Fatalf("CompletedAgents = %d before any stops, want 0", s.CompletedAgentCount())
	}

	// Stop one
	s.HandleEvent(collector.GateEvent{
		Timestamp: time.Now(), Event: collector.EventAgentStop,
		AgentID: "a1", AgentType: "researcher",
	})
	if s.CompletedAgentCount() != 1 {
		t.Errorf("CompletedAgents = %d after 1 stop, want 1", s.CompletedAgentCount())
	}

	// Stop the other
	s.HandleEvent(collector.GateEvent{
		Timestamp: time.Now(), Event: collector.EventAgentStop,
		AgentID: "a2", AgentType: "coder",
	})
	if s.CompletedAgentCount() != 2 {
		t.Errorf("CompletedAgents = %d after 2 stops, want 2", s.CompletedAgentCount())
	}
}

func TestCompletedAgentsIgnoresHistoricalStops(t *testing.T) {
	s := NewAppState()
	past := s.CompletedAgentCount()

	// Historical events (before epoch) should populate activeRecords but
	// not increment totalCompleted.
	historical := time.Now().Add(-10 * time.Minute)
	s.HandleEvent(collector.GateEvent{
		Timestamp: historical, Event: collector.EventAgentStart,
		AgentID: "old1", AgentType: "researcher",
	})
	s.HandleEvent(collector.GateEvent{
		Timestamp: historical.Add(time.Minute), Event: collector.EventAgentStop,
		AgentID: "old1", AgentType: "researcher",
	})
	if s.CompletedAgentCount() != past {
		t.Errorf("historical stop incremented CompletedAgents: got %d, want %d", s.CompletedAgentCount(), past)
	}

	// Live event (after epoch) should increment.
	s.HandleEvent(collector.GateEvent{
		Timestamp: time.Now(), Event: collector.EventAgentStart,
		AgentID: "new1", AgentType: "coder",
	})
	s.HandleEvent(collector.GateEvent{
		Timestamp: time.Now(), Event: collector.EventAgentStop,
		AgentID: "new1", AgentType: "coder",
	})
	if s.CompletedAgentCount() != past+1 {
		t.Errorf("live stop did not increment CompletedAgents: got %d, want %d", s.CompletedAgentCount(), past+1)
	}
}

func TestCompletedAgentsCountsOrphanStop(t *testing.T) {
	s := NewAppState()

	// Stop an agent that was never started — orphan handling increments counter
	s.HandleEvent(collector.GateEvent{
		Timestamp: time.Now(), Event: collector.EventAgentStop,
		AgentID: "phantom", AgentType: "ghost",
	})
	if s.CompletedAgentCount() != 1 {
		t.Errorf("CompletedAgents = %d for orphan stop, want 1", s.CompletedAgentCount())
	}
	if s.OrphanStopCount() != 1 {
		t.Errorf("OrphanStopCount = %d, want 1", s.OrphanStopCount())
	}
}

func TestHandleEventParallelDisengaged(t *testing.T) {
	s := NewAppState()
	ev := collector.GateEvent{
		Timestamp: time.Now(),
		Event:     collector.EventParallelDisengaged,
	}
	s.HandleEvent(ev)

	if s.Alerts.Len() != 1 {
		t.Fatalf("Alerts.Len = %d, want 1", s.Alerts.Len())
	}
	alert := s.Alerts.At(0)
	if alert.Message != "parallel mode disengaged" {
		t.Errorf("alert message = %q, want %q", alert.Message, "parallel mode disengaged")
	}
	if alert.Level != ThreatCool {
		t.Errorf("alert level = %v, want ThreatCool", alert.Level)
	}
}

func TestOfflineSinceStableAcrossConsecutiveOfflineTicks(t *testing.T) {
	s := NewAppState()
	now := time.Now()

	s.Update(testSnap(t, withTime(now), withOnline(true)))
	s.Update(testSnap(t, withTime(now.Add(1*time.Second)), withOnline(false)))
	firstOfflineSince := s.OfflineSince

	s.Update(testSnap(t, withTime(now.Add(2*time.Second)), withOnline(false)))
	if !s.OfflineSince.Equal(firstOfflineSince) {
		t.Errorf("OfflineSince changed on consecutive offline tick: %v → %v", firstOfflineSince, s.OfflineSince)
	}

	s.Update(testSnap(t, withTime(now.Add(5*time.Second)), withOnline(false)))
	if s.OfflineDuration < 4*time.Second {
		t.Errorf("OfflineDuration = %v, want >= 4s after 3 offline ticks", s.OfflineDuration)
	}
	if !s.OfflineSince.Equal(firstOfflineSince) {
		t.Errorf("OfflineSince changed after third offline tick")
	}
}

func TestSmoothedCountsDecayAndPrune(t *testing.T) {
	s := NewAppState()
	now := time.Now()

	// Tick 1: V processes exist → SmoothedCounts["V"] seeded
	procs := []collector.ProcessInfo{{PID: 1, TypeCode: "V"}}
	s.Update(testSnap(t, withTime(now), withProcs(procs)))
	if _, ok := s.SmoothedCounts["V"]; !ok {
		t.Fatal("SmoothedCounts[V] should be seeded after first tick with V procs")
	}

	// Subsequent ticks with no V procs → SmoothedCounts["V"] should decay toward zero
	for i := 1; i <= 100; i++ {
		s.Update(testSnap(t, withTime(now.Add(time.Duration(i)*time.Second))))
	}

	if _, ok := s.SmoothedCounts["V"]; ok {
		t.Errorf("SmoothedCounts[V] = %f, want pruned (deleted) after 100 ticks with no V procs", s.SmoothedCounts["V"])
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

// ── CategoryFilter ──────────────────────────────────────────

func TestSetCategoryFilter(t *testing.T) {
	s := NewAppState()
	s.SetCategoryFilter("build")
	if s.CategoryFilter != "build" {
		t.Errorf("got %q, want %q", s.CategoryFilter, "build")
	}
	s.SetCategoryFilter("")
	if s.CategoryFilter != "" {
		t.Errorf("got %q, want empty", s.CategoryFilter)
	}
}

func TestToggleCategoryFilter(t *testing.T) {
	s := NewAppState()

	// First toggle sets
	s.ToggleCategoryFilter("build")
	if s.CategoryFilter != "build" {
		t.Errorf("first toggle: got %q, want %q", s.CategoryFilter, "build")
	}

	// Same name clears
	s.ToggleCategoryFilter("build")
	if s.CategoryFilter != "" {
		t.Errorf("second toggle: got %q, want empty", s.CategoryFilter)
	}

	// Different name sets to new
	s.ToggleCategoryFilter("build")
	s.ToggleCategoryFilter("shell")
	if s.CategoryFilter != "shell" {
		t.Errorf("different toggle: got %q, want %q", s.CategoryFilter, "shell")
	}
}

func TestCycleCategoryFilter(t *testing.T) {
	s := NewAppState()
	// Seed smoothed cats so build (fixed) + shell (fixed) + node are visible
	s.SmoothedCats["build"] = 2.0
	s.SmoothedCats["shell"] = 1.0
	s.SmoothedCats["node"] = 3.0

	visible := VisibleCategoryNames(s.SmoothedCats)

	// Forward from empty → first visible
	s.CycleCategoryFilter(+1)
	if s.CategoryFilter != visible[0] {
		t.Errorf("forward from empty: got %q, want %q", s.CategoryFilter, visible[0])
	}

	// Forward through all → wraps to empty
	for range visible[1:] {
		s.CycleCategoryFilter(+1)
	}
	// Should be on last
	if s.CategoryFilter != visible[len(visible)-1] {
		t.Errorf("forward to last: got %q, want %q", s.CategoryFilter, visible[len(visible)-1])
	}
	s.CycleCategoryFilter(+1)
	if s.CategoryFilter != "" {
		t.Errorf("forward past last: got %q, want empty", s.CategoryFilter)
	}

	// Backward from empty → last visible
	s.CycleCategoryFilter(-1)
	if s.CategoryFilter != visible[len(visible)-1] {
		t.Errorf("backward from empty: got %q, want %q", s.CategoryFilter, visible[len(visible)-1])
	}

	// direction=0 is a no-op
	prev := s.CategoryFilter
	s.CycleCategoryFilter(0)
	if s.CategoryFilter != prev {
		t.Errorf("direction=0: got %q, want %q (unchanged)", s.CategoryFilter, prev)
	}
}

func TestCycleCategoryFilterFixedOnly(t *testing.T) {
	s := NewAppState()
	// No dynamic categories seeded — only build + shell are visible (fixed)
	visible := VisibleCategoryNames(s.SmoothedCats)
	if len(visible) != 2 {
		t.Fatalf("expected 2 fixed categories, got %d: %v", len(visible), visible)
	}

	// Forward cycles through build → shell → empty
	s.CycleCategoryFilter(+1)
	if s.CategoryFilter != "build" {
		t.Errorf("got %q, want build", s.CategoryFilter)
	}
	s.CycleCategoryFilter(+1)
	if s.CategoryFilter != "shell" {
		t.Errorf("got %q, want shell", s.CategoryFilter)
	}
	s.CycleCategoryFilter(+1)
	if s.CategoryFilter != "" {
		t.Errorf("got %q, want empty", s.CategoryFilter)
	}
}

func TestVisibleCategoryNames(t *testing.T) {
	smoothed := map[string]float64{
		"build": 0,   // fixed — always visible
		"shell": 0,   // fixed — always visible
		"node":  5.0, // dynamic, above threshold
		"go":    0.3, // dynamic, below threshold
		"rust":  0.5, // dynamic, at threshold
	}
	got := VisibleCategoryNames(smoothed)

	want := map[string]bool{"build": true, "shell": true, "node": true, "rust": true}
	gotSet := map[string]bool{}
	for _, name := range got {
		gotSet[name] = true
	}
	if len(got) != len(want) {
		t.Fatalf("got %v, want keys %v", got, want)
	}
	for name := range want {
		if !gotSet[name] {
			t.Errorf("missing %q in result", name)
		}
	}
	if gotSet["go"] {
		t.Error("go should not be visible (0.3 < 0.5)")
	}

	// Verify order matches collector.Categories
	prevOrder := -1
	for _, name := range got {
		for _, cat := range collector.Categories {
			if cat.Name == name && cat.Order < prevOrder {
				t.Errorf("order violation: %q (order %d) after order %d", name, cat.Order, prevOrder)
			}
			if cat.Name == name {
				prevOrder = cat.Order
			}
		}
	}
}

func TestPushAlert(t *testing.T) {
	s := NewAppState()
	alert := AlertEntry{
		Time:    time.Now(),
		Message: "test alert",
		Level:   ThreatWarm,
	}
	s.PushAlert(alert)

	if s.Alerts.Len() != 1 {
		t.Fatalf("Alerts.Len = %d, want 1", s.Alerts.Len())
	}
	got := s.Alerts.Peek()
	if got.Message != "test alert" {
		t.Errorf("alert message = %q, want %q", got.Message, "test alert")
	}
}

// ── SessionUptime ───────────────────────────────────────────

func TestSessionUptimeZeroWithNoAgents(t *testing.T) {
	s := NewAppState()
	if got := s.SessionUptime(); got != 0 {
		t.Errorf("SessionUptime with no agents = %v, want 0", got)
	}
}

func TestSessionUptimeTracksEarliestStart(t *testing.T) {
	s := NewAppState()
	t0 := time.Now().Add(-5 * time.Minute)
	s.HandleEvent(collector.GateEvent{
		Event: collector.EventAgentStart, AgentID: "a1", Timestamp: t0,
	})
	s.HandleEvent(collector.GateEvent{
		Event: collector.EventAgentStart, AgentID: "a2", Timestamp: t0.Add(2 * time.Minute),
	})
	uptime := s.SessionUptime()
	if uptime < 4*time.Minute || uptime > 6*time.Minute {
		t.Errorf("SessionUptime = %v, want ~5m", uptime)
	}
}

func TestSessionUptimeResetsOnCounterReset(t *testing.T) {
	s := NewAppState()
	t0 := time.Now().Add(-10 * time.Minute)
	s.HandleEvent(collector.GateEvent{
		Event: collector.EventAgentStart, AgentID: "a1", Timestamp: t0,
	})
	s.HandleEvent(collector.GateEvent{
		Event: collector.EventCounterReset, Timestamp: time.Now(),
	})
	if got := s.SessionUptime(); got != 0 {
		t.Errorf("SessionUptime after reset = %v, want 0", got)
	}
}

// ── TotalTranscriptBytes ────────────────────────────────────

func TestTotalTranscriptBytesCompletedOnly(t *testing.T) {
	s := NewAppState()
	t0 := time.Now()

	// Create temp transcript files with known sizes so statTranscript
	// populates TranscriptBytes on agent.stop.
	dir := t.TempDir()
	f1 := filepath.Join(dir, "agent-a1.jsonl")
	f2 := filepath.Join(dir, "agent-a2.jsonl")
	os.WriteFile(f1, make([]byte, 1024), 0644) // 1KB
	os.WriteFile(f2, make([]byte, 2048), 0644) // 2KB

	s.HandleEvent(collector.GateEvent{
		Event: collector.EventAgentStart, AgentID: "a1", Timestamp: t0,
	})
	s.HandleEvent(collector.GateEvent{
		Event: collector.EventAgentStop, AgentID: "a1", TranscriptPath: f1, Timestamp: t0.Add(time.Second),
	})
	s.HandleEvent(collector.GateEvent{
		Event: collector.EventAgentStart, AgentID: "a2", Timestamp: t0.Add(2 * time.Second),
	})
	s.HandleEvent(collector.GateEvent{
		Event: collector.EventAgentStop, AgentID: "a2", TranscriptPath: f2, Timestamp: t0.Add(3 * time.Second),
	})
	// Active agent — should NOT contribute (TranscriptBytes == 0)
	s.HandleEvent(collector.GateEvent{
		Event: collector.EventAgentStart, AgentID: "a3", Timestamp: t0.Add(4 * time.Second),
	})

	got := s.TotalTranscriptBytes()
	want := int64(1024 + 2048)
	if got != want {
		t.Errorf("TotalTranscriptBytes = %d, want %d", got, want)
	}
}

// ── AgentTypeCounts ─────────────────────────────────────────

func TestAgentTypeCounts(t *testing.T) {
	s := NewAppState()
	t0 := time.Now()
	// 2 general-purpose, 1 Explore (active), 1 Plan (completed)
	s.HandleEvent(collector.GateEvent{
		Event: collector.EventAgentStart, AgentID: "a1", AgentType: "general-purpose", Timestamp: t0,
	})
	s.HandleEvent(collector.GateEvent{
		Event: collector.EventAgentStop, AgentID: "a1", AgentType: "general-purpose", Timestamp: t0.Add(time.Second),
	})
	s.HandleEvent(collector.GateEvent{
		Event: collector.EventAgentStart, AgentID: "a2", AgentType: "general-purpose", Timestamp: t0.Add(2 * time.Second),
	})
	s.HandleEvent(collector.GateEvent{
		Event: collector.EventAgentStop, AgentID: "a2", AgentType: "general-purpose", Timestamp: t0.Add(3 * time.Second),
	})
	s.HandleEvent(collector.GateEvent{
		Event: collector.EventAgentStart, AgentID: "a3", AgentType: "Explore", Timestamp: t0.Add(4 * time.Second),
	})
	// a3 still active
	s.HandleEvent(collector.GateEvent{
		Event: collector.EventAgentStart, AgentID: "a4", AgentType: "Plan", Timestamp: t0.Add(5 * time.Second),
	})
	s.HandleEvent(collector.GateEvent{
		Event: collector.EventAgentStop, AgentID: "a4", AgentType: "Plan", Timestamp: t0.Add(6 * time.Second),
	})

	counts := s.AgentTypeCounts()
	if counts["general-purpose"] != 2 {
		t.Errorf("general-purpose = %d, want 2", counts["general-purpose"])
	}
	if counts["Explore"] != 1 {
		t.Errorf("Explore = %d, want 1", counts["Explore"])
	}
	if counts["Plan"] != 1 {
		t.Errorf("Plan = %d, want 1", counts["Plan"])
	}
}
