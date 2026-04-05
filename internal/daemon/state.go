package daemon

import "sync"

// ScanStatus represents the current library scan state.
type ScanStatus string

const (
	ScanIdle     ScanStatus = "idle"
	ScanScanning ScanStatus = "scanning"
	ScanDone     ScanStatus = "done"
)

// Speaker represents a discovered Sonos zone player.
type Speaker struct {
	Name string `json:"name"`
	UUID string `json:"uuid"`
	IP   string `json:"ip"`
}

// TrackInfo holds now-playing metadata.
type TrackInfo struct {
	Title    string `json:"title"`
	Artist   string `json:"artist"`
	Album    string `json:"album"`
	ArtURL   string `json:"art_url"`
	Duration int    `json:"duration"`
	URI      string `json:"uri,omitempty"`
}

// QueueItem represents a single item in the Sonos queue.
type QueueItem struct {
	Position int    `json:"position"`
	Title    string `json:"title"`
	Artist   string `json:"artist"`
	Album    string `json:"album"`
	Duration int    `json:"duration"`
	URI      string `json:"uri"`
	IsLocal  bool   `json:"is_local"`
}

// State is the daemon's in-memory state protected by a mutex.
type State struct {
	mu sync.RWMutex

	// Speakers
	Speakers      []Speaker
	ActiveSpeaker *Speaker

	// Playback
	Transport string // PLAYING | PAUSED_PLAYBACK | STOPPED | TRANSITIONING
	Track     TrackInfo
	Volume    int
	IsLineIn  bool

	// Position (local counter, same logic as v1.1)
	Elapsed  int
	Duration int
	Playing  bool

	// TrackGen is incremented on every track URI change, used to detect
	// stale async goroutines that should not overwrite newer state.
	TrackGen uint64

	// Queue (mirror of Sonos Q:0, refreshed on queue-change GENA events)
	Queue []QueueItem

	// Library
	LibraryReady bool
	LibraryScan  ScanStatus
}

// NewState returns an initialised State.
func NewState() *State {
	return &State{
		Transport:   "STOPPED",
		LibraryScan: ScanIdle,
	}
}

// Lock acquires the write lock.
func (s *State) Lock() { s.mu.Lock() }

// Unlock releases the write lock.
func (s *State) Unlock() { s.mu.Unlock() }

// RLock acquires the read lock.
func (s *State) RLock() { s.mu.RLock() }

// RUnlock releases the read lock.
func (s *State) RUnlock() { s.mu.RUnlock() }

// Snapshot returns a copy of the current state (read-locked).
func (s *State) Snapshot() State {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cp := *s
	cp.Speakers = append([]Speaker(nil), s.Speakers...)
	cp.Queue = append([]QueueItem(nil), s.Queue...)
	if s.ActiveSpeaker != nil {
		sp := *s.ActiveSpeaker
		cp.ActiveSpeaker = &sp
	}
	return cp
}
