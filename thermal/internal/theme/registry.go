package theme

import (
	"fmt"
	"sort"
)

// Registry maps theme names to constructor functions.
var Registry = map[string]func() *Theme{
	"classic": Classic,
	"iron":    Iron,
	"mono":    Mono,
	"frappe":  Frappe,
}

// Get returns a fully initialized Theme by name, or an error if not found.
func Get(name string) (*Theme, error) {
	fn, ok := Registry[name]
	if !ok {
		return nil, fmt.Errorf("theme %q not found (available: %v)", name, Names())
	}
	return fn(), nil
}

// Names returns the sorted list of registered theme names.
func Names() []string {
	names := make([]string, 0, len(Registry))
	for k := range Registry {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}
