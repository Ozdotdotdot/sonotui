package sonos

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"
)

// NotifyServer receives UPnP GENA event notifications from Sonos speakers.
type NotifyServer struct {
	LanIP   string
	Port    int
	EventCh chan GENAEvent // buffered, cap 32
	server  *http.Server
}

// NewNotifyServer creates and starts a local HTTP server on the LAN IP,
// listening on a port in the range 34500–34599.
// Override port with portOverride > 0.
func NewNotifyServer(portOverride int) (*NotifyServer, error) {
	lanIP, err := findLanIP()
	if err != nil {
		return nil, fmt.Errorf("find lan ip: %w", err)
	}

	port, ln, err := listenOnRange(lanIP, portOverride)
	if err != nil {
		return nil, fmt.Errorf("listen: %w", err)
	}

	ns := &NotifyServer{
		LanIP:   lanIP,
		Port:    port,
		EventCh: make(chan GENAEvent, 32),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/notify", ns.handleNotify)
	ns.server = &http.Server{Handler: mux}

	go func() {
		if err := ns.server.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Printf("notify server: %v", err)
		}
	}()

	return ns, nil
}

// CallbackURL returns the URL that Sonos should POST NOTIFY events to.
func (ns *NotifyServer) CallbackURL() string {
	return fmt.Sprintf("http://%s:%d/notify", ns.LanIP, ns.Port)
}

// Shutdown stops the notify server.
func (ns *NotifyServer) Shutdown() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = ns.server.Shutdown(ctx)
}

func (ns *NotifyServer) handleNotify(w http.ResponseWriter, r *http.Request) {
	if r.Method != "NOTIFY" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "read error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)

	sid := r.Header.Get("SID")
	service := r.Header.Get("X-Sonotui-Service") // set internally when we subscribe
	if service == "" {
		// Determine service from SID lookup not available here; caller will resolve.
		service = "unknown"
	}

	select {
	case ns.EventCh <- GENAEvent{SID: sid, Service: service, Body: body}:
	default:
		log.Printf("notify: event channel full, dropping event from SID %s", sid)
	}
}

// Subscription tracks a GENA subscription to a single Sonos service.
type Subscription struct {
	SID     string
	Service string
	ip      string
	path    string
	timer   *time.Timer
}

// Subscribe sends a GENA SUBSCRIBE request and returns the subscription.
// renewFn is called ~30 minutes before expiry to renew the subscription.
func Subscribe(ip, path, service, callbackURL string) (*Subscription, error) {
	url := fmt.Sprintf("http://%s:1400%s", ip, path)
	req, err := http.NewRequest("SUBSCRIBE", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("CALLBACK", "<"+callbackURL+">")
	req.Header.Set("NT", "upnp:event")
	req.Header.Set("TIMEOUT", "Second-3600")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("subscribe %s: %s", service, resp.Status)
	}

	sid := strings.TrimSpace(resp.Header.Get("SID"))
	if sid == "" {
		return nil, fmt.Errorf("subscribe %s: no SID in response", service)
	}

	sub := &Subscription{
		SID:     sid,
		Service: service,
		ip:      ip,
		path:    path,
	}
	return sub, nil
}

// Renew sends a GENA renewal request for the subscription.
func (sub *Subscription) Renew() error {
	url := fmt.Sprintf("http://%s:1400%s", sub.ip, sub.path)
	req, err := http.NewRequest("SUBSCRIBE", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("SID", sub.SID)
	req.Header.Set("TIMEOUT", "Second-3600")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("renew %s: %s", sub.Service, resp.Status)
	}
	return nil
}

// Unsubscribe sends a GENA UNSUBSCRIBE request. Logs errors but does not block.
func (sub *Subscription) Unsubscribe() {
	url := fmt.Sprintf("http://%s:1400%s", sub.ip, sub.path)
	req, err := http.NewRequest("UNSUBSCRIBE", url, nil)
	if err != nil {
		log.Printf("unsubscribe %s: build request: %v", sub.Service, err)
		return
	}
	req.Header.Set("SID", sub.SID)

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("unsubscribe %s: %v", sub.Service, err)
		return
	}
	resp.Body.Close()
}

// StartRenewal schedules automatic renewal of the subscription every 1800s.
// onRenewErr is called if a renewal fails (e.g. to resubscribe).
func (sub *Subscription) StartRenewal(onRenewErr func(*Subscription)) {
	if sub.timer != nil {
		sub.timer.Stop()
	}
	sub.timer = time.AfterFunc(1800*time.Second, func() {
		if err := sub.Renew(); err != nil {
			log.Printf("renewal failed for %s: %v", sub.Service, err)
			if onRenewErr != nil {
				onRenewErr(sub)
			}
			return
		}
		// Schedule next renewal.
		sub.StartRenewal(onRenewErr)
	})
}

// StopRenewal cancels the renewal timer.
func (sub *Subscription) StopRenewal() {
	if sub.timer != nil {
		sub.timer.Stop()
		sub.timer = nil
	}
}

// AVTransportState is parsed from an AVTransport LastChange GENA event.
type AVTransportState struct {
	TransportState string // PLAYING | PAUSED_PLAYBACK | STOPPED | TRANSITIONING
	CurrentTrackURI string
	Track          TrackInfo
	Duration       int // seconds; 0 if unavailable
}

// ParseAVTransportEvent parses a raw GENA NOTIFY body for the AVTransport service.
func ParseAVTransportEvent(speakerIP string, body []byte) (AVTransportState, error) {
	lastChange, err := extractLastChange(body)
	if err != nil {
		return AVTransportState{}, fmt.Errorf("extract LastChange: %w", err)
	}

	// Decode entity-escaped inner XML.
	inner := html.UnescapeString(strings.TrimSpace(lastChange))

	state := AVTransportState{}
	dec := xml.NewDecoder(strings.NewReader(inner))
	var inInstance bool

	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "InstanceID" {
				inInstance = true
				continue
			}
			if !inInstance {
				continue
			}
			val := attrVal(t, "val")
			switch t.Name.Local {
			case "TransportState":
				state.TransportState = val
			case "CurrentTrackURI":
				state.CurrentTrackURI = val
			case "CurrentTrackDuration":
				state.Duration = parseDuration(val)
			case "CurrentTrackMetaData":
				if val != "" {
					decoded := html.UnescapeString(val)
					state.Track = parseDIDLLite(speakerIP, []byte(decoded))
				}
			}
		case xml.EndElement:
			if t.Name.Local == "InstanceID" {
				inInstance = false
			}
		}
	}

	return state, nil
}

// ParseRenderingControlEvent extracts the master volume from a RenderingControl GENA event.
// Returns -1 if not found.
func ParseRenderingControlEvent(body []byte) (int, error) {
	lastChange, err := extractLastChange(body)
	if err != nil {
		return -1, fmt.Errorf("extract LastChange: %w", err)
	}

	inner := html.UnescapeString(strings.TrimSpace(lastChange))
	dec := xml.NewDecoder(strings.NewReader(inner))
	var inInstance bool

	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "InstanceID" {
				inInstance = true
				continue
			}
			if !inInstance {
				continue
			}
			if t.Name.Local == "Volume" {
				channel := attrVal(t, "channel")
				if strings.EqualFold(channel, "Master") {
					val := attrVal(t, "val")
					v := 0
					fmt.Sscan(val, &v)
					return v, nil
				}
			}
		case xml.EndElement:
			if t.Name.Local == "InstanceID" {
				inInstance = false
			}
		}
	}
	return -1, nil
}

// extractLastChange pulls the text content of the <LastChange> element from
// a UPnP propertyset body.
func extractLastChange(body []byte) (string, error) {
	dec := xml.NewDecoder(bytes.NewReader(body))
	for {
		tok, err := dec.Token()
		if err != nil {
			if err == io.EOF {
				return "", fmt.Errorf("LastChange element not found")
			}
			return "", err
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		if strings.EqualFold(se.Name.Local, "LastChange") {
			var val string
			if err := dec.DecodeElement(&val, &se); err != nil {
				return "", err
			}
			return val, nil
		}
	}
}

func attrVal(se xml.StartElement, name string) string {
	for _, a := range se.Attr {
		if strings.EqualFold(a.Name.Local, name) {
			return a.Value
		}
	}
	return ""
}

// findLanIP returns the first non-loopback IPv4 address found on the system.
func findLanIP() (string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			var ip net.IP
			switch v := a.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip4 := ip.To4(); ip4 != nil {
				return ip4.String(), nil
			}
		}
	}
	return "", fmt.Errorf("no LAN IPv4 address found")
}

func listenOnRange(lanIP string, portOverride int) (int, net.Listener, error) {
	if portOverride > 0 {
		addr := fmt.Sprintf("%s:%d", lanIP, portOverride)
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			return 0, nil, err
		}
		return portOverride, ln, nil
	}

	for port := 34500; port <= 34599; port++ {
		addr := fmt.Sprintf("%s:%d", lanIP, port)
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			continue
		}
		return port, ln, nil
	}
	return 0, nil, fmt.Errorf("no available port in range 34500-34599")
}
