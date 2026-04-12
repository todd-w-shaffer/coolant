package widgets

import "strings"

// segmentfont.go — 6×8 pixel digit bitmaps rendered as 3-wide × 2-tall
// braille character cells. Extends braillefont.go's 4×8 / 2×1 letter
// pipeline to wider glyphs for the LCD-style temperature readout.

// digitBitmap is a 6-pixel-wide × 8-pixel-tall glyph. Each row is exactly 6
// characters: '#' on, '.' off.
type digitBitmap [8]string

// segmentDigits holds the canonical bitmaps for '0'..'9' (spec §3.2).
var segmentDigits = [10]digitBitmap{
	/* 0 */ {"######", "#....#", "#....#", "#....#", "#....#", "#....#", "#....#", "######"},
	/* 1 */ {"..##..", ".###..", "..##..", "..##..", "..##..", "..##..", "..##..", "######"},
	/* 2 */ {".####.", "#....#", ".....#", "...##.", "..##..", ".##...", "##....", "######"},
	/* 3 */ {".####.", "#....#", ".....#", "..###.", ".....#", "#....#", "#....#", ".####."},
	/* 4 */ {"#....#", "#....#", "#....#", "######", ".....#", ".....#", ".....#", ".....#"},
	/* 5 */ {"######", "#.....", "#.....", "######", ".....#", ".....#", "#....#", ".####."},
	/* 6 */ {".####.", "#.....", "#.....", "######", "#....#", "#....#", "#....#", ".####."},
	/* 7 */ {"######", "#....#", ".....#", "....#.", "...#..", "..#...", ".#....", "#....."},
	/* 8 */ {".####.", "#....#", "#....#", ".####.", "#....#", "#....#", "#....#", ".####."},
	/* 9 */ {".####.", "#....#", "#....#", "######", ".....#", ".....#", "#....#", ".####."},
}

// segmentDegree is the 4-pixel-wide hollow ring degree glyph (top half
// only). Packed into the digitBitmap shape for type uniformity; only
// cols 0–3 are read. The left and right halves mirror each other so the
// two output braille cells form a symmetric ring outline.
var segmentDegree = digitBitmap{
	".##...",
	"#..#..",
	"#..#..",
	".##...",
	"......",
	"......",
	"......",
	"......",
}

// digitToBraille packs a 6×8 bitmap into 3 top + 3 bottom braille runes.
// Top runes encode rows 0–3, bottom runes encode rows 4–7. Same bit layout
// as letterToBraille, widened from 2 to 3 output columns.
func digitToBraille(bmp digitBitmap) (top [3]rune, bot [3]rune) {
	for col := 0; col < 3; col++ {
		pixCol := col * 2
		for row := 0; row < 4; row++ {
			if bmp[row][pixCol] == '#' {
				top[col] |= fontLeftBits[row]
			}
			if bmp[row][pixCol+1] == '#' {
				top[col] |= fontRightBits[row]
			}
			if bmp[row+4][pixCol] == '#' {
				bot[col] |= fontLeftBits[row]
			}
			if bmp[row+4][pixCol+1] == '#' {
				bot[col] |= fontRightBits[row]
			}
		}
		top[col] |= 0x2800
		bot[col] |= 0x2800
	}
	return
}

// degreeToBraille packs the degree glyph's 4 pixel columns into 2 top
// braille cells. Bottom cells are always blank (U+2800) so the glyph
// floats in the top half of its 2-row cell.
func degreeToBraille() (top, bot [2]rune) {
	for col := 0; col < 2; col++ {
		pixCol := col * 2
		for row := 0; row < 4; row++ {
			if segmentDegree[row][pixCol] == '#' {
				top[col] |= fontLeftBits[row]
			}
			if segmentDegree[row][pixCol+1] == '#' {
				top[col] |= fontRightBits[row]
			}
		}
		top[col] |= 0x2800
		bot[col] = 0x2800
	}
	return
}

// RenderTemperature formats value as a zero-padded 3-digit number followed
// by a degree glyph. Returns the two raw braille rows (no ANSI) and the
// constant visible width of 12 cells. Values outside 0–99 clamp to range.
func RenderTemperature(value int) (top, bot string, visWidth int) {
	if value < 0 {
		value = 0
	}
	if value > 99 {
		value = 99
	}
	digits := [3]int{value / 100, (value / 10) % 10, value % 10}

	var topB, botB strings.Builder
	topB.Grow(12 * 4)
	botB.Grow(12 * 4)

	for i, d := range digits {
		t, b := digitToBraille(segmentDigits[d])
		topB.WriteRune(t[0])
		topB.WriteRune(t[1])
		topB.WriteRune(t[2])
		botB.WriteRune(b[0])
		botB.WriteRune(b[1])
		botB.WriteRune(b[2])
		if i < 2 {
			topB.WriteByte(' ')
			botB.WriteByte(' ')
		}
	}
	dt, db := degreeToBraille()
	topB.WriteRune(dt[0])
	topB.WriteRune(dt[1])
	botB.WriteRune(db[0])
	botB.WriteRune(db[1])
	return topB.String(), botB.String(), 13
}
