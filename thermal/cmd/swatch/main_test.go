package main

import (
	"strings"
	"testing"

	"github.com/toddwshaffer/coolant/thermal/internal/theme"
)

// TestAllSectionsNonEmpty verifies every section renderer produces non-empty
// output for every registered theme.
func TestAllSectionsNonEmpty(t *testing.T) {
	t.Helper()

	type sectionFunc func(*theme.Theme) string

	sections := map[string]sectionFunc{
		"severityGradient": renderSeverityGradient,
		"overallGradient":  renderOverallGradient,
		"categoryGradient": renderCategoryGradient,
		"threatColors":     renderThreatColors,
		"sessionDiamonds":  renderSessionDiamonds,
		"gaugeDots":        renderGaugeDots,
		"agentIcons":       renderAgentIcons,
		"offlineSparkline": renderOfflineSparkline,
		"rates":            renderRates,
		"chrome":           renderChrome,
	}

	for _, name := range theme.Names() {
		th, err := theme.Get(name)
		if err != nil {
			t.Fatalf("theme %q: %v", name, err)
		}
		for secName, fn := range sections {
			out := fn(th)
			if strings.TrimSpace(out) == "" {
				t.Errorf("theme %q, section %q: produced empty output", name, secName)
			}
		}
	}
}

// TestRenderAllThemes verifies the full render produces labeled sections.
func TestRenderAllThemes(t *testing.T) {
	for _, name := range theme.Names() {
		th, err := theme.Get(name)
		if err != nil {
			t.Fatalf("theme %q: %v", name, err)
		}
		out := renderTheme(th)
		if !strings.Contains(out, "Severity Gradient") {
			t.Errorf("theme %q: missing Severity Gradient label", name)
		}
		if !strings.Contains(out, "Threat Colors") {
			t.Errorf("theme %q: missing Threat Colors label", name)
		}
		if !strings.Contains(out, "Gauge Dots") {
			t.Errorf("theme %q: missing Gauge Dots label", name)
		}
	}
}

// TestSeverityGradientWidth verifies the braille sparkline ramp has the
// expected number of characters (40 chars = 80 samples at 2 per char).
func TestSeverityGradientWidth(t *testing.T) {
	th, _ := theme.Get("classic")
	out := renderSeverityGradient(th)
	// Strip ANSI escapes to count visible characters
	stripped := stripANSI(out)
	lines := strings.Split(strings.TrimSpace(stripped), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 lines, got %d", len(lines))
	}
	// The bottom row should have swatchSparkWidth visible characters.
	// The top row may have leading/trailing spaces for low-value samples
	// (braille bits == 0 renders as space), so only check the bottom.
	bottom := lines[len(lines)-1]
	runes := []rune(bottom)
	if len(runes) != swatchSparkWidth {
		t.Errorf("bottom row: expected %d visible chars, got %d", swatchSparkWidth, len(runes))
	}
}

// stripANSI removes ANSI escape sequences from a string.
func stripANSI(s string) string {
	var out strings.Builder
	inEsc := false
	for _, r := range s {
		if r == '\033' {
			inEsc = true
			continue
		}
		if inEsc {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEsc = false
			}
			continue
		}
		out.WriteRune(r)
	}
	return out.String()
}
