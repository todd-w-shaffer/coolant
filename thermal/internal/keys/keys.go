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
	// Sparkline visibility toggles — one per slot (positional 1..4 keys map
	// to CPU/MEM/Decomp/Token in slot order). Surfaced via SparklineToggles
	// so the help overlay can render them inline next to each slot's
	// descriptive label rather than as a separate key group.
	ToggleCPU    key.Binding
	ToggleMEM    key.Binding
	ToggleDecomp key.Binding
	ToggleToken  key.Binding
	TogglePretty key.Binding
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
			key.WithHelp("i", "session intel"),
		),
		ToggleCPU: key.NewBinding(
			key.WithKeys("1"),
			key.WithHelp("1", "toggle CPU"),
		),
		ToggleMEM: key.NewBinding(
			key.WithKeys("2"),
			key.WithHelp("2", "toggle MEM"),
		),
		ToggleDecomp: key.NewBinding(
			key.WithKeys("3"),
			key.WithHelp("3", "toggle SWAP"),
		),
		ToggleToken: key.NewBinding(
			key.WithKeys("4"),
			key.WithHelp("4", "toggle TOK"),
		),
		TogglePretty: key.NewBinding(
			key.WithKeys("5"),
			key.WithHelp("5", "toggle PRTY"),
		),
	}
}

// SparklineToggles returns the four per-slot toggle bindings in slot order
// (CPU, MEM, Decomp, Token). Separate from FullHelp because they render
// inline with each sparkline's descriptive label in the help overlay, not
// as a generic key group.
func (k KeyMap) SparklineToggles() []key.Binding {
	return []key.Binding{k.ToggleCPU, k.ToggleMEM, k.ToggleDecomp, k.ToggleToken, k.TogglePretty}
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
