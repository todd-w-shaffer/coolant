package main

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/toddwshaffer/coolant/thermal/internal/anim"
	"github.com/toddwshaffer/coolant/thermal/internal/layout"
	"github.com/toddwshaffer/coolant/thermal/internal/theme"
)

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
	m, cmd := pressKey(t, m, "h")
	if got := m.layout.HelpMode(); got != layout.HelpFull {
		t.Errorf("after 'h' HelpMode = %d, want %d", got, layout.HelpFull)
	}
	if cmd == nil {
		t.Errorf("'h' should return non-nil cmd (auto-dismiss tick)")
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

func TestHelpDismissMsgReturnsToShort(t *testing.T) {
	m := newTestModel(t)
	m, _ = pressKey(t, m, "h")
	if got := m.layout.HelpMode(); got != layout.HelpFull {
		t.Fatalf("setup: HelpMode = %d, want %d", got, layout.HelpFull)
	}
	out, _ := m.Update(layout.HelpDismissMsg{})
	m = out.(model)
	if got := m.layout.HelpMode(); got != layout.HelpShort {
		t.Errorf("after HelpDismissMsg HelpMode = %d, want %d", got, layout.HelpShort)
	}
}
