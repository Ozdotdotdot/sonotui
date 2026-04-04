package sonos

import (
	"fmt"
	"strconv"
)

const (
	renderingPath    = "/MediaRenderer/RenderingControl/Control"
	renderingService = "RenderingControl"
	renderingVersion = 1
)

// GetVolume fetches the current master volume (0–100).
func GetVolume(ip string) (int, error) {
	raw, err := soapCall(ip, renderingPath, renderingService, renderingVersion, "GetVolume", map[string]string{
		"InstanceID": "0",
		"Channel":    "Master",
	})
	if err != nil {
		return 0, err
	}
	val := parseSOAPField(raw, "CurrentVolume")
	v, err := strconv.Atoi(val)
	if err != nil {
		return 0, fmt.Errorf("parse volume %q: %w", val, err)
	}
	return v, nil
}

// SetMute sets the master mute state.
func SetMute(ip string, muted bool) error {
	val := "0"
	if muted {
		val = "1"
	}
	_, err := soapCall(ip, renderingPath, renderingService, renderingVersion, "SetMute", map[string]string{
		"InstanceID":    "0",
		"Channel":       "Master",
		"DesiredMute":   val,
	})
	return err
}

// GetMute fetches the current master mute state.
func GetMute(ip string) (bool, error) {
	raw, err := soapCall(ip, renderingPath, renderingService, renderingVersion, "GetMute", map[string]string{
		"InstanceID": "0",
		"Channel":    "Master",
	})
	if err != nil {
		return false, err
	}
	val := parseSOAPField(raw, "CurrentMute")
	return val == "1", nil
}

// SetVolume sets the master volume (0–100).
func SetVolume(ip string, vol int) error {
	if vol < 0 {
		vol = 0
	}
	if vol > 100 {
		vol = 100
	}
	_, err := soapCall(ip, renderingPath, renderingService, renderingVersion, "SetVolume", map[string]string{
		"InstanceID":    "0",
		"Channel":       "Master",
		"DesiredVolume": strconv.Itoa(vol),
	})
	return err
}
