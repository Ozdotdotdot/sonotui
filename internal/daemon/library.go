package daemon

import (
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dhowden/tag"
	"github.com/sahilm/fuzzy"
)

// Track is a single audio file in the library index.
type Track struct {
	Path        string `json:"path"` // relative to musicRoot
	Title       string `json:"title"`
	Artist      string `json:"artist"`
	Album       string `json:"album"`
	AlbumArtist string `json:"album_artist"`
	Year        int    `json:"year"`
	TrackNum    int    `json:"track_num"`
	Duration    int    `json:"duration"` // seconds; 0 if unavailable
	ArtHash     string `json:"art_hash"`
	ModTimeUnix int64  `json:"mtime,omitempty"` // file mtime in unix nanos; used for incremental scan
	Size        int64  `json:"size,omitempty"`  // file size in bytes; used for incremental scan
}

// Album is a group of tracks sharing album+albumartist.
type Album struct {
	ID         string  `json:"id"` // SHA1 of (AlbumArtist + Album)
	Title      string  `json:"title"`
	Artist     string  `json:"artist"` // AlbumArtist, fallback Artist
	Year       int     `json:"year"`
	Tracks     []Track `json:"tracks"`
	ArtHash    string  `json:"art_hash"`
	Path       string  `json:"path"` // common directory prefix
	TrackCount int     `json:"track_count"`
}

// LibraryEntry is a directory or file entry returned by the Browse API.
type LibraryEntry struct {
	Name     string `json:"name"`
	Type     string `json:"type"` // "dir" | "file"
	Path     string `json:"path"`
	Title    string `json:"title,omitempty"`
	Artist   string `json:"artist,omitempty"`
	Album    string `json:"album,omitempty"`
	Duration int    `json:"duration,omitempty"`
	ArtHash  string `json:"art_hash,omitempty"`
}

// Library holds the full music index and art cache.
type Library struct {
	mu        sync.RWMutex
	musicRoot string
	cachePath string
	tracks    []Track
	albums    []Album
	art       map[string][]byte // hash → raw JPEG bytes
	ready     bool
	scanning  bool
}

var (
	ffprobePath string
	ffprobeOnce sync.Once
)

// NewLibrary creates a Library.
func NewLibrary(musicRoot, cachePath string) *Library {
	return &Library{
		musicRoot: musicRoot,
		cachePath: cachePath,
		art:       make(map[string][]byte),
	}
}

// Ready returns true when the library has been fully scanned.
func (l *Library) Ready() bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.ready
}

// GetArt returns the raw art bytes for a given hash, or nil.
func (l *Library) GetArt(hash string) []byte {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.art[hash]
}

// Tracks returns a copy of the track list.
func (l *Library) Tracks() []Track {
	l.mu.RLock()
	defer l.mu.RUnlock()
	cp := make([]Track, len(l.tracks))
	copy(cp, l.tracks)
	return cp
}

// Albums returns a copy of the album list.
func (l *Library) Albums() []Album {
	l.mu.RLock()
	defer l.mu.RUnlock()
	cp := make([]Album, len(l.albums))
	copy(cp, l.albums)
	return cp
}

// AlbumByID returns the album with the given ID.
func (l *Library) AlbumByID(id string) (Album, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	for _, a := range l.albums {
		if a.ID == id {
			return a, true
		}
	}
	return Album{}, false
}

// Browse returns directory entries for the given relative path.
func (l *Library) Browse(relPath string) ([]LibraryEntry, error) {
	cleanRel := cleanLibraryRelPath(relPath)
	abs := filepath.Join(l.musicRoot, filepath.FromSlash(cleanRel))
	entries, err := os.ReadDir(abs)
	if err != nil {
		return nil, err
	}

	// Build a quick lookup map for tracks by path.
	l.mu.RLock()
	trackMap := make(map[string]Track, len(l.tracks))
	for _, t := range l.tracks {
		trackMap[t.Path] = t
	}
	l.mu.RUnlock()

	var result []LibraryEntry
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		entryRel := joinLibraryRelPath(cleanRel, name)

		if e.IsDir() {
			result = append(result, LibraryEntry{
				Name: name,
				Type: "dir",
				Path: "/" + entryRel,
			})
		} else if isAudioFile(name) {
			le := LibraryEntry{
				Name: name,
				Type: "file",
				Path: "/" + entryRel,
			}
			if t, ok := trackMap[entryRel]; ok {
				le.Title = t.Title
				le.Artist = t.Artist
				le.Album = t.Album
				le.Duration = t.Duration
				le.ArtHash = t.ArtHash
			}
			result = append(result, le)
		}
	}
	return result, nil
}

// SearchTracks does fuzzy search across all tracks (title, artist, album, path).
func (l *Library) SearchTracks(query string) []LibraryEntry {
	l.mu.RLock()
	tracks := l.tracks
	l.mu.RUnlock()

	if len(tracks) == 0 || query == "" {
		return nil
	}

	entries := make([]LibraryEntry, 0, len(tracks))
	corpus := make([]string, 0, len(tracks))
	seenDirs := make(map[string]struct{})

	for _, t := range tracks {
		dir := filepath.ToSlash(filepath.Dir(t.Path))
		for dir != "." && dir != "" {
			if _, ok := seenDirs[dir]; ok {
				break
			}
			seenDirs[dir] = struct{}{}
			entries = append(entries, LibraryEntry{
				Name: filepath.Base(dir),
				Type: "dir",
				Path: "/" + dir,
			})
			corpus = append(corpus, dir+" "+filepath.Base(dir))
			next := filepath.ToSlash(filepath.Dir(dir))
			if next == dir {
				break
			}
			dir = next
		}

		entries = append(entries, LibraryEntry{
			Name:     filepath.Base(t.Path),
			Type:     "file",
			Path:     "/" + filepath.ToSlash(t.Path),
			Title:    t.Title,
			Artist:   t.Artist,
			Album:    t.Album,
			Duration: t.Duration,
			ArtHash:  t.ArtHash,
		})
		corpus = append(corpus, strings.Join([]string{
			t.Title,
			t.Artist,
			t.Album,
			filepath.ToSlash(t.Path),
			filepath.Base(filepath.Dir(t.Path)),
		}, " "))
	}

	matches := fuzzy.Find(query, corpus)
	var result []LibraryEntry
	for _, m := range matches {
		result = append(result, entries[m.Index])
	}
	return result
}

// SearchAlbums does fuzzy search across albums.
func (l *Library) SearchAlbums(query string) []Album {
	l.mu.RLock()
	albums := l.albums
	l.mu.RUnlock()

	if len(albums) == 0 || query == "" {
		return albums
	}

	corpus := make([]string, len(albums))
	for i, a := range albums {
		corpus[i] = a.Title + " " + a.Artist
	}

	matches := fuzzy.Find(query, corpus)
	result := make([]Album, 0, len(matches))
	for _, m := range matches {
		result = append(result, albums[m.Index])
	}
	return result
}

// TrackByPath returns the track at the given library-relative path.
func (l *Library) TrackByPath(relPath string) (Track, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	clean := cleanLibraryRelPath(relPath)
	for _, t := range l.tracks {
		if t.Path == clean {
			return t, true
		}
	}
	return Track{}, false
}

// TracksInDir returns all tracks under a library-relative directory.
func (l *Library) TracksInDir(relDir string) []Track {
	l.mu.RLock()
	defer l.mu.RUnlock()
	prefix := cleanLibraryRelPath(relDir)
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	var result []Track
	for _, t := range l.tracks {
		if strings.HasPrefix(t.Path, prefix) {
			result = append(result, t)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Path < result[j].Path
	})
	return result
}

// TrackFileURI builds the HTTP URI for a track to pass to Sonos.
func (l *Library) TrackFileURI(lanIP string, filePort int, t Track) string {
	encoded := url.PathEscape(t.Path)
	// Preserve slashes in path.
	encoded = strings.ReplaceAll(encoded, "%2F", "/")
	return fmt.Sprintf("http://%s:%d/files/%s", lanIP, filePort, encoded)
}

// ── Scan ──────────────────────────────────────────────────────────────────────

type libraryCache struct {
	Tracks    []Track           `json:"tracks"`
	Art       map[string][]byte `json:"art"`
	ScannedAt time.Time         `json:"scanned_at"`
}

// Scan scans the music root, calling progressFn with (scanned, total).
// Sends library_scan SSE events via the broadcaster.
// When full is false, unchanged files (same mtime+size) are reused from cache,
// skipping tag parsing and ffprobe. When true, every file is re-read.
// Returns false without doing anything if a scan is already running.
func (l *Library) Scan(events *Broadcaster, full bool) bool {
	l.mu.Lock()
	if l.scanning {
		l.mu.Unlock()
		return false
	}
	l.scanning = true
	l.mu.Unlock()

	events.Send(evtLibraryScan("scanning", nil))

	// Try loading cache first for instant availability.
	cached := l.loadCache()
	if cached != nil {
		l.mu.Lock()
		l.tracks = cached.Tracks
		l.art = cached.Art
		l.albums = buildAlbums(cached.Tracks)
		l.ready = true
		l.mu.Unlock()
		events.Send(evtLibraryScan("done", map[string]any{"track_count": len(cached.Tracks)}))
	}

	go func() {
		defer func() {
			l.mu.Lock()
			l.scanning = false
			l.mu.Unlock()
		}()

		// Snapshot the current index for diffing. In full mode we start blank.
		var prev map[string]Track
		var prevArt map[string][]byte
		if !full {
			l.mu.RLock()
			prev = make(map[string]Track, len(l.tracks))
			for _, t := range l.tracks {
				prev[t.Path] = t
			}
			prevArt = make(map[string][]byte, len(l.art))
			for k, v := range l.art {
				prevArt[k] = v
			}
			l.mu.RUnlock()
		}

		tracks, artMap, err := l.scanDiff(events, prev, prevArt)
		if err != nil {
			log.Printf("library scan error: %v", err)
			events.Send(evtError(fmt.Sprintf("Library scan error: %v", err)))
			return
		}
		albums := buildAlbums(tracks)

		l.mu.Lock()
		l.tracks = tracks
		l.art = artMap
		l.albums = albums
		l.ready = true
		l.mu.Unlock()

		events.Send(evtLibraryScan("done", map[string]any{"track_count": len(tracks)}))
		l.saveCache(tracks, artMap)
	}()
	return true
}

// scanDiff walks musicRoot and produces the new track list. When prev is
// non-nil, files whose mtime+size match a cached Track are reused verbatim
// (no tag parse, no ffprobe). The returned art map is pruned to only contain
// hashes still referenced by the surviving track set.
func (l *Library) scanDiff(events *Broadcaster, prev map[string]Track, prevArt map[string][]byte) ([]Track, map[string][]byte, error) {
	var allPaths []string
	err := filepath.WalkDir(l.musicRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if !d.IsDir() && isAudioFile(path) {
			allPaths = append(allPaths, path)
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	tracks := make([]Track, 0, len(allPaths))
	artMap := make(map[string][]byte)

	parsed := 0
	for _, absPath := range allPaths {
		rel, relErr := filepath.Rel(l.musicRoot, absPath)
		if relErr == nil {
			rel = filepath.ToSlash(rel)
		}

		// Fast path: stat matches cached entry → reuse Track verbatim.
		if prev != nil && rel != "" {
			if cached, ok := prev[rel]; ok && cached.ModTimeUnix != 0 {
				if st, statErr := os.Stat(absPath); statErr == nil {
					if st.ModTime().UnixNano() == cached.ModTimeUnix && st.Size() == cached.Size {
						if cached.ArtHash != "" {
							if data, ok := prevArt[cached.ArtHash]; ok {
								artMap[cached.ArtHash] = data
							}
						}
						tracks = append(tracks, cached)
						continue
					}
				}
			}
		}

		// Progress is reported against files actually parsed, not walked.
		if parsed%100 == 0 {
			events.Send(evtLibraryScan("scanning", map[string]any{"parsed": parsed}))
		}
		parsed++

		t, art := readTrack(l.musicRoot, absPath)
		if st, statErr := os.Stat(absPath); statErr == nil {
			t.ModTimeUnix = st.ModTime().UnixNano()
			t.Size = st.Size()
		}
		if art != nil {
			hash := artHash(art)
			t.ArtHash = hash
			artMap[hash] = art
		}
		tracks = append(tracks, t)
	}

	return tracks, artMap, nil
}

func readTrack(musicRoot, absPath string) (Track, []byte) {
	rel, err := filepath.Rel(musicRoot, absPath)
	if err != nil {
		rel = absPath
	}
	rel = filepath.ToSlash(rel)

	t := Track{
		Path:  rel,
		Title: strings.TrimSuffix(filepath.Base(rel), filepath.Ext(rel)),
	}

	f, err := os.Open(absPath)
	if err != nil {
		return t, nil
	}
	defer f.Close()

	md, err := tag.ReadFrom(f)
	if err != nil {
		return t, nil
	}

	if md.Title() != "" {
		t.Title = md.Title()
	}
	t.Artist = md.Artist()
	t.Album = md.Album()
	t.AlbumArtist = md.AlbumArtist()
	if md.Year() > 0 {
		t.Year = md.Year()
	}
	n, _ := md.Track()
	t.TrackNum = n
	t.Duration = probeTrackDuration(absPath)

	// Extract art.
	var artData []byte
	if pic := md.Picture(); pic != nil {
		artData = pic.Data
	}

	return t, artData
}

func probeTrackDuration(absPath string) int {
	ffprobe := resolveFFprobe()
	if ffprobe == "" {
		return 0
	}

	out, err := exec.Command(
		ffprobe,
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		absPath,
	).Output()
	if err != nil {
		return 0
	}

	secs, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil || secs <= 0 {
		return 0
	}
	return int(secs + 0.5)
}

func resolveFFprobe() string {
	ffprobeOnce.Do(func() {
		path, err := exec.LookPath("ffprobe")
		if err != nil {
			log.Printf("library scan: ffprobe not found; track durations will be unavailable for scanned local files")
			return
		}
		ffprobePath = path
	})
	return ffprobePath
}

func artHash(data []byte) string {
	h := sha1.Sum(data)
	return fmt.Sprintf("%x", h[:8])
}

func cleanLibraryRelPath(relPath string) string {
	clean := filepath.ToSlash(filepath.Clean("/" + relPath))
	clean = strings.TrimPrefix(clean, "/")
	if clean == "." {
		return ""
	}
	return clean
}

func joinLibraryRelPath(base, name string) string {
	if base == "" {
		return name
	}
	return filepath.ToSlash(filepath.Join(base, name))
}

func isAudioFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".flac", ".mp3", ".m4a", ".wav", ".ogg":
		return true
	}
	return false
}

// primaryArtist returns the first artist in a multi-artist string, stripping
// everything after the first ";", "/", or "&". This prevents featured-artist
// variants like "Grentperez; Ruel" from creating duplicate album entries.
func primaryArtist(s string) string {
	for _, sep := range []string{";", "/"} {
		if i := strings.Index(s, sep); i >= 0 {
			return strings.TrimSpace(s[:i])
		}
	}
	return s
}

// buildAlbums groups tracks into albums sorted by artist then album.
func buildAlbums(tracks []Track) []Album {
	type key struct{ artist, album string }
	m := make(map[key]*Album)

	for _, t := range tracks {
		artist := t.AlbumArtist
		if artist == "" {
			artist = t.Artist
		}
		artistKey := primaryArtist(artist)
		k := key{artist: artistKey, album: t.Album}
		if m[k] == nil {
			id := albumID(artistKey, t.Album)
			m[k] = &Album{
				ID:     id,
				Title:  t.Album,
				Artist: artistKey,
				Year:   t.Year,
				Path:   filepath.Dir(t.Path),
			}
		}
		al := m[k]
		al.Tracks = append(al.Tracks, t)
		if al.ArtHash == "" && t.ArtHash != "" {
			al.ArtHash = t.ArtHash
		}
		if al.Year == 0 && t.Year > 0 {
			al.Year = t.Year
		}
	}

	albums := make([]Album, 0, len(m))
	for _, al := range m {
		sort.Slice(al.Tracks, func(i, j int) bool {
			if al.Tracks[i].TrackNum != al.Tracks[j].TrackNum {
				return al.Tracks[i].TrackNum < al.Tracks[j].TrackNum
			}
			return al.Tracks[i].Path < al.Tracks[j].Path
		})
		al.TrackCount = len(al.Tracks)
		albums = append(albums, *al)
	}

	sort.Slice(albums, func(i, j int) bool {
		if albums[i].Artist != albums[j].Artist {
			return strings.ToLower(albums[i].Artist) < strings.ToLower(albums[j].Artist)
		}
		return strings.ToLower(albums[i].Title) < strings.ToLower(albums[j].Title)
	})
	return albums
}

func albumID(artist, album string) string {
	h := sha1.Sum([]byte(artist + "\x00" + album))
	return fmt.Sprintf("%x", h[:8])
}

// ── Cache ──────────────────────────────────────────────────────────────────────

func (l *Library) loadCache() *libraryCache {
	data, err := os.ReadFile(l.cachePath)
	if err != nil {
		return nil
	}
	var c libraryCache
	if err := json.Unmarshal(data, &c); err != nil {
		return nil
	}
	if c.Art == nil {
		c.Art = make(map[string][]byte)
	}
	return &c
}

func (l *Library) saveCache(tracks []Track, art map[string][]byte) {
	if err := os.MkdirAll(filepath.Dir(l.cachePath), 0o755); err != nil {
		log.Printf("library cache mkdir: %v", err)
		return
	}
	c := libraryCache{
		Tracks:    tracks,
		Art:       art,
		ScannedAt: time.Now(),
	}
	data, err := json.Marshal(c)
	if err != nil {
		log.Printf("library cache marshal: %v", err)
		return
	}
	if err := os.WriteFile(l.cachePath, data, 0o644); err != nil {
		log.Printf("library cache write: %v", err)
	}
}
