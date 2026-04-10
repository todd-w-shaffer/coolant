package theme

import (
	"strings"
	"testing"
)

func TestClassicInit(t *testing.T) {
	th := Classic()

	// LUTs must be populated (non-empty strings)
	for i := 0; i <= 100; i++ {
		if th.lowMidLUT[i] == "" {
			t.Fatalf("lowMidLUT[%d] is empty", i)
		}
		if th.midHighLUT[i] == "" {
			t.Fatalf("midHighLUT[%d] is empty", i)
		}
	}

	// Cached ANSI strings
	if th.lowANSI == "" {
		t.Fatal("lowANSI is empty")
	}
	if th.highANSI == "" {
		t.Fatal("highANSI is empty")
	}

	// GaugeDots must have formatted strings
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

	// All ThermalLevel fields must be non-nil
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

	// ThreatColors
	for i, c := range th.ThreatColors {
		if c == nil {
			t.Fatalf("ThreatColors[%d] is nil", i)
		}
	}

	// SessionPhase
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

	// Offline
	if th.OfflineFg == nil {
		t.Fatal("OfflineFg is nil")
	}
	if th.OfflineBg == nil {
		t.Fatal("OfflineBg is nil")
	}
	if len(th.OfflineSparkColors) == 0 {
		t.Fatal("OfflineSparkColors is empty")
	}

	// Chrome
	if th.DimColor == nil {
		t.Fatal("DimColor is nil")
	}
	if th.HelpColor == nil {
		t.Fatal("HelpColor is nil")
	}

	// Rate colors
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
	want := []string{"classic", "frappe", "iron", "mono"}
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
