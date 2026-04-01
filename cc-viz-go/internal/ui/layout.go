package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Render a 2x2 grid of pane strings with a status row below.
// Each pane string is pre-rendered content from a pane's View().
func RenderGrid(topLeft, topRight, bottomLeft, bottomRight, statusRow string, width, height int) string {
	if width < 4 || height < 4 {
		return ""
	}

	halfW := width / 2
	leftW := halfW
	rightW := width - halfW

	// Status row takes 2 lines (phase ring + alert summary)
	statusH := 2
	gridH := height - statusH
	if gridH < 2 {
		gridH = 2
	}
	halfH := gridH / 2
	topH := halfH
	bottomH := gridH - halfH

	paneStyle := func(w, h int) lipgloss.Style {
		return lipgloss.NewStyle().
			Width(w).
			Height(h).
			MaxWidth(w).
			MaxHeight(h)
	}

	tl := paneStyle(leftW, topH).Render(topLeft)
	tr := paneStyle(rightW, topH).Render(topRight)
	bl := paneStyle(leftW, bottomH).Render(bottomLeft)
	br := paneStyle(rightW, bottomH).Render(bottomRight)

	topRow := lipgloss.JoinHorizontal(lipgloss.Top, tl, tr)
	botRow := lipgloss.JoinHorizontal(lipgloss.Top, bl, br)
	grid := lipgloss.JoinVertical(lipgloss.Left, topRow, botRow)

	// Pad status row to full width
	status := lipgloss.NewStyle().Width(width).Render(statusRow)

	return strings.Join([]string{grid, status}, "\n")
}
