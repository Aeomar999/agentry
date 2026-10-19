package main

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/storetest"
	"gitlab.com/dev.chempa/agentry/internal/tlsca"
	"gitlab.com/dev.chempa/agentry/internal/web"
)

const trustToken = "0123456789abcdef0123456789abcdef"

// withTheFrontDoor is serve's handler with -https on: the web server behind the
// front door, with a certificate naming 192.168.1.160.
func withTheFrontDoor(t *testing.T) (*tlsca.CA, http.Handler) {
	t.Helper()
	ca, dataDir := testCA(t)
	s, state := storetest.Open(t, filepath.Join(dataDir, "agentry.db"))
	server := web.NewServer(s, state, web.Config{UIToken: trustToken}, http.NotFoundHandler(), nil)
	door := doorOf(ca, "192.168.1.160")
	door.addresses = []string{"https://192.168.1.160:7800", "https://localhost:7800"}
	return ca, frontDoor(server.Handler(), door)
}

// fromAnotherMachine is what handler answers to a GET of path at
// 192.168.1.160:7800 from another machine, with no token, over TLS or
// plain HTTP.
func fromAnotherMachine(handler http.Handler, path string, overTLS bool) *httptest.ResponseRecorder {
	r := httptest.NewRequest("GET", "http://192.168.1.160:7800"+path, nil)
	r.RemoteAddr = "192.168.1.20:5000"
	if overTLS {
		r.TLS = &tls.ConnectionState{}
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w
}

// The trust page is served to anyone, with no token and no redirect, over
// plain HTTP and over TLS. It needs nothing else: no script, no /assets/,
// nothing from elsewhere. It names the CA, shows its fingerprint, links to
// the three downloads and to the https addresses that work from another
// device, and holds nothing secret.
func TestTheTrustPageIsServedToAnyone(t *testing.T) {
	ca, handler := withTheFrontDoor(t)
	for _, overTLS := range []bool{false, true} {
		w := fromAnotherMachine(handler, "/trust/", overTLS)
		body := w.Body.String()
		if w.Code != http.StatusOK || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/html") {
			t.Fatalf("over TLS %v: %d %s, Location %q", overTLS, w.Code, w.Header().Get("Content-Type"), w.Header().Get("Location"))
		}
		for _, want := range []string{"<title>Trust this device</title>", ca.Name(), ca.Fingerprint(),
			`href="/trust/ca.crt"`, `href="/trust/ca.pem"`, `href="/trust/agentry.mobileconfig"`, `href="https://192.168.1.160:7800/"`} {
			if !strings.Contains(body, want) {
				t.Errorf("over TLS %v: the page doesn't have %q", overTLS, want)
			}
		}
		for _, never := range []string{"/assets/", "<script", "src=", "http://", "localhost:7800", trustToken} {
			if strings.Contains(body, never) {
				t.Errorf("over TLS %v: the page has %q", overTLS, never)
			}
		}
		if csp := w.Header().Get("Content-Security-Policy"); !strings.HasPrefix(csp, "default-src 'none'") {
			t.Errorf("over TLS %v: Content-Security-Policy %q", overTLS, csp)
		}
	}
	// Its links are each served, and /trust is the page too.
	for path, want := range map[string]int{"/trust/ca.crt": 200, "/trust/ca.pem": 200, "/trust/" + profileName: 200,
		"/trust": http.StatusPermanentRedirect, "/trust/other": http.StatusNotFound} {
		if w := fromAnotherMachine(handler, path, false); w.Code != want {
			t.Errorf("%s: %d, want %d", path, w.Code, want)
		}
	}
	if w := fromAnotherMachine(handler, "/trust", false); w.Header().Get("Location") != "/trust/" {
		t.Errorf("/trust goes to %q", w.Header().Get("Location"))
	}
}

// The page, at a real https port, to a client trusting the CA.
func TestTheTrustPageOverHTTPS(t *testing.T) {
	ca, handler := withTheFrontDoor(t)
	address := serving(t, ca, handler)
	response, body := get(t, trusting(ca), "https://"+address+"/trust/")
	if response.StatusCode != http.StatusOK || !strings.Contains(body, ca.Fingerprint()) || !strings.Contains(body, `href="https://`+address+`/"`) {
		t.Errorf("%d\n%s", response.StatusCode, body)
	}
}

// Without -https there is no front door: the web server alone doesn't hand
// out the trust page or a certificate.
func TestNoTrustPageWithoutHTTPS(t *testing.T) {
	_, dataDir := testCA(t)
	s, state := storetest.Open(t, filepath.Join(dataDir, "agentry.db"))
	server := web.NewServer(s, state, web.Config{UIToken: trustToken}, http.NotFoundHandler(), nil)
	for _, path := range []string{"/trust/", "/trust/ca.crt", "/trust/" + profileName} {
		w := fromAnotherMachine(server.Handler(), path, false)
		if strings.Contains(w.Body.String(), "Trust this device") || strings.Contains(w.Body.String(), "BEGIN CERTIFICATE") || strings.Contains(w.Body.String(), "plist") {
			t.Errorf("%s without -https: %d %s", path, w.Code, w.Body)
		}
	}
}

// The iOS profile is a property list holding one root certificate payload,
// the CA's own DER, with identifiers and UUIDs that stay the same for the
// CA and differ for another.
func TestTheIOSProfile(t *testing.T) {
	ca, handler := withTheFrontDoor(t)
	w := fromAnotherMachine(handler, "/trust/"+profileName, false)
	if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "application/x-apple-aspen-config" {
		t.Fatalf("%d %s", w.Code, w.Header().Get("Content-Type"))
	}
	top := parsePlist(t, w.Body.Bytes()).(map[string]any)
	payloads, _ := top["PayloadContent"].([]any)
	if len(payloads) != 1 {
		t.Fatalf("payloads: %v", top["PayloadContent"])
	}
	root := payloads[0].(map[string]any)
	der, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(root["PayloadContent"].(string)), ""))
	if err != nil || !bytes.Equal(der, ca.DER()) {
		t.Errorf("the payload's certificate isn't the CA's: %v", err)
	}
	uuid := regexp.MustCompile(`^[0-9A-F]{8}-[0-9A-F]{4}-8[0-9A-F]{3}-[89AB][0-9A-F]{3}-[0-9A-F]{12}$`)
	for _, c := range []struct {
		dict map[string]any
		kind string
	}{{top, "Configuration"}, {root, "com.apple.security.root"}} {
		if c.dict["PayloadType"] != c.kind || c.dict["PayloadDisplayName"] != ca.Name() || c.dict["PayloadVersion"] != "1" ||
			!uuid.MatchString(fmt.Sprint(c.dict["PayloadUUID"])) || !strings.HasPrefix(fmt.Sprint(c.dict["PayloadIdentifier"]), "agentry.ca.") {
			t.Errorf("%s payload: %v", c.kind, c.dict)
		}
	}
	if top["PayloadUUID"] == root["PayloadUUID"] || top["PayloadIdentifier"] == root["PayloadIdentifier"] {
		t.Error("the profile and its payload share an identifier")
	}

	again := fromAnotherMachine(handler, "/trust/"+profileName, true)
	if !bytes.Equal(again.Body.Bytes(), w.Body.Bytes()) {
		t.Error("downloaded again, the profile differs")
	}
	other, otherHandler := withTheFrontDoor(t)
	theirs := parsePlist(t, fromAnotherMachine(otherHandler, "/trust/"+profileName, false).Body.Bytes()).(map[string]any)
	if other.Fingerprint() == ca.Fingerprint() || theirs["PayloadUUID"] == top["PayloadUUID"] || theirs["PayloadIdentifier"] == top["PayloadIdentifier"] {
		t.Error("another CA's profile has the same identifiers")
	}
}

// A CA named after an odd hostname still makes a well-formed profile.
func TestTheProfileEscapesTheName(t *testing.T) {
	var body bytes.Buffer
	if err := profile.Execute(&body, profileView{Name: `Agentry local CA (a<b>&"c")`, Certificate: "AAAA"}); err != nil {
		t.Fatal(err)
	}
	if name := parsePlist(t, body.Bytes()).(map[string]any)["PayloadDisplayName"]; name != `Agentry local CA (a<b>&"c")` {
		t.Errorf("the name came back as %q", name)
	}
}

// parsePlist reads an XML property list: a dict is a map, an array a
// slice, and every other value its text (true and false as their names).
func parsePlist(t *testing.T, data []byte) any {
	t.Helper()
	decoder := xml.NewDecoder(bytes.NewReader(data))
	decoder.Strict = true
	for {
		token, err := decoder.Token()
		if err != nil {
			t.Fatalf("not a property list: %v\n%s", err, data)
		}
		if start, ok := token.(xml.StartElement); ok && start.Name.Local == "plist" {
			value, err := plistValue(decoder, nextStart(decoder))
			if err != nil {
				t.Fatalf("not a property list: %v\n%s", err, data)
			}
			return value
		}
	}
}

func nextStart(decoder *xml.Decoder) xml.StartElement {
	for {
		token, err := decoder.Token()
		if err != nil {
			return xml.StartElement{}
		}
		switch token := token.(type) {
		case xml.StartElement:
			return token
		case xml.EndElement:
			return xml.StartElement{} // the container ended
		}
	}
}

func plistValue(decoder *xml.Decoder, start xml.StartElement) (any, error) {
	switch start.Name.Local {
	case "dict":
		dict := map[string]any{}
		for {
			key := nextStart(decoder)
			if key.Name.Local == "" {
				return dict, nil
			}
			var name string
			if key.Name.Local != "key" || decoder.DecodeElement(&name, &key) != nil {
				return nil, fmt.Errorf("a dict has <%s> where a key goes", key.Name.Local)
			}
			value, err := plistValue(decoder, nextStart(decoder))
			if err != nil {
				return nil, err
			}
			dict[name] = value
		}
	case "array":
		var array []any
		for {
			item := nextStart(decoder)
			if item.Name.Local == "" {
				return array, nil
			}
			value, err := plistValue(decoder, item)
			if err != nil {
				return nil, err
			}
			array = append(array, value)
		}
	case "true", "false":
		return start.Name.Local, decoder.Skip()
	case "string", "data", "integer":
		var text string
		err := decoder.DecodeElement(&text, &start)
		return text, err
	case "":
		return nil, io.ErrUnexpectedEOF
	default:
		return nil, fmt.Errorf("<%s> isn't a property list value", start.Name.Local)
	}
}
