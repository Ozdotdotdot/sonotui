package tui

import (
	"bytes"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"strings"
	"time"

)

// Protocol is the terminal image rendering protocol in use.
type Protocol int

const (
	ProtocolNone Protocol = iota
	ProtocolKitty
	ProtocolSixel
)

// DetectProtocol checks environment variables to determine the image protocol.
func DetectProtocol() Protocol {
	term := os.Getenv("TERM")
	termProg := os.Getenv("TERM_PROGRAM")
	switch {
	// Kitty-native and Kitty-protocol-compatible terminals.
	case term == "xterm-kitty",
		termProg == "kitty",
		termProg == "WezTerm",
		strings.HasPrefix(term, "xterm-ghostty"),
		termProg == "ghostty":
		return ProtocolKitty
	case strings.Contains(term, "sixel"):
		return ProtocolSixel
	default:
		return ProtocolNone
	}
}

// ParseProtocol converts a string flag value ("kitty", "sixel", "none") to a Protocol.
// Returns ProtocolNone and false if the value is unrecognised.
func ParseProtocol(s string) (Protocol, bool) {
	switch strings.ToLower(s) {
	case "kitty":
		return ProtocolKitty, true
	case "sixel":
		return ProtocolSixel, true
	case "none", "":
		return ProtocolNone, true
	default:
		return ProtocolNone, false
	}
}

// ArtPlaceholder is shown when art is unavailable or protocol unsupported.
const ArtPlaceholder = "╭──────────╮\n│    ♫     │\n╰──────────╯"

// FetchAndRenderArt downloads art from url and renders it as colored
// half-block characters that participate correctly in terminal text layout.
// cols and rows are the target terminal cell dimensions of the display area.
// Returns ArtPlaceholder on any error.
func FetchAndRenderArt(url string, proto Protocol, cols, rows int) string {
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
	return renderHalfBlock(img, cols, rows)
}

// renderHalfBlock converts an image to Unicode half-block characters (▀) with
// 24-bit ANSI color. Each terminal cell displays two vertical pixels using
// foreground color (top pixel) and background color (bottom pixel).
func renderHalfBlock(img image.Image, cols, rows int) string {
	if cols <= 0 || rows <= 0 {
		return ArtPlaceholder
	}

	targetH := rows * 2 // two pixel rows per cell row

	// Compute destination size preserving aspect ratio.
	bounds := img.Bounds()
	srcW, srcH := float64(bounds.Dx()), float64(bounds.Dy())
	scale := math.Min(float64(cols)/srcW, float64(targetH)/srcH)
	dstW := int(math.Round(srcW * scale))
	dstH := int(math.Round(srcH * scale))
	if dstW < 1 {
		dstW = 1
	}
	if dstH < 1 {
		dstH = 1
	}
	if dstH%2 != 0 {
		dstH++ // ensure even for clean half-block pairing
	}

	resized := resizeNearest(img, dstW, dstH)
	padLeft := (cols - dstW) / 2

	var buf strings.Builder
	for y := 0; y < dstH; y += 2 {
		if padLeft > 0 {
			buf.WriteString(strings.Repeat(" ", padLeft))
		}
		for x := 0; x < dstW; x++ {
			tr, tg, tb, _ := resized.At(x, y).RGBA()
			var br, bg, bb uint32
			if y+1 < dstH {
				br, bg, bb, _ = resized.At(x, y+1).RGBA()
			}
			fmt.Fprintf(&buf, "\x1b[38;2;%d;%d;%d;48;2;%d;%d;%dm▀",
				tr>>8, tg>>8, tb>>8,
				br>>8, bg>>8, bb>>8)
		}
		buf.WriteString("\x1b[0m")
		if y+2 < dstH {
			buf.WriteByte('\n')
		}
	}
	return buf.String()
}

// resizeNearest performs nearest-neighbor image scaling.
func resizeNearest(src image.Image, dstW, dstH int) *image.RGBA {
	srcBounds := src.Bounds()
	srcW := srcBounds.Dx()
	srcH := srcBounds.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, dstW, dstH))
	for y := 0; y < dstH; y++ {
		srcY := srcBounds.Min.Y + y*srcH/dstH
		for x := 0; x < dstW; x++ {
			srcX := srcBounds.Min.X + x*srcW/dstW
			dst.Set(x, y, src.At(srcX, srcY))
		}
	}
	return dst
}

func fetchURL(url string) ([]byte, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url) //nolint:noctx
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(io.LimitReader(resp.Body, 4<<20))
}
