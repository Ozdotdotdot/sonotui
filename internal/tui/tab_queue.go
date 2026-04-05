package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// QueueModel holds the Queue tab state.
type QueueModel struct {
	Items        []QueueItem
	Cursor       int
	PlayingPos   int // 1-based position of currently playing track
	Width        int
	Height       int
	ddPending    bool
	ConfirmClear bool
}

// NewQueueModel creates a QueueModel.
func NewQueueModel() QueueModel { return QueueModel{} }

// SetItems updates queue items and clamps cursor.
func (m *QueueModel) SetItems(items []QueueItem) {
	m.Items = items
	if m.Cursor >= len(m.Items) {
		m.Cursor = imax(0, len(m.Items)-1)
	}
}

// View renders the Queue tab.
func (m *QueueModel) View() string {
	w := m.Width
	if w == 0 {
		return ""
	}
	innerW := w - 2
	var b strings.Builder

	b.WriteString("╭" + strings.Repeat("─", innerW) + "╮\n")

	countStr := fmt.Sprintf(" %d tracks ", len(m.Items))
	header := queueHeaderStyle.Render(" 2:Queue") +
		strings.Repeat(" ", imax(1, innerW-lipgloss.Width(" 2:Queue")-lipgloss.Width(countStr))) +
		queueDimStyle.Render(countStr)
	b.WriteString(frameLine(header, innerW) + "\n")
	b.WriteString("╞" + strings.Repeat("═", innerW) + "╡\n")

	listH := m.Height - 7
	if listH < 1 {
		listH = 1
	}
	start, end := queueVisibleRange(m.Cursor, listH, len(m.Items))
	for i := start; i < end; i++ {
		b.WriteString(frameLine(m.renderItem(i, innerW-2), innerW) + "\n")
	}
	for i := end - start; i < listH; i++ {
		b.WriteString(frameLine("", innerW) + "\n")
	}

	b.WriteString("╞" + strings.Repeat("═", innerW) + "╡\n")

	helpLine := " " + helpKeyStyle.Render("[p] play") + helpSepStyle.Render("  ") +
		helpDescStyle.Render("[dd] delete") + helpSepStyle.Render("  ") +
		helpDescStyle.Render("[D] clear") + helpSepStyle.Render("  ") +
		helpDescStyle.Render("[J/K] reorder")
	b.WriteString(frameLine(helpLine, innerW) + "\n")

	if m.ConfirmClear {
		confirm := "  " + ephemeralStyle.Render("Clear entire queue? [y]es / [n]o")
		b.WriteString(frameLine(confirm, innerW) + "\n")
	}

	b.WriteString("╰" + strings.Repeat("─", innerW) + "╯")
	return b.String()
}

func queueVisibleRange(cursor, h, total int) (int, int) {
	if total == 0 {
		return 0, 0
	}
	start := cursor - h/2
	if start < 0 {
		start = 0
	}
	end := start + h
	if end > total {
		end = total
		start = imax(0, end-h)
	}
	return start, end
}

func (m *QueueModel) renderItem(i, availW int) string {
	if i >= len(m.Items) {
		return ""
	}
	item := m.Items[i]

	playIcon := "  "
	if item.Position == m.PlayingPos {
		playIcon = queuePlayingStyle.Render("▶ ")
	}
	cursor := "  "
	if i == m.Cursor {
		cursor = queueCursorStyle.Render("→ ")
	}
	posStr := queueDimStyle.Render(fmt.Sprintf("%2d  ", item.Position))

	dur := ""
	if item.Duration > 0 {
		dur = queueDimStyle.Render(fmt.Sprintf("%d:%02d", item.Duration/60, item.Duration%60))
	}
	durW := lipgloss.Width(dur)
	used := lipgloss.Width(cursor) + lipgloss.Width(playIcon) + lipgloss.Width(posStr) + durW + 2
	remaining := availW - used
	if remaining < 10 {
		remaining = 10
	}
	titleW := remaining * 55 / 100
	artistW := remaining - titleW - 1

	var titleStyle, artistStyle lipgloss.Style
	if i == m.Cursor {
		titleStyle = queueNormalStyle.Bold(true)
		artistStyle = queueNormalStyle
	} else {
		titleStyle = queueNormalStyle
		artistStyle = queueDimStyle
	}

	tStr := truncate(item.Title, titleW)
	aStr := truncate(item.Artist, artistW)
	titlePad := imax(0, titleW-lipgloss.Width(tStr))
	artistPad := imax(0, artistW-lipgloss.Width(aStr))

	return cursor + playIcon + posStr +
		titleStyle.Render(tStr) + strings.Repeat(" ", titlePad) + " " +
		artistStyle.Render(aStr) + strings.Repeat(" ", artistPad) + "  " + dur
}

// CursorUp moves cursor up.
func (m *QueueModel) CursorUp() {
	if m.Cursor > 0 {
		m.Cursor--
	}
	m.ddPending = false
}

// CursorDown moves cursor down.
func (m *QueueModel) CursorDown() {
	if m.Cursor < len(m.Items)-1 {
		m.Cursor++
	}
	m.ddPending = false
}

// HalfPageUp moves cursor half a page up.
func (m *QueueModel) HalfPageUp() {
	m.Cursor -= m.queuePageSize() / 2
	if m.Cursor < 0 {
		m.Cursor = 0
	}
	m.ddPending = false
}

// HalfPageDown moves cursor half a page down.
func (m *QueueModel) HalfPageDown() {
	m.Cursor += m.queuePageSize() / 2
	if m.Cursor >= len(m.Items) {
		m.Cursor = imax(0, len(m.Items)-1)
	}
	m.ddPending = false
}

// JumpTop jumps to the first item.
func (m *QueueModel) JumpTop() { m.Cursor = 0; m.ddPending = false }

// JumpBottom jumps to the last item.
func (m *QueueModel) JumpBottom() { m.Cursor = imax(0, len(m.Items)-1); m.ddPending = false }

func (m *QueueModel) queuePageSize() int {
	h := m.Height - 7
	if h < 2 {
		return 2
	}
	return h
}

// CursorPosition returns the 1-based queue position of the cursor item.
func (m *QueueModel) CursorPosition() int {
	if m.Cursor >= 0 && m.Cursor < len(m.Items) {
		return m.Items[m.Cursor].Position
	}
	return 0
}

// HandleD processes 'd' for dd-delete. Returns true when the second 'd' is pressed.
func (m *QueueModel) HandleD() bool {
	if m.ddPending {
		m.ddPending = false
		return true
	}
	m.ddPending = true
	return false
}

// CancelDD clears pending dd state.
func (m *QueueModel) CancelDD() { m.ddPending = false }

func imax(a, b int) int {
	if a > b {
		return a
	}
	return b
}
