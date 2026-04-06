package main

import "fmt"

// Each letter is a bitmap: rows of pixel columns.
// 4 pixels wide × 8 pixels tall = 2 braille chars wide.
// '#' = on, '.' = off

var font = map[rune][8]string{
	'C': {
		".##.",
		"#..#",
		"#...",
		"#...",
		"#...",
		"#...",
		"#..#",
		".##.",
	},
	'P': {
		"###.",
		"#..#",
		"#..#",
		"###.",
		"#...",
		"#...",
		"#...",
		"#...",
	},
	'U': {
		"#..#",
		"#..#",
		"#..#",
		"#..#",
		"#..#",
		"#..#",
		"#..#",
		".##.",
	},
	'M': {
		"#..#",
		"####",
		"####",
		"#.##",
		"#..#",
		"#..#",
		"#..#",
		"#..#",
	},
	'E': {
		"###.",
		"#...",
		"#...",
		"##..",
		"#...",
		"#...",
		"#...",
		"###.",
	},
	'O': {
		".##.",
		"#..#",
		"#..#",
		"#..#",
		"#..#",
		"#..#",
		"#..#",
		".##.",
	},
	'S': {
		".##.",
		"#..#",
		"#...",
		".#..",
		"..#.",
		"...#",
		"#..#",
		".##.",
	},
	'W': {
		"#..#",
		"#..#",
		"#..#",
		"#..#",
		"#.##",
		"####",
		"####",
		"#..#",
	},
	'A': {
		".##.",
		"#..#",
		"#..#",
		"####",
		"#..#",
		"#..#",
		"#..#",
		"#..#",
	},
}

// Braille dot positions:
//   Left  Right
//   0x01  0x08   (row 0)
//   0x02  0x10   (row 1)
//   0x04  0x20   (row 2)
//   0x40  0x80   (row 3)

var leftBits = [4]rune{0x01, 0x02, 0x04, 0x40}
var rightBits = [4]rune{0x08, 0x10, 0x20, 0x80}

func letterToBraille(letter [8]string) (top [2]rune, bot [2]rune) {
	// Each letter is 4 pixels wide = 2 braille chars
	// Top braille char: rows 0-3, Bottom: rows 4-7
	for col := 0; col < 2; col++ {
		for row := 0; row < 4; row++ {
			pixCol := col * 2
			// Left pixel of this braille char
			if pixCol < len(letter[row]) && letter[row][pixCol] == '#' {
				top[col] |= leftBits[row]
			}
			// Right pixel
			if pixCol+1 < len(letter[row]) && letter[row][pixCol+1] == '#' {
				top[col] |= rightBits[row]
			}
			// Bottom half (rows 4-7)
			if pixCol < len(letter[row+4]) && letter[row+4][pixCol] == '#' {
				bot[col] |= leftBits[row]
			}
			if pixCol+1 < len(letter[row+4]) && letter[row+4][pixCol+1] == '#' {
				bot[col] |= rightBits[row]
			}
		}
	}
	return
}

func renderWord(word string) {
	var topLine, botLine string
	for i, ch := range word {
		bmp, ok := font[ch]
		if !ok {
			continue
		}
		if i > 0 {
			topLine += " "
			botLine += " "
		}
		top, bot := letterToBraille(bmp)
		for c := 0; c < 2; c++ {
			topLine += string(0x2800 + top[c])
			botLine += string(0x2800 + bot[c])
		}
	}
	fmt.Println(topLine)
	fmt.Println(botLine)
	fmt.Println()
}

func main() {
	fmt.Println("CPU:")
	renderWord("CPU")

	fmt.Println("MEM:")
	renderWord("MEM")

	fmt.Println("COMP:")
	renderWord("COMP")

	fmt.Println("SWAP:")
	renderWord("SWAP")

	// Show them colored like they'd appear in the gauge
	fmt.Println("With gauge colors:")
	fmt.Printf("\033[38;2;34;197;94mCPU:\033[0m\n")
	renderColoredWord("CPU", "\033[38;2;34;197;94m")
	fmt.Printf("\033[38;2;34;197;94mMEM:\033[0m\n")
	renderColoredWord("MEM", "\033[38;2;34;197;94m")
	fmt.Printf("\033[38;2;234;179;8mCOMP:\033[0m\n")
	renderColoredWord("COMP", "\033[38;2;234;179;8m")
}

func renderColoredWord(word string, ansi string) {
	var topLine, botLine string
	for i, ch := range word {
		bmp, ok := font[ch]
		if !ok {
			continue
		}
		if i > 0 {
			topLine += " "
			botLine += " "
		}
		top, bot := letterToBraille(bmp)
		for c := 0; c < 2; c++ {
			topLine += ansi + string(0x2800+top[c]) + "\033[0m"
			botLine += ansi + string(0x2800+bot[c]) + "\033[0m"
		}
	}
	fmt.Println(topLine)
	fmt.Println(botLine)
	fmt.Println()
}
