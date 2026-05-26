package widgets

import (
	"strings"
	"testing"

	"github.com/toddwshaffer/coolant/thermal/internal/keys"
	"github.com/toddwshaffer/coolant/thermal/internal/theme"
)

func mustTheme(t *testing.T, name string) *theme.Theme {
	t.Helper()
	th, err := theme.Get(name)
	if err != nil {
		t.Fatalf("theme.Get(%q): %v", name, err)
	}
	return th
}

func newHelpAtWidth(t *testing.T, th *theme.Theme, w int) *HelpRenderer {
	t.Helper()
	hr := NewHelpRenderer(th)
	hr.SetWidth(w)
	return hr
}

func TestHelpShortContainsAllLabels(t *testing.T) {
	th := mustTheme(t, "classic")
	out := newHelpAtWidth(t, th, 200).Short(keys.Default())
	for _, want := range []string{"help", "quit", "collapse sessions", "clear completed"} {
		if !strings.Contains(out, want) {
			t.Errorf("Short output missing %q\nfull output: %q", want, out)
		}
	}
}

func TestHelpShortDegradesBelowMinWidth(t *testing.T) {
	th := mustTheme(t, "classic")
	out := newHelpAtWidth(t, th, 40).Short(keys.Default())
	if !strings.Contains(out, "[?]") {
		t.Errorf("Short at width=40 should contain %q, got %q", "[?]", out)
	}
	if strings.Contains(out, "collapse sessions") {
		t.Errorf("Short at width=40 should NOT include full descriptions, got %q", out)
	}
}
