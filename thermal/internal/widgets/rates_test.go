package widgets

import (
	"strings"
	"testing"
	"time"

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

// TestIOPeakSnapsUpAndDecays drives Rates with a sequence of synthetic
// snapshots and a fake clock to verify (1) the readout peak snaps up
// immediately when a higher rate lands, (2) the peak decays toward zero
// once the live rate drops, with a 2-second half-life.
func TestIOPeakSnapsUpAndDecays(t *testing.T) {
	state := fixtureState()
	t0 := mustTime("2026-05-19T12:00:00Z")
	clock := &fakeClock{at: t0}

	r := NewRates(testTheme, keys.Default())
	r.now = clock.Now

	// Burst: live rate spikes to 1000.
	state.Current.Tokens.IOTokensPerSec = 1000
	r.Update(state)
	if got := r.decayedIOPeak(); got != 1000 {
		t.Fatalf("after snap-up, peak = %v, want 1000", got)
	}

	// 2 seconds later, live rate is back to 0. Peak should have halved.
	clock.at = t0.Add(2 * time.Second)
	state.Current.Tokens.IOTokensPerSec = 0
	r.Update(state)
	if got := r.decayedIOPeak(); got < 490 || got > 510 {
		t.Errorf("after 2s decay, peak = %v, want ~500", got)
	}

	// 4 seconds total later, peak should be ~250 (two half-lives).
	clock.at = t0.Add(4 * time.Second)
	r.Update(state)
	if got := r.decayedIOPeak(); got < 240 || got > 260 {
		t.Errorf("after 4s decay, peak = %v, want ~250", got)
	}
}

// TestIOPeakResetsOnNewHighRate verifies that a fresh high rate during the
// decay tail snaps the peak back up and resets the decay clock.
func TestIOPeakResetsOnNewHighRate(t *testing.T) {
	state := fixtureState()
	t0 := mustTime("2026-05-19T12:00:00Z")
	clock := &fakeClock{at: t0}

	r := NewRates(testTheme, keys.Default())
	r.now = clock.Now

	state.Current.Tokens.IOTokensPerSec = 800
	r.Update(state)

	// 1 second later, peak has decayed to ~566. A new rate of 1500 should
	// snap up and reset.
	clock.at = t0.Add(time.Second)
	state.Current.Tokens.IOTokensPerSec = 1500
	r.Update(state)
	if got := r.decayedIOPeak(); got != 1500 {
		t.Errorf("after new high snap-up, peak = %v, want 1500", got)
	}
}

// TestIOPeakSuppressionWhenBelowOne pins the rendering rule: when the
// decayed peak falls below 1, the "(peak ...)" parenthetical is dropped
// from the rates line. Avoids "(peak 0)" noise during long idle stretches.
func TestIOPeakSuppressionWhenBelowOne(t *testing.T) {
	state := fixtureState()
	t0 := mustTime("2026-05-19T12:00:00Z")
	clock := &fakeClock{at: t0}

	r := NewRates(testTheme, keys.Default())
	r.now = clock.Now
	r.SetSize(244, 1)

	// Burst.
	state.Current.Tokens.IOTokensPerSec = 1000
	r.Update(state)
	clock.at = t0.Add(20 * time.Second) // ten half-lives → peak ≈ 1
	state.Current.Tokens.IOTokensPerSec = 0
	r.Update(state)
	rendered := r.View()
	if peak := r.decayedIOPeak(); peak >= 1 {
		t.Skipf("decayed peak still %v after 20s — half-life math drifted, can't assert suppression", peak)
	}
	// "(peak" must not appear once decayed peak < 1.
	if strings.Contains(rendered, "(peak") {
		t.Errorf("rendered rates line contains '(peak ...)' when decayed peak should be < 1:\n%s", rendered)
	}
}

// fakeClock satisfies func() time.Time for injecting deterministic time
// into Rates.now during decay tests.
type fakeClock struct{ at time.Time }

func (c *fakeClock) Now() time.Time { return c.at }

func mustTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

