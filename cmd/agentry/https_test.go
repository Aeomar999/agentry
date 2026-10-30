package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"encoding/pem"
	"html/template"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/core/store"
	"gitlab.com/dev.chempa/agentry/internal/domain/agents"
	"gitlab.com/dev.chempa/agentry/internal/domain/workers"
	"gitlab.com/dev.chempa/agentry/internal/platform/client"
	"gitlab.com/dev.chempa/agentry/internal/platform/hookcall"
	"gitlab.com/dev.chempa/agentry/internal/platform/tlsca"
	"gitlab.com/dev.chempa/agentry/internal/platform/workspace"
	"gitlab.com/dev.chempa/agentry/internal/sharedfiles"
	"gitlab.com/dev.chempa/agentry/internal/testkit/storetest"
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

// doorOf is what the front door needs for ca, with a certificate naming
// hosts, besides localhost and loopback.
func doorOf(ca *tlsca.CA, hosts ...string) trust {
	issuer := tlsca.NewIssuer(ca, func() []string { return hosts }, time.Now)
	return trust{ca: ca, issuer: issuer, addresses: []string{"https://localhost:7800"}}
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

// Over plain HTTP, this machine is served as it is, on loopback or on the
// address it serves; another is sent to the same address over https, path
// and query kept, when the certificate names it. Over TLS everyone is
// served.
func TestTheFrontDoor(t *testing.T) {
	ca, _ := testCA(t)
	door := frontDoor(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, "served") }), doorOf(ca, "192.168.1.160"))
	cases := []struct {
		name, from, path string
		overTLS          bool
		want             string // the body, or the redirect's Location
		arrivedOn        string // the local address, when not loopback
	}{
		{"this machine", "127.0.0.1:5000", "/api/config", false, "served", ""},
		{"this machine on IPv6", "[::1]:5000", "/api/config", false, "served", ""},
		{"this machine, IPv4 in IPv6", "[::ffff:127.0.0.1]:5000", "/", false, "served", ""},
		{"another machine", "192.168.1.20:5000", "/tasks/task-1?tab=log&x=1", false, "https://192.168.1.160:7800/tasks/task-1?tab=log&x=1", ""},
		{"another machine at the root", "100.64.0.7:5000", "/", false, "https://192.168.1.160:7800/", ""},
		{"another machine over TLS", "192.168.1.20:5000", "/api/config", true, "served", ""},
		{"this machine on the LAN address it serves", "192.168.1.160:5000", "/api/config", false, "served", "192.168.1.160:7800"},
		{"this machine, IPv4 in IPv6 on its LAN address", "[::ffff:192.168.1.160]:5000", "/", false, "served", "192.168.1.160:7800"},
		{"another machine on that LAN address", "192.168.1.20:5000", "/api/config", false, "https://192.168.1.160:7800/api/config", "192.168.1.160:7800"},
	}
	for _, c := range cases {
		r := httptest.NewRequest("GET", "http://192.168.1.160:7800"+c.path, nil)
		r.RemoteAddr = c.from
		if c.arrivedOn != "" {
			r = r.WithContext(context.WithValue(r.Context(), http.LocalAddrContextKey, net.TCPAddrFromAddrPort(netip.MustParseAddrPort(c.arrivedOn))))
		}
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

// Another machine asking over plain HTTP at an address the certificate
// doesn't name, such as a global IPv6 address outside the CA's
// constraints or a name it was never issued for, isn't sent to https
// there, where the browser would refuse the certificate: it gets the trust
// page saying so, with the addresses that work.
func TestTheFrontDoorSendsNoOneWhereTheCertificateDoesNotReach(t *testing.T) {
	ca, _ := testCA(t)
	door := doorOf(ca, "192.168.1.160")
	door.addresses = []string{"https://192.168.1.160:7800", "https://localhost:7800"}
	handler := frontDoor(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, "served") }), door)
	for _, host := range []string{"[2600:1700::5]:7800", "elsewhere.example:7800", "203.0.113.9"} {
		r := httptest.NewRequest("GET", "/tasks/task-1", nil)
		r.Host, r.RemoteAddr = host, "192.168.1.20:5000"
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		body := w.Body.String()
		if w.Code != http.StatusMisdirectedRequest || w.Header().Get("Location") != "" || !strings.Contains(body, "cover <code>"+template.HTMLEscapeString(host)+"</code>") ||
			!strings.Contains(body, `href="https://192.168.1.160:7800/"`) || strings.Contains(body, "https://"+host) || strings.Contains(body, "localhost") {
			t.Errorf("at %s: %d, Location %q\n%s", host, w.Code, w.Header().Get("Location"), body)
		}
		// Asked at a deep path, its download links still reach the downloads.
		asked := &url.URL{Scheme: "http", Host: host, Path: "/tasks/task-1"}
		links := downloadLinks(t, body)
		if len(links) != 3 {
			t.Fatalf("at %s: download links %q", host, links)
		}
		for _, link := range links {
			target, err := asked.Parse(link)
			if err != nil {
				t.Fatal(err)
			}
			r := httptest.NewRequest("GET", target.RequestURI(), nil)
			r.Host, r.RemoteAddr = host, "192.168.1.20:5000"
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if want := downloadTypes[path.Ext(target.Path)]; w.Code != http.StatusOK || want == "" || w.Header().Get("Content-Type") != want {
				t.Errorf("at %s, %q is %s: %d %s", host, link, target.Path, w.Code, w.Header().Get("Content-Type"))
			}
		}
	}
}

// A Host whose port isn't a port number doesn't reach the redirect or the
// trust page's links: they use the port the request arrived on instead.
func TestTheFrontDoorTakesOnlyAPortNumberFromTheHost(t *testing.T) {
	ca, _ := testCA(t)
	handler := frontDoor(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, "served") }), doorOf(ca, "192.168.1.160"))
	for _, host := range []string{"192.168.1.160:1@evil.example", "192.168.1.160:80ab", "192.168.1.160:"} {
		for _, path := range []string{"/tasks/task-1", "/trust/"} {
			r := httptest.NewRequest("GET", path, nil)
			r.Host, r.RemoteAddr = host, "192.168.1.20:5000"
			r = r.WithContext(context.WithValue(r.Context(), http.LocalAddrContextKey, net.TCPAddrFromAddrPort(netip.MustParseAddrPort("192.168.1.160:7800"))))
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			out := w.Header().Get("Location") + w.Body.String()
			if !strings.Contains(out, "https://192.168.1.160:7800/") || strings.Contains(out, "evil") || strings.Contains(out, "80ab") || strings.Contains(out, "160:/") {
				t.Errorf("at %s%s: %d, Location %q\n%s", host, path, w.Code, w.Header().Get("Location"), w.Body)
			}
		}
	}
}

// A Host without a port asked at http's own port, 80, over plain HTTP, so
// it is sent on to https there, and not to https's own port, 443, which
// Agentry doesn't serve. That holds when 80 is forwarded to Agentry's real
// port too: the browser reached 80. Over TLS it asked at 443.
func TestAHostWithoutAPortIsSentToThePortItAskedAt(t *testing.T) {
	cases := []struct {
		name, arrivedOn string
		overTLS         bool
		want            string
	}{
		{"over plain HTTP", "192.168.1.160:80", false, "192.168.1.160:80"},
		{"over plain HTTP, forwarded to another port", "192.168.1.160:7800", false, "192.168.1.160:80"},
		{"over plain HTTP, arrival unknown", "", false, "192.168.1.160:80"},
		{"over TLS", "192.168.1.160:443", true, "192.168.1.160"},
	}
	for _, c := range cases {
		r := httptest.NewRequest("GET", "/tasks/task-1", nil)
		r.Host = "192.168.1.160"
		if c.arrivedOn != "" {
			r = r.WithContext(context.WithValue(r.Context(), http.LocalAddrContextKey, net.TCPAddrFromAddrPort(netip.MustParseAddrPort(c.arrivedOn))))
		}
		if c.overTLS {
			r.TLS = &tls.ConnectionState{}
		}
		if got := httpsHost(r); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
	ca, _ := testCA(t)
	handler := frontDoor(http.NotFoundHandler(), doorOf(ca, "192.168.1.160"))
	r := httptest.NewRequest("GET", "/tasks/task-1", nil)
	r.Host, r.RemoteAddr = "192.168.1.160", "192.168.1.20:5000"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if got := w.Header().Get("Location"); got != "https://192.168.1.160:80/tasks/task-1" {
		t.Errorf("sent to %q", got)
	}
}

// downloadTypes are the trust page's downloads' content types, by extension.
var downloadTypes = map[string]string{".crt": "application/x-x509-ca-cert", ".pem": "application/x-pem-file",
	".mobileconfig": "application/x-apple-aspen-config"}

// downloadLinks are the hrefs in the trust page's list of downloads.
func downloadLinks(t *testing.T, page string) []string {
	t.Helper()
	list := regexp.MustCompile(`(?s)<ul class="downloads">(.*?)</ul>`).FindStringSubmatch(page)
	if list == nil {
		t.Fatalf("no downloads in\n%s", page)
	}
	var links []string
	for _, href := range regexp.MustCompile(`href="([^"]*)"`).FindAllStringSubmatch(list[1], -1) {
		links = append(links, href[1])
	}
	return links
}

// The CA's certificate is at /trust/ca.crt and /trust/ca.pem, without the
// token, over plain HTTP from another machine and over https.
func TestTheCACertificateIsServedToAnyone(t *testing.T) {
	ca, dataDir := testCA(t)
	s, state := storetest.Open(t, filepath.Join(dataDir, "agentry.db"))
	server := web.NewServer(s, state, web.Config{UIToken: "0123456789abcdef0123456789abcdef"}, http.NotFoundHandler(), nil)
	door := frontDoor(server.Handler(), doorOf(ca))
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
	server := web.NewServer(s, state, web.Config{UIToken: token, DefaultRepo: t.TempDir(), Blobs: sharedfiles.Disk{Dir: filepath.Join(dataDir, "files")},
		Worktrees: filepath.Join(dataDir, "worktrees")}, http.NotFoundHandler(), nil)
	door := frontDoor(server.Handler(), doorOf(ca))
	paths := []string{
		"/tls/ca.key", "/trust/ca.key", "/trust/../tls/ca.key", "/trust/..%2ftls/ca.key", "/trust/%2e%2e/tls/ca.key",
		"/trust/..%5ctls%5cca.key", "/trust//../../tls/ca.key", "/trust/ca.crt/../ca.key", "/trust/./../tls/ca.key",
		"/../tls/ca.key", "/static/../../tls/ca.key", "/assets/..%2f..%2ftls%2fca.key",
		"/trust/", "/trust", "/trust/ca.crt", "/trust/ca.pem", "/trust/agentry.mobileconfig",
	}
	// task-1's worktree is dataDir/worktrees/task-1, two folders below the
	// data folder, and holds a link to the key.
	worktree := withAWorktree(t, server, s, token, filepath.Join(dataDir, "worktrees", "task-1"))
	if err := os.Symlink(tlsca.KeyPath(dataDir), filepath.Join(worktree, "key")); err != nil {
		t.Fatal(err)
	}
	read := httptest.NewRequest("GET", "/api/tasks/task-1/file?path=notes.md", nil)
	read.Header.Set("Authorization", "Bearer "+token)
	read.TLS = &tls.ConnectionState{}
	w := httptest.NewRecorder()
	door.ServeHTTP(w, read)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "notes") {
		t.Fatalf("a file in the worktree: %d %s", w.Code, w.Body)
	}
	for _, climb := range []string{
		"../../tls/ca.key", "../../../tls/ca.key", "../../../../tls/ca.key", "..%2f..%2ftls%2fca.key", "%2e%2e/%2e%2e/tls/ca.key",
		"..\\..\\tls\\ca.key", "./../../tls/ca.key", url.QueryEscape(tlsca.KeyPath(dataDir)), "key", "./key",
	} {
		paths = append(paths, "/api/tasks/task-1/file?path="+climb,
			"/api/tasks/task-1/shared/file?name="+climb, "/api/tasks/task-1/shared/file?path="+climb)
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
					if signedIn && strings.HasPrefix(path, "/api/") && !refused(w) {
						t.Errorf("%s from %s, TLS %v: %d %s, want it refused", path, from, overTLS, w.Code, body)
					}
				}
			}
		}
	}
}

// refused is whether w hands out nothing from outside the worktree: an
// error, the front door's redirect to https, or an empty read, since a file
// route keeps a climbing path inside the worktree, where no such file is.
func refused(w *httptest.ResponseRecorder) bool {
	switch w.Code {
	case http.StatusOK:
		var read workspace.Contents
		return json.Unmarshal(w.Body.Bytes(), &read) == nil && read.Now == "" && read.Data == "" && !read.TooBig
	case http.StatusPermanentRedirect:
		return strings.HasPrefix(w.Header().Get("Location"), "https://")
	default:
		return w.Code >= 400
	}
}

// withAWorktree creates task-1 on server and makes dir, with a file in it,
// its worktree on this machine. It returns dir.
func withAWorktree(t *testing.T, server *web.Server, s *store.Store, token, dir string) string {
	t.Helper()
	for _, p := range agents.SeedProposals(nil) {
		s.Propose(p)
	}
	r := httptest.NewRequest("POST", "/api/tasks", strings.NewReader(`{"title":"Read files"}`))
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("creating task-1: %d %s", w.Code, w.Body)
	}
	if _, rejection, err := s.Propose(events.Proposal{Stream: "task-1", Actor: events.Actor{Kind: events.Worker, ID: workers.Local},
		Type: events.WorktreeReady, Data: map[string]any{"path": dir, "branch": "b", "port": 7901}}); rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.md"), []byte("notes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// The UI cookie is Secure when signing in over TLS.
func TestTheCookieIsSecureOverTLS(t *testing.T) {
	ca, dataDir := testCA(t)
	const token = "0123456789abcdef0123456789abcdef"
	s, state := storetest.Open(t, filepath.Join(dataDir, "agentry.db"))
	server := web.NewServer(s, state, web.Config{UIToken: token}, http.NotFoundHandler(), nil)
	address := serving(t, ca, frontDoor(server.Handler(), doorOf(ca)))
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

// serve -https prints https links and the CA's fingerprint, and saves an
// https loopback address and the CA for this machine's clients, which reach
// it trusting only that CA. Plain HTTP on loopback still works.
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
	if !strings.HasPrefix(saved.URL, "https://127.0.0.1:") || saved.Page != saved.URL || !saved.HTTPS || saved.CA != string(ca.PEM()) {
		t.Errorf("saved %+v: want https on loopback for clients and the page, with the CA", saved.Where)
	}
	plain := "http://" + strings.TrimPrefix(saved.URL, "https://")
	printed := out.String()
	for _, want := range []string{"Open " + saved.Page + "/?token=", "SHA-256 " + ca.Fingerprint(), "device: " + plain + "/trust/\n"} {
		if !strings.Contains(printed, want) {
			t.Errorf("printed:\n%s\nwant it to say %q", printed, want)
		}
	}
	// `agentry mcp` and the status line find it so, when this machine wasn't
	// connected to a server from its Integrations page.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	mcp, err := connect("", "", "", dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := mcp.Call("GET", "/api/config", nil, nil); err != nil {
		t.Errorf("a client over https with the CA: %v", err)
	}
	unpinned := mcp
	unpinned.CA = ""
	if err := unpinned.Call("GET", "/api/config", nil, nil); err == nil {
		t.Error("a client without the CA trusted the server: the system store shouldn't")
	}
	old := mcp
	old.URL, old.CA = plain, ""
	if err := old.Call("GET", "/api/config", nil, nil); err != nil {
		t.Errorf("a client on loopback over plain HTTP: %v", err)
	}
	if response, _ := get(t, trusting(ca), saved.Page+"/api/config"); response.StatusCode != http.StatusUnauthorized {
		t.Errorf("https without the token: %d, want 401", response.StatusCode)
	}

	// A hook, as a session runs it: the server and CA from its environment.
	hookToken, err := os.ReadFile(filepath.Join(dataDir, hookTokenFile))
	if err != nil {
		t.Fatal(err)
	}
	hookEnv(t, saved.URL)
	t.Setenv(hookcall.EnvToken, strings.TrimSpace(string(hookToken)))
	t.Setenv(hookcall.EnvDeadline, "5s")
	t.Setenv(hookcall.EnvCA, saved.CA)
	if _, _, stderr := runHookWith(`{}`, "PreToolUse"); strings.Contains(stderr, "fail closed") {
		t.Errorf("the hook over https with the CA: %s", stderr)
	}
	t.Setenv(hookcall.EnvCA, "")
	if code, _, stderr := runHookWith(`{}`, "PreToolUse"); code != 2 || !strings.Contains(stderr, "certificate") {
		t.Errorf("the hook without the CA: exit %d, %s; want it refused", code, stderr)
	}
}
