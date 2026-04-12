// Package keys is the central registry for coolant's keybindings.
//
// All key dispatch in cmd/thermal/main.go and all help rendering in
// internal/widgets/help.go consume the same KeyMap so labels and behavior
// can never drift. v1 ships hard-coded defaults; the package is the seam
// for a future TOML loader.
package keys

import "charm.land/bubbles/v2/key"

// KeyMap holds every binding the dashboard responds to. Order of the fields
// is incidental; render order is fixed by ShortHelp / FullHelp below.
type KeyMap struct {
	Help       key.Binding
	Quit       key.Binding
	Collapse   key.Binding
	PurgeStale key.Binding
}

// Default returns the v1 hard-coded KeyMap.
func Default() KeyMap {
	return KeyMap{
		Help: key.NewBinding(
			key.WithKeys("h", "?"),
			key.WithHelp("h/?", "help"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
		Collapse: key.NewBinding(
			key.WithKeys("c"),
			key.WithHelp("c", "collapse sessions"),
		),
		PurgeStale: key.NewBinding(
			key.WithKeys("x"),
			key.WithHelp("x", "purge stale agents"),
		),
	}
}

// ShortHelp satisfies bubbles/help.KeyMap. Order is load-bearing — it
// determines on-screen left-to-right rendering and is asserted by tests.
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Help, k.Quit, k.Collapse, k.PurgeStale}
}

// FullHelp satisfies bubbles/help.KeyMap. Two columns: navigation/quit on
// the left, view actions on the right.
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Help, k.Quit},
		{k.Collapse, k.PurgeStale},
	}
}
