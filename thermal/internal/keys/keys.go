// Package keys is the central registry for coolant's keybindings — shared
// between key dispatch and help rendering so labels and behavior cannot drift.
package keys

import "charm.land/bubbles/v2/key"

type KeyMap struct {
	Help           key.Binding
	Quit           key.Binding
	Collapse       key.Binding
	ClearCompleted key.Binding
	CategoryPrev   key.Binding
	CategoryNext   key.Binding
	CategoryClear  key.Binding
	MouseToggle    key.Binding
	Intel          key.Binding
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
		ClearCompleted: key.NewBinding(
			key.WithKeys("x"),
			key.WithHelp("x", "clear completed"),
		),
		CategoryPrev: key.NewBinding(
			key.WithKeys("["),
			key.WithHelp("[", "prev category"),
		),
		CategoryNext: key.NewBinding(
			key.WithKeys("]"),
			key.WithHelp("]", "next category"),
		),
		CategoryClear: key.NewBinding(
			key.WithKeys("\\"),
			key.WithHelp("\\", "clear filter"),
		),
		MouseToggle: key.NewBinding(
			key.WithKeys("m"),
			key.WithHelp("m", "toggle mouse"),
		),
		Intel: key.NewBinding(
			key.WithKeys("i"),
			key.WithHelp("i", "intel · scoreboard"),
		),
	}
}

// ShortHelp satisfies bubbles/help.KeyMap. Order is load-bearing — tests
// assert left-to-right rendering.
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Help, k.Quit, k.Collapse, k.ClearCompleted, k.CategoryPrev, k.CategoryNext, k.CategoryClear, k.MouseToggle, k.Intel}
}

func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Help, k.Quit},
		{k.Collapse, k.ClearCompleted, k.Intel},
		{k.CategoryPrev, k.CategoryNext, k.CategoryClear, k.MouseToggle},
	}
}
