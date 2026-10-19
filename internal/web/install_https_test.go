package web

import (
	"bytes"
	"cmp"
	"crypto/tls"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/machines"
	"gitlab.com/dev.chempa/agentry/internal/pinned"
	"gitlab.com/dev.chempa/agentry/internal/storetest"
	"gitlab.com/dev.chempa/agentry/internal/tlsca"
	"gitlab.com/dev.chempa/agentry/internal/workerlink"
	"gitlab.com/dev.chempa/agentry/internal/workers"
)

// The commands an -https server hands out check its CA by fingerprint
// before they send the code; those for a public URL, or for a server with
// no CA of its own, are as they always were.
func TestTheCommandsPinTheServersOwnCA(t *testing.T) {
	ca, err := tlsca.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	hash := strings.ToLower(strings.ReplaceAll(ca.Fingerprint(), ":", ""))
	network := []string{"https://192.168.1.5:7800"}
	for _, c := range []struct {
		name    string
		config  Config
		host    string
		proxied bool            // the request came through a reverse proxy doing TLS
		pinned  map[string]bool // each address: are its commands pinned?
	}{
		{"open at localhost", Config{CA: string(ca.PEM()), NetworkURLs: network, LocalURL: "https://localhost:7800"}, "localhost:7800", false,
			map[string]bool{network[0]: true, "https://localhost:7800": true}},
		{"open at a name", Config{CA: string(ca.PEM()), NetworkURLs: network, LocalURL: "https://localhost:7800"}, "agentry.lan:7800", false,
			map[string]bool{network[0]: true, "https://agentry.lan:7800": true, "https://localhost:7800": true}},
		{"a public URL beside", Config{CA: string(ca.PEM()), PublicURL: "https://agentry.example.com", NetworkURLs: network}, "agentry.example.com", true,
			map[string]bool{"https://agentry.example.com": false, network[0]: true}},
		{"behind a proxy with no public URL", Config{CA: string(ca.PEM()), NetworkURLs: network}, "agentry.example.com", true,
			map[string]bool{network[0]: true, "https://agentry.example.com": false}},
		{"no CA", Config{NetworkURLs: []string{"http://192.168.1.5:7800"}}, "localhost:7800", false,
			map[string]bool{"http://192.168.1.5:7800": false, "http://localhost:7800": false}},
	} {
		request := httptest.NewRequest("POST", "/api/workers/invite", nil)
		request.Host = c.host
		if c.proxied {
			request.Header.Set("X-Forwarded-Proto", "https")
		} else if c.config.CA != "" {
			request.TLS = &tls.ConnectionState{}
		}
		answer := (&Server{config: c.config}).installCommands(request, "/worker/install", "c0de")
		want := func(url string) map[string]any {
			if c.pinned[url] {
				return commands(url, "/worker/install", "c0de", ca.Fingerprint())
			}
			return commands(url, "/worker/install", "c0de", "")
		}
		all := answer["addresses"].([]map[string]any)
		if len(all) != len(c.pinned) {
			t.Errorf("%s: %d addresses, want %d: %v", c.name, len(all), len(c.pinned), all)
		}
		for _, each := range append(all, answer) {
			url := cmp.Or(each["url"], answer["address"]).(string)
			if each["command"] != want(url)["command"] || each["windowsCommand"] != want(url)["windowsCommand"] {
				t.Errorf("%s, %s (pinned: %v): %q", c.name, url, c.pinned[url], each["command"])
			}
		}
	}

	pinned := commands("https://192.168.1.5:7800", "/worker/install", "c0de", ca.Fingerprint())
	if want := "sh -c " + shellQuoted(pinnedSh) + " agentry 'https://192.168.1.5:7800' '" + hash + "' 'https://192.168.1.5:7800/worker/install.sh?code=c0de'"; pinned["command"] != want {
		t.Errorf("the sh command:\n %s\nwant\n %s", pinned["command"], want)
	}
	if want := "& { " + pinnedPowerShell + " } 'https://192.168.1.5:7800' '" + hash + "' 'https://192.168.1.5:7800/worker/install.ps1?code=c0de'"; pinned["windowsCommand"] != want {
		t.Errorf("the PowerShell command:\n %s\nwant\n %s", pinned["windowsCommand"], want)
	}
	if strings.Contains(pinnedSh, "'") {
		t.Error("pinnedSh has a single quote, which would end the sh -c quote")
	}
	// A plain HTTP address of an -https server, as this machine opens it,
	// gets the https one in its pinned command.
	local := httptest.NewRequest("POST", "/api/workers/invite", nil)
	local.Host = "localhost:7800"
	if got := (&Server{config: Config{CA: string(ca.PEM())}}).installCommands(local, "/worker/install", "c0de"); !strings.Contains(got["command"].(string), "'https://localhost:7800/worker/install.sh?code=c0de'") {
		t.Errorf("at http://localhost: %s", got["command"])
	}
}

// httpsServer is the web server over https with Agentry's own CA, which it
// hands out at /trust/ca.crt as serve's front door does, with a fake agentry
// to download that writes how it was run to the file in AGENTRY_TEST_LOG.
// It records every request it is sent.
func httpsServer(t *testing.T) (ca *tlsca.CA, url string, requests func() []string) {
	t.Helper()
	ca, err := tlsca.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s, state := storetest.Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	binary := filepath.Join(t.TempDir(), "agentry")
	fake := []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >>\"$AGENTRY_TEST_LOG\"\n")
	os.WriteFile(binary, fake, 0o755)
	os.WriteFile(filepath.Join(filepath.Dir(binary), "agentry-windows-amd64"), fake, 0o755)
	fleet := machines.NewFleet(&machines.Local{Label: workers.Local, WorktreeRoot: t.TempDir()})
	web := NewServer(s, state, Config{UIToken: token, CA: string(ca.PEM()), Fleet: fleet, Workers: workerlink.NewServer(s, fleet),
		Binary: binary}, http.NotFoundHandler(), nil).Handler()
	url, requests = serveTLS(t, ca, ca, web)
	return ca, url, requests
}

// serveTLS serves next over https with a certificate issuedBy issues,
// handing out handedOut at /trust/ca.crt, and records every request.
func serveTLS(t *testing.T, issuedBy, handedOut *tlsca.CA, next http.Handler) (url string, requests func() []string) {
	t.Helper()
	var mu sync.Mutex
	var seen []string
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.URL.RequestURI())
		mu.Unlock()
		if r.URL.Path == "/trust/ca.crt" {
			w.Write(handedOut.DER())
			return
		}
		next.ServeHTTP(w, r)
	}))
	issuer := tlsca.NewIssuer(issuedBy, func() []string { return nil }, time.Now)
	server.Listener = tls.NewListener(server.Listener, &tls.Config{GetCertificate: issuer.GetCertificate})
	server.Config.ErrorLog = discard // the handshakes the pinned commands refuse
	server.Start()
	t.Cleanup(server.Close)
	return "https://" + server.Listener.Addr().String(), func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), seen...)
	}
}

var discard = log.New(io.Discard, "", 0)

// impostor is a server that hands out ca, as a server in between could,
// but can't answer with a certificate ca issued.
func impostor(t *testing.T, ca *tlsca.CA) (url string, requests func() []string) {
	other, err := tlsca.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return serveTLS(t, other, ca, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("echo the impostor's script\n"))
	}))
}

// invite asks the server at url, trusting ca, for an install command.
func invite(t *testing.T, ca *tlsca.CA, url, path string) (code string, answer map[string]any) {
	t.Helper()
	request, _ := http.NewRequest("POST", url+path, nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := pinned.Client(ca.PEM(), 10*time.Second).Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	json.NewDecoder(response.Body).Decode(&answer)
	command, _ := answer["command"].(string)
	_, after, _ := strings.Cut(command, "code=")
	code, _, _ = strings.Cut(after, "'")
	if code == "" {
		t.Fatalf("no code in %v", answer)
	}
	return code, answer
}

// runShell runs a command as pasted into a terminal, in a home of its own,
// and gives back what it printed and what the fake agentry was asked.
func runShell(t *testing.T, shell []string, command string) (out string, ran []string, err error) {
	t.Helper()
	home := t.TempDir()
	log := filepath.Join(home, "agentry.log")
	run := exec.Command(shell[0], append(shell[1:], command)...)
	run.Env = append(os.Environ(), "HOME="+home, "SHELL=/bin/sh", "AGENTRY_BIN="+filepath.Join(home, "bin"),
		"AGENTRY_TEST_LOG="+log, "AGENTRY_WORKER_NAME=")
	var output bytes.Buffer
	run.Stdout, run.Stderr = &output, &output
	err = run.Run()
	logged, _ := os.ReadFile(log)
	return output.String(), strings.Split(strings.TrimSpace(string(logged)), "\n"), err
}

func withCode(requests []string) []string {
	var sent []string
	for _, r := range requests {
		if strings.Contains(r, "code=") {
			sent = append(sent, r)
		}
	}
	return sent
}

// The command the Workers page hands out, run in sh against a server with
// its own CA that this machine doesn't trust: it checks the CA, gets the
// script and agentry trusting that CA alone, and joins with its
// fingerprint. With another fingerprint, nothing carrying the code is sent.
func TestThePinnedInstallCommandsRunInSh(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the sh commands are for macOS and Linux")
	}
	for _, tool := range []string{"sh", "curl"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip("no " + tool)
		}
	}
	ca, url, requests := httpsServer(t)
	for _, c := range []struct{ page, script, ran string }{
		{"/api/workers/invite", "/worker/install.sh", "worker join -ca-fingerprint " + ca.Fingerprint() + " " + url + " "},
		{"/api/integrations/claude-code", "/integrations/claude-code.sh", "mcp connect -ca-fingerprint " + ca.Fingerprint() + " " + url + " "},
	} {
		code, answer := invite(t, ca, url, c.page)
		out, ran, err := runShell(t, []string{"sh", "-c"}, answer["command"].(string))
		if err != nil || len(ran) == 0 || ran[0] != c.ran+code {
			t.Fatalf("%s: %v\n%s\nagentry ran: %q", c.script, err, out, ran)
		}
		sent := withCode(requests())
		if len(sent) < 2 || !strings.HasPrefix(sent[len(sent)-2], c.script+"?code="+code) || !strings.Contains(sent[len(sent)-1], "/agentry?code="+code) {
			t.Errorf("%s: the requests with the code: %v", c.script, sent)
		}
	}

	other, _ := tlsca.Open(t.TempDir())
	code, _ := invite(t, ca, url, "/api/workers/invite")
	before := len(withCode(requests()))
	wrong := commands(url, "/worker/install", code, other.Fingerprint())["command"].(string)
	out, ran, err := runShell(t, []string{"sh", "-c"}, wrong)
	if err == nil || !strings.Contains(out, "another CA than the one this command trusts") || ran[0] != "" {
		t.Errorf("another fingerprint: %v\n%s\nagentry ran: %q", err, out, ran)
	}
	if sent := withCode(requests()); len(sent) != before {
		t.Errorf("the code was sent to a server with another CA: %v", sent[before:])
	}

	// An impostor handing out the same CA gets nothing carrying the code.
	// (Like curl piped to sh anywhere, the command then exits 0.)
	fake, faked := impostor(t, ca)
	out, ran, _ = runShell(t, []string{"sh", "-c"}, commands(fake, "/worker/install", code, ca.Fingerprint())["command"].(string))
	if !strings.Contains(out, "certificate") || ran[0] != "" || strings.Contains(out, "impostor") || len(withCode(faked())) != 0 {
		t.Errorf("an impostor with the CA: %v\n%s\nits requests: %v", err, out, faked())
	}
}

// The PowerShell command, where pwsh and curl.exe are found: on Windows,
// or elsewhere with a curl.exe that runs curl. Agentry for Windows can't
// run here, so this checks the command and the download, not the join.
func TestThePinnedInstallCommandRunsInPowerShell(t *testing.T) {
	for _, tool := range []string{"pwsh", "curl.exe"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip("no " + tool)
		}
	}
	ca, url, requests := httpsServer(t)
	code, answer := invite(t, ca, url, "/api/workers/invite")
	pwsh := []string{"pwsh", "-NoProfile", "-NonInteractive", "-Command"}
	out, _, _ := runShell(t, pwsh, answer["windowsCommand"].(string))
	sent := withCode(requests())
	if len(sent) != 2 || !strings.HasPrefix(sent[0], "/worker/install.ps1?code="+code) || !strings.Contains(sent[1], "/worker/agentry?code="+code+"&os=windows") {
		t.Errorf("the requests with the code: %v\n%s", sent, out)
	}

	other, _ := tlsca.Open(t.TempDir())
	wrong := commands(url, "/worker/install", code, other.Fingerprint())["windowsCommand"].(string)
	out, _, _ = runShell(t, pwsh, wrong)
	if !strings.Contains(out, "another CA than the one this command trusts") {
		t.Errorf("another fingerprint:\n%s", out)
	}
	if again := withCode(requests()); len(again) != len(sent) {
		t.Errorf("the code was sent to a server with another CA: %v", again[len(sent):])
	}

	fake, faked := impostor(t, ca)
	out, _, _ = runShell(t, pwsh, commands(fake, "/worker/install", code, ca.Fingerprint())["windowsCommand"].(string))
	if strings.Contains(out, "impostor") || len(withCode(faked())) != 0 {
		t.Errorf("an impostor with the CA:\n%s\nits requests: %v", out, faked())
	}
}

// The scripts of a server with its own CA carry it, for their download, and
// its fingerprint, for joining; served for a public URL or with no CA,
// they carry neither.
func TestTheScriptsCarryTheCA(t *testing.T) {
	ca, url, _ := httpsServer(t)
	client := pinned.Client(ca.PEM(), 10*time.Second)
	code, _ := invite(t, ca, url, "/api/workers/invite")
	connectCode, _ := invite(t, ca, url, "/api/integrations/claude-code")
	script := func(path string) string {
		response, err := client.Get(url + path)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		body, _ := io.ReadAll(response.Body)
		return string(body)
	}
	pem := strings.TrimSpace(string(ca.PEM()))
	for path, wants := range map[string][]string{
		"/worker/install.sh?code=" + code: {"ca='" + pem + "'", "ca_fingerprint='" + ca.Fingerprint() + "'",
			`curl -fsSL ${cacert:+--cacert "$cacert"} "$server/worker/agentry`, `worker join ${ca_fingerprint:+-ca-fingerprint "$ca_fingerprint"}`},
		"/worker/install.ps1?code=" + code: {"$ca = '" + pem + "'", "$caFingerprint = '" + ca.Fingerprint() + "'",
			"curl.exe -fsSL --cacert $caFile", `Save-Download "$server/worker/agentry`, "$join += '-ca-fingerprint', $caFingerprint"},
		"/integrations/claude-code.sh?code=" + connectCode: {"ca='" + pem + "'", "ca_fingerprint='" + ca.Fingerprint() + "'",
			`curl -fsSL ${cacert:+--cacert "$cacert"} "$server/integrations/agentry`, `mcp connect ${ca_fingerprint:+-ca-fingerprint "$ca_fingerprint"}`},
		"/integrations/claude-code.ps1?code=" + connectCode: {"$ca = '" + pem + "'", "$caFingerprint = '" + ca.Fingerprint() + "'",
			`Save-Download "$server/integrations/agentry`, "$connect += '-ca-fingerprint', $caFingerprint"},
	} {
		got := script(path)
		for _, want := range wants {
			if !strings.Contains(got, want) {
				t.Errorf("%s has no %q:\n%s", path, want, got)
			}
		}
	}

	for _, c := range []struct {
		name   string
		config Config
		tls    bool
	}{
		{"a public URL", Config{CA: string(ca.PEM()), PublicURL: "https://agentry.example.com"}, true},
		{"no CA", Config{}, false},
	} {
		request := httptest.NewRequest("GET", "/worker/install.sh?code=c0de", nil)
		if c.tls {
			request.TLS = &tls.ConnectionState{}
		}
		response := httptest.NewRecorder()
		(&Server{config: c.config}).serveScript(response, request, shScript(installTemplate), func(string) bool { return true }, "")
		if got := response.Body.String(); !strings.Contains(got, "\nca=''\nca_fingerprint=''\n") {
			t.Errorf("%s: the script carries a CA:\n%s", c.name, got)
		}
	}
}
