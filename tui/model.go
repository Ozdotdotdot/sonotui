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
		if m.isLineIn {
			// In line-in mode, space toggles mute.
			return m, cmdToggleMute(sp.IP, m.muted)
		}
		return m, cmdTogglePlayPause(sp.IP, m.transport)

	case key.Matches(msg, keys.Stop):
		if m.isLineIn {
			m.setStatus("Stop not available in Line-In mode")
			return m, nil
		}
		return m, cmdStop(sp.IP)

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

	wide := m.width >= 80

	// Header line.
	header := m.renderHeader()

	// Main content.
	var content string
	if wide {
		content = m.renderWide()
	} else {
		content = m.renderNarrow()
	}

	// Help + status.
	help := m.renderHelp()

	return lipgloss.JoinVertical(lipgloss.Left, header, content, help)
}

func (m Model) renderHeader() string {
	left := " sonotui"

	roomName := ""
	if len(m.speakers) > 0 {
		roomName = m.speakers[m.activeSpeaker].FriendlyName
	} else if m.discovering {
		roomName = "discovering…"
	}

	stateLabel := transportStyle(m.transport).Render(transportLabel(m.transport))
	vol := volStyle.Render(fmt.Sprintf("vol:%d", m.volume))

	right := fmt.Sprintf("%s   %s   %s ", roomName, stateLabel, vol)

	pad := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if pad < 1 {
		pad = 1
	}

	return headerStyle.
		Width(m.width).
		Render(left + strings.Repeat(" ", pad) + right)
}

func (m Model) renderWide() string {
	artWidth := 16
	metaWidth := m.width - artWidth - 1

	artPane := m.renderArtPane(artWidth)
	metaPane := m.renderMetaPane(metaWidth)

	return lipgloss.JoinHorizontal(lipgloss.Top, artPane, metaPane)
}

func (m Model) renderNarrow() string {
	return m.renderMetaPane(m.width)
}

func (m Model) renderArtPane(w int) string {
	art := m.artRendered
	if art == "" {
		art = artPlaceholder
	}
	return lipgloss.NewStyle().
		Width(w).
		Height(6).
		Padding(1, 1).
		Render(art)
}

func (m Model) renderMetaPane(w int) string {
	var lines []string

	if m.isLineIn {
		lines = []string{
			titleStyle.Render("Line-In"),
			artistStyle.Render("analog source"),
			"",
			liveStyle.Render("── live ──"),
		}
	} else {
		lines = []string{
			titleStyle.Render(truncate(m.trackInfo.Title, w-4)),
			artistStyle.Render(truncate(m.trackInfo.Artist, w-4)),
			albumStyle.Render(truncate(m.trackInfo.Album, w-4)),
			"",
			m.renderProgress(w - 4),
		}
	}

	return lipgloss.NewStyle().
		Width(w).
		Padding(1, 2).
		Render(strings.Join(lines, "\n"))
}

func (m Model) renderProgress(availWidth int) string {
	if m.pos.duration <= 0 || m.isLineIn {
		return ""
	}

	timeStr := fmt.Sprintf("  %s / %s", formatDuration(m.pos.elapsed), formatDuration(m.pos.duration))
	barWidth := availWidth - lipgloss.Width(timeStr) - 2
	if barWidth < 4 {
		barWidth = 4
	}

	filled := 0
	if m.pos.duration > 0 {
		filled = barWidth * m.pos.elapsed / m.pos.duration
		if filled > barWidth {
			filled = barWidth
		}
	}
	empty := barWidth - filled

	bar := progressFill.Render(strings.Repeat("█", filled)) +
		progressEmpty.Render(strings.Repeat("░", empty))

	return bar + timeStr
}

func (m Model) renderHelp() string {
	lines := []string{
		helpStyle.Render("[spc] play/pause  [s] stop  [l] line-in  [tab] room"),
		helpStyle.Render("[j/k] vol  [</>] prev/next  [r] discover  [q] quit"),
	}
	if m.status != "" {
		lines = append(lines, ephemeralMsg.Render(m.status))
	}
	if m.discovering {
		lines = append(lines, helpStyle.Render("  discovering speakers…"))
	}
	return " " + strings.Join(lines, "\n ")
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
