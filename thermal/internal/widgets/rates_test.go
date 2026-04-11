package widgets

import (
	"testing"

	"github.com/toddwshaffer/coolant/thermal/internal/collector"
	"github.com/toddwshaffer/coolant/thermal/internal/config"
)

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
