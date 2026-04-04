# sonotui — Claude Code Handoff Spec
**Version**: 1.1 (v0.1 scope)
**Language**: Go
**TUI Framework**: Bubbletea + lipgloss (Charm ecosystem)
**Target**: CachyOS / Linux, kitty/sixel-capable terminal preferred

---

## 0. Before You Write Any Code

Clone the following reference repository. Do not fork it — use it as reference only for SSDP discovery and SOAP call patterns:

```bash
git clone https://github.com/steipete/sonoscli /tmp/sonoscli-ref
```

Read the following files from that repo before writing any Go:
- `/tmp/sonoscli-ref/sonos/soap.go` — SOAP HTTP call pattern
- `/tmp/sonoscli-ref/sonos/discovery.go` — SSDP discovery
- `/tmp/sonoscli-ref/sonos/speaker.go` — speaker struct and UPnP actions

You will write your own implementation. These files show working SOAP/SSDP patterns already tested against real Sonos hardware.

---

## 1. Project Structure

```
sonotui/
├── main.go
├── go.mod
├── go.sum
├── sonos/
│   ├── discovery.go      # SSDP speaker discovery
│   ├── soap.go           # raw SOAP/HTTP call helper
│   ├── transport.go      # AVTransport actions (play/pause/stop/next/prev/line-in)
│   ├── rendering.go      # RenderingControl actions (volume/mute)
│   ├── info.go           # GetPositionInfo (called once per track change)
│   ├── events.go         # GENA subscription lifecycle + local HTTP notify server
│   └── types.go          # shared types: Speaker, TrackInfo, TransportState
├── tui/
│   ├── model.go          # Bubbletea Model, Init, Update, View
│   ├── keys.go           # keybindings
│   ├── styles.go         # lipgloss styles
│   └── art.go            # album art sixel/kitty rendering
└── config/
    └── config.go         # persist last-used speaker IP to XDG_CONFIG_HOME
```

---

## 2. Dependencies (`go.mod`)

```
module github.com/yourusername/sonotui

go 1.22

require (
    github.com/charmbracelet/bubbletea  v0.27.0
    github.com/charmbracelet/lipgloss   v0.13.0
    github.com/charmbracelet/bubbles    v0.20.0
    github.com/koron/go-ssdp            v0.0.4
    github.com/BourgeoisBear/rasterm    v1.1.1
    github.com/dhowden/tag              v0.0.0-20240417053706-3d75bec8d897
)
```

Run `go mod tidy` after setup.

---

## 3. Architecture Overview

sonotui uses a **hybrid event-driven + local counter** architecture. There is no periodic polling of the Sonos speaker for state.

```
┌──────────────────────────────────────────────────────────────┐
│  sonotui process                                             │
│                                                              │
│  ┌──────────────────────┐    GENA NOTIFY (HTTP push)        │
│  │  Local HTTP server   │◄───────────────────────  Sonos    │
│  │  <lan_ip>:<port>     │    - track changed                │
│  │  /notify             │    - play/pause/stop              │
│  └────────┬─────────────┘    - volume changed               │
│           │ tea.Msg          - source changed (line-in)      │
│           ▼                                                  │
│  ┌──────────────────────┐                                    │
│  │   Bubbletea model    │◄── 1s tick (UI only, no network)  │
│  │                      │    increments local position       │
│  │   on track change:   │    counter by 1s when PLAYING     │
│  │   GetPositionInfo    │                                    │
│  │   once → duration    │                                    │
│  │   + start position   │                                    │
│  └──────────────────────┘                                    │
└──────────────────────────────────────────────────────────────┘
```

**What triggers network calls:**
- App startup: SSDP discovery → SUBSCRIBE to 3 services → initial GENA NOTIFY populates state (no separate state SOAP queries needed)
- GENA track change event: one `GetPositionInfo` call to get duration + start position
- GENA resume (PAUSED→PLAYING) event: one `GetPositionInfo` call to resync elapsed
- User command (play/pause/stop/vol/next/prev/line-in): one outbound SOAP call
- Subscription renewal: one SUBSCRIBE call per service every ~30 minutes
- App exit: UNSUBSCRIBE from all 3 services

**What never triggers network calls:**
- The 1s UI tick (pure integer increment)
- Re-renders triggered by keystrokes that don't issue commands

---

## 4. Sonos UPnP — SOAP Reference

All Sonos control is plain HTTP POST to port 1400. No authentication required on LAN.

### 4.1 SOAP helper

```go
// POST to http://<ip>:1400/<path>
// Headers:
//   Content-Type: text/xml; charset="utf-8"
//   SOAPACTION: "urn:schemas-upnp-org:service:<serviceType>:<version>#<action>"
// Timeout: 3 seconds.
// Returns: response body bytes, error
func soapCall(ip, path, serviceType string, version int, action string, args map[string]string) ([]byte, error)
```

### 4.2 Endpoint paths

| Service                   | Path                                         |
|---------------------------|----------------------------------------------|
| AVTransport               | `/MediaRenderer/AVTransport/Control`         |
| RenderingControl          | `/MediaRenderer/RenderingControl/Control`    |
| AVTransport events        | `/MediaRenderer/AVTransport/Event`           |
| RenderingControl events   | `/MediaRenderer/RenderingControl/Event`      |
| ZoneGroupTopology events  | `/ZoneGroupTopology/Event`                   |

### 4.3 Required SOAP actions

#### Play
```
Service: AVTransport:1 | Action: Play
Args: InstanceID=0, Speed=1
```

#### Pause
```
Service: AVTransport:1 | Action: Pause
Args: InstanceID=0
```

#### Stop
```
Service: AVTransport:1 | Action: Stop
Args: InstanceID=0
```

#### Next
```
Service: AVTransport:1 | Action: Next
Args: InstanceID=0
```

#### Previous
```
Service: AVTransport:1 | Action: Previous
Args: InstanceID=0
```

#### Switch to Line-In
```
Service: AVTransport:1 | Action: SetAVTransportURI
Args:
  InstanceID=0
  CurrentURI=x-rincon-stream:RINCON_<uuid>1400
  CurrentURIMetaData=<empty string>
```
The UUID is the speaker's RINCON ID from SSDP/device_description.xml.
`x-rincon-stream:` is the Sonos-specific line-in URI scheme.

#### GetPositionInfo
```
Service: AVTransport:1 | Action: GetPositionInfo
Args: InstanceID=0
Returns XML containing:
  TrackDuration  — "H:MM:SS" — parse to total seconds
  RelTime        — "H:MM:SS" — parse to elapsed seconds
  TrackMetaData  — DIDL-Lite XML string (entity-escaped, decode before parsing)
```

Parse `TrackMetaData` DIDL-Lite for:
- `dc:title`
- `dc:creator` (artist)
- `upnp:album`
- `upnp:albumArtURI` (may be path-only; prepend `http://<speaker_ip>:1400` if no host)

#### GetVolume (startup only — volume changes arrive via GENA after that)
```
Service: RenderingControl:1 | Action: GetVolume
Args: InstanceID=0, Channel=Master
Returns: CurrentVolume (integer 0–100)
```

#### SetVolume
```
Service: RenderingControl:1 | Action: SetVolume
Args: InstanceID=0, Channel=Master, DesiredVolume=<0–100>
```

---

## 5. SSDP Discovery

Use `github.com/koron/go-ssdp` to send M-SEARCH for:
```
ST: urn:schemas-upnp-org:device:ZonePlayer:1
```

Each result gives a `Location` URL. Fetch `http://<ip>:1400/xml/device_description.xml` to extract:
- `<friendlyName>` — room name (e.g. "Living Room")
- `<UDN>` — format `uuid:RINCON_000E58XXXX001400` — strip the `uuid:` prefix

```go
type Speaker struct {
    IP           string   // "192.168.1.42"
    UUID         string   // "RINCON_000E58XXXX001400"
    FriendlyName string   // "Living Room"
}
```

Run discovery as a `tea.Cmd` goroutine at startup. Stream results via `speakersDiscoveredMsg` as they arrive. Do not block startup on discovery completing.

---

## 6. GENA Event Subscription

### 6.1 Local HTTP notify server

Start a `net/http` server bound to the machine's **LAN IP** (not loopback — Sonos needs to reach it) on a random available port in range 34500–34599.

```go
type NotifyServer struct {
    LanIP   string
    Port    int
    EventCh chan GENAEvent  // buffered, cap 32
}

type GENAEvent struct {
    SID     string   // subscription ID from Sonos
    Service string   // "AVTransport" | "RenderingControl" | "ZoneGroupTopology"
    Body    []byte   // raw NOTIFY body XML
}
```

The single `/notify` handler receives HTTP NOTIFY requests, reads the body, sends a `GENAEvent` to `EventCh`. A goroutine consumes the channel, parses the XML, and calls `program.Send()` with appropriate `tea.Msg` types.

### 6.2 Subscribe to services

Subscribe to these three services after the active speaker is selected:

| Service           | Event endpoint                          |
|-------------------|-----------------------------------------|
| AVTransport       | `/MediaRenderer/AVTransport/Event`      |
| RenderingControl  | `/MediaRenderer/RenderingControl/Event` |
| ZoneGroupTopology | `/ZoneGroupTopology/Event`              |

Subscribe request format:
```
SUBSCRIBE /MediaRenderer/AVTransport/Event HTTP/1.1
Host: <speaker_ip>:1400
CALLBACK: <http://<lan_ip>:<notify_port>/notify>
NT: upnp:event
Timeout: Second-3600
```

Store the `SID` response header per service for renewal and unsubscribe.

Sonos sends an **immediate NOTIFY** after subscription containing full current state. Use this initial event to populate the model — do not make separate SOAP queries for initial state (except `GetVolume` if the initial RenderingControl NOTIFY doesn't arrive within 500ms).

### 6.3 Subscription renewal

Renew each subscription at 1800s (half of the 3600s timeout):
```
SUBSCRIBE /MediaRenderer/AVTransport/Event HTTP/1.1
Host: <speaker_ip>:1400
SID: <stored_sid>
Timeout: Second-3600
```
Use `time.AfterFunc(1800 * time.Second, renewFunc)` per subscription. Reset the timer on each successful renewal.

### 6.4 Unsubscribe on exit

Register signal handlers for `SIGINT` and `SIGTERM`. On exit, send UNSUBSCRIBE for all active SIDs:
```
UNSUBSCRIBE /MediaRenderer/AVTransport/Event HTTP/1.1
Host: <speaker_ip>:1400
SID: <stored_sid>
```

**This is mandatory.** Failing to unsubscribe can disrupt the official Sonos app's own subscriptions. If UNSUBSCRIBE fails (network error), log and continue exiting — do not hang.

### 6.5 Parsing AVTransport LastChange

The NOTIFY body contains a `LastChange` property. Its value is an XML-encoded string — entity-decode it first, then parse the inner XML.

Key fields to extract:

```xml
<TransportState val="PLAYING"/>
<!-- PLAYING | PAUSED_PLAYBACK | STOPPED | TRANSITIONING -->

<CurrentTrackURI val="x-rincon-stream:RINCON_..."/>
<!-- prefix x-rincon-stream: → line-in active -->

<CurrentTrackMetaData val="..."/>
<!-- entity-encoded DIDL-Lite: decode then parse for title/artist/album/art -->

<CurrentTrackDuration val="0:05:32"/>
<!-- parse to seconds; 0 or "NOT_IMPLEMENTED" means unavailable -->
```

**On `TransportState` change to `PLAYING` from `PAUSED_PLAYBACK` or `STOPPED`:**
→ dispatch `tea.Cmd` calling `GetPositionInfo` once to resync elapsed position

**On `CurrentTrackURI` change:**
→ dispatch `tea.Cmd` calling `GetPositionInfo` once to get new duration + start position
→ if `albumArtURI` changed, dispatch art fetch

**On `CurrentTrackURI` starts with `x-rincon-stream:`:**
→ set `model.isLineIn = true`
→ clear track info fields
→ skip progress bar and art fetch

### 6.6 Parsing RenderingControl LastChange

```xml
<Volume channel="Master" val="42"/>
```

Extract `val` and update `model.volume` directly. No outbound SOAP call needed.

---

## 7. Position Tracking

```go
type PositionState struct {
    duration int       // total track length in seconds
    elapsed  int       // current elapsed seconds (locally maintained)
    playing  bool      // true when transport state is PLAYING
}
```

**On track change (new `CurrentTrackURI` GENA event):**
```
dispatch GetPositionInfo → positionSyncedMsg{elapsed, duration}
on receipt: pos.elapsed = elapsed; pos.duration = duration; pos.playing = (state==PLAYING)
```

**On resume (PAUSED→PLAYING GENA event):**
```
dispatch GetPositionInfo → positionSyncedMsg{elapsed, duration}
on receipt: pos.elapsed = elapsed; pos.playing = true
```

**On pause or stop (GENA event):**
```
pos.playing = false
(no network call — elapsed freezes at current value)
```

**On 1s UI tick:**
```go
if model.pos.playing && !model.isLineIn {
    model.pos.elapsed++
    if model.pos.elapsed > model.pos.duration && model.pos.duration > 0 {
        model.pos.elapsed = model.pos.duration
    }
}
```

**Display formatting:**
```go
func formatDuration(s int) string {
    return fmt.Sprintf("%d:%02d", s/60, s%60)
}
// "2:14 / 5:32"
```

**Line-in:** `isLineIn == true` → do not render progress bar. Render `── live ──` instead.

---

## 8. Bubbletea Model

```go
type Model struct {
    // Speakers
    speakers      []sonos.Speaker
    activeSpeaker int

    // Subscriptions
    subscriptions map[string]string  // service name → SID
    notify        *sonos.NotifyServer

    // Playback state (updated by GENA events)
    trackInfo     sonos.TrackInfo
    transport     string             // PLAYING | PAUSED_PLAYBACK | STOPPED | TRANSITIONING
    volume        int                // 0–100
    isLineIn      bool

    // Position (local counter)
    pos           PositionState

    // UI state
    status        string
    statusExpiry  time.Time
    width         int
    height        int
    artRendered   string             // pre-rendered escape string from rasterm
    artURL        string             // URL of currently rendered art (change detection)

    // Misc
    discovering   bool
}

type TrackInfo struct {
    Title  string
    Artist string
    Album  string
    ArtURL string
}
```

### 8.1 tea.Msg types

```go
type tickMsg            time.Time
type speakersDiscoveredMsg []sonos.Speaker
type genaEventMsg       sonos.GENAEvent
type positionSyncedMsg  struct{ elapsed, duration int }
type artFetchedMsg      struct{ url string; data []byte }
type statusClearMsg     struct{}
type errMsg             error
```

### 8.2 Init

```go
func (m Model) Init() tea.Cmd {
    return tea.Batch(
        discoverSpeakers(),
        startNotifyServer(),
        tickCmd(),
    )
}
```

Subscriptions are initiated after `speakersDiscoveredMsg` arrives and active speaker is set.

### 8.3 Key bindings

```
space        toggle play/pause
s            stop
k / up       volume +5
j / down     volume -5
K            volume +1
J            volume -1
l            switch to line-in (shows "Already on Line-In" status if isLineIn)
< or ,       previous track
> or .       next track
tab          cycle to next speaker (UNSUBSCRIBE current, SUBSCRIBE new)
r            re-run discovery
q / ctrl+c   quit → UNSUBSCRIBE all → exit
```

All UPnP commands dispatch as non-blocking `tea.Cmd` goroutines. After a SetVolume command do not additionally call GetVolume — the RenderingControl GENA event will update the model within ~200ms.

### 8.4 Tab / room switching

1. Send UNSUBSCRIBE to all 3 services on current speaker
2. Increment `activeSpeaker` (wrap)
3. Send SUBSCRIBE to all 3 services on new speaker
4. Initial NOTIFY from new speaker repopulates state

---

## 9. TUI Layout

### Full layout (≥ 80 columns)

```
┌─────────────────────────────────────────────────────────────┐
│  sonotui              Living Room        ● PLAYING   vol:42 │
├───────────────┬─────────────────────────────────────────────┤
│               │  Thelonious Monk                            │
│  [album art]  │  Round Midnight                             │
│  sixel/kitty  │  Brilliant Corners                          │
│  or box glyph │                                             │
│               │  ████████████░░░░░░░░  2:14 / 5:32         │
├───────────────┴─────────────────────────────────────────────┤
│  [spc] play/pause  [s] stop  [l] line-in  [tab] room        │
│  [j/k] vol  [</>] prev/next  [r] discover  [q] quit         │
│  <ephemeral status — auto-clears after 2s>                  │
└─────────────────────────────────────────────────────────────┘
```

### Narrow layout (< 80 columns)

Hide art pane. Full-width metadata and progress bar.

### Line-in mode

```
│  Line-In                                                    │
│  analog source                                              │
│                                                             │
│  ── live ──                                                 │
```

No progress bar. Art pane shows placeholder box glyph.

### Status bar (top right)

`Living Room   ● PLAYING   vol:42`

Transport state colors (256-color):
- `● PLAYING` → `#5fd700` (green, 82)
- `⏸ PAUSED` → `#ffd700` (yellow, 226)
- `■ STOPPED` → `#585858` (gray, 240)
- `⟳ TRANSITIONING` → `#5fafd7` (blue, 39)

### Progress bar

```
████████████░░░░░░░░  2:14 / 5:32
```

Width scales to available columns. Rendered only when `!isLineIn && pos.duration > 0`.

---

## 10. Album Art

Use `github.com/BourgeoisBear/rasterm`.

```go
func detectProtocol() Protocol {
    switch {
    case os.Getenv("TERM") == "xterm-kitty",
         os.Getenv("TERM_PROGRAM") == "kitty":
        return ProtocolKitty
    case strings.Contains(os.Getenv("TERM"), "sixel"):
        return ProtocolSixel
    default:
        return ProtocolNone
    }
}
```

Art URL from DIDL-Lite `upnp:albumArtURI` — prepend `http://<speaker_ip>:1400` if path-only.

Only re-fetch and re-render when `artURL` changes. On fetch failure or `ProtocolNone`, show:
```
╭──────────╮
│    ♫     │
╰──────────╯
```

---

## 11. Styles

```go
var (
    titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15"))
    artistStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("250"))
    albumStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Italic(true)
    statusPlaying = lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
    statusPaused  = lipgloss.NewStyle().Foreground(lipgloss.Color("226"))
    statusStopped = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
    statusTrans   = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
    volStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
    helpStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
    liveStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("208")).Bold(true)
    progressFill  = lipgloss.NewStyle().Foreground(lipgloss.Color("62"))
    progressEmpty = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
    ephemeralMsg  = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
    borderStyle   = lipgloss.NewStyle().
                       Border(lipgloss.RoundedBorder()).
                       BorderForeground(lipgloss.Color("62"))
)
```

---

## 12. Error Handling

- **SOAP error / timeout (3s)**: log to `$XDG_CACHE_HOME/sonotui/debug.log`, show brief ephemeral status, retry on next user action or event
- **GENA parse error**: log and discard the event, do not crash
- **No speakers found**: show "No Sonos speakers found — press [r] to retry"
- **Single speaker found**: suppress Tab / room selector UI entirely
- **Art fetch failure**: silently show placeholder, no user-visible error
- **UNSUBSCRIBE failure on exit**: log, continue exiting immediately

---

## 13. Configuration

`$XDG_CONFIG_HOME/sonotui/config.toml`:

```toml
[speaker]
last_ip   = "192.168.1.42"
last_name = "Living Room"
```

On startup: attempt to subscribe to saved IP immediately while SSDP discovery runs in background. If saved IP is unreachable within 2s, fall back to discovery results.

---

## 14. Build & Run

```bash
go build -o sonotui ./...
./sonotui
```

Optional flags:
```
--speaker <ip>   skip discovery, connect to this IP directly
--port <n>       override notify server port (default: auto 34500–34599)
--debug          enable debug logging
```

Single static binary. No runtime dependencies.

---

## 15. Out of Scope for v0.1 — Do NOT Implement

- Pushing local files to Sonos queue
- MPD bridge or sync mode
- Queue browsing or editing UI
- Streaming service integration
- Multi-room grouping controls
- Alarms, EQ, sleep timer
- Any cloud API or authentication flow

Do not add stubs or TODOs for these. Keep the codebase minimal.

---

## 16. Definition of Done (v0.1)

- [ ] SSDP discovers all Sonos speakers on the LAN
- [ ] Room can be cycled with Tab; subscriptions transfer cleanly to new speaker
- [ ] GENA subscriptions established for AVTransport, RenderingControl, ZoneGroupTopology
- [ ] UNSUBSCRIBE sent on exit for all active SIDs (SIGINT, SIGTERM, q)
- [ ] Transport state (play/pause/stop) updates instantly from GENA — no polling
- [ ] Volume updates instantly from GENA — no polling
- [ ] Now-playing shows title, artist, album from DIDL-Lite metadata
- [ ] Progress bar renders and increments locally every second (pure UI math)
- [ ] Progress resyncs from GetPositionInfo on track change and resume-from-pause
- [ ] Line-in: detected from `x-rincon-stream:` URI prefix
- [ ] Line-in: shows "Line-In / analog source / live" — no progress bar
- [ ] `l` switches to line-in; shows ephemeral "Already on Line-In" if already active
- [ ] Volume: j/k = ±5, J/K = ±1, clamped 0–100
- [ ] Space toggles play/pause; s stops; < and > send prev/next
- [ ] Album art renders via kitty or sixel on supported terminals
- [ ] Graceful placeholder box on unsupported terminals
- [ ] Speaker unreachable handled gracefully without crashing
- [ ] Single binary, no runtime dependencies
- [ ] Tested on CachyOS with kitty terminal
