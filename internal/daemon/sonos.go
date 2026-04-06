package daemon

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
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/koron/go-ssdp"
)

// ── Constants ─────────────────────────────────────────────────────────────────

const (
	ssdpTarget     = "urn:schemas-upnp-org:device:ZonePlayer:1"
	ssdpWaitSecs   = 3
	deviceDescPath = "/xml/device_description.xml"
	soapTimeout    = 3 * time.Second

	avTransportPath    = "/MediaRenderer/AVTransport/Control"
	avTransportService = "AVTransport"
	avTransportVersion = 1

	renderingPath    = "/MediaRenderer/RenderingControl/Control"
	renderingService = "RenderingControl"
	renderingVersion = 1

	contentDirPath    = "/MediaServer/ContentDirectory/Control"
	contentDirService = "ContentDirectory"
	contentDirVersion = 1
)

// ── SSDP Discovery ────────────────────────────────────────────────────────────

type deviceDescription struct {
	Device struct {
		RoomName     string `xml:"roomName"`
		FriendlyName string `xml:"friendlyName"`
		UDN          string `xml:"UDN"`
	} `xml:"device"`
}

// DiscoverSpeakers performs SSDP discovery and returns all found Sonos speakers.
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
		ip := extractHost(svc.Location)
		if ip == "" || seen[ip] {
			continue
		}
		seen[ip] = true

		sp, err := FetchSpeakerByIP(ctx, ip)
		if err != nil {
			_ = client // suppress unused warning
			continue
		}
		speakers = append(speakers, sp)
	}
	return speakers, nil
}

// FetchSpeakerByIP fetches speaker info directly by IP.
func FetchSpeakerByIP(ctx context.Context, ip string) (Speaker, error) {
	client := &http.Client{Timeout: 3 * time.Second}
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
	name := strings.TrimSpace(dd.Device.RoomName)
	if name == "" {
		name = strings.TrimSpace(dd.Device.FriendlyName)
	}
	if name == "" {
		name = ip
	}
	return Speaker{IP: ip, UUID: udn, Name: name}, nil
}

func extractHost(rawURL string) string {
	i := strings.Index(rawURL, "://")
	if i < 0 {
		return ""
	}
	host := rawURL[i+3:]
	if j := strings.IndexByte(host, '/'); j >= 0 {
		host = host[:j]
	}
	if j := strings.LastIndexByte(host, ':'); j >= 0 {
		host = host[:j]
	}
	return host
}

// ── SOAP ──────────────────────────────────────────────────────────────────────

func soapCall(ip, path, serviceType string, version int, action string, args map[string]string) ([]byte, error) {
	serviceURN := fmt.Sprintf("urn:schemas-upnp-org:service:%s:%d", serviceType, version)
	url := fmt.Sprintf("http://%s:1400%s", ip, path)

	body := buildSOAPEnvelope(serviceURN, action, args)
	client := &http.Client{Timeout: soapTimeout}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", `text/xml; charset="utf-8"`)
	req.Header.Set("SOAPACTION", fmt.Sprintf(`"%s#%s"`, serviceURN, action))

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusOK {
		return raw, nil
	}
	if resp.StatusCode == http.StatusInternalServerError {
		if upnpErr := parseUPnPError(raw); upnpErr != "" {
			return nil, fmt.Errorf("upnp error: %s", upnpErr)
		}
	}
	return nil, fmt.Errorf("soap http %s", resp.Status)
}

func buildSOAPEnvelope(serviceURN, action string, args map[string]string) []byte {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0"?>`)
	b.WriteString(`<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">`)
	b.WriteString(`<s:Body>`)
	b.WriteString(`<u:` + action + ` xmlns:u="` + xmlEscape(serviceURN) + `">`)

	keys := make([]string, 0, len(args))
	for k := range args {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		b.WriteString("<" + k + ">" + xmlEscape(args[k]) + "</" + k + ">")
	}
	b.WriteString(`</u:` + action + `>`)
	b.WriteString(`</s:Body></s:Envelope>`)
	return []byte(b.String())
}

func xmlEscape(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

func parseUPnPError(raw []byte) string {
	type upnpErrBody struct {
		Code        string `xml:"errorCode"`
		Description string `xml:"errorDescription"`
	}
	dec := xml.NewDecoder(bytes.NewReader(raw))
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		if se.Name.Local == "UPnPError" {
			var body upnpErrBody
			if err := dec.DecodeElement(&body, &se); err == nil {
				code := strings.TrimSpace(body.Code)
				desc := strings.TrimSpace(body.Description)
				if code != "" || desc != "" {
					return fmt.Sprintf("%s: %s", code, desc)
				}
			}
		}
	}
	return ""
}

func parseSOAPField(raw []byte, field string) string {
	dec := xml.NewDecoder(bytes.NewReader(raw))
	var inField bool
	for {
		tok, err := dec.Token()
		if err != nil {
			return ""
		}
		switch t := tok.(type) {
		case xml.StartElement:
			inField = t.Name.Local == field
		case xml.EndElement:
			if t.Name.Local == field {
				inField = false
			}
		case xml.CharData:
			if inField {
				return strings.TrimSpace(string(t))
			}
		}
	}
}

func parseSOAPFields(raw []byte, fields ...string) map[string]string {
	want := make(map[string]bool, len(fields))
	for _, f := range fields {
		want[f] = true
	}
	out := make(map[string]string, len(fields))
	dec := xml.NewDecoder(bytes.NewReader(raw))
	var currentKey string
	for {
		tok, err := dec.Token()
		if err != nil {
			return out
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if want[t.Name.Local] {
				currentKey = t.Name.Local
			} else {
				currentKey = ""
			}
		case xml.EndElement:
			currentKey = ""
		case xml.CharData:
			if currentKey != "" {
				out[currentKey] = strings.TrimSpace(string(t))
				currentKey = ""
			}
		}
	}
}

// ── Transport actions ─────────────────────────────────────────────────────────

func sonosPlay(ip string) error {
	_, err := soapCall(ip, avTransportPath, avTransportService, avTransportVersion, "Play",
		map[string]string{"InstanceID": "0", "Speed": "1"})
	return err
}

func sonosPause(ip string) error {
	_, err := soapCall(ip, avTransportPath, avTransportService, avTransportVersion, "Pause",
		map[string]string{"InstanceID": "0"})
	return err
}

func sonosStop(ip string) error {
	_, err := soapCall(ip, avTransportPath, avTransportService, avTransportVersion, "Stop",
		map[string]string{"InstanceID": "0"})
	return err
}

func sonosNext(ip string) error {
	_, err := soapCall(ip, avTransportPath, avTransportService, avTransportVersion, "Next",
		map[string]string{"InstanceID": "0"})
	return err
}

func sonosPrevious(ip string) error {
	_, err := soapCall(ip, avTransportPath, avTransportService, avTransportVersion, "Previous",
		map[string]string{"InstanceID": "0"})
	return err
}

func sonosLineIn(ip, uuid string) error {
	uri := fmt.Sprintf("x-rincon-stream:%s", uuid)
	_, err := soapCall(ip, avTransportPath, avTransportService, avTransportVersion, "SetAVTransportURI",
		map[string]string{"CurrentURI": uri, "CurrentURIMetaData": "", "InstanceID": "0"})
	return err
}

func sonosQueue(ip, uuid string) error {
	uri := fmt.Sprintf("x-rincon-queue:%s#0", uuid)
	_, err := soapCall(ip, avTransportPath, avTransportService, avTransportVersion, "SetAVTransportURI",
		map[string]string{"CurrentURI": uri, "CurrentURIMetaData": "", "InstanceID": "0"})
	return err
}

func sonosSetVolume(ip string, vol int) error {
	if vol < 0 {
		vol = 0
	}
	if vol > 100 {
		vol = 100
	}
	_, err := soapCall(ip, renderingPath, renderingService, renderingVersion, "SetVolume",
		map[string]string{"Channel": "Master", "DesiredVolume": strconv.Itoa(vol), "InstanceID": "0"})
	return err
}

func sonosGetTransportInfo(ip string) (string, error) {
	raw, err := soapCall(ip, avTransportPath, avTransportService, avTransportVersion, "GetTransportInfo",
		map[string]string{"InstanceID": "0"})
	if err != nil {
		return "", err
	}
	state := parseSOAPField(raw, "CurrentTransportState")
	if state == "" {
		state = "STOPPED"
	}
	return state, nil
}

func sonosGetVolume(ip string) (int, error) {
	raw, err := soapCall(ip, renderingPath, renderingService, renderingVersion, "GetVolume",
		map[string]string{"Channel": "Master", "InstanceID": "0"})
	if err != nil {
		return 0, err
	}
	v, _ := strconv.Atoi(parseSOAPField(raw, "CurrentVolume"))
	return v, nil
}

func sonosSetMute(ip string, muted bool) error {
	val := "0"
	if muted {
		val = "1"
	}
	_, err := soapCall(ip, renderingPath, renderingService, renderingVersion, "SetMute",
		map[string]string{"Channel": "Master", "DesiredMute": val, "InstanceID": "0"})
	return err
}

// ── Queue management ──────────────────────────────────────────────────────────

// GetPositionInfo fetches playback position.
type positionResult struct {
	Elapsed  int
	Duration int
	Track    TrackInfo
}

func sonosGetPositionInfo(ip string) (positionResult, error) {
	raw, err := soapCall(ip, avTransportPath, avTransportService, avTransportVersion, "GetPositionInfo",
		map[string]string{"InstanceID": "0"})
	if err != nil {
		return positionResult{}, err
	}
	fields := parseSOAPFields(raw, "TrackDuration", "RelTime", "TrackMetaData")
	return positionResult{
		Elapsed:  parseDuration(fields["RelTime"]),
		Duration: parseDuration(fields["TrackDuration"]),
		Track:    parseDIDLLite(ip, []byte(fields["TrackMetaData"])),
	}, nil
}

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

func formatDurationHMS(secs int) string {
	h := secs / 3600
	m := (secs % 3600) / 60
	s := secs % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("0:%02d:%02d", m, s)
}

// GetQueue fetches the current Sonos queue.
func sonosGetQueue(ip string) ([]QueueItem, error) {
	raw, err := soapCall(ip, contentDirPath, contentDirService, contentDirVersion, "Browse",
		map[string]string{
			"BrowseFlag":     "BrowseDirectChildren",
			"Filter":         "*",
			"ObjectID":       "Q:0",
			"RequestedCount": "0",
			"SortCriteria":   "",
			"StartingIndex":  "0",
		})
	if err != nil {
		return nil, err
	}
	resultStr := parseSOAPField(raw, "Result")
	if resultStr == "" {
		return nil, nil
	}
	return parseQueueDIDL(resultStr)
}

type didlQueueItem struct {
	XMLName xml.Name `xml:"item"`
	ID      string   `xml:"id,attr"`
	Title   string   `xml:"title"`
	Creator string   `xml:"creator"`
	Album   string   `xml:"album"`
	Res     []struct {
		Duration string `xml:"duration,attr"`
		Value    string `xml:",chardata"`
	} `xml:"res"`
}

type didlRoot struct {
	Items []didlQueueItem `xml:"item"`
}

func parseQueueDIDL(s string) ([]QueueItem, error) {
	var root didlRoot
	if err := xml.Unmarshal([]byte(s), &root); err != nil {
		return nil, fmt.Errorf("parse queue DIDL: %w", err)
	}
	items := make([]QueueItem, 0, len(root.Items))
	for i, it := range root.Items {
		var uri string
		var dur int
		if len(it.Res) > 0 {
			uri = strings.TrimSpace(it.Res[0].Value)
			dur = parseDuration(it.Res[0].Duration)
		}
		items = append(items, QueueItem{
			Position: i + 1,
			Title:    it.Title,
			Artist:   it.Creator,
			Album:    it.Album,
			Duration: dur,
			URI:      uri,
			IsLocal:  strings.Contains(uri, ":8990/files/"),
		})
	}
	return items, nil
}

// AddURIToQueue adds a single URI to the queue with DIDL-Lite metadata.
func sonosAddURIToQueue(ip, uri, metadata string) error {
	_, err := soapCall(ip, avTransportPath, avTransportService, avTransportVersion, "AddURIToQueue",
		map[string]string{
			"DesiredFirstTrackNumberEnqueued": "0",
			"EnqueueAsNext":                   "0",
			"EnqueuedURI":                     uri,
			"EnqueuedURIMetaData":             metadata,
			"InstanceID":                      "0",
		})
	return err
}

// RemoveTrackFromQueue removes a single track at the given 1-based position.
func sonosRemoveTrack(ip string, position int) error {
	objectID := fmt.Sprintf("Q:0/%d", position)
	_, err := soapCall(ip, avTransportPath, avTransportService, avTransportVersion, "RemoveTrackFromQueue",
		map[string]string{"InstanceID": "0", "ObjectID": objectID})
	return err
}

// ClearQueue removes all tracks from the queue.
func sonosClearQueue(ip string) error {
	_, err := soapCall(ip, avTransportPath, avTransportService, avTransportVersion, "RemoveAllTracksFromQueue",
		map[string]string{"InstanceID": "0"})
	return err
}

// PlayFromQueue seeks to a queue position and plays.
func sonosPlayFromQueue(ip, uuid string, position int) error {
	if err := sonosQueue(ip, uuid); err != nil {
		return err
	}
	_, err := soapCall(ip, avTransportPath, avTransportService, avTransportVersion, "Seek",
		map[string]string{"InstanceID": "0", "Target": strconv.Itoa(position), "Unit": "TRACK_NR"})
	if err != nil {
		return err
	}
	return sonosPlay(ip)
}

// ReorderQueue moves a track from one position to another (1-based).
func sonosReorderQueue(ip string, from, to int) error {
	_, err := soapCall(ip, avTransportPath, avTransportService, avTransportVersion, "ReorderTracksInQueue",
		map[string]string{
			"InsertBefore":   strconv.Itoa(to),
			"InstanceID":     "0",
			"NumberOfTracks": "1",
			"StartingIndex":  strconv.Itoa(from),
			"UpdateID":       "0",
		})
	return err
}

// BuildDIDLLite builds a DIDL-Lite XML string for a local track.
func BuildDIDLLite(t Track, fileURI, artURI string) string {
	dur := formatDurationHMS(t.Duration)
	mimeType := "audio/flac"
	ext := strings.ToLower(t.Path[strings.LastIndex(t.Path, ".")+1:])
	switch ext {
	case "mp3":
		mimeType = "audio/mpeg"
	case "m4a":
		mimeType = "audio/mp4"
	case "wav":
		mimeType = "audio/wav"
	case "ogg":
		mimeType = "audio/ogg"
	}

	artTag := ""
	if artURI != "" {
		artTag = fmt.Sprintf(`<upnp:albumArtURI>%s</upnp:albumArtURI>`, xmlEscape(artURI))
	}

	return fmt.Sprintf(
		`<DIDL-Lite xmlns:dc="http://purl.org/dc/elements/1.1/"`+
			` xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/"`+
			` xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/">`+
			`<item id="-1" parentID="-1" restricted="true">`+
			`<dc:title>%s</dc:title>`+
			`<dc:creator>%s</dc:creator>`+
			`<upnp:album>%s</upnp:album>`+
			`<upnp:originalTrackNumber>%d</upnp:originalTrackNumber>`+
			`<upnp:class>object.item.audioItem.musicTrack</upnp:class>`+
			`%s`+
			`<res protocolInfo="http-get:*:%s:*" duration="%s">%s</res>`+
			`</item></DIDL-Lite>`,
		xmlEscape(t.Title),
		xmlEscape(t.Artist),
		xmlEscape(t.Album),
		t.TrackNum,
		artTag,
		mimeType,
		dur,
		xmlEscape(fileURI),
	)
}

// ── GENA Notification Server ──────────────────────────────────────────────────

// GENAEvent is a raw UPnP event notification received from a Sonos speaker.
type GENAEvent struct {
	SID     string
	Service string
	Body    []byte
}

// NotifyServer receives UPnP GENA event notifications.
type NotifyServer struct {
	LanIP   string
	Port    int
	EventCh chan GENAEvent
	server  *http.Server
}

// NewNotifyServer creates and starts a local HTTP server on the LAN IP.
func NewNotifyServer(lanIP string) (*NotifyServer, error) {
	port, ln, err := listenOnRange(lanIP)
	if err != nil {
		return nil, fmt.Errorf("listen: %w", err)
	}

	ns := &NotifyServer{
		LanIP:   lanIP,
		Port:    port,
		EventCh: make(chan GENAEvent, 64),
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
	service := r.Header.Get("X-Sonotui-Service")
	if service == "" {
		service = "unknown"
	}
	select {
	case ns.EventCh <- GENAEvent{SID: sid, Service: service, Body: body}:
	default:
		log.Printf("notify: channel full, dropping event SID=%s", sid)
	}
}

// ── Subscription ──────────────────────────────────────────────────────────────

// Subscription tracks a GENA subscription to a single Sonos service.
type Subscription struct {
	SID     string
	Service string
	ip      string
	path    string
	timer   *time.Timer
}

var serviceEventPaths = map[string]string{
	"AVTransport":       "/MediaRenderer/AVTransport/Event",
	"RenderingControl":  "/MediaRenderer/RenderingControl/Event",
	"ZoneGroupTopology": "/ZoneGroupTopology/Event",
}

// Subscribe sends a GENA SUBSCRIBE request.
func Subscribe(ip, path, service, callbackURL string) (*Subscription, error) {
	url := fmt.Sprintf("http://%s:1400%s", ip, path)
	req, err := http.NewRequest("SUBSCRIBE", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("CALLBACK", "<"+callbackURL+">")
	req.Header.Set("NT", "upnp:event")
	req.Header.Set("TIMEOUT", "Second-3600")
	req.Header.Set("X-Sonotui-Service", service) // echo service name for lookup

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
	return &Subscription{SID: sid, Service: service, ip: ip, path: path}, nil
}

// Renew sends a GENA renewal request.
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

// Unsubscribe sends a GENA UNSUBSCRIBE request.
func (sub *Subscription) Unsubscribe() {
	url := fmt.Sprintf("http://%s:1400%s", sub.ip, sub.path)
	req, _ := http.NewRequest("UNSUBSCRIBE", url, nil)
	if req == nil {
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

// ── GENA Event parsing ────────────────────────────────────────────────────────

// AVTransportState is parsed from an AVTransport LastChange GENA event.
type AVTransportState struct {
	TransportState  string
	CurrentTrackURI string
	Track           TrackInfo
	Duration        int
}

// ParseAVTransportEvent parses a raw GENA NOTIFY body for the AVTransport service.
func ParseAVTransportEvent(speakerIP string, body []byte) (AVTransportState, error) {
	lastChange, err := extractLastChange(body)
	if err != nil {
		return AVTransportState{}, fmt.Errorf("extract LastChange: %w", err)
	}
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

// RenderingState holds parsed RenderingControl GENA event data.
type RenderingState struct {
	Volume  int
	Muted   bool
	HasMute bool
}

// ParseRenderingControlEvent extracts volume and mute from a GENA event.
func ParseRenderingControlEvent(body []byte) (RenderingState, error) {
	lastChange, err := extractLastChange(body)
	if err != nil {
		return RenderingState{Volume: -1}, fmt.Errorf("extract LastChange: %w", err)
	}
	inner := html.UnescapeString(strings.TrimSpace(lastChange))
	dec := xml.NewDecoder(strings.NewReader(inner))
	var inInstance bool
	state := RenderingState{Volume: -1}
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
			channel := attrVal(t, "channel")
			if !strings.EqualFold(channel, "Master") && channel != "" {
				continue
			}
			switch t.Name.Local {
			case "Volume":
				v := 0
				fmt.Sscan(attrVal(t, "val"), &v)
				state.Volume = v
			case "Mute":
				state.Muted = attrVal(t, "val") == "1"
				state.HasMute = true
			}
		case xml.EndElement:
			if t.Name.Local == "InstanceID" {
				inInstance = false
			}
		}
	}
	return state, nil
}

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

func parseDIDLLite(speakerIP string, data []byte) TrackInfo {
	type didlItem struct {
		Title    string `xml:"title"`
		Creator  string `xml:"creator"`
		Album    string `xml:"album"`
		AlbumArt string `xml:"albumArtURI"`
		Res      []struct {
			Duration string `xml:"duration,attr"`
			Value    string `xml:",chardata"`
		} `xml:"res"`
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
		artURL = "http://" + speakerIP + ":1400" + artURL
	}
	dur := 0
	uri := ""
	if len(item.Res) > 0 {
		dur = parseDuration(item.Res[0].Duration)
		uri = strings.TrimSpace(item.Res[0].Value)
	}
	return TrackInfo{
		Title:    item.Title,
		Artist:   item.Creator,
		Album:    item.Album,
		ArtURL:   artURL,
		Duration: dur,
		URI:      uri,
	}
}

// ── Network helpers ───────────────────────────────────────────────────────────

// FindLanIP returns the first non-loopback IPv4 address found.
func FindLanIP() (string, error) {
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

func listenOnRange(lanIP string) (int, net.Listener, error) {
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

// ── SonosManager orchestrates discovery, GENA, and state updates ──────────────

// SonosManager manages the full Sonos connection lifecycle.
type SonosManager struct {
	state     *State
	events    *Broadcaster
	lib       *Library
	notify    *NotifyServer
	subs      []*Subscription
	subsMu    sync.Mutex
	lanIP     string
	filePort  int
	preferred string // preferred speaker UUID
}

// NewSonosManager creates a SonosManager.
func NewSonosManager(state *State, events *Broadcaster, lib *Library, lanIP string, filePort int, preferred string) *SonosManager {
	return &SonosManager{
		state:     state,
		events:    events,
		lib:       lib,
		lanIP:     lanIP,
		filePort:  filePort,
		preferred: preferred,
	}
}

func (sm *SonosManager) enrichTrackInfo(t TrackInfo) TrackInfo {
	if sm.lib == nil || t.URI == "" {
		return t
	}
	u, err := url.Parse(t.URI)
	if err != nil {
		return t
	}
	if !strings.HasPrefix(u.Path, "/files/") {
		return t
	}
	relPath, err := url.PathUnescape(strings.TrimPrefix(u.Path, "/files/"))
	if err != nil {
		return t
	}
	track, ok := sm.lib.TrackByPath(relPath)
	if !ok {
		return t
	}
	if t.Title == "" {
		t.Title = track.Title
	}
	if t.Artist == "" {
		t.Artist = track.Artist
	}
	if t.Album == "" {
		t.Album = track.Album
	}
	if t.Duration == 0 {
		t.Duration = track.Duration
	}
	if t.ArtURL == "" && track.ArtHash != "" {
		t.ArtURL = fmt.Sprintf("http://%s:%d/art/%s", sm.lanIP, sm.filePort, track.ArtHash)
	}
	return t
}

// Start launches background discovery and GENA listener.
func (sm *SonosManager) Start() error {
	ns, err := NewNotifyServer(sm.lanIP)
	if err != nil {
		return fmt.Errorf("notify server: %w", err)
	}
	sm.notify = ns

	go sm.processGENAEvents()
	go sm.runDiscovery()
	go sm.runPositionBroadcast()
	go sm.runResubscribeWatchdog()
	return nil
}

// runResubscribeWatchdog resubscribes to the active speaker every 4 minutes.
// This recovers from sleep/wake cycles where the network went down and Sonos
// dropped the GENA subscription, leaving the daemon with stale state.
// Resubscribing causes Sonos to immediately send its current state as the
// first GENA notification, which updates transport/track/volume in the daemon.
func (sm *SonosManager) runResubscribeWatchdog() {
	ticker := time.NewTicker(4 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		sm.state.RLock()
		sp := sm.state.ActiveSpeaker
		sm.state.RUnlock()
		if sp == nil {
			continue
		}
		log.Printf("watchdog: resubscribing to speaker %q", sp.Name)
		sm.subscribeToSpeaker(*sp)
	}
}

// Shutdown unsubscribes and stops background listener resources.
func (sm *SonosManager) Shutdown() {
	sm.subsMu.Lock()
	subs := sm.subs
	sm.subs = nil
	sm.subsMu.Unlock()
	for _, sub := range subs {
		sub.StopRenewal()
		sub.Unsubscribe()
	}
	if sm.notify != nil {
		sm.notify.Shutdown()
	}
}

func (sm *SonosManager) runDiscovery() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	speakers, err := DiscoverSpeakers(ctx)
	if err != nil {
		log.Printf("discovery: %v", err)
		return
	}
	if len(speakers) == 0 {
		log.Printf("discovery: no speakers found")
		return
	}

	sm.state.Lock()
	sm.state.Speakers = speakers
	// Pick active speaker: preferred UUID or first.
	active := speakers[0]
	for _, sp := range speakers {
		if sp.UUID == sm.preferred {
			active = sp
			break
		}
	}
	sm.state.ActiveSpeaker = &active
	sm.state.Unlock()

	sm.events.Send(evtSpeaker(active))
	sm.subscribeToSpeaker(active)
}

// Rediscover re-runs SSDP discovery.
func (sm *SonosManager) Rediscover() {
	go sm.runDiscovery()
}

// Reconnect resubscribes to the active speaker and polls its current state.
// This is the manual equivalent of what the watchdog does every 4 minutes —
// useful after a sleep/wake cycle or any network interruption.
func (sm *SonosManager) Reconnect() error {
	sm.state.RLock()
	sp := sm.state.ActiveSpeaker
	sm.state.RUnlock()
	if sp == nil {
		return fmt.Errorf("no active speaker")
	}
	sm.subscribeToSpeaker(*sp)
	return nil
}

func (sm *SonosManager) subscribeToSpeaker(sp Speaker) {
	// Unsubscribe from previous.
	sm.subsMu.Lock()
	old := sm.subs
	sm.subs = nil
	sm.subsMu.Unlock()
	for _, sub := range old {
		sub.StopRenewal()
		go sub.Unsubscribe()
	}

	cbURL := sm.notify.CallbackURL()
	var newSubs []*Subscription
	for service, path := range serviceEventPaths {
		sub, err := Subscribe(sp.IP, path, service, cbURL)
		if err != nil {
			log.Printf("subscribe %s on %s: %v", service, sp.IP, err)
			continue
		}
		sub.StartRenewal(func(s *Subscription) {
			log.Printf("renewal failed for %s, resubscribing", s.Service)
			if newSub, err := Subscribe(sp.IP, s.path, s.Service, cbURL); err == nil {
				sm.subsMu.Lock()
				for i, old := range sm.subs {
					if old.Service == s.Service {
						sm.subs[i] = newSub
						break
					}
				}
				sm.subsMu.Unlock()
			}
		})
		newSubs = append(newSubs, sub)
	}
	sm.subsMu.Lock()
	sm.subs = newSubs
	sm.subsMu.Unlock()

	go sm.pollInitialState(sp)
}

// pollInitialState fetches the speaker's current transport, position, and
// volume via SOAP immediately after subscribing. This gives the daemon
// accurate state right away without waiting for Sonos to push a GENA event.
func (sm *SonosManager) pollInitialState(sp Speaker) {
	transport, err := sonosGetTransportInfo(sp.IP)
	if err != nil {
		log.Printf("pollInitialState: GetTransportInfo: %v", err)
	}

	pos, err := sonosGetPositionInfo(sp.IP)
	if err != nil {
		log.Printf("pollInitialState: GetPositionInfo: %v", err)
	}

	vol, err := sonosGetVolume(sp.IP)
	if err != nil {
		log.Printf("pollInitialState: GetVolume: %v", err)
	}

	sm.state.Lock()
	if transport != "" {
		sm.state.Transport = transport
		sm.state.Playing = transport == "PLAYING"
	}
	if pos.Elapsed > 0 || pos.Duration > 0 {
		sm.state.Elapsed = pos.Elapsed
		sm.state.Duration = pos.Duration
	}
	if pos.Track.URI != "" {
		sm.state.Track = sm.enrichTrackInfo(pos.Track)
	}
	if vol > 0 {
		sm.state.Volume = vol
	}
	sm.state.Unlock()

	// Broadcast the freshly polled state to any connected SSE clients.
	sm.state.RLock()
	t := sm.state.Track
	elapsed := sm.state.Elapsed
	duration := sm.state.Duration
	currentTransport := sm.state.Transport
	currentVol := sm.state.Volume
	sm.state.RUnlock()

	sm.events.Send(evtTransport(currentTransport))
	if t.URI != "" {
		sm.events.Send(evtTrack(t))
	}
	sm.events.Send(evtPosition(elapsed, duration))
	sm.events.Send(evtVolume(currentVol))
}

// SwitchSpeaker switches the active speaker by UUID.
func (sm *SonosManager) SwitchSpeaker(uuid string) error {
	sm.state.RLock()
	speakers := sm.state.Speakers
	sm.state.RUnlock()

	for _, sp := range speakers {
		if sp.UUID == uuid {
			sm.state.Lock()
			spCopy := sp
			sm.state.ActiveSpeaker = &spCopy
			sm.state.Transport = "STOPPED"
			sm.state.Track = TrackInfo{}
			sm.state.Elapsed = 0
			sm.state.Duration = 0
			sm.state.Playing = false
			sm.state.Unlock()
			sm.events.Send(evtSpeaker(sp))
			sm.subscribeToSpeaker(sp)
			return nil
		}
	}
	return fmt.Errorf("speaker %s not found", uuid)
}

func (sm *SonosManager) processGENAEvents() {
	for evt := range sm.notify.EventCh {
		// Resolve service from SID.
		service := evt.Service
		if service == "unknown" {
			sm.subsMu.Lock()
			for _, sub := range sm.subs {
				if sub.SID == evt.SID {
					service = sub.Service
					break
				}
			}
			sm.subsMu.Unlock()
		}

		sm.state.RLock()
		activeSP := sm.state.ActiveSpeaker
		sm.state.RUnlock()

		speakerIP := ""
		if activeSP != nil {
			speakerIP = activeSP.IP
		}

		switch service {
		case "AVTransport":
			sm.handleAVTransport(speakerIP, evt.Body)
		case "RenderingControl":
			sm.handleRenderingControl(evt.Body)
		case "ZoneGroupTopology":
			// ignored
		}
	}
}

func (sm *SonosManager) handleAVTransport(speakerIP string, body []byte) {
	state, err := ParseAVTransportEvent(speakerIP, body)
	if err != nil {
		log.Printf("parse AVTransport: %v", err)
		return
	}

	sm.state.Lock()
	prevTransport := sm.state.Transport
	prevURI := sm.state.Track.URI
	prevIsLineIn := sm.state.IsLineIn

	if state.TransportState != "" {
		sm.state.Transport = state.TransportState
		sm.state.Playing = state.TransportState == "PLAYING"
	}

	isLineIn := strings.HasPrefix(state.CurrentTrackURI, "x-rincon-stream:")
	sm.state.IsLineIn = isLineIn

	if isLineIn {
		sm.state.Track = TrackInfo{}
	} else if state.CurrentTrackURI != "" {
		track := sm.enrichTrackInfo(state.Track)
		if track.URI == "" {
			track.URI = state.CurrentTrackURI
		}
		sm.state.Track = track
		if state.Duration > 0 {
			sm.state.Duration = state.Duration
		}
		// Increment generation counter when the track URI changes.
		if state.CurrentTrackURI != prevURI {
			sm.state.TrackGen++
		}
	}
	// Capture generation while lock is still held, before spawning goroutine.
	currentGen := sm.state.TrackGen
	sm.state.Unlock()

	// Broadcast events.
	if state.TransportState != "" {
		sm.events.Send(evtTransport(state.TransportState))
	}
	if prevIsLineIn != isLineIn {
		sm.events.Send(evtLineIn(isLineIn))
	}
	if !isLineIn {
		track := sm.enrichTrackInfo(state.Track)
		if track.Title != "" || track.ArtURL != "" || track.URI != "" {
			sm.events.Send(evtTrack(track))
		}
	}

	// Resync position on track change or resume.
	needsSync := false
	if prevIsLineIn && !isLineIn {
		needsSync = true
	}
	if !isLineIn && state.CurrentTrackURI != "" && state.CurrentTrackURI != prevURI {
		needsSync = true
	}
	if state.TransportState == "PLAYING" && (prevTransport == "PAUSED_PLAYBACK" || prevTransport == "STOPPED") {
		needsSync = true
	}
	if !isLineIn && state.CurrentTrackURI != "" &&
		(state.Track == (TrackInfo{}) || state.Duration == 0) {
		needsSync = true
	}

	if needsSync && speakerIP != "" {
		go func(expectedGen uint64, expectedURI string) {
			pos, err := sonosGetPositionInfo(speakerIP)
			if err != nil {
				return
			}
			sm.state.Lock()
			// Abort if the track has changed since we started — our data is stale.
			if sm.state.TrackGen != expectedGen {
				sm.state.Unlock()
				return
			}
			sm.state.Elapsed = pos.Elapsed
			if pos.Duration > 0 {
				sm.state.Duration = pos.Duration
			}
			if pos.Track != (TrackInfo{}) {
				pos.Track = sm.enrichTrackInfo(pos.Track)
				if pos.Track.URI == "" {
					pos.Track.URI = expectedURI
				}
				sm.state.Track = pos.Track
			}
			sm.state.Unlock()
			if pos.Track != (TrackInfo{}) {
				sm.events.Send(evtTrack(pos.Track))
			}
			sm.events.Send(evtPosition(pos.Elapsed, pos.Duration))
		}(currentGen, state.CurrentTrackURI)
	}

	// Queue may have changed on track changes.
	if state.CurrentTrackURI != "" {
		go sm.RefreshQueue()
	}
}

func (sm *SonosManager) handleRenderingControl(body []byte) {
	rs, err := ParseRenderingControlEvent(body)
	if err != nil {
		log.Printf("parse RenderingControl: %v", err)
		return
	}
	if rs.Volume >= 0 {
		sm.state.Lock()
		sm.state.Volume = rs.Volume
		sm.state.Unlock()
		sm.events.Send(evtVolume(rs.Volume))
	}
}

// runPositionBroadcast increments elapsed every second and broadcasts.
func (sm *SonosManager) runPositionBroadcast() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for range ticker.C {
		sm.state.Lock()
		if sm.state.Playing && !sm.state.IsLineIn {
			sm.state.Elapsed++
			if sm.state.Duration > 0 && sm.state.Elapsed > sm.state.Duration {
				sm.state.Elapsed = sm.state.Duration
			}
		}
		elapsed := sm.state.Elapsed
		duration := sm.state.Duration
		playing := sm.state.Playing
		sm.state.Unlock()
		if playing {
			sm.events.Send(evtPosition(elapsed, duration))
		}
	}
}

// RefreshQueue fetches the Sonos queue and updates state.
func (sm *SonosManager) RefreshQueue() {
	sm.state.RLock()
	activeSP := sm.state.ActiveSpeaker
	sm.state.RUnlock()
	if activeSP == nil {
		return
	}
	items, err := sonosGetQueue(activeSP.IP)
	if err != nil {
		log.Printf("get queue: %v", err)
		return
	}
	sm.state.Lock()
	sm.state.Queue = items
	sm.state.Unlock()
	sm.events.Send(evtQueueChanged())
}

// Transport control proxies.

func (sm *SonosManager) activeSpeakerIP() (string, string, error) {
	sm.state.RLock()
	defer sm.state.RUnlock()
	if sm.state.ActiveSpeaker == nil {
		return "", "", fmt.Errorf("no active speaker")
	}
	return sm.state.ActiveSpeaker.IP, sm.state.ActiveSpeaker.UUID, nil
}

func (sm *SonosManager) Play() error {
	ip, _, err := sm.activeSpeakerIP()
	if err != nil {
		return err
	}
	return sonosPlay(ip)
}

func (sm *SonosManager) Pause() error {
	ip, _, err := sm.activeSpeakerIP()
	if err != nil {
		return err
	}
	return sonosPause(ip)
}

func (sm *SonosManager) Stop() error {
	ip, _, err := sm.activeSpeakerIP()
	if err != nil {
		return err
	}
	return sonosStop(ip)
}

func (sm *SonosManager) Next() error {
	ip, _, err := sm.activeSpeakerIP()
	if err != nil {
		return err
	}
	return sonosNext(ip)
}

func (sm *SonosManager) Prev() error {
	ip, _, err := sm.activeSpeakerIP()
	if err != nil {
		return err
	}
	return sonosPrevious(ip)
}

func (sm *SonosManager) LineIn() error {
	ip, uuid, err := sm.activeSpeakerIP()
	if err != nil {
		return err
	}
	return sonosLineIn(ip, uuid)
}

func (sm *SonosManager) SetVolume(vol int) error {
	ip, _, err := sm.activeSpeakerIP()
	if err != nil {
		return err
	}
	return sonosSetVolume(ip, vol)
}

func (sm *SonosManager) SetMute(muted bool) error {
	ip, _, err := sm.activeSpeakerIP()
	if err != nil {
		return err
	}
	return sonosSetMute(ip, muted)
}

func (sm *SonosManager) AddToQueue(uri, metadata string) error {
	ip, _, err := sm.activeSpeakerIP()
	if err != nil {
		return err
	}
	if err := sonosAddURIToQueue(ip, uri, metadata); err != nil {
		return err
	}
	go sm.RefreshQueue()
	return nil
}

func (sm *SonosManager) RemoveFromQueue(position int) error {
	ip, _, err := sm.activeSpeakerIP()
	if err != nil {
		return err
	}
	if err := sonosRemoveTrack(ip, position); err != nil {
		return err
	}
	go sm.RefreshQueue()
	return nil
}

func (sm *SonosManager) ClearQueue() error {
	ip, _, err := sm.activeSpeakerIP()
	if err != nil {
		return err
	}
	if err := sonosClearQueue(ip); err != nil {
		return err
	}
	go sm.RefreshQueue()
	return nil
}

func (sm *SonosManager) PlayFromQueue(position int) error {
	ip, uuid, err := sm.activeSpeakerIP()
	if err != nil {
		return err
	}
	return sonosPlayFromQueue(ip, uuid, position)
}

func (sm *SonosManager) ReorderQueue(from, to int) error {
	ip, _, err := sm.activeSpeakerIP()
	if err != nil {
		return err
	}
	if err := sonosReorderQueue(ip, from, to); err != nil {
		return err
	}
	go sm.RefreshQueue()
	return nil
}
