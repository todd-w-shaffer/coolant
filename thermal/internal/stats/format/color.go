package format

import (
	"image/color"
	"time"

	"charm.land/lipgloss/v2"
)

// StyleKind names a semantic style slot. The renderer maps each kind
// to a concrete lipgloss style so consumers don't import lipgloss
// directly. Adding a new slot here is the right hook for new
// section-specific styling (e.g., a "warning" slot for diagnostics).
type StyleKind int

const (
	StyleHeader StyleKind = iota
	StyleSectionTitle
	StyleRecordValue
	StyleRecordMeta
	StyleWindowLabel
	StyleDimmed
)

// Renderer carries display state shared across helpers — color mode,
// terminal width hint (for distribution truncation), and `time.Now()`
// injection for tests. Both CLI and scoreboard build a Renderer once
// and call methods on it. Free functions in format.go remain available
// for callers that don't need state.
type Renderer struct {
	// Plain forces zero-color output (set when --color=never or when
	// stdout is not a TTY). When false, lipgloss's own detection still
	// applies — a non-TTY caller may end up with no escapes anyway.
	Plain bool

	// Now is the injection point for relative-time tests. Zero value
	// means "use time.Now().UTC()" via Clock().
	Now time.Time

	// Width is a hint for distribution row truncation. 0 means "no
	// truncation".
	Width int
}

// Clock returns the renderer's notion of "now", defaulting to UTC.
// Tests pin this to a fixed instant for golden stability.
func (r Renderer) Clock() time.Time {
	if r.Now.IsZero() {
		return time.Now().UTC()
	}
	return r.Now.UTC()
}

// Style renders a string under the given semantic kind. In Plain mode
// the helper short-circuits to identity — no allocation, no ANSI.
func (r Renderer) Style(kind StyleKind, s string) string {
	if r.Plain {
		return s
	}
	return styleFor(kind).Render(s)
}

func styleFor(kind StyleKind) lipgloss.Style {
	switch kind {
	case StyleHeader:
		return lipgloss.NewStyle().Bold(true).Foreground(rgb(0xc0, 0xc0, 0xc0))
	case StyleSectionTitle:
		return lipgloss.NewStyle().Bold(true).Foreground(rgb(0x88, 0xc0, 0xd0))
	case StyleRecordValue:
		return lipgloss.NewStyle().Foreground(rgb(0xea, 0xea, 0xea))
	case StyleRecordMeta:
		return lipgloss.NewStyle().Foreground(rgb(0x98, 0x98, 0x98))
	case StyleWindowLabel:
		return lipgloss.NewStyle().Foreground(rgb(0xb0, 0xb8, 0xc0))
	case StyleDimmed:
		return lipgloss.NewStyle().Foreground(rgb(0x70, 0x70, 0x70))
	default:
		return lipgloss.NewStyle()
	}
}

func rgb(r, g, b uint8) color.Color {
	return color.RGBA{R: r, G: g, B: b, A: 0xff}
}
