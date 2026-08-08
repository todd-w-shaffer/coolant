package layout

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/toddwshaffer/coolant/thermal/internal/collector"
	"github.com/toddwshaffer/coolant/thermal/internal/stats"
	"github.com/toddwshaffer/coolant/thermal/internal/stats/format"
)

// intelFixture populates a Horizontal with known agent data for intel overlay tests.
func intelFixture(t *testing.T) *Horizontal {
	t.Helper()
	h := newHorizontalForTest(t)
	state := h.State()
	// Use timestamps after epoch (time.Now() at NewAppState init) so
	// completed records pass the epoch gate in HandleEvent.
	t0 := time.Now().Add(time.Millisecond)

	// 3 completed agents: 2 general-purpose, 1 Explore
	state.HandleEvent(collector.GateEvent{
		Event: collector.EventAgentStart, AgentID: "aaa111", AgentType: "general-purpose", Timestamp: t0,
	})
	state.HandleEvent(collector.GateEvent{
		Event: collector.EventAgentStop, AgentID: "aaa111", AgentType: "general-purpose", Timestamp: t0.Add(30 * time.Second),
	})
	state.HandleEvent(collector.GateEvent{
		Event: collector.EventAgentStart, AgentID: "bbb222", AgentType: "general-purpose", Timestamp: t0.Add(time.Minute),
	})
	state.HandleEvent(collector.GateEvent{
		Event: collector.EventAgentStop, AgentID: "bbb222", AgentType: "general-purpose", Timestamp: t0.Add(time.Minute + 45*time.Second),
	})
	state.HandleEvent(collector.GateEvent{
		Event: collector.EventAgentStart, AgentID: "ccc333", AgentType: "Explore", Timestamp: t0.Add(2 * time.Minute),
	})
	state.HandleEvent(collector.GateEvent{
		Event: collector.EventAgentStop, AgentID: "ccc333", AgentType: "Explore", Timestamp: t0.Add(2*time.Minute + 63*time.Second),
	})

	// 2 active agents
	state.HandleEvent(collector.GateEvent{
		Event: collector.EventAgentStart, AgentID: "ddd444", AgentType: "general-purpose", Timestamp: t0.Add(5 * time.Minute),
	})
	state.HandleEvent(collector.GateEvent{
		Event: collector.EventAgentStart, AgentID: "eee555", AgentType: "Plan", Timestamp: t0.Add(6 * time.Minute),
	})

	// 10 gate caps
	for i := 0; i < 10; i++ {
		state.HandleEvent(collector.GateEvent{
			Event: collector.EventGateCap, Command: "vitest", Rewritten: "vitest --maxConcurrency=2",
			Timestamp: t0.Add(time.Duration(i) * time.Second),
		})
	}

	return h
}

func TestIntelViewContentBaseline(t *testing.T) {
	h := intelFixture(t)
	lines := h.intelView()

	if len(lines) != 5 {
		t.Fatalf("intelView returned %d lines, want 5", len(lines))
	}

	// Strip ANSI for content assertions
	stripped := make([]string, len(lines))
	for i, l := range lines {
		stripped[i] = ansi.Strip(l)
	}
	combined := strings.Join(stripped, "\n")

	checks := []string{
		"2 active", "3 completed", "peak",
		"general-purpose", "Explore",
		"avg", "longest",
		"10 throttled", "0 blocked",
		"0 orphans",
	}
	for _, want := range checks {
		if !strings.Contains(combined, want) {
			t.Errorf("intelView missing %q\nfull:\n%s", want, combined)
		}
	}
}

func TestToggleIntelEntersMode(t *testing.T) {
	h := newHorizontalForTest(t)
	h.ToggleIntel()
	if !h.IntelMode() {
		t.Error("ToggleIntel should enter intel mode")
	}
}

func TestDismissIntelReturnsToNormal(t *testing.T) {
	h := newHorizontalForTest(t)
	h.ToggleIntel()
	h.DismissIntel()
	if h.IntelMode() {
		t.Error("DismissIntel should exit intel mode")
	}
}

func TestIntelDismissesHelp(t *testing.T) {
	h := newHorizontalForTest(t)
	h.ToggleHelp()  // enter help
	h.ToggleIntel() // should dismiss help
	if h.HelpMode() != HelpShort {
		t.Error("ToggleIntel should dismiss help")
	}
	if !h.IntelMode() {
		t.Error("ToggleIntel should enter intel mode")
	}
}

func TestHelpDismissesIntel(t *testing.T) {
	h := newHorizontalForTest(t)
	h.ToggleIntel() // enter intel
	h.ToggleHelp()  // should dismiss intel
	if h.IntelMode() {
		t.Error("ToggleHelp should dismiss intel")
	}
	if h.HelpMode() != HelpFull {
		t.Error("ToggleHelp should enter help mode")
	}
}

func TestActiveViewRendersIntelOverlay(t *testing.T) {
	h := intelFixture(t)
	h.SetSize(120, 10)
	// Feed a snapshot so the view isn't blank
	snap := collector.Snapshot{
		Online:   true,
		Sessions: []collector.SessionTree{{RootPID: 1}},
		System:   collector.SystemStats{NCPUs: 10, MemTotalBytes: 16 * 1024 * 1024 * 1024},
	}
	h.State().Update(snap)
	h.Update(h.State())

	h.ToggleIntel()
	output := h.View()
	stripped := ansi.Strip(output)

	if !strings.Contains(stripped, "completed") {
		t.Error("intel overlay should contain 'completed' in active view")
	}
	if !strings.Contains(stripped, "throttled") {
		t.Error("intel overlay should contain 'throttled' in active view")
	}
}

func TestActiveViewIntelTakesPriorityOverHelp(t *testing.T) {
	h := intelFixture(t)
	h.SetSize(120, 10)
	snap := collector.Snapshot{
		Online:   true,
		Sessions: []collector.SessionTree{{RootPID: 1}},
		System:   collector.SystemStats{NCPUs: 10, MemTotalBytes: 16 * 1024 * 1024 * 1024},
	}
	h.State().Update(snap)
	h.Update(h.State())

	// Set both modes (shouldn't happen via UI, but test priority)
	h.ToggleHelp()
	h.ToggleIntel() // this dismisses help, but let's test the render priority
	// Force both on for the edge case
	h.ToggleHelp() // re-enter help while intel is on — ToggleHelp dismisses intel
	// So test the proper way: intel should NOT show when help just dismissed it
	if h.IntelMode() {
		// This confirms mutual exclusion works. The real priority test is:
		// when intel is on, the view shows intel content, not help content.
	}
	// Fresh test: intel on, help off
	h.ToggleIntel()
	output := h.View()
	stripped := ansi.Strip(output)

	if !strings.Contains(stripped, "completed") {
		t.Error("intel overlay should show intel content when active")
	}
	// Help-specific content should NOT appear
	if strings.Contains(stripped, "press any key to dismiss") {
		t.Error("help overlay content should not appear when intel is active")
	}
}

func TestHelpViewContainsIntelScoreboard(t *testing.T) {
	h := newHorizontalForTest(t)
	lines := h.helpView()
	combined := ""
	for _, l := range lines {
		combined += ansi.Strip(l) + " "
	}
	if !strings.Contains(combined, "intel · scoreboard") {
		t.Error("helpView should describe the i binding as 'intel · scoreboard' so the page cycle is discoverable")
	}
}

func TestSessionPageCarriesScoreboardHint(t *testing.T) {
	h := intelFixture(t)
	lines := h.intelView()
	// The hint rides inline on an existing row — the session page
	// keeps its 5-row shape (locked in the scoreboard spec §0).
	if len(lines) != 5 {
		t.Fatalf("session page should stay 5 rows, got %d", len(lines))
	}
	combined := ""
	for _, l := range lines {
		combined += ansi.Strip(l) + "\n"
	}
	if !strings.Contains(combined, "i scoreboard") {
		t.Errorf("session page should hint at the scoreboard page\nfull:\n%s", combined)
	}
}

func TestIntelDismissedOnIdleTransition(t *testing.T) {
	h := newHorizontalForTest(t)
	h.SetSize(120, 10)
	// Push a non-idle snapshot
	snap := collector.Snapshot{
		Online:   true,
		Sessions: []collector.SessionTree{{RootPID: 1}},
		System:   collector.SystemStats{NCPUs: 10, MemTotalBytes: 16 * 1024 * 1024 * 1024},
	}
	h.State().Update(snap)
	h.Update(h.State())

	h.ToggleIntel()
	if !h.IntelMode() {
		t.Fatal("precondition: intel should be on")
	}

	// Push idle snapshot (no sessions)
	idleSnap := collector.Snapshot{Online: true, System: snap.System}
	h.State().Update(idleSnap)
	h.Update(h.State())

	if h.IntelMode() {
		t.Error("intel should auto-dismiss on idle transition")
	}
}

func TestIntelViewTypeTruncation(t *testing.T) {
	h := newHorizontalForTest(t)
	state := h.State()
	t0 := time.Now().Add(time.Millisecond)

	// Create 6 different agent types — should show top 4 + "N other"
	types := []string{"general-purpose", "Explore", "Plan", "code-reviewer", "build-validator", "test-runner"}
	for i, typ := range types {
		id := fmt.Sprintf("agent-%d", i)
		state.HandleEvent(collector.GateEvent{
			Event: collector.EventAgentStart, AgentID: id, AgentType: typ,
			Timestamp: t0.Add(time.Duration(i) * time.Second),
		})
		state.HandleEvent(collector.GateEvent{
			Event: collector.EventAgentStop, AgentID: id, AgentType: typ,
			Timestamp: t0.Add(time.Duration(i)*time.Second + 10*time.Second),
		})
	}

	lines := h.intelView()
	typesRow := ansi.Strip(lines[1])

	if !strings.Contains(typesRow, "other") {
		t.Errorf("types row should contain 'other' for 6+ types, got: %q", typesRow)
	}
}

func TestRenderedAgentIDsProxy(t *testing.T) {
	h := intelFixture(t)
	h.SetSize(120, 10)
	h.SetHighScoreMode(true)
	snap := collector.Snapshot{
		Online:   true,
		Sessions: []collector.SessionTree{{RootPID: 1}},
		System:   collector.SystemStats{NCPUs: 10, MemTotalBytes: 16 * 1024 * 1024 * 1024},
	}
	h.State().Update(snap)
	h.Update(h.State())

	// Force a render so ViewLines populates the cache.
	h.View()

	ids := h.RenderedAgentIDs()
	// intelFixture creates 3 completed agents: aaa111, bbb222, ccc333
	if len(ids) != 3 {
		t.Fatalf("RenderedAgentIDs len = %d, want 3", len(ids))
	}
	// Newest-last ordering: aaa111, bbb222, ccc333
	want := []string{"aaa111", "bbb222", "ccc333"}
	for i, id := range ids {
		if id != want[i] {
			t.Errorf("RenderedAgentIDs[%d] = %q, want %q", i, id, want[i])
		}
	}
}

// ── Focused intel sub-mode ────────────────────────────────────

func TestFocusAgentEntersIntelMode(t *testing.T) {
	h := intelFixture(t)
	h.FocusAgent("aaa111")
	if !h.IntelMode() {
		t.Error("FocusAgent should enter intel mode")
	}
	if h.FocusedAgentID() != "aaa111" {
		t.Errorf("FocusedAgentID = %q, want %q", h.FocusedAgentID(), "aaa111")
	}
}

func TestFocusAgentFromSessionSummary(t *testing.T) {
	h := intelFixture(t)
	h.ToggleIntel() // enter session summary
	h.FocusAgent("bbb222")
	if h.FocusedAgentID() != "bbb222" {
		t.Errorf("FocusedAgentID = %q, want %q", h.FocusedAgentID(), "bbb222")
	}
	if !h.IntelMode() {
		t.Error("should still be in intel mode")
	}
}

func TestToggleIntelFromFocusedClearsFocus(t *testing.T) {
	h := intelFixture(t)
	h.FocusAgent("aaa111")
	h.ToggleIntel() // should clear focus, keep intel (session summary)
	if h.FocusedAgentID() != "" {
		t.Errorf("ToggleIntel from focused should clear focusedAgentID, got %q", h.FocusedAgentID())
	}
	if !h.IntelMode() {
		t.Error("should still be in intel mode (session summary)")
	}
}

func TestToggleIntelCyclesSessionScoreboardOff(t *testing.T) {
	h := intelFixture(t)
	h.ToggleIntel() // off → session summary
	if !h.IntelMode() || h.intelPage != intelPageSession {
		t.Fatalf("first ToggleIntel: mode=%v page=%d, want session page", h.IntelMode(), h.intelPage)
	}
	h.ToggleIntel() // session summary → scoreboard
	if !h.IntelMode() || h.intelPage != intelPageScoreboard {
		t.Fatalf("second ToggleIntel: mode=%v page=%d, want scoreboard page", h.IntelMode(), h.intelPage)
	}
	h.ToggleIntel() // scoreboard → off
	if h.IntelMode() {
		t.Error("third ToggleIntel from scoreboard should exit intel")
	}
	if h.intelPage != intelPageSession {
		t.Errorf("exit should reset page to session, got %d", h.intelPage)
	}
}

func TestToggleIntelFromFocusedLandsOnSessionPage(t *testing.T) {
	h := intelFixture(t)
	h.ToggleIntel() // session
	h.ToggleIntel() // scoreboard
	h.FocusAgent("aaa111")
	h.ToggleIntel() // focused → session summary, never scoreboard
	if h.FocusedAgentID() != "" {
		t.Errorf("ToggleIntel from focused should clear focus, got %q", h.FocusedAgentID())
	}
	if !h.IntelMode() || h.intelPage != intelPageSession {
		t.Errorf("ToggleIntel from focused should land on session page, mode=%v page=%d", h.IntelMode(), h.intelPage)
	}
}

func TestDismissIntelResetsPage(t *testing.T) {
	h := intelFixture(t)
	h.ToggleIntel() // session
	h.ToggleIntel() // scoreboard
	h.DismissIntel()
	if h.intelPage != intelPageSession {
		t.Errorf("DismissIntel should reset page to session, got %d", h.intelPage)
	}
}

func TestIdleTransitionResetsPage(t *testing.T) {
	h := intelFixture(t)
	h.SetSize(120, 10)
	snap := collector.Snapshot{
		Online:   true,
		Sessions: []collector.SessionTree{{RootPID: 1}},
		System:   collector.SystemStats{NCPUs: 10, MemTotalBytes: 16 * 1024 * 1024 * 1024},
	}
	h.State().Update(snap)
	h.Update(h.State())

	h.ToggleIntel() // session
	h.ToggleIntel() // scoreboard

	idleSnap := collector.Snapshot{Online: true, System: snap.System}
	h.State().Update(idleSnap)
	h.Update(h.State())

	if h.intelPage != intelPageSession {
		t.Errorf("idle transition should reset page to session, got %d", h.intelPage)
	}
}

func TestDismissIntelClearsFocus(t *testing.T) {
	h := intelFixture(t)
	h.FocusAgent("aaa111")
	h.DismissIntel()
	if h.FocusedAgentID() != "" {
		t.Errorf("DismissIntel should clear focusedAgentID, got %q", h.FocusedAgentID())
	}
	if h.IntelMode() {
		t.Error("DismissIntel should exit intel mode")
	}
}

func TestToggleHelpClearsFocus(t *testing.T) {
	h := intelFixture(t)
	h.FocusAgent("aaa111")
	h.ToggleHelp() // should dismiss intel + clear focus
	if h.FocusedAgentID() != "" {
		t.Errorf("ToggleHelp should clear focusedAgentID, got %q", h.FocusedAgentID())
	}
}

func TestIdleTransitionClearsFocus(t *testing.T) {
	h := intelFixture(t)
	h.SetSize(120, 10)
	snap := collector.Snapshot{
		Online:   true,
		Sessions: []collector.SessionTree{{RootPID: 1}},
		System:   collector.SystemStats{NCPUs: 10, MemTotalBytes: 16 * 1024 * 1024 * 1024},
	}
	h.State().Update(snap)
	h.Update(h.State())

	h.FocusAgent("aaa111")
	// Idle transition
	idleSnap := collector.Snapshot{Online: true, System: snap.System}
	h.State().Update(idleSnap)
	h.Update(h.State())

	if h.FocusedAgentID() != "" {
		t.Errorf("idle transition should clear focusedAgentID, got %q", h.FocusedAgentID())
	}
}

func TestFocusedIntelViewRendersAgentRecord(t *testing.T) {
	h := intelFixture(t)
	h.FocusAgent("aaa111")
	lines := h.intelView()

	stripped := make([]string, len(lines))
	for i, l := range lines {
		stripped[i] = ansi.Strip(l)
	}
	combined := strings.Join(stripped, "\n")

	checks := []string{"aaa111", "general-purpose", "30s"}
	for _, want := range checks {
		if !strings.Contains(combined, want) {
			t.Errorf("focused intelView missing %q\nfull:\n%s", want, combined)
		}
	}
}

func TestFocusedIntelTranscriptLabelHasZoneMark(t *testing.T) {
	h := newHorizontalForTest(t)
	state := h.State()
	t0 := time.Now().Add(time.Millisecond)
	state.HandleEvent(collector.GateEvent{
		Event: collector.EventAgentStart, AgentID: "zone1", AgentType: "general-purpose",
		Timestamp: t0,
	})
	state.HandleEvent(collector.GateEvent{
		Event: collector.EventAgentStop, AgentID: "zone1", AgentType: "general-purpose",
		TranscriptPath: "/tmp/coolant-test/transcript.jsonl",
		Timestamp:      t0.Add(10 * time.Second),
	})

	h.FocusAgent("zone1")
	lines := h.intelView()
	// The size row (index 2) must contain bubblezone markers (\x1b[...z)
	// which inflate len() beyond ansi.StringWidth — the telltale of zone.Mark.
	sizeRow := lines[2]
	visWidth := ansi.StringWidth(sizeRow)
	rawLen := len(sizeRow)
	if rawLen <= visWidth {
		t.Errorf("size row should contain zone markers (len=%d should exceed visWidth=%d)", rawLen, visWidth)
	}
	// Visible text should include "transcript" as the click target
	if !strings.Contains(ansi.Strip(sizeRow), "transcript") {
		t.Error("size row should contain 'transcript' label")
	}
}

func TestFocusedTranscriptPath(t *testing.T) {
	h := newHorizontalForTest(t)
	state := h.State()
	t0 := time.Now().Add(time.Millisecond)
	state.HandleEvent(collector.GateEvent{
		Event: collector.EventAgentStart, AgentID: "tp1", AgentType: "general-purpose",
		Timestamp: t0,
	})
	state.HandleEvent(collector.GateEvent{
		Event: collector.EventAgentStop, AgentID: "tp1", AgentType: "general-purpose",
		TranscriptPath: "/tmp/test/transcript.jsonl",
		Timestamp:      t0.Add(5 * time.Second),
	})

	// Not focused — should return empty
	if got := h.FocusedTranscriptPath(); got != "" {
		t.Errorf("FocusedTranscriptPath without focus = %q, want empty", got)
	}

	// Focus — should return the path
	h.FocusAgent("tp1")
	if got := h.FocusedTranscriptPath(); got != "/tmp/test/transcript.jsonl" {
		t.Errorf("FocusedTranscriptPath = %q, want %q", got, "/tmp/test/transcript.jsonl")
	}

	// Dismiss — should return empty again
	h.DismissIntel()
	if got := h.FocusedTranscriptPath(); got != "" {
		t.Errorf("FocusedTranscriptPath after dismiss = %q, want empty", got)
	}
}

func TestFocusedIntelNoZoneMarkWithoutAbsolutePath(t *testing.T) {
	h := newHorizontalForTest(t)
	state := h.State()
	t0 := time.Now().Add(time.Millisecond)
	state.HandleEvent(collector.GateEvent{
		Event: collector.EventAgentStart, AgentID: "rel1", AgentType: "general-purpose",
		Timestamp: t0,
	})
	state.HandleEvent(collector.GateEvent{
		Event: collector.EventAgentStop, AgentID: "rel1", AgentType: "general-purpose",
		TranscriptPath: "relative/path.jsonl",
		Timestamp:      t0.Add(5 * time.Second),
	})

	h.FocusAgent("rel1")
	lines := h.intelView()
	// Size row should NOT have zone markers for relative paths
	sizeRow := lines[2]
	visWidth := ansi.StringWidth(sizeRow)
	rawLen := len(sizeRow)
	if rawLen > visWidth+50 {
		t.Errorf("size row should not contain zone markers for relative path (len=%d, visWidth=%d)", rawLen, visWidth)
	}
}

func TestFocusedIntelNilGuardFallsBack(t *testing.T) {
	h := newHorizontalForTest(t)
	// Focus on a non-existent agent
	h.FocusAgent("nonexistent")
	lines := h.intelView()
	// Should fall back to session summary (not panic or show empty)
	if h.FocusedAgentID() != "" {
		t.Error("nil-guard should have cleared focusedAgentID")
	}
	if len(lines) == 0 {
		t.Error("fallback should produce non-empty view")
	}
}

func TestFocusedIntelPurgedRecord(t *testing.T) {
	h := newHorizontalForTest(t)
	state := h.State()
	// Start an agent, then fire counter.reset — the active record gets
	// flushed into completed with Purged: true.
	t0 := time.Now()
	state.HandleEvent(collector.GateEvent{
		Event: collector.EventAgentStart, AgentID: "purge1", AgentType: "general-purpose", Timestamp: t0,
	})
	state.HandleEvent(collector.GateEvent{
		Event: collector.EventCounterReset, Timestamp: t0,
	})

	h.FocusAgent("purge1")
	lines := h.intelView()
	stripped := make([]string, len(lines))
	for i, l := range lines {
		stripped[i] = ansi.Strip(l)
	}
	combined := strings.Join(stripped, "\n")
	if !strings.Contains(combined, "purged") {
		t.Errorf("purged record should show 'purged' label\nfull:\n%s", combined)
	}
}

func TestFocusedIntelOrphanRecord(t *testing.T) {
	h := newHorizontalForTest(t)
	state := h.State()
	t0 := time.Now().Add(time.Millisecond)
	// Create an orphan (stop with no matching start)
	state.HandleEvent(collector.GateEvent{
		Event: collector.EventAgentStop, AgentID: "orphan1", AgentType: "Explore", Timestamp: t0,
	})

	h.FocusAgent("orphan1")
	lines := h.intelView()
	stripped := make([]string, len(lines))
	for i, l := range lines {
		stripped[i] = ansi.Strip(l)
	}
	combined := strings.Join(stripped, "\n")
	if !strings.Contains(combined, "orphan") {
		t.Errorf("orphan record should show 'orphan' label\nfull:\n%s", combined)
	}
}

func TestFormatUptime(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{0, "0m00s"},
		{30 * time.Second, "0m30s"},
		{59*time.Minute + 59*time.Second, "59m59s"},
		{time.Hour, "1h00m"},
		{2*time.Hour + 14*time.Minute, "2h14m"},
	}
	for _, tt := range tests {
		if got := formatUptime(tt.d); got != tt.want {
			t.Errorf("formatUptime(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{0, "0s"},
		{30 * time.Second, "30s"},
		{59 * time.Second, "59s"},
		{60 * time.Second, "1m0s"},
		{90 * time.Second, "1m30s"},
	}
	for _, tt := range tests {
		if got := formatDuration(tt.d); got != tt.want {
			t.Errorf("formatDuration(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

func TestFormatBytesCompact(t *testing.T) {
	tests := []struct {
		b    int64
		want string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1024 * 1024, "1.0 MB"},
		{2516582, "2.4 MB"},
	}
	for _, tt := range tests {
		if got := formatBytesCompact(tt.b); got != tt.want {
			t.Errorf("formatBytesCompact(%d) = %q, want %q", tt.b, got, tt.want)
		}
	}
}

func TestIntelViewEmptyState(t *testing.T) {
	h := newHorizontalForTest(t)
	lines := h.intelView()

	if len(lines) != 5 {
		t.Fatalf("intelView returned %d lines, want 5", len(lines))
	}

	stripped := make([]string, len(lines))
	for i, l := range lines {
		stripped[i] = ansi.Strip(l)
	}
	combined := strings.Join(stripped, "\n")

	checks := []string{
		"0 active", "0 completed",
		"no completions yet",
		"0 throttled",
	}
	for _, want := range checks {
		if !strings.Contains(combined, want) {
			t.Errorf("empty intelView missing %q\nfull:\n%s", want, combined)
		}
	}
}

// ── Scoreboard page: cache + pull discipline ──────────────────

// countingStatsSource is a statsSource fake that counts every method
// call so tests can assert the pull discipline (once per page entry,
// zero calls from the steady-state render path).
type countingStatsSource struct {
	calls int
	snap  stats.Snapshot
}

func (c *countingStatsSource) Snapshot() stats.Snapshot { c.calls++; return c.snap }
func (c *countingStatsSource) VisibleWindows() []string { c.calls++; return []string{"7d", "alltime"} }
func (c *countingStatsSource) Window(days int) stats.Counters {
	c.calls++
	return stats.Counters{}
}
func (c *countingStatsSource) WindowByType(days int) map[string]int64 {
	c.calls++
	return map[string]int64{}
}
func (c *countingStatsSource) WindowByProject(days int) map[string]int64 {
	c.calls++
	return map[string]int64{}
}

func newCountingSource() *countingStatsSource {
	return &countingStatsSource{snap: stats.Snapshot{FirstSeen: time.Now().UTC().Add(-time.Hour)}}
}

func enterScoreboard(h *Horizontal) {
	h.ToggleIntel() // off → session
	h.ToggleIntel() // session → scoreboard
}

func TestScoreboardCachePulledOncePerEntry(t *testing.T) {
	h := newHorizontalForTest(t)
	src := newCountingSource()
	h.scoreboardSrc = src

	enterScoreboard(h)
	afterEntry := src.calls
	if afterEntry == 0 {
		t.Fatal("entering the scoreboard page should pull the cache")
	}

	for i := 0; i < 5; i++ {
		h.intelView()
	}
	if src.calls != afterEntry {
		t.Errorf("steady-state renders must not touch the source: %d calls after entry, %d after 5 renders", afterEntry, src.calls)
	}

	// Cycle out and back in — re-entry re-pulls.
	h.ToggleIntel() // scoreboard → off
	enterScoreboard(h)
	if src.calls <= afterEntry {
		t.Error("re-entering the scoreboard page should re-pull the cache")
	}
}

func TestScoreboardCacheDayRolloverRepulls(t *testing.T) {
	h := newHorizontalForTest(t)
	src := newCountingSource()
	h.scoreboardSrc = src

	enterScoreboard(h)
	h.intelView()
	before := src.calls

	// Simulate a UTC day rollover since the pull.
	h.sbCache.pulledAt = h.sbCache.pulledAt.Add(-24 * time.Hour)
	h.intelView()
	if src.calls <= before {
		t.Error("render after UTC day rollover should re-pull the cache")
	}
}

func TestDismissIntelZeroesScoreboardCache(t *testing.T) {
	h := newHorizontalForTest(t)
	src := newCountingSource()
	h.scoreboardSrc = src

	enterScoreboard(h)
	if h.sbCache.pulledAt.IsZero() {
		t.Fatal("precondition: cache should be populated after entry")
	}
	h.DismissIntel()
	if !h.sbCache.pulledAt.IsZero() {
		t.Error("DismissIntel should zero the scoreboard cache")
	}
}

func TestScoreboardWindowKeysCached(t *testing.T) {
	h := newHorizontalForTest(t)
	src := newCountingSource()
	h.scoreboardSrc = src

	enterScoreboard(h)
	want := []string{"today", "7d", "alltime"}
	if len(h.sbCache.windowKeys) != len(want) {
		t.Fatalf("windowKeys = %v, want %v", h.sbCache.windowKeys, want)
	}
	for i, k := range want {
		if h.sbCache.windowKeys[i] != k {
			t.Errorf("windowKeys[%d] = %q, want %q", i, h.sbCache.windowKeys[i], k)
		}
	}
	for _, k := range want {
		if _, ok := h.sbCache.windows[k]; !ok {
			t.Errorf("windows map missing cached counters for %q", k)
		}
	}
}

// ── Scoreboard page: band rendering ───────────────────────────

// scoreboardAggFixture folds Schema:1 events into a fresh aggregator.
// Timestamps sit within the last hour so the install age pins to the
// youngest tier — VisibleWindows() = ["7d","alltime"] — keeping
// window-label assertions stable as the test suite ages.
func scoreboardAggFixture(t *testing.T) *stats.Aggregator {
	t.Helper()
	agg := stats.New(stats.Config{})
	t0 := time.Now().UTC().Add(-30 * time.Minute)
	agg.Fold(collector.GateEvent{
		Schema: 1, Event: collector.EventAgentStart, AgentID: "a1", AgentType: "general-purpose",
		Project: "coolant", SessionID: "sess1", Timestamp: t0,
	}, 0)
	agg.Fold(collector.GateEvent{
		Schema: 1, Event: collector.EventAgentStart, AgentID: "a2", AgentType: "Explore",
		Project: "coolant", SessionID: "sess1", Timestamp: t0.Add(time.Second),
	}, 0)
	agg.Fold(collector.GateEvent{
		Schema: 1, Event: collector.EventAgentStop, AgentID: "a1", AgentType: "general-purpose",
		Project: "coolant", SessionID: "sess1", TokensIn: 1200, TokensOut: 3400, ToolCallCount: 12,
		Timestamp: t0.Add(5 * time.Minute),
	}, 0)
	agg.Fold(collector.GateEvent{
		Schema: 1, Event: collector.EventAgentStop, AgentID: "a2", AgentType: "Explore",
		Project: "coolant", SessionID: "sess1", Timestamp: t0.Add(7 * time.Minute),
	}, 0)
	// Three more sequential agents: a second general-purpose keeps it
	// on top of the by_type sort, and 4 distinct types overflow the
	// top-3 distributions cut so "(N more)" renders.
	for i, typ := range []string{"general-purpose", "Plan", "code-reviewer"} {
		id := fmt.Sprintf("a%d", i+3)
		start := t0.Add(time.Duration(8+2*i) * time.Minute)
		agg.Fold(collector.GateEvent{
			Schema: 1, Event: collector.EventAgentStart, AgentID: id, AgentType: typ,
			Project: "coolant", SessionID: "sess1", Timestamp: start,
		}, 0)
		agg.Fold(collector.GateEvent{
			Schema: 1, Event: collector.EventAgentStop, AgentID: id, AgentType: typ,
			Project: "coolant", SessionID: "sess1", Timestamp: start.Add(time.Minute),
		}, 0)
	}
	return agg
}

func scoreboardStripped(t *testing.T, h *Horizontal) (lines []string, combined string) {
	t.Helper()
	lines = h.intelView()
	stripped := make([]string, len(lines))
	for i, l := range lines {
		stripped[i] = ansi.Strip(l)
	}
	return lines, strings.Join(stripped, "\n")
}

func TestScoreboardPageRendersAllGroups(t *testing.T) {
	h := newHorizontalForTest(t)
	h.SetSize(240, 10)
	h.State().AttachAggregator(scoreboardAggFixture(t))
	enterScoreboard(h)
	lines, combined := scoreboardStripped(t, h)

	// Band must never exceed the gauge-row canvas (overlayContent
	// silently truncates beyond it).
	if len(lines) > 6 {
		t.Fatalf("scoreboard band is %d rows, must not exceed 6", len(lines))
	}
	if !strings.Contains(combined, "scoreboard") {
		t.Errorf("title row missing 'scoreboard'\nfull:\n%s", combined)
	}
	// Records group: shared board vocabulary, all boards present.
	for _, b := range format.Boards() {
		if !strings.Contains(combined, b.Label) {
			t.Errorf("records group missing board %q\nfull:\n%s", b.Label, combined)
		}
	}
	if !strings.Contains(combined, format.BurstBoardLabel) {
		t.Errorf("records group missing burst board\nfull:\n%s", combined)
	}
	// Windows group: one row per cached key, labels via the shared helper.
	if len(h.sbCache.windowKeys) == 0 {
		t.Fatal("cache should carry window keys")
	}
	for _, k := range h.sbCache.windowKeys {
		if !strings.Contains(combined, format.FormatWindowLabel(k)) {
			t.Errorf("windows group missing label for %q\nfull:\n%s", k, combined)
		}
	}
	// Windows group: counter values dispatch to the right source calls
	// — the fixture folds exactly 5 starts and 5 stops today.
	if !strings.Contains(combined, "5 started · 5 completed") {
		t.Errorf("windows group missing today's folded counter values\nfull:\n%s", combined)
	}
	// Distributions group: data keys present, and the 4th type
	// overflows the top-3 cut into the overflow indicator.
	for _, want := range []string{"general-purpose", "Explore", "coolant", "(1 more)"} {
		if !strings.Contains(combined, want) {
			t.Errorf("distributions group missing %q\nfull:\n%s", want, combined)
		}
	}
}

func TestScoreboardOldestTierBandFitsCanvas(t *testing.T) {
	// The >=90-day tier is the max-content shape: 5 window rows
	// (today · 7d · 30d · 90d · lifetime). The band must still fit the
	// 6-row gauge canvas — overlayContent truncates silently beyond it.
	h := newHorizontalForTest(t)
	h.SetSize(300, 10)
	agg := stats.New(stats.Config{})
	old := time.Now().UTC().Add(-100 * 24 * time.Hour)
	agg.Fold(collector.GateEvent{
		Schema: 1, Event: collector.EventAgentStart, AgentID: "old1", AgentType: "general-purpose",
		Project: "coolant", SessionID: "oldsess", Timestamp: old,
	}, 0)
	agg.Fold(collector.GateEvent{
		Schema: 1, Event: collector.EventAgentStop, AgentID: "old1", AgentType: "general-purpose",
		Project: "coolant", SessionID: "oldsess", Timestamp: old.Add(time.Minute),
	}, 0)
	h.State().AttachAggregator(agg)
	enterScoreboard(h)
	lines, combined := scoreboardStripped(t, h)

	if len(h.sbCache.windowKeys) != 5 {
		t.Fatalf("oldest tier should cache 5 window keys, got %v", h.sbCache.windowKeys)
	}
	if len(lines) > 6 {
		t.Fatalf("max-content band is %d rows, must not exceed the 6-row canvas:\n%s", len(lines), combined)
	}
	for _, k := range h.sbCache.windowKeys {
		if !strings.Contains(combined, format.FormatWindowLabel(k)) {
			t.Errorf("band missing window row %q\nfull:\n%s", k, combined)
		}
	}
}

func TestScoreboardNilAggregatorFallback(t *testing.T) {
	h := newHorizontalForTest(t) // NewAppState starts with no aggregator
	h.SetSize(240, 10)
	enterScoreboard(h)
	lines, combined := scoreboardStripped(t, h)
	if len(lines) != 1 {
		t.Fatalf("nil-aggregator page should be a single line, got %d", len(lines))
	}
	if !strings.Contains(combined, "stats unavailable") {
		t.Errorf("nil-aggregator page missing fallback copy, got: %q", combined)
	}
}

func TestScoreboardFirstSeenZeroFallback(t *testing.T) {
	h := newHorizontalForTest(t)
	h.SetSize(240, 10)
	h.State().AttachAggregator(stats.New(stats.Config{})) // attached, nothing folded
	enterScoreboard(h)
	lines, combined := scoreboardStripped(t, h)
	if len(lines) != 1 {
		t.Fatalf("FirstSeen-zero page should be a single line, got %d", len(lines))
	}
	if !strings.Contains(combined, "no agent activity") {
		t.Errorf("FirstSeen-zero page missing neutral copy, got: %q", combined)
	}
	// A fresh healthy install is also FirstSeen-zero — the upgrade
	// hint is CLI-only and must never appear here (§3.5).
	if strings.Contains(combined, "install.sh") || strings.Contains(combined, "upgrade") {
		t.Errorf("FirstSeen-zero page must not carry upgrade copy, got: %q", combined)
	}
}

func TestScoreboardEmptyBoardsRenderDash(t *testing.T) {
	h := newHorizontalForTest(t)
	h.SetSize(240, 10)
	agg := stats.New(stats.Config{})
	// One tokenless agent: MostTokensAgent / MostToolCallsAgent stay
	// empty (zero values are excluded from leaderboards).
	t0 := time.Now().UTC().Add(-10 * time.Minute)
	agg.Fold(collector.GateEvent{
		Schema: 1, Event: collector.EventAgentStart, AgentID: "b1", AgentType: "claude",
		SessionID: "sess2", Timestamp: t0,
	}, 0)
	agg.Fold(collector.GateEvent{
		Schema: 1, Event: collector.EventAgentStop, AgentID: "b1", AgentType: "claude",
		SessionID: "sess2", Timestamp: t0.Add(time.Minute),
	}, 0)
	h.State().AttachAggregator(agg)
	enterScoreboard(h)
	_, combined := scoreboardStripped(t, h)

	if !strings.Contains(combined, "most tokens (agent)") {
		t.Fatalf("empty board should still render its label\nfull:\n%s", combined)
	}
	if !strings.Contains(combined, "—") {
		t.Errorf("empty boards should render the dash glyph\nfull:\n%s", combined)
	}
	// No per-board "(no records yet)" strings on the overlay (§0).
	if strings.Contains(combined, "no records yet") {
		t.Errorf("overlay must not use the CLI's '(no records yet)' copy\nfull:\n%s", combined)
	}
}

// ── Scoreboard page: width adaptation ─────────────────────────

func TestScoreboardWidthDropOrder(t *testing.T) {
	h := newHorizontalForTest(t)
	h.State().AttachAggregator(scoreboardAggFixture(t))
	hasDist := func(s string) bool { return strings.Contains(s, "by type") }
	hasWindows := func(s string) bool {
		return strings.Contains(s, format.FormatWindowLabel("today"))
	}
	hasRecords := func(s string) bool { return strings.Contains(s, "peak concurrent") }

	h.SetSize(240, 10)
	enterScoreboard(h)
	_, wide := scoreboardStripped(t, h)
	if !hasDist(wide) || !hasWindows(wide) || !hasRecords(wide) {
		t.Fatalf("all three groups should render at 240 cols:\n%s", wide)
	}

	// Sweep narrower: groups must drop in order (distributions first,
	// then windows) and never reappear as width shrinks. Records
	// survive down to the narrow floor. Exact breakpoints are
	// implementation-tuned — the sweep asserts order, not columns.
	sawRecordsOnly := false
	prevDist, prevWindows := true, true
	for w := 239; w >= scoreboardMinWidth; w-- {
		h.SetSize(w, 10)
		_, s := scoreboardStripped(t, h)
		d, wn := hasDist(s), hasWindows(s)
		if d && !prevDist {
			t.Fatalf("distributions reappeared at width %d", w)
		}
		if wn && !prevWindows {
			t.Fatalf("windows reappeared at width %d", w)
		}
		if d && !wn {
			t.Fatalf("wrong drop order at width %d: distributions present without windows", w)
		}
		if !hasRecords(s) {
			t.Fatalf("records must survive above the narrow floor, missing at width %d:\n%s", w, s)
		}
		if !d && !wn {
			sawRecordsOnly = true
		}
		prevDist, prevWindows = d, wn
	}
	if !sawRecordsOnly {
		t.Error("sweep never reached the records-only state before the narrow floor")
	}

	// Below the floor: single dim fallback line, cycle order intact.
	h.SetSize(scoreboardMinWidth-1, 10)
	lines, s := scoreboardStripped(t, h)
	if len(lines) != 1 {
		t.Fatalf("narrow fallback should be a single line, got %d:\n%s", len(lines), s)
	}
	if !strings.Contains(s, "wider") {
		t.Errorf("narrow fallback should mention needing a wider window, got: %q", s)
	}
	if !h.IntelMode() {
		t.Error("narrow fallback must not alter intel state")
	}
}

func TestToggleHelpZeroesScoreboardCache(t *testing.T) {
	h := newHorizontalForTest(t)
	h.scoreboardSrc = newCountingSource()
	enterScoreboard(h)
	if h.sbCache.pulledAt.IsZero() {
		t.Fatal("precondition: cache populated after entry")
	}
	h.ToggleHelp()
	if !h.sbCache.pulledAt.IsZero() {
		t.Error("ToggleHelp exits intel and must zero the scoreboard cache like every other exit path")
	}
	if h.intelPage != intelPageSession {
		t.Errorf("ToggleHelp should reset page to session, got %d", h.intelPage)
	}
}

func TestScoreboardZeroCachePullsOnDirectPageEntry(t *testing.T) {
	// Entry paths that bypass ToggleIntel (the pulledAt.IsZero guard's
	// reason to exist) must pull on first render rather than showing a
	// zero-valued page.
	h := newHorizontalForTest(t)
	src := newCountingSource()
	h.scoreboardSrc = src
	h.intelMode = true
	h.intelPage = intelPageScoreboard // no ToggleIntel, no eager pull
	h.intelView()
	if src.calls == 0 {
		t.Error("render with a zero cache should pull from the source")
	}
	if h.sbCache.pulledAt.IsZero() {
		t.Error("cache should be populated after the render-path pull")
	}
}
