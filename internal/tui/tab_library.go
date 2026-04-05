package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// LibraryModel holds the Library tab state.
type LibraryModel struct {
	CurrentPath   string
	Entries       []LibraryEntry
	Cursor        int
	Width         int
	Height        int
	Searching     bool
	SearchQuery   string
	SearchResults []LibraryEntry
	SearchCursor  int
	StatusMsg     string
}

// NewLibraryModel creates a LibraryModel.
func NewLibraryModel() LibraryModel {
	return LibraryModel{CurrentPath: "/"}
}

// SetEntries updates directory entries and clamps cursor.
func (m *LibraryModel) SetEntries(path string, entries []LibraryEntry) {
	m.CurrentPath = path
	m.Entries = entries
	if m.Cursor >= len(m.Entries) {
		m.Cursor = imax(0, len(m.Entries)-1)
	}
}

// SetSearchResults updates search results.
func (m *LibraryModel) SetSearchResults(results []LibraryEntry) {
	m.SearchResults = results
	m.SearchCursor = 0
}

// CurrentEntry returns the entry under the cursor (search-aware).
func (m *LibraryModel) CurrentEntry() *LibraryEntry {
	if m.Searching && len(m.SearchResults) > 0 {
		if m.SearchCursor < len(m.SearchResults) {
			e := m.SearchResults[m.SearchCursor]
			return &e
		}
		return nil
	}
	if m.Cursor < len(m.Entries) {
		e := m.Entries[m.Cursor]
		return &e
	}
	return nil
}

// View renders the Library tab.
func (m *LibraryModel) View() string {
	w := m.Width
	if w == 0 {
		return ""
	}
	innerW := w - 2
	var b strings.Builder

	b.WriteString("╭" + strings.Repeat("─", innerW) + "╮\n")

	pathStr := m.CurrentPath
	if lipgloss.Width(pathStr) > innerW-16 {
		pathStr = "…" + pathStr[len(pathStr)-(innerW-17):]
	}
	header := libHeaderStyle.Render(" 3:Library") + libDimStyle.Render("  "+pathStr)
	b.WriteString(frameLine(header, innerW) + "\n")
	b.WriteString("╞" + strings.Repeat("═", innerW) + "╡\n")

	listH := m.Height - 7
	if listH < 1 {
		listH = 1
	}
	if m.Searching {
		b.WriteString(m.libRenderSearchView(innerW, listH))
	} else {
		b.WriteString(m.libRenderBrowseView(innerW, listH))
	}

	b.WriteString("╞" + strings.Repeat("═", innerW) + "╡\n")
	if m.Searching {
		prompt := " " + searchPromptStyle.Render("/") + searchInputStyle.Render(m.SearchQuery)
		b.WriteString(frameLine(prompt, innerW) + "\n")
	} else {
		helpLine := " " + helpKeyStyle.Render("[a] add") + helpSepStyle.Render("  ") +
			helpDescStyle.Render("[A] add all") + helpSepStyle.Render("  ") +
			helpDescStyle.Render("[enter] open") + helpSepStyle.Render("  ") +
			helpDescStyle.Render("[/] search") + helpSepStyle.Render("  ") +
			helpDescStyle.Render("[⌫] up")
		b.WriteString(frameLine(helpLine, innerW) + "\n")
	}
	if m.StatusMsg != "" {
		b.WriteString(frameLine("  "+ephemeralStyle.Render(m.StatusMsg), innerW) + "\n")
	}

	b.WriteString("╰" + strings.Repeat("─", innerW) + "╯")
	return b.String()
}

func (m *LibraryModel) libRenderBrowseView(innerW, listH int) string {
	var b strings.Builder
	start, end := libClampRange(m.Cursor, listH, len(m.Entries))
	for i := start; i < end; i++ {
		b.WriteString(frameLine(m.libRenderEntry(i, m.Entries[i], innerW-2), innerW) + "\n")
	}
	for i := end - start; i < listH; i++ {
		b.WriteString(frameLine("", innerW) + "\n")
	}
	return b.String()
}

func (m *LibraryModel) libRenderSearchView(innerW, listH int) string {
	var b strings.Builder
	results := m.SearchResults
	if len(results) == 0 {
		b.WriteString(frameLine("  "+libDimStyle.Render("no results"), innerW) + "\n")
		for i := 1; i < listH; i++ {
			b.WriteString(frameLine("", innerW) + "\n")
		}
		return b.String()
	}
	start, end := libClampRange(m.SearchCursor, listH, len(results))
	for i := start; i < end; i++ {
		b.WriteString(frameLine(m.libRenderSearchResult(i, results[i], innerW-2), innerW) + "\n")
	}
	for i := end - start; i < listH; i++ {
		b.WriteString(frameLine("", innerW) + "\n")
	}
	return b.String()
}

func (m *LibraryModel) libRenderEntry(i int, e LibraryEntry, availW int) string {
	cursor := "  "
	if i == m.Cursor {
		cursor = libCursorStyle.Render("→ ")
	}
	if e.Type == "dir" {
		icon := libDirStyle.Render("📁 ")
		name := libDirStyle.Render(truncate(e.Name, availW-8))
		return cursor + icon + name
	}
	icon := libFileStyle.Render("🎵 ")
	dur := ""
	if e.Duration > 0 {
		dur = libDimStyle.Render(fmt.Sprintf("%d:%02d", e.Duration/60, e.Duration%60))
	}
	durW := lipgloss.Width(dur)
	nameW := availW - lipgloss.Width(cursor) - 4 - durW - 2
	if nameW < 4 {
		nameW = 4
	}
	nameStr := truncate(e.Name, nameW)
	namePad := imax(0, nameW-lipgloss.Width(nameStr))
	return cursor + icon + libFileStyle.Render(nameStr) + strings.Repeat(" ", namePad) + "  " + dur
}

func (m *LibraryModel) libRenderSearchResult(i int, e LibraryEntry, availW int) string {
	cursor := "  "
	if i == m.SearchCursor {
		cursor = libCursorStyle.Render("→ ")
	}
	title := e.Title
	if title == "" {
		title = e.Name
	}
	artist := libDimStyle.Render(truncate(e.Artist, availW/3))
	titleStr := libFileStyle.Render(truncate(title, availW/2))
	path := libDimStyle.Render(truncate(e.Path, availW/4))
	return cursor + "🎵 " + titleStr + " · " + artist + " " + path
}

// CursorUp moves cursor up.
func (m *LibraryModel) CursorUp() {
	if m.Searching {
		if m.SearchCursor > 0 {
			m.SearchCursor--
		}
		return
	}
	if m.Cursor > 0 {
		m.Cursor--
	}
}

// CursorDown moves cursor down.
func (m *LibraryModel) CursorDown() {
	if m.Searching {
		if m.SearchCursor < len(m.SearchResults)-1 {
			m.SearchCursor++
		}
		return
	}
	if m.Cursor < len(m.Entries)-1 {
		m.Cursor++
	}
}

// HalfPageUp moves up half a page.
func (m *LibraryModel) HalfPageUp() {
	pg := m.libPageSize() / 2
	if m.Searching {
		m.SearchCursor -= pg
		if m.SearchCursor < 0 {
			m.SearchCursor = 0
		}
		return
	}
	m.Cursor -= pg
	if m.Cursor < 0 {
		m.Cursor = 0
	}
}

// HalfPageDown moves down half a page.
func (m *LibraryModel) HalfPageDown() {
	pg := m.libPageSize() / 2
	if m.Searching {
		m.SearchCursor += pg
		if m.SearchCursor >= len(m.SearchResults) {
			m.SearchCursor = imax(0, len(m.SearchResults)-1)
		}
		return
	}
	m.Cursor += pg
	if m.Cursor >= len(m.Entries) {
		m.Cursor = imax(0, len(m.Entries)-1)
	}
}

// JumpTop jumps to first entry.
func (m *LibraryModel) JumpTop() {
	if m.Searching {
		m.SearchCursor = 0
		return
	}
	m.Cursor = 0
}

// JumpBottom jumps to last entry.
func (m *LibraryModel) JumpBottom() {
	if m.Searching {
		m.SearchCursor = imax(0, len(m.SearchResults)-1)
		return
	}
	m.Cursor = imax(0, len(m.Entries)-1)
}

func (m *LibraryModel) libPageSize() int {
	h := m.Height - 7
	if h < 2 {
		return 2
	}
	return h
}

// ParentPath returns the parent of the current path.
func (m *LibraryModel) ParentPath() string {
	p := strings.TrimSuffix(m.CurrentPath, "/")
	if p == "" {
		return "/"
	}
	idx := strings.LastIndex(p, "/")
	if idx <= 0 {
		return "/"
	}
	return p[:idx]
}

func libClampRange(cursor, h, total int) (int, int) {
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
