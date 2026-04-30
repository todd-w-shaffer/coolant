package cc

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// newTestWriter constructs a Writer rooted in t.TempDir's ~/.coolant
// equivalent, returning the writer and the findings JSONL path.
func newTestWriter(t *testing.T) (*Writer, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), ".coolant")
	path := filepath.Join(dir, "cc-otel-findings.jsonl")
	w := NewWriter(path, nil)
	return w, path
}

func sampleFinding(kind FindingKind) Finding {
	return Finding{
		Schema:         1,
		TS:             time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC),
		WindowAnchor:   "2026-04-28",
		SignalType:     SignalTypeMetric,
		FindingKind:    kind,
		CCVersion:      "1.x.x",
		CoolantVersion: "0.23.0",
		Metric:         "claude_code_token_usage_tokens_total",
		MetricAttrs: MetricAttrs{
			Model:       "claude-opus-4-7",
			QuerySource: "subagent",
			Type:        "input",
		},
		SessionID: "abcd1234",
		Expected:  Numeric{Int: 12345},
		Observed:  Numeric{Int: 11200},
		DeltaPct:  9.3,
		Severity:  SeverityWarn,
	}
}

func readLines(t *testing.T, path string) []map[string]any {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		t.Fatalf("open: %v", err)
	}
	defer f.Close()
	var out []map[string]any
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal(line, &m); err != nil {
			t.Fatalf("malformed line %q: %v", string(line), err)
		}
		out = append(out, m)
	}
	return out
}

func TestWriter_AppendsValidFinding(t *testing.T) {
	w, path := newTestWriter(t)
	if err := w.Write(sampleFinding(KindValueMismatch)); err != nil {
		t.Fatalf("Write: %v", err)
	}
	lines := readLines(t, path)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	got := lines[0]
	if got["schema"].(float64) != 1 {
		t.Errorf("schema field missing or wrong: %v", got["schema"])
	}
	if got["signal_type"].(string) != "metric" {
		t.Errorf("signal_type expected metric, got %v", got["signal_type"])
	}
	if got["finding_kind"].(string) != string(KindValueMismatch) {
		t.Errorf("finding_kind wrong: %v", got["finding_kind"])
	}
}

func TestWriter_DedupViaIdentityTuple(t *testing.T) {
	w, path := newTestWriter(t)
	f := sampleFinding(KindValueMismatch)
	for i := 0; i < 3; i++ {
		if err := w.Write(f); err != nil {
			t.Fatalf("Write %d: %v", i, err)
		}
	}
	lines := readLines(t, path)
	if len(lines) != 1 {
		t.Fatalf("expected dedup to 1 line, got %d", len(lines))
	}
}

func TestWriter_DifferentQuerySourceIsNotDeduped(t *testing.T) {
	w, path := newTestWriter(t)
	f1 := sampleFinding(KindValueMismatch)
	f2 := sampleFinding(KindValueMismatch)
	f2.MetricAttrs.QuerySource = "auxiliary"
	if err := w.Write(f1); err != nil {
		t.Fatalf("Write f1: %v", err)
	}
	if err := w.Write(f2); err != nil {
		t.Fatalf("Write f2: %v", err)
	}
	lines := readLines(t, path)
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines (different query_source = different identity), got %d", len(lines))
	}
}

func TestWriter_OneShotLifetimeKinds(t *testing.T) {
	w, path := newTestWriter(t)
	cases := []FindingKind{
		KindCcOtelOffline,
		KindPreV3Cache,
		KindReceiverBindFailed,
		KindNonFiniteMetric,
		KindOversizeJSONLLine,
		KindReceiverRateLimited,
		KindAuxiliaryTokensUnreconciled,
		KindSessionIDDisabled,
	}
	for _, k := range cases {
		f := sampleFinding(k)
		f.WindowAnchor = "2026-04-28T12:00"
		f.Severity = SeverityInfo
		if err := w.Write(f); err != nil {
			t.Fatalf("Write %s first: %v", k, err)
		}
		f.WindowAnchor = "2026-04-28T12:01"
		if err := w.Write(f); err != nil {
			t.Fatalf("Write %s second: %v", k, err)
		}
	}
	lines := readLines(t, path)
	if len(lines) != len(cases) {
		t.Fatalf("one-shot kinds should fire once per kind, expected %d lines, got %d", len(cases), len(lines))
	}
}

func TestWriter_OneShotResetForResume(t *testing.T) {
	w, path := newTestWriter(t)
	f := sampleFinding(KindCcOtelOffline)
	f.Severity = SeverityInfo
	if err := w.Write(f); err != nil {
		t.Fatalf("offline 1: %v", err)
	}

	r := sampleFinding(KindCcOtelResumed)
	r.Severity = SeverityInfo
	w.ResetOneShot(KindCcOtelOffline)
	if err := w.Write(r); err != nil {
		t.Fatalf("resumed: %v", err)
	}

	if err := w.Write(f); err != nil {
		t.Fatalf("offline 2: %v", err)
	}
	lines := readLines(t, path)
	if len(lines) != 3 {
		t.Fatalf("flap cycle should produce 3 lines (offline/resumed/offline), got %d", len(lines))
	}
}

func TestWriter_ConcurrentGoroutinesDoNotTearLines(t *testing.T) {
	w, path := newTestWriter(t)
	const goroutines = 16
	const perGoroutine = 32
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(gid int) {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				f := sampleFinding(KindValueMismatch)
				f.SessionID = fmt.Sprintf("sess-%d-%d", gid, i)
				f.WindowAnchor = "2026-04-28"
				if err := w.Write(f); err != nil {
					t.Errorf("write: %v", err)
				}
			}
		}(g)
	}
	wg.Wait()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	for _, line := range bytes.Split(bytes.TrimRight(data, "\n"), []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal(line, &m); err != nil {
			t.Fatalf("torn line %q: %v", string(line), err)
		}
	}
}

func TestWriter_ENOENTRecreatesParentDir(t *testing.T) {
	w, path := newTestWriter(t)
	f := sampleFinding(KindValueMismatch)
	if err := w.Write(f); err != nil {
		t.Fatalf("first write: %v", err)
	}
	if err := os.RemoveAll(filepath.Dir(path)); err != nil {
		t.Fatalf("remove parent: %v", err)
	}
	f2 := sampleFinding(KindValueMismatch)
	f2.SessionID = "different"
	if err := w.Write(f2); err != nil {
		t.Fatalf("post-unlink write should succeed via single-retry mkdir: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("findings file should exist after retry: %v", err)
	}
}

func TestWriter_ENOENTStderrMessageOnRepeatedFailure(t *testing.T) {
	dir := filepath.Join(t.TempDir(), ".coolant")
	path := filepath.Join(dir, "cc-otel-findings.jsonl")
	var stderr bytes.Buffer
	w := NewWriter(path, &stderr)

	if err := os.MkdirAll(filepath.Dir(dir), 0o700); err != nil {
		t.Fatalf("mk parent: %v", err)
	}
	if err := os.WriteFile(filepath.Dir(path), []byte("not a dir"), 0o600); err != nil {
		t.Fatalf("write blocker: %v", err)
	}
	defer os.Remove(filepath.Dir(path))

	if err := w.Write(sampleFinding(KindValueMismatch)); err == nil {
		t.Fatalf("expected error when parent path is a file")
	}
	got := stderr.String()
	if !strings.HasPrefix(got, "cc-findings: write failed: ") {
		t.Errorf("stderr message mismatch: %q", got)
	}
	if strings.Contains(got, "claude-opus-4-7") || strings.Contains(got, "11200") {
		t.Errorf("stderr leaked finding payload: %q", got)
	}
}

func TestWriter_RotationAtSizeCap(t *testing.T) {
	w, path := newTestWriter(t)
	w.RotationSizeBytes = 4 * 1024

	for i := 0; i < 200; i++ {
		f := sampleFinding(KindValueMismatch)
		f.SessionID = fmt.Sprintf("sess-%05d", i)
		if err := w.Write(f); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
	}
	primary, err := os.Stat(path)
	if err != nil {
		t.Fatalf("primary: %v", err)
	}
	if primary.Size() > 4*1024+1024 {
		t.Fatalf("primary should be near cap after rotation, got %d bytes", primary.Size())
	}
	if _, err := os.Stat(path + ".1"); err != nil {
		t.Fatalf("rotated sibling missing: %v", err)
	}
}

func TestFinding_NumericTypeRejectsNonFinite(t *testing.T) {
	if !math.IsInf(math.Inf(1), 1) {
		t.Fatal("sanity check failed")
	}
	n := Numeric{Float: math.NaN()}
	if !n.IsNonFinite() {
		t.Errorf("NaN must report non-finite")
	}
	n2 := Numeric{Float: math.Inf(-1)}
	if !n2.IsNonFinite() {
		t.Errorf("-Inf must report non-finite")
	}
	n3 := Numeric{Int: 5}
	if n3.IsNonFinite() {
		t.Errorf("integer numeric must be finite")
	}
}

func TestMetricAttrs_MarshalsKnownFieldsOnly(t *testing.T) {
	a := MetricAttrs{Model: "claude-opus-4-7", QuerySource: "subagent", Type: "input"}
	b, err := json.Marshal(a)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(b)
	if !strings.Contains(got, `"model":"claude-opus-4-7"`) ||
		!strings.Contains(got, `"query_source":"subagent"`) ||
		!strings.Contains(got, `"type":"input"`) {
		t.Errorf("expected known fields in JSON: %s", got)
	}
	if strings.Contains(got, `"command_line"`) || strings.Contains(got, `"file_path"`) {
		t.Errorf("MetricAttrs leaked content-gated key: %s", got)
	}
}

func TestWriter_DifferentWindowAnchorsAreSeparateLines(t *testing.T) {
	// Writer dedupes by exact tuple; the §3.1 rule that window_anchor
	// is the UTC-day key for BOTH ReconcileToday and ReconcileWindow
	// is a contract enforced at the reconcile layer, not the writer.
	w, path := newTestWriter(t)
	f := sampleFinding(KindValueMismatch)
	f.WindowAnchor = "2026-04-28"
	if err := w.Write(f); err != nil {
		t.Fatalf("write: %v", err)
	}
	f2 := sampleFinding(KindValueMismatch)
	f2.WindowAnchor = "2026-04-29"
	if err := w.Write(f2); err != nil {
		t.Fatalf("write 2: %v", err)
	}
	lines := readLines(t, path)
	if len(lines) != 2 {
		t.Fatalf("different UTC-day anchors are different identities; got %d lines", len(lines))
	}
}
