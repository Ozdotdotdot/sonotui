package tui

import "github.com/charmbracelet/lipgloss"

var (
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15"))
	artistStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("250"))
	albumStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Italic(true)
	statusPlaying = lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
	statusPaused  = lipgloss.NewStyle().Foreground(lipgloss.Color("226"))
	statusStopped = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	statusTrans   = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	volStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	helpStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	liveStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("208")).Bold(true)
	progressFill  = lipgloss.NewStyle().Foreground(lipgloss.Color("62"))
	progressEmpty = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	ephemeralMsg  = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	headerStyle   = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15")).
			Background(lipgloss.Color("237"))
	borderStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62"))
)

// transportStyle returns the style for the given transport state string.
func transportStyle(state string) lipgloss.Style {
	switch state {
	case "PLAYING":
		return statusPlaying
	case "PAUSED_PLAYBACK":
		return statusPaused
	case "STOPPED":
		return statusStopped
	case "TRANSITIONING":
		return statusTrans
	default:
		return statusStopped
	}
}

// transportLabel returns the display symbol + label for a transport state.
func transportLabel(state string) string {
	switch state {
	case "PLAYING":
		return "● PLAYING"
	case "PAUSED_PLAYBACK":
		return "⏸ PAUSED"
	case "STOPPED":
		return "■ STOPPED"
	case "TRANSITIONING":
		return "⟳ TRANSITIONING"
	default:
		return "■ STOPPED"
	}
}
