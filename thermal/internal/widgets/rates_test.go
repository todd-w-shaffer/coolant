package widgets

import (
	"strings"
	"testing"

	"github.com/toddwshaffer/coolant/thermal/internal/collector"
	"github.com/toddwshaffer/coolant/thermal/internal/ui"
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
				{TypeCode: "V"}, // test
				{TypeCode: "V"}, // test
				{TypeCode: "N"}, // run
				{TypeCode: "S"}, // shell
			},
		},
	}
	got := sessionGroupCounts(sessions)
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	g := got[0]
	if g.total() != 4 {
		t.Errorf("total = %d, want 4", g.total())
	}
	if g.cats[catIndex["test"]] != 2 {
		t.Errorf("cats[test] = %d, want 2", g.cats[catIndex["test"]])
	}
	if g.cats[catIndex["run"]] != 1 {
		t.Errorf("cats[run] = %d, want 1", g.cats[catIndex["run"]])
	}
	if g.cats[catIndex["shell"]] != 1 {
		t.Errorf("cats[shell] = %d, want 1", g.cats[catIndex["shell"]])
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

// ── renderSessionRow ─────────────────────────────────────────

func TestRenderSessionRowEmpty(t *testing.T) {
	got := renderSessionRow(nil)
	if !strings.Contains(got, "no sessions") {
		t.Errorf("renderSessionRow(nil) = %q, want 'no sessions'", got)
	}
}

func TestRenderSessionRowContainsGlyphs(t *testing.T) {
	sessions := []collector.SessionTree{
		{
			RootPID: 1001,
			Descendants: []collector.ProcessInfo{
				{TypeCode: "V"}, // test → ▲
				{TypeCode: "N"}, // run → ●
				{TypeCode: "S"}, // shell → ·
			},
		},
	}
	got := renderSessionRow(sessions)
	testGlyph := ui.CategoryGlyph["test"]
	runGlyph := ui.CategoryGlyph["run"]
	shellGlyph := ui.CategoryGlyph["shell"]
	if !strings.Contains(got, testGlyph) {
		t.Errorf("missing test glyph %q in %q", testGlyph, got)
	}
	if !strings.Contains(got, runGlyph) {
		t.Errorf("missing run glyph %q in %q", runGlyph, got)
	}
	if !strings.Contains(got, shellGlyph) {
		t.Errorf("missing shell glyph %q in %q", shellGlyph, got)
	}
}

func TestRenderSessionRowContainsCount(t *testing.T) {
	sessions := []collector.SessionTree{
		{
			RootPID:     1001,
			Descendants: []collector.ProcessInfo{{TypeCode: "V"}, {TypeCode: "V"}},
		},
	}
	got := renderSessionRow(sessions)
	if !strings.Contains(got, "[02]") {
		t.Errorf("expected [02] count in %q", got)
	}
}

func TestRenderSessionRowGlyphOrder(t *testing.T) {
	sessions := []collector.SessionTree{
		{
			RootPID: 1001,
			Descendants: []collector.ProcessInfo{
				{TypeCode: "S"}, // shell — should render last
				{TypeCode: "V"}, // test — should render first
			},
		},
	}
	got := renderSessionRow(sessions)
	testGlyph := ui.CategoryGlyph["test"]
	shellGlyph := ui.CategoryGlyph["shell"]
	testIdx := strings.Index(got, testGlyph)
	shellIdx := strings.Index(got, shellGlyph)
	if testIdx < 0 || shellIdx < 0 {
		t.Fatalf("missing glyphs in %q", got)
	}
	if testIdx > shellIdx {
		t.Errorf("test glyph (%d) should come before shell glyph (%d)", testIdx, shellIdx)
	}
}

func TestRenderSessionRowEmptySessionCollapses(t *testing.T) {
	sessions := []collector.SessionTree{
		{RootPID: 1001, Descendants: nil}, // no children
	}
	got := renderSessionRow(sessions)
	// Should show dim diamond only, no count like [00]
	if strings.Contains(got, "[00]") {
		t.Errorf("empty session should not show [00] count, got %q", got)
	}
	if !strings.Contains(got, ui.SessionGlyph) {
		t.Errorf("empty session should still show diamond glyph, got %q", got)
	}
}

func TestRenderSessionRowMultipleSessions(t *testing.T) {
	sessions := []collector.SessionTree{
		{RootPID: 1001, Descendants: []collector.ProcessInfo{{TypeCode: "V"}}},
		{RootPID: 1002, Descendants: []collector.ProcessInfo{{TypeCode: "N"}, {TypeCode: "N"}}},
	}
	got := renderSessionRow(sessions)
	if !strings.Contains(got, "[01]") {
		t.Errorf("expected [01] for first session in %q", got)
	}
	if !strings.Contains(got, "[02]") {
		t.Errorf("expected [02] for second session in %q", got)
	}
}

// ── formatFixedCount ─────────────────────────────────────────

func TestFormatFixedCountTwoDigit(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{0, "00"},
		{5, "05"},
		{42, "42"},
		{99, "99"},
		{100, "++"},
	}
	for _, tt := range tests {
		got := formatFixedCount(tt.n)
		if got != tt.want {
			t.Errorf("formatFixedCount(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}
