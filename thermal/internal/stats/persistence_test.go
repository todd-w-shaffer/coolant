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
	if got.Records.PeakConcurrent.Value != want.Records.PeakConcurrent.Value {
		t.Errorf("PeakConcurrent: want %d, got %d", want.Records.PeakConcurrent.Value, got.Records.PeakConcurrent.Value)
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
			PeakConcurrent: RecordEntry{Value: 9, SessionID: "s1", At: at},
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
	if snap.Records.PeakConcurrent.Value != 9 {
		t.Errorf("Records preserved: want 9, got %d", snap.Records.PeakConcurrent.Value)
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
	if got := c.Snapshot().Records.PeakConcurrent.Value; got != 5 {
		t.Errorf("PeakConcurrent after max-merge: want 5, got %d", got)
	}
}
