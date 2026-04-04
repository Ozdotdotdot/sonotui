package sonos

// Speaker represents a discovered Sonos zone player.
type Speaker struct {
	IP           string // "192.168.1.42"
	UUID         string // "RINCON_000E58XXXX001400"
	FriendlyName string // "Living Room"
}

// TrackInfo holds now-playing metadata parsed from DIDL-Lite.
type TrackInfo struct {
	Title  string
	Artist string
	Album  string
	ArtURL string
}

// GENAEvent is a raw UPnP event notification received from a Sonos speaker.
type GENAEvent struct {
	SID     string // subscription ID
	Service string // "AVTransport" | "RenderingControl" | "ZoneGroupTopology"
	Body    []byte // raw NOTIFY body XML
}
