package collector

import (
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTokenCollectorDedupesMultiBlockResponse(t *testing.T) {
	// Critical invariant: Claude Code emits one transcript row per content
	// block of a single response, all sharing the same message.id and
	// repeating the SAME usage object. Naive summing would double-count
	// every multi-block response.
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")

	tc := NewTokenCollector()
	tc.scanFile(path) // would no-op even if file existed (cold-start)

	// Pre-seed the offset so the next scan actually reads.
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	tc.scanFile(path)

	// Two rows sharing msg_1 (thinking + tool_use blocks of one response).
	// One row for msg_2 (next response).
	body := `{"type":"assistant","message":{"id":"msg_1","usage":{"input_tokens":100,"output_tokens":50,"cache_creation_input_tokens":0,"cache_read_input_tokens":200}}}` + "\n" +
		`{"type":"assistant","message":{"id":"msg_1","usage":{"input_tokens":100,"output_tokens":50,"cache_creation_input_tokens":0,"cache_read_input_tokens":200}}}` + "\n" +
		`{"type":"assistant","message":{"id":"msg_2","usage":{"input_tokens":10,"output_tokens":5,"cache_creation_input_tokens":50,"cache_read_input_tokens":100}}}` + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	tc.scanFile(path)

	if tc.acc.Stats.InputTotal != 110 {
		t.Errorf("InputTotal = %d, want 110 (duplicate msg_1 row must not double-count)", tc.acc.Stats.InputTotal)
	}
	if tc.acc.Stats.OutputTotal != 55 {
		t.Errorf("OutputTotal = %d, want 55", tc.acc.Stats.OutputTotal)
	}
	if tc.acc.Stats.CacheCreateTotal != 50 {
		t.Errorf("CacheCreateTotal = %d, want 50", tc.acc.Stats.CacheCreateTotal)
	}
	if tc.acc.Stats.CacheReadTotal != 300 {
		t.Errorf("CacheReadTotal = %d, want 300", tc.acc.Stats.CacheReadTotal)
	}
}

func TestCacheWindowEmpty(t *testing.T) {
	var w cacheWindow
	if got := w.ratio(); got != 0 {
		t.Errorf("empty window ratio = %v, want 0", got)
	}
}

func TestCacheWindowSingleDelta(t *testing.T) {
	var w cacheWindow
	// 700 cache_read out of (100 input + 200 cache_create + 700 cache_read) = 70%
	w.add(cacheDelta{read: 700, total: 100 + 200 + 700})
	if got := w.ratio(); math.Abs(got-0.7) > 1e-9 {
		t.Errorf("ratio = %v, want 0.7", got)
	}
}

func TestCacheWindowAveragesAcrossTicks(t *testing.T) {
	var w cacheWindow
	w.add(cacheDelta{read: 0, total: 100})    // tick 1: 0% hit
	w.add(cacheDelta{read: 100, total: 100}) // tick 2: 100% hit
	// Combined: 100 read / 200 total = 50%
	if got := w.ratio(); math.Abs(got-0.5) > 1e-9 {
		t.Errorf("ratio = %v, want 0.5", got)
	}
}

func TestCacheWindowEvictsAfterCapacity(t *testing.T) {
	// Fill the window with 100%-hit deltas, then push one 0%-hit delta past
	// the capacity and verify the very-first 100% delta is gone (otherwise
	// the window would still read 100% from sticky history).
	var w cacheWindow
	for i := 0; i < cacheWindowSize; i++ {
		w.add(cacheDelta{read: 100, total: 100})
	}
	if got := w.ratio(); got != 1.0 {
		t.Fatalf("full-100%% window ratio = %v, want 1.0", got)
	}
	// Replace the oldest entry with a 0%-hit delta.
	w.add(cacheDelta{read: 0, total: 100})
	// New sum: (cacheWindowSize-1)*100 read / cacheWindowSize*100 total
	want := float64(cacheWindowSize-1) / float64(cacheWindowSize)
	if got := w.ratio(); math.Abs(got-want) > 1e-9 {
		t.Errorf("ratio after eviction = %v, want %v", got, want)
	}
}

func TestCacheWindowIgnoresZeroTotalTicks(t *testing.T) {
	// Idle ticks (no token activity) shouldn't poison the ratio.
	var w cacheWindow
	w.add(cacheDelta{read: 90, total: 100}) // 90%
	for i := 0; i < 5; i++ {
		w.add(cacheDelta{}) // idle ticks
	}
	if got := w.ratio(); math.Abs(got-0.9) > 1e-9 {
		t.Errorf("ratio with idle ticks = %v, want 0.9", got)
	}
}

func TestParseTranscriptLineExtractsAssistantUsage(t *testing.T) {
	line := []byte(`{"type":"assistant","message":{"id":"msg_abc","usage":{"input_tokens":6,"output_tokens":190,"cache_creation_input_tokens":18771,"cache_read_input_tokens":17735}}}`)
	id, usage, ok := parseTranscriptLine(line)
	if !ok {
		t.Fatal("expected ok=true for valid assistant row")
	}
	if id != "msg_abc" {
		t.Errorf("id = %q, want msg_abc", id)
	}
	if usage.Input != 6 || usage.Output != 190 || usage.CacheCreate != 18771 || usage.CacheRead != 17735 {
		t.Errorf("usage = %+v", usage)
	}
}

func TestParseTranscriptLineSkipsNonAssistantRows(t *testing.T) {
	cases := []string{
		`{"type":"user","message":{"role":"user","content":"hi"}}`,
		`{"type":"summary"}`,
		`{"type":"assistant","message":{"id":"msg_x"}}`, // no usage field
		`not even json`,
		``,
	}
	for _, c := range cases {
		if _, _, ok := parseTranscriptLine([]byte(c)); ok {
			t.Errorf("parseTranscriptLine(%q) returned ok=true, want false", c)
		}
	}
}

func TestTokenCollectorTickProducesWindowedCacheRatio(t *testing.T) {
	// Drive a couple of ticks against a real session file and verify the
	// reported CacheHitRatio reflects the delta-window, not the lifetime
	// total. With cold-start, tick #1 seeds the offset; tick #2 sees the
	// freshly appended message and that single delta defines the ratio.
	dir := t.TempDir()
	tc := NewTokenCollector()
	tc.projects = dir
	// Put the file inside a project subdir so the glob picks it up.
	projDir := filepath.Join(dir, "demo-project")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(projDir, "session.jsonl")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	t0 := time.Now()
	tc.Tick(t0) // cold-start: seeds offset to 0

	// One message: cache_read 800, input 200, cache_create 0 → 80% windowed
	body := `{"type":"assistant","message":{"id":"m1","usage":{"input_tokens":200,"output_tokens":50,"cache_creation_input_tokens":0,"cache_read_input_tokens":800}}}` + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	stats := tc.Tick(t0.Add(time.Second))
	if got := stats.CacheHitRatio; math.Abs(got-0.8) > 1e-9 {
		t.Errorf("windowed CacheHitRatio = %v, want 0.8", got)
	}
}

// seedTranscriptProject creates a project dir + empty session.jsonl and
// returns (tc-projects-root, session-file-path). Caller writes JSONL to
// session-file-path to drive transcript activity.
func seedTranscriptProject(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	projDir := filepath.Join(dir, "demo-project")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(projDir, "session.jsonl")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	return dir, path
}

// IOTokensPerSec must reflect input+output only — a tick whose ONLY
// change is a huge cache_read delta must report zero IO rate. Guards
// against regressions that would re-introduce cache-read-driven
// 200k/s spikes on the sparkline.
func TestTokenCollector_IORateExcludesCacheRead(t *testing.T) {
	dir, path := seedTranscriptProject(t)
	tc := NewTokenCollector()
	tc.projects = dir

	t0 := time.Now()
	// Tick 1: one assistant message with some input/output and zero cache.
	body1 := `{"type":"assistant","message":{"id":"m1","usage":{"input_tokens":100,"output_tokens":200,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}` + "\n"
	if err := os.WriteFile(path, []byte(body1), 0o644); err != nil {
		t.Fatal(err)
	}
	tc.Tick(t0)

	// Tick 2 (one second later): a second message whose ONLY delta is
	// a giant cache_read. Input/output unchanged from tick 1.
	body2 := body1 + `{"type":"assistant","message":{"id":"m2","usage":{"input_tokens":0,"output_tokens":0,"cache_creation_input_tokens":0,"cache_read_input_tokens":250000}}}` + "\n"
	if err := os.WriteFile(path, []byte(body2), 0o644); err != nil {
		t.Fatal(err)
	}
	stats := tc.Tick(t0.Add(time.Second))

	if stats.CacheReadTotal != 250000 {
		t.Fatalf("test precondition: CacheReadTotal want 250000, got %d", stats.CacheReadTotal)
	}
	if stats.IOTokensPerSec != 0 {
		t.Errorf("cache_read delta leaked into IOTokensPerSec: %v (want 0)", stats.IOTokensPerSec)
	}
}

func TestTokenCollectorColdStartSkipsHistory(t *testing.T) {
	// A pre-existing session file should not have its accumulated history
	// counted as "just happened" on first scan. Offsets must seed to file size
	// at discovery, not 0.
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	historic := `{"type":"assistant","message":{"id":"msg_old","usage":{"input_tokens":1000,"output_tokens":1000,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}` + "\n"
	if err := os.WriteFile(path, []byte(historic), 0o644); err != nil {
		t.Fatal(err)
	}

	tc := NewTokenCollector()
	tc.scanFile(path) // first scan should only seed offset

	if tc.acc.Stats.InputTotal != 0 {
		t.Errorf("cold-start replayed history: InputTotal = %d, want 0", tc.acc.Stats.InputTotal)
	}

	// Append a new message; only this one should count.
	fresh := `{"type":"assistant","message":{"id":"msg_new","usage":{"input_tokens":7,"output_tokens":3,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}` + "\n"
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(fresh); err != nil {
		t.Fatal(err)
	}
	f.Close()

	tc.scanFile(path)

	if tc.acc.Stats.InputTotal != 7 {
		t.Errorf("InputTotal after append = %d, want 7 (history must remain skipped)", tc.acc.Stats.InputTotal)
	}
	if tc.acc.Stats.OutputTotal != 3 {
		t.Errorf("OutputTotal after append = %d, want 3", tc.acc.Stats.OutputTotal)
	}
}
