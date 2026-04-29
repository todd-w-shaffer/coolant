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

// mkEvent constructs a GateEvent fixture. The variadic opts layer on
// extra fields like tokens / tool calls without churning existing call
// sites, which pass no opts and continue to work unchanged.
func mkEvent(schema int, event, agentID, sessionID string, ts time.Time, opts ...func(*collector.GateEvent)) collector.GateEvent {
	ev := collector.GateEvent{
		Schema:    schema,
		Event:     event,
		Timestamp: ts,
		AgentID:   agentID,
		SessionID: sessionID,
		AgentType: "Explore",
		Project:   "coolant",
	}
	for _, opt := range opts {
		opt(&ev)
	}
	return ev
}

// WithTokens sets per-agent token telemetry on a fixture event.
// Used by agent-stop telemetry tests.
func WithTokens(in, out int64) func(*collector.GateEvent) {
	return func(e *collector.GateEvent) {
		e.TokensIn = in
		e.TokensOut = out
	}
}

// WithToolCalls sets per-agent tool-call count on a fixture event.
func WithToolCalls(n int64) func(*collector.GateEvent) {
	return func(e *collector.GateEvent) {
		e.ToolCallCount = n
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
	if got := a.Snapshot().Lifetime(); !got.IsZero() {
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

	rec := a.Snapshot().Records.PeakConcurrent[0]
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

	rec := a.Snapshot().Records.LongestAgentS[0]
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

	rec := a.Snapshot().Records.LongestSessionS[0]
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

	rec := a.Snapshot().Records.MostAgentsSession[0]
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

	rec := a.Snapshot().Records.BiggestBurst[0]
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

// ── per-day shape fields (cycle 2) ─────────────────────────

func TestPeakConcurrentDayObservedOnStart(t *testing.T) {
	a := newTestAggregator(t)
	day := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		a.Fold(mkEvent(1, collector.EventAgentStart, "a"+string(rune('0'+i)), "s1", day.Add(time.Duration(i)*time.Second)), 0)
	}
	if got := a.Snapshot().Daily["2026-04-25"].PeakConcurrentDay; got != 5 {
		t.Errorf("PeakConcurrentDay after 5 starts: want 5, got %d", got)
	}
	// Two stops + one start: peak stays 5.
	a.Fold(mkEvent(1, collector.EventAgentStop, "a0", "s1", day.Add(10*time.Second)), 0)
	a.Fold(mkEvent(1, collector.EventAgentStop, "a1", "s1", day.Add(11*time.Second)), 0)
	a.Fold(mkEvent(1, collector.EventAgentStart, "a5", "s1", day.Add(12*time.Second)), 0)
	if got := a.Snapshot().Daily["2026-04-25"].PeakConcurrentDay; got != 5 {
		t.Errorf("PeakConcurrentDay should be max-only, got %d", got)
	}
}

func TestDistinctProjectsDay(t *testing.T) {
	a := newTestAggregator(t)
	day := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
	for i, project := range []string{"coolant", "thermal-enterprise", "coolant"} {
		ev := mkEvent(1, collector.EventAgentStart, "a"+string(rune('0'+i)), "s"+string(rune('0'+i)), day.Add(time.Duration(i)*time.Second))
		ev.Project = project
		a.Fold(ev, 0)
	}
	if got := a.Snapshot().Daily["2026-04-25"].DistinctProjectsDay; got != 2 {
		t.Errorf("DistinctProjectsDay (2 distinct in 3 starts): want 2, got %d", got)
	}
}

func TestDistinctSessionsDay(t *testing.T) {
	a := newTestAggregator(t)
	day := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
	for i, sid := range []string{"s1", "s1", "s2"} {
		a.Fold(mkEvent(1, collector.EventAgentStart, "a"+string(rune('0'+i)), sid, day.Add(time.Duration(i)*time.Second)), 0)
	}
	if got := a.Snapshot().Daily["2026-04-25"].DistinctSessionsDay; got != 2 {
		t.Errorf("DistinctSessionsDay (2 distinct in 3 starts): want 2, got %d", got)
	}
}

func TestPastDayEventDoesNotCorruptCurrentDaySets(t *testing.T) {
	a := newTestAggregator(t)
	dayB := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
	dayA := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)

	// Establish today (dayB) with two distinct projects.
	for i, project := range []string{"coolant", "thermal-enterprise"} {
		ev := mkEvent(1, collector.EventAgentStart, "b"+string(rune('0'+i)), "sb"+string(rune('0'+i)), dayB.Add(time.Duration(i)*time.Second))
		ev.Project = project
		a.Fold(ev, 0)
	}
	if got := a.Snapshot().Daily["2026-04-26"].DistinctProjectsDay; got != 2 {
		t.Fatalf("precondition: dayB DistinctProjectsDay=2, got %d", got)
	}

	// Out-of-order JSONL replay: a stale dayA event arrives. The
	// rollover handler must not freeze-and-clear today's set when a
	// past-day event lands; today's set is the freshest authoritative
	// state, not yesterday's.
	stale := mkEvent(1, collector.EventAgentStart, "a-stale", "s-stale", dayA)
	stale.Project = "marketplace"
	a.Fold(stale, 0)

	// Add another start in dayB after the stale event — its distinct
	// count must reflect the SAME 2 projects from before plus any
	// it adds, not start from 1 because of an erroneous freeze.
	ev := mkEvent(1, collector.EventAgentStart, "b3", "sb3", dayB.Add(10*time.Second))
	ev.Project = "coolant" // same as one already counted
	a.Fold(ev, 0)

	if got := a.Snapshot().Daily["2026-04-26"].DistinctProjectsDay; got != 2 {
		t.Errorf("dayB distinct count corrupted by past-day event: want 2, got %d", got)
	}
}

func TestMkEventVariadicAppliesOpt(t *testing.T) {
	// The variadic plumbing exists for telemetry's future
	// WithCost/WithTokens/WithToolCalls opts. Pin it now so a
	// regression that breaks the option-application loop fails
	// before telemetry lands.
	now := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
	ev := mkEvent(1, collector.EventAgentStart, "a1", "s1", now, func(e *collector.GateEvent) {
		e.AgentType = "Plan"
		e.Project = "thermal-enterprise"
	})
	if ev.AgentType != "Plan" {
		t.Errorf("variadic opt did not mutate AgentType: got %q", ev.AgentType)
	}
	if ev.Project != "thermal-enterprise" {
		t.Errorf("variadic opt did not mutate Project: got %q", ev.Project)
	}
}

func TestDayRolloverFreezesDistinctSets(t *testing.T) {
	a := newTestAggregator(t)
	dayA := time.Date(2026, 4, 25, 23, 0, 0, 0, time.UTC)
	dayB := time.Date(2026, 4, 26, 1, 0, 0, 0, time.UTC)

	for i, project := range []string{"coolant", "thermal-enterprise", "coolant"} {
		ev := mkEvent(1, collector.EventAgentStart, "a"+string(rune('0'+i)), "s"+string(rune('0'+i)), dayA.Add(time.Duration(i)*time.Second))
		ev.Project = project
		a.Fold(ev, 0)
	}
	// Cross day boundary.
	evB := mkEvent(1, collector.EventAgentStart, "b1", "sb", dayB)
	evB.Project = "marketplace"
	a.Fold(evB, 0)

	snap := a.Snapshot()
	// Day A frozen at 2 distinct projects.
	if got := snap.Daily["2026-04-25"].DistinctProjectsDay; got != 2 {
		t.Errorf("frozen day-A DistinctProjectsDay: want 2, got %d", got)
	}
	// Day B starts at 1.
	if got := snap.Daily["2026-04-26"].DistinctProjectsDay; got != 1 {
		t.Errorf("new day-B DistinctProjectsDay: want 1, got %d", got)
	}
	// In-memory map for day A cleared.
	a.mu.RLock()
	defer a.mu.RUnlock()
	if _, ok := a.dailyDistinctProjects["2026-04-25"]; ok {
		t.Errorf("day-A distinct projects map should be cleared after rollover")
	}
	if _, ok := a.dailyDistinctSessions["2026-04-25"]; ok {
		t.Errorf("day-A distinct sessions map should be cleared after rollover")
	}
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

	rec := a.Snapshot().Records.LongestSessionS[0]
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

	rec := a.Snapshot().Records.LongestSessionS[0]
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

	rec := a.Snapshot().Records.LongestSessionS[0]
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
	if got := a.Snapshot().Records.LongestAgentS.Top().Value; got != 0 {
		t.Errorf("negative agent duration leaked into record: got %d", got)
	}
}

func TestNegativeDurationClampsSessionEnd(t *testing.T) {
	a := newTestAggregator(t)
	now := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
	a.Fold(mkEvent(1, collector.EventSessionStart, "", "s1", now), 0)
	// session.end before session.start (NTP backstep).
	a.Fold(mkEvent(1, collector.EventSessionEnd, "", "s1", now.Add(-1*time.Hour)), 0)
	if got := a.Snapshot().Records.LongestSessionS.Top().Value; got != 0 {
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

	rec := a.Snapshot().Records.LongestSessionS[0]
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

	if got := a.Snapshot().Records.LongestSessionS.Top().Value; got != 0 {
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
	first := a.Snapshot().Records.LongestSessionS.Top().Value
	if first != 600 {
		t.Fatalf("staleness sweep precondition: want 600, got %d", first)
	}

	// Late session.end arrives — lifecycle math gets 10h, replaces
	// the swept value. No double-count, no regression.
	end := start.Add(10 * time.Hour)
	a.Fold(mkEvent(1, collector.EventSessionEnd, "", "late", end), 0)

	rec := a.Snapshot().Records.LongestSessionS[0]
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
	if life := a.Snapshot().Lifetime(); !life.IsZero() {
		t.Errorf("counter.underflow mutated counters: %+v", life)
	}
}

// ── per-agent telemetry counters ────────────────────────────

func TestFoldUpdatesTelemetryCounters(t *testing.T) {
	a := newTestAggregator(t)
	now := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
	a.Fold(mkEvent(1, collector.EventAgentStart, "a1", "s1", now), 0)
	a.Fold(mkEvent(1, collector.EventAgentStop, "a1", "s1", now.Add(time.Minute),
		WithTokens(1500, 200), WithToolCalls(7)), 0)

	bucket := a.Snapshot().Daily["2026-04-26"]
	if bucket.TokensInTotal != 1500 {
		t.Errorf("TokensInTotal: want 1500, got %d", bucket.TokensInTotal)
	}
	if bucket.TokensOutTotal != 200 {
		t.Errorf("TokensOutTotal: want 200, got %d", bucket.TokensOutTotal)
	}
	if bucket.ToolCallsTotal != 7 {
		t.Errorf("ToolCallsTotal: want 7, got %d", bucket.ToolCallsTotal)
	}
}

func TestFoldClampsNegativeTelemetry(t *testing.T) {
	a := newTestAggregator(t)
	now := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
	a.Fold(mkEvent(1, collector.EventAgentStart, "a1", "s1", now), 0)
	// Synthesize negative values — defensive clamp at fold time.
	a.Fold(mkEvent(1, collector.EventAgentStop, "a1", "s1", now.Add(time.Minute),
		WithTokens(-100, -50), WithToolCalls(-3)), 0)

	snap := a.Snapshot()
	bucket := snap.Daily["2026-04-26"]
	if bucket.TokensInTotal != 0 {
		t.Errorf("negative TokensIn leaked: got %d", bucket.TokensInTotal)
	}
	if bucket.TokensOutTotal != 0 {
		t.Errorf("negative TokensOut leaked: got %d", bucket.TokensOutTotal)
	}
	if bucket.ToolCallsTotal != 0 {
		t.Errorf("negative ToolCallCount leaked: got %d", bucket.ToolCallsTotal)
	}
	// Leaderboards must also stay empty — the clamp must apply BEFORE
	// the leaderboard insert, not just before the counter add.
	if got := len(snap.Records.MostTokensAgent); got != 0 {
		t.Errorf("negative tokens entered MostTokensAgent: %d entries", got)
	}
	if got := len(snap.Records.MostToolCallsAgent); got != 0 {
		t.Errorf("negative tool calls entered MostToolCallsAgent: %d entries", got)
	}
}

func TestFoldTelemetryAccumulatesAcrossAgents(t *testing.T) {
	a := newTestAggregator(t)
	now := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
	a.Fold(mkEvent(1, collector.EventAgentStart, "a1", "s1", now), 0)
	a.Fold(mkEvent(1, collector.EventAgentStart, "a2", "s1", now.Add(time.Second)), 0)
	a.Fold(mkEvent(1, collector.EventAgentStop, "a1", "s1", now.Add(time.Minute),
		WithTokens(1000, 100), WithToolCalls(5)), 0)
	a.Fold(mkEvent(1, collector.EventAgentStop, "a2", "s1", now.Add(2*time.Minute),
		WithTokens(2500, 300), WithToolCalls(12)), 0)

	bucket := a.Snapshot().Daily["2026-04-26"]
	if bucket.TokensInTotal != 3500 {
		t.Errorf("two-agent TokensInTotal: want 3500, got %d", bucket.TokensInTotal)
	}
	if bucket.ToolCallsTotal != 17 {
		t.Errorf("two-agent ToolCallsTotal: want 17, got %d", bucket.ToolCallsTotal)
	}
}

func TestLongestAgentRecordPicksUpEventFieldsWhenMetaSparse(t *testing.T) {
	// Prior bug: LongestAgentS read meta directly from a.agentMeta
	// without the event-field fallback that the new telemetry
	// records use. If agent.start landed with empty agent_type/project
	// (e.g. a degraded source) and agent.stop carried them, the record
	// dropped them silently. resolveAgentMeta unifies the path.
	a := newTestAggregator(t)
	now := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
	start := mkEvent(1, collector.EventAgentStart, "a1", "s1", now)
	start.AgentType = ""
	start.Project = ""
	a.Fold(start, 0)
	stop := mkEvent(1, collector.EventAgentStop, "a1", "s1", now.Add(60*time.Second))
	stop.AgentType = "Plan"
	stop.Project = "thermal-enterprise"
	a.Fold(stop, 0)

	rec := a.Snapshot().Records.LongestAgentS[0]
	if rec.AgentType != "Plan" {
		t.Errorf("LongestAgentS.AgentType: want Plan from event fallback, got %q", rec.AgentType)
	}
	if rec.Project != "thermal-enterprise" {
		t.Errorf("LongestAgentS.Project: want thermal-enterprise from event fallback, got %q", rec.Project)
	}
}

// ── Records.Top + BestDay helpers (cycle 3) ────────────────

func TestRecordListTopOnEmpty(t *testing.T) {
	var rl RecordList
	if got := rl.Top(); got != (RecordEntry{}) {
		t.Errorf("Top on empty list: want zero RecordEntry, got %+v", got)
	}
}

func TestRecordListTopReturnsHighest(t *testing.T) {
	var rl RecordList
	at := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
	rl = rl.Insert(RecordEntry{Value: 5, AgentID: "a1", At: at})
	rl = rl.Insert(RecordEntry{Value: 9, AgentID: "a2", At: at})
	rl = rl.Insert(RecordEntry{Value: 3, AgentID: "a3", At: at})
	if got := rl.Top().Value; got != 9 {
		t.Errorf("Top: want 9, got %d", got)
	}
}

func TestBestDayMethodsReturnEmptyOnNoData(t *testing.T) {
	a := newTestAggregator(t)
	snap := a.Snapshot()
	if d, v := snap.BestDayPeakConcurrent(); d != "" || v != 0 {
		t.Errorf("empty BestDayPeakConcurrent: want (\"\",0), got (%q,%d)", d, v)
	}
	if d, v := snap.BestDayDistinctProjects(); d != "" || v != 0 {
		t.Errorf("empty BestDayDistinctProjects: want (\"\",0), got (%q,%d)", d, v)
	}
	if d, v := snap.BestDayDistinctSessions(); d != "" || v != 0 {
		t.Errorf("empty BestDayDistinctSessions: want (\"\",0), got (%q,%d)", d, v)
	}
}

func TestBestDayPeakConcurrent(t *testing.T) {
	s := Snapshot{
		Daily: map[string]Counters{
			"2026-04-23": {PeakConcurrentDay: 3},
			"2026-04-24": {PeakConcurrentDay: 7}, // best
			"2026-04-25": {PeakConcurrentDay: 5},
		},
	}
	d, v := s.BestDayPeakConcurrent()
	if d != "2026-04-24" || v != 7 {
		t.Errorf("BestDayPeakConcurrent: want (\"2026-04-24\",7), got (%q,%d)", d, v)
	}
}

func TestBestDayDistinctProjects(t *testing.T) {
	s := Snapshot{
		Daily: map[string]Counters{
			"2026-04-25": {DistinctProjectsDay: 4}, // best
			"2026-04-24": {DistinctProjectsDay: 2},
		},
	}
	d, v := s.BestDayDistinctProjects()
	if d != "2026-04-25" || v != 4 {
		t.Errorf("BestDayDistinctProjects: want (\"2026-04-25\",4), got (%q,%d)", d, v)
	}
}

// ── leaderboard merge across checkpoints ────────────────────

func TestMaxMergeLeaderboard(t *testing.T) {
	at := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
	disk := Records{
		PeakConcurrent: RecordList{
			{Value: 10, AgentID: "a1", SessionID: "s1", At: at},
			{Value: 8, AgentID: "a2", SessionID: "s2", At: at},
			{Value: 6, AgentID: "a3", SessionID: "s3", At: at},
		},
	}
	cand := Records{
		PeakConcurrent: RecordList{
			// Same composite key as disk's a2 but higher value — wins.
			{Value: 12, AgentID: "a2", SessionID: "s2", At: at.Add(time.Hour)},
			{Value: 7, AgentID: "a4", SessionID: "s4", At: at},
		},
	}
	merged := maxMergeRecords(disk, cand).PeakConcurrent
	want := []int64{12, 10, 7, 6}
	if len(merged) != len(want) {
		t.Fatalf("merged length: want %d, got %d (%+v)", len(want), len(merged), merged)
	}
	for i, v := range want {
		if merged[i].Value != v {
			t.Errorf("merged[%d].Value: want %d, got %d", i, v, merged[i].Value)
		}
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

// ── windowed by_type / by_project ──────────────────────────

func TestWindowByTypeEmptyForFreshInstall(t *testing.T) {
	a := newTestAggregator(t)
	if got := a.WindowByType(7); len(got) != 0 {
		t.Errorf("fresh install WindowByType(7): want empty, got %v", got)
	}
	if got := a.WindowByProject(30); len(got) != 0 {
		t.Errorf("fresh install WindowByProject(30): want empty, got %v", got)
	}
	if got := a.WindowByType(0); len(got) != 0 {
		t.Errorf("WindowByType(0): want empty, got %v", got)
	}
	if got := a.WindowByType(-3); len(got) != 0 {
		t.Errorf("WindowByType(-3): want empty (silent clamp), got %v", got)
	}
}

func TestWindowByTypeSumsAcrossDays(t *testing.T) {
	a := newTestAggregator(t)
	now := time.Now().UTC().Truncate(24 * time.Hour)
	// day-3: two Explores, one Plan
	day3 := now.Add(-3 * 24 * time.Hour).Add(time.Hour)
	for i, kind := range []string{"Explore", "Explore", "Plan"} {
		ev := mkEvent(1, collector.EventAgentStart, "d3-"+string(rune('a'+i)), "s-d3", day3)
		ev.AgentType = kind
		ev.Project = "coolant"
		a.Fold(ev, 0)
	}
	// day-1: one Explore, one Doc
	day1 := now.Add(-1 * 24 * time.Hour).Add(time.Hour)
	for i, kind := range []string{"Explore", "Doc"} {
		ev := mkEvent(1, collector.EventAgentStart, "d1-"+string(rune('a'+i)), "s-d1", day1)
		ev.AgentType = kind
		ev.Project = "thermal-enterprise"
		a.Fold(ev, 0)
	}
	// day-31: stale Plan (outside 30d)
	day31 := now.Add(-31 * 24 * time.Hour).Add(time.Hour)
	stale := mkEvent(1, collector.EventAgentStart, "d31-a", "s-d31", day31)
	stale.AgentType = "Plan"
	a.Fold(stale, 0)

	w7 := a.WindowByType(7)
	if w7["Explore"] != 3 {
		t.Errorf("WindowByType(7)[Explore]: want 3, got %d", w7["Explore"])
	}
	if w7["Plan"] != 1 {
		t.Errorf("WindowByType(7)[Plan]: want 1 (day-3 only), got %d", w7["Plan"])
	}
	if w7["Doc"] != 1 {
		t.Errorf("WindowByType(7)[Doc]: want 1, got %d", w7["Doc"])
	}

	w30 := a.WindowByType(30)
	if w30["Plan"] != 1 {
		t.Errorf("WindowByType(30) excludes day-31: want Plan=1, got %d", w30["Plan"])
	}

	wp := a.WindowByProject(7)
	if wp["coolant"] != 3 {
		t.Errorf("WindowByProject(7)[coolant]: want 3, got %d", wp["coolant"])
	}
	if wp["thermal-enterprise"] != 2 {
		t.Errorf("WindowByProject(7)[thermal-enterprise]: want 2, got %d", wp["thermal-enterprise"])
	}
}

func TestWindowByTypeDeepCopy(t *testing.T) {
	a := newTestAggregator(t)
	now := time.Now().UTC()
	a.Fold(mkEvent(1, collector.EventAgentStart, "a1", "s1", now), 0)

	first := a.WindowByType(7)
	first["Explore"] = 999
	first["injected"] = 42

	second := a.WindowByType(7)
	if second["Explore"] != 1 {
		t.Errorf("returned map aliased to internal state: want Explore=1, got %d", second["Explore"])
	}
	if _, ok := second["injected"]; ok {
		t.Errorf("returned map aliased: 'injected' leaked into a second call")
	}
}

func TestWindowByTypeCardinalityCap(t *testing.T) {
	a := newTestAggregator(t)
	now := time.Now().UTC()
	// Fold 51 distinct agent_types in arrival order. The cap is
	// "first-50-distinct-seen-wins" (§0.4): types 0..49 own their
	// slots, type 50 routes to "__other".
	for i := 0; i < 51; i++ {
		ev := mkEvent(1, collector.EventAgentStart, "a"+string(rune('a'+i%26))+string(rune('a'+i/26)), "s1", now.Add(time.Duration(i)*time.Second))
		ev.AgentType = "type-" + string(rune('a'+i%26)) + string(rune('a'+i/26))
		a.Fold(ev, 0)
	}
	snap := a.Snapshot()
	// Lifetime ByType: 50 typed entries + "__other".
	if len(snap.ByType) != 51 {
		t.Errorf("lifetime ByType cap: want 51 entries (50 typed + __other), got %d", len(snap.ByType))
	}
	if snap.ByType["__other"] != 1 {
		t.Errorf("lifetime ByType[__other]: want 1, got %d", snap.ByType["__other"])
	}
	// Daily bucket map: same shape on today's bucket.
	today := dayKey(now)
	bucket := snap.Daily[today]
	if len(bucket.ByTypeDay) != 51 {
		t.Errorf("daily ByTypeDay cap: want 51 entries, got %d", len(bucket.ByTypeDay))
	}
	if bucket.ByTypeDay["__other"] != 1 {
		t.Errorf("daily ByTypeDay[__other]: want 1, got %d", bucket.ByTypeDay["__other"])
	}
}

func TestWindowByTypeCapKeepsExistingKeysIncrementing(t *testing.T) {
	// First-50-distinct-seen-wins: once at cap, EXISTING keys keep
	// incrementing freely; only NEW keys route to __other. A
	// high-cardinality early burst can permanently hide later
	// high-frequency types — intentional per §0.4.
	a := newTestAggregator(t)
	now := time.Now().UTC()
	// Fill the cap with 50 one-shot pioneers.
	for i := 0; i < 50; i++ {
		ev := mkEvent(1, collector.EventAgentStart, "p"+string(rune('a'+i%26))+string(rune('a'+i/26)), "s1", now.Add(time.Duration(i)*time.Second))
		ev.AgentType = "pioneer-" + string(rune('a'+i%26)) + string(rune('a'+i/26))
		a.Fold(ev, 0)
	}
	// Then 5 more fires of an existing pioneer — must hit its slot,
	// NOT __other.
	for i := 0; i < 5; i++ {
		ev := mkEvent(1, collector.EventAgentStart, "rep"+string(rune('a'+i)), "s1", now.Add(time.Hour+time.Duration(i)*time.Second))
		ev.AgentType = "pioneer-aa"
		a.Fold(ev, 0)
	}
	// Then 1 newcomer — must route to __other.
	newcomer := mkEvent(1, collector.EventAgentStart, "newcomer", "s1", now.Add(2*time.Hour))
	newcomer.AgentType = "Plan"
	a.Fold(newcomer, 0)

	snap := a.Snapshot()
	if got := snap.ByType["pioneer-aa"]; got != 6 {
		t.Errorf("existing pioneer slot increments freely past cap: want 6, got %d", got)
	}
	if got := snap.ByType["__other"]; got != 1 {
		t.Errorf("late newcomer routes to __other: want 1, got %d", got)
	}
	if _, present := snap.ByType["Plan"]; present {
		t.Errorf("late 'Plan' should NOT have its own slot — first-50-distinct-seen-wins")
	}
}

func TestLifetimeAndDailySumStayConsistent(t *testing.T) {
	// Drift guard, totals-only: sum(daily.ByTypeDay) ==
	// sum(snap.ByType) across a multi-day fold sequence (per §0.2).
	// Per-key equality is NOT asserted — same-fold cap activations
	// can transiently route the same key differently.
	a := newTestAggregator(t)
	now := time.Now().UTC()
	for d := 0; d < 5; d++ {
		base := now.Add(-time.Duration(d) * 24 * time.Hour)
		for i := 0; i < 7; i++ {
			ev := mkEvent(1, collector.EventAgentStart, "a"+string(rune('a'+d))+string(rune('a'+i)), "s"+string(rune('a'+d)), base.Add(time.Duration(i)*time.Minute))
			ev.AgentType = []string{"Explore", "Plan", "Doc"}[i%3]
			ev.Project = []string{"coolant", "thermal-enterprise"}[i%2]
			a.Fold(ev, 0)
		}
	}
	snap := a.Snapshot()
	var lifetimeSum, dailySum int64
	for _, v := range snap.ByType {
		lifetimeSum += v
	}
	for _, c := range snap.Daily {
		for _, v := range c.ByTypeDay {
			dailySum += v
		}
	}
	if lifetimeSum != dailySum {
		t.Errorf("by_type drift: lifetime sum=%d, daily sum=%d (want equal)", lifetimeSum, dailySum)
	}

	var lifetimePSum, dailyPSum int64
	for _, v := range snap.ByProject {
		lifetimePSum += v
	}
	for _, c := range snap.Daily {
		for _, v := range c.ByProjectDay {
			dailyPSum += v
		}
	}
	if lifetimePSum != dailyPSum {
		t.Errorf("by_project drift: lifetime sum=%d, daily sum=%d", lifetimePSum, dailyPSum)
	}
}

func TestDriftGuardFiresOncePerInstance(t *testing.T) {
	// Forge divergence by mutating a.byType after Fold has populated
	// both maps — Snapshot must detect the mismatch and bump degraded
	// EXACTLY ONCE per Aggregator instance, not once per Snapshot
	// call. A second Aggregator (fresh New) gets its own sync.Once
	// and bumps independently.
	a := newTestAggregator(t)
	now := time.Now().UTC()
	a.Fold(mkEvent(1, collector.EventAgentStart, "a1", "s1", now), 0)
	// Inject extra lifetime count without a matching daily increment.
	a.mu.Lock()
	a.byType["Explore"] += 99
	a.mu.Unlock()

	_ = a.Snapshot()
	_ = a.Snapshot()
	_ = a.Snapshot()
	deg, _ := os.ReadFile(a.cfg.DegradedPath)
	if got := bytesNewlineCount(deg); got != 1 {
		t.Errorf("once-per-instance: 3 Snapshot calls on same Aggregator with persistent drift, want 1 degraded bump, got %d", got)
	}

	// Second aggregator pointed at the same cache file — should not
	// inherit the first's sync.Once.
	b := New(Config{
		CachePath:    a.cfg.CachePath,
		JSONLPath:    a.cfg.JSONLPath,
		DegradedPath: a.cfg.DegradedPath,
	})
	b.Fold(mkEvent(1, collector.EventAgentStart, "b1", "s1", now), 0)
	b.mu.Lock()
	b.byType["Explore"] += 99
	b.mu.Unlock()
	_ = b.Snapshot()
	deg, _ = os.ReadFile(a.cfg.DegradedPath)
	if got := bytesNewlineCount(deg); got != 2 {
		t.Errorf("fresh aggregator gets its own once: want 2 total bumps, got %d", got)
	}
}

func bytesNewlineCount(b []byte) int {
	n := 0
	for _, c := range b {
		if c == '\n' {
			n++
		}
	}
	return n
}

func TestCountersIsZero(t *testing.T) {
	if !(Counters{}).IsZero() {
		t.Errorf("zero-value Counters: IsZero must be true")
	}
	if (Counters{AgentsStarted: 1}).IsZero() {
		t.Errorf("AgentsStarted=1: IsZero must be false")
	}
	if (Counters{ByTypeDay: map[string]int64{"k": 1}}).IsZero() {
		t.Errorf("ByTypeDay non-empty: IsZero must be false")
	}
	if (Counters{ByProjectDay: map[string]int64{"k": 1}}).IsZero() {
		t.Errorf("ByProjectDay non-empty: IsZero must be false")
	}
	// Empty (non-nil) maps still count as zero — they carry no data.
	if !(Counters{ByTypeDay: map[string]int64{}}).IsZero() {
		t.Errorf("empty ByTypeDay map: IsZero must be true")
	}
}
