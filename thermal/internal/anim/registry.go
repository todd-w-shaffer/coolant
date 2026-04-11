package anim

import (
	"fmt"
	"sort"
)

// registry maps profile names to constructor functions.
var registry = map[string]func() *Profile{
	"default": Default,
	"calm":    Calm,
	"intense": Intense,
}

// Get returns a fully initialized Profile by name, or an error if not found.
func Get(name string) (*Profile, error) {
	fn, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("animation profile %q not found (available: %v)", name, Names())
	}
	return fn(), nil
}

// Names returns the sorted list of registered profile names.
func Names() []string {
	names := make([]string, 0, len(registry))
	for k := range registry {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}
