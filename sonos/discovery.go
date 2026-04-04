package sonos

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/koron/go-ssdp"
)

const (
	ssdpTarget     = "urn:schemas-upnp-org:device:ZonePlayer:1"
	ssdpWaitSecs   = 3
	deviceDescPath = "/xml/device_description.xml"
)

type deviceDescription struct {
	Device struct {
		FriendlyName string `xml:"friendlyName"`
		UDN          string `xml:"UDN"`
	} `xml:"device"`
}

// DiscoverSpeakers performs SSDP discovery and returns all found Sonos speakers.
// It blocks for up to ~ssdpWaitSecs seconds. Run as a tea.Cmd goroutine.
func DiscoverSpeakers(ctx context.Context) ([]Speaker, error) {
	list, err := ssdp.Search(ssdpTarget, ssdpWaitSecs, "")
	if err != nil {
		return nil, fmt.Errorf("ssdp search: %w", err)
	}

	client := &http.Client{Timeout: 3 * time.Second}
	seen := map[string]bool{}
	var speakers []Speaker

	for _, svc := range list {
		if svc.Location == "" {
			continue
		}
		// Extract IP from location URL.
		ip := extractHost(svc.Location)
		if ip == "" || seen[ip] {
			continue
		}
		seen[ip] = true

		sp, err := fetchSpeakerInfo(ctx, client, ip)
		if err != nil {
			continue
		}
		speakers = append(speakers, sp)
	}

	return speakers, nil
}

// FetchSpeakerByIP attempts to connect to a speaker at the given IP directly.
func FetchSpeakerByIP(ctx context.Context, ip string) (Speaker, error) {
	client := &http.Client{Timeout: 3 * time.Second}
	return fetchSpeakerInfo(ctx, client, ip)
}

func fetchSpeakerInfo(ctx context.Context, client *http.Client, ip string) (Speaker, error) {
	url := fmt.Sprintf("http://%s:1400%s", ip, deviceDescPath)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Speaker{}, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return Speaker{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Speaker{}, fmt.Errorf("device description: %s", resp.Status)
	}

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return Speaker{}, err
	}

	var dd deviceDescription
	if err := xml.Unmarshal(raw, &dd); err != nil {
		return Speaker{}, err
	}

	udn := strings.TrimPrefix(strings.TrimSpace(dd.Device.UDN), "uuid:")
	name := strings.TrimSpace(dd.Device.FriendlyName)
	if name == "" {
		name = ip
	}

	return Speaker{
		IP:           ip,
		UUID:         udn,
		FriendlyName: name,
	}, nil
}

func extractHost(rawURL string) string {
	// Fast path: find "://" then take until next "/" or ":".
	i := strings.Index(rawURL, "://")
	if i < 0 {
		return ""
	}
	host := rawURL[i+3:]
	// Strip path.
	if j := strings.IndexByte(host, '/'); j >= 0 {
		host = host[:j]
	}
	// Strip port.
	if j := strings.LastIndexByte(host, ':'); j >= 0 {
		host = host[:j]
	}
	return host
}
