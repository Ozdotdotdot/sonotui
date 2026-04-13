# sonotui

Self-hosted Sonos control. Go daemon + Rust terminal UI with native album art.

![sonotui](docs/NowPlaying.png)

## Why

Sonos speakers are great! Until you stop using a streaming service and realise
you have no good way to play your own music through them. Aux and AirPlay work,
but neither is a proper solution.

sonotui takes direct inspiration from [mpd](https://www.musicpd.org/): a small
daemon runs on a server, serves your local library over HTTP, and pushes tracks
directly to your Sonos speakers. A terminal client — inspired by
[rmpc](https://mierak.github.io/rmpc/) — connects to the daemon from anywhere.

## Features

- **Serves your local library to Sonos:** no streaming service required
- **Native album art:** rendered pixel-perfect via the Kitty graphics protocol in Kitty and Ghostty
- **Vim-style navigation:** `hjkl`, `gg`/`G`, `/` search, `:` commands
- **Real-time updates:** SSE event stream keeps every connected client in sync instantly
- **Zero-config discovery:** daemon advertises over mDNS; the TUI finds it automatically on first launch
- **Album and library browser:** browse by folder or by album metadata, with fuzzy search
- **Queue management:** reorder, remove, jump to track, batch-add directories
- **Real-time spectrum endpoint:** FFT-derived frequency band data for the currently playing track, consumable by external visualizers over the LAN
- **Connection resilience:** the TUI retries unreachable daemons with exponential backoff and falls back to mDNS discovery automatically
- **Fast!** The client is written in Rust, and the daemon in Go!

## Install

**Daemon** — runs on your server (Linux or macOS):

```sh
curl -fsSL https://raw.githubusercontent.com/ozdotdotdot/sonotui/main/scripts/install.sh | sh
sonotuid --install   # register as a system service (systemd on Linux, launchd on macOS)
```

**TUI** — runs on your daily driver (Linux or macOS):

```sh
curl -fsSL https://raw.githubusercontent.com/ozdotdotdot/sonotui/main/scripts/install-tui.sh | sh
sonotui
```

The TUI finds the daemon automatically — no IP addresses to configure.

## How it works

**`sonotuid`** (daemon) runs on an always-on machine. It scans your music library,
serves tracks and album art over HTTP, and exposes a REST API + real-time SSE event
stream on port 8989. It also advertises itself via mDNS so clients can find it without
manual IP configuration.

**`sonotui`** (TUI) connects to the daemon from any terminal. It renders album art
natively using the Kitty graphics protocol in Kitty and Ghostty, falls back to
Unicode half-blocks elsewhere. On first launch it scans the LAN for the daemon and
auto-connects — or shows a picker if it finds more than one.

Both binaries are statically compiled with no runtime dependencies.

## Spectrum endpoint

The daemon exposes a `GET /spectrum` endpoint that returns real-time FFT frequency
band data for the currently playing local track. This is designed for external
visualizers — for example, a Raspberry Pi driving RGB on a keyboard over the LAN.

```sh
curl -s http://your-server:8989/spectrum
```

```json
{
  "bands": [0.04, 0.49, 0.64, 0.79, 1.0, 0.17, 0.14, 0.16, 0.26, 0.08, 0.01, 0.01, 0.01, 0.0, 0.0, 0.0],
  "elapsed": 12,
  "track_gen": 0,
  "playing": true
}
```

- **`bands`**: 16 logarithmic frequency bands (~60 Hz to ~16 kHz), normalized 0.0–1.0
- **`?bands=N`**: request 8–32 bands instead of the default 16
- Returns `null` bands when nothing is playing, during line-in, or for non-local tracks
- Designed for polling at 10–30 Hz; each response is ~200 bytes

Requires `ffmpeg` on the server (installed alongside `ffprobe`, which the daemon
already uses for library scanning).

## Documentation

- [API Reference](docs/api.md) — all REST endpoints and SSE event types
- [Configuration](docs/config.md) — config files, CLI flags, discovery behaviour
- [Contributing](CONTRIBUTING.md) — build setup, dev workflow

## Building from source

```sh
# Daemon
go build ./cmd/sonotuid/

# TUI
cd tui-rs && cargo build --release
```

Requires Go 1.22+, Rust stable, and `ffmpeg`/`ffprobe` on the daemon host.

## Screenshots

| Albums | Library | Vertical layout |
|---|---|---|
| ![Albums](docs/Albums.png) | ![Library](docs/Library.png) | ![Vertical](docs/NowPlayingVertical.png) |
