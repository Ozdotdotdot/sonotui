# Contributing

PRs are welcome. For anything significant, open an issue first so we can agree
on the approach before you invest time in it.

## Prerequisites

- Go 1.22+
- Rust stable (`rustup` recommended)

## Build

```sh
# Daemon
go build ./cmd/sonotuid/

# TUI
cd tui-rs && cargo build
```

## Dev setup

Run the daemon against your local music library:

```sh
./sonotuid -debug
```

In a second terminal, run the TUI pointed at it:

```sh
cd tui-rs && cargo run -- --host 127.0.0.1
```

The `-debug` flag enables verbose logging. The `--host` flag skips mDNS discovery
and connects directly.

## Project structure

| Path | What it is |
|---|---|
| `cmd/sonotuid/` | Daemon entry point, config loading, mDNS advertisement |
| `internal/daemon/` | Daemon core: REST API, Sonos manager, library, SSE broadcaster |
| `tui-rs/` | Rust terminal UI |
| `docs/` | API and configuration reference |
| `scripts/` | Build and install scripts |
| `.github/workflows/` | Release pipeline (builds binaries on tag push) |

## Releasing

Push a version tag and GitHub Actions handles the rest:

```sh
git tag v0.2.0 && git push origin v0.2.0
```

This builds binaries for Linux and macOS (amd64 + arm64) and publishes them as a
GitHub Release. To build locally instead, use `scripts/build-release.sh`.
