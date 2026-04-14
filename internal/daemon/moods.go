package daemon

import (
	"bufio"
	"encoding/json"
	"log"
	"math/rand/v2"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// MoodEntry describes a single source of tracks within a mood.
type MoodEntry struct {
	Type  string `json:"type"`  // "artist" or "path"
	Value string `json:"value"` // artist name or library-relative path
}

// Mood is a named collection of music entries that can be loaded into the queue.
type Mood struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Shuffle     bool        `json:"shuffle"`
	Entries     []MoodEntry `json:"entries"`
}

// moodJSON is the on-disk shape inside moods.json (keyed by mood name).
type moodJSON struct {
	Description string      `json:"description"`
	Shuffle     bool        `json:"shuffle"`
	Entries     []MoodEntry `json:"entries"`
}

// MoodManager loads and resolves mood definitions against the music library.
type MoodManager struct {
	mu       sync.RWMutex
	moodsDir string
	lib      *Library
	moods    map[string]Mood
}

// NewMoodManager creates a MoodManager that reads from moodsDir.
func NewMoodManager(moodsDir string, lib *Library) *MoodManager {
	return &MoodManager{
		moodsDir: moodsDir,
		lib:      lib,
		moods:    make(map[string]Mood),
	}
}

// Load reads mood definitions from moods.json and .m3u files in the moods directory.
// It can be called multiple times to hot-reload.
func (m *MoodManager) Load() {
	merged := make(map[string]Mood)

	// Pass 1: .m3u files (lower priority).
	m3us, _ := filepath.Glob(filepath.Join(m.moodsDir, "*.m3u"))
	for _, path := range m3us {
		name := strings.TrimSuffix(filepath.Base(path), ".m3u")
		mood, err := parseM3U(path, name)
		if err != nil {
			log.Printf("moods: error parsing %s: %v", path, err)
			continue
		}
		merged[name] = mood
	}

	// Pass 2: moods.json (higher priority, overwrites .m3u).
	jsonPath := filepath.Join(m.moodsDir, "moods.json")
	if f, err := os.Open(jsonPath); err == nil {
		defer f.Close()
		var raw map[string]moodJSON
		if err := json.NewDecoder(f).Decode(&raw); err != nil {
			log.Printf("moods: error decoding moods.json: %v", err)
		} else {
			for name, def := range raw {
				if _, dup := merged[name]; dup {
					log.Printf("moods: %q defined in both moods.json and .m3u — using moods.json", name)
				}
				merged[name] = Mood{
					Name:        name,
					Description: def.Description,
					Shuffle:     def.Shuffle,
					Entries:     def.Entries,
				}
			}
		}
	}

	m.mu.Lock()
	m.moods = merged
	m.mu.Unlock()

	log.Printf("moods: loaded %d mood(s)", len(merged))
}

// MoodListItem is returned by List with resolved track counts.
type MoodListItem struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Shuffle     bool   `json:"shuffle"`
	TrackCount  int    `json:"track_count"`
}

// List returns all loaded moods with resolved track counts.
func (m *MoodManager) List() []MoodListItem {
	m.mu.RLock()
	moods := make([]Mood, 0, len(m.moods))
	for _, mood := range m.moods {
		moods = append(moods, mood)
	}
	m.mu.RUnlock()

	sort.Slice(moods, func(i, j int) bool { return moods[i].Name < moods[j].Name })

	items := make([]MoodListItem, len(moods))
	for i, mood := range moods {
		tracks := m.resolveTracks(mood)
		items[i] = MoodListItem{
			Name:        mood.Name,
			Description: mood.Description,
			Shuffle:     mood.Shuffle,
			TrackCount:  len(tracks),
		}
	}
	return items
}

// Resolve looks up a mood by name and returns its tracks and shuffle preference.
func (m *MoodManager) Resolve(name string) ([]Track, bool, error) {
	m.mu.RLock()
	mood, ok := m.moods[name]
	m.mu.RUnlock()

	if !ok {
		return nil, false, nil
	}

	tracks := m.resolveTracks(mood)

	if mood.Shuffle {
		rand.Shuffle(len(tracks), func(i, j int) {
			tracks[i], tracks[j] = tracks[j], tracks[i]
		})
	}

	return tracks, mood.Shuffle, nil
}

// resolveTracks expands all mood entries into a deduplicated track list.
func (m *MoodManager) resolveTracks(mood Mood) []Track {
	seen := make(map[string]struct{})
	var tracks []Track

	for _, entry := range mood.Entries {
		var resolved []Track
		switch entry.Type {
		case "artist":
			resolved = m.tracksByArtist(entry.Value)
		case "path":
			if t, ok := m.lib.TrackByPath(entry.Value); ok {
				resolved = []Track{t}
			} else {
				resolved = m.lib.TracksInDir(entry.Value)
			}
		}
		for _, t := range resolved {
			if _, dup := seen[t.Path]; !dup {
				seen[t.Path] = struct{}{}
				tracks = append(tracks, t)
			}
		}
	}
	return tracks
}

// tracksByArtist returns all tracks from albums whose artist matches (case-insensitive).
func (m *MoodManager) tracksByArtist(artist string) []Track {
	lower := strings.ToLower(artist)
	var tracks []Track
	for _, album := range m.lib.Albums() {
		if strings.ToLower(album.Artist) == lower {
			tracks = append(tracks, album.Tracks...)
		}
	}
	// Sort by album path then track number for consistent ordering.
	sort.Slice(tracks, func(i, j int) bool {
		if tracks[i].Album != tracks[j].Album {
			return tracks[i].Album < tracks[j].Album
		}
		return tracks[i].TrackNum < tracks[j].TrackNum
	})
	return tracks
}

// parseM3U reads a .m3u file and returns a Mood.
func parseM3U(path, name string) (Mood, error) {
	f, err := os.Open(path)
	if err != nil {
		return Mood{}, err
	}
	defer f.Close()

	mood := Mood{Name: name}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		// Special directives.
		if strings.EqualFold(line, "#SHUFFLE") {
			mood.Shuffle = true
			continue
		}
		if after, ok := strings.CutPrefix(line, "#ARTIST:"); ok {
			mood.Entries = append(mood.Entries, MoodEntry{Type: "artist", Value: strings.TrimSpace(after)})
			continue
		}
		// Regular comment.
		if strings.HasPrefix(line, "#") {
			continue
		}

		// Everything else is a library-relative path.
		mood.Entries = append(mood.Entries, MoodEntry{Type: "path", Value: line})
	}
	return mood, scanner.Err()
}
