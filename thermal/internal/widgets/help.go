package widgets

import (
	"strings"

	"charm.land/bubbles/v2/help"

	"github.com/toddwshaffer/coolant/thermal/internal/config"
	"github.com/toddwshaffer/coolant/thermal/internal/keys"
	"github.com/toddwshaffer/coolant/thermal/internal/theme"
	"github.com/toddwshaffer/coolant/thermal/internal/ui"
)

const helpDismissHint = "press any key to dismiss"

// HelpRenderer caches one bubbles help.Model per owner. Mutated in place on
// SetWidth / Short / Full, so nothing allocates on the ~6-7 Hz render path.
type HelpRenderer struct {
	model help.Model
	width int
}

func NewHelpRenderer(th *theme.Theme) *HelpRenderer {
	m := help.New()
	m.Styles = th.HelpStyles()
	return &HelpRenderer{model: m}
}

func (hr *HelpRenderer) SetWidth(w int) {
	hr.width = w
	hr.model.SetWidth(w)
}

// Short renders the one-line short form. Below config.HelpShortMinWidth it
// degrades to a compact "[?]".
func (hr *HelpRenderer) Short(km keys.KeyMap) string {
	if hr.width < config.HelpShortMinWidth {
		return ui.DimText("[?]")
	}
	hr.model.ShowAll = false
	return hr.model.View(km)
}

// Full renders the two-column full help prefixed with a dismiss hint.
func (hr *HelpRenderer) Full(km keys.KeyMap) string {
	hr.model.ShowAll = true
	var sb strings.Builder
	sb.WriteString(ui.DimText(helpDismissHint))
	sb.WriteString("\n")
	sb.WriteString(hr.model.View(km))
	return sb.String()
}
