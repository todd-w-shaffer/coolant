package main

import (
	"fmt"
	"math"
	"time"
)

func main() {
	// Single braille dot (bottom-left: dot 7)
	dot := rune(0x2800 | 0x40)

	// Deep red target color
	const r, g, b = 239, 68, 68

	fmt.Print("\033[?25l") // hide cursor
	defer fmt.Print("\033[?25h\n")

	step := 0
	for {
		// Sine wave: 0→1→0 over ~2 seconds
		t := float64(step) / 60.0 * math.Pi
		alpha := (math.Sin(t) + 1.0) / 2.0 // 0.0 – 1.0

		// Blend toward black
		cr := int(float64(r) * alpha)
		cg := int(float64(g) * alpha)
		cb := int(float64(b) * alpha)

		fmt.Printf("\r  \033[38;2;%d;%d;%dm%c\033[0m  alpha=%.2f  ", cr, cg, cb, dot, alpha)

		step++
		time.Sleep(33 * time.Millisecond) // ~30fps

		// Quit after ~6 seconds (3 full cycles)
		if step > 180 {
			break
		}
	}
}
