package layout

import (
	"testing"

	"github.com/toddwshaffer/coolant/thermal/internal/anim"
	"github.com/toddwshaffer/coolant/thermal/internal/keys"
	"github.com/toddwshaffer/coolant/thermal/internal/theme"
)

func newHorizontalForTest(t *testing.T) *Horizontal {
	t.Helper()
	th, err := theme.Get("classic")
	if err != nil {
		t.Fatalf("theme.Get: %v", err)
	}
	return NewHorizontal(th, anim.Default(), keys.Default())
}

func TestToggleHelpEntersFullMode(t *testing.T) {
	h := newHorizontalForTest(t)
	if got := h.HelpMode(); got != HelpShort {
		t.Fatalf("initial HelpMode = %d, want %d", got, HelpShort)
	}
	h.ToggleHelp()
	if got := h.HelpMode(); got != HelpFull {
		t.Errorf("after ToggleHelp HelpMode = %d, want %d", got, HelpFull)
	}
}

func TestDismissHelpReturnsToShort(t *testing.T) {
	h := newHorizontalForTest(t)
	h.ToggleHelp()
	h.DismissHelp()
	if got := h.HelpMode(); got != HelpShort {
		t.Errorf("after DismissHelp HelpMode = %d, want %d", got, HelpShort)
	}
}

func TestDismissHelpIdempotent(t *testing.T) {
	h := newHorizontalForTest(t)
	h.DismissHelp()
	h.DismissHelp()
	if got := h.HelpMode(); got != HelpShort {
		t.Errorf("repeated DismissHelp HelpMode = %d, want %d", got, HelpShort)
	}
}

func TestToggleHelpFromFullReturnsToShort(t *testing.T) {
	h := newHorizontalForTest(t)
	h.ToggleHelp()
	h.ToggleHelp()
	if got := h.HelpMode(); got != HelpShort {
		t.Errorf("toggling from full HelpMode = %d, want %d", got, HelpShort)
	}
}
