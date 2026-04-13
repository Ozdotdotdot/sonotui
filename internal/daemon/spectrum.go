package daemon

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"math/cmplx"
	"net/url"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/madelynnblue/go-dsp/fft"
)

const (
	spectrumSampleRate = 44100
	spectrumFFTSize    = 2048
	spectrumChunkSec   = 30
	spectrumTickRate   = 20 // Hz
	spectrumMinFreq    = 60.0
	spectrumMaxFreq    = 16000.0
	spectrumDefBands   = 16
)

// SpectrumFrame is the JSON response for GET /spectrum.
type SpectrumFrame struct {
	Bands    []float64 `json:"bands"`
	Elapsed  int       `json:"elapsed"`
	TrackGen uint64    `json:"track_gen"`
	Playing  bool      `json:"playing"`
}

// Spectrum manages real-time FFT analysis of the currently playing local track.
type Spectrum struct {
	mu        sync.RWMutex
	state     *State
	musicRoot string

	// Decoder state (protected by mu).
	trackGen   uint64
	trackPath  string // absolute path to current audio file
	cancel     context.CancelFunc
	pcmBuf     []int16
	chunkStart int // start offset in seconds
	chunkEnd   int // end offset in seconds

	// Precomputed (immutable after init).
	ffmpegPath string
	hannWindow []float64
	frame      SpectrumFrame
}

var (
	resolvedFFmpeg string
	ffmpegOnce     sync.Once
)

func resolveFFmpeg() string {
	ffmpegOnce.Do(func() {
		path, err := exec.LookPath("ffmpeg")
		if err != nil {
			log.Printf("spectrum: ffmpeg not found; spectrum endpoint will be unavailable")
			return
		}
		resolvedFFmpeg = path
	})
	return resolvedFFmpeg
}

// NewSpectrum creates a Spectrum analyzer.
func NewSpectrum(state *State, musicRoot string) *Spectrum {
	s := &Spectrum{
		state:      state,
		musicRoot:  musicRoot,
		ffmpegPath: resolveFFmpeg(),
		hannWindow: makeHannWindow(spectrumFFTSize),
	}
	return s
}

func makeHannWindow(n int) []float64 {
	w := make([]float64, n)
	for i := range w {
		w[i] = 0.5 * (1.0 - math.Cos(2.0*math.Pi*float64(i)/float64(n-1)))
	}
	return w
}

// Frame returns the latest spectrum frame (thread-safe).
func (s *Spectrum) Frame() SpectrumFrame {
	s.mu.RLock()
	f := s.frame
	// Copy the bands slice so the caller gets an independent snapshot.
	if f.Bands != nil {
		cp := make([]float64, len(f.Bands))
		copy(cp, f.Bands)
		f.Bands = cp
	}
	s.mu.RUnlock()
	return f
}

// Run is the main spectrum loop. Call as a goroutine: go spectrum.Run(ctx).
func (s *Spectrum) Run(ctx context.Context) {
	if s.ffmpegPath == "" {
		log.Printf("spectrum: disabled (ffmpeg not available)")
		return
	}
	log.Printf("spectrum: started (FFT=%d, rate=%dHz, chunk=%ds)", spectrumFFTSize, spectrumTickRate, spectrumChunkSec)

	ticker := time.NewTicker(time.Second / spectrumTickRate)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.cancelDecode()
			return
		case <-ticker.C:
			s.tick(ctx)
		}
	}
}

func (s *Spectrum) tick(ctx context.Context) {
	// Snapshot playback state under read lock.
	s.state.RLock()
	elapsed := s.state.Elapsed
	trackGen := s.state.TrackGen
	trackURI := s.state.Track.URI
	playing := s.state.Playing
	isLineIn := s.state.IsLineIn
	s.state.RUnlock()

	// Not playing or line-in: emit a null/zero frame.
	if !playing || isLineIn {
		s.mu.Lock()
		s.frame = SpectrumFrame{
			Bands:    nil,
			Elapsed:  elapsed,
			TrackGen: trackGen,
			Playing:  false,
		}
		s.mu.Unlock()
		return
	}

	// Resolve local file path from URI.
	filePath, isLocal := s.resolveLocalPath(trackURI)
	if !isLocal {
		s.mu.Lock()
		s.frame = SpectrumFrame{
			Bands:    nil,
			Elapsed:  elapsed,
			TrackGen: trackGen,
			Playing:  true,
		}
		s.mu.Unlock()
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Track changed — reset buffer and start decoding from current position.
	if trackGen != s.trackGen || filePath != s.trackPath {
		s.cancelDecode()
		s.trackGen = trackGen
		s.trackPath = filePath
		s.pcmBuf = nil
		s.chunkStart = 0
		s.chunkEnd = 0
		s.loadChunk(ctx, elapsed)
	}

	// Elapsed moved outside buffered range (seek or natural progression).
	if s.pcmBuf != nil && (elapsed < s.chunkStart || elapsed >= s.chunkEnd) {
		s.loadChunk(ctx, elapsed)
	}

	// No buffer available yet — emit zero bands.
	if s.pcmBuf == nil {
		s.frame = SpectrumFrame{
			Bands:    zeroBands(spectrumDefBands),
			Elapsed:  elapsed,
			TrackGen: trackGen,
			Playing:  true,
		}
		return
	}

	// Compute sample offset within the chunk.
	sampleOffset := (elapsed - s.chunkStart) * spectrumSampleRate
	if sampleOffset+spectrumFFTSize > len(s.pcmBuf) {
		sampleOffset = len(s.pcmBuf) - spectrumFFTSize
	}
	if sampleOffset < 0 {
		sampleOffset = 0
	}

	bands := s.computeFFT(s.pcmBuf[sampleOffset:sampleOffset+spectrumFFTSize], spectrumDefBands)

	s.frame = SpectrumFrame{
		Bands:    bands,
		Elapsed:  elapsed,
		TrackGen: trackGen,
		Playing:  true,
	}
}

// loadChunk decodes a chunk starting at startSec. Must be called with s.mu held.
func (s *Spectrum) loadChunk(ctx context.Context, startSec int) {
	s.cancelDecode()

	decodeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	s.cancel = cancel

	pcm, err := s.decodeChunk(decodeCtx, s.trackPath, startSec, spectrumChunkSec)
	if err != nil {
		log.Printf("spectrum: decode error at %ds: %v", startSec, err)
		s.pcmBuf = nil
		return
	}

	s.pcmBuf = pcm
	s.chunkStart = startSec
	s.chunkEnd = startSec + spectrumChunkSec
}

func (s *Spectrum) cancelDecode() {
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
}

// decodeChunk runs ffmpeg to extract raw PCM from the audio file.
func (s *Spectrum) decodeChunk(ctx context.Context, filePath string, startSec, durSec int) ([]int16, error) {
	cmd := exec.CommandContext(ctx, s.ffmpegPath,
		"-ss", strconv.Itoa(startSec),
		"-t", strconv.Itoa(durSec),
		"-i", filePath,
		"-f", "s16le",
		"-ac", "1",
		"-ar", strconv.Itoa(spectrumSampleRate),
		"-v", "error",
		"pipe:1",
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return nil, fmt.Errorf("%w: %s", err, msg)
		}
		return nil, err
	}

	raw := stdout.Bytes()
	if len(raw) < 2 {
		return nil, fmt.Errorf("no audio data decoded")
	}

	samples := make([]int16, len(raw)/2)
	for i := range samples {
		samples[i] = int16(binary.LittleEndian.Uint16(raw[i*2 : i*2+2]))
	}
	return samples, nil
}

// computeFFT applies a Hann window, runs FFT, and bins magnitudes into logarithmic bands.
func (sp *Spectrum) computeFFT(samples []int16, numBands int) []float64 {
	n := len(samples)
	if n < spectrumFFTSize {
		return zeroBands(numBands)
	}

	// Apply Hann window and convert to float64.
	input := make([]float64, n)
	for i, v := range samples {
		input[i] = float64(v) * sp.hannWindow[i]
	}

	// Run FFT.
	spectrum := fft.FFTReal(input)

	// Only the first half of the FFT output is useful (Nyquist).
	halfN := n / 2

	// Compute magnitude for each bin.
	magnitudes := make([]float64, halfN)
	for i := 1; i < halfN; i++ {
		magnitudes[i] = cmplx.Abs(spectrum[i])
	}

	// Bin into logarithmic frequency bands.
	bands := make([]float64, numBands)
	freqPerBin := float64(spectrumSampleRate) / float64(n)

	// Logarithmic band edges.
	logMin := math.Log10(spectrumMinFreq)
	logMax := math.Log10(spectrumMaxFreq)
	logStep := (logMax - logMin) / float64(numBands)

	for b := 0; b < numBands; b++ {
		fLow := math.Pow(10, logMin+float64(b)*logStep)
		fHigh := math.Pow(10, logMin+float64(b+1)*logStep)

		binLow := int(fLow / freqPerBin)
		binHigh := int(fHigh / freqPerBin)
		if binLow < 1 {
			binLow = 1
		}
		if binHigh >= halfN {
			binHigh = halfN - 1
		}
		if binHigh < binLow {
			binHigh = binLow
		}

		// Average magnitude across bins in this band.
		var sum float64
		count := 0
		for i := binLow; i <= binHigh; i++ {
			sum += magnitudes[i]
			count++
		}
		if count > 0 {
			bands[b] = sum / float64(count)
		}
	}

	// Normalize to 0.0-1.0 range.
	var peak float64
	for _, v := range bands {
		if v > peak {
			peak = v
		}
	}
	if peak > 0 {
		for i := range bands {
			bands[i] /= peak
		}
	}

	return bands
}

// resolveLocalPath extracts the absolute file path from a local track URI.
func (s *Spectrum) resolveLocalPath(uri string) (string, bool) {
	if uri == "" {
		return "", false
	}
	u, err := url.Parse(uri)
	if err != nil {
		return "", false
	}
	if !strings.HasPrefix(u.Path, "/files/") {
		return "", false
	}
	relPath, err := url.PathUnescape(strings.TrimPrefix(u.Path, "/files/"))
	if err != nil {
		return "", false
	}
	absPath := filepath.Join(s.musicRoot, relPath)
	return absPath, true
}

func zeroBands(n int) []float64 {
	return make([]float64, n)
}
