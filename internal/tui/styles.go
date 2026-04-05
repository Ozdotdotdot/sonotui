package tui

import "github.com/charmbracelet/lipgloss"

// ── Color palette ─────────────────────────────────────────────────────────────

var (
	artCardBorderColor = lipgloss.Color("240")
	trackCardBorderColor = lipgloss.Color("172")
	volCardBorderColor   = lipgloss.Color("39")
)

var transportBorderColor = map[string]lipgloss.Color{
	"PLAYING":         lipgloss.Color("82"),
	"PAUSED_PLAYBACK": lipgloss.Color("226"),
	"STOPPED":         lipgloss.Color("240"),
	"TRANSITIONING":   lipgloss.Color("39"),
}

// ── Content styles ────────────────────────────────────────────────────────────

var (
	// Track card
	trackTitleStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15"))
	trackArtistStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	trackAlbumStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Italic(true)

	// Volume
	volLabelStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	volBarFillStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	volBarEmptyStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	volPctStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("248"))

	// Status / transport
	statusPlayingStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("82"))
	statusPausedStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("226"))
	statusStoppedStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("240"))
	statusTransStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	statusRoomStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("248"))

	// Progress bar
	progFillStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("62"))
	progEmptyStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	progTimeStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("248"))

	// Header
	headerBrandStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("62"))
	headerRoomStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("248"))

	// Tab bar
	tabActiveStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15")).Underline(true)
	tabInactiveStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	// Live / line-in
	liveStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("208"))

	// Ephemeral status
	ephemeralStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))

	// Help bar
	helpKeyStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("248"))
	helpSepStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	helpDescStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	// Queue tab
	queueHeaderStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("62"))
	queueCursorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("62")).Bold(true)
	queuePlayingStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
	queueNormalStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("248"))
	queueDimStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	// Library tab
	libHeaderStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("62"))
	libDirStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)
	libFileStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("248"))
	libCursorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("62")).Bold(true)
	libDimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	// Search
	searchPromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)
	searchInputStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("15"))

	// Command line
	cmdPromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("62")).Bold(true)
	cmdInputStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("15"))

	// Banner (daemon unreachable)
	bannerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))

	// Albums
	albumHeaderStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("62"))
	albumDimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	albumTitleStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15"))
	albumArtistStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	albumMetaStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Italic(true)
	albumCursorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("62")).Bold(true)
	albumNormalStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("248"))

	// Scanning indicator
	scanningStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("226")).Italic(true)

	// Help overlay
	helpOverlayBorder = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("62")).
				Padding(1, 2)
	helpSectionStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("62"))
	helpItemStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("248"))
)

// TransportStateStyle returns the styled transport label string.
func TransportStateStyle(state string) string {
	switch state {
	case "PLAYING":
		return statusPlayingStyle.Render("● PLAYING")
	case "PAUSED_PLAYBACK":
		return statusPausedStyle.Render("⏸ PAUSED")
	case "STOPPED":
		return statusStoppedStyle.Render("■ STOPPED")
	case "TRANSITIONING":
		return statusTransStyle.Render("⟳ TRANSIT")
	default:
		return statusStoppedStyle.Render("■ STOPPED")
	}
}

// TransportCardBorderColor returns the border color for the given transport state.
func TransportCardBorderColor(state string) lipgloss.Color {
	if c, ok := transportBorderColor[state]; ok {
		return c
	}
	return lipgloss.Color("240")
}

// cardStyle returns a lipgloss card style with given total dimensions.
func cardStyle(borderColor lipgloss.Color, totalW, totalH int) lipgloss.Style {
	contentW := totalW - 2 - 2
	if contentW < 1 {
		contentW = 1
	}
	contentH := totalH - 2
	if contentH < 1 {
		contentH = 1
	}
	return lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(borderColor).
		Padding(0, 1).
		Width(contentW).
		Height(contentH)
}
