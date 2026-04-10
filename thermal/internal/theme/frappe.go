package theme

import (
	"image/color"

	colorful "github.com/lucasb-eyer/go-colorful"

	"charm.land/lipgloss/v2"
)

// Catppuccin Frappe palette — exact hex values.
var (
	frRosewater = lipgloss.Color("#f2d5cf")
	frFlamingo  = lipgloss.Color("#eebebe")
	frPink      = lipgloss.Color("#f4b8e4")
	frMauve     = lipgloss.Color("#ca9ee6")
	frRed       = lipgloss.Color("#e78284")
	frMaroon    = lipgloss.Color("#ea999c")
	frPeach     = lipgloss.Color("#ef9f76")
	frYellow    = lipgloss.Color("#e5c890")
	frGreen     = lipgloss.Color("#a6d189")
	frTeal      = lipgloss.Color("#81c8be")
	frSky       = lipgloss.Color("#99d1db")
	frSapphire  = lipgloss.Color("#85c1dc")
	frBlue      = lipgloss.Color("#8caaee")
	frLavender  = lipgloss.Color("#babbf1")
	frText      = lipgloss.Color("#c6d0f5")
	frSubtext1  = lipgloss.Color("#b5bfe2")
	frSubtext0  = lipgloss.Color("#a5adce")
	frOverlay2  = lipgloss.Color("#949cbb")
	frOverlay1  = lipgloss.Color("#838ba7")
	frOverlay0  = lipgloss.Color("#737994")
	frSurface2  = lipgloss.Color("#626880")
	frSurface1  = lipgloss.Color("#51576d")
	frSurface0  = lipgloss.Color("#414559")
	frBase      = lipgloss.Color("#303446")
	frMantle    = lipgloss.Color("#292c3c")
	frCrust     = lipgloss.Color("#232634")
)

// Frappe returns a theme built from the Catppuccin Frappe palette.
// Every color is a native Frappe swatch — designed to feel like the
// terminal theme extended into the dashboard, not painted over it.
func Frappe() *Theme {
	t := &Theme{
		Name: "frappe",

		// Severity gradient: sapphire (cool) → yellow (warm) → red (hot)
		GradientLow:  mustHex("#85c1dc"),
		GradientMid:  mustHex("#e5c890"),
		GradientHigh: mustHex("#e78284"),

		// Overall thermal gradient (headline bar)
		OverallGradient: [5]ThermalLevel{
			{Fg: frSurface2, Bg: frMantle}, // cold: dim surface on mantle
			{Fg: frBlue, Bg: frMantle},     // cool: blue
			{Fg: frYellow, Bg: frSurface0}, // warm: yellow
			{Fg: frPeach, Bg: frSurface1},  // hot: peach
			{Fg: frRed, Bg: frSurface2},    // critical: red on raised surface
		},

		// Category thermal gradient (category boxes)
		CategoryGradient: [5]ThermalLevel{
			{Fg: frSurface2, Bg: frMantle}, // cold: barely visible
			{Fg: frLavender, Bg: frMantle}, // cool: lavender hint
			{Fg: frYellow, Bg: frSurface0}, // warm: yellow
			{Fg: frPeach, Bg: frSurface1},  // hot: peach
			{Fg: frRed, Bg: frSurface2},    // critical: red
		},

		// Threat colors
		ThreatColors: [4]color.Color{
			frBlue,   // Cool: calm blue
			frYellow, // Warm: attention yellow
			frPeach,  // Hot: urgent peach
			frRed,    // Meltdown: red
		},

		// Session phase escalation
		SessionPhase: SessionPhaseColors{
			Idle:      frOverlay0, // muted
			Active:    frBlue,     // calm
			Language:  frYellow,   // canary
			Build:     frPeach,    // active
			Explosion: frRed,      // alarm
		},

		// Gauge dots — 4 distinct Catppuccin accent colors
		GaugeDots: [4]GaugeDotColor{
			{Char: "●", Color: frLavender}, // CPU: lavender
			{Char: "●", Color: frTeal},     // MEM: teal
			{Char: "●", Color: frMauve},    // COMP: mauve
			{Char: "●", Color: frGreen},    // GPU: green
		},

		// Accent: sapphire — matches the cool-state sparkline color
		AccentR: 133.0,
		AccentG: 193.0,
		AccentB: 220.0,

		// Offline state — sky on mantle
		OfflineFg: frCrust,
		OfflineBg: frSurface1,
		OfflineSparkColors: []colorful.Color{
			mustHex("#8caaee"), // blue
			mustHex("#85c1dc"), // sapphire
			mustHex("#99d1db"), // sky
			mustHex("#81c8be"), // teal
			mustHex("#a6d189"), // green
			mustHex("#babbf1"), // lavender
		},

		// Chrome — native Frappe text hierarchy
		DimColor:  frOverlay0, // muted but readable on base
		HelpColor: frSubtext0, // secondary text
		IdleColor: frSapphire, // cool accent for idle state

		// Rate display
		SpawnColor: frPeach,    // warm = spawning
		DeathColor: frSapphire, // cool = dying
		NetColor:   frText,     // neutral
	}

	t.Init()
	return t
}
