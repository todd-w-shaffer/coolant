package main

import (
	"os"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	zone "github.com/lrstanley/bubblezone/v2"

	"github.com/toddwshaffer/coolant/thermal/internal/anim"
	"github.com/toddwshaffer/coolant/thermal/internal/collector"
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
