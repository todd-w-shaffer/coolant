// Package ui defines shared colors, glyphs, and text helpers used
// across all dashboard widgets.
package ui

import (
	"image/color"

	"charm.land/lipgloss/v2"
	"github.com/toddwshaffer/coolant/thermal/internal/model"
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
	Char      string
	ANSI      string      // raw ANSI color (for sparkline context where lipgloss isn't used)
	Color     color.Color // lipgloss color for styled rendering
	Formatted string      // pre-computed: ANSI + Char + reset + space (e.g., "\033[37m●\033[0m ")
}

// GaugeDots are the sparkline row indicators: CPU (white), MEM (cyan), COMP (magenta), GPU (green).
var GaugeDots []GaugeDot

func init() {
	raw := []struct {
		char  string
		ansi  string
		color color.Color
	}{
		{"●", "\033[37m", lipgloss.Color("7")}, // cpu — white
		{"●", "\033[36m", lipgloss.Color("6")}, // mem — cyan
		{"●", "\033[35m", lipgloss.Color("5")}, // compressor — magenta
		{"●", "\033[32m", lipgloss.Color("2")}, // gpu — green (future sparkline color)
	}
	GaugeDots = make([]GaugeDot, len(raw))
	for i, r := range raw {
		GaugeDots[i] = GaugeDot{
			Char:      r.char,
			ANSI:      r.ansi,
			Color:     r.color,
			Formatted: r.ansi + r.char + "\033[0m ",
		}
	}
}

// TypeColor maps process type codes to lipgloss colors, shared across all widgets.
var TypeColor = map[string]color.Color{
	"N":  lipgloss.Color("2"),   // green — node
	"G":  lipgloss.Color("3"),   // yellow — grep
	"V":  lipgloss.Color("1"),   // red — vitest
	"S":  lipgloss.Color("6"),   // cyan — shell
	"R":  lipgloss.Color("5"),   // magenta — ripgrep
	"F":  lipgloss.Color("4"),   // blue — find
	"C":  lipgloss.Color("7"),   // white — claude
	"P":  lipgloss.Color("11"),  // bright yellow — python
	"T":  lipgloss.Color("14"),  // bright cyan — tsc
	"GO": lipgloss.Color("6"),   // cyan — go
	"RS": lipgloss.Color("208"), // orange — rust
	"SW": lipgloss.Color("166"), // deep orange — swift
	"X":  lipgloss.Color("8"),   // gray — other
}

// CategoryColor maps process-based categories to lipgloss colors.
var CategoryColor = map[string]color.Color{
	"build":  lipgloss.Color("208"), // orange — heavy but finite
	"shell":  lipgloss.Color("8"),   // gray — ephemeral
	"node":   lipgloss.Color("2"),   // green — JS runtime
	"go":     lipgloss.Color("6"),   // cyan — Go runtime
	"python": lipgloss.Color("11"),  // bright yellow — Python
	"rust":   lipgloss.Color("208"), // orange — Rust compilation
	"swift":  lipgloss.Color("166"), // deep orange — Swift compilation
}

// AgentGlyphHollow and AgentGlyphFilled are the hexagon pair for breathing
// agent indicators. The dot alternates between hollow and filled as it breathes.
const AgentGlyphHollow = "⬡"
const AgentGlyphFilled = "⬢"

// CategoryGlyph maps activity categories to distinct single-cell unicode glyphs.
// Visual weight mirrors resource weight: heavy categories get solid shapes.
var CategoryGlyph = map[string]string{
	"build":  "■", // square — solid, heavy
	"shell":  "·", // middle dot — ephemeral
	"node":   "●", // circle — JS runtime
	"go":     "◆", // diamond — Go runtime
	"python": "▲", // triangle — Python
	"rust":   "◇", // hollow diamond — Rust
	"swift":  "⬡", // hexagon — Swift
}

// CategoryGlyphFormatted holds pre-rendered ANSI-colored glyph strings,
// eliminating per-frame lipgloss.NewStyle() allocations in hot render paths.
var CategoryGlyphFormatted map[string]string

func init() {
	CategoryGlyphFormatted = make(map[string]string, len(CategoryGlyph))
	for name, glyph := range CategoryGlyph {
		clr := CategoryColor[name]
		if clr == nil {
			clr = DimColor
		}
		CategoryGlyphFormatted[name] = ColorText(clr, glyph)
	}
}

// ThreatColor maps threat levels to lipgloss colors — canonical source.
var ThreatColor = map[model.ThreatLevel]color.Color{
	model.ThreatCool:     lipgloss.Color("2"),   // green
	model.ThreatWarm:     lipgloss.Color("3"),   // yellow
	model.ThreatHot:      lipgloss.Color("208"), // orange
	model.ThreatMeltdown: lipgloss.Color("1"),   // red
}
