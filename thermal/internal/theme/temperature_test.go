package theme

import "testing"

// TestOverallTemperatureFg_Distinct — every temperature value 0..99 produces
// a populated ANSI escape. Continuous gradient LUT must not have gaps.
func TestOverallTemperatureFg_Distinct(t *testing.T) {
	th := Classic()
	for v := 0; v < 100; v++ {
		if got := th.OverallTemperatureFg(v); got == "" {
			t.Errorf("OverallTemperatureFg(%d) is empty", v)
		}
	}
}

// TestOverallTemperatureFg_NotSnapped — the LUT interpolates between the
// five OverallGradient anchors. Values between anchor positions must not
// match their nearest anchor exactly, otherwise the gradient is snapping.
func TestOverallTemperatureFg_NotSnapped(t *testing.T) {
	th := Classic()
	// Anchors sit at value*4/99 = 0..4, i.e. values 0, ~25, ~50, ~74, 99.
	// Pick values squarely between anchors and assert they differ from both.
	type pair struct {
		mid, anchorLo, anchorHi int
	}
	cases := []pair{
		{mid: 12, anchorLo: 0, anchorHi: 25},
		{mid: 37, anchorLo: 25, anchorHi: 50},
		{mid: 62, anchorLo: 50, anchorHi: 75},
		{mid: 87, anchorLo: 75, anchorHi: 99},
	}
	for _, c := range cases {
		mid := th.OverallTemperatureFg(c.mid)
		lo := th.OverallTemperatureFg(c.anchorLo)
		hi := th.OverallTemperatureFg(c.anchorHi)
		if mid == lo {
			t.Errorf("value=%d fg equals anchor-lo (value=%d): %q", c.mid, c.anchorLo, mid)
		}
		if mid == hi {
			t.Errorf("value=%d fg equals anchor-hi (value=%d): %q", c.mid, c.anchorHi, mid)
		}
	}
}

// TestOverallTemperatureFg_DistinctPerBand — the four band centers (Cool,
// Warm, Hot, Meltdown sample points) produce four distinct ANSI escapes.
func TestOverallTemperatureFg_DistinctPerBand(t *testing.T) {
	th := Iron()
	seen := map[string]int{}
	// Pick representative values from each band.
	for _, v := range []int{15, 40, 65, 90} {
		ansi := th.OverallTemperatureFg(v)
		if prev, ok := seen[ansi]; ok {
			t.Errorf("values %d and %d collide on fg %q", prev, v, ansi)
		}
		seen[ansi] = v
	}
}

// TestOverallTemperatureFg_Clamp — out-of-range values clamp without panic
// and produce a valid escape.
func TestOverallTemperatureFg_Clamp(t *testing.T) {
	th := Classic()
	if got := th.OverallTemperatureFg(-5); got == "" {
		t.Errorf("negative value produced empty escape")
	}
	if got := th.OverallTemperatureFg(150); got == "" {
		t.Errorf("over-range value produced empty escape")
	}
}
