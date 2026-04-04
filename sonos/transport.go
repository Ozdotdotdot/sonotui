package sonos

import "fmt"

const (
	avTransportPath    = "/MediaRenderer/AVTransport/Control"
	avTransportService = "AVTransport"
	avTransportVersion = 1
)

// Play resumes playback on the speaker.
func Play(ip string) error {
	_, err := soapCall(ip, avTransportPath, avTransportService, avTransportVersion, "Play", map[string]string{
		"InstanceID": "0",
		"Speed":      "1",
	})
	return err
}

// Pause pauses playback on the speaker.
func Pause(ip string) error {
	_, err := soapCall(ip, avTransportPath, avTransportService, avTransportVersion, "Pause", map[string]string{
		"InstanceID": "0",
	})
	return err
}

// Stop stops playback on the speaker.
func Stop(ip string) error {
	_, err := soapCall(ip, avTransportPath, avTransportService, avTransportVersion, "Stop", map[string]string{
		"InstanceID": "0",
	})
	return err
}

// Next skips to the next track.
func Next(ip string) error {
	_, err := soapCall(ip, avTransportPath, avTransportService, avTransportVersion, "Next", map[string]string{
		"InstanceID": "0",
	})
	return err
}

// Previous goes back to the previous track.
func Previous(ip string) error {
	_, err := soapCall(ip, avTransportPath, avTransportService, avTransportVersion, "Previous", map[string]string{
		"InstanceID": "0",
	})
	return err
}

// SwitchToLineIn sets the transport URI to the speaker's analog line-in input.
// uuid is the full RINCON_xxx ID from the Speaker struct.
func SwitchToLineIn(ip, uuid string) error {
	uri := fmt.Sprintf("x-rincon-stream:%s", uuid)
	_, err := soapCall(ip, avTransportPath, avTransportService, avTransportVersion, "SetAVTransportURI", map[string]string{
		"InstanceID":          "0",
		"CurrentURI":          uri,
		"CurrentURIMetaData":  "",
	})
	return err
}
