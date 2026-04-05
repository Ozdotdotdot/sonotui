package tui

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// DaemonAddr holds the daemon connection address.
type DaemonAddr struct {
	Host string
	Port int
}

func (a DaemonAddr) Base() string {
	return fmt.Sprintf("http://%s:%d", a.Host, a.Port)
}

// StatusResponse mirrors the daemon's /status JSON response.
type StatusResponse struct {
	Transport    string      `json:"transport"`
	Track        TrackInfo   `json:"track"`
	Volume       int         `json:"volume"`
	Elapsed      int         `json:"elapsed"`
	Duration     int         `json:"duration"`
	IsLineIn     bool        `json:"is_line_in"`
	Speaker      *SpeakerInfo `json:"speaker"`
	LibraryReady bool        `json:"library_ready"`
}

// TrackInfo mirrors daemon.TrackInfo.
type TrackInfo struct {
	Title    string `json:"title"`
	Artist   string `json:"artist"`
	Album    string `json:"album"`
	ArtURL   string `json:"art_url"`
	Duration int    `json:"duration"`
}

// SpeakerInfo mirrors daemon.Speaker.
type SpeakerInfo struct {
	Name string `json:"name"`
	UUID string `json:"uuid"`
	IP   string `json:"ip"`
}

// QueueItem mirrors daemon.QueueItem.
type QueueItem struct {
	Position int    `json:"position"`
	Title    string `json:"title"`
	Artist   string `json:"artist"`
	Album    string `json:"album"`
	Duration int    `json:"duration"`
	URI      string `json:"uri"`
	IsLocal  bool   `json:"is_local"`
}

// LibraryEntry mirrors daemon.LibraryEntry.
type LibraryEntry struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Path     string `json:"path"`
	Title    string `json:"title,omitempty"`
	Artist   string `json:"artist,omitempty"`
	Album    string `json:"album,omitempty"`
	Duration int    `json:"duration,omitempty"`
	ArtHash  string `json:"art_hash,omitempty"`
}

// Album mirrors daemon.Album (summary form from /albums).
type Album struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Artist     string `json:"artist"`
	Year       int    `json:"year"`
	TrackCount int    `json:"track_count"`
	ArtHash    string `json:"art_hash"`
	Path       string `json:"path"`
}

// AlbumDetail mirrors daemon.Album (full form from /albums/:id).
type AlbumDetail struct {
	Album
	Tracks []LibraryEntry `json:"tracks"`
}

// SSEEvent is a parsed server-sent event.
type SSEEvent struct {
	Type string
	Raw  map[string]json.RawMessage
}

// Client is an HTTP client for the sonotuid daemon.
type Client struct {
	addr DaemonAddr
	http *http.Client
}

// NewClient creates a Client.
func NewClient(addr DaemonAddr) *Client {
	return &Client{
		addr: addr,
		http: &http.Client{Timeout: 5 * time.Second},
	}
}

// Ping checks if the daemon is reachable.
func (c *Client) Ping() bool {
	resp, err := c.http.Get(c.addr.Base() + "/status")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// GetStatus fetches the full daemon status.
func (c *Client) GetStatus() (*StatusResponse, error) {
	resp, err := c.http.Get(c.addr.Base() + "/status")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var s StatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		return nil, err
	}
	return &s, nil
}

// GetQueue fetches the current queue.
func (c *Client) GetQueue() ([]QueueItem, error) {
	resp, err := c.http.Get(c.addr.Base() + "/queue")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var items []QueueItem
	return items, json.NewDecoder(resp.Body).Decode(&items)
}

// GetSpeakers fetches all discovered speakers.
func (c *Client) GetSpeakers() ([]SpeakerInfo, error) {
	resp, err := c.http.Get(c.addr.Base() + "/speakers")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var speakers []SpeakerInfo
	return speakers, json.NewDecoder(resp.Body).Decode(&speakers)
}

// BrowseLibrary fetches a library directory listing.
func (c *Client) BrowseLibrary(path string) ([]LibraryEntry, error) {
	if path == "" || path == "/" {
		path = ""
	}
	resp, err := c.http.Get(c.addr.Base() + "/library" + path)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result struct {
		Entries []LibraryEntry `json:"entries"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Entries, nil
}

// SearchLibrary does a fuzzy library search.
func (c *Client) SearchLibrary(q string) ([]LibraryEntry, error) {
	resp, err := c.http.Get(c.addr.Base() + "/library/search?q=" + q)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var items []LibraryEntry
	return items, json.NewDecoder(resp.Body).Decode(&items)
}

// GetAlbums fetches the album list.
func (c *Client) GetAlbums() ([]Album, error) {
	resp, err := c.http.Get(c.addr.Base() + "/albums")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var albums []Album
	return albums, json.NewDecoder(resp.Body).Decode(&albums)
}

// SearchAlbums does a fuzzy album search.
func (c *Client) SearchAlbums(q string) ([]Album, error) {
	resp, err := c.http.Get(c.addr.Base() + "/albums/search?q=" + q)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var albums []Album
	return albums, json.NewDecoder(resp.Body).Decode(&albums)
}

// GetAlbumDetail fetches full album detail including track list.
func (c *Client) GetAlbumDetail(id string) (*AlbumDetail, error) {
	resp, err := c.http.Get(c.addr.Base() + "/albums/" + id)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var al AlbumDetail
	return &al, json.NewDecoder(resp.Body).Decode(&al)
}

// ── Transport commands ─────────────────────────────────────────────────────────

func (c *Client) post(path string, body any) error {
	var r io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		r = bytes.NewReader(data)
	} else {
		r = strings.NewReader("{}")
	}
	resp, err := c.http.Post(c.addr.Base()+path, "application/json", r)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (c *Client) Play() error  { return c.post("/play", nil) }
func (c *Client) Pause() error { return c.post("/pause", nil) }
func (c *Client) Stop() error  { return c.post("/stop", nil) }
func (c *Client) Next() error  { return c.post("/next", nil) }
func (c *Client) Prev() error  { return c.post("/prev", nil) }
func (c *Client) LineIn() error { return c.post("/linein", nil) }

func (c *Client) SetVolume(v int) error {
	return c.post("/volume", map[string]int{"value": v})
}

func (c *Client) SetVolumeRelative(delta int) error {
	return c.post("/volume/relative", map[string]int{"delta": delta})
}

func (c *Client) SetActiveSpeaker(uuid string) error {
	return c.post("/speakers/active", map[string]string{"uuid": uuid})
}

// Queue commands.
func (c *Client) PlayFromQueue(pos int) error {
	return c.post(fmt.Sprintf("/queue/%d/play", pos), nil)
}

func (c *Client) DeleteFromQueue(pos int) error {
	req, _ := http.NewRequest(http.MethodDelete, c.addr.Base()+fmt.Sprintf("/queue/%d", pos), nil)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (c *Client) ClearQueue() error {
	req, _ := http.NewRequest(http.MethodDelete, c.addr.Base()+"/queue", nil)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (c *Client) ReorderQueue(from, to int) error {
	return c.post("/queue/reorder", map[string]int{"from": from, "to": to})
}

func (c *Client) AddPathsToQueue(paths []string) error {
	return c.post("/queue/batch", map[string][]string{"paths": paths})
}

// ── SSE stream ────────────────────────────────────────────────────────────────

// StreamEvents connects to the SSE stream and sends parsed events to ch.
// It reconnects automatically on disconnect. Call cancel() to stop.
func (c *Client) StreamEvents(ch chan<- SSEEvent, done <-chan struct{}) {
	go func() {
		for {
			select {
			case <-done:
				return
			default:
			}

			if err := c.readSSE(ch, done); err != nil {
				log.Printf("sse: %v — reconnecting in 5s", err)
			}

			select {
			case <-done:
				return
			case <-time.After(5 * time.Second):
			}
		}
	}()
}

func (c *Client) readSSE(ch chan<- SSEEvent, done <-chan struct{}) error {
	req, err := http.NewRequest(http.MethodGet, c.addr.Base()+"/events", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")

	// No timeout for SSE connection.
	sseClient := &http.Client{}
	resp, err := sseClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		select {
		case <-done:
			return nil
		default:
		}

		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")

		var raw map[string]json.RawMessage
		if err := json.Unmarshal([]byte(data), &raw); err != nil {
			log.Printf("sse parse: %v", err)
			continue
		}

		var evtType string
		if t, ok := raw["type"]; ok {
			json.Unmarshal(t, &evtType) //nolint:errcheck
		}

		select {
		case <-done:
			return nil
		case ch <- SSEEvent{Type: evtType, Raw: raw}:
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("sse scanner: %w", err)
	}
	return fmt.Errorf("sse connection closed")
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// ParseInt extracts an int from a RawMessage.
func ParseInt(r json.RawMessage) int {
	var v int
	json.Unmarshal(r, &v) //nolint:errcheck
	return v
}

// ParseString extracts a string from a RawMessage.
func ParseString(r json.RawMessage) string {
	var v string
	json.Unmarshal(r, &v) //nolint:errcheck
	return v
}

// ParseBool extracts a bool from a RawMessage.
func ParseBool(r json.RawMessage) bool {
	var v bool
	json.Unmarshal(r, &v) //nolint:errcheck
	return v
}

// ParseFloat extracts a float64 from a RawMessage.
func ParseFloat(r json.RawMessage) float64 {
	var v float64
	json.Unmarshal(r, &v) //nolint:errcheck
	return v
}
