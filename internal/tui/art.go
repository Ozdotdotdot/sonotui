package tui

import (
	"bytes"
	"image"
	"image/color/palette"
	"image/draw"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/BourgeoisBear/rasterm"
)

// Protocol is the terminal image rendering protocol in use.
type Protocol int

const (
	ProtocolNone  Protocol = iota
	ProtocolKitty
	ProtocolSixel
)

// DetectProtocol checks environment variables to determine the image protocol.
func DetectProtocol() Protocol {
	term := os.Getenv("TERM")
	termProg := os.Getenv("TERM_PROGRAM")
	switch {
	case term == "xterm-kitty", termProg == "kitty":
		return ProtocolKitty
	case strings.Contains(term, "sixel"):
		return ProtocolSixel
	default:
		return ProtocolNone
	}
}

// ArtPlaceholder is shown when art is unavailable or protocol unsupported.
const ArtPlaceholder = "╭──────────╮\n│    ♫     │\n╰──────────╯"

// FetchAndRenderArt downloads art from url and renders it for the terminal.
// Returns ArtPlaceholder on any error.
func FetchAndRenderArt(url string, proto Protocol) string {
	if proto == ProtocolNone || url == "" {
		return ArtPlaceholder
	}
	data, err := fetchURL(url)
	if err != nil {
		log.Printf("art: fetch %s: %v", url, err)
		return ArtPlaceholder
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		log.Printf("art: decode: %v", err)
		return ArtPlaceholder
	}

	var buf strings.Builder
	switch proto {
	case ProtocolKitty:
		if err := rasterm.KittyWriteImage(&writerAdapter{&buf}, img, rasterm.KittyImgOpts{}); err != nil {
			log.Printf("art: kitty render: %v", err)
			return ArtPlaceholder
		}
	case ProtocolSixel:
		paletted := imageToPaletted(img)
		if err := rasterm.SixelWriteImage(&writerAdapter{&buf}, paletted); err != nil {
			log.Printf("art: sixel render: %v", err)
			return ArtPlaceholder
		}
	}
	return buf.String()
}

func imageToPaletted(src image.Image) *image.Paletted {
	bounds := src.Bounds()
	p := image.NewPaletted(bounds, palette.WebSafe)
	draw.FloydSteinberg.Draw(p, bounds, src, bounds.Min)
	return p
}

type writerAdapter struct{ b *strings.Builder }

func (w *writerAdapter) Write(p []byte) (int, error) { return w.b.Write(p) }

func fetchURL(url string) ([]byte, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url) //nolint:noctx
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(io.LimitReader(resp.Body, 4<<20))
}
