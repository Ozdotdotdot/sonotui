package tui

import "github.com/charmbracelet/bubbles/key"

type keyMap struct {
	PlayPause key.Binding
	Stop      key.Binding
	Mute      key.Binding
	VolUp     key.Binding
	VolDown   key.Binding
	VolUp1    key.Binding
	VolDown1  key.Binding
	LineIn    key.Binding
	Prev      key.Binding
	Next      key.Binding
	Tab       key.Binding
	Discover  key.Binding
	Quit      key.Binding
}

var keys = keyMap{
	PlayPause: key.NewBinding(
		key.WithKeys(" "),
		key.WithHelp("[spc]", "play/pause"),
	),
	Stop: key.NewBinding(
		key.WithKeys("s"),
		key.WithHelp("[s]", "stop"),
	),
	Mute: key.NewBinding(
		key.WithKeys("m"),
		key.WithHelp("[m]", "mute/unmute"),
	),
	VolUp: key.NewBinding(
		key.WithKeys("k", "up"),
		key.WithHelp("[k]", "vol+5"),
	),
	VolDown: key.NewBinding(
		key.WithKeys("j", "down"),
		key.WithHelp("[j]", "vol-5"),
	),
	VolUp1: key.NewBinding(
		key.WithKeys("K"),
		key.WithHelp("[K]", "vol+1"),
	),
	VolDown1: key.NewBinding(
		key.WithKeys("J"),
		key.WithHelp("[J]", "vol-1"),
	),
	LineIn: key.NewBinding(
		key.WithKeys("l"),
		key.WithHelp("[l]", "line-in"),
	),
	Prev: key.NewBinding(
		key.WithKeys("<", ","),
		key.WithHelp("[<]", "prev"),
	),
	Next: key.NewBinding(
		key.WithKeys(">", "."),
		key.WithHelp("[>]", "next"),
	),
	Tab: key.NewBinding(
		key.WithKeys("tab"),
		key.WithHelp("[tab]", "room"),
	),
	Discover: key.NewBinding(
		key.WithKeys("r"),
		key.WithHelp("[r]", "discover"),
	),
	Quit: key.NewBinding(
		key.WithKeys("q", "ctrl+c"),
		key.WithHelp("[q]", "quit"),
	),
}
