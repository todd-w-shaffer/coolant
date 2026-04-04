package main

import (
	"fmt"

	"github.com/toddwshaffer/coolant/thermal/internal/widgets"
)

func main() {
	width := 40
	thresh := &widgets.SparkThresholds{Warn: 70, Crit: 90}

	// Simulate data ticks arriving. Show just the rightmost 20 braille chars
	// (stripped of ANSI) so we can see the shape progression.
	var data []float64
	samples := []float64{0, 0, 5, 12, 25, 40, 55, 70, 80, 85, 60, 30, 15, 8, 50, 90, 100}

	for tick, val := range samples {
		data = append(data, val)

		pair := widgets.RenderSparkline(data, width, 0, thresh)
		clean := stripAnsi(pair.Bottom)

		// Show only last 20 chars (rightmost braille)
		runes := []rune(clean)
		show := runes
		if len(show) > 20 {
			show = show[len(show)-20:]
		}

		label := fmt.Sprintf("t=%02d [%3.0f%%]", tick, val)
		fmt.Printf("%s  ...%s\n", label, string(show))
	}
}

func stripAnsi(s string) string {
	var out []byte
	i := 0
	for i < len(s) {
		if s[i] == '\033' {
			// Skip until 'm'
			for i < len(s) && s[i] != 'm' {
				i++
			}
			i++ // skip 'm'
		} else {
			out = append(out, s[i])
			i++
		}
	}
	return string(out)
}
