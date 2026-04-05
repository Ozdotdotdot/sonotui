package daemon

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
)

// SSEEvent is a single event sent over the SSE stream.
type SSEEvent struct {
	data []byte
}

// Broadcaster manages SSE client connections and broadcasts events.
type Broadcaster struct {
	mu      sync.Mutex
	clients map[chan SSEEvent]struct{}
}

// NewBroadcaster creates a ready-to-use Broadcaster.
func NewBroadcaster() *Broadcaster {
	return &Broadcaster{clients: make(map[chan SSEEvent]struct{})}
}

// Subscribe registers a new SSE client channel and returns it.
func (b *Broadcaster) Subscribe() chan SSEEvent {
	ch := make(chan SSEEvent, 32)
	b.mu.Lock()
	b.clients[ch] = struct{}{}
	b.mu.Unlock()
	return ch
}

// Unsubscribe removes a client channel.
func (b *Broadcaster) Unsubscribe(ch chan SSEEvent) {
	b.mu.Lock()
	delete(b.clients, ch)
	b.mu.Unlock()
}

// Send broadcasts an event to all connected clients.
func (b *Broadcaster) Send(payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		log.Printf("sse marshal: %v", err)
		return
	}
	evt := SSEEvent{data: data}
	b.mu.Lock()
	for ch := range b.clients {
		select {
		case ch <- evt:
		default:
			// Slow client — drop event.
		}
	}
	b.mu.Unlock()
}

// ServeHTTP implements http.Handler for the SSE endpoint.
func (b *Broadcaster) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	ch := b.Subscribe()
	defer b.Unsubscribe(ch)

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case evt := <-ch:
			fmt.Fprintf(w, "data: %s\n\n", evt.data)
			flusher.Flush()
		}
	}
}

// ── Typed event constructors ──────────────────────────────────────────────────

func evtTransport(state string) map[string]any {
	return map[string]any{"type": "transport", "state": state}
}

func evtTrack(t TrackInfo) map[string]any {
	return map[string]any{
		"type":     "track",
		"title":    t.Title,
		"artist":   t.Artist,
		"album":    t.Album,
		"art_url":  t.ArtURL,
		"duration": t.Duration,
		"uri":      t.URI,
	}
}

func evtPosition(elapsed, duration int) map[string]any {
	return map[string]any{"type": "position", "elapsed": elapsed, "duration": duration}
}

func evtVolume(value int) map[string]any {
	return map[string]any{"type": "volume", "value": value}
}

func evtLineIn(active bool) map[string]any {
	return map[string]any{"type": "linein", "active": active}
}

func evtQueueChanged() map[string]any {
	return map[string]any{"type": "queue_changed"}
}

func evtSpeaker(sp Speaker) map[string]any {
	return map[string]any{"type": "speaker", "name": sp.Name, "uuid": sp.UUID}
}

func evtLibraryScan(status string, extra map[string]any) map[string]any {
	m := map[string]any{"type": "library_scan", "status": status}
	for k, v := range extra {
		m[k] = v
	}
	return m
}

func evtError(msg string) map[string]any {
	return map[string]any{"type": "error", "message": msg}
}
