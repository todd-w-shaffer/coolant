package cc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/toddwshaffer/coolant/thermal/internal/stats"
)

type stubAggregator struct {
	snap        stats.Snapshot
	byTypeWin   map[string]int64
	byProjWin   map[string]int64
	byTypeWin1  map[string]int64
	byProjWin1  map[string]int64
	windowDays1 bool
}

func (s *stubAggregator) Snapshot() stats.Snapshot { return s.snap }
func (s *stubAggregator) WindowByType(days int) map[string]int64 {
	if days == 1 && s.windowDays1 {
		return s.byTypeWin1
	}
	return s.byTypeWin
}
func (s *stubAggregator) WindowByProject(days int) map[string]int64 {
	if days == 1 && s.windowDays1 {
		return s.byProjWin1
	}
	return s.byProjWin
}

func newReconcileFixture(t *testing.T) (*Reconciler, *MetricsTailer, *stubAggregator, string) {
	t.Helper()
	jsonl := filepath.Join(t.TempDir(), "cc-otel.jsonl")
	mt := NewMetricsTailer(jsonl)
	w, findingsPath := newTestWriter(t)
	stub := &stubAggregator{snap: stats.Snapshot{SchemaVersion: 3, LastUpdated: time.Now().UTC()}}
	a := NewAdapter(AdapterConfig{Findings: w, CCVersion: "1.x.x"})
	r := NewReconciler(ReconcilerConfig{
		Tailer:     mt,
		Aggregator: stub,
		Adapter:    a,
		Findings:   w,
	})
	return r, mt, stub, findingsPath
}

func findingsContains(t *testing.T, path, kind string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	return strings.Count(string(data), `"finding_kind":"`+kind+`"`)
}

func TestReconcileToday_ShortCircuitsBelowV3(t *testing.T) {
	r, _, stub, findingsPath := newReconcileFixture(t)
	stub.snap.SchemaVersion = 2

	if err := r.ReconcileToday(); err != nil {
		t.Fatalf("ReconcileToday: %v", err)
	}
	if got := findingsContains(t, findingsPath, "pre_v3_cache"); got != 1 {
		t.Errorf("expected ONE pre_v3_cache finding, got %d", got)
	}
	// Subsequent ticks must remain quiet on pre_v3_cache (one-shot).
	for i := 0; i < 3; i++ {
		_ = r.ReconcileToday()
	}
	if got := findingsContains(t, findingsPath, "pre_v3_cache"); got != 1 {
		t.Errorf("pre_v3_cache must be one-shot per process, got %d", got)
	}
}

func TestReconcileWindow_ShortCircuitsBelowV3(t *testing.T) {
	r, _, stub, findingsPath := newReconcileFixture(t)
	stub.snap.SchemaVersion = 2

	if err := r.ReconcileWindow(7); err != nil {
		t.Fatalf("ReconcileWindow: %v", err)
	}
	if got := findingsContains(t, findingsPath, "pre_v3_cache"); got != 1 {
		t.Errorf("expected pre_v3_cache (gate at top, not after aggregator read), got %d", got)
	}
}

func TestReconcileWindow_ZeroDaysReturnsError(t *testing.T) {
	r, _, _, _ := newReconcileFixture(t)
	if err := r.ReconcileWindow(0); err == nil {
		t.Errorf("ReconcileWindow(0) must return error per §3.1 (aggregator returns empty for days=0 → would emit false missing_emission)")
	}
}

func TestReconcile_EmptyFeedFiresOfflineThenResumed(t *testing.T) {
	r, mt, _, findingsPath := newReconcileFixture(t)
	mt.Start()
	t.Cleanup(mt.Stop)

	// No JSONL lines AND no successful POST = offline.
	r.SetClock(func() time.Time { return time.Now().Add(5 * time.Minute) })
	r.SetReceiverPostTime(time.Time{}) // never posted
	r.checkOnlineStatus()

	if got := findingsContains(t, findingsPath, "cc_otel_offline"); got != 1 {
		t.Errorf("expected ONE cc_otel_offline finding, got %d", got)
	}

	// Now simulate a successful POST + new line.
	r.SetReceiverPostTime(time.Now())
	if err := os.MkdirAll(filepath.Dir(mt.JSONLPath), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeJSONLLines(t, mt.JSONLPath, []jsonlLine{
		makeLine("claude_code.token.usage", 10, map[string]string{"query_source": "subagent", "type": "input"}),
	})
	waitFor(t, time.Second, func() bool {
		return mt.Count("claude_code.token.usage", map[string]string{"query_source": "subagent", "type": "input"}) > 0
	})
	r.SetClock(time.Now)
	r.checkOnlineStatus()
	if got := findingsContains(t, findingsPath, "cc_otel_resumed"); got != 1 {
		t.Errorf("expected ONE cc_otel_resumed after resumption, got %d", got)
	}

	// Flap cycle: offline → resumed → offline must produce a fresh
	// offline (boolean reset on resumption).
	r.SetReceiverPostTime(time.Time{})
	r.SetClock(func() time.Time { return time.Now().Add(5 * time.Minute) })
	r.checkOnlineStatus()
	if got := findingsContains(t, findingsPath, "cc_otel_offline"); got != 2 {
		t.Errorf("flap should produce a second cc_otel_offline, got %d", got)
	}
}

func TestReconcile_HardCrashFlushWindowSuppressesMissingEmission(t *testing.T) {
	r, _, _, findingsPath := newReconcileFixture(t)

	// Agent.stop just happened — within the 17s suppression window.
	now := time.Now().UTC()
	stopTS := now.Add(-5 * time.Second)
	r.MarkAgentStop("sess-A", stopTS)

	r.SetClock(func() time.Time { return now })
	suppressed := r.shouldSuppressMissingForCrashWindow("sess-A", now)
	if !suppressed {
		t.Errorf("agent.stop within [T-17s, T] must suppress missing_emission per §0.9")
	}
	if got := findingsContains(t, findingsPath, "cc_flush_truncated"); got != 1 {
		t.Errorf("expected single cc_flush_truncated finding for the suppressed session, got %d", got)
	}

	// Older stop must NOT suppress.
	stopTS2 := now.Add(-30 * time.Second)
	r.MarkAgentStop("sess-B", stopTS2)
	if r.shouldSuppressMissingForCrashWindow("sess-B", now) {
		t.Errorf("agent.stop older than T-17s must NOT suppress")
	}
}

func TestReconcile_OTLPRetryIsClassifiedAsSuspected(t *testing.T) {
	r, _, _, _ := newReconcileFixture(t)

	// expected=10, observed=20 → 2× → likely retry
	got := r.classifyValueMismatch(10, 20)
	if got != classRetry {
		t.Errorf("2× over expected>=5 should be suspected_otlp_retry, got %v", got)
	}
	// expected=10, observed=30 → 3× → likely retry
	if r.classifyValueMismatch(10, 30) != classRetry {
		t.Errorf("3× should be suspected_otlp_retry")
	}
	// expected=10, observed=40 → 4× → outside retry-plausible
	if r.classifyValueMismatch(10, 40) != classMismatch {
		t.Errorf("4× is outside retry-plausible multiples → value_mismatch")
	}
	// expected=2, observed=4 → small-expected guard says still mismatch
	if r.classifyValueMismatch(2, 4) != classMismatch {
		t.Errorf("expected<5 with 2× must NOT be suspected_otlp_retry (small-expected guard)")
	}
	// Non-integral 1.5x → mismatch
	if r.classifyValueMismatch(10, 15) != classMismatch {
		t.Errorf("non-integral multiple is mismatch, not retry")
	}
}

func TestReconcile_UTCMidnightGraceDowngradesSeverity(t *testing.T) {
	r, _, _, _ := newReconcileFixture(t)
	at2358UTC := time.Date(2026, 4, 28, 23, 58, 30, 0, time.UTC)
	r.SetClock(func() time.Time { return at2358UTC })
	if r.severityForCount(15, true) != SeverityWarn {
		t.Errorf("count drift 15%% inside [23:58, 00:02] downgrades error→warn (15%% > 10%% would normally be error)")
	}
	at1200UTC := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)
	r.SetClock(func() time.Time { return at1200UTC })
	if r.severityForCount(15, true) != SeverityError {
		t.Errorf("count drift 15%% outside grace window stays error")
	}
}

func TestReconcile_QuerySourceLikeForLikeTokens(t *testing.T) {
	r, mt, stub, _ := newReconcileFixture(t)
	mt.Start()
	t.Cleanup(mt.Stop)

	stub.snap.Daily = map[string]stats.Counters{
		time.Now().UTC().Format("2006-01-02"): {
			TokensInTotal: 1000,
		},
	}

	if err := os.MkdirAll(filepath.Dir(mt.JSONLPath), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeJSONLLines(t, mt.JSONLPath, []jsonlLine{
		makeLine("claude_code.token.usage", 1000, map[string]string{"query_source": "subagent", "type": "input"}),
		makeLine("claude_code.token.usage", 5000, map[string]string{"query_source": "main", "type": "input"}),
		makeLine("claude_code.token.usage", 200, map[string]string{"query_source": "auxiliary", "type": "input"}),
	})
	waitFor(t, time.Second, func() bool {
		return mt.Count("claude_code.token.usage", map[string]string{"query_source": "subagent", "type": "input"}) > 0
	})

	// CC OTEL totals would be 6200 unfiltered; query_source=subagent
	// matches coolant's 1000 exactly. The filter must produce zero
	// drift on a true match.
	got := r.subagentInputTokens(time.Now().UTC().Format("2006-01-02"))
	if got != 1000 {
		t.Errorf("subagent-input tokens want 1000, got %d", got)
	}
}

func TestReconcile_LockOrderingNoDeadlockOnConcurrentTicks(t *testing.T) {
	r, mt, _, _ := newReconcileFixture(t)
	mt.Start()
	t.Cleanup(mt.Stop)

	done := make(chan struct{})
	go func() {
		for i := 0; i < 50; i++ {
			_ = r.ReconcileToday()
		}
		close(done)
	}()

	for i := 0; i < 50; i++ {
		_ = r.ReconcileWindow(1)
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("concurrent ticks deadlocked")
	}
}
