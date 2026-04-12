package widgets

import (
	"testing"
	"unicode/utf8"
)

// TestDigitToBraille_Zero — the '0' bitmap from spec §3.2 encodes to the
// hand-computed braille runes. If this fails the bit-packing walk is wrong.
func TestDigitToBraille_Zero(t *testing.T) {
	top, bot := digitToBraille(segmentDigits[0])

	wantTop := [3]rune{0x284F, 0x2809, 0x28B9}
	wantBot := [3]rune{0x28C7, 0x28C0, 0x28F8}

	for i, r := range top {
		if r != wantTop[i] {
			t.Errorf("top[%d] = %#x, want %#x", i, r, wantTop[i])
		}
	}
	for i, r := range bot {
		if r != wantBot[i] {
			t.Errorf("bot[%d] = %#x, want %#x", i, r, wantBot[i])
		}
	}
}

// TestDigitToBraille_AllDigits — every digit produces six braille runes with
// at least one dot set somewhere (catches missing glyphs — a fully-blank
// digit means the bitmap wasn't filled in).
func TestDigitToBraille_AllDigits(t *testing.T) {
	for d := 0; d < 10; d++ {
		top, bot := digitToBraille(segmentDigits[d])
		for i, r := range top {
			if r < 0x2800 || r > 0x28FF {
				t.Errorf("digit %d top[%d] = %#x out of braille range", d, i, r)
			}
		}
		for i, r := range bot {
			if r < 0x2800 || r > 0x28FF {
				t.Errorf("digit %d bot[%d] = %#x out of braille range", d, i, r)
			}
		}
		// A real glyph lights at least one dot across the six runes.
		lit := rune(0)
		for _, r := range top {
			lit |= r - 0x2800
		}
		for _, r := range bot {
			lit |= r - 0x2800
		}
		if lit == 0 {
			t.Errorf("digit %d is blank — bitmap not filled in", d)
		}
	}
}

// TestDegreeToBraille — top rune carries dots, bot rune is blank (U+2800).
func TestDegreeToBraille(t *testing.T) {
	top, bot := degreeToBraille()
	if top == 0x2800 {
		t.Errorf("degree top rune is blank, want dots set")
	}
	if top < 0x2800 || top > 0x28FF {
		t.Errorf("degree top = %#x out of braille range", top)
	}
	if bot != 0x2800 {
		t.Errorf("degree bot = %#x, want 0x2800 (blank)", bot)
	}
}

// TestRenderTemperature_Formatting — clamp, zero-pad, and constant 12-cell width.
func TestRenderTemperature_Formatting(t *testing.T) {
	cases := []struct {
		value   int
		wantLen int
	}{
		{0, 12},
		{7, 12},
		{42, 12},
		{99, 12},
		{-5, 12},
		{150, 12},
	}
	for _, tc := range cases {
		top, bot, w := RenderTemperature(tc.value)
		if w != 12 {
			t.Errorf("value=%d visWidth=%d, want 12", tc.value, w)
		}
		if n := utf8.RuneCountInString(top); n != tc.wantLen {
			t.Errorf("value=%d top rune count=%d, want %d", tc.value, n, tc.wantLen)
		}
		if n := utf8.RuneCountInString(bot); n != tc.wantLen {
			t.Errorf("value=%d bot rune count=%d, want %d", tc.value, n, tc.wantLen)
		}
	}
}

// TestRenderTemperature_DigitSelection — the digits rendered in the top row
// reflect the zero-padded value. Compares the three digit-head runes
// (positions 0, 4, 8) against each digit's canonical head rune.
func TestRenderTemperature_DigitSelection(t *testing.T) {
	cases := []struct {
		value int
		want  [3]int // digit values
	}{
		{0, [3]int{0, 0, 0}},
		{7, [3]int{0, 0, 7}},
		{42, [3]int{0, 4, 2}},
		{99, [3]int{0, 9, 9}},
		{-5, [3]int{0, 0, 0}},
		{150, [3]int{0, 9, 9}},
	}
	for _, tc := range cases {
		top, _, _ := RenderTemperature(tc.value)
		runes := []rune(top)
		for slot, digit := range tc.want {
			// Each digit occupies 3 runes starting at slot*4 (3 digit chars + 1 gap).
			head := runes[slot*4]
			wantTop, _ := digitToBraille(segmentDigits[digit])
			if head != wantTop[0] {
				t.Errorf("value=%d slot=%d: head=%#x, want digit %d head=%#x",
					tc.value, slot, head, digit, wantTop[0])
			}
		}
	}
}
