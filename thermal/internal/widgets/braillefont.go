package widgets

import "strings"

// braillefont.go — 4×8 pixel bitmap font rendered as 2-wide braille characters.
// Used for startup labels that scroll off as sparkline data fills in.

// letterBitmap defines a 4-pixel-wide × 8-pixel-tall glyph.
// Each string is one row: '#' = on, '.' = off.
type letterBitmap [8]string

// brailleFont maps uppercase letters to 4×8 pixel bitmaps.
var brailleFont = map[rune]letterBitmap{
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

// Braille dot bit positions per row within a character:
//
//	Left  Right
//	0x01  0x08   (row 0, top)
//	0x02  0x10   (row 1)
//	0x04  0x20   (row 2)
//	0x40  0x80   (row 3, bottom)
//
// fontLeftBits and fontRightBits are braille dot bit positions per row within a character.
var fontLeftBits = [4]rune{0x01, 0x02, 0x04, 0x40}
var fontRightBits = [4]rune{0x08, 0x10, 0x20, 0x80}

// letterToBraille converts a 4×8 bitmap to 2 top + 2 bottom braille runes.
func letterToBraille(bmp letterBitmap) (top [2]rune, bot [2]rune) {
	for col := 0; col < 2; col++ {
		for row := 0; row < 4; row++ {
			pixCol := col * 2
			if pixCol < len(bmp[row]) && bmp[row][pixCol] == '#' {
				top[col] |= fontLeftBits[row]
			}
			if pixCol+1 < len(bmp[row]) && bmp[row][pixCol+1] == '#' {
				top[col] |= fontRightBits[row]
			}
			if pixCol < len(bmp[row+4]) && bmp[row+4][pixCol] == '#' {
				bot[col] |= fontLeftBits[row]
			}
			if pixCol+1 < len(bmp[row+4]) && bmp[row+4][pixCol+1] == '#' {
				bot[col] |= fontRightBits[row]
			}
		}
	}
	return
}

// BrailleWord holds pre-rendered braille runes for a word.
type BrailleWord struct {
	Top []rune
	Bot []rune
}

// RenderBrailleWord converts a word to top/bottom braille rune slices.
// Each letter is 2 braille chars wide with 1-char gap between letters.
func RenderBrailleWord(word string) BrailleWord {
	var top, bot []rune
	for i, ch := range word {
		bmp, ok := brailleFont[ch]
		if !ok {
			continue
		}
		if i > 0 {
			top = append(top, ' ')
			bot = append(bot, ' ')
		}
		t, b := letterToBraille(bmp)
		top = append(top, 0x2800|t[0], 0x2800|t[1])
		bot = append(bot, 0x2800|b[0], 0x2800|b[1])
	}
	return BrailleWord{Top: top, Bot: bot}
}

// OverlayLabel replaces leading empty positions in a SparkPair with colored
// braille text. The label scrolls left as sparkline data fills in from the right.
//
// dataLen: number of samples in render history (drives scroll position)
// sparkWidth: total sparkline character width
// ansiColor: ANSI escape for the label color
func OverlayLabel(pair SparkPair, label BrailleWord, dataLen, sparkWidth int, ansiColor string) SparkPair {
	labelWidth := len(label.Top)
	if labelWidth == 0 {
		return pair
	}

	// How many empty chars on the left of the sparkline
	emptyLeft := max(0, sparkWidth-dataLen)
	// How many label chars have scrolled off the left edge
	overlap := max(0, labelWidth-emptyLeft)
	// How many label chars are still visible
	visible := labelWidth - overlap
	if visible <= 0 {
		return pair
	}

	// Count actual leading spaces in the sparkline output. Interpolation can
	// create non-zero values at the padding boundary, so we can't trust the
	// calculated emptyLeft — count real single-byte spaces instead.
	topSpaces := countLeadingSpaces(pair.Top)
	botSpaces := countLeadingSpaces(pair.Bottom)
	available := min(topSpaces, botSpaces)
	if visible > available {
		visible = available
	}
	if visible <= 0 {
		return pair
	}

	// Build colored label prefix from the visible portion
	topPrefix := coloredBrailleRunes(label.Top[overlap:overlap+visible], ansiColor)
	botPrefix := coloredBrailleRunes(label.Bot[overlap:overlap+visible], ansiColor)

	// Replace leading space bytes with the label.
	topOut := topPrefix + pair.Top[visible:]
	botOut := botPrefix + pair.Bottom[visible:]

	return SparkPair{Top: topOut, Bottom: botOut}
}

// countLeadingSpaces returns the number of leading 0x20 bytes in s.
func countLeadingSpaces(s string) int {
	for i := 0; i < len(s); i++ {
		if s[i] != ' ' {
			return i
		}
	}
	return len(s)
}

// coloredBrailleRunes renders braille runes with ANSI color. Spaces pass through plain.
func coloredBrailleRunes(runes []rune, ansiColor string) string {
	var sb strings.Builder
	sb.Grow(len(runes) * 30)
	for _, r := range runes {
		if r == ' ' {
			sb.WriteRune(' ')
		} else {
			sb.WriteString(ansiColor)
			sb.WriteRune(r)
			sb.WriteString(sparkReset)
		}
	}
	return sb.String()
}
