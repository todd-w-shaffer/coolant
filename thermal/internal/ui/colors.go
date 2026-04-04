package ui

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// Shared color constants — use these instead of hardcoding lipgloss.Color("8") etc.
var (
	DimColor  = lipgloss.Color("8") // gray — dim/muted text
	CyanColor = lipgloss.Color("6") // cyan — cool/offline accents
)

// ColorText renders text in the given foreground color. Replaces the
// repeated lipgloss.NewStyle().Foreground(c).Render(text) pattern.
func ColorText(c color.Color, text string) string {
	return lipgloss.NewStyle().Foreground(c).Render(text)
}

// DimText renders text in the shared dim gray color.
func DimText(text string) string {
	return ColorText(DimColor, text)
}

// GaugeDot defines a colored dot indicator for sparkline rows.
type GaugeDot struct {
	Char  string
	ANSI  string // raw ANSI color (for sparkline context where lipgloss isn't used)
	Color color.Color
}

// GaugeDots are the three sparkline row indicators: CPU (white), MEM (cyan), COMP (magenta).
var GaugeDots = []GaugeDot{
	{"●", "\033[37m", lipgloss.Color("7")}, // cpu — white
	{"●", "\033[36m", lipgloss.Color("6")}, // mem — cyan
	{"●", "\033[35m", lipgloss.Color("5")}, // compressor — magenta
}

// TypeColor maps process type codes to lipgloss colors, shared across all widgets.
var TypeColor = map[string]color.Color{
	"N": lipgloss.Color("2"),  // green — node
	"G": lipgloss.Color("3"),  // yellow — grep
	"V": lipgloss.Color("1"),  // red — vitest
	"S": lipgloss.Color("6"),  // cyan — shell
	"R": lipgloss.Color("5"),  // magenta — ripgrep
	"F": lipgloss.Color("4"),  // blue — find
	"C": lipgloss.Color("7"),  // white — claude
	"P": lipgloss.Color("11"), // bright yellow — python
	"T": lipgloss.Color("14"), // bright cyan — tsc
	"X": lipgloss.Color("8"),  // gray — other
}

// CategoryColor maps activity categories to lipgloss colors.
var CategoryColor = map[string]color.Color{
	"test":   lipgloss.Color("1"),   // red — the machine killer
	"build":  lipgloss.Color("208"), // orange — heavy but finite
	"run":    lipgloss.Color("3"),   // yellow — runtime processes
	"search": lipgloss.Color("4"),   // blue — lightweight exploration
	"shell":  lipgloss.Color("8"),   // gray — ephemeral
}

// Threshold defaults.
const (
	SpawnWarn = 10
	SpawnCrit = 20
	NetWarn   = 5
	NetCrit   = 15
	TotalWarn = 50
	TotalCrit = 100
)

// ThresholdColor returns green/yellow/red based on value vs thresholds.
func ThresholdColor(val, warn, crit float64) color.Color {
	if val >= crit {
		return lipgloss.Color("1") // red
	}
	if val >= warn {
		return lipgloss.Color("3") // yellow
	}
	return lipgloss.Color("7") // white
}
