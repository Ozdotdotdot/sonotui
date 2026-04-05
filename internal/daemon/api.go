package daemon

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
)

// API handles the REST API for the daemon on :8989.
type API struct {
	state   *State
	events  *Broadcaster
	sonos   *SonosManager
	lib     *Library
	lanIP   string
	filePort int
}

// NewAPI creates an API handler.
func NewAPI(state *State, events *Broadcaster, sonos *SonosManager, lib *Library, lanIP string, filePort int) *API {
	return &API{
		state:    state,
		events:   events,
		sonos:    sonos,
		lib:      lib,
		lanIP:    lanIP,
		filePort: filePort,
	}
}

// Handler returns the http.Handler for the REST API.
func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()

	// Transport.
	mux.HandleFunc("POST /play", a.handlePlay)
	mux.HandleFunc("POST /pause", a.handlePause)
	mux.HandleFunc("POST /stop", a.handleStop)
	mux.HandleFunc("POST /next", a.handleNext)
	mux.HandleFunc("POST /prev", a.handlePrev)
	mux.HandleFunc("POST /linein", a.handleLineIn)
	mux.HandleFunc("POST /volume", a.handleSetVolume)
	mux.HandleFunc("POST /volume/relative", a.handleRelativeVolume)

	// State + SSE.
	mux.HandleFunc("GET /status", a.handleStatus)
	mux.HandleFunc("GET /events", a.events.ServeHTTP)

	// Speakers.
	mux.HandleFunc("GET /speakers", a.handleGetSpeakers)
	mux.HandleFunc("POST /speakers/active", a.handleSetActiveSpeaker)

	// Queue.
	mux.HandleFunc("GET /queue", a.handleGetQueue)
	mux.HandleFunc("POST /queue", a.handleAddToQueue)
	mux.HandleFunc("DELETE /queue", a.handleClearQueue)
	mux.HandleFunc("POST /queue/batch", a.handleBatchQueue)
	mux.HandleFunc("POST /queue/reorder", a.handleReorderQueue)

	// Queue item by position — Go 1.22 pattern routing.
	mux.HandleFunc("/queue/", a.handleQueueItem)

	// Library.
	mux.HandleFunc("GET /library", a.handleLibraryRoot)
	mux.HandleFunc("GET /library/search", a.handleLibrarySearch)
	mux.HandleFunc("/library/", a.handleLibraryPath)

	// Albums.
	mux.HandleFunc("GET /albums", a.handleGetAlbums)
	mux.HandleFunc("GET /albums/search", a.handleAlbumsSearch)
	mux.HandleFunc("/albums/", a.handleAlbumByID)

	// Art (served by fileserver on :8990, but also proxied here for TUI convenience).
	mux.HandleFunc("GET /art/", a.handleArt)

	return mux
}

// ── helpers ───────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("writeJSON: %v", err)
	}
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg}) //nolint:errcheck
}

func readJSON(r *http.Request, v any) error {
	return json.NewDecoder(r.Body).Decode(v)
}

// ── Transport ─────────────────────────────────────────────────────────────────

func (a *API) handlePlay(w http.ResponseWriter, r *http.Request) {
	if err := a.sonos.Play(); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]string{"status": "ok"})
}

func (a *API) handlePause(w http.ResponseWriter, r *http.Request) {
	if err := a.sonos.Pause(); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]string{"status": "ok"})
}

func (a *API) handleStop(w http.ResponseWriter, r *http.Request) {
	if err := a.sonos.Stop(); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]string{"status": "ok"})
}

func (a *API) handleNext(w http.ResponseWriter, r *http.Request) {
	if err := a.sonos.Next(); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]string{"status": "ok"})
}

func (a *API) handlePrev(w http.ResponseWriter, r *http.Request) {
	if err := a.sonos.Prev(); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]string{"status": "ok"})
}

func (a *API) handleLineIn(w http.ResponseWriter, r *http.Request) {
	if err := a.sonos.LineIn(); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]string{"status": "ok"})
}

func (a *API) handleSetVolume(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Value int `json:"value"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if err := a.sonos.SetVolume(body.Value); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]string{"status": "ok"})
}

func (a *API) handleRelativeVolume(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Delta int `json:"delta"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	snap := a.state.Snapshot()
	newVol := snap.Volume + body.Delta
	if newVol < 0 {
		newVol = 0
	}
	if newVol > 100 {
		newVol = 100
	}
	if err := a.sonos.SetVolume(newVol); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]string{"status": "ok"})
}

// ── Status ────────────────────────────────────────────────────────────────────

func (a *API) handleStatus(w http.ResponseWriter, r *http.Request) {
	snap := a.state.Snapshot()

	type speakerResp struct {
		Name string `json:"name"`
		UUID string `json:"uuid"`
		IP   string `json:"ip"`
	}

	var sp *speakerResp
	if snap.ActiveSpeaker != nil {
		sp = &speakerResp{
			Name: snap.ActiveSpeaker.Name,
			UUID: snap.ActiveSpeaker.UUID,
			IP:   snap.ActiveSpeaker.IP,
		}
	}

	writeJSON(w, map[string]any{
		"transport":     snap.Transport,
		"track":         snap.Track,
		"volume":        snap.Volume,
		"elapsed":       snap.Elapsed,
		"duration":      snap.Duration,
		"is_line_in":    snap.IsLineIn,
		"speaker":       sp,
		"library_ready": snap.LibraryReady,
	})
}

// ── Speakers ──────────────────────────────────────────────────────────────────

func (a *API) handleGetSpeakers(w http.ResponseWriter, r *http.Request) {
	snap := a.state.Snapshot()
	writeJSON(w, snap.Speakers)
}

func (a *API) handleSetActiveSpeaker(w http.ResponseWriter, r *http.Request) {
	var body struct {
		UUID string `json:"uuid"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if err := a.sonos.SwitchSpeaker(body.UUID); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]string{"status": "ok"})
}

// ── Queue ─────────────────────────────────────────────────────────────────────

func (a *API) handleGetQueue(w http.ResponseWriter, r *http.Request) {
	snap := a.state.Snapshot()
	writeJSON(w, snap.Queue)
}

func (a *API) handleAddToQueue(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URI      string         `json:"uri"`
		Metadata map[string]any `json:"metadata"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	meta := ""
	if body.Metadata != nil {
		if m, err := json.Marshal(body.Metadata); err == nil {
			meta = string(m)
		}
	}
	if err := a.sonos.AddToQueue(body.URI, meta); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]string{"status": "ok"})
}

func (a *API) handleClearQueue(w http.ResponseWriter, r *http.Request) {
	if err := a.sonos.ClearQueue(); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]string{"status": "ok"})
}

// handleQueueItem routes /queue/:position and /queue/:position/play.
func (a *API) handleQueueItem(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/queue/")
	parts := strings.SplitN(path, "/", 2)

	pos, err := strconv.Atoi(parts[0])
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid position")
		return
	}

	if len(parts) == 2 && parts[1] == "play" && r.Method == http.MethodPost {
		if err := a.sonos.PlayFromQueue(pos); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, map[string]string{"status": "ok"})
		return
	}

	if r.Method == http.MethodDelete {
		if err := a.sonos.RemoveFromQueue(pos); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, map[string]string{"status": "ok"})
		return
	}

	writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
}

func (a *API) handleReorderQueue(w http.ResponseWriter, r *http.Request) {
	var body struct {
		From int `json:"from"`
		To   int `json:"to"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if err := a.sonos.ReorderQueue(body.From, body.To); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]string{"status": "ok"})
}

// handleBatchQueue adds multiple local tracks to the queue by relative path.
func (a *API) handleBatchQueue(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Paths []string `json:"paths"`
	}
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}

	added := 0
	for _, p := range body.Paths {
		t, ok := a.lib.TrackByPath(p)
		if !ok {
			continue
		}
		fileURI := a.lib.TrackFileURI(a.lanIP, a.filePort, t)
		artURI := ""
		if t.ArtHash != "" {
			artURI = fmt.Sprintf("http://%s:%d/art/%s", a.lanIP, a.filePort, t.ArtHash)
		}
		metadata := BuildDIDLLite(t, fileURI, artURI)
		if err := a.sonos.AddToQueue(fileURI, metadata); err != nil {
			log.Printf("batch queue add %s: %v", p, err)
			continue
		}
		added++
	}

	snap := a.state.Snapshot()
	writeJSON(w, map[string]any{
		"added":        added,
		"queue_length": len(snap.Queue),
	})
}

// ── Library ───────────────────────────────────────────────────────────────────

func (a *API) handleLibraryRoot(w http.ResponseWriter, r *http.Request) {
	entries, err := a.lib.Browse("/")
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]any{"path": "/", "entries": entries})
}

func (a *API) handleLibrarySearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	results := a.lib.SearchTracks(q)
	writeJSON(w, results)
}

func (a *API) handleLibraryPath(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		if strings.Contains(r.URL.Path, "/library/search") {
			a.handleLibrarySearch(w, r)
			return
		}
		relPath := strings.TrimPrefix(r.URL.Path, "/library")
		if relPath == "" {
			relPath = "/"
		}
		entries, err := a.lib.Browse(relPath)
		if err != nil {
			writeErr(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSON(w, map[string]any{"path": relPath, "entries": entries})
		return
	}
	writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
}

// ── Albums ────────────────────────────────────────────────────────────────────

func (a *API) handleGetAlbums(w http.ResponseWriter, r *http.Request) {
	albums := a.lib.Albums()
	// Strip Tracks from list response for brevity.
	type albumSummary struct {
		ID         string `json:"id"`
		Title      string `json:"title"`
		Artist     string `json:"artist"`
		Year       int    `json:"year"`
		TrackCount int    `json:"track_count"`
		ArtHash    string `json:"art_hash"`
		Path       string `json:"path"`
	}
	result := make([]albumSummary, len(albums))
	for i, al := range albums {
		result[i] = albumSummary{
			ID:         al.ID,
			Title:      al.Title,
			Artist:     al.Artist,
			Year:       al.Year,
			TrackCount: al.TrackCount,
			ArtHash:    al.ArtHash,
			Path:       al.Path,
		}
	}
	writeJSON(w, result)
}

func (a *API) handleAlbumsSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	albums := a.lib.SearchAlbums(q)
	writeJSON(w, albums)
}

func (a *API) handleAlbumByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/albums/")
	if id == "" || strings.Contains(id, "/") {
		writeErr(w, http.StatusBadRequest, "invalid album id")
		return
	}
	al, ok := a.lib.AlbumByID(id)
	if !ok {
		writeErr(w, http.StatusNotFound, "album not found")
		return
	}
	writeJSON(w, al)
}

// ── Art ───────────────────────────────────────────────────────────────────────

func (a *API) handleArt(w http.ResponseWriter, r *http.Request) {
	hash := strings.TrimPrefix(r.URL.Path, "/art/")
	// Strip any file extension.
	hash = strings.TrimSuffix(hash, filepath.Ext(hash))
	if hash == "" {
		http.NotFound(w, r)
		return
	}
	data := a.lib.GetArt(hash)
	if data == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Write(data) //nolint:errcheck
}
