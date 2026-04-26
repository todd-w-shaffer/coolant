package stats

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/toddwshaffer/coolant/thermal/internal/collector"
)

// ── checkpoint roundtrip ───────────────────────────────────

func TestCheckpointRoundtrip(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{
		CachePath:    filepath.Join(dir, "stats.json"),
		JSONLPath:    filepath.Join(dir, "events.jsonl"),
		DegradedPath: filepath.Join(dir, "degraded.count"),
	}

	a := New(cfg)
	now := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
	a.Fold(mkEvent(1, collector.EventAgentStart, "a1", "s1", now), 0)
	a.Fold(mkEvent(1, collector.EventAgentStart, "a2", "s1", now.Add(time.Second)), 0)
	a.Fold(mkEvent(1, collector.EventAgentStop, "a1", "s1", now.Add(time.Minute)), 0)

	want := a.Snapshot()
	if err := a.Checkpoint(); err != nil {
		t.Fatalf("Checkpoint: %v", err)
	}

	// Reload from disk via fresh aggregator.
	b := New(cfg)
	got := b.Snapshot()
	// LastUpdated/FirstSeen should match; ByType/ByProject/Daily/Records too.
	if !got.FirstSeen.Equal(want.FirstSeen) {
		t.Errorf("FirstSeen: want %v, got %v", want.FirstSeen, got.FirstSeen)
	}
	if !reflect.DeepEqual(got.ByType, want.ByType) {
		t.Errorf("ByType mismatch:\nwant: %v\ngot:  %v", want.ByType, got.ByType)
	}
	if !reflect.DeepEqual(got.ByProject, want.ByProject) {
		t.Errorf("ByProject mismatch:\nwant: %v\ngot:  %v", want.ByProject, got.ByProject)
	}
	if !reflect.DeepEqual(got.Daily, want.Daily) {
		t.Errorf("Daily mismatch:\nwant: %v\ngot:  %v", want.Daily, got.Daily)
	}
	if got.Records.PeakConcurrent.Top().Value != want.Records.PeakConcurrent.Top().Value {
		t.Errorf("PeakConcurrent: want %d, got %d", want.Records.PeakConcurrent.Top().Value, got.Records.PeakConcurrent.Top().Value)
	}
}

// ── cold start: missing cache ──────────────────────────────

func TestNewWithMissingCacheReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	a := New(Config{
		CachePath:    filepath.Join(dir, "does-not-exist.json"),
		JSONLPath:    filepath.Join(dir, "events.jsonl"),
		DegradedPath: filepath.Join(dir, "degraded.count"),
	})
	snap := a.Snapshot()
	if !snap.FirstSeen.IsZero() {
		t.Errorf("FirstSeen on empty: want zero, got %v", snap.FirstSeen)
	}
	if life := snap.Lifetime(); life.AgentsStarted != 0 {
		t.Errorf("AgentsStarted on empty: want 0, got %d", life.AgentsStarted)
	}
}

// ── mangled cache: no panic, no data ───────────────────────

func TestNewWithMangledCacheRecovers(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "stats.json")
	if err := os.WriteFile(cachePath, []byte("not json {{{"), 0o644); err != nil {
		t.Fatalf("write mangled cache: %v", err)
	}
	a := New(Config{
		CachePath:    cachePath,
		JSONLPath:    filepath.Join(dir, "events.jsonl"),
		DegradedPath: filepath.Join(dir, "degraded.count"),
	})
	if got := a.Snapshot().Lifetime().AgentsStarted; got != 0 {
		t.Errorf("mangled cache should yield empty state, got AgentsStarted=%d", got)
	}
}

// ── schema mismatch: permissive partial parse ──────────────

func TestSchemaMismatchPermissivePartialParse(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "stats.json")
	at := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)
	// Hand-craft a future-schema cache: only the fields we want preserved.
	future := map[string]any{
		"schema_version": 999,
		"first_seen":     at.Format(time.RFC3339),
		"by_type":        map[string]int64{"Explore": 42},
		"by_project":     map[string]int64{"coolant": 100},
		"daily": map[string]Counters{
			"2026-04-25": {AgentsStarted: 5, AgentsCompleted: 5},
		},
		"records": Records{
			PeakConcurrent: RecordList{{Value: 9, SessionID: "s1", At: at}},
		},
		// And some bogus future fields that should be discarded.
		"future_field":     "zorp",
		"future_structure": map[string]int{"a": 1},
	}
	buf, _ := json.Marshal(future)
	if err := os.WriteFile(cachePath, buf, 0o644); err != nil {
		t.Fatalf("write future cache: %v", err)
	}

	a := New(Config{
		CachePath:    cachePath,
		JSONLPath:    filepath.Join(dir, "events.jsonl"),
		DegradedPath: filepath.Join(dir, "degraded.count"),
	})
	snap := a.Snapshot()

	if snap.ByType["Explore"] != 42 {
		t.Errorf("ByType preserved across schema bump: want 42, got %d", snap.ByType["Explore"])
	}
	if snap.ByProject["coolant"] != 100 {
		t.Errorf("ByProject preserved: want 100, got %d", snap.ByProject["coolant"])
	}
	if snap.Daily["2026-04-25"].AgentsStarted != 5 {
		t.Errorf("Daily preserved: want 5 starts, got %d", snap.Daily["2026-04-25"].AgentsStarted)
	}
	if got := snap.Records.PeakConcurrent.Top().Value; got != 9 {
		t.Errorf("Records preserved: want 9, got %d", got)
	}
}

// ── concurrent checkpoints (single process, both deltas merged) ──

func TestConcurrentCheckpointsMergeDisjointDeltas(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{
		CachePath:    filepath.Join(dir, "stats.json"),
		JSONLPath:    filepath.Join(dir, "events.jsonl"),
		DegradedPath: filepath.Join(dir, "degraded.count"),
	}

	now := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)

	// Aggregator A folds 3 Explores.
	a := New(cfg)
	for i := 0; i < 3; i++ {
		a.Fold(mkEvent(1, collector.EventAgentStart, "a"+string(rune('0'+i)), "sa", now.Add(time.Duration(i)*time.Second)), 0)
	}

	// Aggregator B folds 5 Plans (fresh load → sees empty cache, then folds).
	b := New(cfg)
	for i := 0; i < 5; i++ {
		ev := mkEvent(1, collector.EventAgentStart, "b"+string(rune('0'+i)), "sb", now.Add(time.Duration(10+i)*time.Second))
		ev.AgentType = "Plan"
		b.Fold(ev, 0)
	}

	// Race-y interleave: spawn both checkpoints in parallel.
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); _ = a.Checkpoint() }()
	go func() { defer wg.Done(); _ = b.Checkpoint() }()
	wg.Wait()

	// On disk: by_type[Explore]=3, by_type[Plan]=5; total starts (by sum daily) = 8.
	c := New(cfg)
	snap := c.Snapshot()
	if snap.ByType["Explore"] != 3 {
		t.Errorf("disk ByType[Explore] after concurrent checkpoint: want 3, got %d", snap.ByType["Explore"])
	}
	if snap.ByType["Plan"] != 5 {
		t.Errorf("disk ByType[Plan]: want 5, got %d", snap.ByType["Plan"])
	}
	if got := snap.Lifetime().AgentsStarted; got != 8 {
		t.Errorf("disk Lifetime.AgentsStarted: want 8, got %d", got)
	}
}

// ── directory creation ────────────────────────────────────

func TestCheckpointCreatesParentDir(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "nested", "stats.json")
	a := New(Config{
		CachePath:    cachePath,
		JSONLPath:    filepath.Join(dir, "events.jsonl"),
		DegradedPath: filepath.Join(dir, "degraded.count"),
	})
	now := time.Now().UTC()
	a.Fold(mkEvent(1, collector.EventAgentStart, "a", "s", now), 0)
	if err := a.Checkpoint(); err != nil {
		t.Fatalf("Checkpoint should create parent dir lazily: %v", err)
	}
	info, err := os.Stat(filepath.Dir(cachePath))
	if err != nil {
		t.Fatalf("parent dir not created: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Errorf("parent dir mode: want 0700, got %o", perm)
	}
}

// ── stale-agent prune ─────────────────────────────────────

func TestCheckpointPrunesStaleAgents(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{
		CachePath:    filepath.Join(dir, "stats.json"),
		JSONLPath:    filepath.Join(dir, "events.jsonl"),
		DegradedPath: filepath.Join(dir, "degraded.count"),
	}
	a := New(cfg)

	now := time.Now().UTC()
	// Stale agent: started 25h ago, never stopped (crash / kill -9 case).
	stale := mkEvent(1, "agent.start", "stale-1", "s1", now.Add(-25*time.Hour))
	a.Fold(stale, 0)
	// Fresh agent: still active.
	a.Fold(mkEvent(1, "agent.start", "fresh-1", "s1", now.Add(-1*time.Hour)), 0)

	// Pre-checkpoint: both entries present.
	if _, ok := a.agentStarts["stale-1"]; !ok {
		t.Fatalf("precondition: stale-1 should be tracked")
	}
	if _, ok := a.agentStarts["fresh-1"]; !ok {
		t.Fatalf("precondition: fresh-1 should be tracked")
	}

	if err := a.Checkpoint(); err != nil {
		t.Fatalf("Checkpoint: %v", err)
	}

	// Post-checkpoint: stale dropped, fresh kept.
	if _, ok := a.agentStarts["stale-1"]; ok {
		t.Errorf("stale-1 should have been pruned (>24h old)")
	}
	if _, ok := a.agentStarts["fresh-1"]; !ok {
		t.Errorf("fresh-1 should still be tracked (<24h old)")
	}
	// agentMeta tracks alongside; verify both deleted together.
	if _, ok := a.agentMeta["stale-1"]; ok {
		t.Errorf("agentMeta[stale-1] should have been pruned alongside agentStarts")
	}
}

// ── today distinct sets persistence ───────────────────────

func TestTodayDistinctSetsRoundtripCache(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{
		CachePath:    filepath.Join(dir, "stats.json"),
		JSONLPath:    filepath.Join(dir, "events.jsonl"),
		DegradedPath: filepath.Join(dir, "degraded.count"),
	}
	a := New(cfg)
	now := time.Now().UTC()
	for _, project := range []string{"coolant", "thermal-enterprise", "marketplace"} {
		ev := mkEvent(1, collector.EventAgentStart, "a-"+project, "s-"+project, now)
		ev.Project = project
		a.Fold(ev, 0)
	}
	if err := a.Checkpoint(); err != nil {
		t.Fatalf("Checkpoint: %v", err)
	}
	b := New(cfg)
	todayKey := dayKey(time.Now())
	b.mu.RLock()
	defer b.mu.RUnlock()
	projects := b.dailyDistinctProjects[todayKey]
	if len(projects) != 3 {
		t.Errorf("after restore: want 3 distinct projects, got %d", len(projects))
	}
	for _, p := range []string{"coolant", "thermal-enterprise", "marketplace"} {
		if _, ok := projects[p]; !ok {
			t.Errorf("after restore: project %q missing from set", p)
		}
	}
}

func TestTodayDistinctSetsDiscardedOnDateChange(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{
		CachePath:    filepath.Join(dir, "stats.json"),
		JSONLPath:    filepath.Join(dir, "events.jsonl"),
		DegradedPath: filepath.Join(dir, "degraded.count"),
	}
	// Hand-craft a cache with a today_distinct block dated yesterday.
	yesterday := dayKey(time.Now().Add(-24 * time.Hour))
	persisted := Snapshot{
		SchemaVersion: CurrentSchemaVersion,
		Daily: map[string]Counters{
			yesterday: {AgentsStarted: 7, DistinctProjectsDay: 2},
		},
		TodayDistinct: TodayDistinctSets{
			Date:     yesterday,
			Projects: []string{"coolant", "thermal-enterprise"},
		},
	}
	buf, _ := json.Marshal(persisted)
	if err := os.WriteFile(cfg.CachePath, buf, 0o644); err != nil {
		t.Fatalf("seed cache: %v", err)
	}
	a := New(cfg)
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.currentDayKey != "" {
		t.Errorf("stale today block should not set currentDayKey, got %q", a.currentDayKey)
	}
	if got := len(a.dailyDistinctProjects); got != 0 {
		t.Errorf("stale today projects should be discarded, got %d entries", got)
	}
	// Yesterday's bucket count survives.
	if got := a.daily[yesterday].DistinctProjectsDay; got != 2 {
		t.Errorf("frozen yesterday bucket count should survive: want 2, got %d", got)
	}
}

// ── v1 → v2 cache migration (cycle 3) ─────────────────────

func TestV1CacheLoadsAsSingleElementSlices(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "stats.json")
	at := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)
	// Hand-craft a v1 cache: each record is a single-object shape, not
	// an array. Custom UnmarshalJSON on RecordList/BurstRecordList
	// should accept this and wrap as a 1-elem slice so historical
	// records survive the migration.
	v1 := map[string]any{
		"schema_version": 1,
		"first_seen":     at.Format(time.RFC3339),
		"daily": map[string]Counters{
			"2026-04-01": {AgentsStarted: 5, AgentsCompleted: 5},
		},
		"records": map[string]any{
			"peak_concurrent":     map[string]any{"value": 9, "session_id": "s1", "at": at.Format(time.RFC3339)},
			"longest_agent_s":     map[string]any{"value": 312, "agent_id": "a1", "at": at.Format(time.RFC3339)},
			"longest_session_s":   map[string]any{"value": 8943, "session_id": "s2", "at": at.Format(time.RFC3339)},
			"most_agents_session": map[string]any{"value": 32, "session_id": "s3", "at": at.Format(time.RFC3339)},
			"biggest_burst":       map[string]any{"count": 6, "window_s": 2, "session_id": "s4", "at": at.Format(time.RFC3339)},
		},
	}
	buf, _ := json.Marshal(v1)
	if err := os.WriteFile(cachePath, buf, 0o644); err != nil {
		t.Fatalf("seed v1 cache: %v", err)
	}
	a := New(Config{
		CachePath:    cachePath,
		JSONLPath:    filepath.Join(dir, "events.jsonl"),
		DegradedPath: filepath.Join(dir, "degraded.count"),
	})
	snap := a.Snapshot()
	checks := []struct {
		name string
		got  []RecordEntry
		want int64
	}{
		{"PeakConcurrent", snap.Records.PeakConcurrent, 9},
		{"LongestAgentS", snap.Records.LongestAgentS, 312},
		{"LongestSessionS", snap.Records.LongestSessionS, 8943},
		{"MostAgentsSession", snap.Records.MostAgentsSession, 32},
	}
	for _, c := range checks {
		if len(c.got) != 1 {
			t.Errorf("%s: want 1-elem slice from v1 cache, got %d", c.name, len(c.got))
			continue
		}
		if c.got[0].Value != c.want {
			t.Errorf("%s: want value %d, got %d", c.name, c.want, c.got[0].Value)
		}
	}
	if len(snap.Records.BiggestBurst) != 1 || snap.Records.BiggestBurst[0].Count != 6 {
		t.Errorf("BiggestBurst v1 migration: %+v", snap.Records.BiggestBurst)
	}
}

func TestCheckpointRefusesSchemaDowngrade(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{
		CachePath:    filepath.Join(dir, "stats.json"),
		JSONLPath:    filepath.Join(dir, "events.jsonl"),
		DegradedPath: filepath.Join(dir, "degraded.count"),
	}
	// Write a v3 cache to disk by hand (simulating a future binary's
	// output). The current build is v2 — checkpoint must abort the
	// write rather than clobber the future data.
	at := time.Now().UTC()
	future := Snapshot{
		SchemaVersion: CurrentSchemaVersion + 1,
		FirstSeen:     at,
		LastUpdated:   at,
		Daily: map[string]Counters{
			"2026-04-25": {AgentsStarted: 99},
		},
	}
	buf, _ := json.Marshal(future)
	if err := os.WriteFile(cfg.CachePath, buf, 0o644); err != nil {
		t.Fatalf("seed future cache: %v", err)
	}
	originalBytes, _ := os.ReadFile(cfg.CachePath)

	a := New(cfg)
	a.Fold(mkEvent(1, collector.EventAgentStart, "a1", "s1", at), 0)
	if err := a.Checkpoint(); err != nil {
		t.Fatalf("Checkpoint should not error on schema downgrade refusal: %v", err)
	}
	// Disk file must be unchanged.
	gotBytes, _ := os.ReadFile(cfg.CachePath)
	if string(gotBytes) != string(originalBytes) {
		t.Errorf("schema downgrade refusal failed — disk file was overwritten")
	}
	// Degraded counter must have bumped.
	deg, _ := os.ReadFile(cfg.DegradedPath)
	if len(deg) == 0 {
		t.Errorf("schema downgrade refusal should bump degraded counter; file empty")
	}
}

func TestLoadCacheLogsRecordsParseError(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{
		CachePath:    filepath.Join(dir, "stats.json"),
		JSONLPath:    filepath.Join(dir, "events.jsonl"),
		DegradedPath: filepath.Join(dir, "degraded.count"),
	}
	// Cache with a corrupt records block — schema_version mismatched
	// to force the permissive path AND the records value is structurally
	// invalid so the per-field unmarshal errors out. Without the
	// error-visibility hook, this used to silently zero records;
	// the new code must surface it via the degraded counter.
	cache := map[string]any{
		"schema_version": 999,
		"records":        "not an object",
	}
	buf, _ := json.Marshal(cache)
	if err := os.WriteFile(cfg.CachePath, buf, 0o644); err != nil {
		t.Fatalf("seed cache: %v", err)
	}
	_ = New(cfg)
	deg, _ := os.ReadFile(cfg.DegradedPath)
	if len(deg) == 0 {
		t.Errorf("records parse error should bump degraded counter; file empty")
	}
}

// ── max-merge records ─────────────────────────────────────

func TestRecordsMaxMergedAcrossCheckpoints(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{
		CachePath:    filepath.Join(dir, "stats.json"),
		JSONLPath:    filepath.Join(dir, "events.jsonl"),
		DegradedPath: filepath.Join(dir, "degraded.count"),
	}

	now := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)

	// First aggregator: peak 5.
	a := New(cfg)
	for i := 0; i < 5; i++ {
		a.Fold(mkEvent(1, collector.EventAgentStart, "a"+string(rune('0'+i)), "s1", now.Add(time.Duration(i)*time.Second)), 0)
	}
	if err := a.Checkpoint(); err != nil {
		t.Fatalf("Checkpoint a: %v", err)
	}

	// Second aggregator: peak 3 (smaller). Records.peak should remain 5.
	b := New(cfg)
	for i := 0; i < 3; i++ {
		b.Fold(mkEvent(1, collector.EventAgentStart, "b"+string(rune('0'+i)), "s2", now.Add(time.Hour+time.Duration(i)*time.Second)), 0)
	}
	if err := b.Checkpoint(); err != nil {
		t.Fatalf("Checkpoint b: %v", err)
	}

	c := New(cfg)
	if got := c.Snapshot().Records.PeakConcurrent.Top().Value; got != 5 {
		t.Errorf("PeakConcurrent after max-merge: want 5, got %d", got)
	}
}
