package widgets

import (
	"os"
	"testing"

	"charm.land/lipgloss/v2"
	zone "github.com/lrstanley/bubblezone/v2"
)

// TestMain initializes the global zone manager so that zone.Mark calls
// in production code (renderCatCell) don't panic during tests.
func TestMain(m *testing.M) {
	zone.NewGlobal()
	os.Exit(m.Run())
}

func TestBubblezoneRoundtrip(t *testing.T) {
	input := "hello"
	marked := zone.Mark("smoke:1", input)

	if marked == input {
		t.Fatal("zone.Mark should add marker bytes; got identical string")
	}

	scanned := zone.Scan(marked)
	inputWidth := lipgloss.Width(input)
	scannedWidth := lipgloss.Width(scanned)

	if scannedWidth != inputWidth {
		t.Errorf("zone.Scan width = %d, want %d (equal to input)", scannedWidth, inputWidth)
	}
}
