// Package keys is the central registry for coolant's keybindings — shared
// between key dispatch and help rendering so labels and behavior cannot drift.
package keys

import "charm.land/bubbles/v2/key"

type KeyMap struct {
	Help       key.Binding
	Quit       key.Binding
	Collapse   key.Binding
	PurgeStale key.Binding
}

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

// ShortHelp satisfies bubbles/help.KeyMap. Order is load-bearing — tests
// assert left-to-right rendering.
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Help, k.Quit, k.Collapse, k.PurgeStale}
}

func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Help, k.Quit},
		{k.Collapse, k.PurgeStale},
	}
}
