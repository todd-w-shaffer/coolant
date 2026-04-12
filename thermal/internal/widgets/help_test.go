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

func TestHelpShortViewContainsAllLabels(t *testing.T) {
	th := mustTheme(t, "classic")
	out := HelpShortView(th, keys.Default(), 200)
	for _, want := range []string{"help", "quit", "collapse sessions", "purge stale agents"} {
		if !strings.Contains(out, want) {
			t.Errorf("HelpShortView output missing %q\nfull output: %q", want, out)
		}
	}
}

func TestHelpShortViewDegradesBelowMinWidth(t *testing.T) {
	th := mustTheme(t, "classic")
	out := HelpShortView(th, keys.Default(), 40)
	if !strings.Contains(out, "[?]") {
		t.Errorf("HelpShortView at width=40 should contain %q, got %q", "[?]", out)
	}
	if strings.Contains(out, "collapse sessions") {
		t.Errorf("HelpShortView at width=40 should NOT include full descriptions, got %q", out)
	}
}

func TestRatesViewFullModeReturnsHelpOnly(t *testing.T) {
	th := mustTheme(t, "classic")
	state := fixtureState()
	r := NewRates(th, keys.Default(), func() int8 { return rateHelpFull })
	r.SetSize(200, 1)
	r.Update(state)
	out := r.View()
	if !strings.Contains(out, "press any key to dismiss") {
		t.Errorf("full-mode View() should contain dismiss hint, got %q", out)
	}
	if strings.Contains(out, "CPU:") {
		t.Errorf("full-mode View() should NOT contain CPU stats, got %q", out)
	}
}

func TestHelpFullViewIncludesAllGroups(t *testing.T) {
	th := mustTheme(t, "classic")
	out := HelpFullView(th, keys.Default(), 200)
	for _, want := range []string{"help", "quit", "collapse sessions", "purge stale agents", "press any key to dismiss"} {
		if !strings.Contains(out, want) {
			t.Errorf("HelpFullView output missing %q\nfull output: %q", want, out)
		}
	}
}
