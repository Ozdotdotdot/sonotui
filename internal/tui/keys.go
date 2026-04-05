package tui

import "github.com/charmbracelet/bubbles/key"

// GlobalKeys are available on all tabs.
type globalKeyMap struct {
	Tab1      key.Binding
	Tab2      key.Binding
	Tab3      key.Binding
	Tab4      key.Binding
	TabNext   key.Binding
	TabPrev   key.Binding
	CycleSpeaker key.Binding
	PlayPause key.Binding
	Stop      key.Binding
	Prev      key.Binding
	Next      key.Binding
	VolUp     key.Binding
	VolDown   key.Binding
	VolUp1    key.Binding
	VolDown1  key.Binding
	LineIn    key.Binding
	Discover  key.Binding
	CmdLine   key.Binding
	Help      key.Binding
	Quit      key.Binding
}

var globalKeys = globalKeyMap{
	Tab1: key.NewBinding(key.WithKeys("1"), key.WithHelp("1", "Now Playing")),
	Tab2: key.NewBinding(key.WithKeys("2"), key.WithHelp("2", "Queue")),
	Tab3: key.NewBinding(key.WithKeys("3"), key.WithHelp("3", "Library")),
	Tab4: key.NewBinding(key.WithKeys("4"), key.WithHelp("4", "Albums")),
	TabNext: key.NewBinding(key.WithKeys("g", "t"), key.WithHelp("gt", "next tab")),
	TabPrev: key.NewBinding(key.WithKeys("g", "T"), key.WithHelp("gT", "prev tab")),
	CycleSpeaker: key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "cycle speaker")),
	PlayPause: key.NewBinding(key.WithKeys(" "), key.WithHelp("spc", "play/pause")),
	Stop:      key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "stop")),
	Prev:      key.NewBinding(key.WithKeys("<", ","), key.WithHelp("<", "prev")),
	Next:      key.NewBinding(key.WithKeys(">", "."), key.WithHelp(">", "next")),
	VolUp:     key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("k", "vol+5")),
	VolDown:   key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("j", "vol-5")),
	VolUp1:    key.NewBinding(key.WithKeys("K"), key.WithHelp("K", "vol+1")),
	VolDown1:  key.NewBinding(key.WithKeys("J"), key.WithHelp("J", "vol-1")),
	LineIn:    key.NewBinding(key.WithKeys("l"), key.WithHelp("l", "line-in")),
	Discover:  key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "discover")),
	CmdLine:   key.NewBinding(key.WithKeys(":"), key.WithHelp(":", "command")),
	Help:      key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
	Quit:      key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
}

// QueueKeys are active in the Queue tab.
type queueKeyMap struct {
	Play   key.Binding
	Delete key.Binding
	Clear  key.Binding
	MoveUp key.Binding
	MoveDown key.Binding
	Top    key.Binding
	Bottom key.Binding
	Up     key.Binding
	Down   key.Binding
	HalfUp key.Binding
	HalfDown key.Binding
}

var queueKeys = queueKeyMap{
	Play:     key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "play from here")),
	Delete:   key.NewBinding(key.WithKeys("d"), key.WithHelp("dd", "delete track")),
	Clear:    key.NewBinding(key.WithKeys("D"), key.WithHelp("D", "clear queue")),
	MoveUp:   key.NewBinding(key.WithKeys("K"), key.WithHelp("K", "move up")),
	MoveDown: key.NewBinding(key.WithKeys("J"), key.WithHelp("J", "move down")),
	Top:      key.NewBinding(key.WithKeys("g"), key.WithHelp("gg", "top")),
	Bottom:   key.NewBinding(key.WithKeys("G"), key.WithHelp("G", "bottom")),
	Up:       key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("k", "up")),
	Down:     key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("j", "down")),
	HalfUp:   key.NewBinding(key.WithKeys("ctrl+u"), key.WithHelp("C-u", "half page up")),
	HalfDown: key.NewBinding(key.WithKeys("ctrl+d"), key.WithHelp("C-d", "half page down")),
}

// LibraryKeys are active in the Library tab.
type libraryKeyMap struct {
	Enter    key.Binding
	Add      key.Binding
	AddAll   key.Binding
	Back     key.Binding
	Search   key.Binding
	SearchNext key.Binding
	SearchPrev key.Binding
	Escape   key.Binding
	Top      key.Binding
	Bottom   key.Binding
	Up       key.Binding
	Down     key.Binding
	HalfUp   key.Binding
	HalfDown key.Binding
}

var libraryKeys = libraryKeyMap{
	Enter:      key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open/add")),
	Add:        key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "add to queue")),
	AddAll:     key.NewBinding(key.WithKeys("A"), key.WithHelp("A", "add all")),
	Back:       key.NewBinding(key.WithKeys("backspace"), key.WithHelp("⌫", "go up")),
	Search:     key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search")),
	SearchNext: key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "next match")),
	SearchPrev: key.NewBinding(key.WithKeys("N"), key.WithHelp("N", "prev match")),
	Escape:     key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "clear search")),
	Top:        key.NewBinding(key.WithKeys("g"), key.WithHelp("gg", "top")),
	Bottom:     key.NewBinding(key.WithKeys("G"), key.WithHelp("G", "bottom")),
	Up:         key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("k", "up")),
	Down:       key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("j", "down")),
	HalfUp:     key.NewBinding(key.WithKeys("ctrl+u"), key.WithHelp("C-u", "half up")),
	HalfDown:   key.NewBinding(key.WithKeys("ctrl+d"), key.WithHelp("C-d", "half down")),
}

// AlbumKeys are active in the Albums tab.
type albumKeyMap struct {
	Enter      key.Binding
	Add        key.Binding
	Search     key.Binding
	SearchNext key.Binding
	SearchPrev key.Binding
	Escape     key.Binding
	Rescan     key.Binding
	Top        key.Binding
	Bottom     key.Binding
	Up         key.Binding
	Down       key.Binding
	TrackUp    key.Binding
	TrackDown  key.Binding
}

var albumKeys = albumKeyMap{
	Enter:      key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "expand/add")),
	Add:        key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "add album")),
	Search:     key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search")),
	SearchNext: key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "next match")),
	SearchPrev: key.NewBinding(key.WithKeys("N"), key.WithHelp("N", "prev match")),
	Escape:     key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "close")),
	Rescan:     key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "rescan")),
	Top:        key.NewBinding(key.WithKeys("g"), key.WithHelp("gg", "top")),
	Bottom:     key.NewBinding(key.WithKeys("G"), key.WithHelp("G", "bottom")),
	Up:         key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("k", "up")),
	Down:       key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("j", "down")),
	TrackUp:    key.NewBinding(key.WithKeys("K"), key.WithHelp("K", "track up")),
	TrackDown:  key.NewBinding(key.WithKeys("J"), key.WithHelp("J", "track down")),
}
