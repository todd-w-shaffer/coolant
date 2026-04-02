package ui

import "github.com/charmbracelet/lipgloss"

// TypeColor maps process type codes to lipgloss colors, shared across all widgets.
var TypeColor = map[string]lipgloss.Color{
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
var CategoryColor = map[string]lipgloss.Color{
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
func ThresholdColor(val, warn, crit int) lipgloss.Color {
	if val >= crit {
		return lipgloss.Color("1") // red
	}
	if val >= warn {
		return lipgloss.Color("3") // yellow
	}
	return lipgloss.Color("7") // white
}
