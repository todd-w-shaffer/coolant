package cc

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// feedToken absorbs one cumulative token data point into the tailer, bucketed
// by its UTC timestamp's day (exactly as the live receiver writes them).
func feedToken(mt *MetricsTailer, ts time.Time, typeVal string, value float64) {
	line := fmt.Sprintf(
		`{"schema":1,"ts":%q,"metric":"claude_code.token.usage","value":%g,"attrs":{"type":%q,"query_source":"main"}}`,
		ts.UTC().Format(time.RFC3339Nano), value, typeVal,
	)
	mt.absorbLine([]byte(line))
}

// Integration across a synthetic UTC midnight, driving the REAL tailer +
// adapter (no host-clock change, no live receiver). Closes the harness gap that
// the collector unit tests leave: those mock the adapter via fakeOTELView, so
// they never exercise the real `dayKey := now.UTC()` against the tailer's
// `ts.UTC()` per-day bucketing. Proves the bucket the fakeOTELView okDays map
// stands in for behaves the way the Bug-1 fix assumes:
//   - day 1 with data → OTELTokens ok=true with the right sums
//   - day 2 before any export → ok=false (empty bucket) and NO spurious drift
//     finding (the midnight roll is a gap, not a partial-cache miss)
//   - day 2 once it exports → ok=true again
func TestIntegration_OTELTokensAcrossUTCMidnight(t *testing.T) {
	tailer := NewMetricsTailer("") // path unused — we feed absorbLine directly
	findingsPath := filepath.Join(t.TempDir(), "findings.jsonl")
	adapter := NewAdapter(AdapterConfig{
		Findings:       NewWriter(findingsPath, io.Discard),
		CCVersion:      "ccTest",
		CoolantVersion: "test",
	})
	adapter.SetTailer(tailer)

	// Day 1, just before midnight UTC: a full cumulative reading, all four types.
	day1 := time.Date(2026, 6, 1, 23, 59, 0, 0, time.UTC)
	feedToken(tailer, day1, "input", 1000)
	feedToken(tailer, day1, "output", 400)
	feedToken(tailer, day1, "cacheCreation", 200)
	feedToken(tailer, day1, "cacheRead", 5000)

	in, out, cc, cr, ok := adapter.OTELTokens(day1.Add(30 * time.Second))
	if !ok {
		t.Fatal("day 1: OTELTokens ok=false, want true (bucket has data)")
	}
	if in != 1000 || out != 400 || cc != 200 || cr != 5000 {
		t.Errorf("day 1 sums = (%d,%d,%d,%d), want (1000,400,200,5000)", in, out, cc, cr)
	}

	// Day 2, just after midnight UTC, before any day-2 export: the per-day
	// cumulative bucket has rolled empty. OTELTokens must report ok=false — and
	// must NOT fire a schema-drift finding, because a fully empty bucket is a
	// gap (no-data-yet), not the partial-cache miss that signals a rename.
	day2 := time.Date(2026, 6, 2, 0, 1, 0, 0, time.UTC)
	_, _, _, _, ok = adapter.OTELTokens(day2)
	if ok {
		t.Error("day 2 (empty bucket): OTELTokens ok=true, want false")
	}
	if data, _ := os.ReadFile(findingsPath); len(data) > 0 {
		t.Errorf("midnight roll wrote a spurious drift finding:\n%s", data)
	}

	// Day 2 starts exporting — the new day's bucket fills and OTELTokens
	// recovers without any drift having fired.
	feedToken(tailer, day2, "input", 50)
	feedToken(tailer, day2, "output", 20)
	feedToken(tailer, day2, "cacheCreation", 10)
	feedToken(tailer, day2, "cacheRead", 100)
	in, _, _, _, ok = adapter.OTELTokens(day2.Add(time.Minute))
	if !ok || in != 50 {
		t.Errorf("day 2 after export: ok=%v in=%d, want ok=true in=50", ok, in)
	}
	if data, _ := os.ReadFile(findingsPath); strings.Contains(string(data), "token_lookup_miss") {
		t.Errorf("unexpected token_lookup_miss finding across the boundary:\n%s", data)
	}
}
