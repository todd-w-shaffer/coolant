package theme

import (
	"charm.land/bubbles/v2/help"
	"charm.land/lipgloss/v2"
)

func (t *Theme) HelpStyles() help.Styles {
	return t.helpStyles
}

func buildHelpStyles(t *Theme) help.Styles {
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
