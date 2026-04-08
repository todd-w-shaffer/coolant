package main

import (
	"fmt"
	"strings"

	"github.com/toddwshaffer/coolant/thermal/internal/widgets"
)

func renderWord(word string) {
	w := widgets.RenderBrailleWord(word)
	var top, bot strings.Builder
	for i, r := range w.Top {
		if r == ' ' {
			top.WriteRune(' ')
			bot.WriteRune(' ')
			continue
		}
		top.WriteRune(r)
		bot.WriteRune(w.Bot[i])
	}
	fmt.Println(top.String())
	fmt.Println(bot.String())
	fmt.Println()
}

func renderColoredWord(word string, ansi string) {
	w := widgets.RenderBrailleWord(word)
	var top, bot strings.Builder
	for i, r := range w.Top {
		if r == ' ' {
			top.WriteRune(' ')
			bot.WriteRune(' ')
		} else {
			top.WriteString(ansi)
			top.WriteRune(r)
			top.WriteString("\033[0m")
			bot.WriteString(ansi)
			bot.WriteRune(w.Bot[i])
			bot.WriteString("\033[0m")
		}
	}
	fmt.Println(top.String())
	fmt.Println(bot.String())
	fmt.Println()
}

func main() {
	fmt.Println("CPU:")
	renderWord("CPU")

	fmt.Println("MEM:")
	renderWord("MEM")

	fmt.Println("SWAP:")
	renderWord("SWAP")

	fmt.Println("With gauge colors:")
	fmt.Printf("\033[38;2;34;197;94mCPU:\033[0m\n")
	renderColoredWord("CPU", "\033[38;2;34;197;94m")
	fmt.Printf("\033[38;2;34;197;94mMEM:\033[0m\n")
	renderColoredWord("MEM", "\033[38;2;34;197;94m")
}
