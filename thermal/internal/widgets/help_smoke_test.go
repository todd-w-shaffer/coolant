package widgets

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
)

// smokeKeyMap is a minimal help.KeyMap implementation for the compat test.
type smokeKeyMap struct {
	Help key.Binding
}

func (s smokeKeyMap) ShortHelp() []key.Binding  { return []key.Binding{s.Help} }
func (s smokeKeyMap) FullHelp() [][]key.Binding { return [][]key.Binding{{s.Help}} }

// TestBubblesHelpRendersSomething guards against future drift in the
// charm.land/bubbles/v2 dependency. If this test starts failing, the bubbles
// API contract has changed and the help wiring must be revisited.
func TestBubblesHelpRendersSomething(t *testing.T) {
	m := help.New()
	m.SetWidth(80)
	km := smokeKeyMap{
		Help: key.NewBinding(
			key.WithKeys("h"),
			key.WithHelp("h", "help"),
		),
	}
	out := m.View(km)
	if out == "" {
		t.Fatalf("expected non-empty help view, got empty string")
	}
	if !strings.Contains(out, "help") {
		t.Fatalf("expected help output to contain description token %q, got %q", "help", out)
	}
}
