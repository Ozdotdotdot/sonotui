package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// NowPlayingState holds data for the Now Playing tab view.
type NowPlayingState struct {
	Transport   string
	Track       TrackInfo
	Volume      int
	Elapsed     int
	Duration    int
	IsLineIn    bool
	ArtRendered string
	SpeakerName string
	StatusMsg   string
}

// NowPlayingView renders the Now Playing tab.
func NowPlayingView(s NowPlayingState, width, height int) string {
	innerW := width - 2
	var b strings.Builder

	b.WriteString("╭" + strings.Repeat("─", innerW) + "╮\n")
	b.WriteString(frameLine(npHeaderContent(s, innerW), innerW) + "\n")
	b.WriteString("╞" + strings.Repeat("═", innerW) + "╡\n")

	contentW := innerW - 2
	var contentLines []string
	if width >= 80 {
		contentLines = npWideContent(s, contentW)
	} else {
		contentLines = npNarrowContent(s, contentW)
	}
	b.WriteString(frameLine("", innerW) + "\n")
	for _, cl := range contentLines {
		b.WriteString(frameLine(" "+cl, innerW) + "\n")
	}
	b.WriteString(frameLine("", innerW) + "\n")

	b.WriteString("╞" + strings.Repeat("═", innerW) + "╡\n")

	for _, hl := range npHelpLines(s, innerW) {
		b.WriteString(frameLine(hl, innerW) + "\n")
	}

	b.WriteString("╰" + strings.Repeat("─", innerW) + "╯")
	return b.String()
}

func frameLine(content string, innerW int) string {
	visW := lipgloss.Width(content)
	pad := innerW - visW
	if pad < 0 {
		pad = 0
	}
	return "│" + content + strings.Repeat(" ", pad) + "│"
}

func npHeaderContent(s NowPlayingState, innerW int) string {
	brand := headerBrandStyle.Render(" sonotui")
	sep := headerRoomStyle.Render(" › ")
	room := s.SpeakerName
	if room == "" {
		room = "no speaker"
	}
	left := brand + sep + headerRoomStyle.Render(room)
	state := TransportStateStyle(s.Transport)
	vol := volLabelStyle.Render(fmt.Sprintf("vol:%d", s.Volume))
	right := state + "   " + vol + " "
	gap := innerW - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

func npWideContent(s NowPlayingState, availW int) []string {
	const artCardW = 22
	const rowGap = 2
	trackCardW := availW - artCardW - rowGap
	if trackCardW < 20 {
		trackCardW = 20
	}
	const cardH = 9
	artCard := npArtCard(s, artCardW, cardH)
	trackCard := npTrackCard(s, trackCardW, cardH)
	row1 := lipgloss.JoinHorizontal(lipgloss.Top, artCard, strings.Repeat(" ", rowGap), trackCard)

	volCardW := availW * 2 / 5
	statusCardW := availW - volCardW - rowGap
	const bottomH = 4
	volCard := npVolCard(s, volCardW, bottomH)
	statusCard := npStatusCard(s, statusCardW, bottomH)
	row2 := lipgloss.JoinHorizontal(lipgloss.Top, volCard, strings.Repeat(" ", rowGap), statusCard)

	var lines []string
	lines = append(lines, strings.Split(row1, "\n")...)
	lines = append(lines, "")
	lines = append(lines, strings.Split(row2, "\n")...)
	return lines
}

func npNarrowContent(s NowPlayingState, availW int) []string {
	artCard := npArtCard(s, availW, 7)
	trackCard := npTrackCard(s, availW, 9)
	volCard := npVolCard(s, availW/2-1, 4)
	statusCard := npStatusCard(s, availW-availW/2-1, 4)
	bottomRow := lipgloss.JoinHorizontal(lipgloss.Top, volCard, " ", statusCard)
	var lines []string
	lines = append(lines, strings.Split(artCard, "\n")...)
	lines = append(lines, "")
	lines = append(lines, strings.Split(trackCard, "\n")...)
	lines = append(lines, "")
	lines = append(lines, strings.Split(bottomRow, "\n")...)
	return lines
}

func npArtCard(s NowPlayingState, totalW, totalH int) string {
	art := s.ArtRendered
	if art == "" {
		art = ArtPlaceholder
	}
	return npCardStyle(artCardBorderColor, totalW, totalH).
		Align(lipgloss.Center, lipgloss.Center).
		Render(art)
}

func npTrackCard(s NowPlayingState, totalW, totalH int) string {
	contentW := totalW - 4
	if contentW < 4 {
		contentW = 4
	}
	var lines []string
	if s.IsLineIn {
		lines = []string{
			trackTitleStyle.Render("Line-In"),
			trackArtistStyle.Render("analog source"),
			"",
			liveStyle.Render("── LIVE ──"),
		}
	} else {
		title := trackTitleStyle.Render(truncate(s.Track.Title, contentW))
		artist := trackArtistStyle.Render(truncate(s.Track.Artist, contentW))
		album := trackAlbumStyle.Render(truncate(s.Track.Album, contentW))
		progress := npProgressBar(s, contentW)
		lines = []string{title, artist, album, "", progress}
	}
	return npCardStyle(trackCardBorderColor, totalW, totalH).Render(strings.Join(lines, "\n"))
}

func npVolCard(s NowPlayingState, totalW, totalH int) string {
	contentW := totalW - 4
	if contentW < 4 {
		contentW = 4
	}
	bar := npVolBar(s.Volume, contentW)
	return npCardStyle(volCardBorderColor, totalW, totalH).
		Align(lipgloss.Left, lipgloss.Center).
		Render(bar)
}

func npStatusCard(s NowPlayingState, totalW, totalH int) string {
	state := TransportStateStyle(s.Transport)
	room := ""
	if s.SpeakerName != "" {
		room = "  " + statusRoomStyle.Render(s.SpeakerName)
	}
	return npCardStyle(TransportCardBorderColor(s.Transport), totalW, totalH).
		Align(lipgloss.Left, lipgloss.Center).
		Render(state + room)
}

func npProgressBar(s NowPlayingState, availW int) string {
	if s.Duration <= 0 {
		return ""
	}
	timeStr := progTimeStyle.Render(fmt.Sprintf(" %s / %s",
		formatDuration(s.Elapsed), formatDuration(s.Duration)))
	timeW := lipgloss.Width(timeStr)
	barW := availW - timeW - 1
	if barW < 4 {
		barW = 4
	}
	filled := 0
	if s.Duration > 0 {
		filled = barW * s.Elapsed / s.Duration
		if filled > barW {
			filled = barW
		}
	}
	empty := barW - filled
	bar := progFillStyle.Render(strings.Repeat("█", filled)) +
		progEmptyStyle.Render(strings.Repeat("░", empty))
	return bar + timeStr
}

func npVolBar(volume, availW int) string {
	label := volLabelStyle.Render("Vol")
	pct := volPctStyle.Render(fmt.Sprintf(" %3d%%", volume))
	barW := availW - lipgloss.Width(label) - 1 - lipgloss.Width(pct) - 2
	if barW < 4 {
		barW = 4
	}
	filled := barW * volume / 100
	if filled > barW {
		filled = barW
	}
	empty := barW - filled
	bar := "▐" + volBarFillStyle.Render(strings.Repeat("█", filled)) +
		volBarEmptyStyle.Render(strings.Repeat("░", empty)) + "▌"
	return label + " " + bar + pct
}

func npHelpLines(s NowPlayingState, innerW int) []string {
	hk := helpKeyStyle
	hd := helpDescStyle
	hs := helpSepStyle.Render("  ")
	line1 := " " + hk.Render("[spc] play/pause") + hs + hd.Render("[s] stop") + hs +
		hd.Render("[l] line-in") + hs + hd.Render("[tab] room") + hs + hd.Render("[q] quit")
	line2 := " " + hk.Render("[j/k] vol±5") + hs + hd.Render("[J/K] vol±1") + hs +
		hd.Render("[</>] prev/next") + hs + hd.Render("[r] discover")
	lines := []string{line1, line2}
	if s.StatusMsg != "" {
		lines = append(lines, " "+ephemeralStyle.Render(s.StatusMsg))
	}
	return lines
}

func npCardStyle(borderColor lipgloss.Color, totalW, totalH int) lipgloss.Style {
	contentW := totalW - 4
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
