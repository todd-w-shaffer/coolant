// Command swatch renders a theme palette preview.
// Used for iterating on palettes without booting the full dashboard.
//
// Usage:
//
//	go run ./cmd/swatch                  # classic (default)
//	go run ./cmd/swatch --theme iron     # specific theme
//	go run ./cmd/swatch --all            # all themes stacked
//	go run ./cmd/swatch --animate        # live 3s animation preview (tidal, KITT, highscore)
package main

import (
	"flag"
	"fmt"
	"image/color"
	"os"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/toddwshaffer/coolant/thermal/internal/anim"
	"github.com/toddwshaffer/coolant/thermal/internal/theme"
	"github.com/toddwshaffer/coolant/thermal/internal/ui"
	"github.com/toddwshaffer/coolant/thermal/internal/widgets"
)

const swatchSparkWidth = 40 // braille chars (80 samples at 2 per char)

func main() {
	themeName := flag.String("theme", "classic", "theme name")
	animName := flag.String("animation", "default", "animation profile name")
	all := flag.Bool("all", false, "render all themes stacked")
	animate := flag.Bool("animate", false, "live animation preview (~3s bubbletea loop)")
	flag.Parse()

	if *all {
		for i, name := range theme.Names() {
			if i > 0 {
				fmt.Println()
			}
			th, err := theme.Get(name)
			if err != nil {
				fmt.Fprintf(os.Stderr, "theme %q: %v\n", name, err)
				os.Exit(1)
			}
			fmt.Print(renderTheme(th))
		}
		fmt.Println()
		for i, name := range anim.Names() {
			if i > 0 {
				fmt.Println()
			}
			ap, _ := anim.Get(name)
			fmt.Print(renderAnimProfile(ap))
		}
		return
	}

	th, err := theme.Get(*themeName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	ap, err := anim.Get(*animName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	if *animate {
		if err := runAnimate(th, ap); err != nil {
			fmt.Fprintf(os.Stderr, "animate: %v\n", err)
			os.Exit(1)
		}
		return
	}

	fmt.Print(renderTheme(th))
	fmt.Println()
	fmt.Print(renderAnimProfile(ap))
}

// swatchHeader renders a padded ═══ header line for a swatch section.
func swatchHeader(label string) string {
	header := fmt.Sprintf("═══ %s ", label)
	headerRunes := []rune(header)
	if len(headerRunes) < 60 {
		header += strings.Repeat("═", 60-len(headerRunes))
	}
	return header + "\n"
}

// renderTheme renders the full swatch for a single theme.
func renderTheme(th *theme.Theme) string {
	var sb strings.Builder
	sb.WriteString(swatchHeader(th.Name))

	sections := []struct {
		label string
		fn    func(*theme.Theme) string
	}{
		{"Severity Gradient", renderSeverityGradient},
		{"Overall Gradient", renderOverallGradient},
		{"Category Gradient", renderCategoryGradient},
		{"Threat Colors", renderThreatColors},
		{"Session Diamonds", renderSessionDiamonds},
		{"Gauge Dots", renderGaugeDots},
		{"Agent Icons", renderAgentIcons},
		{"Offline Sparkline", renderOfflineSparkline},
		{"Rates", renderRates},
		{"Chrome", renderChrome},
	}

	for _, sec := range sections {
		sb.WriteString(dimLabel(sec.label))
		sb.WriteString("\n")
		sb.WriteString(sec.fn(th))
		sb.WriteString("\n")
	}

	return sb.String()
}

// dimLabel renders a section label in dim gray.
func dimLabel(label string) string {
	return fmt.Sprintf("\033[38;5;245m── %s\033[0m", label)
}

// renderSeverityGradient renders a braille sparkline ramp from 0% to 100%,
// colored by the theme's severity gradient. Uses the real sparkline renderer.
func renderSeverityGradient(th *theme.Theme) string {
	// Generate a smooth ramp: 0→100 over swatchSparkWidth+1 samples
	n := swatchSparkWidth + 1
	data := make([]float64, n)
	for i := range data {
		data[i] = float64(i) / float64(n-1) * 100.0
	}

	thresh := &theme.SparkThresholds{Warn: 40, Crit: 75}
	buf := widgets.NewSparkBufs(swatchSparkWidth)
	pair := widgets.RenderSparkline(data, nil, swatchSparkWidth, 100, thresh, 0, buf, th, false)

	var sb strings.Builder
	sb.WriteString(pair.Top)
	sb.WriteString("\n")
	sb.WriteString(pair.Bottom)
	return sb.String()
}

// renderOverallGradient renders the 5 overall thermal levels as labeled boxes.
func renderOverallGradient(th *theme.Theme) string {
	labels := []string{"COOL", "WARM", "HOT", "CRITICAL", "OFFLINE"}
	var sb strings.Builder
	for i, level := range th.OverallGradient {
		style := lipgloss.NewStyle().
			Foreground(level.Fg).
			Background(level.Bg).
			Padding(0, 1)
		sb.WriteString(style.Render(labels[i]))
		if i < len(th.OverallGradient)-1 {
			sb.WriteRune(' ')
		}
	}
	return sb.String()
}

// renderCategoryGradient renders the 5 category thermal levels as labeled boxes.
func renderCategoryGradient(th *theme.Theme) string {
	labels := []string{"cold", "cool", "warm", "hot", "crit"}
	var sb strings.Builder
	for i, level := range th.CategoryGradient {
		// Show as "build:NNN" style label matching the dashboard
		label := fmt.Sprintf("%s:%03d", labels[i], i*3)
		style := lipgloss.NewStyle().
			Foreground(level.Fg).
			Background(level.Bg).
			Padding(0, 1)
		sb.WriteString(style.Render(label))
		if i < len(th.CategoryGradient)-1 {
			sb.WriteRune(' ')
		}
	}
	return sb.String()
}

// renderThreatColors renders the 4 threat level indicators.
func renderThreatColors(th *theme.Theme) string {
	labels := []string{"COOL", "WARM", "HOT", "MELTDOWN"}
	var sb strings.Builder
	for i, c := range th.ThreatColors {
		sb.WriteString(colorText(c, "● "+labels[i]))
		if i < len(th.ThreatColors)-1 {
			sb.WriteString("  ")
		}
	}
	return sb.String()
}

// renderSessionDiamonds renders the 5 session phase escalation states.
func renderSessionDiamonds(th *theme.Theme) string {
	phases := []struct {
		name  string
		color color.Color
	}{
		{"idle", th.SessionPhase.Idle},
		{"active", th.SessionPhase.Active},
		{"language", th.SessionPhase.Language},
		{"build", th.SessionPhase.Build},
		{"explosion", th.SessionPhase.Explosion},
	}
	var sb strings.Builder
	for i, p := range phases {
		sb.WriteString(colorText(p.color, ui.SessionDiamondGlyph+" "+p.name))
		if i < len(phases)-1 {
			sb.WriteString("  ")
		}
	}
	return sb.String()
}

// renderGaugeDots renders the 4 gauge dot indicators with their pre-formatted output.
func renderGaugeDots(th *theme.Theme) string {
	labels := []string{"CPU", "MEM", "COMP", "GPU"}
	var sb strings.Builder
	for i, d := range th.GaugeDots {
		sb.WriteString(d.Formatted)
		sb.WriteString(labels[i])
		if i < len(th.GaugeDots)-1 {
			sb.WriteString("  ")
		}
	}
	return sb.String()
}

// renderAgentIcons renders agent hexagon glyphs at 5 brightness levels.
// Uses OverallGradient[0].Bg as the interpolation backdrop — the idle
// cold-cell bg the dashboard actually renders agent dots against — so
// previews reflect how the theme will look in thermo.
func renderAgentIcons(th *theme.Theme) string {
	glyphs := []string{"⬡", "⏣", "⬢", "⏣", "⬡"}
	levels := []float64{0.2, 0.4, 0.6, 0.8, 1.0}
	bg := th.OverallGradient[0].Bg
	var sb strings.Builder
	for i, brightness := range levels {
		r, g, b, _ := widgets.BreatheFg(th.AccentR, th.AccentG, th.AccentB, brightness, bg).RGBA()
		sb.WriteString(fmt.Sprintf("\033[38;2;%d;%d;%dm%s\033[0m", r>>8, g>>8, b>>8, glyphs[i]))
		if i < len(levels)-1 {
			sb.WriteRune(' ')
		}
	}
	return sb.String()
}

// renderOfflineSparkline renders the offline rainbow dots pattern.
func renderOfflineSparkline(th *theme.Theme) string {
	var sb strings.Builder
	for i := 0; i < swatchSparkWidth; i++ {
		colorIdx := i % len(th.OfflineSparkANSI)
		// Cycle through fun braille-like patterns
		patterns := []rune{'⡀', '⠁', '⠂', '⠄', '⡁', '⠆', '⠅'}
		patIdx := (i*7 + i*i*3) % len(patterns)
		sb.WriteString(th.OfflineSparkANSI[colorIdx])
		sb.WriteRune(patterns[patIdx])
		sb.WriteString("\033[0m")
	}
	return sb.String()
}

// renderRates renders the rate display colors.
func renderRates(th *theme.Theme) string {
	var sb strings.Builder
	sb.WriteString(colorText(th.SpawnColor, "warm:+004/s"))
	sb.WriteString("  ")
	sb.WriteString(colorText(th.DeathColor, "cool:-003/s"))
	sb.WriteString("  ")
	sb.WriteString(colorText(th.NetColor, "net:+001/s"))
	return sb.String()
}

// renderChrome renders the chrome/UI frame colors.
func renderChrome(th *theme.Theme) string {
	var sb strings.Builder
	sb.WriteString(colorText(th.DimColor, "dim text"))
	sb.WriteString("  ")
	sb.WriteString(colorText(th.HelpColor, "help text"))
	sb.WriteString("  ")
	sb.WriteString(colorText(th.IdleColor, "idle text"))
	return sb.String()
}

// colorText renders text with lipgloss foreground color.
func colorText(c color.Color, text string) string {
	return lipgloss.NewStyle().Foreground(c).Render(text)
}

// renderAnimProfile renders a summary of an animation profile's key constants.
func renderAnimProfile(ap *anim.Profile) string {
	var sb strings.Builder
	sb.WriteString(swatchHeader("animation: " + ap.Name))

	sections := []struct {
		label string
		lines []string
	}{
		{"Tidal Wave", []string{
			fmt.Sprintf("  phase step: %.4f  wave mix: %.2f  breath mix: %.2f", ap.TidalPhaseStep, ap.TidalWaveMix, ap.TidalBreathMix),
			fmt.Sprintf("  bright floor: %.2f  phase spread: %.1f rad", ap.TidalBrightFloor, ap.TidalPhaseSpread),
			fmt.Sprintf("  glyph thresholds: filled > %.2f  mid > %.2f", ap.GlyphFilledThresh, ap.GlyphMidThresh),
		}},
		{"KITT Scanner", []string{
			fmt.Sprintf("  sweep rate: %.4f  ambient: %.2f  peak: %.2f", ap.KITTSweepRate, ap.KITTAmbient, ap.KITTPeak),
			fmt.Sprintf("  sigma²: %.2f  single bright: %.2f", ap.KITTSigmaSq, ap.KITTSingleBright),
		}},
		{"Breathing", []string{
			fmt.Sprintf("  phase step: %.4f  stale rate: %.2f  stale dim: %.2f", ap.BreathePhaseStep, ap.BreatheStaleRate, ap.BreatheStaleDim),
		}},
		{"Spring Physics", []string{
			fmt.Sprintf("  freq: %.1f  damping: %.1f  peak decay: %.4f", ap.SpringFreq, ap.SpringDamping, ap.PeakDecayRate),
		}},
	}

	for _, sec := range sections {
		sb.WriteString(dimLabel(sec.label))
		sb.WriteString("\n")
		for _, line := range sec.lines {
			sb.WriteString(line)
			sb.WriteString("\n")
		}
	}

	return sb.String()
}
