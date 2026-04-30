package cc

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeJSONLLines(t *testing.T, path string, lines []jsonlLine) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	fh, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer fh.Close()
	for _, line := range lines {
		buf, err := json.Marshal(line)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		buf = append(buf, '\n')
		if _, err := fh.Write(buf); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
}

func makeLine(metric string, val float64, attrs map[string]string) jsonlLine {
	return jsonlLine{
		Schema: 1,
		TS:     time.Now().UTC().Format(time.RFC3339Nano),
		Metric: metric,
		Value:  val,
		Attrs:  attrs,
	}
}

func newRunningTailer(t *testing.T) (*MetricsTailer, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cc-otel.jsonl")
	mt := NewMetricsTailer(path)
	mt.PollInterval = 30 * time.Millisecond
	mt.Start()
	t.Cleanup(mt.Stop)
	return mt, path
}

func waitFor(t *testing.T, deadline time.Duration, cond func() bool) {
	t.Helper()
	start := time.Now()
	for time.Since(start) < deadline {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("condition not met within %s", deadline)
}

func TestTailer_PicksUpAppendedLines(t *testing.T) {
	mt, path := newRunningTailer(t)

	writeJSONLLines(t, path, []jsonlLine{
		makeLine("claude_code.token.usage", 100, map[string]string{"query_source": "subagent", "type": "input"}),
		makeLine("claude_code.token.usage", 200, map[string]string{"query_source": "subagent", "type": "input"}),
	})

	waitFor(t, time.Second, func() bool {
		return mt.Count("claude_code.token.usage", map[string]string{"query_source": "subagent", "type": "input"}) == 2
	})
	if got := mt.Sum("claude_code.token.usage", map[string]string{"query_source": "subagent", "type": "input"}); got != 300 {
		t.Fatalf("sum want 300, got %v", got)
	}
}

func TestTailer_TruncationResetsAggregate(t *testing.T) {
	mt, path := newRunningTailer(t)

	writeJSONLLines(t, path, []jsonlLine{
		makeLine("claude_code.token.usage", 100, map[string]string{"query_source": "subagent", "type": "input"}),
	})
	waitFor(t, time.Second, func() bool {
		return mt.Sum("claude_code.token.usage", map[string]string{"query_source": "subagent", "type": "input"}) == 100
	})

	// Truncate (size shrinks below offset). Per §3.2 the aggregate must
	// also reset — re-reading surviving lines without reset would
	// double-count.
	if err := os.Truncate(path, 0); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	waitFor(t, time.Second, func() bool {
		return mt.Sum("claude_code.token.usage", map[string]string{"query_source": "subagent", "type": "input"}) == 0
	})

	writeJSONLLines(t, path, []jsonlLine{
		makeLine("claude_code.token.usage", 50, map[string]string{"query_source": "subagent", "type": "input"}),
	})
	waitFor(t, time.Second, func() bool {
		return mt.Sum("claude_code.token.usage", map[string]string{"query_source": "subagent", "type": "input"}) == 50
	})
}

func TestTailer_InodeChangeResetsAggregate(t *testing.T) {
	mt, path := newRunningTailer(t)

	writeJSONLLines(t, path, []jsonlLine{
		makeLine("claude_code.token.usage", 100, map[string]string{"query_source": "subagent", "type": "input"}),
	})
	waitFor(t, time.Second, func() bool {
		return mt.Sum("claude_code.token.usage", map[string]string{"query_source": "subagent", "type": "input"}) == 100
	})

	// Recreate the file with a fresh inode — same path, new identity.
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove: %v", err)
	}
	writeJSONLLines(t, path, []jsonlLine{
		makeLine("claude_code.token.usage", 25, map[string]string{"query_source": "subagent", "type": "input"}),
	})
	waitFor(t, time.Second, func() bool {
		return mt.Sum("claude_code.token.usage", map[string]string{"query_source": "subagent", "type": "input"}) == 25
	})
}

func TestTailer_BufferCapHandlesLargeLine(t *testing.T) {
	mt, path := newRunningTailer(t)

	// 768 KB legitimate resource-attr-heavy line. Default
	// bufio.Scanner buffer is 64 KB and would silently drop this.
	bigVal := strings.Repeat("X", 768*1024)
	line := jsonlLine{
		Schema: 1,
		TS:     time.Now().UTC().Format(time.RFC3339Nano),
		Metric: "claude_code.token.usage",
		Value:  42,
		Attrs:  map[string]string{"model": bigVal},
	}
	writeJSONLLines(t, path, []jsonlLine{line})

	waitFor(t, 2*time.Second, func() bool {
		return mt.Count("claude_code.token.usage", map[string]string{"model": bigVal}) == 1
	})
}

func TestTailer_OversizeLineDroppedWithFinding(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cc-otel.jsonl")
	mt := NewMetricsTailer(path)
	w, findingsPath := newTestWriter(t)
	mt.Findings = w
	mt.PollInterval = 30 * time.Millisecond
	mt.Start()
	t.Cleanup(mt.Stop)

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	fh, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	// 2 MB raw line — over the 1 MB scanner cap.
	if _, err := fh.WriteString(strings.Repeat("X", 2*1024*1024)); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := fh.WriteString("\n"); err != nil {
		t.Fatalf("write nl: %v", err)
	}
	fh.Close()

	waitFor(t, 2*time.Second, func() bool {
		data, err := os.ReadFile(findingsPath)
		if err != nil {
			return false
		}
		return strings.Contains(string(data), `"finding_kind":"oversize_jsonl_line"`)
	})
}

func TestTailer_AggregateKeyedByMetricAndAttrs(t *testing.T) {
	mt, path := newRunningTailer(t)

	writeJSONLLines(t, path, []jsonlLine{
		makeLine("claude_code.token.usage", 100, map[string]string{"query_source": "subagent", "type": "input"}),
		makeLine("claude_code.token.usage", 200, map[string]string{"query_source": "main", "type": "input"}),
		makeLine("claude_code.token.usage", 300, map[string]string{"query_source": "subagent", "type": "output"}),
	})

	waitFor(t, time.Second, func() bool {
		return mt.Count("claude_code.token.usage", map[string]string{"query_source": "subagent", "type": "input"}) == 1
	})
	if got := mt.Sum("claude_code.token.usage", map[string]string{"query_source": "subagent", "type": "input"}); got != 100 {
		t.Errorf("subagent/input want 100, got %v", got)
	}
	if got := mt.Sum("claude_code.token.usage", map[string]string{"query_source": "main", "type": "input"}); got != 200 {
		t.Errorf("main/input want 200, got %v", got)
	}
	if got := mt.Sum("claude_code.token.usage", map[string]string{"query_source": "subagent", "type": "output"}); got != 300 {
		t.Errorf("subagent/output want 300, got %v", got)
	}
}

func TestTailer_PrunesPriorDayAtUTCRollover(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cc-otel.jsonl")
	mt := NewMetricsTailer(path)
	mt.PollInterval = 30 * time.Millisecond
	mt.Start()
	t.Cleanup(mt.Stop)

	// Inject a stale entry timestamped 3 days ago and a fresh entry.
	stale := jsonlLine{
		Schema: 1,
		TS:     time.Now().UTC().Add(-72 * time.Hour).Format(time.RFC3339Nano),
		Metric: "claude_code.token.usage",
		Value:  500,
		Attrs:  map[string]string{"query_source": "subagent", "type": "input"},
	}
	fresh := jsonlLine{
		Schema: 1,
		TS:     time.Now().UTC().Format(time.RFC3339Nano),
		Metric: "claude_code.token.usage",
		Value:  10,
		Attrs:  map[string]string{"query_source": "subagent", "type": "input"},
	}
	writeJSONLLines(t, path, []jsonlLine{stale, fresh})

	waitFor(t, time.Second, func() bool {
		return mt.Count("claude_code.token.usage", map[string]string{"query_source": "subagent", "type": "input"}) > 0
	})

	// Force the pruning sweep with windowDays=1 — only fresh survives.
	mt.PruneOlderThan(1)
	if got := mt.Sum("claude_code.token.usage", map[string]string{"query_source": "subagent", "type": "input"}); got != 10 {
		t.Errorf("after prune want 10 (fresh only), got %v", got)
	}
}

func TestTailer_StopReturnsCleanly(t *testing.T) {
	mt, _ := newRunningTailer(t)
	mt.Stop()
	mt.Stop() // idempotent
}

func TestTailer_HandlesFileAbsentOnStart(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "cc-otel.jsonl")
	mt := NewMetricsTailer(path)
	mt.PollInterval = 30 * time.Millisecond
	mt.Start()
	t.Cleanup(mt.Stop)

	// Now create the file lazily — tailer should pick it up.
	writeJSONLLines(t, path, []jsonlLine{
		makeLine("claude_code.token.usage", 7, map[string]string{"type": "input"}),
	})
	waitFor(t, time.Second, func() bool {
		return mt.Count("claude_code.token.usage", map[string]string{"type": "input"}) == 1
	})
}

func TestTailer_ConcurrentReadersNoDeadlock(t *testing.T) {
	mt, path := newRunningTailer(t)

	for i := 0; i < 5; i++ {
		writeJSONLLines(t, path, []jsonlLine{
			makeLine("claude_code.token.usage", 1, map[string]string{"query_source": "subagent", "type": "input"}),
		})
	}

	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			_ = mt.Sum("claude_code.token.usage", map[string]string{"query_source": "subagent", "type": "input"})
			_ = mt.Count("claude_code.token.usage", map[string]string{"query_source": "subagent", "type": "input"})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("readers deadlocked")
	}
}

func TestTailer_DroppedOversizeLineFiresOneShot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cc-otel.jsonl")
	mt := NewMetricsTailer(path)
	w, findingsPath := newTestWriter(t)
	mt.Findings = w
	mt.PollInterval = 30 * time.Millisecond
	mt.Start()
	t.Cleanup(mt.Stop)

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	for i := 0; i < 3; i++ {
		fh, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		fh.WriteString(strings.Repeat("X", 2*1024*1024) + "\n")
		fh.Close()
	}

	waitFor(t, 2*time.Second, func() bool {
		data, err := os.ReadFile(findingsPath)
		if err != nil {
			return false
		}
		return strings.Contains(string(data), `"finding_kind":"oversize_jsonl_line"`)
	})

	data, _ := os.ReadFile(findingsPath)
	count := strings.Count(string(data), `"finding_kind":"oversize_jsonl_line"`)
	if count != 1 {
		t.Errorf("expected single oversize_jsonl_line one-shot, got %d", count)
	}
}

func TestTailer_LinesWithoutMetricAreSkipped(t *testing.T) {
	mt, path := newRunningTailer(t)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	fh, _ := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	fh.WriteString("not json\n")
	fh.WriteString("{}\n") // valid json but no metric
	fh.WriteString(fmt.Sprintf(`{"schema":1,"metric":"x","value":5,"ts":%q,"attrs":{"a":"b"}}`+"\n",
		time.Now().UTC().Format(time.RFC3339Nano)))
	fh.Close()

	waitFor(t, time.Second, func() bool {
		return mt.Count("x", map[string]string{"a": "b"}) == 1
	})
}
