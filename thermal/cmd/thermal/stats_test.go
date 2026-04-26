package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/toddwshaffer/coolant/thermal/internal/stats"
)

// emptyCfg disables persistence and JSONL fold — runStats becomes a
// pure rendering exercise against an empty aggregator. Used by every
// flag-parsing test below.
func emptyCfg() stats.Config {
	return stats.Config{}
}

func TestRunStatsHelpExitsZeroToStdout(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runStats(&stdout, &stderr, []string{"--help"}, emptyCfg())
	if code != 0 {
		t.Errorf("--help exit code: want 0, got %d", code)
	}
	if stdout.Len() == 0 {
		t.Errorf("--help should print usage to stdout, stdout is empty")
	}
	if stderr.Len() != 0 {
		t.Errorf("--help should not write to stderr, got: %q", stderr.String())
	}
}

func TestRunStatsBogusFlagExitsTwo(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runStats(&stdout, &stderr, []string{"--nope"}, emptyCfg())
	if code != 2 {
		t.Errorf("--nope exit code: want 2, got %d", code)
	}
	if stderr.Len() == 0 {
		t.Errorf("bogus flag should print usage to stderr")
	}
}

func TestRunStatsBogusWindowExitsTwo(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runStats(&stdout, &stderr, []string{"--window=bogus"}, emptyCfg())
	if code != 2 {
		t.Errorf("--window=bogus exit code: want 2, got %d", code)
	}
	if !strings.Contains(stderr.String(), "7d") {
		t.Errorf("--window=bogus should list valid windows in stderr, got: %q", stderr.String())
	}
}

func TestRunStatsBogusColorExitsTwo(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runStats(&stdout, &stderr, []string{"--color=bogus"}, emptyCfg())
	if code != 2 {
		t.Errorf("--color=bogus exit code: want 2, got %d", code)
	}
	if !strings.Contains(stderr.String(), "auto") {
		t.Errorf("--color=bogus should list valid modes, got: %q", stderr.String())
	}
}

func TestRunStatsTopClampHigh(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runStats(&stdout, &stderr,
		[]string{"--top=99", "--color=never"}, emptyCfg())
	if code != 0 {
		t.Errorf("--top=99 exit code: want 0, got %d", code)
	}
	if !strings.Contains(stderr.String(), "clamped") {
		t.Errorf("--top=99 should warn about clamp, stderr: %q", stderr.String())
	}
}

func TestRunStatsTopClampLow(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runStats(&stdout, &stderr,
		[]string{"--top=0", "--color=never"}, emptyCfg())
	if code != 0 {
		t.Errorf("--top=0 exit code: want 0, got %d", code)
	}
	if !strings.Contains(stderr.String(), "clamped") {
		t.Errorf("--top=0 should warn about clamp, stderr: %q", stderr.String())
	}
}

// writeSnapshotCache materializes a stats cache file at path so
// runStats has something to load via stats.New(cfg). Returns the
// recorded mtime so callers can assert no-Checkpoint after runStats.
func writeSnapshotCache(t *testing.T, path string, snap stats.Snapshot) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	buf, err := json.Marshal(snap)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, buf, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestRunStatsDoesNotCheckpoint(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "stats.json")
	snap := stats.Snapshot{
		SchemaVersion: stats.CurrentSchemaVersion,
		FirstSeen:     time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
		LastUpdated:   time.Date(2026, 4, 26, 0, 0, 0, 0, time.UTC),
	}
	writeSnapshotCache(t, cachePath, snap)

	beforeBuf, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatalf("read before: %v", err)
	}

	cfg := stats.Config{CachePath: cachePath}
	var stdout, stderr bytes.Buffer
	code := runStats(&stdout, &stderr, []string{"--color=never"}, cfg)
	if code != 0 {
		t.Errorf("exit code: want 0, got %d (stderr: %q)", code, stderr.String())
	}

	afterBuf, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatalf("read after: %v", err)
	}
	// Content-hash compare instead of mtime: macOS APFS has 1ns
	// resolution but a no-op rewrite is still observable. Bytes-equal
	// is the strongest "no Checkpoint" proxy available without an
	// internal hook.
	if !bytes.Equal(beforeBuf, afterBuf) {
		t.Errorf("cache modified across runStats — Checkpoint contract violated")
	}
}

func TestRunStatsSkewedInstallWarnsToStderr(t *testing.T) {
	dir := t.TempDir()
	jsonlPath := filepath.Join(dir, "events.jsonl")
	// JSONL has bytes but no schema:1 events (simulates pre-schema bash).
	if err := os.WriteFile(jsonlPath, []byte(`{"event":"old"}`+"\n"), 0o644); err != nil {
		t.Fatalf("write jsonl: %v", err)
	}

	cfg := stats.Config{JSONLPath: jsonlPath}
	var stdout, stderr bytes.Buffer
	code := runStats(&stdout, &stderr, []string{"--color=never"}, cfg)
	if code != 0 {
		t.Errorf("skewed-install exit: want 0, got %d", code)
	}
	if !strings.Contains(stderr.String(), "schema:1") {
		t.Errorf("skewed-install warning missing from stderr, got: %q", stderr.String())
	}
}

func TestRunStatsEmptyJSONLNoSkewWarning(t *testing.T) {
	dir := t.TempDir()
	jsonlPath := filepath.Join(dir, "events.jsonl")
	// Empty file → "no events yet, not skewed" per §0.2.
	if err := os.WriteFile(jsonlPath, []byte{}, 0o644); err != nil {
		t.Fatalf("write empty jsonl: %v", err)
	}

	cfg := stats.Config{JSONLPath: jsonlPath}
	var stdout, stderr bytes.Buffer
	code := runStats(&stdout, &stderr, []string{"--color=never"}, cfg)
	if code != 0 {
		t.Errorf("empty jsonl exit: want 0, got %d", code)
	}
	if strings.Contains(stderr.String(), "schema:1") {
		t.Errorf("empty jsonl should not warn, stderr: %q", stderr.String())
	}
}

// populatedSnapshot returns a snapshot rich enough that rendering
// exercises every section. Used by body-rendering tests.
func populatedSnapshot() stats.Snapshot {
	at := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	return stats.Snapshot{
		SchemaVersion: stats.CurrentSchemaVersion,
		FirstSeen:     time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		LastUpdated:   time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC),
		Records: stats.Records{
			PeakConcurrent: stats.RecordList{
				{Value: 8, AgentType: "general", Project: "coolant",
					SessionID: "a3f99999", At: at},
			},
			LongestAgentS: stats.RecordList{
				{Value: 312, AgentType: "Explore", Project: "thermal",
					SessionID: "b71c0000", AgentID: "ag-1", At: at},
			},
			LongestSessionS: stats.RecordList{
				{Value: 8000, SessionID: "a3f99999", At: at},
			},
			MostAgentsSession: stats.RecordList{
				{Value: 31, SessionID: "a3f99999", At: at},
			},
			BiggestBurst: stats.BurstRecordList{
				{Count: 12, WindowS: 2, SessionID: "a3f99999", At: at},
			},
			MostTokensAgent: stats.RecordList{
				{Value: 1_200_000, AgentType: "Explore", Project: "coolant",
					SessionID: "8d220000", AgentID: "ag-2", At: at},
			},
			MostToolCallsAgent: stats.RecordList{
				{Value: 287, AgentType: "general", Project: "thermal",
					SessionID: "b71c0000", AgentID: "ag-3", At: at},
			},
		},
		ByType: map[string]int64{
			"Explore": 142, "general": 54, "code-simplifier": 12,
		},
		ByProject: map[string]int64{
			"coolant": 89, "thermal": 71,
		},
		Daily: map[string]stats.Counters{
			"2026-04-26": {AgentsStarted: 18, AgentsCompleted: 17, AgentsOrphaned: 1, Sessions: 3},
			"2026-04-25": {AgentsStarted: 20, AgentsCompleted: 19, AgentsOrphaned: 1, Sessions: 4},
		},
	}
}

func TestRunStatsRendersAllSections(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "stats.json")
	writeSnapshotCache(t, cachePath, populatedSnapshot())

	cfg := stats.Config{CachePath: cachePath}
	var stdout, stderr bytes.Buffer
	code := runStats(&stdout, &stderr, []string{"--color=never"}, cfg)
	if code != 0 {
		t.Fatalf("exit code: want 0, got %d (stderr: %q)", code, stderr.String())
	}

	out := stdout.String()
	for _, s := range []string{"records", "windows", "distributions"} {
		if !strings.Contains(out, s) {
			t.Errorf("output missing section %q\n--- stdout:\n%s", s, out)
		}
	}
}

// epipeWriter returns syscall.EPIPE on every Write, simulating a
// reader that closed its end (e.g., `thermo stats | head -5`). Per
// §0.4, runStats must exit 0 in this case — SIGPIPE is normal Unix
// flow, not a coolant failure.
type epipeWriter struct{}

func (epipeWriter) Write(p []byte) (int, error) {
	return 0, syscall.EPIPE
}

func TestRunStatsHandlesSIGPIPE(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "stats.json")
	writeSnapshotCache(t, cachePath, populatedSnapshot())

	cfg := stats.Config{CachePath: cachePath}

	// Text mode: writes silently swallow errors; exit 0.
	var stderr bytes.Buffer
	code := runStats(epipeWriter{}, &stderr, []string{"--color=never"}, cfg)
	if code != 0 {
		t.Errorf("text mode EPIPE: want 0, got %d (stderr: %q)", code, stderr.String())
	}

	// JSON mode: encoder returns an error on EPIPE; runStats must
	// distinguish EPIPE from genuine I/O failure and still exit 0.
	stderr.Reset()
	code = runStats(epipeWriter{}, &stderr, []string{"--json"}, cfg)
	if code != 0 {
		t.Errorf("json mode EPIPE: want 0, got %d (stderr: %q)", code, stderr.String())
	}
}

func TestRunStatsJSONRoundTrips(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "stats.json")
	writeSnapshotCache(t, cachePath, populatedSnapshot())

	cfg := stats.Config{CachePath: cachePath}
	var stdout, stderr bytes.Buffer
	code := runStats(&stdout, &stderr, []string{"--json"}, cfg)
	if code != 0 {
		t.Fatalf("exit code: want 0, got %d (stderr: %q)", code, stderr.String())
	}
	var snap stats.Snapshot
	if err := json.Unmarshal(stdout.Bytes(), &snap); err != nil {
		t.Fatalf("--json output not valid JSON: %v\nstdout: %q", err, stdout.String())
	}
	if snap.SchemaVersion != stats.CurrentSchemaVersion {
		t.Errorf("round-trip schema: want %d, got %d", stats.CurrentSchemaVersion, snap.SchemaVersion)
	}
}

func TestRunStatsJSONSuppressesSkewedWarning(t *testing.T) {
	dir := t.TempDir()
	jsonlPath := filepath.Join(dir, "events.jsonl")
	if err := os.WriteFile(jsonlPath, []byte(`{"event":"old"}`+"\n"), 0o644); err != nil {
		t.Fatalf("write jsonl: %v", err)
	}
	cfg := stats.Config{JSONLPath: jsonlPath}
	var stdout, stderr bytes.Buffer
	code := runStats(&stdout, &stderr, []string{"--json"}, cfg)
	if code != 0 {
		t.Errorf("exit code: want 0, got %d", code)
	}
	if strings.Contains(stderr.String(), "schema:1") {
		t.Errorf("--json should suppress skewed-install warning, stderr: %q", stderr.String())
	}
}

func TestRunStatsJSONDoesNotSuppressTopClamp(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "stats.json")
	writeSnapshotCache(t, cachePath, populatedSnapshot())
	cfg := stats.Config{CachePath: cachePath}
	var stdout, stderr bytes.Buffer
	// --top=99 + --json: per §0.3, clamp note still goes to stderr.
	code := runStats(&stdout, &stderr, []string{"--json", "--top=99"}, cfg)
	if code != 0 {
		t.Errorf("exit code: want 0, got %d", code)
	}
	if !strings.Contains(stderr.String(), "clamped") {
		t.Errorf("--json --top=99: clamp note must still appear in stderr, got: %q",
			stderr.String())
	}
}

// extractWindowsSection slices the rendered body so window-row
// assertions don't collide with the "last 30 days" string that
// appears in the distributions section title.
func extractWindowsSection(out string) string {
	const head = "windows\n"
	i := strings.Index(out, head)
	if i < 0 {
		return ""
	}
	rest := out[i+len(head):]
	if j := strings.Index(rest, "\n\n"); j >= 0 {
		return rest[:j]
	}
	return rest
}

func TestRunStatsWindowFilter(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "stats.json")
	writeSnapshotCache(t, cachePath, populatedSnapshot())

	cfg := stats.Config{CachePath: cachePath}
	var stdout, stderr bytes.Buffer
	code := runStats(&stdout, &stderr, []string{"--window=7d", "--color=never"}, cfg)
	if code != 0 {
		t.Fatalf("exit code: want 0, got %d (stderr: %q)", code, stderr.String())
	}
	body := extractWindowsSection(stdout.String())
	if !strings.Contains(body, "7 days") {
		t.Errorf("window=7d should render '7 days' label, got:\n%s", body)
	}
	for _, exclude := range []string{"30 days", "60 days", "90 days", "today (UTC)", "lifetime"} {
		if strings.Contains(body, exclude) {
			t.Errorf("window=7d windows-section should not contain %q, got:\n%s", exclude, body)
		}
	}
}

func TestRunStatsLifetimeWindow(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "stats.json")
	writeSnapshotCache(t, cachePath, populatedSnapshot())

	cfg := stats.Config{CachePath: cachePath}
	var stdout, stderr bytes.Buffer
	code := runStats(&stdout, &stderr, []string{"--window=lifetime", "--color=never"}, cfg)
	if code != 0 {
		t.Fatalf("exit code: want 0, got %d", code)
	}
	if !strings.Contains(stdout.String(), "lifetime") {
		t.Errorf("window=lifetime should show 'lifetime' label")
	}
}

func TestRunStatsCaseFoldingWindow(t *testing.T) {
	var stdout, stderr bytes.Buffer
	// Mixed case + whitespace should normalize to "7d" and not error.
	code := runStats(&stdout, &stderr,
		[]string{"--window= 7D ", "--color=never"}, emptyCfg())
	if code != 0 {
		t.Errorf("--window=' 7D ' exit code: want 0, got %d (stderr: %q)",
			code, stderr.String())
	}
}
