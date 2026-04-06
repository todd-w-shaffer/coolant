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

// ── renderSessionRow ─────────────────────────────────────────

func TestRenderSessionRowEmptyNoCode(t *testing.T) {
	got := renderSessionRow(nil, false, false)
	// No sessions, no Desktop, no Chrome — should be empty
	if got != "" {
		t.Errorf("renderSessionRow(nil, false, false) = %q, want empty", got)
	}
}

func TestRenderSessionRowEmptyWithDesktop(t *testing.T) {
	got := renderSessionRow(nil, true, true)
	if !strings.Contains(got, "Desktop") || !strings.Contains(got, "Chrome") {
		t.Errorf("expected Desktop and Chrome even with no sessions, got %q", got)
	}
}

func TestRenderSessionRowContainsGlyphs(t *testing.T) {
	sessions := []collector.SessionTree{
		{
			RootPID: 1001,
			Descendants: []collector.ProcessInfo{
				{TypeCode: "N"},  // node → ●
				{TypeCode: "GO"}, // go → ◆
				{TypeCode: "S"},  // shell → ·
			},
		},
	}
	got := renderSessionRow(sessions, false, false)
	nodeGlyph := ui.CategoryGlyph["node"]
	goGlyph := ui.CategoryGlyph["go"]
	shellGlyph := ui.CategoryGlyph["shell"]
	if !strings.Contains(got, nodeGlyph) {
		t.Errorf("missing node glyph %q in %q", nodeGlyph, got)
	}
	if !strings.Contains(got, goGlyph) {
		t.Errorf("missing go glyph %q in %q", goGlyph, got)
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
	got := renderSessionRow(sessions, false, false)
	if !strings.Contains(got, "[02]") {
		t.Errorf("expected [02] count in %q", got)
	}
}

func TestRenderSessionRowGlyphOrder(t *testing.T) {
	sessions := []collector.SessionTree{
		{
			RootPID: 1001,
			Descendants: []collector.ProcessInfo{
				{TypeCode: "N"}, // node — should render after build/shell
				{TypeCode: "T"}, // build — should render first
			},
		},
	}
	got := renderSessionRow(sessions, false, false)
	buildGlyph := ui.CategoryGlyph["build"]
	nodeGlyph := ui.CategoryGlyph["node"]
	buildIdx := strings.Index(got, buildGlyph)
	nodeIdx := strings.Index(got, nodeGlyph)
	if buildIdx < 0 || nodeIdx < 0 {
		t.Fatalf("missing glyphs in %q", got)
	}
	if buildIdx > nodeIdx {
		t.Errorf("build glyph (%d) should come before node glyph (%d)", buildIdx, nodeIdx)
	}
}

func TestRenderSessionRowIdleSessionsHidden(t *testing.T) {
	// Empty sessions should not render individual diamonds
	sessions := []collector.SessionTree{
		{RootPID: 1001, Descendants: nil},
		{RootPID: 1002, Descendants: nil},
		{RootPID: 1003, Descendants: nil},
	}
	got := renderSessionRow(sessions, false, false)
	// Should show "+3" idle count, not three dim diamonds
	if !strings.Contains(got, "+3") {
		t.Errorf("expected +3 idle trailer, got %q", got)
	}
}

func TestRenderSessionRowMixedActiveAndIdle(t *testing.T) {
	sessions := []collector.SessionTree{
		{RootPID: 1001, Descendants: []collector.ProcessInfo{{TypeCode: "V"}}},
		{RootPID: 1002, Descendants: nil},
		{RootPID: 1003, Descendants: nil},
	}
	got := renderSessionRow(sessions, false, false)
	// Active session should show glyphs and count
	if !strings.Contains(got, "[01]") {
		t.Errorf("expected [01] for active session, got %q", got)
	}
	// Two idle sessions should show +2 trailer
	if !strings.Contains(got, "+2") {
		t.Errorf("expected +2 idle trailer, got %q", got)
	}
}

func TestRenderSessionRowAllActiveNoTrailer(t *testing.T) {
	sessions := []collector.SessionTree{
		{RootPID: 1001, Descendants: []collector.ProcessInfo{{TypeCode: "V"}}},
		{RootPID: 1002, Descendants: []collector.ProcessInfo{{TypeCode: "N"}}},
	}
	got := renderSessionRow(sessions, false, false)
	// No idle trailer when all sessions are active
	if strings.Contains(got, "+") {
		t.Errorf("should have no idle trailer when all active, got %q", got)
	}
}

func TestRenderSessionRowMultipleSessions(t *testing.T) {
	sessions := []collector.SessionTree{
		{RootPID: 1001, Descendants: []collector.ProcessInfo{{TypeCode: "V"}}},
		{RootPID: 1002, Descendants: []collector.ProcessInfo{{TypeCode: "N"}, {TypeCode: "N"}}},
	}
	got := renderSessionRow(sessions, false, false)
	if !strings.Contains(got, "[01]") {
		t.Errorf("expected [01] for first session in %q", got)
	}
	if !strings.Contains(got, "[02]") {
		t.Errorf("expected [02] for second session in %q", got)
	}
}

func TestRenderSessionRowDesktopIndicator(t *testing.T) {
	sessions := []collector.SessionTree{
		{RootPID: 1001, Descendants: []collector.ProcessInfo{{TypeCode: "V"}}},
	}
	got := renderSessionRow(sessions, true, false)
	if !strings.Contains(got, "Desktop") {
		t.Errorf("expected Desktop indicator, got %q", got)
	}
	if strings.Contains(got, "Chrome") {
		t.Errorf("should not show Chrome, got %q", got)
	}
}

func TestRenderSessionRowChromeIndicator(t *testing.T) {
	sessions := []collector.SessionTree{
		{RootPID: 1001, Descendants: []collector.ProcessInfo{{TypeCode: "V"}}},
	}
	got := renderSessionRow(sessions, false, true)
	if !strings.Contains(got, "Chrome") {
		t.Errorf("expected Chrome indicator, got %q", got)
	}
	if strings.Contains(got, "Desktop") {
		t.Errorf("should not show Desktop, got %q", got)
	}
}

func TestRenderSessionRowBothIndicators(t *testing.T) {
	sessions := []collector.SessionTree{
		{RootPID: 1001, Descendants: []collector.ProcessInfo{{TypeCode: "V"}}},
	}
	got := renderSessionRow(sessions, true, true)
	if !strings.Contains(got, "Desktop") || !strings.Contains(got, "Chrome") {
		t.Errorf("expected both indicators, got %q", got)
	}
}

func TestRenderSessionRowNoIndicators(t *testing.T) {
	sessions := []collector.SessionTree{
		{RootPID: 1001, Descendants: []collector.ProcessInfo{{TypeCode: "V"}}},
	}
	got := renderSessionRow(sessions, false, false)
	if strings.Contains(got, "Desktop") || strings.Contains(got, "Chrome") {
		t.Errorf("should not show any indicators, got %q", got)
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
