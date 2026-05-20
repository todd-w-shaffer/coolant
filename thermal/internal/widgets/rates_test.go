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

// newRatesForDecay sets up a Rates with width=126 so ioPeakHalfLife
// resolves to 1s (sparkSeconds = (126-6)/30 = 4; half-life = 4/4 = 1).
// Clean math for the decay tests below.
func newRatesForDecay(t *testing.T, clock *fakeClock) *Rates {
	t.Helper()
	r := NewRates(testTheme, keys.Default())
	r.now = clock.Now
	r.SetSize(126, 1)
	return r
}

// TestIOPeakSnapsUpAndDecays verifies snap-up on burst and exponential
// fade between bursts with half-life derived from the sparkline window.
func TestIOPeakSnapsUpAndDecays(t *testing.T) {
	state := fixtureState()
	t0 := mustTime("2026-05-19T12:00:00Z")
	clock := &fakeClock{at: t0}
	r := newRatesForDecay(t, clock)

	state.Current.Tokens.IOTokensPerSec = 1000
	r.Update(state)
	if got := r.decayedIOPeak(); got != 1000 {
		t.Fatalf("after snap-up, peak = %v, want 1000", got)
	}

	clock.at = t0.Add(time.Second) // one half-life
	state.Current.Tokens.IOTokensPerSec = 0
	r.Update(state)
	if got := r.decayedIOPeak(); got < 490 || got > 510 {
		t.Errorf("after 1s (1 half-life), peak = %v, want ~500", got)
	}

	clock.at = t0.Add(2 * time.Second) // two half-lives
	r.Update(state)
	if got := r.decayedIOPeak(); got < 240 || got > 260 {
		t.Errorf("after 2s (2 half-lives), peak = %v, want ~250", got)
	}
}

// TestIOPeakResetsOnNewHighRate verifies a fresh high rate during the
// decay tail snaps the peak back up and resets the decay clock.
func TestIOPeakResetsOnNewHighRate(t *testing.T) {
	state := fixtureState()
	t0 := mustTime("2026-05-19T12:00:00Z")
	clock := &fakeClock{at: t0}
	r := newRatesForDecay(t, clock)

	state.Current.Tokens.IOTokensPerSec = 800
	r.Update(state)

	clock.at = t0.Add(time.Second)
	state.Current.Tokens.IOTokensPerSec = 1500
	r.Update(state)
	if got := r.decayedIOPeak(); got != 1500 {
		t.Errorf("after new high snap-up, peak = %v, want 1500", got)
	}
}

// TestIOReadoutShowsDecayedPeak pins the render rule: the io value in
// the rates line is the decayed peak (not the raw IOTokensPerSec).
func TestIOReadoutShowsDecayedPeak(t *testing.T) {
	state := fixtureState()
	t0 := mustTime("2026-05-19T12:00:00Z")
	clock := &fakeClock{at: t0}
	r := newRatesForDecay(t, clock)

	state.Current.Tokens.IOTokensPerSec = 1000
	r.Update(state)
	if got := r.View(); !strings.Contains(got, "io 1.0k/s") {
		t.Errorf("at burst, View should contain 'io 1.0k/s'; got:\n%s", got)
	}

	// Live rate drops to 0; readout should still show the decaying peak,
	// not 0.
	clock.at = t0.Add(time.Second)
	state.Current.Tokens.IOTokensPerSec = 0
	r.Update(state)
	if got := r.View(); !strings.Contains(got, "io 500/s") {
		t.Errorf("during decay, View should contain 'io 500/s'; got:\n%s", got)
	}
}

// TestIOPeakHalfLifeScalesWithWidth verifies the derivation: a wider
// terminal gets a longer half-life because its sparkline window holds
// more time.
func TestIOPeakHalfLifeScalesWithWidth(t *testing.T) {
	r := NewRates(testTheme, keys.Default())

	r.SetSize(126, 1) // sparkSeconds = 4 → half-life = 1
	if got := r.ioPeakHalfLife(); got < 0.99 || got > 1.01 {
		t.Errorf("width=126: half-life = %v, want ~1.0", got)
	}

	r.SetSize(246, 1) // sparkSeconds = 8 → half-life = 2
	if got := r.ioPeakHalfLife(); got < 1.99 || got > 2.01 {
		t.Errorf("width=246: half-life = %v, want ~2.0", got)
	}

	r.SetSize(30, 1) // sparkSeconds clamped → half-life = 0.25
	if got := r.ioPeakHalfLife(); got < 0.24 || got > 0.26 {
		t.Errorf("width=30: half-life = %v, want ~0.25", got)
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

