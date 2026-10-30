package web

import (
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/domain/agents"
	"gitlab.com/dev.chempa/agentry/internal/platform/tlsca"
	"gitlab.com/dev.chempa/agentry/internal/testkit/storetest"
)

// trustSeen is what the page is told about trusting the server's own CA:
// in /api/config for the address it is open at, and in a sign-in code's
// reply for each address; nil where there is none.
type trustSeen struct {
	config    map[string]string
	addresses map[string]map[string]string // by url
}

// trustAsSeen asks the server, as a page open at host, what it says about
// trusting its CA. proxy is how a reverse proxy doing TLS reached it: "",
// "http" or "https".
func trustAsSeen(t *testing.T, config Config, host string, overTLS bool, proxy string) (trustSeen, string) {
	t.Helper()
	s, state := storetest.Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	for _, p := range agents.SeedProposals(nil) {
		s.Propose(p)
	}
	config.UIToken = token
	handler := NewServer(s, state, config, http.NotFoundHandler(), nil).Handler()
	ask := func(method, path string, into any) string {
		request := httptest.NewRequest(method, path, nil)
		request.Host, request.RemoteAddr = host, "192.168.1.20:51000"
		request.Header.Set("Authorization", "Bearer "+token)
		if proxy != "" {
			request.Header.Set("X-Forwarded-Proto", "https")
		}
		if overTLS {
			request.TLS = &tls.ConnectionState{}
		}
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s %s: %d %s", method, path, recorder.Code, recorder.Body)
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), into); err != nil {
			t.Fatal(err)
		}
		return recorder.Body.String()
	}
	var shown struct{ Trust map[string]string }
	var code struct {
		Addresses []struct {
			URL   string
			Trust map[string]string
		}
	}
	bodies := ask("GET", "/api/config", &shown) + ask("POST", "/api/signin/codes", &code)
	seen := trustSeen{config: shown.Trust, addresses: map[string]map[string]string{}}
	for _, a := range code.Addresses {
		seen.addresses[a.URL] = a.Trust
	}
	return seen, bodies
}

// A server serving -https with its own CA tells the page where to trust it,
// for the address the page is open at and for each address a sign-in code
// is for, with the CA's fingerprint: the trust page over plain HTTP, which
// it is served at too, at the same port. A server with no CA of its own,
// such as one behind a reverse proxy doing TLS, says nothing about it; nor
// does one with a CA for its public URL, which the proxy's certificate
// covers.
func TestThePageIsToldWhereToTrustTheServersOwnCA(t *testing.T) {
	ca, err := tlsca.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	fingerprint := ca.Fingerprint()
	trust := func(url string) map[string]string { return map[string]string{"url": url, "fingerprint": fingerprint} }
	withCA := Config{CA: string(ca.PEM()), NetworkURLs: []string{"https://192.168.1.5:7800", "https://agentry.lan"}, LocalURL: "https://localhost:7800"}
	withCAAndPublic := withCA
	withCAAndPublic.PublicURL = "https://agentry.example.com"
	for _, c := range []struct {
		name    string
		config  Config
		host    string
		overTLS bool
		proxy   string
		want    trustSeen
	}{
		{"-https, open at a network address", withCA, "192.168.1.5:7800", true, "", trustSeen{
			config: trust("http://192.168.1.5:7800/trust/"),
			addresses: map[string]map[string]string{"https://192.168.1.5:7800": trust("http://192.168.1.5:7800/trust/"),
				"https://agentry.lan": trust("https://agentry.lan/trust/"), "https://localhost:7800": trust("http://localhost:7800/trust/")}}},
		{"-https, open over plain HTTP at localhost", withCA, "localhost:7800", false, "", trustSeen{
			config: trust("http://localhost:7800/trust/"),
			addresses: map[string]map[string]string{"https://192.168.1.5:7800": trust("http://192.168.1.5:7800/trust/"),
				"https://agentry.lan": trust("https://agentry.lan/trust/"), "http://localhost:7800": trust("http://localhost:7800/trust/")}}},
		{"-https and a public URL, open through the proxy", withCAAndPublic, "agentry.example.com", false, "http", trustSeen{
			addresses: map[string]map[string]string{"https://agentry.example.com": nil, "https://192.168.1.5:7800": trust("http://192.168.1.5:7800/trust/"),
				"https://agentry.lan": trust("https://agentry.lan/trust/"), "https://localhost:7800": trust("http://localhost:7800/trust/")}}},
		{"no CA", Config{NetworkURLs: []string{"http://192.168.1.5:7800"}, LocalURL: "http://localhost:7800"}, "192.168.1.5:7800", false, "", trustSeen{
			addresses: map[string]map[string]string{"http://192.168.1.5:7800": nil, "http://localhost:7800": nil}}},
		{"-public-url behind a proxy doing TLS, no -https", Config{PublicURL: "https://agentry.example.com", LocalURL: "http://localhost:7800"}, "agentry.example.com", false, "http", trustSeen{
			addresses: map[string]map[string]string{"https://agentry.example.com": nil, "http://localhost:7800": nil}}},
	} {
		got, bodies := trustAsSeen(t, c.config, c.host, c.overTLS, c.proxy)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s:\n got %v\nwant %v", c.name, got, c.want)
		}
		if c.config.CA == "" && strings.Contains(bodies, `"trust"`) {
			t.Errorf("%s: a server with no CA of its own speaks of trust: %s", c.name, bodies)
		}
		if strings.Contains(bodies, token) || strings.Contains(bodies, "PRIVATE KEY") {
			t.Errorf("%s: a secret in %s", c.name, bodies)
		}
	}
}
