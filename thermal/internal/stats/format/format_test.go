package format

import (
	"strings"
	"testing"
	"time"

	"github.com/toddwshaffer/coolant/thermal/internal/stats"
)

func TestFormatCount(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0"},
		{1, "1"},
		{999, "999"},
		{1_000, "1.0K"},
		{1_500, "1.5K"},
		{12_345, "12K"},
		{123_456, "123K"},
		{999_999, "999K"},
		{1_000_000, "1.0M"},
		{1_500_000_000, "1.5B"},
		{12_000_000_000, "12B"},
		{999_999_999_999, "999B+"},
	}
	for _, c := range cases {
		got := FormatCount(c.in)
		if got != c.want {
			t.Errorf("FormatCount(%d): want %q, got %q", c.in, c.want, got)
		}
	}
}

func TestFormatDuration(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0s"},
		{59, "59s"},
		{60, "1m"},
		{95, "1m 35s"},
		{312, "5m 12s"},
		{3599, "59m 59s"},
		{3600, "1h"},
		{8943, "2h 29m"},
		{90061, "1d 1h"},
	}
	for _, c := range cases {
		got := FormatDuration(c.in)
		if got != c.want {
			t.Errorf("FormatDuration(%d): want %q, got %q", c.in, c.want, got)
		}
	}
}

func TestFormatBytes(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0 B"},
		{1023, "1023 B"},
		{1024, "1.0 KB"},
		{12_345, "12.1 KB"},
		{1_048_575, "1024.0 KB"},
		{1_048_576, "1.0 MB"},
		{4_200_000, "4.0 MB"},
		{1_073_741_823, "1024.0 MB"},
		{1_073_741_824, "1.0 GB"},
	}
	for _, c := range cases {
		got := FormatBytes(c.in)
		if got != c.want {
			t.Errorf("FormatBytes(%d): want %q, got %q", c.in, c.want, got)
		}
	}
}

func TestFormatTokens(t *testing.T) {
	cases := []struct {
		in, out int64
		want    string
	}{
		{0, 0, "—"},
		{1234, 5678, "1.2K↑ · 5.7K↓"},
		{1_200_000, 47_000, "1.2M↑ · 47K↓"},
	}
	for _, c := range cases {
		got := FormatTokens(c.in, c.out)
		if got != c.want {
			t.Errorf("FormatTokens(%d,%d): want %q, got %q", c.in, c.out, c.want, got)
		}
	}
}

func TestFormatDayCompare(t *testing.T) {
	cases := []struct {
		today, best int64
		bestDate    string
		want        string
	}{
		{0, 0, "", "—"},
		{12, 47, "2026-03-12", "12 today (best: 47, Mar 12 (UTC))"},
		{50, 47, "2026-03-12", "50 today (new best, prev: 47, Mar 12 (UTC))"},
		{47, 47, "2026-03-12", "47 today (best: 47, Mar 12 (UTC))"},
	}
	for _, c := range cases {
		got := FormatDayCompare(c.today, c.best, c.bestDate)
		if got != c.want {
			t.Errorf("FormatDayCompare(%d,%d,%q): want %q, got %q",
				c.today, c.best, c.bestDate, c.want, got)
		}
	}
}

func TestFormatWindowLabel(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"today", "today (UTC)"},
		{"7d", "7 days"},
		{"30d", "30 days"},
		{"60d", "60 days"},
		{"90d", "90 days"},
		{"alltime", "lifetime"},
		{"lifetime", "lifetime"},
		{"unknown", "unknown"},
	}
	for _, c := range cases {
		got := FormatWindowLabel(c.in)
		if got != c.want {
			t.Errorf("FormatWindowLabel(%q): want %q, got %q", c.in, c.want, got)
		}
	}
}

func TestRendererPlainStripsANSI(t *testing.T) {
	r := Renderer{Plain: true, Now: time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)}
	// Plain mode should not introduce ANSI escapes via Style.
	got := r.Style(StyleHeader, "records")
	if strings.ContainsRune(got, '\x1b') {
		t.Errorf("Plain mode produced ANSI escape: %q", got)
	}
	if got != "records" {
		t.Errorf("Plain Style: want %q, got %q", "records", got)
	}
}

func TestRendererColorEmitsANSI(t *testing.T) {
	r := Renderer{Plain: false, Now: time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)}
	got := r.Style(StyleHeader, "records")
	// Lipgloss may emit zero escapes if terminal detection says no
	// color is supported. We only assert the *passthrough* — Plain
	// false does not strip user-visible content.
	if !strings.Contains(got, "records") {
		t.Errorf("color Style dropped content: %q", got)
	}
}

func TestRendererNowDefaultsToUTC(t *testing.T) {
	r := Renderer{}
	if !r.Clock().IsZero() && r.Clock().Location() != time.UTC {
		t.Errorf("Clock(): expected UTC location, got %v", r.Clock().Location())
	}
}

func TestFormatRecord(t *testing.T) {
	now := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name      string
		entry     stats.RecordEntry
		kind      RecordKind
		wantValue string
		wantMeta  string
	}{
		{
			name: "full attribution",
			entry: stats.RecordEntry{
				Value: 8, AgentType: "general", Project: "coolant",
				SessionID: "abcd1234", AgentID: "ag-01",
				At: now.Add(-3 * 24 * time.Hour),
			},
			kind:      KindCount,
			wantValue: "8",
			wantMeta:  "general · coolant · session abcd · 3d ago",
		},
		{
			name: "session-scoped (no agent)",
			entry: stats.RecordEntry{
				Value: 31, SessionID: "xyz98765",
				At: now.Add(-2 * time.Hour),
			},
			kind:      KindCount,
			wantValue: "31",
			wantMeta:  "session xyz9 · 2h ago",
		},
		{
			name: "duration kind",
			entry: stats.RecordEntry{
				Value: 312, AgentType: "Explore",
				SessionID: "b71c0000",
				At:        now.Add(-2 * 24 * time.Hour),
			},
			kind:      KindDuration,
			wantValue: "5m 12s",
			wantMeta:  "Explore · session b71c · 2d ago",
		},
		{
			name:      "zero / empty entry",
			entry:     stats.RecordEntry{},
			kind:      KindCount,
			wantValue: "0",
			wantMeta:  dashGlyph,
		},
	}
	for _, c := range cases {
		v, m := FormatRecord(c.kind, c.entry, now)
		if v != c.wantValue {
			t.Errorf("FormatRecord[%s] value: want %q, got %q", c.name, c.wantValue, v)
		}
		if m != c.wantMeta {
			t.Errorf("FormatRecord[%s] meta: want %q, got %q", c.name, c.wantMeta, m)
		}
	}
}

func TestFormatBurstRecord(t *testing.T) {
	now := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)
	b := stats.BurstRecord{
		Count: 12, WindowS: 2, SessionID: "a3f99999",
		At: now.Add(-4 * 24 * time.Hour),
	}
	v, m := FormatBurstRecord(b, now)
	if v != "12" {
		t.Errorf("burst value: want %q, got %q", "12", v)
	}
	wantMeta := "in 2s · session a3f9 · 4d ago"
	if m != wantMeta {
		t.Errorf("burst meta: want %q, got %q", wantMeta, m)
	}
}

func TestFormatLeaderboard(t *testing.T) {
	now := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)
	rl := stats.RecordList{
		{Value: 10, SessionID: "s1aaaaaa", At: now.Add(-1 * 24 * time.Hour)},
		{Value: 8, SessionID: "s2bbbbbb", At: now.Add(-2 * 24 * time.Hour)},
		{Value: 6, SessionID: "s3cccccc", At: now.Add(-3 * 24 * time.Hour)},
	}
	rows := FormatLeaderboard(KindCount, rl, 2, now)
	if len(rows) != 2 {
		t.Fatalf("leaderboard len: want 2, got %d", len(rows))
	}
	if rows[0][0] != "10" {
		t.Errorf("row 0 value: want %q, got %q", "10", rows[0][0])
	}
	// Empty list returns nil/empty.
	empty := FormatLeaderboard(KindCount, stats.RecordList{}, 5, now)
	if len(empty) != 0 {
		t.Errorf("empty leaderboard: want 0 rows, got %d", len(empty))
	}
	// Helper does NOT clamp — request more than len returns len.
	overflow := FormatLeaderboard(KindCount, rl, 99, now)
	if len(overflow) != 3 {
		t.Errorf("overflow request: want %d rows, got %d", 3, len(overflow))
	}
}

func TestFormatBurstLeaderboard(t *testing.T) {
	now := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)
	bl := stats.BurstRecordList{
		{Count: 12, WindowS: 2, SessionID: "a3f99999", At: now.Add(-4 * 24 * time.Hour)},
		{Count: 8, WindowS: 2, SessionID: "b71c0000", At: now.Add(-6 * 24 * time.Hour)},
	}
	rows := FormatBurstLeaderboard(bl, 5, now)
	if len(rows) != 2 {
		t.Fatalf("burst leaderboard len: want 2, got %d", len(rows))
	}
	if rows[0][0] != "12" {
		t.Errorf("burst row 0 value: want %q, got %q", "12", rows[0][0])
	}
	if !strings.Contains(rows[0][1], "in 2s") {
		t.Errorf("burst row 0 meta missing 'in 2s': %q", rows[0][1])
	}
}

func TestFormatDistributionRow(t *testing.T) {
	cases := []struct {
		key                string
		lifetime, last30d  int64
		wantLabel, wantStr string
	}{
		{"Explore", 142, 98, "Explore", "142 · 98"},
		{"general", 54, 0, "general", "54 · 0"},
	}
	for _, c := range cases {
		l, s := FormatDistributionRow(c.key, c.lifetime, c.last30d)
		if l != c.wantLabel {
			t.Errorf("dist row label: want %q, got %q", c.wantLabel, l)
		}
		if s != c.wantStr {
			t.Errorf("dist row counts: want %q, got %q", c.wantStr, s)
		}
	}
}

func TestFormatRelativeTime(t *testing.T) {
	now := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		t    time.Time
		want string
	}{
		{"zero", time.Time{}, "—"},
		{"future", now.Add(time.Hour), "—"},
		{"under-60s", now.Add(-5 * time.Second), "just now"},
		{"floor-edge", now.Add(-59 * time.Second), "just now"},
		{"60s-tier", now.Add(-90 * time.Second), "1m ago"},
		{"minutes", now.Add(-30 * time.Minute), "30m ago"},
		{"hours", now.Add(-2 * time.Hour), "2h ago"},
		{"days", now.Add(-3 * 24 * time.Hour), "3d ago"},
		{"weeks", now.Add(-10 * 24 * time.Hour), "1w ago"},
		{"date-tier", time.Date(2026, 3, 12, 0, 0, 0, 0, time.UTC), "Mar 12"},
	}
	for _, c := range cases {
		got := FormatRelativeTime(c.t, now)
		if got != c.want {
			t.Errorf("FormatRelativeTime[%s]: want %q, got %q", c.name, c.want, got)
		}
	}
}

func TestWindowKeys(t *testing.T) {
	cases := []struct {
		name    string
		visible []string
		want    []string
	}{
		{"young tier carries alltime", []string{"7d", "alltime"}, []string{"today", "7d", "alltime"}},
		{"mid tier gains lifetime tail", []string{"7d", "30d"}, []string{"today", "7d", "30d", "lifetime"}},
		{"60d tier gains lifetime tail", []string{"7d", "30d", "60d"}, []string{"today", "7d", "30d", "60d", "lifetime"}},
		{"oldest tier carries alltime", []string{"7d", "30d", "90d", "alltime"}, []string{"today", "7d", "30d", "90d", "alltime"}},
		{"literal lifetime not doubled", []string{"7d", "lifetime"}, []string{"today", "7d", "lifetime"}},
		{"empty visible still bracketed", nil, []string{"today", "lifetime"}},
	}
	for _, c := range cases {
		got := WindowKeys(c.visible)
		if len(got) != len(c.want) {
			t.Errorf("%s: WindowKeys(%v) = %v, want %v", c.name, c.visible, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("%s: WindowKeys(%v)[%d] = %q, want %q", c.name, c.visible, i, got[i], c.want[i])
			}
		}
	}
}

func TestBoardsPickDistinctLists(t *testing.T) {
	rec := stats.Records{
		PeakConcurrent:     stats.RecordList{{Value: 1}},
		LongestAgentS:      stats.RecordList{{Value: 2}},
		LongestSessionS:    stats.RecordList{{Value: 3}},
		MostAgentsSession:  stats.RecordList{{Value: 4}},
		MostTokensAgent:    stats.RecordList{{Value: 5}},
		MostToolCallsAgent: stats.RecordList{{Value: 6}},
	}
	boards := Boards()
	if len(boards) != 6 {
		t.Fatalf("Boards() returned %d boards, want 6", len(boards))
	}
	seenValues := map[int64]bool{}
	for _, b := range boards {
		if b.Label == "" {
			t.Error("board with empty label")
		}
		rl := b.Pick(rec)
		if len(rl) != 1 {
			t.Fatalf("board %q picked list of len %d, want 1", b.Label, len(rl))
		}
		if seenValues[rl[0].Value] {
			t.Errorf("board %q picks the same list as an earlier board", b.Label)
		}
		seenValues[rl[0].Value] = true
	}
	// Duration-valued boards must carry KindDuration so values render
	// as time, not counts.
	kinds := map[string]RecordKind{}
	for _, b := range boards {
		kinds[b.Label] = b.Kind
	}
	if kinds["longest agent"] != KindDuration || kinds["longest session"] != KindDuration {
		t.Error("longest agent/session boards must be KindDuration")
	}
	if kinds["peak concurrent"] != KindCount {
		t.Error("peak concurrent must be KindCount")
	}
	if BurstBoardLabel == "" {
		t.Error("BurstBoardLabel must be non-empty")
	}
}

func TestFormatWindowCounters(t *testing.T) {
	cases := []struct {
		name string
		c    stats.Counters
		want string
	}{
		{
			"zero tails drop",
			stats.Counters{AgentsStarted: 4, AgentsCompleted: 4, Sessions: 1},
			"4 started · 4 completed · 0 orphaned · 1 sessions",
		},
		{
			"transcripts and gate.cap append",
			stats.Counters{AgentsStarted: 47, AgentsCompleted: 46, AgentsOrphaned: 1, Sessions: 12, TranscriptBytesTotal: 4404019, GateCapEvents: 3},
			"47 started · 46 completed · 1 orphaned · 12 sessions · 4.2 MB transcripts · 3 gate.cap",
		},
		{
			"gate.cap without transcripts",
			stats.Counters{AgentsStarted: 1, GateCapEvents: 3},
			"1 started · 0 completed · 0 orphaned · 0 sessions · 3 gate.cap",
		},
		{
			"transcripts without gate.cap",
			stats.Counters{AgentsStarted: 2, TranscriptBytesTotal: 2048},
			"2 started · 0 completed · 0 orphaned · 0 sessions · 2.0 KB transcripts",
		},
	}
	for _, c := range cases {
		if got := FormatWindowCounters(c.c); got != c.want {
			t.Errorf("%s: FormatWindowCounters = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestDistRows(t *testing.T) {
	lifetime := map[string]int64{"general-purpose": 198, "Explore": 31, "__other": 31}
	last30 := map[string]int64{"general-purpose": 87, "Plan": 5}
	rows := DistRows(lifetime, last30)
	wantKeys := []string{"general-purpose", "Explore", "__other", "Plan"}
	if len(rows) != len(wantKeys) {
		t.Fatalf("DistRows returned %d rows, want %d: %v", len(rows), len(wantKeys), rows)
	}
	for i, k := range wantKeys {
		if rows[i].Key != k {
			t.Errorf("rows[%d].Key = %q, want %q (__other sinks to its count tier, 30d-only keys trail)", i, rows[i].Key, k)
		}
	}
	if rows[0].Lifetime != 198 || rows[0].Last30 != 87 {
		t.Errorf("rows[0] counts = %d/%d, want 198/87", rows[0].Lifetime, rows[0].Last30)
	}
	if rows[3].Lifetime != 0 || rows[3].Last30 != 5 {
		t.Errorf("30d-only key counts = %d/%d, want 0/5", rows[3].Lifetime, rows[3].Last30)
	}
}

func TestFormatTop1(t *testing.T) {
	now := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	rl := stats.RecordList{{Value: 8, At: now.Add(-6 * 24 * time.Hour)}}
	v, when := FormatTop1(KindCount, rl, now)
	if v != "8" || when != "6d ago" {
		t.Errorf("FormatTop1 = (%q, %q), want (\"8\", \"6d ago\")", v, when)
	}
	v, when = FormatTop1(KindDuration, stats.RecordList{{Value: 312, At: now.Add(-time.Hour)}}, now)
	if v != "5m 12s" || when != "1h ago" {
		t.Errorf("FormatTop1 duration = (%q, %q), want (\"5m 12s\", \"1h ago\")", v, when)
	}
	// Zero-safe: empty list → dash value, no time.
	v, when = FormatTop1(KindCount, nil, now)
	if v != "—" || when != "" {
		t.Errorf("FormatTop1 empty = (%q, %q), want (\"—\", \"\")", v, when)
	}
	// Zero timestamp collapses to "" rather than showing the dash.
	v, when = FormatTop1(KindCount, stats.RecordList{{Value: 3}}, now)
	if v != "3" || when != "" {
		t.Errorf("FormatTop1 zero-time = (%q, %q), want (\"3\", \"\")", v, when)
	}
}

func TestFormatBurstTop1(t *testing.T) {
	now := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	v, when := FormatBurstTop1(stats.BurstRecordList{{Count: 6, WindowS: 2, At: now.Add(-30 * time.Second)}}, now)
	if v != "6" || when != "just now" {
		t.Errorf("FormatBurstTop1 = (%q, %q), want (\"6\", \"just now\")", v, when)
	}
	v, when = FormatBurstTop1(nil, now)
	if v != "—" || when != "" {
		t.Errorf("FormatBurstTop1 empty = (%q, %q), want (\"—\", \"\")", v, when)
	}
}

func TestParseWindowKey(t *testing.T) {
	cases := []struct {
		key  string
		kind WindowKind
		days int
	}{
		{"today", WindowToday, 0},
		{"alltime", WindowLifetime, 0},
		{"lifetime", WindowLifetime, 0},
		{"7d", WindowDays, 7},
		{"30d", WindowDays, 30},
		{"90d", WindowDays, 90},
		{"180d", WindowDays, 180}, // future tiers parse with zero edits
		{"0d", WindowUnknown, 0},
		{"-3d", WindowUnknown, 0},
		{"bogus", WindowUnknown, 0},
		{"d", WindowUnknown, 0},
	}
	for _, c := range cases {
		kind, days := ParseWindowKey(c.key)
		if kind != c.kind || days != c.days {
			t.Errorf("ParseWindowKey(%q) = (%v, %d), want (%v, %d)", c.key, kind, days, c.kind, c.days)
		}
	}
}
