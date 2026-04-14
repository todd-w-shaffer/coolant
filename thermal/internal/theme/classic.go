package theme

import (
	"image/color"

	colorful "github.com/lucasb-eyer/go-colorful"

	"charm.land/lipgloss/v2"
)

// Classic returns the default theme -- backward compatible with the
// original hardcoded color values. Traffic-light severity, Anthropic
// orange accents, proven and readable.
func Classic() *Theme {
	t := &Theme{
		Name: "classic",

		// Severity gradient anchors
		GradientLow:  mustHex("#22c55e"), // green
		GradientMid:  mustHex("#eab308"), // yellow
		GradientHigh: mustHex("#ef4444"), // red

		// Overall thermal gradient (headline bar): 5 levels
		OverallGradient: [5]ThermalLevel{
			{Fg: lipgloss.Color("236"), Bg: lipgloss.Color("233")}, // cold
			{Fg: lipgloss.Color("2"), Bg: lipgloss.Color("233")},   // cool: green
			{Fg: lipgloss.Color("3"), Bg: lipgloss.Color("234")},   // warm: yellow
			{Fg: lipgloss.Color("208"), Bg: lipgloss.Color("235")}, // hot: orange
			{Fg: lipgloss.Color("196"), Bg: lipgloss.Color("52")},  // critical: red on dark red
		},

		// Category thermal gradient (category boxes): 5 levels
		CategoryGradient: [5]ThermalLevel{
			{Fg: lipgloss.Color("236"), Bg: lipgloss.Color("233")}, // cold
			{Fg: lipgloss.Color("180"), Bg: lipgloss.Color("234")}, // cool: dim amber
			{Fg: lipgloss.Color("214"), Bg: lipgloss.Color("235")}, // warm: orange
			{Fg: lipgloss.Color("208"), Bg: lipgloss.Color("236")}, // hot: bright orange
			{Fg: lipgloss.Color("196"), Bg: lipgloss.Color("52")},  // critical: red on dark red
		},

		// Threat colors indexed by ThreatLevel
		ThreatColors: [4]color.Color{
			lipgloss.Color("2"),   // Cool: green
			lipgloss.Color("3"),   // Warm: yellow
			lipgloss.Color("208"), // Hot: orange
			lipgloss.Color("1"),   // Meltdown: red
		},

		// Session phase escalation colors
		SessionPhase: SessionPhaseColors{
			Idle:      lipgloss.Color("245"), // gray
			Active:    lipgloss.Color("2"),   // green
			Language:  lipgloss.Color("3"),   // yellow
			Build:     lipgloss.Color("208"), // orange
			Explosion: lipgloss.Color("196"), // red
		},

		// Gauge dots: CPU (white), MEM (cyan), COMP (magenta), GPU (green)
		// ANSIOverride preserves 16-color escapes that render as the terminal's palette colors.
		GaugeDots: [4]GaugeDotColor{
			{Char: "\u25cf", Color: lipgloss.Color("7"), ANSIOverride: "\033[37m"}, // CPU: white
			{Char: "\u25cf", Color: lipgloss.Color("6"), ANSIOverride: "\033[36m"}, // MEM: cyan
			{Char: "\u25cf", Color: lipgloss.Color("5"), ANSIOverride: "\033[35m"}, // COMP: magenta
			{Char: "\u25cf", Color: lipgloss.Color("2"), ANSIOverride: "\033[32m"}, // GPU: green
		},

		// Accent: Anthropic orange
		AccentR: 232.0,
		AccentG: 115.0,
		AccentB: 74.0,

		// Offline state — neutral pre-data backdrop (see iron.go for
		// rationale; vivid backdrops flash on startup).
		OfflineFg: lipgloss.Color("245"),
		OfflineBg: lipgloss.Color("235"),
		OfflineSparkColors: []colorful.Color{
			mustHex("#ff0000"), // red
			mustHex("#ffff00"), // yellow
			mustHex("#00ff00"), // green
			mustHex("#00ffff"), // cyan
			mustHex("#0000ff"), // blue
			mustHex("#ff00ff"), // magenta
		},

		// Chrome
		DimColor:  lipgloss.Color("8"),   // gray
		HelpColor: lipgloss.Color("250"), // light gray
		IdleColor: lipgloss.Color("6"),   // cyan — cool/idle accent

		// Rate display
		SpawnColor: lipgloss.Color("208"), // orange
		DeathColor: lipgloss.Color("6"),   // cyan
		NetColor:   lipgloss.Color("7"),   // white

		// Heat bloom ramp — traffic-light aesthetic, Classic palette.
		BloomRamp: [4]BloomRampStop{
			{Core: mustHex("#3b82f6"), Edge: mustHex("#1e3a8a")}, // COOL: blue
			{Core: mustHex("#eab308"), Edge: mustHex("#92400e")}, // WARM: amber
			{Core: mustHex("#f97316"), Edge: mustHex("#b45309")}, // HOT: orange
			{Core: mustHex("#ef4444"), Edge: mustHex("#7f1d1d")}, // MELTDOWN: red
		},
	}

	t.Init()
	return t
}
