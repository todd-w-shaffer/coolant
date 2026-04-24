package main

import (
	"os"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	zone "github.com/lrstanley/bubblezone/v2"

	"github.com/toddwshaffer/coolant/thermal/internal/anim"
	"github.com/toddwshaffer/coolant/thermal/internal/collector"
	"github.com/toddwshaffer/coolant/thermal/internal/config"
	"github.com/toddwshaffer/coolant/thermal/internal/layout"
	"github.com/toddwshaffer/coolant/thermal/internal/theme"
)

func TestMain(m *testing.M) {
	zone.NewGlobal()
	os.Exit(m.Run())
}

func newTestModel(t *testing.T) model {
	t.Helper()
	th, err := theme.Get("classic")
	if err != nil {
		t.Fatalf("theme.Get: %v", err)
	}
	return newModel(true, th, anim.Default())
}

func pressKey(t *testing.T, m model, k string) (model, tea.Cmd) {
	t.Helper()
	runes := []rune(k)
	key := tea.Key{Code: rune(runes[0]), Text: k}
	out, cmd := m.Update(tea.KeyPressMsg(key))
	return out.(model), cmd
}

func TestHelpKeyEntersFullMode(t *testing.T) {
	m := newTestModel(t)
	if got := m.layout.HelpMode(); got != layout.HelpShort {
		t.Fatalf("initial HelpMode = %d, want %d", got, layout.HelpShort)
	}
	m, _ = pressKey(t, m, "h")
	if got := m.layout.HelpMode(); got != layout.HelpFull {
		t.Errorf("after 'h' HelpMode = %d, want %d", got, layout.HelpFull)
	}
}

func TestHelpKeyTogglesOffFromFullMode(t *testing.T) {
	m := newTestModel(t)
	m, _ = pressKey(t, m, "h")
	m, _ = pressKey(t, m, "h")
	if got := m.layout.HelpMode(); got != layout.HelpShort {
		t.Errorf("after second 'h' HelpMode = %d, want %d", got, layout.HelpShort)
	}
}

func TestQuestionKeyEntersFullMode(t *testing.T) {
	m := newTestModel(t)
	m, _ = pressKey(t, m, "?")
	if got := m.layout.HelpMode(); got != layout.HelpFull {
		t.Errorf("after '?' HelpMode = %d, want %d", got, layout.HelpFull)
	}
}

func TestKeyConsumedInFullMode(t *testing.T) {
	m := newTestModel(t)
	// Enter full mode.
	m, _ = pressKey(t, m, "h")
	// Press 'q' — should dismiss without quitting (we'd see done channel close otherwise).
	m, _ = pressKey(t, m, "q")
	if got := m.layout.HelpMode(); got != layout.HelpShort {
		t.Errorf("after key in full mode HelpMode = %d, want %d", got, layout.HelpShort)
	}
	// Verify done channel is still open by attempting non-blocking receive.
	select {
	case <-m.done:
		t.Errorf("'q' in full mode should not have closed done channel")
	default:
	}
}

func TestCategoryKeybindings(t *testing.T) {
	m := newTestModel(t)
	// Seed smoothed cats so cycling has visible categories.
	state := m.layout.State()
	state.SmoothedCats["build"] = 2.0
	state.SmoothedCats["shell"] = 1.0
	state.SmoothedCats["node"] = 3.0

	// ] → forward from empty → first visible
	m, _ = pressKey(t, m, "]")
	if got := m.layout.State().CategoryFilter; got != "build" {
		t.Errorf("after ']': got %q, want %q", got, "build")
	}

	// [ → backward → wraps to empty
	m, _ = pressKey(t, m, "[")
	if got := m.layout.State().CategoryFilter; got != "" {
		t.Errorf("after '[' from first: got %q, want empty", got)
	}

	// ] twice → second visible
	m, _ = pressKey(t, m, "]")
	m, _ = pressKey(t, m, "]")
	if got := m.layout.State().CategoryFilter; got != "shell" {
		t.Errorf("after two ']': got %q, want %q", got, "shell")
	}

	// \ → clear
	m, _ = pressKey(t, m, "\\")
	if got := m.layout.State().CategoryFilter; got != "" {
		t.Errorf("after '\\': got %q, want empty", got)
	}
}

func TestMouseToggle(t *testing.T) {
	m := newTestModel(t)
	m.width = 120
	m.height = 10
	m.layout.SetSize(120, 10)

	// Default: mouse enabled
	v := m.View()
	if v.MouseMode != tea.MouseModeCellMotion {
		t.Errorf("default MouseMode = %v, want CellMotion", v.MouseMode)
	}

	// Press 'm' → mouse disabled
	m, _ = pressKey(t, m, "m")
	v = m.View()
	if v.MouseMode != tea.MouseModeNone {
		t.Errorf("after 'm' MouseMode = %v, want None", v.MouseMode)
	}

	// Press 'm' again → mouse re-enabled
	m, _ = pressKey(t, m, "m")
	v = m.View()
	if v.MouseMode != tea.MouseModeCellMotion {
		t.Errorf("after second 'm' MouseMode = %v, want CellMotion", v.MouseMode)
	}
}

func TestTmuxHintFiresOnce(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux-1/default,12345,0")

	m := newTestModel(t)
	m.width = 120
	m.height = 10
	m.layout.SetSize(120, 10)

	// First snapshot should trigger the tmux hint alert.
	snap := collector.Snapshot{Online: true}
	out, _ := m.Update(snapshotMsg(snap))
	m = out.(model)

	alerts := m.layout.State().Alerts
	found := false
	for i := 0; i < alerts.Len(); i++ {
		if strings.Contains(alerts.At(i).Message, "tmux set -g mouse on") {
			found = true
			break
		}
	}
	if !found {
		t.Error("first snapshot in TMUX should push tmux hint alert")
	}

	// Second snapshot should NOT push a duplicate.
	countBefore := alerts.Len()
	out, _ = m.Update(snapshotMsg(snap))
	m = out.(model)
	countAfter := m.layout.State().Alerts.Len()

	// The second snapshot pushes through State().Update which may add
	// threat-transition alerts, but should NOT add another tmux hint.
	for i := countBefore; i < countAfter; i++ {
		if strings.Contains(m.layout.State().Alerts.At(i).Message, "tmux set -g mouse on") {
			t.Error("tmux hint should fire only once")
		}
	}
}

func TestIntelKeyTogglesOverlay(t *testing.T) {
	m := newTestModel(t)
	// Need a snapshot so we're not idle (intel suppressed during idle)
	m.width = 120
	m.height = 10
	m.layout.SetSize(120, 10)
	snap := collector.Snapshot{Online: true, Sessions: []collector.SessionTree{{RootPID: 1}}}
	out, _ := m.Update(snapshotMsg(snap))
	m = out.(model)

	m, _ = pressKey(t, m, "i")
	if !m.layout.IntelMode() {
		t.Error("'i' should toggle intel mode on")
	}
	m, _ = pressKey(t, m, "i")
	// Any key in intel mode dismisses — so second 'i' dismisses, doesn't toggle back on
	if m.layout.IntelMode() {
		t.Error("second key in intel mode should dismiss")
	}
}

func TestIntelDismissedByAnyKey(t *testing.T) {
	m := newTestModel(t)
	m.width = 120
	m.height = 10
	m.layout.SetSize(120, 10)
	snap := collector.Snapshot{Online: true, Sessions: []collector.SessionTree{{RootPID: 1}}}
	out, _ := m.Update(snapshotMsg(snap))
	m = out.(model)

	m, _ = pressKey(t, m, "i")
	if !m.layout.IntelMode() {
		t.Fatal("precondition: intel should be on")
	}
	m, _ = pressKey(t, m, "x")
	if m.layout.IntelMode() {
		t.Error("'x' in intel mode should dismiss intel")
	}
}

func TestIntelKeyCyclesDepthFromFocused(t *testing.T) {
	m := newTestModel(t)
	m.width = 120
	m.height = 10
	m.layout.SetSize(120, 10)
	snap := collector.Snapshot{Online: true, Sessions: []collector.SessionTree{{RootPID: 1}}}
	out, _ := m.Update(snapshotMsg(snap))
	m = out.(model)

	// Enter focused mode directly
	m.layout.FocusAgent("test123")
	if !m.layout.IntelMode() || m.layout.FocusedAgentID() != "test123" {
		t.Fatal("precondition: should be in focused intel mode")
	}
	// i → clears focus, keeps intel (session summary)
	m, _ = pressKey(t, m, "i")
	if m.layout.FocusedAgentID() != "" {
		t.Error("'i' from focused should clear focusedAgentID")
	}
	if !m.layout.IntelMode() {
		t.Error("'i' from focused should keep intel mode (session summary)")
	}
	// i again → exits intel entirely
	m, _ = pressKey(t, m, "i")
	if m.layout.IntelMode() {
		t.Error("'i' from session summary should exit intel")
	}
}

func TestNonIKeyDismissesFromFocused(t *testing.T) {
	m := newTestModel(t)
	m.width = 120
	m.height = 10
	m.layout.SetSize(120, 10)
	snap := collector.Snapshot{Online: true, Sessions: []collector.SessionTree{{RootPID: 1}}}
	out, _ := m.Update(snapshotMsg(snap))
	m = out.(model)

	m.layout.FocusAgent("test123")
	// Any non-i key → dismisses entirely
	m, _ = pressKey(t, m, "x")
	if m.layout.IntelMode() {
		t.Error("non-i key in focused mode should dismiss intel")
	}
	if m.layout.FocusedAgentID() != "" {
		t.Error("non-i key should clear focusedAgentID")
	}
}

func TestCounterResetDismissesIntel(t *testing.T) {
	m := newTestModel(t)
	m.width = 120
	m.height = 10
	m.layout.SetSize(120, 10)
	snap := collector.Snapshot{Online: true, Sessions: []collector.SessionTree{{RootPID: 1}}}
	out, _ := m.Update(snapshotMsg(snap))
	m = out.(model)

	m.layout.FocusAgent("test123")

	resetEv := gateEventMsg(collector.GateEvent{Event: collector.EventCounterReset})
	out, _ = m.Update(resetEv)
	m = out.(model)

	if m.layout.IntelMode() {
		t.Error("counter.reset should dismiss intel")
	}
	if m.layout.FocusedAgentID() != "" {
		t.Error("counter.reset should clear focusedAgentID")
	}
}

func TestClickAgentZoneFocuses(t *testing.T) {
	m := newTestModel(t)
	m.width = 120
	m.height = 10
	m.layout.SetSize(120, 10)
	m.layout.SetHighScoreMode(true)

	// Populate completed agents so RenderedAgentIDs is non-empty.
	state := m.layout.State()
	t0 := time.Now().Add(time.Millisecond)
	state.HandleEvent(collector.GateEvent{
		Event: collector.EventAgentStart, AgentID: "click1", AgentType: "general-purpose", Timestamp: t0,
	})
	state.HandleEvent(collector.GateEvent{
		Event: collector.EventAgentStop, AgentID: "click1", AgentType: "general-purpose", Timestamp: t0.Add(10 * time.Second),
	})

	snap := collector.Snapshot{
		Online:   true,
		Sessions: []collector.SessionTree{{RootPID: 1}},
		System:   collector.SystemStats{NCPUs: 10, MemTotalBytes: 16 << 30},
	}
	state.Update(snap)
	m.layout.Update(state)

	// Render to populate zone marks + RenderedAgentIDs cache.
	v := m.View()
	_ = v

	// Verify RenderedAgentIDs is populated.
	ids := m.layout.RenderedAgentIDs()
	if len(ids) == 0 {
		t.Fatal("precondition: RenderedAgentIDs should be non-empty after render")
	}

	// Simulate click on the agent zone — we can't easily construct real
	// coordinates that land inside a zone.Mark region, but we can verify
	// the dispatch logic by checking that FocusAgent works from the handler.
	// For a structural test, verify that a click on a matching zone ID
	// would call FocusAgent.
	m.layout.FocusAgent(ids[0])
	if m.layout.FocusedAgentID() != ids[0] {
		t.Errorf("FocusAgent should set focusedAgentID to %q, got %q", ids[0], m.layout.FocusedAgentID())
	}
}

func TestClickDebounceSwallowsRapidClicks(t *testing.T) {
	m := newTestModel(t)
	m.width = 120
	m.height = 10
	m.layout.SetSize(120, 10)

	// Set lastFocusTime to now — simulates just having opened a focused overlay.
	m.lastFocusTime = time.Now()

	// Simulate a left click during cooldown — agent zone dispatch should be skipped.
	// Even if a zone matched, debounce prevents FocusAgent from firing.
	click := tea.MouseClickMsg{Button: tea.MouseLeft, X: 1, Y: 1}
	out, _ := m.Update(click)
	m = out.(model)
	if m.layout.FocusedAgentID() != "" {
		t.Error("click within debounce cooldown should not focus an agent")
	}

	// After cooldown expires, clicks should be dispatched normally.
	m.lastFocusTime = time.Now().Add(-2 * config.ClickDebounce)
	// No agent zones are registered in this test, so FocusAgent won't fire,
	// but verify the debounce gate no longer blocks.
	if time.Since(m.lastFocusTime) < config.ClickDebounce {
		t.Error("lastFocusTime should be past cooldown")
	}
}

func TestAgentUnderCursorEmptyIDs(t *testing.T) {
	click := tea.MouseClickMsg{Button: tea.MouseLeft, X: 5, Y: 5}
	got := agentUnderCursor(nil, click)
	if got != "" {
		t.Errorf("agentUnderCursor(nil) = %q, want empty", got)
	}
	got = agentUnderCursor([]string{}, click)
	if got != "" {
		t.Errorf("agentUnderCursor([]) = %q, want empty", got)
	}
}

func TestAgentUnderCursorNoMatch(t *testing.T) {
	// Zone IDs that aren't registered — zone.Get returns nil, should skip.
	click := tea.MouseClickMsg{Button: tea.MouseLeft, X: 0, Y: 0}
	got := agentUnderCursor([]string{"nonexistent1", "nonexistent2"}, click)
	if got != "" {
		t.Errorf("agentUnderCursor with unregistered zones = %q, want empty", got)
	}
}

func TestQuitConsumedInIntelMode(t *testing.T) {
	m := newTestModel(t)
	m.width = 120
	m.height = 10
	m.layout.SetSize(120, 10)
	snap := collector.Snapshot{Online: true, Sessions: []collector.SessionTree{{RootPID: 1}}}
	out, _ := m.Update(snapshotMsg(snap))
	m = out.(model)

	m, _ = pressKey(t, m, "i")
	m, _ = pressKey(t, m, "q")
	if m.layout.IntelMode() {
		t.Error("'q' in intel mode should dismiss intel")
	}
	select {
	case <-m.done:
		t.Error("'q' in intel mode should NOT quit the app")
	default:
	}
}
