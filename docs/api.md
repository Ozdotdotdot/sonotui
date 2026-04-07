# sonotuid API Reference

The daemon exposes a REST API on port **8989** (configurable via `api_port`).
All responses are JSON. All endpoints include permissive CORS headers
(`Access-Control-Allow-Origin: *`), so the API is directly usable from browsers
and iOS/Android HTTP clients.

---

## Transport

### `POST /play`
Resume playback on the active speaker.

**Response** `200`
```json
{ "status": "ok" }
```

---

### `POST /pause`
Pause playback.

**Response** `200`
```json
{ "status": "ok" }
```

---

### `POST /stop`
Stop playback.

**Response** `200`
```json
{ "status": "ok" }
```

---

### `POST /next`
Skip to next track in queue.

**Response** `200`
```json
{ "status": "ok" }
```

---

### `POST /prev`
Skip to previous track in queue.

**Response** `200`
```json
{ "status": "ok" }
```

---

### `POST /linein`
Switch the active speaker to its line-in source.

**Response** `200`
```json
{ "status": "ok" }
```

---

### `POST /seek`
Seek to an absolute position in the current track.

**Body**
```json
{ "seconds": 90 }
```

**Response** `200`
```json
{ "status": "ok" }
```

---

### `POST /volume`
Set absolute volume.

**Body**
```json
{ "value": 50 }
```
`value` is clamped to `0–100`.

**Response** `200`
```json
{ "status": "ok" }
```

---

### `POST /volume/relative`
Adjust volume by a signed delta. Clamped to `0–100`.

**Body**
```json
{ "delta": -5 }
```

**Response** `200`
```json
{ "status": "ok" }
```

---

## State

### `GET /status`
Full snapshot of current playback state.

**Response** `200`
```json
{
  "transport":     "PLAYING",
  "track": {
    "title":    "Song Title",
    "artist":   "Artist Name",
    "album":    "Album Name",
    "art_url":  "http://192.168.1.10:8990/art/abc123",
    "duration": 240,
    "uri":      "http://192.168.1.10:8990/files/..."
  },
  "volume":        42,
  "elapsed":       37,
  "duration":      240,
  "is_line_in":    false,
  "speaker": {
    "name": "Living Room",
    "uuid": "RINCON_...",
    "ip":   "192.168.1.20"
  },
  "library_ready": true
}
```

`transport` values: `PLAYING` `PAUSED_PLAYBACK` `STOPPED` `TRANSITIONING`

`speaker` is `null` when no active speaker is selected.

---

### `GET /events`
Server-Sent Events stream. Connect once and receive real-time state updates.

The stream sends a `: ping` comment line every 15 seconds to prevent idle
timeouts on iOS and behind NAT.

**Event format**
```
data: {"type":"transport","state":"PLAYING"}
```

#### Event types

| type | Fields |
|---|---|
| `transport` | `state` (string) |
| `track` | `title` `artist` `album` `art_url` `duration` `uri` |
| `position` | `elapsed` `duration` |
| `volume` | `value` |
| `linein` | `active` (bool) |
| `queue_changed` | — |
| `speaker` | `name` `uuid` |
| `library_scan` | `status` + optional extra fields |
| `error` | `message` |

**Example** (curl)
```sh
curl -N http://localhost:8989/events
```

---

## Speakers

### `GET /speakers`
List all discovered Sonos speakers on the network.

**Response** `200`
```json
[
  { "name": "Living Room", "uuid": "RINCON_...", "ip": "192.168.1.20" },
  { "name": "Kitchen",     "uuid": "RINCON_...", "ip": "192.168.1.21" }
]
```

---

### `POST /speakers/active`
Switch the active speaker by UUID.

**Body**
```json
{ "uuid": "RINCON_..." }
```

**Response** `200`
```json
{ "status": "ok" }
```

---

### `POST /reconnect`
Force the daemon to reconnect to the current active speaker (useful after a
network change or speaker restart).

**Response** `200`
```json
{ "status": "ok" }
```

---

## Queue

### `GET /queue`
Get the current Sonos queue.

**Response** `200`
```json
[
  {
    "title":    "Song Title",
    "artist":   "Artist Name",
    "album":    "Album Name",
    "uri":      "http://...",
    "art_url":  "http://...",
    "duration": 240,
    "position": 1,
    "is_local": true
  }
]
```

---

### `POST /queue`
Add a single track to the queue by URI.

**Body**
```json
{
  "uri":      "http://192.168.1.10:8990/files/path/to/track.flac",
  "metadata": {}
}
```

**Response** `200`
```json
{ "status": "ok" }
```

---

### `DELETE /queue`
Clear the entire queue.

**Response** `200`
```json
{ "status": "ok" }
```

---

### `POST /queue/batch`
Add multiple local tracks or directories to the queue by library-relative path.
Directories are expanded recursively.

**Body**
```json
{ "paths": ["Jazz/Miles Davis", "Blues/Robert Johnson/track.flac"] }
```

**Response** `200`
```json
{ "added": 14, "queue_length": 22 }
```

---

### `POST /queue/reorder`
Move a track within the queue. Positions are 1-indexed.

**Body**
```json
{ "from": 3, "to": 1 }
```

**Response** `200`
```json
{ "status": "ok" }
```

---

### `POST /queue/{position}/play`
Start playback from a specific queue position (1-indexed).

**Response** `200`
```json
{ "status": "ok" }
```

---

### `DELETE /queue/{position}`
Remove a single track from the queue by position (1-indexed).

**Response** `200`
```json
{ "status": "ok" }
```

---

## Library

The library is your local music directory, scanned on startup and on demand.

### `GET /library`
List the root of the music library.

**Response** `200`
```json
{
  "path": "/",
  "entries": [
    { "name": "Jazz",  "path": "/Jazz",  "is_dir": true,  "duration": 0 },
    { "name": "track.flac", "path": "/track.flac", "is_dir": false, "duration": 210 }
  ]
}
```

---

### `GET /library/{path}`
List a subdirectory.

**Response** `200` — same shape as `GET /library`

---

### `POST /library/rescan`
Trigger a background library rescan.

**Response** `200`
```json
{ "status": "started" }
```
Returns `"already_scanning"` if a scan is already in progress.

---

### `GET /library/search?q={query}`
Full-text search across track titles, artists, and albums.

**Response** `200` — array of library entries matching the query.

---

## Albums

Albums are derived from track metadata during library scanning.

### `GET /albums`
List all albums. Track lists are omitted for brevity.

**Response** `200`
```json
[
  {
    "id":          "abc123",
    "title":       "Kind of Blue",
    "artist":      "Miles Davis",
    "year":        1959,
    "track_count": 5,
    "art_hash":    "def456",
    "path":        "/Jazz/Miles Davis/Kind of Blue"
  }
]
```

---

### `GET /albums/{id}`
Full album detail including all tracks.

**Response** `200`
```json
{
  "id":     "abc123",
  "title":  "Kind of Blue",
  "artist": "Miles Davis",
  "year":   1959,
  "tracks": [ ... ]
}
```

---

### `GET /albums/search?q={query}`
Search albums by title or artist.

**Response** `200` — array of full album objects.

---

## Art

### `GET /art/{hash}`
Fetch album art as a JPEG binary. The hash comes from `art_hash` fields in
album and track responses.

**Response** `200`
```
Content-Type: image/jpeg
Cache-Control: public, max-age=86400
<binary JPEG data>
```

Returns `404` if the hash is not found.

The file server on port **8990** also exposes art at `/art/{hash}` — same
content, same hashes. The API proxy at `/art/` exists as a convenience so
clients only need to talk to one port.

---

## Web UI

When the daemon is built with embedded web assets, it serves a web UI:

- `GET /` — serves `index.html`
- `GET /static/*` — serves static assets
