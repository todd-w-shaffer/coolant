package collector

import "testing"

func TestCommToTypeCoveredByTypeToCategory(t *testing.T) {
	seen := make(map[string]bool)
	for _, code := range commToType {
		seen[code] = true
	}
	for code := range seen {
		if _, ok := TypeToCategory[code]; !ok {
			t.Errorf("commToType produces type code %q with no entry in TypeToCategory", code)
		}
	}
}
