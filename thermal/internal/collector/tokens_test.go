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
	w.add(cacheDelta{read: 0, total: 100})   // tick 1: 0% hit
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

// fakeOTELView is a controllable OTELTokenView for the fan-in tests —
// avoids booting the OTEL receiver / tailer for collector-level tests.
//
// OTELTokens is now-aware: when okDays is non-nil it gates ok on the
// query's UTC date (mirroring the real adapter, whose per-day cumulative
// bucket rolls empty at 00:00 UTC). When okDays is nil it falls back to
// the static ok flag, so the pre-existing fan-in tests are unaffected.
type fakeOTELView struct {
	input, output, cacheCreate, cacheRead int64
	ok                                    bool
	live                                  bool
	okDays                                map[string]bool // UTC "2006-01-02" → bucket has data
	driftCalls                            []string
}

func (f *fakeOTELView) OTELTokens(now time.Time) (int64, int64, int64, int64, bool) {
	ok := f.ok
	if f.okDays != nil {
		ok = f.okDays[now.UTC().Format("2006-01-02")]
	}
	return f.input, f.output, f.cacheCreate, f.cacheRead, ok
}
func (f *fakeOTELView) IsOTELLive(time.Time) bool { return f.live }
func (f *fakeOTELView) ObserveTokenSchemaDrift(field, _ string) {
	f.driftCalls = append(f.driftCalls, field)
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

// LOAD TEST (research Q3): both transcript + OTEL active and reporting
// the SAME tokens for a normal foreground turn. The naive merge would
// 2x-inflate every input/output token. The either-or rule must produce
// OTEL numbers only.
func TestTokenCollector_FanIn_OTELLiveSuppressesTranscriptDouble(t *testing.T) {
	dir, path := seedTranscriptProject(t)

	view := &fakeOTELView{
		live: true, ok: true,
		input: 1000, output: 500, cacheCreate: 100, cacheRead: 200,
	}
	tc := NewTokenCollector(WithOTELView(view))
	tc.projects = dir

	t0 := time.Now()
	tc.Tick(t0) // cold start seeds transcript offset

	// Both sources now report the same usage (the normal foreground case
	// the research called out — naive merge would emit 2x).
	body := `{"type":"assistant","message":{"id":"m1","usage":{"input_tokens":1000,"output_tokens":500,"cache_creation_input_tokens":100,"cache_read_input_tokens":200}}}` + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	stats := tc.Tick(t0.Add(time.Second))

	if stats.InputTotal != 1000 {
		t.Errorf("InputTotal want 1000 (OTEL-only, NOT 2000 naive-sum), got %d", stats.InputTotal)
	}
	if stats.OutputTotal != 500 {
		t.Errorf("OutputTotal want 500, got %d", stats.OutputTotal)
	}
	if stats.CacheCreateTotal != 100 {
		t.Errorf("CacheCreateTotal want 100, got %d", stats.CacheCreateTotal)
	}
	if stats.CacheReadTotal != 200 {
		t.Errorf("CacheReadTotal want 200, got %d", stats.CacheReadTotal)
	}
}

func TestTokenCollector_FanIn_OTELOnlyWhenNoTranscript(t *testing.T) {
	view := &fakeOTELView{
		live: true, ok: true,
		input: 700, output: 200, cacheCreate: 50, cacheRead: 100,
	}
	tc := NewTokenCollector(WithOTELView(view))
	tc.projects = "" // no transcript

	t0 := time.Now()
	tc.Tick(t0)
	stats := tc.Tick(t0.Add(time.Second))

	if stats.InputTotal != 700 || stats.OutputTotal != 200 ||
		stats.CacheCreateTotal != 50 || stats.CacheReadTotal != 100 {
		t.Errorf("OTEL-only totals mismatch: %+v", stats)
	}
}

func TestTokenCollector_FanIn_TranscriptOnlyWhenNoView(t *testing.T) {
	dir, path := seedTranscriptProject(t)

	tc := NewTokenCollector() // no view
	tc.projects = dir
	t0 := time.Now()
	tc.Tick(t0)

	body := `{"type":"assistant","message":{"id":"m1","usage":{"input_tokens":42,"output_tokens":7,"cache_creation_input_tokens":3,"cache_read_input_tokens":11}}}` + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	stats := tc.Tick(t0.Add(time.Second))

	if stats.InputTotal != 42 || stats.OutputTotal != 7 ||
		stats.CacheCreateTotal != 3 || stats.CacheReadTotal != 11 {
		t.Errorf("transcript-only totals mismatch: %+v", stats)
	}
}

// Source flip from OTEL → transcript must surface transcript numbers
// without sampling a giant negative delta into the rate. The OTEL totals
// here are deliberately much larger than transcript so a naive
// "input+output - last" sample would go strongly negative.
func TestTokenCollector_FanIn_OTELOfflineRevertsWithoutRateSpike(t *testing.T) {
	dir, path := seedTranscriptProject(t)
	view := &fakeOTELView{
		live: true, ok: true,
		input: 5000, output: 1000,
	}
	tc := NewTokenCollector(WithOTELView(view))
	tc.projects = dir

	t0 := time.Now()
	tc.Tick(t0) // cold start

	body := `{"type":"assistant","message":{"id":"m1","usage":{"input_tokens":100,"output_tokens":50,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}` + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	live := tc.Tick(t0.Add(time.Second))
	if live.InputTotal != 5000 {
		t.Fatalf("OTEL-live first tick: InputTotal want 5000, got %d", live.InputTotal)
	}

	// OTEL goes silent past the debounce window.
	view.live = false
	off := tc.Tick(t0.Add(2 * time.Second))

	if off.InputTotal != 100 {
		t.Errorf("after OTEL offline: InputTotal want 100 (transcript), got %d", off.InputTotal)
	}
	if off.IOTokensPerSec < 0 {
		t.Errorf("source flip yielded negative rate: %v", off.IOTokensPerSec)
	}
}

// Schema-drift gate: receiver live, lookup returns ok=false AFTER we'd
// previously seen ok=true (i.e. OTEL was producing data and stopped).
// That's the "attribute got renamed" signature — fire the drift call
// and fall back to transcript for this tick.
func TestTokenCollector_FanIn_SchemaDriftFiresAfterPreviouslySeenData(t *testing.T) {
	dir, path := seedTranscriptProject(t)
	view := &fakeOTELView{live: true, ok: true, input: 500}
	tc := NewTokenCollector(WithOTELView(view))
	tc.projects = dir

	t0 := time.Now()
	tc.Tick(t0)

	// Tick with OTEL producing.
	tc.Tick(t0.Add(time.Second))

	// Now OTEL still live but lookup misses (simulating an attribute rename).
	view.ok = false
	body := `{"type":"assistant","message":{"id":"m1","usage":{"input_tokens":42,"output_tokens":7,"cache_creation_input_tokens":3,"cache_read_input_tokens":11}}}` + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	stats := tc.Tick(t0.Add(2 * time.Second))

	if stats.InputTotal != 42 {
		t.Errorf("drift fallback: InputTotal want 42 (transcript), got %d", stats.InputTotal)
	}
	if len(view.driftCalls) == 0 {
		t.Errorf("expected ObserveTokenSchemaDrift call, got none")
	}
}

// Cold start with OTEL never having produced data must NOT fire a
// false-positive drift call. Receiver might be live but the first
// export simply hasn't landed yet.
func TestTokenCollector_FanIn_NoDriftCallOnColdStartLookupMiss(t *testing.T) {
	view := &fakeOTELView{live: true, ok: false}
	tc := NewTokenCollector(WithOTELView(view))
	tc.projects = ""

	t0 := time.Now()
	tc.Tick(t0)
	tc.Tick(t0.Add(time.Second))

	if len(view.driftCalls) != 0 {
		t.Errorf("cold-start lookup miss must not fire drift: got %v", view.driftCalls)
	}
}

// BUG 1: at 00:00 UTC the adapter's per-day cumulative bucket rolls empty, so
// OTELTokens returns ok=false. The collector must treat that as a gap (no-data
// for the new day) — a baseline reset, NOT a schema-drift event. The original
// code fired token_lookup_miss and burned the one-shot driftFired, so a genuine
// later attribute rename could never fire for the rest of the process.
func TestTokenCollector_FanIn_DateRollDoesNotBurnDriftGate(t *testing.T) {
	dir, _ := seedTranscriptProject(t)
	view := &fakeOTELView{
		live:  true,
		input: 500,
		okDays: map[string]bool{
			"2026-06-01": true,  // day 1 bucket has data
			"2026-06-02": false, // day 2 bucket empty (the midnight roll)
		},
	}
	tc := NewTokenCollector(WithOTELView(view))
	tc.projects = dir

	// Day 1, OTEL producing.
	tc.Tick(time.Date(2026, 6, 1, 23, 59, 0, 0, time.UTC))

	// Day 2 just after midnight UTC — bucket rolled empty → ok=false. This is a
	// date roll, not a rename: no drift, and the one-shot gate stays available.
	tc.Tick(time.Date(2026, 6, 2, 0, 1, 0, 0, time.UTC))

	if len(view.driftCalls) != 0 {
		t.Errorf("UTC date roll fired a false drift: %v (want none)", view.driftCalls)
	}
	if tc.driftFired {
		t.Error("date roll burned the one-shot driftFired gate; a real later rename can no longer fire")
	}

	// Day 2 resumes producing, then a genuine same-day miss (attribute rename)
	// must STILL fire the gate — proof the midnight roll didn't consume it.
	view.okDays["2026-06-02"] = true
	tc.Tick(time.Date(2026, 6, 2, 9, 0, 0, 0, time.UTC))
	view.okDays["2026-06-02"] = false
	tc.Tick(time.Date(2026, 6, 2, 14, 0, 0, 0, time.UTC))

	if len(view.driftCalls) == 0 {
		t.Error("a genuine same-day miss after the roll did not fire drift (gate wrongly unavailable)")
	}
}

// BUG 2: the cumulative tok/bill readout must never step backward when the
// authoritative source flips (OTEL↔transcript) and their absolute totals
// differ. The rate/sparkline path is already flip-clamped; the readout was
// not, so at midnight and on every flip it stepped to the other source's
// (lower) total. Fix carries the last-good cumulative (max-hold) so WorkTotal
// and BillTotal are monotonic across an OTEL→transcript→OTEL flip.
func TestTokenCollector_FanIn_ReadoutMonotonicAcrossSourceFlip(t *testing.T) {
	dir, path := seedTranscriptProject(t)
	view := &fakeOTELView{
		live: true, ok: true,
		input: 10000, output: 0, cacheCreate: 0, cacheRead: 5000, // work=10000 bill=15000
	}
	tc := NewTokenCollector(WithOTELView(view))
	tc.projects = dir

	t0 := time.Now()

	// A small transcript turn — far below the OTEL absolute, so a naive
	// readout would step DOWN when OTEL goes offline and transcript takes over.
	body := `{"type":"assistant","message":{"id":"m1","usage":{"input_tokens":100,"output_tokens":50,"cache_creation_input_tokens":0,"cache_read_input_tokens":20}}}` + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	var lastWork, lastBill int64
	check := func(label string, s TokenStats) {
		if s.WorkTotal < lastWork {
			t.Errorf("%s: WorkTotal stepped backward: %d → %d", label, lastWork, s.WorkTotal)
		}
		if s.BillTotal < lastBill {
			t.Errorf("%s: BillTotal stepped backward: %d → %d", label, lastBill, s.BillTotal)
		}
		lastWork, lastBill = s.WorkTotal, s.BillTotal
	}

	s0 := tc.Tick(t0) // OTEL: work=10000 (excludes cache_read), bill=15000 (includes it)
	if s0.WorkTotal != 10000 {
		t.Errorf("WorkTotal want 10000 (input+output+cacheCreate, NOT cache_read); got %d", s0.WorkTotal)
	}
	if s0.BillTotal != 15000 {
		t.Errorf("BillTotal want 15000 (work + cache_read); got %d", s0.BillTotal)
	}
	check("t0 OTEL", s0)
	view.live = false                                    // OTEL offline → transcript
	check("t1 transcript", tc.Tick(t0.Add(time.Second))) // naive would drop to work=0
	view.live = true                                     // OTEL back
	check("t2 OTEL", tc.Tick(t0.Add(2*time.Second)))     // back to OTEL absolute
	view.live = false                                    // and offline again
	check("t3 transcript", tc.Tick(t0.Add(3*time.Second)))

	if lastWork < 10000 {
		t.Errorf("final WorkTotal %d lost the OTEL high-water mark (want ≥ 10000)", lastWork)
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
