package theme

import (
	"image/color"

	colorful "github.com/lucasb-eyer/go-colorful"

	"charm.land/lipgloss/v2"
)

// Mono returns a single-hue theme where heat is communicated through
// brightness alone. Warm amber base — cold is dim, hot is blazing.
// Accessibility-friendly: works without color as an information channel.
// Tuned for dark terminal backgrounds.
func Mono() *Theme {
	t := &Theme{
		Name: "mono",

		// Severity gradient: dim amber → warm medium → bright moccasin
		GradientLow:  mustHex("#5a4a35"),
		GradientMid:  mustHex("#b8956a"),
		GradientHigh: mustHex("#ffe4b5"),

		// Overall thermal gradient — brightness ramp, one hue family
		OverallGradient: [5]ThermalLevel{
			{Fg: lipgloss.Color("238"), Bg: lipgloss.Color("234")}, // cold: barely there
			{Fg: lipgloss.Color("137"), Bg: lipgloss.Color("234")}, // cool: dim khaki
			{Fg: lipgloss.Color("179"), Bg: lipgloss.Color("235")}, // warm: medium amber
			{Fg: lipgloss.Color("222"), Bg: lipgloss.Color("236")}, // hot: bright gold
			{Fg: lipgloss.Color("230"), Bg: lipgloss.Color("94")},  // critical: white on deep amber
		},

		// Category gradient — same as overall (mono = no category color distinction)
		CategoryGradient: [5]ThermalLevel{
			{Fg: lipgloss.Color("238"), Bg: lipgloss.Color("234")},
			{Fg: lipgloss.Color("137"), Bg: lipgloss.Color("234")},
			{Fg: lipgloss.Color("179"), Bg: lipgloss.Color("235")},
			{Fg: lipgloss.Color("222"), Bg: lipgloss.Color("236")},
			{Fg: lipgloss.Color("230"), Bg: lipgloss.Color("94")},
		},

		// Rail-only critical override — 230 is blazing near-white; solo
		// on iconBg it spikes too bright for a single-hue theme whose
		// identity is "amber at different intensities." 166 is a deep
		// orange that reads clearly as "hotter than the 222 hot step"
		// while staying in the amber/ember hue register.
		RailCriticalOverride: lipgloss.Color("166"),

		// Threat colors — brightness only
		ThreatColors: [4]color.Color{
			lipgloss.Color("137"), // Cool: dim
			lipgloss.Color("179"), // Warm: medium
			lipgloss.Color("222"), // Hot: bright
			lipgloss.Color("230"), // Meltdown: blazing
		},

		// Session phases — same brightness ramp
		SessionPhase: SessionPhaseColors{
			Idle:      lipgloss.Color("240"), // gray
			Active:    lipgloss.Color("137"), // dim khaki
			Language:  lipgloss.Color("179"), // medium amber
			Build:     lipgloss.Color("222"), // bright gold
			Explosion: lipgloss.Color("230"), // blazing white
		},

		// Gauge dots — 4 brightness levels of the same amber family.
		// Differentiated by brightness, not hue. Read labels, not colors.
		GaugeDots: [4]GaugeDotColor{
			{Char: "●", Color: lipgloss.Color("223"), ANSIOverride: "\033[38;5;223m"}, // CPU: bright cream
			{Char: "●", Color: lipgloss.Color("179"), ANSIOverride: "\033[38;5;179m"}, // MEM: medium amber
			{Char: "●", Color: lipgloss.Color("137"), ANSIOverride: "\033[38;5;137m"}, // COMP: dim khaki
			{Char: "●", Color: lipgloss.Color("101"), ANSIOverride: "\033[38;5;101m"}, // GPU: dark olive
		},

		// Accent: warm amber glow
		AccentR: 200.0,
		AccentG: 170.0,
		AccentB: 120.0,

		// Offline — dim amber scatter
		OfflineFg: lipgloss.Color("238"),
		OfflineBg: lipgloss.Color("235"),
		OfflineSparkColors: []colorful.Color{
			mustHex("#3d3225"),
			mustHex("#5a4a35"),
			mustHex("#7a6a50"),
			mustHex("#9a8a6a"),
			mustHex("#b8956a"),
			mustHex("#d4b896"),
		},

		// Chrome
		DimColor:  lipgloss.Color("243"),
		HelpColor: lipgloss.Color("248"),
		IdleColor: lipgloss.Color("137"), // dim khaki

		// Rates — brightness differentiated
		SpawnColor: lipgloss.Color("222"), // bright (spawns = heat)
		DeathColor: lipgloss.Color("137"), // dim (deaths = cooling)
		NetColor:   lipgloss.Color("179"), // medium

		// Heat bloom ramp — single-hue amber brightness curve.
		BloomRamp: [4]BloomRampStop{
			{Core: mustHex("#404040"), Edge: mustHex("#1a1a1a")}, // COOL: neutral dim
			{Core: mustHex("#a16207"), Edge: mustHex("#451a03")}, // WARM: faint amber
			{Core: mustHex("#d97706"), Edge: mustHex("#7c2d12")}, // HOT: amber
			{Core: mustHex("#fbbf24"), Edge: mustHex("#b45309")}, // MELTDOWN: bright amber
		},
	}

	t.Init()
	return t
}
