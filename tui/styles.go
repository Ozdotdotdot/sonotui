package tui

import "github.com/charmbracelet/lipgloss"

// ── Multi-zone color palette ─────────────────────────────────────────────────

// Art card — neutral gray
var artCardBorderColor = lipgloss.Color("240")

// Track info card — warm amber
var trackCardBorderColor = lipgloss.Color("172")

// Volume card — cool cyan
var volCardBorderColor = lipgloss.Color("39")

// Transport state border colors (for status card)
var transportBorderColor = map[string]lipgloss.Color{
	"PLAYING":          lipgloss.Color("82"),
	"PAUSED_PLAYBACK":  lipgloss.Color("226"),
	"STOPPED":          lipgloss.Color("240"),
	"TRANSITIONING":    lipgloss.Color("39"),
}

// ── Content styles ────────────────────────────────────────────────────────────

var (
	// Track card content
	trackTitleStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15"))
	trackArtistStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	trackAlbumStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Italic(true)

	// Volume card
	volLabelStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	volBarFillStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	volBarEmptyStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	volPctStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("248"))

	// Status card
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
	headerLeftStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15"))
	headerBrandStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("62"))
	headerRoomStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("248"))

	// Live / line-in
	liveStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("208"))

	// Ephemeral status
	ephemeralStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))

	// Help
	helpKeyStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("248"))
	helpSepStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	helpDescStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
)

// transportStateStyle returns the styled transport label string.
func transportStateStyle(state string) string {
	label, style := transportLabelAndStyle(state)
	return style.Render(label)
}

func transportLabelAndStyle(state string) (string, lipgloss.Style) {
	switch state {
	case "PLAYING":
		return "● PLAYING", statusPlayingStyle
	case "PAUSED_PLAYBACK":
		return "⏸ PAUSED", statusPausedStyle
	case "STOPPED":
		return "■ STOPPED", statusStoppedStyle
	case "TRANSITIONING":
		return "⟳ TRANSIT", statusTransStyle
	default:
		return "■ STOPPED", statusStoppedStyle
	}
}

func transportCardBorderColor(state string) lipgloss.Color {
	if c, ok := transportBorderColor[state]; ok {
		return c
	}
	return lipgloss.Color("240")
}

// ── Card helper ───────────────────────────────────────────────────────────────

// cardStyle returns a lipgloss style for a card with given total width/height.
// totalW and totalH are the full dimensions including border.
// Horizontal padding of 1 is included.
func cardStyle(borderColor lipgloss.Color, totalW, totalH int) lipgloss.Style {
	contentW := totalW - 2 - 2 // -2 border, -2 padding (1 each side)
	if contentW < 1 {
		contentW = 1
	}
	contentH := totalH - 2 // -2 border (top+bottom)
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
