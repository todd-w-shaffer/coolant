package theme

import (
	"image/color"

	colorful "github.com/lucasb-eyer/go-colorful"

	"charm.land/lipgloss/v2"
)

// Iron returns the FLIR thermal camera theme — blackbody radiation palette.
// Purple through magenta to amber to white-hot. Tuned for dark terminal
// backgrounds (catppuccin frappe, tokyo night, etc.) — cool-state purples
// are lifted to stay visible against ~#303446 surfaces.
func Iron() *Theme {
	t := &Theme{
		Name: "iron",

		// Severity gradient: muted purple → magenta-pink → incandescent amber
		GradientLow:  mustHex("#5b2d8e"),
		GradientMid:  mustHex("#c2185b"),
		GradientHigh: mustHex("#ffcc02"),

		// Overall thermal gradient (headline bar)
		// Backgrounds use 234-236 range to float above dark terminal surfaces
		OverallGradient: [5]ThermalLevel{
			{Fg: lipgloss.Color("236"), Bg: lipgloss.Color("234")}, // cold: dim but visible
			{Fg: lipgloss.Color("99"), Bg: lipgloss.Color("234")},  // cool: medium purple
			{Fg: lipgloss.Color("168"), Bg: lipgloss.Color("235")}, // warm: rose-magenta
			{Fg: lipgloss.Color("208"), Bg: lipgloss.Color("236")}, // hot: amber
			{Fg: lipgloss.Color("229"), Bg: lipgloss.Color("130")}, // critical: white on burnt orange
		},

		// Category thermal gradient (category boxes)
		CategoryGradient: [5]ThermalLevel{
			{Fg: lipgloss.Color("236"), Bg: lipgloss.Color("234")}, // cold: dim
			{Fg: lipgloss.Color("99"), Bg: lipgloss.Color("234")},  // cool: medium purple
			{Fg: lipgloss.Color("133"), Bg: lipgloss.Color("235")}, // warm: orchid
			{Fg: lipgloss.Color("208"), Bg: lipgloss.Color("236")}, // hot: amber
			{Fg: lipgloss.Color("229"), Bg: lipgloss.Color("130")}, // critical: white on burnt orange
		},

		// Rail-only critical override — the paired critical Fg (229, a
		// cream-white) reads as flashbulb-harsh when used as a solo
		// underline color on iconBg. 130 is the burnt orange that
		// served as critical's paired Bg; solo on iconBg it lands as
		// the FLIR blackbody ember tone the whole theme is built
		// around.
		RailCriticalOverride: lipgloss.Color("130"),

		// Threat colors (indexed by ThreatLevel)
		ThreatColors: [4]color.Color{
			lipgloss.Color("99"),  // Cool: medium purple (visible on dark bg)
			lipgloss.Color("168"), // Warm: rose-magenta
			lipgloss.Color("208"), // Hot: amber
			lipgloss.Color("229"), // Meltdown: bright yellow-white
		},

		// Session phase escalation
		SessionPhase: SessionPhaseColors{
			Idle:      lipgloss.Color("240"), // mid-dark gray (visible on frappe)
			Active:    lipgloss.Color("99"),  // medium purple
			Language:  lipgloss.Color("133"), // orchid
			Build:     lipgloss.Color("208"), // amber
			Explosion: lipgloss.Color("229"), // white-hot
		},

		// Gauge dots — purple/magenta/amber family
		GaugeDots: [4]GaugeDotColor{
			{Char: "●", Color: lipgloss.Color("183"), ANSIOverride: "\033[38;5;183m"}, // CPU: light purple
			{Char: "●", Color: lipgloss.Color("168"), ANSIOverride: "\033[38;5;168m"}, // MEM: rose
			{Char: "●", Color: lipgloss.Color("208"), ANSIOverride: "\033[38;5;208m"}, // COMP: amber
			{Char: "●", Color: lipgloss.Color("229"), ANSIOverride: "\033[38;5;229m"}, // GPU: pale yellow
		},

		// Accent: deep rose
		AccentR: 200.0,
		AccentG: 80.0,
		AccentB: 120.0,

		// Offline state — neutral pre-data backdrop. Saturated theme
		// colors here read as a startup "flash" before the first
		// snapshot lands; 235 sits dim under the bloom and any theme.
		OfflineFg: lipgloss.Color("236"),
		OfflineBg: lipgloss.Color("235"),
		OfflineSparkColors: []colorful.Color{
			mustHex("#5b2d8e"), // medium purple
			mustHex("#7b3f9e"), // brighter purple
			mustHex("#a0406e"), // mauve-rose
			mustHex("#c2185b"), // hot pink
			mustHex("#e65100"), // deep orange
			mustHex("#ffab00"), // amber
		},

		// Chrome — lifted for dark bg legibility
		DimColor:  lipgloss.Color("243"), // mid gray (was 237)
		HelpColor: lipgloss.Color("248"), // light gray
		IdleColor: lipgloss.Color("99"),  // medium purple

		// Rate display
		SpawnColor:  lipgloss.Color("208"), // amber
		DeathColor:  lipgloss.Color("99"),  // medium purple
		NetColor:    lipgloss.Color("248"), // light gray
		TokensColor: lipgloss.Color("213"), // pink — token throughput accent

		// Heat bloom ramp — FLIR blackbody aesthetic.
		BloomRamp: [4]BloomRampStop{
			{Core: mustHex("#4c1d95"), Edge: mustHex("#1e1b4b")}, // COOL: deep violet
			{Core: mustHex("#c026d3"), Edge: mustHex("#6b21a8")}, // WARM: magenta
			{Core: mustHex("#f97316"), Edge: mustHex("#9a3412")}, // HOT: amber
			{Core: mustHex("#fcd34d"), Edge: mustHex("#d97706")}, // MELTDOWN: white-hot yellow
		},
	}

	t.Init()
	return t
}
