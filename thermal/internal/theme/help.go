// Package theme — help.go provides the bridge from coolant's palette to
// the bubbles/help component's Styles struct. Keeps widgets free of any
// direct theme-color → lipgloss.Style conversion.
package theme

import (
	"charm.land/bubbles/v2/help"
	"charm.land/lipgloss/v2"
)

// HelpStyles maps the theme's HelpColor onto bubbles/help's key styles
// (ShortKey, FullKey) and DimColor onto the descriptive / separator /
// ellipsis styles. No new theme fields required — every built-in palette
// already populates HelpColor and DimColor.
func (t *Theme) HelpStyles() help.Styles {
	keyStyle := lipgloss.NewStyle().Foreground(t.HelpColor)
	dimStyle := lipgloss.NewStyle().Foreground(t.DimColor)
	return help.Styles{
		ShortKey:       keyStyle,
		ShortDesc:      dimStyle,
		ShortSeparator: dimStyle,
		FullKey:        keyStyle,
		FullDesc:       dimStyle,
		FullSeparator:  dimStyle,
		Ellipsis:       dimStyle,
	}
}
