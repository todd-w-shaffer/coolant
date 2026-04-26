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
