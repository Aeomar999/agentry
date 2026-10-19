package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
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
