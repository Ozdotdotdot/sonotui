package daemon

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBrowseNormalizesNestedPaths(t *testing.T) {
	root := t.TempDir()
	artistDir := filepath.Join(root, "Kanye West")
	albumDir := filepath.Join(artistDir, "Graduation")
	if err := os.MkdirAll(albumDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	trackPath := filepath.Join(albumDir, "01 - Good Morning.flac")
	if err := os.WriteFile(trackPath, []byte("not real flac"), 0o644); err != nil {
		t.Fatalf("write track: %v", err)
	}

	lib := NewLibrary(root, filepath.Join(t.TempDir(), "library.json"))

	rootEntries, err := lib.Browse("/")
	if err != nil {
		t.Fatalf("browse root: %v", err)
	}
	if got, want := len(rootEntries), 1; got != want {
		t.Fatalf("root entries = %d, want %d", got, want)
	}
	if got, want := rootEntries[0].Path, "/Kanye West"; got != want {
		t.Fatalf("root path = %q, want %q", got, want)
	}

	artistEntries, err := lib.Browse(rootEntries[0].Path)
	if err != nil {
		t.Fatalf("browse artist: %v", err)
	}
	if got, want := len(artistEntries), 1; got != want {
		t.Fatalf("artist entries = %d, want %d", got, want)
	}
	if got, want := artistEntries[0].Path, "/Kanye West/Graduation"; got != want {
		t.Fatalf("artist path = %q, want %q", got, want)
	}
}

func TestTrackByPathAcceptsBrowseStylePaths(t *testing.T) {
	lib := NewLibrary("", "")
	lib.tracks = []Track{{Path: "Kanye West/Graduation/01 - Good Morning.flac", Title: "Good Morning"}}

	if _, ok := lib.TrackByPath("/Kanye West/Graduation/01 - Good Morning.flac"); !ok {
		t.Fatal("TrackByPath should accept single-slash browse path")
	}
	if _, ok := lib.TrackByPath("//Kanye West/Graduation/01 - Good Morning.flac"); !ok {
		t.Fatal("TrackByPath should accept duplicate-slash browse path")
	}
}

func TestSearchTracksIncludesDirectoryMatches(t *testing.T) {
	lib := NewLibrary("", "")
	lib.tracks = []Track{
		{
			Path:   "Kanye West/Graduation/01 - Good Morning.flac",
			Title:  "Good Morning",
			Artist: "Kanye West",
			Album:  "Graduation",
		},
	}

	results := lib.SearchTracks("kanye")
	if len(results) == 0 {
		t.Fatal("expected search results")
	}

	for _, result := range results {
		if result.Type == "dir" && result.Path == "/Kanye West" {
			return
		}
	}
	t.Fatal("expected a directory result for /Kanye West")
}
