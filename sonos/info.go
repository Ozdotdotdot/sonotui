package sonos

import (
	"encoding/xml"
	"html"
	"strconv"
	"strings"
)

// PositionInfo holds the result of a GetPositionInfo SOAP call.
type PositionInfo struct {
	Elapsed  int // seconds
	Duration int // seconds
	Track    TrackInfo
}

// GetPositionInfo fetches the current playback position and track metadata.
func GetPositionInfo(ip string) (PositionInfo, error) {
	raw, err := soapCall(ip, avTransportPath, avTransportService, avTransportVersion, "GetPositionInfo", map[string]string{
		"InstanceID": "0",
	})
	if err != nil {
		return PositionInfo{}, err
	}

	fields := parseSOAPFields(raw, "TrackDuration", "RelTime", "TrackMetaData")

	duration := parseDuration(fields["TrackDuration"])
	elapsed := parseDuration(fields["RelTime"])
	track := parseTrackMetaData(ip, fields["TrackMetaData"])

	return PositionInfo{
		Elapsed:  elapsed,
		Duration: duration,
		Track:    track,
	}, nil
}

// parseDuration parses "H:MM:SS" or "MM:SS" into total seconds.
func parseDuration(s string) int {
	s = strings.TrimSpace(s)
	if s == "" || s == "NOT_IMPLEMENTED" {
		return 0
	}
	parts := strings.Split(s, ":")
	var total int
	for _, p := range parts {
		n, _ := strconv.Atoi(p)
		total = total*60 + n
	}
	return total
}

// parseTrackMetaData parses entity-escaped DIDL-Lite XML into a TrackInfo.
func parseTrackMetaData(speakerIP, raw string) TrackInfo {
	if raw == "" {
		return TrackInfo{}
	}
	decoded := html.UnescapeString(raw)
	return parseDIDLLite(speakerIP, []byte(decoded))
}

// ParseDIDLLite parses a DIDL-Lite XML document into TrackInfo.
// Exported so events.go can reuse it.
func ParseDIDLLite(speakerIP string, data []byte) TrackInfo {
	return parseDIDLLite(speakerIP, data)
}

func parseDIDLLite(speakerIP string, data []byte) TrackInfo {
	type didlItem struct {
		Title    string `xml:"title"`
		Creator  string `xml:"creator"`
		Album    string `xml:"album"`
		AlbumArt string `xml:"albumArtURI"`
	}
	type didlRoot struct {
		Items []didlItem `xml:"item"`
	}

	var root didlRoot
	if err := xml.Unmarshal(data, &root); err != nil || len(root.Items) == 0 {
		return TrackInfo{}
	}
	item := root.Items[0]

	artURL := item.AlbumArt
	if artURL != "" && !strings.HasPrefix(artURL, "http") {
		// Path-only — prepend speaker base URL.
		artURL = "http://" + speakerIP + ":1400" + artURL
	}

	return TrackInfo{
		Title:  item.Title,
		Artist: item.Creator,
		Album:  item.Album,
		ArtURL: artURL,
	}
}
