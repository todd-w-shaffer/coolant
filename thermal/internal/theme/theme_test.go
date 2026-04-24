package theme

import (
	"fmt"
	"image/color"
	"math"
	"strings"
	"testing"
)

// relLuminance returns the WCAG 2.x relative luminance of a color.Color,
// in [0,1].
func relLuminance(c color.Color) float64 {
	r, g, b, _ := c.RGBA()
	chanLin := func(v uint32) float64 {
		x := float64(v) / 65535.0
		if x <= 0.03928 {
			return x / 12.92
		}
		return math.Pow((x+0.055)/1.055, 2.4)
	}
	return 0.2126*chanLin(r) + 0.7152*chanLin(g) + 0.0722*chanLin(b)
}

// contrastRatio returns WCAG contrast ratio between two colors.
func contrastRatio(a, b color.Color) float64 {
	la, lb := relLuminance(a), relLuminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// TestDimRecedesMoreThanHelp is a cross-theme invariant: DimColor's luminance
// distance from OverallGradient[0].Bg must be smaller than HelpColor's. That
// is, on any backdrop, dim recedes more than help does. Mode-agnostic —
// dark themes have dim above bg, light themes have dim below bg.
func TestDimRecedesMoreThanHelp(t *testing.T) {
	for _, name := range Names() {
		th, err := Get(name)
		if err != nil {
			t.Fatalf("Get(%q): %v", name, err)
		}
		bgLum := relLuminance(th.OverallGradient[0].Bg)
		dimDist := math.Abs(relLuminance(th.DimColor) - bgLum)
		helpDist := math.Abs(relLuminance(th.HelpColor) - bgLum)
		if dimDist >= helpDist {
			t.Errorf("theme %q: dim distance (%.3f) should be < help distance (%.3f) from bg (lum=%.3f)",
				name, dimDist, helpDist, bgLum)
		}
	}
}

// TestColdIsOnlyRecessedLevel is a cross-theme invariant: cold (level 0) has
// the lowest Fg/Bg contrast among non-alarm levels; levels 1..3 each assert
// more than cold. Level [4] is exempt because alarm-chamber designs (Classic:
// red on dark-red; Frappe: red on raised grey surface) intentionally reduce
// Fg/Bg contrast to shift the alarm register from contrast to hue
// saturation — that's a design pattern, not a bug. Cold's contrast must be
// ≥ 1.25:1 so it recedes rather than disappears entirely (low-vision floor).
func TestColdIsOnlyRecessedLevel(t *testing.T) {
	const coldFloor = 1.25
	for _, name := range Names() {
		th, err := Get(name)
		if err != nil {
			t.Fatalf("Get(%q): %v", name, err)
		}
		coldContrast := contrastRatio(th.OverallGradient[0].Fg, th.OverallGradient[0].Bg)
		if coldContrast < coldFloor {
			t.Errorf("theme %q: cold contrast %.3f < floor %.3f (cold recedes, not disappears)",
				name, coldContrast, coldFloor)
		}
		for i := 1; i <= 3; i++ {
			c := contrastRatio(th.OverallGradient[i].Fg, th.OverallGradient[i].Bg)
			if c <= coldContrast {
				t.Errorf("theme %q: level[%d] contrast %.3f <= cold %.3f (non-alarm levels must assert)",
					name, i, c, coldContrast)
			}
		}
	}
}

// TestDimAndHelpStayDistinct is a cross-theme invariant: DimColor and
// HelpColor must stay perceptually separable so the focused-intel overlay's
// dim() field labels don't flatten against its ct() data values. Floor is
// 1.9:1 — the real-world minimum across existing dark themes. Latte was the
// motivator (its savor pass surfaced the overlay failure mode); its
// laOverlay0/laSubtext1 pairing clears the bar comfortably at ~2.4:1.
func TestDimAndHelpStayDistinct(t *testing.T) {
	const floor = 1.9
	for _, name := range Names() {
		th, err := Get(name)
		if err != nil {
			t.Fatalf("Get(%q): %v", name, err)
		}
		c := contrastRatio(th.DimColor, th.HelpColor)
		if c < floor {
			t.Errorf("theme %q: contrast(Dim, Help) = %.3f, want >= %.3f",
				name, c, floor)
		}
	}
}

// colorString stringifies a color for assertion. lipgloss.Color in v2 is
// a string alias, so %v yields the ANSI code or hex literal.
func colorString(c interface{}) string {
	return fmt.Sprintf("%v", c)
}

func TestAllThemesProvideBloomRamp(t *testing.T) {
	var zero BloomRampStop
	for _, name := range Names() {
		th, err := Get(name)
		if err != nil {
			t.Fatalf("Get(%q): %v", name, err)
		}
		for i, stop := range th.BloomRamp {
			if stop == zero {
				t.Errorf("theme %q BloomRamp[%d] is zero", name, i)
			}
		}
		// LUT endpoints must round-trip through BloomColor.
		low := th.BloomColor(0, 0)
		if low.R == 0 && low.G == 0 && low.B == 0 {
			t.Errorf("theme %q BloomColor(0,0) is pure black — LUT likely unpopulated", name)
		}
		hot := th.BloomColor(1, 0)
		if hot.R == 0 && hot.G == 0 && hot.B == 0 {
			t.Errorf("theme %q BloomColor(1,0) is pure black — LUT likely unpopulated", name)
		}
	}
}

// TestOfflineBgIsNeutral guards against vivid pre-data backdrops. The
// offline frame is a transient that ships before the first snapshot
// lands; saturated theme colors here read as a "flash" on startup
// (iron's dark magenta, classic's steel blue). Each themed pre-data
// backdrop must be a dim neutral (ANSI 234-236) — frappe and latte are
// both exempt by omission (not allowlist): their Catppuccin surfaces
// (frMantle, laMantle) are palette-native perceptual neutrals, so the
// vivid-flash failure mode this test catches does not apply.
func TestOfflineBgIsNeutral(t *testing.T) {
	allowed := map[string]bool{"234": true, "235": true, "236": true}
	for _, name := range []string{"classic", "iron", "mono"} {
		th, err := Get(name)
		if err != nil {
			t.Fatalf("Get(%q): %v", name, err)
		}
		s := colorString(th.OfflineBg)
		if !allowed[s] {
			t.Errorf("theme %q OfflineBg = %q; want one of 234/235/236 (dim neutral)", name, s)
		}
	}
}

// TestAllThemesInitialize walks every registered theme and asserts every
// exported palette field is non-nil / non-empty after Init. Replaces per-theme
// init tests — every new theme inherits coverage just by landing in Registry.
func TestAllThemesInitialize(t *testing.T) {
	for _, name := range Names() {
		t.Run(name, func(t *testing.T) {
			th, err := Get(name)
			if err != nil {
				t.Fatalf("Get(%q): %v", name, err)
			}
			assertThemeInitialized(t, th)
		})
	}
}

func assertThemeInitialized(t *testing.T, th *Theme) {
	t.Helper()

	for i := 0; i <= 100; i++ {
		if th.lowMidLUT[i] == "" {
			t.Fatalf("lowMidLUT[%d] is empty", i)
		}
		if th.midHighLUT[i] == "" {
			t.Fatalf("midHighLUT[%d] is empty", i)
		}
	}
	if th.lowANSI == "" {
		t.Fatal("lowANSI is empty")
	}
	if th.highANSI == "" {
		t.Fatal("highANSI is empty")
	}

	for i, dot := range th.GaugeDots {
		if dot.Formatted == "" {
			t.Fatalf("GaugeDots[%d].Formatted is empty", i)
		}
		if dot.ANSI == "" {
			t.Fatalf("GaugeDots[%d].ANSI is empty", i)
		}
		if dot.Char == "" {
			t.Fatalf("GaugeDots[%d].Char is empty", i)
		}
		if dot.Color == nil {
			t.Fatalf("GaugeDots[%d].Color is nil", i)
		}
	}

	for i, level := range th.OverallGradient {
		if level.Fg == nil {
			t.Fatalf("OverallGradient[%d].Fg is nil", i)
		}
		if level.Bg == nil {
			t.Fatalf("OverallGradient[%d].Bg is nil", i)
		}
	}
	for i, level := range th.CategoryGradient {
		if level.Fg == nil {
			t.Fatalf("CategoryGradient[%d].Fg is nil", i)
		}
		if level.Bg == nil {
			t.Fatalf("CategoryGradient[%d].Bg is nil", i)
		}
	}

	for i, c := range th.ThreatColors {
		if c == nil {
			t.Fatalf("ThreatColors[%d] is nil", i)
		}
	}

	if th.SessionPhase.Idle == nil {
		t.Fatal("SessionPhase.Idle is nil")
	}
	if th.SessionPhase.Active == nil {
		t.Fatal("SessionPhase.Active is nil")
	}
	if th.SessionPhase.Language == nil {
		t.Fatal("SessionPhase.Language is nil")
	}
	if th.SessionPhase.Build == nil {
		t.Fatal("SessionPhase.Build is nil")
	}
	if th.SessionPhase.Explosion == nil {
		t.Fatal("SessionPhase.Explosion is nil")
	}

	if th.OfflineFg == nil {
		t.Fatal("OfflineFg is nil")
	}
	if th.OfflineBg == nil {
		t.Fatal("OfflineBg is nil")
	}
	if len(th.OfflineSparkColors) == 0 {
		t.Fatal("OfflineSparkColors is empty")
	}

	if th.DimColor == nil {
		t.Fatal("DimColor is nil")
	}
	if th.HelpColor == nil {
		t.Fatal("HelpColor is nil")
	}

	if th.SpawnColor == nil {
		t.Fatal("SpawnColor is nil")
	}
	if th.DeathColor == nil {
		t.Fatal("DeathColor is nil")
	}
	if th.NetColor == nil {
		t.Fatal("NetColor is nil")
	}
}

func TestSeverityColor(t *testing.T) {
	th := Classic()

	tests := []struct {
		name   string
		value  float64
		thresh *SparkThresholds
		prefix string // expected ANSI escape prefix
	}{
		{
			name:   "nil thresh returns low color",
			value:  50.0,
			thresh: nil,
			prefix: "\033[38;2;",
		},
		{
			name:   "zero value returns low end",
			value:  0,
			thresh: &SparkThresholds{Warn: 70, Crit: 90},
			prefix: "\033[38;2;",
		},
		{
			name:   "at warn boundary uses mid-high LUT",
			value:  70.0,
			thresh: &SparkThresholds{Warn: 70, Crit: 90},
			prefix: "\033[38;2;",
		},
		{
			name:   "above crit returns high color",
			value:  95.0,
			thresh: &SparkThresholds{Warn: 70, Crit: 90},
			prefix: "\033[38;2;",
		},
		{
			name:   "exactly at crit returns high color",
			value:  90.0,
			thresh: &SparkThresholds{Warn: 70, Crit: 90},
			prefix: "\033[38;2;",
		},
		{
			name:   "mid range below warn",
			value:  35.0,
			thresh: &SparkThresholds{Warn: 70, Crit: 90},
			prefix: "\033[38;2;",
		},
		{
			name:   "mid range between warn and crit",
			value:  80.0,
			thresh: &SparkThresholds{Warn: 70, Crit: 90},
			prefix: "\033[38;2;",
		},
		{
			name:   "zero warn threshold",
			value:  5.0,
			thresh: &SparkThresholds{Warn: 0, Crit: 90},
			prefix: "\033[38;2;",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := th.SeverityColor(tt.value, tt.thresh)
			if !strings.HasPrefix(got, tt.prefix) {
				t.Errorf("SeverityColor(%v, %v) = %q, want prefix %q", tt.value, tt.thresh, got, tt.prefix)
			}
		})
	}

	// Verify identical output to the expected LUT values at boundaries
	thresh := &SparkThresholds{Warn: 70, Crit: 90}

	// At zero, should equal lowANSI (the green anchor)
	atZero := th.SeverityColor(0, thresh)
	if atZero != th.lowANSI {
		t.Errorf("SeverityColor(0) = %q, want lowANSI %q", atZero, th.lowANSI)
	}

	// Above crit, should equal highANSI (the red anchor)
	aboveCrit := th.SeverityColor(100, thresh)
	if aboveCrit != th.highANSI {
		t.Errorf("SeverityColor(100) = %q, want highANSI %q", aboveCrit, th.highANSI)
	}

	// At warn boundary, should equal midHighLUT[0] (start of mid->high)
	atWarn := th.SeverityColor(70, thresh)
	if atWarn != th.midHighLUT[0] {
		t.Errorf("SeverityColor(70) = %q, want midHighLUT[0] %q", atWarn, th.midHighLUT[0])
	}
}

func TestRegistryGet(t *testing.T) {
	tests := []struct {
		name    string
		theme   string
		wantErr bool
	}{
		{name: "classic exists", theme: "classic", wantErr: false},
		{name: "unknown returns error", theme: "nonexistent", wantErr: true},
		{name: "empty name returns error", theme: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			th, err := Get(tt.theme)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Get(%q) expected error, got nil", tt.theme)
				}
				return
			}
			if err != nil {
				t.Fatalf("Get(%q) unexpected error: %v", tt.theme, err)
			}
			if th.Name != tt.theme {
				t.Errorf("Get(%q).Name = %q", tt.theme, th.Name)
			}
		})
	}
}

func TestRegistryNames(t *testing.T) {
	names := Names()
	want := []string{"classic", "frappe", "iron", "latte", "mono"}
	if len(names) != len(want) {
		t.Fatalf("Names() returned %d entries, want %d: %v", len(names), len(want), names)
	}
	for i, name := range names {
		if name != want[i] {
			t.Errorf("Names()[%d] = %q, want %q", i, name, want[i])
		}
	}
}

func TestGetUnknownThemeReturnsError(t *testing.T) {
	_, err := Get("cyberpunk")
	if err == nil {
		t.Fatal("Get(\"cyberpunk\") should return error for unknown theme")
	}
	if !strings.Contains(err.Error(), "cyberpunk") {
		t.Errorf("error should mention the unknown name, got: %v", err)
	}
	if !strings.Contains(err.Error(), "classic") {
		t.Errorf("error should list available themes, got: %v", err)
	}
}

func TestClassicIdleColorSet(t *testing.T) {
	th := Classic()
	if th.IdleColor == nil {
		t.Fatal("IdleColor is nil")
	}
}

func TestGaugeDotANSIOverride(t *testing.T) {
	th := Classic()
	// Classic uses ANSIOverride for 16-color compat
	for i, dot := range th.GaugeDots {
		if dot.ANSIOverride == "" {
			t.Fatalf("GaugeDots[%d].ANSIOverride is empty in Classic", i)
		}
		if dot.ANSI != dot.ANSIOverride {
			t.Errorf("GaugeDots[%d].ANSI = %q, want ANSIOverride %q", i, dot.ANSI, dot.ANSIOverride)
		}
	}
}

func TestGaugeDotTruecolorFallback(t *testing.T) {
	// A theme without ANSIOverride should derive truecolor ANSI
	th := Classic()
	th.GaugeDots[0].ANSIOverride = "" // clear override
	th.Init()
	if th.GaugeDots[0].ANSI == "" {
		t.Fatal("GaugeDots[0].ANSI should be derived via truecolor when no override")
	}
	if !strings.HasPrefix(th.GaugeDots[0].ANSI, "\033[38;2;") {
		t.Errorf("GaugeDots[0].ANSI = %q, want truecolor prefix", th.GaugeDots[0].ANSI)
	}
}
