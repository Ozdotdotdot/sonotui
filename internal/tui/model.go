package tui

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ── tea.Msg types ─────────────────────────────────────────────────────────────

type tickMsg time.Time
type sseEventMsg SSEEvent
type artFetchedMsg struct {
	url  string
	data string
}
type statusClearMsg struct{}
type errMsg struct{ err error }
type queueAddResultMsg struct{ count int }

// daemonConnectedMsg is sent when initial /status fetch succeeds.
type daemonConnectedMsg StatusResponse

// queueFetchedMsg is sent after fetching the queue.
type queueFetchedMsg []QueueItem

// albumsFetchedMsg is sent after fetching the album list.
type albumsFetchedMsg []Album

// albumDetailFetchedMsg is sent after fetching album tracks.
type albumDetailFetchedMsg struct {
	id     string
	tracks []LibraryEntry
}

// libraryFetchedMsg is sent after browsing a library path.
type libraryFetchedMsg struct {
	path    string
	entries []LibraryEntry
}

// searchResultMsg is sent after a library or album search.
type searchResultMsg struct {
	kind    string // "library" | "albums"
	results []LibraryEntry
	albums  []Album
}

// ── Active tab constants ──────────────────────────────────────────────────────

const (
	TabNowPlaying = 0
	TabQueue      = 1
	TabLibrary    = 2
	TabAlbums     = 3
)

// ── Model ─────────────────────────────────────────────────────────────────────

// Model is the Bubbletea application model for sonotui v2.
type Model struct {
	// Daemon client.
	client    *Client
	connected bool
	sseDone   chan struct{}
	sseEvents chan SSEEvent

	// Playback state (from SSE / /status).
	transport string
	track     TrackInfo
	volume    int
	elapsed   int
	duration  int
	isLineIn  bool
	speakers  []SpeakerInfo
	speaker   *SpeakerInfo
	statusTTL time.Duration

	// UI state.
	activeTab    int
	width        int
	height       int
	status       string
	statusExpiry time.Time

	// Art.
	artRendered string
	artURL      string
	artProto    Protocol

	// Command overlay.
	cmdActive bool
	cmdInput  string

	// Help overlay.
	helpActive bool

	// Tab models.
	queueTab   QueueModel
	libTab     LibraryModel
	albumTab   AlbumModel

	// g-key sequence tracking (for gg/gt/gT).
	gPending bool
}

// NewModel creates the initial v2 Model.
func NewModel(addr DaemonAddr, artProto Protocol) Model {
	m := Model{
		client:    NewClient(addr),
		transport: "STOPPED",
		artProto:  artProto,
		statusTTL: 4 * time.Second,
		sseDone:   make(chan struct{}),
		sseEvents: make(chan SSEEvent, 64),
		queueTab:  NewQueueModel(),
		libTab:    NewLibraryModel(),
		albumTab:  NewAlbumModel(),
	}
	m.artRendered = ArtPlaceholder
	return m
}

// ── Init ──────────────────────────────────────────────────────────────────────

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		cmdConnect(m.client),
		tickCmd(),
	)
}

// ── Update ────────────────────────────────────────────────────────────────────

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.syncTabDimensions()
		return m, nil

	case tickMsg:
		if m.status != "" && time.Now().After(m.statusExpiry) {
			m.clearStatus()
		}
		return m, tickCmd()

	case daemonConnectedMsg:
		m.connected = true
		s := StatusResponse(msg)
		m.applyStatus(&s)
		// Start SSE stream.
		m.client.StreamEvents(m.sseEvents, m.sseDone)
		// Fetch initial data.
		cmds := []tea.Cmd{
			cmdWaitSSE(m.sseEvents),
			cmdFetchQueue(m.client),
			cmdFetchLibrary(m.client, "/"),
		}
		// If already playing when we connect, kick off art fetch now.
		// (SSE track events won't re-trigger because m.artURL is already set.)
		if m.artURL != "" {
			cmds = append(cmds, cmdFetchArt(m.artURL, m.artProto))
		}
		return m, tea.Batch(cmds...)

	case sseEventMsg:
		evt := SSEEvent(msg)
		cmds := m.handleSSEEvent(evt)
		cmds = append(cmds, cmdWaitSSE(m.sseEvents))
		return m, tea.Batch(cmds...)

	case queueFetchedMsg:
		items := []QueueItem(msg)
		m.queueTab.SetItems(items)
		return m, nil

	case libraryFetchedMsg:
		m.libTab.SetEntries(msg.path, msg.entries)
		return m, nil

	case albumsFetchedMsg:
		albums := []Album(msg)
		m.albumTab.SetAlbums(albums)
		return m, nil

	case albumDetailFetchedMsg:
		entries := make([]LibraryEntry, len(msg.tracks))
		copy(entries, msg.tracks)
		m.albumTab.SetExpandTracks(entries)
		return m, nil

	case searchResultMsg:
		switch msg.kind {
		case "library":
			m.libTab.SetSearchResults(msg.results)
		case "albums":
			m.albumTab.SearchResult = msg.albums
		}
		return m, nil

	case artFetchedMsg:
		if msg.url == m.artURL {
			m.artRendered = msg.data
		}
		return m, nil

	case statusClearMsg:
		m.clearStatus()
		return m, nil

	case errMsg:
		log.Printf("error: %v", msg.err)
		m.setStatus(fmt.Sprintf("Error: %v", msg.err))
		return m, nil

	case queueAddResultMsg:
		if msg.count == 1 {
			if m.isLineIn {
				m.setStatus("Added 1 item to queue. Press space to switch from line-in.")
			} else {
				m.setStatus("Added 1 item to queue")
			}
		} else if msg.count > 1 {
			if m.isLineIn {
				m.setStatus(fmt.Sprintf("Added %d items to queue. Press space to switch from line-in.", msg.count))
			} else {
				m.setStatus(fmt.Sprintf("Added %d items to queue", msg.count))
			}
		} else {
			if m.isLineIn {
				m.setStatus("Added selection to queue. Press space to switch from line-in.")
			} else {
				m.setStatus("Added selection to queue")
			}
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

func (m *Model) handleSSEEvent(evt SSEEvent) []tea.Cmd {
	var cmds []tea.Cmd
	switch evt.Type {
	case "transport":
		if s, ok := evt.Raw["state"]; ok {
			m.transport = ParseString(s)
		}
	case "track":
		if t, ok := evt.Raw["title"]; ok {
			m.track.Title = ParseString(t)
		}
		if a, ok := evt.Raw["artist"]; ok {
			m.track.Artist = ParseString(a)
		}
		if al, ok := evt.Raw["album"]; ok {
			m.track.Album = ParseString(al)
		}
		if d, ok := evt.Raw["duration"]; ok {
			m.duration = ParseInt(d)
			m.track.Duration = m.duration
		}
		if au, ok := evt.Raw["art_url"]; ok {
			newURL := ParseString(au)
			m.track.ArtURL = newURL
			if newURL != m.artURL {
				m.artURL = newURL
				if newURL != "" {
					cmds = append(cmds, cmdFetchArt(newURL, m.artProto))
				} else {
					m.artRendered = ArtPlaceholder
				}
			}
		}
	case "position":
		if e, ok := evt.Raw["elapsed"]; ok {
			m.elapsed = ParseInt(e)
		}
		if d, ok := evt.Raw["duration"]; ok {
			m.duration = ParseInt(d)
		}
	case "volume":
		if v, ok := evt.Raw["value"]; ok {
			m.volume = ParseInt(v)
		}
	case "linein":
		if a, ok := evt.Raw["active"]; ok {
			m.isLineIn = ParseBool(a)
		}
	case "queue_changed":
		cmds = append(cmds, cmdFetchQueue(m.client))
	case "speaker":
		sp := &SpeakerInfo{}
		if name, ok := evt.Raw["name"]; ok {
			sp.Name = ParseString(name)
		}
		if uuid, ok := evt.Raw["uuid"]; ok {
			sp.UUID = ParseString(uuid)
		}
		m.speaker = sp
	case "library_scan":
		if s, ok := evt.Raw["status"]; ok {
			status := ParseString(s)
			m.albumTab.ScanStatus = status
			if status == "done" {
				m.albumTab.LibraryReady = true
				cmds = append(cmds, cmdFetchAlbums(m.client))
			}
			if p, ok := evt.Raw["progress"]; ok {
				m.albumTab.ScanProgress = ParseFloat(p)
			}
		}
	case "error":
		if msg, ok := evt.Raw["message"]; ok {
			m.setStatus("Daemon: " + ParseString(msg))
		}
	}
	return cmds
}

func (m *Model) applyStatus(s *StatusResponse) {
	m.transport = s.Transport
	m.track = s.Track
	m.volume = s.Volume
	m.elapsed = s.Elapsed
	m.duration = s.Duration
	m.isLineIn = s.IsLineIn
	m.albumTab.LibraryReady = s.LibraryReady
	m.speaker = s.Speaker
	if s.LibraryReady {
		m.albumTab.ScanStatus = "done"
	} else {
		m.albumTab.ScanStatus = "scanning"
	}

	if artURL := s.Track.ArtURL; artURL != "" {
		m.artURL = artURL
	}
}

// ── Key handling ──────────────────────────────────────────────────────────────

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Command line mode.
	if m.cmdActive {
		return m.handleCmdInput(msg)
	}
	// Help overlay.
	if m.helpActive {
		if key.Matches(msg, globalKeys.Help) || key.Matches(msg, globalKeys.Quit) ||
			msg.String() == "esc" {
			m.helpActive = false
		}
		return m, nil
	}

	// Global keys (available on all tabs).
	switch {
	case key.Matches(msg, globalKeys.Quit):
		close(m.sseDone)
		return m, tea.Quit

	case key.Matches(msg, globalKeys.Help):
		m.helpActive = true
		m.gPending = false
		return m, nil

	case key.Matches(msg, globalKeys.CmdLine):
		m.cmdActive = true
		m.cmdInput = ""
		m.gPending = false
		return m, nil

	case key.Matches(msg, globalKeys.Tab1):
		m.activeTab = TabNowPlaying
		m.gPending = false
		return m, nil

	case key.Matches(msg, globalKeys.Tab2):
		m.activeTab = TabQueue
		m.gPending = false
		return m, nil

	case key.Matches(msg, globalKeys.Tab3):
		m.activeTab = TabLibrary
		m.gPending = false
		return m, nil

	case key.Matches(msg, globalKeys.Tab4):
		m.activeTab = TabAlbums
		m.gPending = false
		if m.albumTab.LibraryReady && len(m.albumTab.Albums) == 0 {
			return m, cmdFetchAlbums(m.client)
		}
		return m, nil

	case msg.String() == "g":
		if m.gPending {
			// gg = jump to top
			m.gPending = false
			switch m.activeTab {
			case TabQueue:
				m.queueTab.JumpTop()
			case TabLibrary:
				m.libTab.JumpTop()
			case TabAlbums:
				m.albumTab.JumpTop()
			}
			return m, nil
		}
		m.gPending = true
		return m, nil

	case msg.String() == "t" && m.gPending:
		m.gPending = false
		m.activeTab = (m.activeTab + 1) % 4
		return m, nil

	case msg.String() == "T" && m.gPending:
		m.gPending = false
		m.activeTab = (m.activeTab + 3) % 4
		return m, nil
	}

	m.gPending = false

	// Transport controls (global, not in search mode).
	if !m.libTab.Searching && !m.albumTab.Searching {
		switch {
		case key.Matches(msg, globalKeys.PlayPause):
			return m, cmdTogglePlayPause(m.client, m.transport, m.isLineIn, len(m.queueTab.Items))
		case key.Matches(msg, globalKeys.Stop):
			return m, cmdPost(m.client, m.client.Stop)
		case key.Matches(msg, globalKeys.Prev):
			return m, cmdPost(m.client, m.client.Prev)
		case key.Matches(msg, globalKeys.Next):
			return m, cmdPost(m.client, m.client.Next)
		case key.Matches(msg, globalKeys.LineIn):
			return m, cmdPost(m.client, m.client.LineIn)
		case key.Matches(msg, globalKeys.Discover):
			if m.activeTab != TabAlbums {
				m.setStatus("Re-discovering speakers…")
				return m, nil
			}
		}
	}

	// Volume (only on Now Playing tab for J/K, global j/k when not in list tabs).
	switch m.activeTab {
	case TabNowPlaying:
		switch {
		case key.Matches(msg, globalKeys.VolUp):
			return m, cmdSetVolumeRel(m.client, 5)
		case key.Matches(msg, globalKeys.VolDown):
			return m, cmdSetVolumeRel(m.client, -5)
		case key.Matches(msg, globalKeys.VolUp1):
			return m, cmdSetVolumeRel(m.client, 1)
		case key.Matches(msg, globalKeys.VolDown1):
			return m, cmdSetVolumeRel(m.client, -1)
		case key.Matches(msg, globalKeys.CycleSpeaker):
			return m.cycleSpeaker()
		}

	case TabQueue:
		return m.handleQueueKey(msg)

	case TabLibrary:
		return m.handleLibraryKey(msg)

	case TabAlbums:
		return m.handleAlbumKey(msg)
	}

	return m, nil
}

func (m Model) cycleSpeaker() (tea.Model, tea.Cmd) {
	if len(m.speakers) <= 1 {
		return m, nil
	}
	// Find current speaker index and advance.
	return m, nil // TODO: implement speaker cycling via /speakers/active
}

// Queue tab key handling.
func (m Model) handleQueueKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	qt := &m.queueTab

	// Clear confirm.
	if qt.ConfirmClear {
		switch msg.String() {
		case "y":
			qt.ConfirmClear = false
			return m, cmdPost(m.client, m.client.ClearQueue)
		case "n", "esc", "q":
			qt.ConfirmClear = false
		}
		return m, nil
	}

	switch {
	case key.Matches(msg, queueKeys.Up):
		qt.CursorUp()
	case key.Matches(msg, queueKeys.Down):
		qt.CursorDown()
	case key.Matches(msg, queueKeys.HalfUp):
		qt.HalfPageUp()
	case key.Matches(msg, queueKeys.HalfDown):
		qt.HalfPageDown()
	case msg.String() == "g":
		if m.gPending {
			qt.JumpTop()
			m.gPending = false
		} else {
			m.gPending = true
		}
	case key.Matches(msg, queueKeys.Bottom):
		qt.JumpBottom()
	case key.Matches(msg, queueKeys.Play):
		if pos := qt.CursorPosition(); pos > 0 {
			return m, func() tea.Msg {
				m.client.PlayFromQueue(pos) //nolint:errcheck
				return nil
			}
		}
	case key.Matches(msg, queueKeys.Delete):
		if qt.HandleD() {
			if pos := qt.CursorPosition(); pos > 0 {
				return m, func() tea.Msg {
					m.client.DeleteFromQueue(pos) //nolint:errcheck
					return nil
				}
			}
		}
	case key.Matches(msg, queueKeys.Clear):
		qt.ConfirmClear = true
	case key.Matches(msg, queueKeys.MoveDown):
		if pos := qt.CursorPosition(); pos > 0 && pos < len(qt.Items) {
			qt.CursorDown()
			p := pos
			return m, func() tea.Msg {
				m.client.ReorderQueue(p, p+1) //nolint:errcheck
				return nil
			}
		}
	case key.Matches(msg, queueKeys.MoveUp):
		if pos := qt.CursorPosition(); pos > 1 {
			qt.CursorUp()
			p := pos
			return m, func() tea.Msg {
				m.client.ReorderQueue(p, p-1) //nolint:errcheck
				return nil
			}
		}
	// Volume still works in queue tab via j/k (overridden by queue up/down).
	// Global transport keys already handled above.
	}
	return m, nil
}

// Library tab key handling.
func (m Model) handleLibraryKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	lt := &m.libTab

	if lt.Searching {
		return m.handleLibrarySearch(msg)
	}

	switch {
	case key.Matches(msg, libraryKeys.Up):
		lt.CursorUp()
	case key.Matches(msg, libraryKeys.Down):
		lt.CursorDown()
	case key.Matches(msg, libraryKeys.HalfUp):
		lt.HalfPageUp()
	case key.Matches(msg, libraryKeys.HalfDown):
		lt.HalfPageDown()
	case msg.String() == "g":
		if m.gPending {
			lt.JumpTop()
			m.gPending = false
		}
	case key.Matches(msg, libraryKeys.Bottom):
		lt.JumpBottom()
	case key.Matches(msg, libraryKeys.Back):
		parent := lt.ParentPath()
		return m, cmdFetchLibrary(m.client, parent)
	case key.Matches(msg, libraryKeys.Enter):
		if e := lt.CurrentEntry(); e != nil {
			if e.Type == "dir" {
				return m, cmdFetchLibrary(m.client, e.Path)
			}
			// File: add to queue.
			path := e.Path
			cl := m.client
			return m, func() tea.Msg {
				if err := cl.AddPathsToQueue([]string{path}); err != nil {
					return errMsg{err}
				}
				return queueAddResultMsg{count: 1}
			}
		}
	case key.Matches(msg, libraryKeys.Add):
		if e := lt.CurrentEntry(); e != nil {
			path := e.Path
			cl := m.client
			if e.Type == "dir" {
				m.setStatus("Adding directory to queue…")
			} else {
				m.setStatus("Adding track to queue…")
			}
			return m, func() tea.Msg {
				if err := cl.AddPathsToQueue([]string{path}); err != nil {
					return errMsg{err}
				}
				if e.Type == "dir" {
					return queueAddResultMsg{count: -1}
				}
				return queueAddResultMsg{count: 1}
			}
		}
	case key.Matches(msg, libraryKeys.AddAll):
		// Collect all file paths in current directory.
		var paths []string
		for _, e := range lt.Entries {
			if e.Type == "file" {
				paths = append(paths, e.Path)
			}
		}
		if len(paths) > 0 {
			cl := m.client
			m.setStatus(fmt.Sprintf("Adding %d tracks to queue…", len(paths)))
			return m, func() tea.Msg {
				if err := cl.AddPathsToQueue(paths); err != nil {
					return errMsg{err}
				}
				return queueAddResultMsg{count: len(paths)}
			}
		}
	case key.Matches(msg, libraryKeys.Search):
		lt.Searching = true
		lt.SearchQuery = ""
	}
	return m, nil
}

func (m Model) handleLibrarySearch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	lt := &m.libTab
	switch {
	case key.Matches(msg, libraryKeys.Escape):
		lt.Searching = false
		lt.SearchQuery = ""
		lt.SearchResults = nil
	case msg.String() == "backspace":
		if len(lt.SearchQuery) > 0 {
			lt.SearchQuery = lt.SearchQuery[:len(lt.SearchQuery)-1]
			if lt.SearchQuery != "" {
				return m, cmdSearchLibrary(m.client, lt.SearchQuery)
			}
			lt.SearchResults = nil
		}
	case key.Matches(msg, libraryKeys.SearchNext):
		if lt.SearchCursor < len(lt.SearchResults)-1 {
			lt.SearchCursor++
		}
	case key.Matches(msg, libraryKeys.SearchPrev):
		if lt.SearchCursor > 0 {
			lt.SearchCursor--
		}
	case key.Matches(msg, libraryKeys.Enter):
		if e := lt.CurrentEntry(); e != nil {
			lt.Searching = false
			lt.SearchQuery = ""
			lt.SearchResults = nil
			if e.Type == "dir" {
				return m, cmdFetchLibrary(m.client, e.Path)
			}
			// Navigate to the file's directory.
			dir := e.Path[:strings.LastIndex(e.Path, "/")]
			if dir == "" {
				dir = "/"
			}
			return m, cmdFetchLibrary(m.client, dir)
		}
	default:
		if len(msg.String()) == 1 {
			lt.SearchQuery += msg.String()
			if lt.SearchQuery != "" {
				return m, cmdSearchLibrary(m.client, lt.SearchQuery)
			}
		}
	}
	return m, nil
}

// Albums tab key handling.
func (m Model) handleAlbumKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	at := &m.albumTab

	if !at.LibraryReady {
		return m, nil
	}

	if at.Searching {
		return m.handleAlbumSearch(msg)
	}

	switch {
	case key.Matches(msg, albumKeys.Up):
		if at.Expanded {
			at.TrackCursorUp()
		} else {
			at.CursorUp()
		}
	case key.Matches(msg, albumKeys.Down):
		if at.Expanded {
			at.TrackCursorDown()
		} else {
			at.CursorDown()
		}
	case key.Matches(msg, albumKeys.TrackUp):
		at.TrackCursorUp()
	case key.Matches(msg, albumKeys.TrackDown):
		at.TrackCursorDown()
	case msg.String() == "g":
		if m.gPending {
			at.JumpTop()
			m.gPending = false
		}
	case key.Matches(msg, albumKeys.Bottom):
		at.JumpBottom()
	case key.Matches(msg, albumKeys.Escape):
		if at.Expanded {
			at.Collapse()
		}
	case key.Matches(msg, albumKeys.Enter):
		if !at.Expanded {
			at.ExpandCurrent()
			if al := at.CurrentAlbum(); al != nil {
				return m, cmdFetchAlbumDetail(m.client, al.ID)
			}
		} else {
			// Second Enter: add album to queue.
			paths := at.CurrentTrackPaths()
			if len(paths) > 0 {
				cl := m.client
				m.setStatus("Adding album to queue…")
				return m, func() tea.Msg {
					cl.AddPathsToQueue(paths) //nolint:errcheck
					return nil
				}
			}
		}
	case key.Matches(msg, albumKeys.Add):
		if at.Expanded {
			paths := at.CurrentTrackPaths()
			if len(paths) > 0 {
				cl := m.client
				m.setStatus("Adding album to queue…")
				return m, func() tea.Msg {
					cl.AddPathsToQueue(paths) //nolint:errcheck
					return nil
				}
			}
		} else if al := at.CurrentAlbum(); al != nil {
			// Need to fetch tracks first.
			cl := m.client
			id := al.ID
			m.setStatus("Adding album to queue…")
			return m, func() tea.Msg {
				detail, err := cl.GetAlbumDetail(id)
				if err != nil {
					return errMsg{err}
				}
				var paths []string
				for _, t := range detail.Tracks {
					paths = append(paths, t.Path)
				}
				cl.AddPathsToQueue(paths) //nolint:errcheck
				return nil
			}
		}
	case key.Matches(msg, albumKeys.Search):
		at.Searching = true
		at.SearchQuery = ""
	case key.Matches(msg, albumKeys.Rescan):
		m.setStatus("Rescanning library…")
		// Trigger rescan via command line / daemon API (not yet exposed — no-op for now).
	}
	return m, nil
}

func (m Model) handleAlbumSearch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	at := &m.albumTab
	switch {
	case key.Matches(msg, albumKeys.Escape):
		at.Searching = false
		at.SearchQuery = ""
		at.SearchResult = nil
	case msg.String() == "backspace":
		if len(at.SearchQuery) > 0 {
			at.SearchQuery = at.SearchQuery[:len(at.SearchQuery)-1]
			if at.SearchQuery != "" {
				return m, cmdSearchAlbums(m.client, at.SearchQuery)
			}
			at.SearchResult = nil
		}
	case key.Matches(msg, albumKeys.SearchNext):
		if at.Cursor < len(at.SearchResult)-1 {
			at.Cursor++
		}
	case key.Matches(msg, albumKeys.SearchPrev):
		if at.Cursor > 0 {
			at.Cursor--
		}
	case key.Matches(msg, albumKeys.Enter):
		at.Searching = false
		at.SearchQuery = ""
		if len(at.SearchResult) > 0 {
			at.Albums = at.SearchResult
			at.SearchResult = nil
			at.Cursor = 0
		}
	default:
		if len(msg.String()) == 1 {
			at.SearchQuery += msg.String()
			return m, cmdSearchAlbums(m.client, at.SearchQuery)
		}
	}
	return m, nil
}

// Command line handling.
func (m Model) handleCmdInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.cmdActive = false
		m.cmdInput = ""
		return m, nil
	case "enter":
		cmd := strings.TrimSpace(m.cmdInput)
		m.cmdActive = false
		m.cmdInput = ""
		return m.execCommand(cmd)
	case "backspace":
		if len(m.cmdInput) > 0 {
			m.cmdInput = m.cmdInput[:len(m.cmdInput)-1]
		}
	default:
		if len(msg.String()) == 1 {
			m.cmdInput += msg.String()
		}
	}
	return m, nil
}

func (m Model) execCommand(cmd string) (tea.Model, tea.Cmd) {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return m, nil
	}
	switch parts[0] {
	case "q", "quit":
		close(m.sseDone)
		return m, tea.Quit
	case "help":
		m.helpActive = true
	case "rescan":
		m.setStatus("Rescanning library…")
	case "speaker", "room":
		if len(parts) > 1 {
			name := strings.Join(parts[1:], " ")
			// Fuzzy match speakers by name.
			for _, sp := range m.speakers {
				if strings.Contains(strings.ToLower(sp.Name), strings.ToLower(name)) {
					uuid := sp.UUID
					return m, func() tea.Msg {
						m.client.SetActiveSpeaker(uuid) //nolint:errcheck
						return nil
					}
				}
			}
			m.setStatus("Speaker not found: " + name)
		}
	default:
		m.setStatus("Unknown command: " + parts[0])
	}
	return m, nil
}

// ── View ──────────────────────────────────────────────────────────────────────

func (m Model) View() string {
	if m.width == 0 {
		return ""
	}

	if m.helpActive {
		return m.helpOverlay()
	}

	// Render tab bar + active tab.
	var content string
	switch m.activeTab {
	case TabNowPlaying:
		content = NowPlayingView(NowPlayingState{
			Transport:   m.transport,
			Track:       m.track,
			Volume:      m.volume,
			Elapsed:     m.elapsed,
			Duration:    m.duration,
			IsLineIn:    m.isLineIn,
			ArtRendered: m.artRendered,
			SpeakerName: m.activeSpeakerName(),
			StatusMsg:   m.status,
		}, m.width, m.height)
	case TabQueue:
		content = m.queueTab.View()
	case TabLibrary:
		content = m.libTab.View()
	case TabAlbums:
		content = m.albumTab.View()
	}

	// Command line overlay at bottom.
	if m.cmdActive {
		lines := strings.Split(content, "\n")
		if len(lines) > 0 {
			cmdLine := cmdPromptStyle.Render(":") + cmdInputStyle.Render(m.cmdInput) + "█"
			lines[len(lines)-1] = cmdLine
		}
		content = strings.Join(lines, "\n")
	}

	return content
}

func (m Model) activeSpeakerName() string {
	if m.speaker != nil && m.speaker.Name != "" {
		return m.speaker.Name
	}
	if len(m.speakers) > 0 {
		return m.speakers[0].Name
	}
	return ""
}

func (m *Model) syncTabDimensions() {
	m.queueTab.Width = m.width
	m.queueTab.Height = m.height
	m.queueTab.StatusMsg = m.status
	m.libTab.Width = m.width
	m.libTab.Height = m.height
	m.libTab.StatusMsg = m.status
	m.albumTab.Width = m.width
	m.albumTab.Height = m.height
	m.albumTab.StatusMsg = m.status
}

func (m *Model) setStatus(msg string) {
	m.status = msg
	m.statusExpiry = time.Now().Add(m.statusTTL)
	m.queueTab.StatusMsg = msg
	m.libTab.StatusMsg = msg
	m.albumTab.StatusMsg = msg
}

func (m *Model) clearStatus() {
	m.status = ""
	m.queueTab.StatusMsg = ""
	m.libTab.StatusMsg = ""
	m.albumTab.StatusMsg = ""
}

func (m *Model) SetStatusTTL(d time.Duration) {
	if d <= 0 {
		return
	}
	m.statusTTL = d
}

func (m Model) helpOverlay() string {
	lines := []string{
		helpSectionStyle.Render("Global"),
		helpItemStyle.Render("  1-4        Switch tab"),
		helpItemStyle.Render("  gt / gT    Cycle tabs"),
		helpItemStyle.Render("  tab        Cycle speakers"),
		helpItemStyle.Render("  space      Play / pause"),
		helpItemStyle.Render("  s          Stop"),
		helpItemStyle.Render("  < / >      Prev / next track"),
		helpItemStyle.Render("  j / k      Volume -5 / +5  (Now Playing)"),
		helpItemStyle.Render("  l          Switch to line-in"),
		helpItemStyle.Render("  :          Command line"),
		helpItemStyle.Render("  ?          Toggle help"),
		helpItemStyle.Render("  q          Quit"),
		"",
		helpSectionStyle.Render("Queue tab"),
		helpItemStyle.Render("  p          Play from cursor"),
		helpItemStyle.Render("  dd         Delete track"),
		helpItemStyle.Render("  D          Clear queue"),
		helpItemStyle.Render("  J / K      Move track down / up"),
		"",
		helpSectionStyle.Render("Library / Albums tab"),
		helpItemStyle.Render("  a          Add to queue"),
		helpItemStyle.Render("  A          Add all / add album"),
		helpItemStyle.Render("  /          Search"),
		helpItemStyle.Render("  n / N      Next / prev match"),
		helpItemStyle.Render("  enter      Open / expand"),
		helpItemStyle.Render("  backspace  Go up  (Library)"),
		helpItemStyle.Render("  r          Rescan library  (Albums)"),
		"",
		helpItemStyle.Render("  Press ? or Esc to close"),
	}

	box := helpOverlayBorder.Render(strings.Join(lines, "\n"))
	// Center the overlay.
	bw := lipgloss.Width(box)
	bh := lipgloss.Height(box)
	leftPad := (m.width - bw) / 2
	topPad := (m.height - bh) / 2
	if leftPad < 0 {
		leftPad = 0
	}
	if topPad < 0 {
		topPad = 0
	}
	var b strings.Builder
	for i := 0; i < topPad; i++ {
		b.WriteString("\n")
	}
	for _, line := range strings.Split(box, "\n") {
		b.WriteString(strings.Repeat(" ", leftPad) + line + "\n")
	}
	return b.String()
}

// ── tea.Cmd factories ─────────────────────────────────────────────────────────

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func cmdConnect(c *Client) tea.Cmd {
	return func() tea.Msg {
		// Retry until connected.
		for i := 0; i < 3; i++ {
			s, err := c.GetStatus()
			if err == nil {
				return daemonConnectedMsg(*s)
			}
			time.Sleep(500 * time.Millisecond)
		}
		return errMsg{fmt.Errorf("daemon not reachable")}
	}
}

func cmdWaitSSE(ch <-chan SSEEvent) tea.Cmd {
	return func() tea.Msg {
		return sseEventMsg(<-ch)
	}
}

func cmdFetchQueue(c *Client) tea.Cmd {
	return func() tea.Msg {
		items, err := c.GetQueue()
		if err != nil {
			return errMsg{err}
		}
		return queueFetchedMsg(items)
	}
}

func cmdFetchLibrary(c *Client, path string) tea.Cmd {
	return func() tea.Msg {
		entries, err := c.BrowseLibrary(path)
		if err != nil {
			return errMsg{err}
		}
		return libraryFetchedMsg{path: path, entries: entries}
	}
}

func cmdFetchAlbums(c *Client) tea.Cmd {
	return func() tea.Msg {
		albums, err := c.GetAlbums()
		if err != nil {
			return errMsg{err}
		}
		return albumsFetchedMsg(albums)
	}
}

func cmdFetchAlbumDetail(c *Client, id string) tea.Cmd {
	return func() tea.Msg {
		detail, err := c.GetAlbumDetail(id)
		if err != nil {
			return errMsg{err}
		}
		return albumDetailFetchedMsg{id: id, tracks: detail.Tracks}
	}
}

func cmdSearchLibrary(c *Client, q string) tea.Cmd {
	return func() tea.Msg {
		results, err := c.SearchLibrary(q)
		if err != nil {
			return errMsg{err}
		}
		return searchResultMsg{kind: "library", results: results}
	}
}

func cmdSearchAlbums(c *Client, q string) tea.Cmd {
	return func() tea.Msg {
		albums, err := c.SearchAlbums(q)
		if err != nil {
			return errMsg{err}
		}
		return searchResultMsg{kind: "albums", albums: albums}
	}
}

func cmdTogglePlayPause(c *Client, transport string, isLineIn bool, queueLen int) tea.Cmd {
	return func() tea.Msg {
		var err error
		switch {
		case transport == "PLAYING" && !isLineIn:
			err = c.Pause()
		case isLineIn && queueLen > 0:
			err = c.PlayFromQueue(1)
		default:
			err = c.Play()
		}
		if err != nil {
			return errMsg{err}
		}
		return nil
	}
}

func cmdPost(c *Client, fn func() error) tea.Cmd {
	return func() tea.Msg {
		if err := fn(); err != nil {
			return errMsg{err}
		}
		return nil
	}
}

func cmdSetVolumeRel(c *Client, delta int) tea.Cmd {
	return func() tea.Msg {
		if err := c.SetVolumeRelative(delta); err != nil {
			return errMsg{err}
		}
		return nil
	}
}

func cmdFetchArt(url string, proto Protocol) tea.Cmd {
	return func() tea.Msg {
		rendered := FetchAndRenderArt(url, proto)
		return artFetchedMsg{url: url, data: rendered}
	}
}
