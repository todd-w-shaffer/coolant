package widgets

import (
	"strings"
	"testing"

	"github.com/toddwshaffer/coolant/thermal/internal/collector"
	"github.com/toddwshaffer/coolant/thermal/internal/config"
	"github.com/toddwshaffer/coolant/thermal/internal/keys"
)

func TestHumanizeRate(t *testing.T) {
	tests := []struct {
		in   float64
		want string
	}{
		{0, "0"},
		{47, "47"},
		{999, "999"},
		{1000, "1.0k"},
		{1234, "1.2k"},
		{1_500_000, "1.5M"},
		{2_000_000_000, "2.0G"},
		{-5, "0"},
	}
	for _, tt := range tests {
		if got := humanizeRate(tt.in); got != tt.want {
			t.Errorf("humanizeRate(%v) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// ── sessionGroupCounts ───────────────────────────────────────

func TestSessionGroupCountsEmpty(t *testing.T) {
	got := sessionGroupCounts(nil)
	if len(got) != 0 {
		t.Errorf("sessionGroupCounts(nil) len = %d, want 0", len(got))
	}
}

func TestSessionGroupCountsCategorizesDescendants(t *testing.T) {
	sessions := []collector.SessionTree{
		{
			RootPID: 1001,
			Descendants: []collector.ProcessInfo{
				{TypeCode: "V"},  // node (vitest shows as node)
				{TypeCode: "V"},  // node
				{TypeCode: "N"},  // node
				{TypeCode: "S"},  // shell
				{TypeCode: "GO"}, // go
			},
		},
	}
	got := sessionGroupCounts(sessions)
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	g := got[0]
	if g.total() != 5 {
		t.Errorf("total = %d, want 5", g.total())
	}
	if g.cats[catIndex["node"]] != 3 {
		t.Errorf("cats[node] = %d, want 3", g.cats[catIndex["node"]])
	}
	if g.cats[catIndex["shell"]] != 1 {
		t.Errorf("cats[shell] = %d, want 1", g.cats[catIndex["shell"]])
	}
	if g.cats[catIndex["go"]] != 1 {
		t.Errorf("cats[go] = %d, want 1", g.cats[catIndex["go"]])
	}
}

func TestSessionGroupCountsMultipleSessions(t *testing.T) {
	sessions := []collector.SessionTree{
		{RootPID: 1001, Descendants: []collector.ProcessInfo{{TypeCode: "V"}}},
		{RootPID: 1002, Descendants: []collector.ProcessInfo{{TypeCode: "N"}, {TypeCode: "N"}}},
	}
	got := sessionGroupCounts(sessions)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].total() != 1 {
		t.Errorf("session 0 total = %d, want 1", got[0].total())
	}
	if got[1].total() != 2 {
		t.Errorf("session 1 total = %d, want 2", got[1].total())
	}
}

func TestSessionGroupCountsUnknownTypeDefaultsToShell(t *testing.T) {
	sessions := []collector.SessionTree{
		{RootPID: 1001, Descendants: []collector.ProcessInfo{{TypeCode: "Z"}}},
	}
	got := sessionGroupCounts(sessions)
	if got[0].cats[catIndex["shell"]] != 1 {
		t.Errorf("unknown type code should map to shell, got cats = %v", got[0].cats)
	}
}

// ── sessionPhaseColor ────────────────────────────────────────

func makeGroup(typeCodes ...string) *sessionGroup {
	sessions := []collector.SessionTree{
		{RootPID: 1, Descendants: make([]collector.ProcessInfo, len(typeCodes))},
	}
	for i, tc := range typeCodes {
		sessions[0].Descendants[i] = collector.ProcessInfo{TypeCode: tc}
	}
	groups := sessionGroupCounts(sessions)
	return &groups[0]
}

func TestSessionPhaseIdleReturnsIdle(t *testing.T) {
	g := &sessionGroup{}
	got := sessionPhaseColor(g, testTheme)
	if got != testTheme.SessionPhase.Idle {
		t.Errorf("empty session should return testTheme.SessionPhase.Idle")
	}
}

func TestSessionPhaseShellsOnlyReturnsGreen(t *testing.T) {
	g := makeGroup("S", "S", "S")
	got := sessionPhaseColor(g, testTheme)
	if got != testTheme.SessionPhase.Active {
		t.Errorf("shells below threshold should return testTheme.SessionPhase.Active")
	}
}

func TestSessionPhaseLanguageReturnsYellow(t *testing.T) {
	g := makeGroup("N") // node = language
	got := sessionPhaseColor(g, testTheme)
	if got != testTheme.SessionPhase.Language {
		t.Errorf("language category should return testTheme.SessionPhase.Language")
	}
}

func TestSessionPhaseBuildReturnsOrange(t *testing.T) {
	g := makeGroup("T") // tsc = build
	got := sessionPhaseColor(g, testTheme)
	if got != testTheme.SessionPhase.Build {
		t.Errorf("build category should return testTheme.SessionPhase.Build")
	}
}

func TestSessionPhaseBuildTrumpLanguage(t *testing.T) {
	g := makeGroup("N", "T") // node + build
	got := sessionPhaseColor(g, testTheme)
	if got != testTheme.SessionPhase.Build {
		t.Errorf("build should trump language, got %v", got)
	}
}

func TestSessionPhaseShellExplosionReturnsRed(t *testing.T) {
	codes := make([]string, config.C.Categories.ShellExplosion)
	for i := range codes {
		codes[i] = "S"
	}
	g := makeGroup(codes...)
	got := sessionPhaseColor(g, testTheme)
	if got != testTheme.SessionPhase.Explosion {
		t.Errorf("shell explosion (%d shells) should return testTheme.SessionPhase.Explosion", config.C.Categories.ShellExplosion)
	}
}

func TestSessionPhaseShellExplosionTrumpsAll(t *testing.T) {
	codes := make([]string, config.C.Categories.ShellExplosion)
	for i := range codes {
		codes[i] = "S"
	}
	codes = append(codes, "N", "T") // add language + build
	g := makeGroup(codes...)
	got := sessionPhaseColor(g, testTheme)
	if got != testTheme.SessionPhase.Explosion {
		t.Errorf("shell explosion should trump build and language")
	}
}

func TestSessionPhaseAllRuntimesReturnYellow(t *testing.T) {
	for _, tc := range []string{"N", "GO", "P", "RS", "SW"} {
		g := makeGroup(tc)
		got := sessionPhaseColor(g, testTheme)
		if got != testTheme.SessionPhase.Language {
			t.Errorf("runtime type %q should return testTheme.SessionPhase.Language", tc)
		}
	}
}

// TestTokReadoutShowsCumulative pins the render rule: the token value
// in the rates line is the billable cumulative — sum of Input + Output +
// CacheCreate + CacheRead since launch, formatted via format.FormatCount.
// This matches Claude Code's per-agent "X tokens" reports (cache reads
// are billable input, not free).
func TestTokReadoutShowsCumulative(t *testing.T) {
	state := fixtureState()
	r := NewRates(testTheme, keys.Default())
	r.SetSize(126, 1)

	state.Current.Tokens.InputTotal = 200
	state.Current.Tokens.OutputTotal = 100
	state.Current.Tokens.CacheCreateTotal = 300
	state.Current.Tokens.CacheReadTotal = 400
	r.Update(state)
	got := r.View()
	if !strings.Contains(got, "tok 1.0K") {
		t.Errorf("View should contain 'tok 1.0K' (200+100+300+400); got:\n%s", got)
	}
	if strings.Contains(got, "tok 1.0K/s") {
		t.Errorf("readout must not carry /s suffix (cumulative, not rate); got:\n%s", got)
	}
}

// TestTokReadoutMonotonic verifies the cumulative readout never steps
// backward across snapshots — the old decay-peak rule would have
// emitted a fading number when the burst ended; cumulative climbs.
func TestTokReadoutMonotonic(t *testing.T) {
	state := fixtureState()
	r := NewRates(testTheme, keys.Default())
	r.SetSize(126, 1)

	state.Current.Tokens.InputTotal = 200
	state.Current.Tokens.OutputTotal = 100
	state.Current.Tokens.CacheCreateTotal = 300
	state.Current.Tokens.CacheReadTotal = 400
	r.Update(state)
	first := r.View()

	state.Current.Tokens.InputTotal = 400
	state.Current.Tokens.OutputTotal = 200
	state.Current.Tokens.CacheCreateTotal = 600
	state.Current.Tokens.CacheReadTotal = 800
	r.Update(state)
	second := r.View()

	if !strings.Contains(first, "tok 1.0K") {
		t.Errorf("first snapshot: expected 'tok 1.0K' (1000 sum); got:\n%s", first)
	}
	if !strings.Contains(second, "tok 2.0K") {
		t.Errorf("second snapshot: expected 'tok 2.0K' (2000 sum); got:\n%s", second)
	}
}

// TestTokReadoutAcrossOTELFlip simulates a transcript→OTEL source flip
// by having the collector's totals jump (as they would when OTEL takes
// over the cumulative baseline). The widget only consumes the final
// TokenStats, so this is a black-box assertion that no widget-side
// bookkeeping snapshots a baseline that could go backward — across all
// four token fields, not just input/output.
func TestTokReadoutAcrossOTELFlip(t *testing.T) {
	state := fixtureState()
	r := NewRates(testTheme, keys.Default())
	r.SetSize(126, 1)

	state.Current.Tokens.InputTotal = 200
	state.Current.Tokens.OutputTotal = 100
	state.Current.Tokens.CacheCreateTotal = 300
	state.Current.Tokens.CacheReadTotal = 400
	r.Update(state)
	if got := r.View(); !strings.Contains(got, "tok 1.0K") {
		t.Errorf("pre-flip: expected 'tok 1.0K'; got:\n%s", got)
	}

	// OTEL takes over — totals replace transcript baseline. Cumulative
	// stays non-decreasing because OTEL has already been ticking in
	// the background; the collector merges them as the new authority.
	state.Current.Tokens.InputTotal = 1000
	state.Current.Tokens.OutputTotal = 500
	state.Current.Tokens.CacheCreateTotal = 1500
	state.Current.Tokens.CacheReadTotal = 2000
	r.Update(state)
	if got := r.View(); !strings.Contains(got, "tok 5.0K") {
		t.Errorf("post-flip: expected 'tok 5.0K'; got:\n%s", got)
	}
}

// TestTokReadoutCacheHeavyWorkload locks in the new billable-total
// semantic against future regression. On a cache-heavy workload
// (CacheReadTotal ≫ Input+Output, which is the common case for agent
// research bursts — ~70-90% of billable input lands as cache_read),
// the readout must reflect the full billable total, not just the
// fresh-model-traffic slice. Reverting to Input+Output would show 8K
// here instead of 100K — caught by this test.
func TestTokReadoutCacheHeavyWorkload(t *testing.T) {
	state := fixtureState()
	r := NewRates(testTheme, keys.Default())
	r.SetSize(126, 1)

	state.Current.Tokens.InputTotal = 5000
	state.Current.Tokens.OutputTotal = 3000
	state.Current.Tokens.CacheCreateTotal = 2000
	state.Current.Tokens.CacheReadTotal = 90000
	r.Update(state)
	got := r.View()
	if !strings.Contains(got, "tok 100K") {
		t.Errorf("cache-heavy workload: expected 'tok 100K' (full billable total); got:\n%s", got)
	}
	if strings.Contains(got, "tok 8.0K") {
		t.Errorf("readout regressed to Input+Output only (excludes cache); got:\n%s", got)
	}
}

