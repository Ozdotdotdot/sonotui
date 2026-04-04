package tui

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ozdotdotdot/sonotui/sonos"
)

// ── tea.Msg types ────────────────────────────────────────────────────────────

type tickMsg time.Time

type speakersDiscoveredMsg []sonos.Speaker

type notifyServerStartedMsg struct {
	ns  *sonos.NotifyServer
	err error
}

type subscriptionStartedMsg struct {
	subs []*sonos.Subscription
	err  error
}

type genaEventMsg sonos.GENAEvent

type positionSyncedMsg struct{ elapsed, duration int }

type artFetchedMsg struct {
	url  string
	data string // pre-rendered escape string
}

type statusClearMsg struct{}

type errMsg struct{ err error }

// ── position state ───────────────────────────────────────────────────────────

type positionState struct {
	duration int  // total track length in seconds
	elapsed  int  // current elapsed seconds (locally maintained)
	playing  bool // true when transport state is PLAYING
}

// ── model ────────────────────────────────────────────────────────────────────

// Model is the Bubbletea application model.
type Model struct {
	// Speakers
	speakers      []sonos.Speaker
	activeSpeaker int

	// Subscriptions
	subscriptions []*sonos.Subscription
	notify        *sonos.NotifyServer

	// Playback state (updated by GENA events)
	trackInfo sonos.TrackInfo
	transport string // PLAYING | PAUSED_PLAYBACK | STOPPED | TRANSITIONING
	volume    int    // 0–100
	muted     bool
	isLineIn  bool

	// Position (local counter)
	pos positionState

	// UI state
	status       string
	statusExpiry time.Time
	width        int
	height       int
	artRendered  string // pre-rendered escape string
	artURL       string // URL currently rendered (change detection)

	// Misc
	discovering bool
	artProto    Protocol
	portOverride int
}

// NewModel creates the initial model.
func NewModel(artProto Protocol, portOverride int) Model {
	return Model{
		transport:    "STOPPED",
		artProto:     artProto,
		portOverride: portOverride,
	}
}

// NewModelWithSpeakers creates a model pre-populated with known speakers
// (e.g. from saved config or --speaker flag). Discovery still runs in Init.
func NewModelWithSpeakers(artProto Protocol, portOverride int, speakers []sonos.Speaker) Model {
	m := NewModel(artProto, portOverride)
	m.speakers = speakers
	return m
}

// ActiveSpeaker returns a pointer to the currently active speaker, or nil.
func (m Model) ActiveSpeaker() *sonos.Speaker {
	if len(m.speakers) == 0 {
		return nil
	}
	sp := m.speakers[m.activeSpeaker]
	return &sp
}

// ── Init ─────────────────────────────────────────────────────────────────────

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		cmdDiscoverSpeakers(),
		cmdStartNotifyServer(m.portOverride),
		tickCmd(),
	)
}

// ── Update ───────────────────────────────────────────────────────────────────

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tickMsg:
		if m.pos.playing && !m.isLineIn {
			m.pos.elapsed++
			if m.pos.duration > 0 && m.pos.elapsed > m.pos.duration {
				m.pos.elapsed = m.pos.duration
			}
		}
		// Clear ephemeral status if expired.
		if m.status != "" && time.Now().After(m.statusExpiry) {
			m.status = ""
		}
		return m, tickCmd()

	case speakersDiscoveredMsg:
		m.discovering = false
		speakers := []sonos.Speaker(msg)
		if len(speakers) == 0 && len(m.speakers) == 0 {
			m.setStatus("No Sonos speakers found — press [r] to retry")
			return m, nil
		}
		// Merge new results, preserving the active speaker if possible.
		m.speakers = speakers
		if m.activeSpeaker >= len(m.speakers) {
			m.activeSpeaker = 0
		}
		// If we already have a notify server, subscribe to the active speaker.
		if m.notify != nil && len(m.speakers) > 0 {
			return m, cmdSubscribe(m.speakers[m.activeSpeaker], m.notify.CallbackURL())
		}
		return m, nil

	case notifyServerStartedMsg:
		if msg.err != nil {
			log.Printf("notify server error: %v", msg.err)
			m.setStatus("Failed to start event server")
			return m, nil
		}
		m.notify = msg.ns
		var cmds []tea.Cmd
		cmds = append(cmds, cmdWaitGENAEvent(m.notify.EventCh))
		if len(m.speakers) > 0 {
			cmds = append(cmds, cmdSubscribe(m.speakers[m.activeSpeaker], m.notify.CallbackURL()))
		}
		return m, tea.Batch(cmds...)

	case subscriptionStartedMsg:
		if msg.err != nil {
			log.Printf("subscribe error: %v", msg.err)
			m.setStatus("Subscription failed")
			return m, nil
		}
		for _, sub := range msg.subs {
			sub.StartRenewal(nil)
		}
		m.subscriptions = append(m.subscriptions, msg.subs...)
		return m, nil

	case genaEventMsg:
		event := sonos.GENAEvent(msg)
		var cmds []tea.Cmd

		// Resolve service from SID.
		service := m.resolveService(event.SID)
		if service == "" {
			service = event.Service
		}

		switch service {
		case "AVTransport":
			cmds = append(cmds, m.handleAVTransportEvent(event.Body)...)
		case "RenderingControl":
			m.handleRenderingControlEvent(event.Body)
		// ZoneGroupTopology events ignored for v0.1
		}

		cmds = append(cmds, cmdWaitGENAEvent(m.notify.EventCh))
		return m, tea.Batch(cmds...)

	case positionSyncedMsg:
		m.pos.elapsed = msg.elapsed
		if msg.duration > 0 {
			m.pos.duration = msg.duration
		}
		return m, nil

	case artFetchedMsg:
		if msg.url == m.artURL {
			m.artRendered = msg.data
		}
		return m, nil

	case statusClearMsg:
		m.status = ""
		return m, nil

	case errMsg:
		log.Printf("error: %v", msg.err)
		m.setStatus(fmt.Sprintf("Error: %v", msg.err))
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

func (m *Model) handleAVTransportEvent(body []byte) []tea.Cmd {
	if len(m.speakers) == 0 {
		return nil
	}
	speakerIP := m.speakers[m.activeSpeaker].IP

	state, err := sonos.ParseAVTransportEvent(speakerIP, body)
	if err != nil {
		log.Printf("parse AVTransport event: %v", err)
		return nil
	}

	prevTransport := m.transport
	prevURI := ""
	if m.isLineIn {
		prevURI = "x-rincon-stream:"
	}

	m.transport = state.TransportState
	m.pos.playing = state.TransportState == "PLAYING"

	var cmds []tea.Cmd

	// Detect line-in.
	if strings.HasPrefix(state.CurrentTrackURI, "x-rincon-stream:") {
		m.isLineIn = true
		m.trackInfo = sonos.TrackInfo{}
		m.artURL = ""
		m.artRendered = artPlaceholder
	} else {
		m.isLineIn = false

		// On track change: sync position + art.
		if state.CurrentTrackURI != "" && state.CurrentTrackURI != prevURI {
			cmds = append(cmds, cmdGetPositionInfo(speakerIP))
		}

		// Update track info from event if present.
		if state.Track.Title != "" || state.Track.ArtURL != "" {
			m.trackInfo = state.Track
			if state.Track.ArtURL != m.artURL {
				m.artURL = state.Track.ArtURL
				if state.Track.ArtURL != "" {
					cmds = append(cmds, cmdFetchArt(state.Track.ArtURL, m.artProto))
				} else {
					m.artRendered = artPlaceholder
				}
			}
		}
		if state.Duration > 0 {
			m.pos.duration = state.Duration
		}
	}

	// On resume from pause/stop → sync position.
	if m.transport == "PLAYING" &&
		(prevTransport == "PAUSED_PLAYBACK" || prevTransport == "STOPPED") {
		cmds = append(cmds, cmdGetPositionInfo(speakerIP))
	}

	// On pause/stop → freeze counter.
	if m.transport == "PAUSED_PLAYBACK" || m.transport == "STOPPED" {
		m.pos.playing = false
	}

	return cmds
}

func (m *Model) handleRenderingControlEvent(body []byte) {
	state, err := sonos.ParseRenderingControlEvent(body)
	if err != nil {
		log.Printf("parse RenderingControl event: %v", err)
		return
	}
	if state.Volume >= 0 {
		m.volume = state.Volume
	}
	if state.HasMute {
		m.muted = state.Muted
	}
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if len(m.speakers) == 0 {
		// Only allow discover and quit when no speakers.
		switch {
		case key.Matches(msg, keys.Discover):
			return m.doDiscover()
		case key.Matches(msg, keys.Quit):
			return m.doQuit()
		}
		return m, nil
	}

	sp := m.speakers[m.activeSpeaker]

	switch {
	case key.Matches(msg, keys.Quit):
		return m.doQuit()

	case key.Matches(msg, keys.PlayPause):
		return m, cmdTogglePlayPause(sp.IP, m.transport)

	case key.Matches(msg, keys.Stop):
		if m.isLineIn {
			m.setStatus("Stop not available in Line-In mode")
			return m, nil
		}
		return m, cmdStop(sp.IP)

	case key.Matches(msg, keys.Mute):
		return m, cmdToggleMute(sp.IP, m.muted)

	case key.Matches(msg, keys.VolUp):
		return m, cmdSetVolume(sp.IP, m.volume+5)

	case key.Matches(msg, keys.VolDown):
		return m, cmdSetVolume(sp.IP, m.volume-5)

	case key.Matches(msg, keys.VolUp1):
		return m, cmdSetVolume(sp.IP, m.volume+1)

	case key.Matches(msg, keys.VolDown1):
		return m, cmdSetVolume(sp.IP, m.volume-1)

	case key.Matches(msg, keys.LineIn):
		if m.isLineIn {
			m.setStatus("Already on Line-In")
			return m, nil
		}
		return m, cmdSwitchToLineIn(sp.IP, sp.UUID)

	case key.Matches(msg, keys.Prev):
		if m.isLineIn {
			m.setStatus("Not available in Line-In mode")
			return m, nil
		}
		return m, cmdPrev(sp.IP)

	case key.Matches(msg, keys.Next):
		if m.isLineIn {
			m.setStatus("Not available in Line-In mode")
			return m, nil
		}
		return m, cmdNext(sp.IP)

	case key.Matches(msg, keys.Tab):
		if len(m.speakers) <= 1 {
			return m, nil
		}
		return m.doSwitchSpeaker()

	case key.Matches(msg, keys.Discover):
		return m.doDiscover()
	}

	return m, nil
}

func (m Model) doQuit() (tea.Model, tea.Cmd) {
	// Unsubscribe all in goroutine so we don't block.
	subs := m.subscriptions
	go func() {
		for _, sub := range subs {
			sub.StopRenewal()
			sub.Unsubscribe()
		}
	}()
	if m.notify != nil {
		go m.notify.Shutdown()
	}
	return m, tea.Quit
}

func (m Model) doDiscover() (tea.Model, tea.Cmd) {
	if m.discovering {
		return m, nil
	}
	m.discovering = true
	m.setStatus("Discovering speakers...")
	return m, cmdDiscoverSpeakers()
}

func (m Model) doSwitchSpeaker() (tea.Model, tea.Cmd) {
	// Unsubscribe from current speaker.
	subs := m.subscriptions
	go func() {
		for _, sub := range subs {
			sub.StopRenewal()
			sub.Unsubscribe()
		}
	}()
	m.subscriptions = nil

	// Advance to next speaker.
	m.activeSpeaker = (m.activeSpeaker + 1) % len(m.speakers)

	// Reset playback state.
	m.trackInfo = sonos.TrackInfo{}
	m.transport = "STOPPED"
	m.pos = positionState{}
	m.artRendered = artPlaceholder
	m.artURL = ""

	// Subscribe to new speaker.
	sp := m.speakers[m.activeSpeaker]
	m.setStatus("Switched to " + sp.FriendlyName)
	if m.notify != nil {
		return m, cmdSubscribe(sp, m.notify.CallbackURL())
	}
	return m, nil
}

// resolveService maps a SID to a service name using active subscriptions.
func (m *Model) resolveService(sid string) string {
	for _, sub := range m.subscriptions {
		if sub.SID == sid {
			return sub.Service
		}
	}
	return ""
}

func (m *Model) setStatus(msg string) {
	m.status = msg
	m.statusExpiry = time.Now().Add(2 * time.Second)
}

// ── View ─────────────────────────────────────────────────────────────────────

func (m Model) View() string {
	if m.width == 0 {
		return ""
	}
	innerW := m.width - 2 // space inside the │ │ frame chars

	var b strings.Builder

	// Outer top border.
	b.WriteString("╭" + strings.Repeat("─", innerW) + "╮\n")

	// Header line.
	b.WriteString(m.frameLine(m.headerContent(innerW), innerW))
	b.WriteString("\n")

	// Separator.
	b.WriteString("╞" + strings.Repeat("═", innerW) + "╡\n")

	// Content area — 1 space padding each side.
	contentW := innerW - 2
	var contentLines []string
	if m.width >= 80 {
		contentLines = m.wideContent(contentW)
	} else {
		contentLines = m.narrowContent(contentW)
	}
	b.WriteString(m.frameLine("", innerW) + "\n") // top breathing room
	for _, cl := range contentLines {
		b.WriteString(m.frameLine(" "+cl, innerW) + "\n")
	}
	b.WriteString(m.frameLine("", innerW) + "\n") // bottom breathing room

	// Separator.
	b.WriteString("╞" + strings.Repeat("═", innerW) + "╡\n")

	// Help lines.
	for _, hl := range m.helpLines(innerW) {
		b.WriteString(m.frameLine(hl, innerW) + "\n")
	}

	// Outer bottom border.
	b.WriteString("╰" + strings.Repeat("─", innerW) + "╯")

	return b.String()
}

// frameLine wraps content in │...│ padding to exactly innerW chars.
func (m Model) frameLine(content string, innerW int) string {
	visW := lipgloss.Width(content)
	pad := innerW - visW
	if pad < 0 {
		pad = 0
	}
	return "│" + content + strings.Repeat(" ", pad) + "│"
}

// headerContent builds the header row content string (exactly innerW visual chars).
func (m Model) headerContent(innerW int) string {
	brand := headerBrandStyle.Render(" sonotui")
	sep := headerRoomStyle.Render(" › ")

	room := ""
	if len(m.speakers) > 0 {
		room = m.speakers[m.activeSpeaker].FriendlyName
	} else if m.discovering {
		room = "discovering…"
	}
	roomStr := headerRoomStyle.Render(room)

	left := brand + sep + roomStr

	muteStr := ""
	if m.muted {
		muteStr = " 🔇"
	}
	state := transportStateStyle(m.transport)
	vol := volLabelStyle.Render(fmt.Sprintf("vol:%d", m.volume))
	right := state + muteStr + "   " + vol + " "

	gap := innerW - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

// wideContent returns the lines of the content area for wide terminals.
func (m Model) wideContent(availW int) []string {
	const artCardW = 22
	const rowGap = 2
	trackCardW := availW - artCardW - rowGap
	if trackCardW < 20 {
		trackCardW = 20
	}
	const cardH = 9 // total card height including border

	artCard := m.renderArtCard(artCardW, cardH)
	trackCard := m.renderTrackCard(trackCardW, cardH)
	row1 := lipgloss.JoinHorizontal(lipgloss.Top, artCard, strings.Repeat(" ", rowGap), trackCard)

	// Bottom row: vol + status cards.
	volCardW := availW * 2 / 5
	statusCardW := availW - volCardW - rowGap
	const bottomH = 4
	volCard := m.renderVolCard(volCardW, bottomH)
	statusCard := m.renderStatusCard(statusCardW, bottomH)
	row2 := lipgloss.JoinHorizontal(lipgloss.Top, volCard, strings.Repeat(" ", rowGap), statusCard)

	var lines []string
	lines = append(lines, strings.Split(row1, "\n")...)
	lines = append(lines, "") // gap between rows
	lines = append(lines, strings.Split(row2, "\n")...)
	return lines
}

// narrowContent returns lines for narrow terminals (<80 cols) — single column.
func (m Model) narrowContent(availW int) []string {
	const artH = 7
	artCard := m.renderArtCard(availW, artH)

	const trackH = 9
	trackCard := m.renderTrackCard(availW, trackH)

	const volH = 4
	volCard := m.renderVolCard(availW/2-1, volH)
	statusCard := m.renderStatusCard(availW-availW/2-1, volH)
	bottomRow := lipgloss.JoinHorizontal(lipgloss.Top, volCard, " ", statusCard)

	var lines []string
	lines = append(lines, strings.Split(artCard, "\n")...)
	lines = append(lines, "")
	lines = append(lines, strings.Split(trackCard, "\n")...)
	lines = append(lines, "")
	lines = append(lines, strings.Split(bottomRow, "\n")...)
	return lines
}

// renderArtCard renders the album art card.
func (m Model) renderArtCard(totalW, totalH int) string {
	art := m.artRendered
	if art == "" {
		art = artPlaceholder
	}
	return cardStyle(artCardBorderColor, totalW, totalH).
		Align(lipgloss.Center, lipgloss.Center).
		Render(art)
}

// renderTrackCard renders the track info + progress card.
func (m Model) renderTrackCard(totalW, totalH int) string {
	contentW := totalW - 4 // border + padding
	if contentW < 4 {
		contentW = 4
	}

	var lines []string
	if m.isLineIn {
		lines = []string{
			trackTitleStyle.Render("Line-In"),
			trackArtistStyle.Render("analog source"),
			"",
			liveStyle.Render("── LIVE ──"),
		}
	} else {
		title := trackTitleStyle.Render(truncate(m.trackInfo.Title, contentW))
		artist := trackArtistStyle.Render(truncate(m.trackInfo.Artist, contentW))
		album := trackAlbumStyle.Render(truncate(m.trackInfo.Album, contentW))
		progress := m.renderProgressBar(contentW)
		lines = []string{title, artist, album, "", progress}
	}

	return cardStyle(trackCardBorderColor, totalW, totalH).
		Render(strings.Join(lines, "\n"))
}

// renderVolCard renders the volume card.
func (m Model) renderVolCard(totalW, totalH int) string {
	contentW := totalW - 4
	if contentW < 4 {
		contentW = 4
	}
	bar := m.renderVolBar(contentW)
	return cardStyle(volCardBorderColor, totalW, totalH).
		Align(lipgloss.Left, lipgloss.Center).
		Render(bar)
}

// renderStatusCard renders the transport state + room name card.
func (m Model) renderStatusCard(totalW, totalH int) string {
	state := transportStateStyle(m.transport)
	muteStr := ""
	if m.muted && m.isLineIn {
		muteStr = " 🔇 muted"
	}
	room := ""
	if len(m.speakers) > 0 {
		room = "  " + statusRoomStyle.Render(m.speakers[m.activeSpeaker].FriendlyName)
	}
	content := state + muteStr + room
	return cardStyle(transportCardBorderColor(m.transport), totalW, totalH).
		Align(lipgloss.Left, lipgloss.Center).
		Render(content)
}

// renderProgressBar renders the progress bar to fit availW chars.
func (m Model) renderProgressBar(availW int) string {
	if m.pos.duration <= 0 {
		return ""
	}
	timeStr := progTimeStyle.Render(fmt.Sprintf(" %s / %s", formatDuration(m.pos.elapsed), formatDuration(m.pos.duration)))
	timeW := lipgloss.Width(timeStr)
	barW := availW - timeW - 1
	if barW < 4 {
		barW = 4
	}

	filled := 0
	if m.pos.duration > 0 {
		filled = barW * m.pos.elapsed / m.pos.duration
		if filled > barW {
			filled = barW
		}
	}
	empty := barW - filled

	bar := progFillStyle.Render(strings.Repeat("█", filled)) +
		progEmptyStyle.Render(strings.Repeat("░", empty))
	return bar + timeStr
}

// renderVolBar renders a volume bar like: Vol ▐███████░░░░░▌ 65%
func (m Model) renderVolBar(availW int) string {
	label := volLabelStyle.Render("Vol")
	pct := volPctStyle.Render(fmt.Sprintf(" %3d%%", m.volume))
	// Bar takes the remaining space: availW - "Vol" - " " - barChars - pct
	barW := availW - lipgloss.Width(label) - 1 - lipgloss.Width(pct) - 2 // -2 for ▐▌
	if barW < 4 {
		barW = 4
	}
	filled := barW * m.volume / 100
	if filled > barW {
		filled = barW
	}
	empty := barW - filled
	bar := "▐" + volBarFillStyle.Render(strings.Repeat("█", filled)) +
		volBarEmptyStyle.Render(strings.Repeat("░", empty)) + "▌"
	return label + " " + bar + pct
}

// helpLines returns the help bar content lines (no frame chars).
func (m Model) helpLines(innerW int) []string {
	hk := helpKeyStyle
	hd := helpDescStyle
	hs := helpSepStyle.Render("  ")

	var line1, line2 string
	if m.isLineIn {
		muteLabel := "[m] mute"
		if m.muted {
			muteLabel = "[m] unmute"
		}
		line1 = " " + hk.Render("[spc] play/pause") + hs + hd.Render(muteLabel) + hs + hd.Render("[l] line-in") + hs + hd.Render("[tab] room") + hs + hd.Render("[q] quit")
		line2 = " " + hk.Render("[j/k] vol±5") + hs + hd.Render("[J/K] vol±1") + hs + hd.Render("[r] discover")
	} else {
		line1 = " " + hk.Render("[spc] play/pause") + hs + hd.Render("[s] stop") + hs + hd.Render("[m] mute") + hs + hd.Render("[l] line-in") + hs + hd.Render("[tab] room") + hs + hd.Render("[q] quit")
		line2 = " " + hk.Render("[j/k] vol±5") + hs + hd.Render("[J/K] vol±1") + hs + hd.Render("[</>] prev/next") + hs + hd.Render("[r] discover")
	}

	lines := []string{line1, line2}

	if m.status != "" {
		lines = append(lines, " "+ephemeralStyle.Render(m.status))
	} else if m.discovering {
		lines = append(lines, " "+helpDescStyle.Render("discovering speakers…"))
	}

	return lines
}

// ── helper formatters ─────────────────────────────────────────────────────────

func formatDuration(s int) string {
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

func truncate(s string, max int) string {
	if max <= 0 || lipgloss.Width(s) <= max {
		return s
	}
	runes := []rune(s)
	for len(runes) > 0 && lipgloss.Width(string(runes)) > max-1 {
		runes = runes[:len(runes)-1]
	}
	return string(runes) + "…"
}

// ── tea.Cmd factories ─────────────────────────────────────────────────────────

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func cmdDiscoverSpeakers() tea.Cmd {
	return func() tea.Msg {
		speakers, err := sonos.DiscoverSpeakers(context.Background())
		if err != nil {
			log.Printf("discover: %v", err)
			return speakersDiscoveredMsg(nil)
		}
		return speakersDiscoveredMsg(speakers)
	}
}

func cmdStartNotifyServer(portOverride int) tea.Cmd {
	return func() tea.Msg {
		ns, err := sonos.NewNotifyServer(portOverride)
		return notifyServerStartedMsg{ns: ns, err: err}
	}
}

var serviceEventPaths = map[string]string{
	"AVTransport":       "/MediaRenderer/AVTransport/Event",
	"RenderingControl":  "/MediaRenderer/RenderingControl/Event",
	"ZoneGroupTopology": "/ZoneGroupTopology/Event",
}

func cmdSubscribe(sp sonos.Speaker, callbackURL string) tea.Cmd {
	return func() tea.Msg {
		var subs []*sonos.Subscription
		for service, path := range serviceEventPaths {
			sub, err := sonos.Subscribe(sp.IP, path, service, callbackURL)
			if err != nil {
				log.Printf("subscribe %s on %s: %v", service, sp.IP, err)
				continue
			}
			subs = append(subs, sub)
		}
		if len(subs) == 0 {
			return subscriptionStartedMsg{err: fmt.Errorf("all subscriptions failed for %s", sp.IP)}
		}
		return subscriptionStartedMsg{subs: subs}
	}
}

func cmdWaitGENAEvent(ch <-chan sonos.GENAEvent) tea.Cmd {
	return func() tea.Msg {
		return genaEventMsg(<-ch)
	}
}

func cmdGetPositionInfo(ip string) tea.Cmd {
	return func() tea.Msg {
		info, err := sonos.GetPositionInfo(ip)
		if err != nil {
			log.Printf("GetPositionInfo: %v", err)
			return nil
		}
		return positionSyncedMsg{elapsed: info.Elapsed, duration: info.Duration}
	}
}

func cmdFetchArt(url string, proto Protocol) tea.Cmd {
	return func() tea.Msg {
		rendered := FetchAndRenderArt(url, proto)
		return artFetchedMsg{url: url, data: rendered}
	}
}

func cmdToggleMute(ip string, currentlyMuted bool) tea.Cmd {
	return func() tea.Msg {
		if err := sonos.SetMute(ip, !currentlyMuted); err != nil {
			return errMsg{err}
		}
		return nil
	}
}

func cmdTogglePlayPause(ip, transport string) tea.Cmd {
	return func() tea.Msg {
		var err error
		if transport == "PLAYING" {
			err = sonos.Pause(ip)
		} else {
			err = sonos.Play(ip)
		}
		if err != nil {
			return errMsg{err}
		}
		return nil
	}
}

func cmdStop(ip string) tea.Cmd {
	return func() tea.Msg {
		if err := sonos.Stop(ip); err != nil {
			return errMsg{err}
		}
		return nil
	}
}

func cmdSetVolume(ip string, vol int) tea.Cmd {
	if vol < 0 {
		vol = 0
	}
	if vol > 100 {
		vol = 100
	}
	return func() tea.Msg {
		if err := sonos.SetVolume(ip, vol); err != nil {
			return errMsg{err}
		}
		return nil
	}
}

func cmdSwitchToLineIn(ip, uuid string) tea.Cmd {
	return func() tea.Msg {
		if err := sonos.SwitchToLineIn(ip, uuid); err != nil {
			return errMsg{err}
		}
		return nil
	}
}

func cmdPrev(ip string) tea.Cmd {
	return func() tea.Msg {
		if err := sonos.Previous(ip); err != nil {
			return errMsg{err}
		}
		return nil
	}
}

func cmdNext(ip string) tea.Cmd {
	return func() tea.Msg {
		if err := sonos.Next(ip); err != nil {
			return errMsg{err}
		}
		return nil
	}
}
