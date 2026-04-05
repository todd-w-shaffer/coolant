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

// SessionGlyph is the diamond icon for session/agent markers.
const SessionGlyph = "◆"

// CategoryGlyph maps activity categories to distinct single-cell unicode glyphs.
// Visual weight mirrors resource weight: heavy categories get solid shapes.
var CategoryGlyph = map[string]string{
	"test":   "▲", // triangle — danger, the machine killer
	"build":  "■", // square — solid, heavy
	"run":    "●", // circle — standard process
	"search": "◇", // hollow diamond — lightweight
	"shell":  "·", // middle dot — ephemeral
}

// CategoryGlyphDefault is used when category is unknown.
const CategoryGlyphDefault = "·"

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

// Threshold defaults.
const (
	SpawnWarn = 10
	SpawnCrit = 20
	NetWarn   = 5
	NetCrit   = 15
	TotalWarn = 50
	TotalCrit = 100
)

// ThreatColor maps threat levels to lipgloss colors — canonical source.
var ThreatColor = map[model.ThreatLevel]color.Color{
	model.ThreatCool:     lipgloss.Color("2"),   // green
	model.ThreatWarm:     lipgloss.Color("3"),   // yellow
	model.ThreatHot:      lipgloss.Color("208"), // orange
	model.ThreatMeltdown: lipgloss.Color("1"),   // red
}
