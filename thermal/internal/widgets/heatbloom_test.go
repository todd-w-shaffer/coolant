package widgets

import (
	"flag"
	"fmt"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/toddwshaffer/coolant/thermal/internal/anim"
	"github.com/toddwshaffer/coolant/thermal/internal/config"
	"github.com/toddwshaffer/coolant/thermal/internal/theme"
)

var updateGoldenBloom = flag.Bool("update-bloom-golden", false, "update heatbloom golden files")

func renderBloomRow(b *HeatBloom, row int, fallback color.Color) string {
	var sb strings.Builder
	for col := 0; col < b.width; col++ {
		c := b.BgAt(col, row, fallback)
		r, g, bl, _ := c.RGBA()
		fmt.Fprintf(&sb, "[%03d,%03d,%03d]", r>>8, g>>8, bl>>8)
	}
	return sb.String()
}

func bloomGoldenCheck(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *updateGoldenBloom {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v (run with -update-bloom-golden to create)", path, err)
	}
	if string(want) != got {
		t.Errorf("golden mismatch: %s\nWANT:\n%s\nGOT:\n%s", path, string(want), got)
	}
}

func newTestBloom(t *testing.T) *HeatBloom {
	t.Helper()
	th, err := theme.Get("frappe")
	if err != nil {
		t.Fatalf("theme: %v", err)
	}
	return NewHeatBloom(th, anim.Default())
}

func TestHeatBloomConstruct(t *testing.T) {
	b := newTestBloom(t)
	if b == nil {
		t.Fatal("NewHeatBloom returned nil")
	}
}

func TestHeatBloomSetSizeZero(t *testing.T) {
	b := newTestBloom(t)
	b.SetSize(0, 2)
	fallback := color.RGBA{R: 1, G: 2, B: 3, A: 255}
	got := b.BgAt(0, 0, fallback)
	if got != fallback {
		t.Errorf("zero-size BgAt = %v, want fallback %v", got, fallback)
	}
}

func TestHeatBloomAlphaFalloff(t *testing.T) {
	b := newTestBloom(t)
	b.SetSize(40, 2)
	b.heat = 1.0
	b.breathePhase = math.Pi / 2 // mid-swell
	anchorCol := int(config.BloomAnchorX * float64(b.width))

	centerA := b.alphaAt(anchorCol, 1)
	edgeA := b.alphaAt(0, 1)
	nearBoundary := b.alphaAt(int(config.BloomRightBoundary*40)-1, 1)

	if centerA <= edgeA {
		t.Errorf("center alpha %v should exceed far-left edge alpha %v", centerA, edgeA)
	}
	if centerA <= nearBoundary {
		t.Errorf("center alpha %v should exceed near-boundary alpha %v", centerA, nearBoundary)
	}
	if centerA < 0.5 {
		t.Errorf("center alpha %v should be >= 0.5 at peak heat", centerA)
	}
}

func TestHeatBloomBgAtBlendsInside(t *testing.T) {
	b := newTestBloom(t)
	b.SetSize(40, 2)
	b.heat = 1.0
	b.breathePhase = math.Pi / 2
	fallback := color.RGBA{R: 41, G: 44, B: 60, A: 255}
	anchorCol := int(config.BloomAnchorX * float64(b.width))

	got := b.BgAt(anchorCol, 1, fallback)
	if got == fallback {
		t.Errorf("BgAt at anchor returned fallback; expected blended color")
	}
}

func TestHeatBloomGolden_Heat0(t *testing.T) {
	b := newTestBloom(t)
	b.SetSize(40, 2)
	b.heat = 0
	b.breathePhase = 0
	fallback := color.RGBA{R: 41, G: 44, B: 60, A: 255}
	got := renderBloomRow(b, 0, fallback) + "\n" + renderBloomRow(b, 1, fallback)
	bloomGoldenCheck(t, "heatbloom_heat_0.golden", got)
}

func TestHeatBloomGolden_HeatHalf(t *testing.T) {
	b := newTestBloom(t)
	b.SetSize(40, 2)
	b.heat = 0.5
	b.breathePhase = math.Pi / 2
	fallback := color.RGBA{R: 41, G: 44, B: 60, A: 255}
	got := renderBloomRow(b, 0, fallback) + "\n" + renderBloomRow(b, 1, fallback)
	bloomGoldenCheck(t, "heatbloom_heat_half.golden", got)
}

func TestHeatBloomGolden_Heat1(t *testing.T) {
	b := newTestBloom(t)
	b.SetSize(40, 2)
	b.heat = 1.0
	b.breathePhase = 0
	fallback := color.RGBA{R: 41, G: 44, B: 60, A: 255}
	got := renderBloomRow(b, 0, fallback) + "\n" + renderBloomRow(b, 1, fallback)
	bloomGoldenCheck(t, "heatbloom_heat_1.golden", got)
}

func TestHeatBloomAnimTickClampsHeat(t *testing.T) {
	b := newTestBloom(t)
	b.SetSize(40, 2)
	// Drive a hard step from 0 → 1 and let the spring rip. Underdamped
	// physics will naturally overshoot past 1.0; AnimTick must clamp so
	// the bloom never paints with heat > 1 (the "flashbulb" frame).
	b.heat = 0
	b.heatVel = 0
	b.heatTarget = 1.0
	maxHeat := 0.0
	for i := 0; i < 240; i++ {
		b.AnimTick(false)
		if b.heat > maxHeat {
			maxHeat = b.heat
		}
	}
	if maxHeat > 1.0+1e-9 {
		t.Errorf("heat overshot to %v; want clamped to <= 1.0", maxHeat)
	}
}

func TestHeatBloomFirstUpdatePrimes(t *testing.T) {
	b := newTestBloom(t)
	b.SetSize(40, 2)
	// Mimic the AppState the bloom sees on the first snapshot after boot.
	// First Update must seed heat at the target so the spring starts at
	// rest — no startup ramp, no overshoot. Subsequent Updates must ease.
	b.setTarget(0.8)
	if math.Abs(b.heat-0.8) > 1e-9 {
		t.Errorf("first setTarget did not prime heat: got %v, want 0.8", b.heat)
	}
	if b.heatVel != 0 {
		t.Errorf("first setTarget left non-zero velocity: %v", b.heatVel)
	}
	b.setTarget(0.2)
	if math.Abs(b.heat-0.8) > 1e-9 {
		t.Errorf("second setTarget should not snap heat; got %v, want 0.8 (ease via AnimTick)", b.heat)
	}
	if b.heatTarget != 0.2 {
		t.Errorf("second setTarget should update heatTarget: got %v, want 0.2", b.heatTarget)
	}
}

func TestHeatBloomBgAtPastRightBoundary(t *testing.T) {
	b := newTestBloom(t)
	b.SetSize(40, 2)
	fallback := color.RGBA{R: 1, G: 2, B: 3, A: 255}
	boundary := int(config.BloomRightBoundary * float64(40))
	got := b.BgAt(boundary+2, 0, fallback)
	if got != fallback {
		t.Errorf("BgAt past boundary = %v, want fallback %v", got, fallback)
	}
}

func TestHeatBloomNoRightBleed(t *testing.T) {
	b := newTestBloom(t)
	widths := []int{40, 80, 120, 200}
	for _, w := range widths {
		b.SetSize(w, 2)
		for _, heat := range []float64{0, 0.5, 1.0} {
			b.heat = heat
			for _, phase := range []float64{0, math.Pi / 2, math.Pi, 3 * math.Pi / 2} {
				b.breathePhase = phase
				boundary := int(config.BloomRightBoundary * float64(w))
				for col := boundary; col < w; col++ {
					for row := 0; row < 2; row++ {
						fallback := color.RGBA{R: 99, G: 99, B: 99, A: 255}
						got := b.BgAt(col, row, fallback)
						if got != fallback {
							t.Errorf("bleed at w=%d heat=%v phase=%v col=%d row=%d: got %v", w, heat, phase, col, row, got)
						}
					}
				}
			}
		}
	}
}

func TestHeatBloomBreathFreezesWhenCalm(t *testing.T) {
	b := NewHeatBloom(testTheme, testAnim)
	b.setTarget(0.5) // some heat so the breath oscillates
	b.AnimTick(false)
	p1 := b.breathePhase
	if p1 == 0 {
		t.Fatal("breath should advance when not calm")
	}
	b.AnimTick(true)
	if b.breathePhase != p1 {
		t.Errorf("breath advanced while calm: %v -> %v", p1, b.breathePhase)
	}
}
