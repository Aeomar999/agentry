package workersapi_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/core/httpapi"
	"gitlab.com/dev.chempa/agentry/internal/core/store"
	"gitlab.com/dev.chempa/agentry/internal/domain/workers"
	"gitlab.com/dev.chempa/agentry/internal/domain/workers/workerlink"
	"gitlab.com/dev.chempa/agentry/internal/platform/tlsca"
	"gitlab.com/dev.chempa/agentry/internal/runtime/machines"
	"gitlab.com/dev.chempa/agentry/internal/testkit/storetest"
	"gitlab.com/dev.chempa/agentry/internal/web"
)

// The routes a machine calls with its join code and no token: the install
// scripts, agentry for its system, and joining. Each checks the code
// itself, so each must refuse a bad one.

// scripts is the folder install.sh and install.ps1 are in.
const scripts = "../../../web/"

// joinServer is web's server with a hub for workers and config on top, and
// its log, to make join codes in.
func joinServer(t *testing.T, config web.Config) (*httptest.Server, *store.Store) {
	t.Helper()
	s, state := storetest.Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	fleet := machines.NewFleet(&machines.Local{Label: workers.Local})
	config.UIToken, config.Fleet, config.Workers = token, fleet, workerlink.NewServer(s, fleet)
	config.Binary = filepath.Join(t.TempDir(), "agentry")
	os.WriteFile(config.Binary, []byte("the agentry binary"), 0o755)
	server := httptest.NewServer(web.NewServer(s, state, config, http.NotFoundHandler(), nil).Handler())
	t.Cleanup(server.Close)
	return server, s
}

// joinCode is a code made at made, so it expired an hour after.
func joinCode(t *testing.T, s *store.Store, made time.Time) string {
	t.Helper()
	code, p := workers.Invite(httpapi.You, made)
	if _, rejection, err := s.Propose(p); err != nil || rejection != nil {
		t.Fatal(rejection, err)
	}
	return code
}

// usedCode is a code a machine has joined with already.
func usedCode(t *testing.T, server *httptest.Server, s *store.Store) string {
	t.Helper()
	code := joinCode(t, s, time.Now())
	if status, body := send(t, server, "POST /api/worker/join", `{"code":"`+code+`","name":"first"}`); status != 200 {
		t.Fatalf("joining: %d %s", status, body)
	}
	return code
}

// send calls route with body and no token, as a joining machine does.
func send(t *testing.T, server *httptest.Server, route, body string) (int, string) {
	t.Helper()
	response := sent(t, server, route, body)
	defer response.Body.Close()
	content, _ := io.ReadAll(response.Body)
	return response.StatusCode, string(content)
}

func sent(t *testing.T, server *httptest.Server, route, body string) *http.Response {
	t.Helper()
	method, path, _ := strings.Cut(route, " ")
	request, _ := http.NewRequest(method, server.URL+path, strings.NewReader(body))
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

// The refusals, word for word, of each route a joining machine calls.
var refusals = []struct{ route, body, refusal string }{
	{"GET /worker/install.sh?code=", "", "echo 'That join code is unknown, used or expired. Make a new one on the Workers page.' >&2; exit 1\n"},
	{"GET /worker/install.ps1?code=", "", "Write-Error 'That join code is unknown, used or expired. Make a new one on the Workers page.'; exit 1\n"},
	{"GET /worker/agentry?os=" + runtime.GOOS + "&arch=" + runtime.GOARCH + "&code=", "", "that join code is unknown, used or expired\n"},
	{"POST /api/worker/join", `{"code":"%s","name":"second"}`, "that join code is unknown, used or expired; make a new one on the Workers page\n"},
}

// Every route refuses no code, a wrong one, a used one and an expired one
// alike: 401, with the same words.
func TestTheJoinRoutesRefuseACodeThatDoesNotWork(t *testing.T) {
	server, s := joinServer(t, web.Config{})
	for name, code := range map[string]string{
		"no code":      "",
		"a wrong code": "wrong",
		"a used code":  usedCode(t, server, s),
		"an expired":   joinCode(t, s, time.Now().Add(-workers.InviteLifetime-time.Minute)),
	} {
		for _, r := range refusals {
			route, body := r.route+code, r.body
			if body != "" {
				route, body = r.route, strings.Replace(body, "%s", code, 1)
			}
			if status, got := send(t, server, route, body); status != http.StatusUnauthorized || got != r.refusal {
				t.Errorf("%s with %s: %d %q, want 401 %q", r.route, name, status, got, r.refusal)
			}
		}
	}
	if status, got := send(t, server, "POST /api/worker/join", "not json"); status != http.StatusBadRequest ||
		got != "bad request: invalid character 'o' in literal null (expecting 'u')\n" {
		t.Errorf("joining with a body that isn't JSON: %d %q", status, got)
	}
}

// A server with no hub for workers has none of these routes: a GET gets
// what the page's GET / gives any path it doesn't know, and the POST is
// 405, as no route takes it.
func TestWithNoHubTheJoinRoutesAreNotThere(t *testing.T) {
	s, state := storetest.Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	server := httptest.NewServer(web.NewServer(s, state, web.Config{UIToken: token}, http.NotFoundHandler(), nil).Handler())
	t.Cleanup(server.Close)
	code := joinCode(t, s, time.Now())
	unknownStatus, unknown := send(t, server, "GET /worker/nothing-here", "")
	for _, r := range refusals {
		route, body, wantStatus, want := r.route+code, r.body, unknownStatus, unknown
		if body != "" {
			route, body = r.route, strings.Replace(body, "%s", code, 1)
			wantStatus, want = http.StatusMethodNotAllowed, "Method Not Allowed\n"
		}
		if status, got := send(t, server, route, body); status != wantStatus || got != want {
			t.Errorf("%s with no hub: %d %q, want %d %q", r.route, status, got, wantStatus, want)
		}
	}
}

// The scripts come filled with the server's address and the code, and,
// for a server with its own CA, the CA and its fingerprint; the rest of
// the script is as written.
func TestTheInstallScriptsAreFilledIn(t *testing.T) {
	ca, err := tlsca.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	pem := strings.TrimSpace(string(ca.PEM()))
	for _, c := range []struct {
		name            string
		config          web.Config
		ca, fingerprint string
	}{
		{"no CA", web.Config{}, "", ""},
		{"its own CA", web.Config{CA: string(ca.PEM())}, pem, ca.Fingerprint()},
	} {
		server, s := joinServer(t, c.config)
		code := joinCode(t, s, time.Now())
		for _, script := range []struct{ file, kind, quote string }{
			{"install.sh", "text/x-shellscript", `'\''`},
			{"install.ps1", "text/plain; charset=utf-8", "''"},
		} {
			template, err := os.ReadFile(scripts + script.file)
			if err != nil {
				t.Fatal(err)
			}
			quoted := func(value string) string { return "'" + strings.ReplaceAll(value, "'", script.quote) + "'" }
			want := strings.NewReplacer("{{SERVER}}", quoted(server.URL), "{{CODE}}", quoted(code),
				"{{CA}}", quoted(c.ca), "{{CA_FINGERPRINT}}", quoted(c.fingerprint)).Replace(string(template))
			response := sent(t, server, "GET /worker/"+script.file+"?code="+code, "")
			got, _ := io.ReadAll(response.Body)
			response.Body.Close()
			if response.StatusCode != 200 || string(got) != want {
				t.Errorf("%s, %s: %d\n%s\nwant\n%s", c.name, script.file, response.StatusCode, got, want)
			}
			if kind := response.Header.Get("Content-Type"); kind != script.kind {
				t.Errorf("%s, %s: Content-Type %q, want %q", c.name, script.file, kind, script.kind)
			}
		}
	}
}

// With a code that works, agentry for the machine's system comes down, or
// a 404 for a system the server has no agentry for.
func TestAJoiningMachineDownloadsAgentry(t *testing.T) {
	server, s := joinServer(t, web.Config{})
	code := joinCode(t, s, time.Now())
	if status, got := send(t, server, "GET /worker/agentry?code="+code+"&os="+runtime.GOOS+"&arch="+runtime.GOARCH, ""); status != 200 || got != "the agentry binary" {
		t.Errorf("agentry for this system: %d %q", status, got)
	}
	if status, got := send(t, server, "GET /worker/agentry?code="+code+"&os=plan9&arch=mips", ""); status != http.StatusNotFound ||
		got != "this server has no agentry for plan9/mips\n" {
		t.Errorf("agentry for plan9/mips: %d %q", status, got)
	}
}

// A machine is named what it asked for, or after its host, made into a
// name the rules take; a name taken, this machine's included, gets -2,
// -3 and so on.
func TestAJoiningMachineIsNamedAfterWhatItAskedFor(t *testing.T) {
	server, s := joinServer(t, web.Config{})
	for _, c := range []struct{ asked, host, want string }{
		{"Laptop", "", "laptop"},
		{"", "Laptop.lan", "laptop-2"},
		{"laptop", "desk", "laptop-3"},
		{"My Big_Box!", "", "my-big-box"},
		{"", "Mac Mini.local", "mac-mini"},
		{"2nd", "", "worker-2nd"},
		{"", "", "worker"},
		{"--", "-.lan", "worker-2"},
		{"local", "", "local-2"},
		{"a-very-long-name-for-one-machine", "", "a-very-long-name-for-one-mac"},
		{"twenty-seven-letters-long-x-y", "", "twenty-seven-letters-long-x"},
	} {
		body, _ := json.Marshal(map[string]any{"code": joinCode(t, s, time.Now()), "name": c.asked, "machine": map[string]string{"host": c.host}})
		status, got := send(t, server, "POST /api/worker/join", string(body))
		var joined struct{ Name, Credential string }
		json.Unmarshal([]byte(got), &joined)
		if status != 200 || joined.Name != c.want || joined.Credential == "" {
			t.Errorf("asked %q on host %q: %d %s, want the name %q", c.asked, c.host, status, got, c.want)
		}
	}
}
