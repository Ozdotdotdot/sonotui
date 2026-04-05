package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// AlbumModel holds the Albums tab state.
type AlbumModel struct {
	Albums       []Album
	Cursor       int
	Expanded     bool
	ExpandedID   string
	ExpandTracks []LibraryEntry
	TrackCursor  int
	Width        int
	Height       int

	Searching    bool
	SearchQuery  string
	SearchResult []Album

	LibraryReady bool
	ScanProgress float64
	ScanStatus   string

	StatusMsg string
}

// NewAlbumModel creates an AlbumModel.
func NewAlbumModel() AlbumModel { return AlbumModel{} }

// SetAlbums updates the album list.
func (m *AlbumModel) SetAlbums(albums []Album) {
	m.Albums = albums
	if m.Cursor >= len(m.Albums) {
		m.Cursor = imax(0, len(m.Albums)-1)
	}
}

// VisibleAlbums returns Albums or SearchResult depending on search mode.
func (m *AlbumModel) VisibleAlbums() []Album {
	if m.Searching && len(m.SearchResult) > 0 {
		return m.SearchResult
	}
	return m.Albums
}

// CurrentAlbum returns the album under the cursor.
func (m *AlbumModel) CurrentAlbum() *Album {
	albums := m.VisibleAlbums()
	if m.Cursor < len(albums) {
		a := albums[m.Cursor]
		return &a
	}
	return nil
}

// SetExpandTracks sets the track list for the expanded album.
func (m *AlbumModel) SetExpandTracks(tracks []LibraryEntry) {
	m.ExpandTracks = tracks
	m.TrackCursor = 0
}

// CurrentTrackPaths returns all track paths for the expanded album.
func (m *AlbumModel) CurrentTrackPaths() []string {
	paths := make([]string, 0, len(m.ExpandTracks))
	for _, t := range m.ExpandTracks {
		paths = append(paths, t.Path)
	}
	return paths
}

// ExpandCurrent marks the current album as expanded.
func (m *AlbumModel) ExpandCurrent() {
	if al := m.CurrentAlbum(); al != nil {
		m.Expanded = true
		m.ExpandedID = al.ID
		m.TrackCursor = 0
	}
}

// Collapse collapses the expanded album.
func (m *AlbumModel) Collapse() {
	m.Expanded = false
	m.ExpandedID = ""
	m.ExpandTracks = nil
	m.TrackCursor = 0
}

// View renders the Albums tab.
func (m *AlbumModel) View() string {
	w := m.Width
	if w == 0 {
		return ""
	}
	innerW := w - 2
	var b strings.Builder

	b.WriteString("╭" + strings.Repeat("─", innerW) + "╮\n")

	albums := m.VisibleAlbums()
	countStr := fmt.Sprintf(" %d albums ", len(albums))
	header := albumHeaderStyle.Render(" 4:Albums") +
		strings.Repeat(" ", imax(1, innerW-lipgloss.Width(" 4:Albums")-lipgloss.Width(countStr))) +
		albumDimStyle.Render(countStr)
	b.WriteString(frameLine(header, innerW) + "\n")
	b.WriteString("╞" + strings.Repeat("═", innerW) + "╡\n")

	if !m.LibraryReady {
		b.WriteString(m.alRenderLoading(innerW))
	} else if m.Expanded {
		b.WriteString(m.alRenderExpanded(innerW))
	} else {
		b.WriteString(m.alRenderList(innerW))
	}

	b.WriteString("╞" + strings.Repeat("═", innerW) + "╡\n")
	if m.Searching {
		prompt := " " + searchPromptStyle.Render("/") + searchInputStyle.Render(m.SearchQuery)
		b.WriteString(frameLine(prompt, innerW) + "\n")
	} else {
		helpLine := " " + helpKeyStyle.Render("[a] add album") + helpSepStyle.Render("  ") +
			helpDescStyle.Render("[enter] expand") + helpSepStyle.Render("  ") +
			helpDescStyle.Render("[/] search") + helpSepStyle.Render("  ") +
			helpDescStyle.Render("[r] rescan")
		b.WriteString(frameLine(helpLine, innerW) + "\n")
	}
	if m.StatusMsg != "" {
		b.WriteString(frameLine("  "+ephemeralStyle.Render(m.StatusMsg), innerW) + "\n")
	}

	b.WriteString("╰" + strings.Repeat("─", innerW) + "╯")
	return b.String()
}

func (m *AlbumModel) alRenderLoading(innerW int) string {
	listH := m.Height - 7
	if listH < 1 {
		listH = 1
	}
	var b strings.Builder
	msg := "  " + scanningStyle.Render("Scanning library…")
	if m.ScanStatus == "scanning" && m.ScanProgress > 0 {
		msg = fmt.Sprintf("  "+scanningStyle.Render("Scanning library… %.0f%%"), m.ScanProgress*100)
	}
	b.WriteString(frameLine(msg, innerW) + "\n")
	for i := 1; i < listH; i++ {
		b.WriteString(frameLine("", innerW) + "\n")
	}
	return b.String()
}

func (m *AlbumModel) alRenderList(innerW int) string {
	albums := m.VisibleAlbums()
	listH := m.Height - 7
	if listH < 1 {
		listH = 1
	}
	var b strings.Builder
	start, end := libClampRange(m.Cursor, listH, len(albums))
	for i := start; i < end; i++ {
		b.WriteString(frameLine(m.alRenderRow(i, albums[i], innerW-2), innerW) + "\n")
	}
	for i := end - start; i < listH; i++ {
		b.WriteString(frameLine("", innerW) + "\n")
	}
	return b.String()
}

func (m *AlbumModel) alRenderExpanded(innerW int) string {
	listH := m.Height - 7
	if listH < 1 {
		listH = 1
	}
	leftW := (innerW - 1) / 3
	rightW := innerW - leftW - 1

	albums := m.VisibleAlbums()
	var leftLines, rightLines []string
	startA, endA := libClampRange(m.Cursor, listH, len(albums))
	for i := startA; i < endA; i++ {
		leftLines = append(leftLines, m.alRenderRow(i, albums[i], leftW-2))
	}
	for len(leftLines) < listH {
		leftLines = append(leftLines, "")
	}

	if al := m.CurrentAlbum(); al != nil {
		rightLines = append(rightLines, albumTitleStyle.Render(truncate(al.Title, rightW-2)))
		meta := al.Artist
		if al.Year > 0 {
			meta += fmt.Sprintf("  ·  %d", al.Year)
		}
		meta += fmt.Sprintf("  ·  %d tracks", al.TrackCount)
		rightLines = append(rightLines, albumArtistStyle.Render(truncate(meta, rightW-2)))
		sep := imin(rightW-4, 40)
		rightLines = append(rightLines, albumDimStyle.Render(strings.Repeat("─", sep)))
		rightLines = append(rightLines, "")

		startT, endT := libClampRange(m.TrackCursor, listH-4, len(m.ExpandTracks))
		for i := startT; i < endT; i++ {
			t := m.ExpandTracks[i]
			cur := "   "
			if i == m.TrackCursor {
				cur = albumCursorStyle.Render("→  ")
			}
			dur := ""
			if t.Duration > 0 {
				dur = albumDimStyle.Render(fmt.Sprintf("%d:%02d", t.Duration/60, t.Duration%60))
			}
			num := albumDimStyle.Render(fmt.Sprintf("%2d  ", i+1))
			title := truncate(t.Title, rightW-14)
			rightLines = append(rightLines, cur+num+title+"  "+dur)
		}
	}
	for len(rightLines) < listH {
		rightLines = append(rightLines, "")
	}

	var b strings.Builder
	for i := 0; i < listH; i++ {
		lCell := ""
		if i < len(leftLines) {
			lCell = leftLines[i]
		}
		rCell := ""
		if i < len(rightLines) {
			rCell = rightLines[i]
		}
		lPad := imax(0, leftW-lipgloss.Width(lCell))
		b.WriteString(frameLine(lCell+strings.Repeat(" ", lPad)+"│"+rCell, innerW) + "\n")
	}
	return b.String()
}

func (m *AlbumModel) alRenderRow(i int, al Album, availW int) string {
	cursor := "  "
	if i == m.Cursor {
		cursor = albumCursorStyle.Render("→ ")
	}
	year := ""
	if al.Year > 0 {
		year = albumDimStyle.Render(fmt.Sprintf(" %d", al.Year))
	}
	yearW := lipgloss.Width(year)
	cW := lipgloss.Width(cursor)
	titleW := (availW - cW - yearW) * 55 / 100
	artistW := availW - cW - yearW - titleW - 2
	if artistW < 4 {
		artistW = 4
	}

	var titleSt, artistSt lipgloss.Style
	if i == m.Cursor {
		titleSt = albumTitleStyle
		artistSt = albumArtistStyle
	} else {
		titleSt = albumNormalStyle
		artistSt = albumDimStyle
	}

	tStr := truncate(al.Title, titleW)
	aStr := truncate(al.Artist, artistW)
	tPad := imax(0, titleW-lipgloss.Width(tStr))
	aPad := imax(0, artistW-lipgloss.Width(aStr))

	return cursor + titleSt.Render(tStr) + strings.Repeat(" ", tPad) + " " +
		artistSt.Render(aStr) + strings.Repeat(" ", aPad) + year
}

// Cursor movement.
func (m *AlbumModel) CursorUp()        { albums := m.VisibleAlbums(); _ = albums; if m.Cursor > 0 { m.Cursor-- } }
func (m *AlbumModel) CursorDown()      { albums := m.VisibleAlbums(); if m.Cursor < len(albums)-1 { m.Cursor++ } }
func (m *AlbumModel) TrackCursorUp()   { if m.TrackCursor > 0 { m.TrackCursor-- } }
func (m *AlbumModel) TrackCursorDown() { if m.TrackCursor < len(m.ExpandTracks)-1 { m.TrackCursor++ } }
func (m *AlbumModel) JumpTop()         { m.Cursor = 0 }
func (m *AlbumModel) JumpBottom()      { m.Cursor = imax(0, len(m.VisibleAlbums())-1) }

func imin(a, b int) int {
	if a < b {
		return a
	}
	return b
}
