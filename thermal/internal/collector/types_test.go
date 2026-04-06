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

func TestCategoriesContainExpectedNames(t *testing.T) {
	expected := map[string]bool{
		"build": true, "shell": true,
		"node": true, "go": true, "python": true, "rust": true,
	}
	found := make(map[string]bool)
	for _, cat := range Categories {
		found[cat.Name] = true
	}
	for name := range expected {
		if !found[name] {
			t.Errorf("missing category %q in Categories", name)
		}
	}
	for name := range found {
		if !expected[name] {
			t.Errorf("unexpected category %q in Categories", name)
		}
	}
}

func TestFixedCategoriesExist(t *testing.T) {
	if !FixedCategories["build"] {
		t.Error("build should be a fixed category")
	}
	if !FixedCategories["shell"] {
		t.Error("shell should be a fixed category")
	}
	if FixedCategories["node"] {
		t.Error("node should not be a fixed category")
	}
}

func TestTypeToCategory_GoAndRust(t *testing.T) {
	if TypeToCategory["GO"] != "go" {
		t.Errorf("TypeToCategory[GO] = %q, want go", TypeToCategory["GO"])
	}
	if TypeToCategory["RS"] != "rust" {
		t.Errorf("TypeToCategory[RS] = %q, want rust", TypeToCategory["RS"])
	}
}

func TestTypeToCategory_NodeAbsorbsTestAndSearch(t *testing.T) {
	// V (vitest) and N (node) both map to "node" — no separate "test" category
	if TypeToCategory["V"] != "node" {
		t.Errorf("TypeToCategory[V] = %q, want node", TypeToCategory["V"])
	}
	if TypeToCategory["N"] != "node" {
		t.Errorf("TypeToCategory[N] = %q, want node", TypeToCategory["N"])
	}
	// G (grep) and R (ripgrep) map to shell — no separate "search" category
	if TypeToCategory["G"] != "shell" {
		t.Errorf("TypeToCategory[G] = %q, want shell", TypeToCategory["G"])
	}
	if TypeToCategory["R"] != "shell" {
		t.Errorf("TypeToCategory[R] = %q, want shell", TypeToCategory["R"])
	}
}
