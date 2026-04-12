// Package widgets — help.go is a thin adapter over charm.land/bubbles/v2/help.
// It wires coolant's keys.KeyMap and theme.HelpStyles together for the rates
// strip's help slot (short form) and the in-place full panel.
package widgets

import (
	"strings"

	"charm.land/bubbles/v2/help"

	"github.com/toddwshaffer/coolant/thermal/internal/config"
	"github.com/toddwshaffer/coolant/thermal/internal/keys"
	"github.com/toddwshaffer/coolant/thermal/internal/theme"
	"github.com/toddwshaffer/coolant/thermal/internal/ui"
)

// helpDismissHint is the prefix prepended to the full-help view that tells
// the user how to collapse the panel. Lives in widgets (presentation), not
// keys (semantic).
const helpDismissHint = "press any key to dismiss"

// newHelpModel constructs a bubbles help.Model with the theme's HelpStyles
// applied and the requested width set. Both short and full views share this
// constructor.
func newHelpModel(th *theme.Theme, width int) help.Model {
	m := help.New()
	m.Styles = th.HelpStyles()
	m.SetWidth(width)
	return m
}

// HelpShortView renders the one-line short form for the rates strip.
// Below config.HelpShortMinWidth, it degrades to a compact dim "[?]" token.
func HelpShortView(th *theme.Theme, km keys.KeyMap, width int) string {
	if width < config.HelpShortMinWidth {
		return ui.DimText("[?]")
	}
	m := newHelpModel(th, width)
	return m.View(km)
}

// HelpFullView renders the two-column full help, prefixed with a dismiss
// hint. Width-aware via the underlying bubbles/help layout.
func HelpFullView(th *theme.Theme, km keys.KeyMap, width int) string {
	m := newHelpModel(th, width)
	m.ShowAll = true
	var sb strings.Builder
	sb.WriteString(ui.DimText(helpDismissHint))
	sb.WriteString("\n")
	sb.WriteString(m.View(km))
	return sb.String()
}
