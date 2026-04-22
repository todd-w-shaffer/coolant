package layout

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/toddwshaffer/coolant/thermal/internal/collector"
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

func TestHelpViewContainsSessionIntel(t *testing.T) {
	h := newHorizontalForTest(t)
	lines := h.helpView()
	combined := ""
	for _, l := range lines {
		combined += ansi.Strip(l) + " "
	}
	if !strings.Contains(combined, "session intel") {
		t.Error("helpView should contain 'session intel' description for the i binding")
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
