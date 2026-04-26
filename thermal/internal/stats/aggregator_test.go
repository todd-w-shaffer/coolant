package stats

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/toddwshaffer/coolant/thermal/internal/collector"
)

// newTestAggregator constructs an Aggregator pointed at temp paths so
// tests don't touch user state. transcriptStat returns 0 by default —
// individual tests can override via the optional third arg.
func newTestAggregator(t *testing.T) *Aggregator {
	t.Helper()
	dir := t.TempDir()
	return New(Config{
		CachePath:    filepath.Join(dir, "stats.json"),
		JSONLPath:    filepath.Join(dir, "events.jsonl"),
		DegradedPath: filepath.Join(dir, "degraded.count"),
		// TranscriptStat zero-value → falls back to os.Stat-derived size,
		// which returns 0 for missing files. Most tests don't care.
	})
}

func mkEvent(schema int, event, agentID, sessionID string, ts time.Time) collector.GateEvent {
	return collector.GateEvent{
		Schema:    schema,
		Event:     event,
		Timestamp: ts,
		AgentID:   agentID,
		SessionID: sessionID,
		AgentType: "Explore",
		Project:   "coolant",
	}
}

// ── schema gate ────────────────────────────────────────────

func TestSchemaGateDropsPreVersioning(t *testing.T) {
	a := newTestAggregator(t)
	now := time.Now().UTC()
	// Schema=0 is the zero value (no schema field on the event).
	a.Fold(mkEvent(0, collector.EventAgentStart, "a1", "s1", now), 0)
	if got := a.Snapshot().Lifetime().AgentsStarted; got != 0 {
		t.Errorf("schema=0 event was folded; want 0 starts, got %d", got)
	}
}

func TestSchemaGateDropsFutureSchema(t *testing.T) {
	a := newTestAggregator(t)
	now := time.Now().UTC()
	a.Fold(mkEvent(MaxKnownSchema+1, collector.EventAgentStart, "a1", "s1", now), 0)
	if got := a.Snapshot().Lifetime().AgentsStarted; got != 0 {
		t.Errorf("schema=%d (>MaxKnownSchema) was folded; want 0 starts, got %d", MaxKnownSchema+1, got)
	}
}

func TestSchemaGateAcceptsCurrentSchema(t *testing.T) {
	a := newTestAggregator(t)
	now := time.Now().UTC()
	a.Fold(mkEvent(1, collector.EventAgentStart, "a1", "s1", now), 0)
	if got := a.Snapshot().Lifetime().AgentsStarted; got != 1 {
		t.Errorf("schema=1 event dropped; want 1 start, got %d", got)
	}
}

func TestUnknownEventTypeIgnored(t *testing.T) {
	a := newTestAggregator(t)
	now := time.Now().UTC()
	a.Fold(mkEvent(1, "future.thing", "a1", "s1", now), 0)
	// No panic, no counter touched.
	if got := a.Snapshot().Lifetime(); got != (Counters{}) {
		t.Errorf("unknown event type changed counters: %+v", got)
	}
}

// ── counter folding ────────────────────────────────────────

func TestFoldAgentStartIncrementsCounters(t *testing.T) {
	a := newTestAggregator(t)
	now := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
	a.Fold(mkEvent(1, collector.EventAgentStart, "a1", "s1", now), 0)

	snap := a.Snapshot()
	life := snap.Lifetime()
	if life.AgentsStarted != 1 {
		t.Errorf("AgentsStarted: want 1, got %d", life.AgentsStarted)
	}
	if life.Sessions != 1 {
		t.Errorf("Sessions (first sighting of s1): want 1, got %d", life.Sessions)
	}
	if snap.ByType["Explore"] != 1 {
		t.Errorf("ByType[Explore]: want 1, got %d", snap.ByType["Explore"])
	}
	if snap.ByProject["coolant"] != 1 {
		t.Errorf("ByProject[coolant]: want 1, got %d", snap.ByProject["coolant"])
	}
}

func TestFoldAgentStopMatchedIncrementsCompleted(t *testing.T) {
	a := newTestAggregator(t)
	now := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
	a.Fold(mkEvent(1, collector.EventAgentStart, "a1", "s1", now), 0)
	a.Fold(mkEvent(1, collector.EventAgentStop, "a1", "s1", now.Add(time.Minute)), 0)

	life := a.Snapshot().Lifetime()
	if life.AgentsCompleted != 1 {
		t.Errorf("AgentsCompleted: want 1, got %d", life.AgentsCompleted)
	}
	if life.AgentsOrphaned != 0 {
		t.Errorf("AgentsOrphaned (matched stop): want 0, got %d", life.AgentsOrphaned)
	}
}

func TestFoldAgentStopOrphanedIncrementsOrphaned(t *testing.T) {
	a := newTestAggregator(t)
	now := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
	// Stop with no matching start.
	a.Fold(mkEvent(1, collector.EventAgentStop, "ghost", "s1", now), 0)

	life := a.Snapshot().Lifetime()
	if life.AgentsOrphaned != 1 {
		t.Errorf("AgentsOrphaned: want 1, got %d", life.AgentsOrphaned)
	}
	if life.AgentsCompleted != 0 {
		t.Errorf("AgentsCompleted (orphan stop): want 0, got %d", life.AgentsCompleted)
	}
}

func TestFoldGateCapIncrementsCounter(t *testing.T) {
	a := newTestAggregator(t)
	now := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
	a.Fold(mkEvent(1, collector.EventGateCap, "", "s1", now), 0)
	if got := a.Snapshot().Lifetime().GateCapEvents; got != 1 {
		t.Errorf("GateCapEvents: want 1, got %d", got)
	}
}

func TestFoldCounterResetIsNoOp(t *testing.T) {
	a := newTestAggregator(t)
	now := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
	a.Fold(mkEvent(1, collector.EventAgentStart, "a1", "s1", now), 0)
	a.Fold(mkEvent(1, collector.EventCounterReset, "", "", now.Add(time.Second)), 0)

	// counter.reset doesn't reset stats — sessions are keyed on session_id.
	life := a.Snapshot().Lifetime()
	if life.AgentsStarted != 1 {
		t.Errorf("counter.reset wiped AgentsStarted: want 1, got %d", life.AgentsStarted)
	}
}

func TestFoldSessionDedup(t *testing.T) {
	a := newTestAggregator(t)
	now := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
	// Three agents in one session — sessions counter increments once.
	a.Fold(mkEvent(1, collector.EventAgentStart, "a1", "s1", now), 0)
	a.Fold(mkEvent(1, collector.EventAgentStart, "a2", "s1", now.Add(time.Second)), 0)
	a.Fold(mkEvent(1, collector.EventAgentStart, "a3", "s1", now.Add(2*time.Second)), 0)
	if got := a.Snapshot().Lifetime().Sessions; got != 1 {
		t.Errorf("Sessions (one session, 3 starts): want 1, got %d", got)
	}
}

// ── transcript bytes ───────────────────────────────────────

func TestFoldTranscriptBytesViaStat(t *testing.T) {
	dir := t.TempDir()
	transcript := filepath.Join(dir, "agent.jsonl")
	if err := os.WriteFile(transcript, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}

	a := New(Config{
		CachePath:    filepath.Join(dir, "stats.json"),
		JSONLPath:    filepath.Join(dir, "events.jsonl"),
		DegradedPath: filepath.Join(dir, "degraded.count"),
	})

	now := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
	stop := mkEvent(1, collector.EventAgentStop, "a1", "s1", now)
	stop.TranscriptPath = transcript
	a.Fold(stop, 0)

	if got := a.Snapshot().Lifetime().TranscriptBytesTotal; got != 5 {
		t.Errorf("TranscriptBytesTotal: want 5, got %d", got)
	}
}

func TestFoldTranscriptBytesMissingContributesZero(t *testing.T) {
	a := newTestAggregator(t)
	now := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
	stop := mkEvent(1, collector.EventAgentStop, "a1", "s1", now)
	stop.TranscriptPath = "/does/not/exist.jsonl"
	a.Fold(stop, 0)

	// Stop still counts as completed/orphaned, but no bytes added.
	if got := a.Snapshot().Lifetime().TranscriptBytesTotal; got != 0 {
		t.Errorf("missing transcript should contribute 0, got %d", got)
	}
}

// ── degraded counter readout ───────────────────────────────

func TestDegradedWritesTotalReadAtSnapshot(t *testing.T) {
	a := newTestAggregator(t)
	// Pre-write 3 newlines — same shape coolant_event's fallback emits.
	if err := os.WriteFile(a.cfg.DegradedPath, []byte("\n\n\n"), 0o644); err != nil {
		t.Fatalf("write degraded counter: %v", err)
	}
	if got := a.Snapshot().Lifetime().DegradedWritesTotal; got != 3 {
		t.Errorf("DegradedWritesTotal: want 3, got %d", got)
	}
}

func TestDegradedWritesTotalAbsentIsZero(t *testing.T) {
	a := newTestAggregator(t)
	// File doesn't exist.
	if got := a.Snapshot().Lifetime().DegradedWritesTotal; got != 0 {
		t.Errorf("DegradedWritesTotal (no file): want 0, got %d", got)
	}
}

// ── records ────────────────────────────────────────────────

func TestFoldDropsEmptyAgentType(t *testing.T) {
	a := newTestAggregator(t)
	now := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
	// CC orphan-stop bug pattern: empty agent_type with populated agent_id.
	// Pre-spec behavior would have inflated AgentsOrphaned. Defensive
	// skip means counters stay at zero — the bash hook is the primary
	// drop site, but this guard catches in-flight degraded-write
	// fallbacks and historical JSONL replays.
	stop := mkEvent(1, collector.EventAgentStop, "ghost", "s1", now)
	stop.AgentType = ""
	a.Fold(stop, 0)

	life := a.Snapshot().Lifetime()
	if got := life.AgentsCompleted; got != 0 {
		t.Errorf("empty agent_type stop should not complete; got %d", got)
	}
	if got := life.AgentsOrphaned; got != 0 {
		t.Errorf("empty agent_type stop should not orphan; got %d", got)
	}
}

func TestRecordPeakConcurrent(t *testing.T) {
	a := newTestAggregator(t)
	now := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
	// 3 starts → peak 3, then 1 stop → 2 active, then 1 start → peak still 3.
	a.Fold(mkEvent(1, collector.EventAgentStart, "a1", "s1", now), 0)
	a.Fold(mkEvent(1, collector.EventAgentStart, "a2", "s1", now.Add(time.Second)), 0)
	a.Fold(mkEvent(1, collector.EventAgentStart, "a3", "s1", now.Add(2*time.Second)), 0)
	a.Fold(mkEvent(1, collector.EventAgentStop, "a1", "s1", now.Add(3*time.Second)), 0)
	a.Fold(mkEvent(1, collector.EventAgentStart, "a4", "s1", now.Add(4*time.Second)), 0)

	rec := a.Snapshot().Records.PeakConcurrent
	if rec.Value != 3 {
		t.Errorf("PeakConcurrent.Value: want 3, got %d", rec.Value)
	}
	if rec.SessionID != "s1" {
		t.Errorf("PeakConcurrent.SessionID: want s1, got %q", rec.SessionID)
	}
}

func TestRecordLongestAgent(t *testing.T) {
	a := newTestAggregator(t)
	now := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
	// a1: 60s; a2: 30s. Longest = 60.
	a.Fold(mkEvent(1, collector.EventAgentStart, "a1", "s1", now), 0)
	a.Fold(mkEvent(1, collector.EventAgentStart, "a2", "s1", now.Add(10*time.Second)), 0)
	a.Fold(mkEvent(1, collector.EventAgentStop, "a2", "s1", now.Add(40*time.Second)), 0)
	a.Fold(mkEvent(1, collector.EventAgentStop, "a1", "s1", now.Add(60*time.Second)), 0)

	rec := a.Snapshot().Records.LongestAgentS
	if rec.Value != 60 {
		t.Errorf("LongestAgentS.Value: want 60s, got %d", rec.Value)
	}
	if rec.AgentID != "a1" {
		t.Errorf("LongestAgentS.AgentID: want a1, got %q", rec.AgentID)
	}
}

func TestRecordLongestSession(t *testing.T) {
	a := newTestAggregator(t)
	now := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
	// s1: first start at t+0, last stop at t+100 → 100s.
	a.Fold(mkEvent(1, collector.EventAgentStart, "a1", "s1", now), 0)
	a.Fold(mkEvent(1, collector.EventAgentStop, "a1", "s1", now.Add(100*time.Second)), 0)
	// s2: 30s span; s1 stays the longest.
	a.Fold(mkEvent(1, collector.EventAgentStart, "a2", "s2", now.Add(200*time.Second)), 0)
	a.Fold(mkEvent(1, collector.EventAgentStop, "a2", "s2", now.Add(230*time.Second)), 0)

	rec := a.Snapshot().Records.LongestSessionS
	if rec.Value != 100 {
		t.Errorf("LongestSessionS.Value: want 100s, got %d", rec.Value)
	}
	if rec.SessionID != "s1" {
		t.Errorf("LongestSessionS.SessionID: want s1, got %q", rec.SessionID)
	}
}

func TestRecordMostAgentsSession(t *testing.T) {
	a := newTestAggregator(t)
	now := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
	// s1: 5 agents; s2: 2 agents.
	for i := 0; i < 5; i++ {
		a.Fold(mkEvent(1, collector.EventAgentStart, "a"+string(rune('0'+i)), "s1", now.Add(time.Duration(i)*time.Second)), 0)
	}
	a.Fold(mkEvent(1, collector.EventAgentStart, "b1", "s2", now.Add(time.Hour)), 0)
	a.Fold(mkEvent(1, collector.EventAgentStart, "b2", "s2", now.Add(time.Hour+time.Second)), 0)

	rec := a.Snapshot().Records.MostAgentsSession
	if rec.Value != 5 {
		t.Errorf("MostAgentsSession.Value: want 5, got %d", rec.Value)
	}
	if rec.SessionID != "s1" {
		t.Errorf("MostAgentsSession.SessionID: want s1, got %q", rec.SessionID)
	}
}

func TestRecordBiggestBurst(t *testing.T) {
	a := newTestAggregator(t)
	now := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
	// 4 starts within 1 second → burst of 4.
	for i := 0; i < 4; i++ {
		a.Fold(mkEvent(1, collector.EventAgentStart, "a"+string(rune('0'+i)), "s1", now.Add(time.Duration(i*250)*time.Millisecond)), 0)
	}
	// 1 start much later — falls outside the 2s window, doesn't extend the burst.
	a.Fold(mkEvent(1, collector.EventAgentStart, "z", "s1", now.Add(10*time.Second)), 0)

	rec := a.Snapshot().Records.BiggestBurst
	if rec.Count != 4 {
		t.Errorf("BiggestBurst.Count: want 4, got %d", rec.Count)
	}
	if rec.WindowS != 2 {
		t.Errorf("BiggestBurst.WindowS: want 2, got %d", rec.WindowS)
	}
}

// ── first_seen anchor ──────────────────────────────────────

func TestFirstSeenSetOnFirstFold(t *testing.T) {
	a := newTestAggregator(t)
	first := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
	a.Fold(mkEvent(1, collector.EventAgentStart, "a1", "s1", first), 0)
	a.Fold(mkEvent(1, collector.EventAgentStart, "a2", "s2", first.Add(time.Hour)), 0)

	got := a.Snapshot().FirstSeen
	if !got.Equal(first) {
		t.Errorf("FirstSeen: want %v, got %v", first, got)
	}
}

// ── daily buckets ──────────────────────────────────────────

func TestDailyBucketsSplitByUTCDay(t *testing.T) {
	a := newTestAggregator(t)
	day1 := time.Date(2026, 4, 24, 10, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)

	a.Fold(mkEvent(1, collector.EventAgentStart, "a1", "s1", day1), 0)
	a.Fold(mkEvent(1, collector.EventAgentStart, "a2", "s2", day2), 0)
	a.Fold(mkEvent(1, collector.EventAgentStart, "a3", "s3", day2.Add(time.Hour)), 0)

	daily := a.Snapshot().Daily
	if daily["2026-04-24"].AgentsStarted != 1 {
		t.Errorf("day1 starts: want 1, got %d", daily["2026-04-24"].AgentsStarted)
	}
	if daily["2026-04-25"].AgentsStarted != 2 {
		t.Errorf("day2 starts: want 2, got %d", daily["2026-04-25"].AgentsStarted)
	}
}

func TestCrossDaySessionAttributedToFirstStartDay(t *testing.T) {
	a := newTestAggregator(t)
	day1 := time.Date(2026, 4, 24, 23, 30, 0, 0, time.UTC)
	day2 := time.Date(2026, 4, 25, 0, 30, 0, 0, time.UTC)
	day3 := time.Date(2026, 4, 26, 1, 0, 0, 0, time.UTC)

	// Session s1 starts on day1, agents continue on day2 + day3.
	a.Fold(mkEvent(1, collector.EventAgentStart, "a1", "s1", day1), 0)
	a.Fold(mkEvent(1, collector.EventAgentStart, "a2", "s1", day2), 0)
	a.Fold(mkEvent(1, collector.EventAgentStart, "a3", "s1", day3), 0)

	daily := a.Snapshot().Daily
	if daily["2026-04-24"].Sessions != 1 {
		t.Errorf("day1 sessions: want 1, got %d", daily["2026-04-24"].Sessions)
	}
	if daily["2026-04-25"].Sessions != 0 {
		t.Errorf("day2 sessions: want 0 (already counted on day1), got %d", daily["2026-04-25"].Sessions)
	}
	if daily["2026-04-26"].Sessions != 0 {
		t.Errorf("day3 sessions: want 0, got %d", daily["2026-04-26"].Sessions)
	}
}

// ── Window + Lifetime ──────────────────────────────────────

func TestWindowSumsLastNDays(t *testing.T) {
	a := newTestAggregator(t)
	now := time.Now().UTC().Truncate(24 * time.Hour) // start of today
	// Plant events on today, yesterday, 3 days ago, 10 days ago.
	for _, ago := range []int{0, 1, 3, 10} {
		ts := now.Add(-time.Duration(ago) * 24 * time.Hour).Add(time.Hour)
		a.Fold(mkEvent(1, collector.EventAgentStart, "a"+string(rune('0'+ago)), "s"+string(rune('0'+ago)), ts), 0)
	}
	// Window(7) sums today + yesterday + 3 days ago = 3 starts.
	w7 := a.Window(7)
	if w7.AgentsStarted != 3 {
		t.Errorf("Window(7).AgentsStarted: want 3, got %d", w7.AgentsStarted)
	}
	// Window(2) sums today + yesterday = 2.
	w2 := a.Window(2)
	if w2.AgentsStarted != 2 {
		t.Errorf("Window(2).AgentsStarted: want 2, got %d", w2.AgentsStarted)
	}
}

func TestLifetimeSumsAllDaily(t *testing.T) {
	a := newTestAggregator(t)
	now := time.Now().UTC()
	for i := 0; i < 5; i++ {
		ts := now.Add(-time.Duration(i*24) * time.Hour)
		a.Fold(mkEvent(1, collector.EventAgentStart, "a"+string(rune('0'+i)), "s"+string(rune('0'+i)), ts), 0)
	}
	if got := a.Snapshot().Lifetime().AgentsStarted; got != 5 {
		t.Errorf("Lifetime.AgentsStarted: want 5, got %d", got)
	}
}

func TestVisibleWindowsScalesWithInstallAge(t *testing.T) {
	cases := []struct {
		ageDays int
		want    []string
	}{
		{0, []string{"7d", "alltime"}},
		{29, []string{"7d", "alltime"}},
		{30, []string{"7d", "30d"}},
		{59, []string{"7d", "30d"}},
		{60, []string{"7d", "30d", "60d"}},
		{89, []string{"7d", "30d", "60d"}},
		{90, []string{"7d", "30d", "90d", "alltime"}},
		{500, []string{"7d", "30d", "90d", "alltime"}},
	}
	for _, tc := range cases {
		a := newTestAggregator(t)
		// Backdate first_seen by injecting a very old fold.
		first := time.Now().UTC().Add(-time.Duration(tc.ageDays) * 24 * time.Hour)
		a.Fold(mkEvent(1, collector.EventAgentStart, "a", "s", first), 0)

		got := a.VisibleWindows()
		if !equalStringSlice(got, tc.want) {
			t.Errorf("ageDays=%d: VisibleWindows want %v, got %v", tc.ageDays, tc.want, got)
		}
	}
}

func equalStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ── lifecycle (session.start / session.end) ────────────────

func TestSessionEndUsesLifecycleMath(t *testing.T) {
	a := newTestAggregator(t)
	now := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
	// Lifecycle: session.start, agent runs, session.end. Duration
	// reads from explicit start/end, not from first agent.start.
	a.Fold(mkEvent(1, collector.EventSessionStart, "", "s1", now), 0)
	a.Fold(mkEvent(1, collector.EventAgentStart, "a1", "s1", now.Add(30*time.Second)), 0)
	a.Fold(mkEvent(1, collector.EventAgentStop, "a1", "s1", now.Add(50*time.Second)), 0)
	a.Fold(mkEvent(1, collector.EventSessionEnd, "", "s1", now.Add(120*time.Second)), 0)

	rec := a.Snapshot().Records.LongestSessionS
	if rec.Value != 120 {
		t.Errorf("LongestSessionS via lifecycle math: want 120, got %d", rec.Value)
	}
	if rec.SessionID != "s1" {
		t.Errorf("LongestSessionS.SessionID: want s1, got %q", rec.SessionID)
	}
}

func TestSessionEndFallbackToInferred(t *testing.T) {
	a := newTestAggregator(t)
	now := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
	// No session.start (pre-rollout JSONL); session.end falls back to
	// (end - first agent.start) duration.
	a.Fold(mkEvent(1, collector.EventAgentStart, "a1", "s1", now), 0)
	a.Fold(mkEvent(1, collector.EventAgentStop, "a1", "s1", now.Add(40*time.Second)), 0)
	a.Fold(mkEvent(1, collector.EventSessionEnd, "", "s1", now.Add(75*time.Second)), 0)

	rec := a.Snapshot().Records.LongestSessionS
	if rec.Value != 75 {
		t.Errorf("inferred-fallback LongestSessionS: want 75, got %d", rec.Value)
	}
}

func TestSessionEndSynthesizesOrphans(t *testing.T) {
	a := newTestAggregator(t)
	now := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
	// Three active agents in s1; session.end fires before any stop.
	a.Fold(mkEvent(1, collector.EventSessionStart, "", "s1", now), 0)
	a.Fold(mkEvent(1, collector.EventAgentStart, "a1", "s1", now.Add(time.Second)), 0)
	a.Fold(mkEvent(1, collector.EventAgentStart, "a2", "s1", now.Add(2*time.Second)), 0)
	a.Fold(mkEvent(1, collector.EventAgentStart, "a3", "s1", now.Add(3*time.Second)), 0)
	a.Fold(mkEvent(1, collector.EventSessionEnd, "", "s1", now.Add(60*time.Second)), 0)

	life := a.Snapshot().Lifetime()
	if life.AgentsOrphaned != 3 {
		t.Errorf("AgentsOrphaned after session.end: want 3, got %d", life.AgentsOrphaned)
	}
	// Verify the active set was cleared.
	a.mu.RLock()
	defer a.mu.RUnlock()
	if got := len(a.agentStarts); got != 0 {
		t.Errorf("agentStarts: want 0 after orphan synthesis, got %d", got)
	}
}

func TestSessionEndIdempotentForRepeatedEnds(t *testing.T) {
	a := newTestAggregator(t)
	now := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
	a.Fold(mkEvent(1, collector.EventSessionStart, "", "s1", now), 0)
	a.Fold(mkEvent(1, collector.EventAgentStart, "a1", "s1", now.Add(time.Second)), 0)
	a.Fold(mkEvent(1, collector.EventSessionEnd, "", "s1", now.Add(60*time.Second)), 0)
	// Second session.end (e.g., from JSONL replay) — must not
	// double-count orphans.
	a.Fold(mkEvent(1, collector.EventSessionEnd, "", "s1", now.Add(60*time.Second)), 0)
	if got := a.Snapshot().Lifetime().AgentsOrphaned; got != 1 {
		t.Errorf("repeat session.end double-counted orphans: got %d", got)
	}
}

func TestSessionStartIdempotent(t *testing.T) {
	a := newTestAggregator(t)
	now := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
	// Two session.start events for the same sid (e.g., resume + clear
	// both bypassing the matcher in some hypothetical config). The
	// EARLIEST start wins so duration math doesn't shrink on replay.
	a.Fold(mkEvent(1, collector.EventSessionStart, "", "s1", now), 0)
	a.Fold(mkEvent(1, collector.EventSessionStart, "", "s1", now.Add(30*time.Second)), 0)
	a.Fold(mkEvent(1, collector.EventSessionEnd, "", "s1", now.Add(100*time.Second)), 0)

	rec := a.Snapshot().Records.LongestSessionS
	if rec.Value != 100 {
		t.Errorf("idempotent session.start: want 100s (from earliest), got %d", rec.Value)
	}
}

// ── negative-duration clamp ────────────────────────────────

func TestNegativeDurationClampsAgentStop(t *testing.T) {
	a := newTestAggregator(t)
	now := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
	// agent.stop ts < agent.start ts (NTP backstep simulation).
	a.Fold(mkEvent(1, collector.EventAgentStart, "a1", "s1", now), 0)
	a.Fold(mkEvent(1, collector.EventAgentStop, "a1", "s1", now.Add(-30*time.Second)), 0)
	// No record set; no panic. LongestAgentS stays at zero value.
	if got := a.Snapshot().Records.LongestAgentS.Value; got != 0 {
		t.Errorf("negative agent duration leaked into record: got %d", got)
	}
}

func TestNegativeDurationClampsSessionEnd(t *testing.T) {
	a := newTestAggregator(t)
	now := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
	a.Fold(mkEvent(1, collector.EventSessionStart, "", "s1", now), 0)
	// session.end before session.start (NTP backstep).
	a.Fold(mkEvent(1, collector.EventSessionEnd, "", "s1", now.Add(-1*time.Hour)), 0)
	if got := a.Snapshot().Records.LongestSessionS.Value; got != 0 {
		t.Errorf("negative session duration leaked: got %d", got)
	}
}

// ── staleness sweep ────────────────────────────────────────

func TestStaleSessionAutoClosesAtCutoff(t *testing.T) {
	a := newTestAggregator(t)
	// Backdate session.start beyond the 8h cutoff; last activity at
	// +10min. Snapshot's staleness sweep computes duration as
	// (last_activity - start) = 10min.
	start := time.Now().Add(-9 * time.Hour)
	activity := start.Add(10 * time.Minute)
	a.Fold(mkEvent(1, collector.EventSessionStart, "", "stale", start), 0)
	a.Fold(mkEvent(1, collector.EventAgentStart, "a1", "stale", activity), 0)

	rec := a.Snapshot().Records.LongestSessionS
	want := int64(10 * 60)
	if rec.Value != want {
		t.Errorf("stale-session sweep: want %d, got %d", want, rec.Value)
	}
	if rec.SessionID != "stale" {
		t.Errorf("stale-session sid: want stale, got %q", rec.SessionID)
	}
}

func TestStaleSessionRespectsCutoffBoundary(t *testing.T) {
	a := newTestAggregator(t)
	// Backdate to 7h ago — under 8h cutoff. Sweep must NOT fire.
	start := time.Now().Add(-7 * time.Hour)
	a.Fold(mkEvent(1, collector.EventSessionStart, "", "fresh", start), 0)
	a.Fold(mkEvent(1, collector.EventAgentStart, "a1", "fresh", start.Add(time.Minute)), 0)

	if got := a.Snapshot().Records.LongestSessionS.Value; got != 0 {
		t.Errorf("under-cutoff session was swept: got %d", got)
	}
}

func TestStaleSessionLateEndStillCloses(t *testing.T) {
	a := newTestAggregator(t)
	start := time.Now().Add(-10 * time.Hour)
	activity := start.Add(10 * time.Minute)
	a.Fold(mkEvent(1, collector.EventSessionStart, "", "late", start), 0)
	a.Fold(mkEvent(1, collector.EventAgentStart, "a1", "late", activity), 0)

	// Snapshot before session.end — sweep records 10min.
	first := a.Snapshot().Records.LongestSessionS.Value
	if first != 600 {
		t.Fatalf("staleness sweep precondition: want 600, got %d", first)
	}

	// Late session.end arrives — lifecycle math gets 10h, replaces
	// the swept value. No double-count, no regression.
	end := start.Add(10 * time.Hour)
	a.Fold(mkEvent(1, collector.EventSessionEnd, "", "late", end), 0)

	rec := a.Snapshot().Records.LongestSessionS
	if rec.Value != int64(10*3600) {
		t.Errorf("late session.end: want %d, got %d", 10*3600, rec.Value)
	}
}

// ── counter.underflow ──────────────────────────────────────

func TestCounterUnderflowIsFoldNoOp(t *testing.T) {
	a := newTestAggregator(t)
	now := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
	// counter.underflow is diagnostic-only at v1 — no aggregator
	// mutation, no panic.
	evt := mkEvent(1, collector.EventCounterUnderflow, "", "s1", now)
	a.Fold(evt, 0)
	if life := a.Snapshot().Lifetime(); life != (Counters{}) {
		t.Errorf("counter.underflow mutated counters: %+v", life)
	}
}

// ── pruneStale lifecycle map cleanup ──────────────────────

func TestPruneStaleDropsLifecycleMaps(t *testing.T) {
	a := newTestAggregator(t)
	old := time.Now().Add(-25 * time.Hour) // beyond staleAgentCutoff (24h)
	fresh := time.Now().Add(-1 * time.Hour)
	a.Fold(mkEvent(1, collector.EventSessionStart, "", "old", old), 0)
	a.Fold(mkEvent(1, collector.EventSessionEnd, "", "old", old.Add(time.Minute)), 0)
	a.Fold(mkEvent(1, collector.EventSessionStart, "", "fresh", fresh), 0)

	a.mu.Lock()
	a.pruneStale(time.Now())
	a.mu.Unlock()

	a.mu.RLock()
	defer a.mu.RUnlock()
	if _, ok := a.sessionStart["old"]; ok {
		t.Errorf("pruneStale left old sessionStart entry")
	}
	if _, ok := a.sessionEnded["old"]; ok {
		t.Errorf("pruneStale left old sessionEnded entry")
	}
	if _, ok := a.lastActivityForSession["old"]; ok {
		t.Errorf("pruneStale left old lastActivityForSession entry")
	}
	if _, ok := a.sessionStart["fresh"]; !ok {
		t.Errorf("pruneStale dropped fresh session — only stale entries should go")
	}
}

// ── concurrency ────────────────────────────────────────────

func TestConcurrentFoldAndSnapshot(t *testing.T) {
	a := newTestAggregator(t)
	now := time.Now().UTC()

	// One folder + many readers in parallel. -race catches torn reads.
	done := make(chan struct{})
	go func() {
		for i := 0; i < 1000; i++ {
			a.Fold(mkEvent(1, collector.EventAgentStart, "a", "s", now.Add(time.Duration(i)*time.Millisecond)), int64(i))
		}
		close(done)
	}()

	// 1000 snapshot calls during the fold storm.
	for i := 0; i < 1000; i++ {
		_ = a.Snapshot()
	}
	<-done

	// Sanity: aggregate counts match.
	if got := a.Snapshot().Lifetime().AgentsStarted; got != 1000 {
		t.Errorf("AgentsStarted after 1000 folds: want 1000, got %d", got)
	}
}
