package config

import "testing"

func TestTidalPhaseSpreadExists(t *testing.T) {
	// TidalPhaseSpread was previously hardcoded inline (1.5 rad/dot).
	// Verify it exists as a named constant and is positive.
	if TidalPhaseSpread <= 0 {
		t.Errorf("TidalPhaseSpread = %v, want > 0", TidalPhaseSpread)
	}
}
