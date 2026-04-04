package sonos

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

const soapTimeout = 3 * time.Second

// soapCall sends a UPnP SOAP request to the given Sonos speaker.
// path is e.g. "/MediaRenderer/AVTransport/Control"
// serviceType is e.g. "AVTransport", version is e.g. 1
// Returns the raw response body on success.
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
	b.WriteString(`<u:`)
	b.WriteString(action)
	b.WriteString(` xmlns:u="`)
	b.WriteString(xmlEscape(serviceURN))
	b.WriteString(`">`)

	// Sort keys for deterministic output.
	keys := make([]string, 0, len(args))
	for k := range args {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := args[k]
		b.WriteString("<")
		b.WriteString(k)
		b.WriteString(">")
		b.WriteString(xmlEscape(v))
		b.WriteString("</")
		b.WriteString(k)
		b.WriteString(">")
	}

	b.WriteString(`</u:`)
	b.WriteString(action)
	b.WriteString(`>`)
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

// parseSOAPField extracts a single named field from a SOAP response body.
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

// parseSOAPFields extracts multiple named fields from a SOAP response body.
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
