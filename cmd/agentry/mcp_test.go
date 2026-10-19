package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/client"
	"gitlab.com/dev.chempa/agentry/internal/tlsca"
)

// connectable is a server with -https's front door whose Integrations page
// takes any code for the credential "m1", and counts the codes it was
// sent.
func connectable(t *testing.T) (ca *tlsca.CA, address string, codes *atomic.Int32) {
	t.Helper()
	ca, _ = testCA(t)
	codes = &atomic.Int32{}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/integrations/connect", func(w http.ResponseWriter, r *http.Request) {
		codes.Add(1)
		json.NewEncoder(w).Encode(map[string]string{"credential": "m1"})
	})
	mux.HandleFunc("GET /api/config", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer m1" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Write([]byte("{}"))
	})
	t.Setenv("XDG_CONFIG_HOME", t.TempDir()) // not this machine's real connection
	return ca, serving(t, ca, frontDoor(mux, ca)), codes
}

func mcpConnect(args ...string) (int, string) {
	var out bytes.Buffer
	code := runMCP(append([]string{"connect"}, args...), nil, &out, &out)
	return code, out.String()
}

func TestMCPConnectOverHTTPSKeepsTheCA(t *testing.T) {
	ca, address, codes := connectable(t)
	if code, out := mcpConnect("-ca-fingerprint", ca.Fingerprint(), "http://"+address, "code"); code != 0 {
		t.Fatalf("exit %d: %s", code, out)
	}
	connected, ok := client.Connected()
	if !ok || connected.URL != "https://"+address || connected.CA != string(ca.PEM()) {
		t.Fatalf("connected %+v, %v", connected.Where, ok)
	}
	if err := connected.Call("GET", "/api/config", nil, nil); err != nil {
		t.Error(err)
	}
	if codes.Load() != 1 {
		t.Errorf("%d codes sent, want 1", codes.Load())
	}
}

func TestMCPConnectRefusesAWrongFingerprintBeforeTheCodeIsSent(t *testing.T) {
	_, address, codes := connectable(t)
	other, _ := testCA(t)
	code, out := mcpConnect("-ca-fingerprint", other.Fingerprint(), "https://"+address, "code")
	if code == 0 || !strings.Contains(out, "isn't trusted") {
		t.Errorf("exit %d: %s", code, out)
	}
	if codes.Load() != 0 {
		t.Error("the code was sent to a server whose CA didn't match")
	}
	if _, err := os.Stat(client.ConnectionFile()); err == nil {
		t.Error("it saved a connection")
	}
}

func TestMCPConnectOverHTTPSWithoutAFingerprintSaysWhichToGive(t *testing.T) {
	ca, address, codes := connectable(t)
	code, out := mcpConnect("https://"+address, "code")
	if code == 0 || !strings.Contains(out, "-ca-fingerprint "+ca.Fingerprint()) {
		t.Errorf("exit %d: %s", code, out)
	}
	if codes.Load() != 0 {
		t.Error("the code was sent to a server it doesn't trust")
	}
}

// --server and --token name a server the same way: by its CA's fingerprint
// when it is served with one.
func TestMCPWithServerAndTokenOverHTTPS(t *testing.T) {
	ca, address, _ := connectable(t)
	agentry, err := connect("https://"+address, "m1", ca.Fingerprint(), "")
	if err != nil {
		t.Fatal(err)
	}
	if err := agentry.Call("GET", "/api/config", nil, nil); err != nil {
		t.Error(err)
	}
	if _, err := connect("https://"+address, "m1", "", ""); err == nil || !strings.Contains(err.Error(), "-ca-fingerprint") {
		t.Errorf("without a fingerprint: %v", err)
	}
}

// hookServer is connectable's server as the hooks reach it, through
// AGENTRY_URL and AGENTRY_CA_FINGERPRINT, counting the requests for its CA.
func hookServer(t *testing.T) (ca *tlsca.CA, address string, caRequests *atomic.Int32) {
	t.Helper()
	ca, _ = testCA(t)
	caRequests = &atomic.Int32{}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/config", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("{}")) })
	door := frontDoor(mux, ca)
	address = serving(t, ca, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == caPath {
			caRequests.Add(1)
		}
		door.ServeHTTP(w, r)
	}))
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("AGENTRY_URL", "https://"+address)
	t.Setenv("AGENTRY_TOKEN", "m1")
	t.Setenv("AGENTRY_CA_FINGERPRINT", ca.Fingerprint())
	return ca, address, caRequests
}

// The hooks run with every prompt, so the CA they checked once is trusted
// next time with no request for it, however its fingerprint is written.
func TestTheHooksCheckTheCAOnce(t *testing.T) {
	ca, _, caRequests := hookServer(t)
	for i := range 2 {
		agentry, err := connectFromEnv()
		if err != nil {
			t.Fatalf("run %d: %v", i+1, err)
		}
		if err := agentry.Call("GET", "/api/config", nil, nil); err != nil {
			t.Fatalf("run %d: %v", i+1, err)
		}
	}
	t.Setenv("AGENTRY_CA_FINGERPRINT", strings.ToLower(strings.ReplaceAll(ca.Fingerprint(), ":", "")))
	if _, err := connectFromEnv(); err != nil {
		t.Fatal(err)
	}
	if got := caRequests.Load(); got != 2 { // the first check: the CA, then the pinned check
		t.Errorf("%d requests for the CA, want 2", got)
	}
	if info, err := os.Stat(keptCAFile()); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("the kept CA: %v, %v", info, err)
	}
}

func TestAnotherFingerprintChecksTheCAAgain(t *testing.T) {
	_, _, caRequests := hookServer(t)
	if _, err := connectFromEnv(); err != nil {
		t.Fatal(err)
	}
	other, _ := testCA(t)
	t.Setenv("AGENTRY_CA_FINGERPRINT", other.Fingerprint())
	if _, err := connectFromEnv(); err == nil || !strings.Contains(err.Error(), "isn't trusted") {
		t.Errorf("another fingerprint: %v", err)
	}
	if got := caRequests.Load(); got != 3 {
		t.Errorf("%d requests for the CA, want 3: the first check, and the CA again", got)
	}
}

// A kept CA's address is worked out from the server given, so a "server"
// planted in the file, as an older one had, is never sent the token.
func TestAKeptCAIsTrustedOnlyAtTheServerGiven(t *testing.T) {
	ca, address, caRequests := hookServer(t)
	planted := &atomic.Int32{}
	elsewhere := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { planted.Add(1) }))
	defer elsewhere.Close()
	kept, _ := json.Marshal(map[string]string{"given": "https://" + address, "server": elsewhere.URL, "ca": string(ca.PEM())})
	if err := os.MkdirAll(filepath.Dir(keptCAFile()), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keptCAFile(), kept, 0o600); err != nil {
		t.Fatal(err)
	}
	agentry, err := connectFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if agentry.URL != "https://"+address {
		t.Errorf("aimed at %s, want https://%s", agentry.URL, address)
	}
	if err := agentry.Call("GET", "/api/config", nil, nil); err != nil {
		t.Error(err)
	}
	if got := planted.Load(); got != 0 {
		t.Errorf("the planted server got %d requests", got)
	}
	if got := caRequests.Load(); got != 0 {
		t.Errorf("%d requests for the CA, want none: it was kept", got)
	}
}

// A kept CA the server no longer answers with, as after `agentry ca
// -regenerate`, is dropped, so the next run checks again.
func TestAKeptCAIsDroppedWhenTheServerIsNoLongerItsOwn(t *testing.T) {
	_, address, _ := hookServer(t)
	old, _ := testCA(t)
	if err := keepCA(keptCA{Given: "https://" + address, CA: string(old.PEM())}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGENTRY_CA_FINGERPRINT", old.Fingerprint())
	agentry, err := connectFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if err := agentry.Call("GET", "/api/config", nil, nil); err == nil {
		t.Fatal("trusted a server another CA vouches for")
	}
	if _, err := os.Stat(keptCAFile()); !os.IsNotExist(err) {
		t.Errorf("the kept CA is still there: %v", err)
	}
}
