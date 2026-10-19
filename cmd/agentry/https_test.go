package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/pem"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/client"
	"gitlab.com/dev.chempa/agentry/internal/sharedfiles"
	"gitlab.com/dev.chempa/agentry/internal/storetest"
	"gitlab.com/dev.chempa/agentry/internal/tlsca"
	"gitlab.com/dev.chempa/agentry/internal/web"
)

// testCA is a CA in a fresh data folder, which it returns too.
func testCA(t *testing.T) (*tlsca.CA, string) {
	t.Helper()
	dataDir := t.TempDir()
	ca, err := tlsca.Open(tlsca.Dir(dataDir))
	if err != nil {
		t.Fatal(err)
	}
	return ca, dataDir
}

// serving serves handler on a free loopback port with -https's config, and
// stops it when the test ends. It returns the port's address.
func serving(t *testing.T, ca *tlsca.CA, handler http.Handler) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- serveUntilDone(ctx, listener, handler, tlsConfig(tlsca.NewIssuer(ca, func() []string { return nil }, time.Now)))
	}()
	t.Cleanup(func() {
		cancel()
		if err := <-done; err != nil {
			t.Errorf("serving: %v", err)
		}
	})
	return listener.Addr().String()
}

// trusting is a client that trusts only ca, and doesn't follow redirects.
func trusting(ca *tlsca.CA) *http.Client {
	return &http.Client{Timeout: 5 * time.Second,
		Transport:     &http.Transport{TLSClientConfig: &tls.Config{RootCAs: ca.Pool()}, ForceAttemptHTTP2: true},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func get(t *testing.T, c *http.Client, url string) (*http.Response, string) {
	t.Helper()
	response, err := c.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	return response, string(body)
}

// One port serves both: a TLS handshake goes to the TLS server, verified
// against the CA for 127.0.0.1 and localhost, over HTTP/2; anything else
// is plain HTTP.
func TestOnePortServesTLSAndPlainHTTP(t *testing.T) {
	ca, _ := testCA(t)
	address := serving(t, ca, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS != nil {
			io.WriteString(w, "tls "+r.Proto)
			return
		}
		io.WriteString(w, "plain")
	}))
	_, port, _ := net.SplitHostPort(address)
	c := trusting(ca)
	for _, host := range []string{"127.0.0.1", "localhost"} {
		if _, body := get(t, c, "https://"+host+":"+port+"/"); body != "tls HTTP/2.0" {
			t.Errorf("https at %s: %q", host, body)
		}
	}
	if _, body := get(t, c, "http://"+address+"/"); body != "plain" {
		t.Errorf("plain HTTP: %q", body)
	}
}

// A connection that sends nothing in time is closed, and holds up no other.
func TestASilentConnectionHoldsUpNoOther(t *testing.T) {
	ca, _ := testCA(t)
	address := serving(t, ca, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, "ok") }))
	silent, err := net.Dial("tcp", address)
	if err != nil {
		t.Fatal(err)
	}
	defer silent.Close()
	if _, body := get(t, trusting(ca), "http://"+address+"/"); body != "ok" {
		t.Errorf("beside a silent connection: %q", body)
	}
}

// Over plain HTTP, this machine is served as it is; another is sent to the
// same address over https, path and query kept. Over TLS everyone is served.
func TestTheFrontDoor(t *testing.T) {
	ca, _ := testCA(t)
	door := frontDoor(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, "served") }), ca)
	cases := []struct {
		name, from, path string
		overTLS          bool
		want             string // the body, or the redirect's Location
	}{
		{"this machine", "127.0.0.1:5000", "/api/config", false, "served"},
		{"this machine on IPv6", "[::1]:5000", "/api/config", false, "served"},
		{"this machine, IPv4 in IPv6", "[::ffff:127.0.0.1]:5000", "/", false, "served"},
		{"another machine", "192.168.1.20:5000", "/tasks/task-1?tab=log&x=1", false, "https://box:7800/tasks/task-1?tab=log&x=1"},
		{"another machine at the root", "100.64.0.7:5000", "/", false, "https://box:7800/"},
		{"another machine over TLS", "192.168.1.20:5000", "/api/config", true, "served"},
		{"the trust pages", "192.168.1.20:5000", "/trust/", false, "served"},
	}
	for _, c := range cases {
		r := httptest.NewRequest("GET", "http://box:7800"+c.path, nil)
		r.RemoteAddr = c.from
		if c.overTLS {
			r.TLS = &tls.ConnectionState{}
		}
		w := httptest.NewRecorder()
		door.ServeHTTP(w, r)
		got := w.Body.String()
		if w.Code == http.StatusPermanentRedirect {
			got = w.Header().Get("Location")
		}
		if got != c.want {
			t.Errorf("%s: %d %q, want %q", c.name, w.Code, got, c.want)
		}
	}
}

// The CA's certificate is at /trust/ca.crt and /trust/ca.pem, without the
// token, over plain HTTP from another machine and over https.
func TestTheCACertificateIsServedToAnyone(t *testing.T) {
	ca, dataDir := testCA(t)
	s, state := storetest.Open(t, filepath.Join(dataDir, "agentry.db"))
	server := web.NewServer(s, state, web.Config{UIToken: "0123456789abcdef0123456789abcdef"}, http.NotFoundHandler(), nil)
	door := frontDoor(server.Handler(), ca)
	for _, overTLS := range []bool{false, true} {
		for path, want := range map[string][]byte{"/trust/ca.crt": ca.DER(), "/trust/ca.pem": ca.PEM()} {
			r := httptest.NewRequest("GET", "http://box:7800"+path, nil)
			r.RemoteAddr = "192.168.1.20:5000"
			if overTLS {
				r.TLS = &tls.ConnectionState{}
			}
			w := httptest.NewRecorder()
			door.ServeHTTP(w, r)
			if w.Code != http.StatusOK || !bytes.Equal(w.Body.Bytes(), want) {
				t.Errorf("%s over TLS %v: %d, %d bytes, want the CA's %d", path, overTLS, w.Code, w.Body.Len(), len(want))
			}
		}
	}
	block, _ := pem.Decode(ca.PEM())
	if block == nil || !bytes.Equal(block.Bytes, ca.DER()) {
		t.Error("ca.pem and ca.crt hold different certificates")
	}
}

// No path, with the token or without, over plain HTTP or TLS, hands out the
// CA's key, however it climbs out of where it starts.
func TestTheCAKeyIsNeverServed(t *testing.T) {
	ca, dataDir := testCA(t)
	key, err := os.ReadFile(tlsca.KeyPath(dataDir))
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode(key)
	const token = "0123456789abcdef0123456789abcdef"
	s, state := storetest.Open(t, filepath.Join(dataDir, "agentry.db"))
	server := web.NewServer(s, state, web.Config{UIToken: token, Blobs: sharedfiles.Disk{Dir: filepath.Join(dataDir, "files")},
		Worktrees: filepath.Join(dataDir, "worktrees")}, http.NotFoundHandler(), nil)
	door := frontDoor(server.Handler(), ca)
	paths := []string{
		"/tls/ca.key", "/trust/ca.key", "/trust/../tls/ca.key", "/trust/..%2ftls/ca.key", "/trust/%2e%2e/tls/ca.key",
		"/trust/..%5ctls%5cca.key", "/trust//../../tls/ca.key", "/trust/ca.crt/../ca.key", "/trust/./../tls/ca.key",
		"/../tls/ca.key", "/static/../../tls/ca.key", "/assets/..%2f..%2ftls%2fca.key",
		"/api/tasks/task-1/files/..%2f..%2ftls%2fca.key", "/api/tasks/task-1/files/../../tls/ca.key",
	}
	for _, path := range paths {
		for _, from := range []string{"127.0.0.1:5000", "192.168.1.20:5000"} {
			for _, overTLS := range []bool{false, true} {
				for _, signedIn := range []bool{false, true} {
					r := httptest.NewRequest("GET", path, nil) // as sent, its .. kept
					r.Host, r.RemoteAddr = "box:7800", from
					if overTLS {
						r.TLS = &tls.ConnectionState{}
					}
					if signedIn {
						r.Header.Set("Authorization", "Bearer "+token)
					}
					w := httptest.NewRecorder()
					door.ServeHTTP(w, r)
					body := w.Body.Bytes()
					if bytes.Contains(body, key) || bytes.Contains(body, block.Bytes) || bytes.Contains(body, []byte("PRIVATE KEY")) {
						t.Errorf("%s from %s, TLS %v, signed in %v: the key was served", path, from, overTLS, signedIn)
					}
				}
			}
		}
	}
}

// The UI cookie is Secure when signing in over TLS.
func TestTheCookieIsSecureOverTLS(t *testing.T) {
	ca, dataDir := testCA(t)
	const token = "0123456789abcdef0123456789abcdef"
	s, state := storetest.Open(t, filepath.Join(dataDir, "agentry.db"))
	server := web.NewServer(s, state, web.Config{UIToken: token}, http.NotFoundHandler(), nil)
	address := serving(t, ca, frontDoor(server.Handler(), ca))
	c := trusting(ca)
	for url, secure := range map[string]bool{"https://" + address: true, "http://" + address: false} {
		response, _ := get(t, c, url+"/?token="+token)
		cookies := response.Cookies()
		if len(cookies) == 0 {
			t.Fatalf("%s: no cookie set (%d)", url, response.StatusCode)
		}
		for _, cookie := range cookies {
			if cookie.Secure != secure {
				t.Errorf("%s: cookie %s Secure %v, want %v", url, cookie.Name, cookie.Secure, secure)
			}
		}
	}
}

// Links people open are https with -https; what this machine's clients call
// stays plain HTTP on loopback.
func TestHTTPSLinks(t *testing.T) {
	every, _ := net.ResolveTCPAddr("tcp", "0.0.0.0:7800")
	if got := localURL(every, "https"); got != "https://127.0.0.1:7800" {
		t.Errorf("localURL = %s", got)
	}
	if got := localhostURL(every, "https"); got != "https://localhost:7800" {
		t.Errorf("localhostURL = %s", got)
	}
	for _, url := range networkURLs(every, "https") {
		if !strings.HasPrefix(url, "https://") {
			t.Errorf("a network URL: %s", url)
		}
	}
	if got := plainURLs([]string{"https://192.168.1.5:7800"}); got[0] != "http://192.168.1.5:7800" {
		t.Errorf("plainURLs = %v", got)
	}
}

// syncBuffer is a bytes.Buffer serve may write to while a test reads it.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// serve -https prints https links and the CA's fingerprint, and saves a
// plain-HTTP loopback address for this machine's clients, which it serves.
func TestServeHTTPS(t *testing.T) {
	dataDir, repo := t.TempDir(), t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	var out syncBuffer
	done := make(chan error, 1)
	go func() {
		done <- serve(ctx, serveOptions{address: "127.0.0.1:0", repo: repo, dataDir: dataDir, https: true}, &out)
	}()
	defer func() {
		cancel()
		if err := <-done; err != nil {
			t.Errorf("serve: %v", err)
		}
	}()
	for deadline := time.Now().Add(10 * time.Second); !strings.Contains(out.String(), "Trust it once"); time.Sleep(20 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("serve printed:\n%s", out.String())
		}
	}
	ca, err := tlsca.Open(tlsca.Dir(dataDir))
	if err != nil {
		t.Fatal(err)
	}
	saved, err := client.Local(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(saved.URL, "http://127.0.0.1:") || !strings.HasPrefix(saved.Page, "https://127.0.0.1:") || !saved.HTTPS {
		t.Errorf("saved %+v: want plain HTTP for clients and https for the page", saved.Where)
	}
	printed := out.String()
	for _, want := range []string{"Open " + saved.Page + "/?token=", "SHA-256 " + ca.Fingerprint(), saved.URL + "/trust/ca.crt"} {
		if !strings.Contains(printed, want) {
			t.Errorf("printed:\n%s\nwant it to say %q", printed, want)
		}
	}
	if err := saved.Call("GET", "/api/config", nil, nil); err != nil {
		t.Errorf("a client on loopback over plain HTTP: %v", err)
	}
	if response, _ := get(t, trusting(ca), saved.Page+"/api/config"); response.StatusCode != http.StatusUnauthorized {
		t.Errorf("https without the token: %d, want 401", response.StatusCode)
	}
}
