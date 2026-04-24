package theme

import (
	"image/color"

	colorful "github.com/lucasb-eyer/go-colorful"

	"charm.land/lipgloss/v2"
)

// Catppuccin Latte palette — upstream hex values. Latte is Catppuccin's
// light flavor; coolant's first light theme. Terminal requirement: 24-bit
// truecolor. On 16/256-color fallback the hex palette collapses into
// muddied severity hues — users on legacy terminals should stay on
// classic.
var (
	laRosewater = lipgloss.Color("#dc8a78")
	laFlamingo  = lipgloss.Color("#dd7878")
	laPink      = lipgloss.Color("#ea76cb")
	laMauve     = lipgloss.Color("#8839ef")
	laRed       = lipgloss.Color("#d20f39")
	laMaroon    = lipgloss.Color("#e64553")
	laPeach     = lipgloss.Color("#fe640b")
	laYellow    = lipgloss.Color("#df8e1d")
	laGreen     = lipgloss.Color("#40a02b")
	laTeal      = lipgloss.Color("#179299")
	laSky       = lipgloss.Color("#04a5e5")
	laSapphire  = lipgloss.Color("#209fb5")
	laBlue      = lipgloss.Color("#1e66f5")
	laLavender  = lipgloss.Color("#7287fd")
	laText      = lipgloss.Color("#4c4f69")
	laSubtext1  = lipgloss.Color("#5c5f77")
	laSubtext0  = lipgloss.Color("#6c6f85")
	laOverlay2  = lipgloss.Color("#7c7f93")
	laOverlay1  = lipgloss.Color("#8c8fa1")
	laOverlay0  = lipgloss.Color("#9ca0b0")
	laSurface2  = lipgloss.Color("#acb0be")
	laSurface1  = lipgloss.Color("#bcc0cc")
	laSurface0  = lipgloss.Color("#ccd0da")
	laBase      = lipgloss.Color("#eff1f5")
	laMantle    = lipgloss.Color("#e6e9ef")
	laCrust     = lipgloss.Color("#dce0e8")
)

// Latte returns a theme built from the Catppuccin Latte palette — coolant's
// first light-mode theme. Severity anchors are darkened variants (green,
// peach, red) chosen so digit-dense readouts stay honest against laBase.
// Yellow demotes to single-glyph roles (threat pill, session diamond) where
// letterform parsing isn't required.
func Latte() *Theme {
	t := &Theme{
		Name: "latte",

		// Severity gradient: green → peach → red. Yellow on laBase is
		// ~2.3:1 contrast — fails WCAG AA on numeric readouts where
		// letterform parsing matters. Peach is ~2.8:1 with much stronger
		// hue separation; stays honest as a "getting warm" color.
		GradientLow:  mustHex("#40a02b"), // laGreen
		GradientMid:  mustHex("#fe640b"), // laPeach
		GradientHigh: mustHex("#d20f39"), // laRed

		// Overall thermal gradient (headline bar). Bgs step surface →
		// mantle → subtle warm tints; bg *warms* with heat (inverse of
		// dark themes where bg lightens).
		OverallGradient: [5]ThermalLevel{
			{Fg: laOverlay0, Bg: laBase}, // cold: dim on base
			{Fg: laBlue, Bg: laBase},     // cool: blue
			{Fg: laPeach, Bg: laMantle},  // warm: peach (yellow fails the same WCAG floor)
			{Fg: laRed, Bg: laSurface0},  // hot: red on raised surface
			{Fg: laRed, Bg: laSurface1},  // critical: red on most-raised
		},

		// Category thermal gradient
		CategoryGradient: [5]ThermalLevel{
			{Fg: laOverlay0, Bg: laBase},
			{Fg: laLavender, Bg: laBase}, // cool: lavender hint
			{Fg: laPeach, Bg: laMantle},  // warm: peach
			{Fg: laRed, Bg: laSurface0},  // hot: red
			{Fg: laRed, Bg: laSurface1},  // critical: red
		},

		// Rail-only critical override — laRed solo on iconBg reads
		// hot-bright on light surface; laMaroon is Catppuccin's softer
		// red (same alarm register, lower amplitude).
		RailCriticalOverride: laMaroon,

		// Threat colors
		ThreatColors: [4]color.Color{
			laBlue,   // Cool
			laYellow, // Warm (single-glyph pill — letterform parsing not required)
			laPeach,  // Hot
			laRed,    // Meltdown
		},

		// Session phase escalation
		SessionPhase: SessionPhaseColors{
			Idle:      laOverlay0, // muted
			Active:    laBlue,     // calm
			Language:  laYellow,   // canary
			Build:     laPeach,    // active
			Explosion: laRed,      // alarm
		},

		// Gauge dots — 4 distinct Catppuccin accent colors, hex-native
		// (no ANSIOverride — Frappe's pattern).
		GaugeDots: [4]GaugeDotColor{
			{Char: "●", Color: laLavender}, // CPU
			{Char: "●", Color: laTeal},     // MEM
			{Char: "●", Color: laMauve},    // COMP
			{Char: "●", Color: laGreen},    // GPU
		},

		// Accent: laBlue. Sits outside the severity band (green → peach
		// → red); saturated/dark enough that brightness=1 asserts on
		// laBase. laSapphire and laLavender trade chroma for softness
		// and read as washed on light surface.
		AccentR: 30.0,
		AccentG: 102.0,
		AccentB: 245.0,

		// Offline state — quip text on palette-native mantle. Skipped
		// by TestOfflineBgIsNeutral (opts out by omission, same
		// mechanism as frappe).
		OfflineFg: laSubtext1,
		OfflineBg: laMantle,
		OfflineSparkColors: []colorful.Color{
			mustHex("#1e66f5"), // blue
			mustHex("#209fb5"), // sapphire
			mustHex("#04a5e5"), // sky
			mustHex("#179299"), // teal
			mustHex("#40a02b"), // green
			mustHex("#7287fd"), // lavender
		},

		// Chrome — direction inverts vs dark themes. On light bg, Dim
		// is darker than text (recedes) and Help is closer to text
		// (readable). The mode-agnostic rule enforced cross-theme is
		// "Dim recedes from bg more than Help does."
		DimColor:  laOverlay0, // recedes (lighter, closer to laBase bg)
		HelpColor: laSubtext1, // closer to text, readable
		IdleColor: laSapphire, // cool accent for idle state

		// Rate display — laSapphire (not laBlue) for DeathColor by
		// design: dying is a fade-out signal, sapphire is softer than
		// the breathing-accent blue. NetColor uses subtext1 for neutral
		// readability.
		SpawnColor: laPeach,
		DeathColor: laSapphire,
		NetColor:   laSubtext1,

		// Heat bloom ramp — Catppuccin accents, cool→meltdown across 4
		// stops. All six Core/Edge stops sit at luminance 0.13–0.30
		// against laBase ≈ 0.88; ample distance for the bg-aware bloom
		// alpha-composite to read at every heat level.
		BloomRamp: [4]BloomRampStop{
			{Core: mustHex("#1e66f5"), Edge: mustHex("#7287fd")}, // COOL: blue → lavender
			{Core: mustHex("#df8e1d"), Edge: mustHex("#fe640b")}, // WARM: yellow → peach
			{Core: mustHex("#fe640b"), Edge: mustHex("#d20f39")}, // HOT:  peach → red
			{Core: mustHex("#d20f39"), Edge: mustHex("#e64553")}, // MELTDOWN: red → maroon
		},
	}

	t.Init()
	return t
}
