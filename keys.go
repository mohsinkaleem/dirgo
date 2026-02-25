package main

import "github.com/charmbracelet/bubbles/key"

// KeyMap defines all keybindings.
type KeyMap struct {
	Up        key.Binding
	Down      key.Binding
	Left      key.Binding
	Right     key.Binding
	Top       key.Binding
	Bottom    key.Binding
	PageUp    key.Binding
	PageDown  key.Binding
	QuickLook key.Binding
	Refresh   key.Binding
	TopView   key.Binding
	Open      key.Binding
	Search    key.Binding
	Hidden    key.Binding
	DirOnly   key.Binding
	Help      key.Binding
	Quit      key.Binding
	Escape    key.Binding
	CountAll  key.Binding
	GoTo      key.Binding
	Delete    key.Binding
}

// DefaultKeyMap returns the default key bindings.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "move up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "move down"),
		),
		Left: key.NewBinding(
			key.WithKeys("left", "backspace"),
			key.WithHelp("←", "parent dir"),
		),
		Right: key.NewBinding(
			key.WithKeys("right", "l", "enter"),
			key.WithHelp("→/l", "open dir"),
		),
		Top: key.NewBinding(
			key.WithKeys("g"),
			key.WithHelp("g", "go to top"),
		),
		Bottom: key.NewBinding(
			key.WithKeys("G"),
			key.WithHelp("G", "go to bottom"),
		),
		PageUp: key.NewBinding(
			key.WithKeys("pgup", "ctrl+u"),
			key.WithHelp("PgUp/^U", "page up"),
		),
		PageDown: key.NewBinding(
			key.WithKeys("pgdown", "ctrl+d"),
			key.WithHelp("PgDn/^D", "page down"),
		),
		QuickLook: key.NewBinding(
			key.WithKeys(" "),
			key.WithHelp("space", "quick look"),
		),
		Refresh: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "refresh"),
		),
		TopView: key.NewBinding(
			key.WithKeys("t"),
			key.WithHelp("t", "top 10 view"),
		),
		Open: key.NewBinding(
			key.WithKeys("o"),
			key.WithHelp("o", "open in finder"),
		),
		Search: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "search/filter"),
		),
		Hidden: key.NewBinding(
			key.WithKeys("h"),
			key.WithHelp("h", "toggle hidden"),
		),
		DirOnly: key.NewBinding(
			key.WithKeys("f"),
			key.WithHelp("f", "cycle filter"),
		),
		Delete: key.NewBinding(
			key.WithKeys("d"),
			key.WithHelp("d", "move to trash"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "help"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
		Escape: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "cancel"),
		),
		CountAll: key.NewBinding(
			key.WithKeys("s"),
			key.WithHelp("s", "count lines (all)"),
		),
		GoTo: key.NewBinding(
			key.WithKeys("c"),
			key.WithHelp("c", "cd to path"),
		),
	}
}
