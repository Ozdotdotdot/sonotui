# Configuration Reference

sonotui has two separate components, each with its own config file.

---

## Daemon (`sonotuid`)

### Config file

**Location:** `~/.config/sonotuid/config.toml`
(override with `-config /path/to/file`)

```toml
[server]
api_port     = 8989
file_port    = 8990
lan_ip       = ""
display_name = ""

[library]
path  = "~/Music"
cache = "~/.cache/sonotuid/library.json"

[sonos]
preferred_speaker = ""
```

### Fields

#### `[server]`

| Key | Default | Description |
|---|---|---|
| `api_port` | `8989` | Port for the REST API and SSE event stream |
| `file_port` | `8990` | Port for the file server that streams music and art to Sonos |
| `lan_ip` | auto-detected | IP address used in file URIs sent to Sonos speakers. Auto-detected from your network interfaces if unset. Set explicitly if auto-detection picks the wrong interface. |
| `display_name` | system hostname | Name advertised over mDNS. Shown in the TUI picker and iOS discovery list when multiple daemons are present. |

#### `[library]`

| Key | Default | Description |
|---|---|---|
| `path` | `~/Music` | Root directory of your music library. Scanned recursively on startup and on `POST /library/rescan`. |
| `cache` | `~/.cache/sonotuid/library.json` | Path to the library metadata cache. Delete to force a full rescan from scratch. |

#### `[sonos]`

| Key | Default | Description |
|---|---|---|
| `preferred_speaker` | — | UUID of the Sonos speaker to activate on startup. Find UUIDs via `GET /speakers`. If unset, the daemon connects to whichever speaker responds first. |

### CLI flags

| Flag | Default | Description |
|---|---|---|
| `-config` | `~/.config/sonotuid/config.toml` | Path to config file |
| `-debug` | `false` | Enable verbose logging with microsecond timestamps |
| `-install` | `false` | Install as a system service and exit. On Linux, writes `~/.config/systemd/user/sonotuid.service` and enables it via systemctl. On macOS, writes `~/Library/LaunchAgents/com.ozdotdotdot.sonotuid.plist` and loads it via launchctl. Logs on macOS go to `/tmp/sonotuid.log`. |

### mDNS advertisement

The daemon advertises itself on the local network as a `_sonogui._tcp` Bonjour/mDNS
service. This is how the TUI and iOS companion app discover it automatically without
manual IP configuration.

The advertised name is `display_name` from config, falling back to the system hostname.
You can verify the advertisement is working from another machine:

```sh
# Linux (avahi)
avahi-browse _sonogui._tcp

# macOS
dns-sd -B _sonogui._tcp local.
```

---

## TUI (`sonotui`)

### Config file

**Location:** `~/.config/sonotui/config.toml`
(respects `$XDG_CONFIG_HOME` if set)

```toml
host = "100.64.x.x"
port = 8989
```

This file is written automatically when you select a daemon from the startup
picker. You generally do not need to edit it manually — see [Discovery](#discovery).

### Fields

| Key | Default | Description |
|---|---|---|
| `host` | — | Daemon hostname or IP address |
| `port` | `8989` | Daemon port |

### CLI flags

| Flag | Default | Description |
|---|---|---|
| `--host` | — | Daemon host. Bypasses discovery and config entirely when set. |
| `--port` | — | Daemon port. Bypasses discovery when set alongside `--host`. |
| `--art` | `auto` | Album art rendering mode. See [Art modes](#art-modes). |
| `--no-discover` | `false` | Skip mDNS discovery; fall back to config file or `127.0.0.1:8989`. |

### Discovery

On first launch (no saved config), the TUI scans the local network for
`_sonogui._tcp` services for 300ms:

- **One daemon found** — connects automatically, no prompt.
- **Multiple daemons found** — shows a picker. Your selection is saved to the
  config file and used on all subsequent launches without re-scanning.
- **None found** — falls back to `127.0.0.1:8989`.

Once a host is saved to config, mDNS discovery is skipped entirely on future
launches for instant startup.

**Mesh VPN preference:** When a daemon is reachable via both a LAN address
(`192.168.x.x`) and a mesh VPN address (Tailscale, Netbird, Headscale — all use
the `100.64.0.0/10` CGNAT range), the VPN address is preferred. VPN addresses are
stable across network changes and work whether you're at home or remote. Plain LAN
addresses are used for everyone without a mesh VPN.

To force a specific address regardless of discovery, use `--host`:

```sh
sonotui --host 192.168.1.10
```

To re-run discovery and update the saved config, delete the config file:

```sh
rm ~/.config/sonotui/config.toml
sonotui
```

### Art modes

| Mode | Description |
|---|---|
| `auto` | Detect terminal capabilities automatically. Uses Kitty protocol in Kitty and Ghostty; falls back to Unicode half-blocks elsewhere. |
| `kitty` | Force Kitty graphics protocol (true-colour pixel art). |
| `halfblock` | Force Unicode half-block approximation. Works in any true-colour terminal. |
| `none` | Disable album art entirely. |
