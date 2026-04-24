package layout

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
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

// TestHelpViewCoversAllBindings asserts that every keybinding registered in
// KeyMap appears somewhere in the full help overlay. Catches drift when a
// binding is added to keys.go but helpView() in horizontal.go isn't updated.
func TestHelpViewMentionsClickToInspect(t *testing.T) {
	h := newHorizontalForTest(t)
	lines := h.helpView()
	var combined string
	for _, line := range lines {
		combined += ansi.Strip(line) + " "
	}
	if !strings.Contains(combined, "click dot for details") {
		t.Errorf("helpView should mention click-to-inspect: %q", combined)
	}
}

func TestHelpViewCoversAllBindings(t *testing.T) {
	h := newHorizontalForTest(t)
	lines := h.helpView()

	// Flatten and strip ANSI so we match plain text.
	var combined string
	for _, line := range lines {
		combined += ansi.Strip(line) + " "
	}

	// Check FullHelp groups 1+ (group 0 is help/quit — meta keys that
	// don't need full-overlay entries since "press any key" covers them).
	km := keys.Default()
	for gi, group := range km.FullHelp() {
		if gi == 0 {
			continue
		}
		for _, b := range group {
			help := b.Help()
			if !strings.Contains(combined, help.Key) {
				t.Errorf("helpView missing key %q (desc: %q) from FullHelp group %d", help.Key, help.Desc, gi)
			}
		}
	}
}
