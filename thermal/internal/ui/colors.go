// Package ui defines shared colors, glyphs, and text helpers used
// across all dashboard widgets.
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

// Agent hexagon glyphs — three states for wave animation.
// Hollow (trough) → benzene/mid (shoulder) → filled (peak).
const AgentGlyphHollow = "⬡"
const AgentGlyphMid = "⏣"
const AgentGlyphFilled = "⬢"

// SessionDiamondGlyph is the phase-colored session indicator rendered in the
// headline, the help legend, and the swatch preview.
const SessionDiamondGlyph = "⌬"

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
