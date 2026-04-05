package daemon

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// FileServer serves FLAC files and embedded art from ~/Music on :8990.
// It is used exclusively by the Sonos speaker (not the TUI).
type FileServer struct {
	musicRoot string
	library   *Library
}

// NewFileServer creates a FileServer.
func NewFileServer(musicRoot string, lib *Library) *FileServer {
	return &FileServer{musicRoot: musicRoot, library: lib}
}

// Handler returns an http.Handler that serves /files/* and /art/:hash.
func (fs *FileServer) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/files/", fs.serveFile)
	mux.HandleFunc("/art/", fs.serveArt)
	return mux
}

func (fs *FileServer) serveFile(w http.ResponseWriter, r *http.Request) {
	// Strip /files/ prefix.
	rel := strings.TrimPrefix(r.URL.Path, "/files/")
	if rel == "" {
		http.NotFound(w, r)
		return
	}
	// Sanitize: prevent directory traversal.
	rel = filepath.Clean("/" + rel)
	rel = strings.TrimPrefix(rel, "/")
	if strings.Contains(rel, "..") {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	abs := filepath.Join(fs.musicRoot, rel)
	f, err := os.Open(abs)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", contentTypeFor(rel))
	http.ServeContent(w, r, info.Name(), info.ModTime(), f)
}

func (fs *FileServer) serveArt(w http.ResponseWriter, r *http.Request) {
	hash := strings.TrimPrefix(r.URL.Path, "/art/")
	if hash == "" {
		http.NotFound(w, r)
		return
	}

	data := fs.library.GetArt(hash)
	if data == nil {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	http.ServeContent(w, r, hash+".jpg", time.Time{}, newBytesReadSeeker(data))
}

func contentTypeFor(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".flac":
		return "audio/flac"
	case ".mp3":
		return "audio/mpeg"
	case ".m4a":
		return "audio/mp4"
	case ".wav":
		return "audio/wav"
	case ".ogg":
		return "audio/ogg"
	default:
		return "application/octet-stream"
	}
}

// FileURI builds the HTTP URI for a local track (called by Sonos).
func FileURI(lanIP string, filePort int, relPath string) string {
	return fmt.Sprintf("http://%s:%d/files/%s", lanIP, filePort, relPath)
}

// ArtURI builds the HTTP URI for embedded art.
func ArtURI(apiHost string, apiPort int, hash string) string {
	return fmt.Sprintf("http://%s:%d/art/%s", apiHost, apiPort, hash)
}

// ── bytes.ReadSeeker shim ──────────────────────────────────────────────────────

type bytesReadSeeker struct {
	data []byte
	pos  int64
}

func newBytesReadSeeker(data []byte) *bytesReadSeeker {
	return &bytesReadSeeker{data: data}
}

func (b *bytesReadSeeker) Read(p []byte) (int, error) {
	if b.pos >= int64(len(b.data)) {
		return 0, fmt.Errorf("EOF")
	}
	n := copy(p, b.data[b.pos:])
	b.pos += int64(n)
	return n, nil
}

func (b *bytesReadSeeker) Seek(offset int64, whence int) (int64, error) {
	var abs int64
	switch whence {
	case 0: // io.SeekStart
		abs = offset
	case 1: // io.SeekCurrent
		abs = b.pos + offset
	case 2: // io.SeekEnd
		abs = int64(len(b.data)) + offset
	default:
		return 0, fmt.Errorf("invalid whence %d", whence)
	}
	if abs < 0 {
		return 0, fmt.Errorf("negative seek position")
	}
	b.pos = abs
	return abs, nil
}
