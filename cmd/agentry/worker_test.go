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

	"gitlab.com/dev.chempa/agentry/internal/tlsca"
	"gitlab.com/dev.chempa/agentry/internal/workerlink"
)

// joinable is a server with -https's front door that takes any code, and
// counts the joins it was sent.
func joinable(t *testing.T) (ca *tlsca.CA, address string, joins *atomic.Int32) {
	t.Helper()
	ca, _ = testCA(t)
	joins = &atomic.Int32{}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/worker/join", func(w http.ResponseWriter, r *http.Request) {
		joins.Add(1)
		json.NewEncoder(w).Encode(map[string]string{"name": "laptop", "credential": "c1"})
	})
	mux.HandleFunc("HEAD "+workerlink.UpdatePath, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer c1" {
			w.WriteHeader(http.StatusUnauthorized)
		}
	})
	return ca, serving(t, ca, frontDoor(mux, ca)), joins
}

func join(dir string, args ...string) (int, string) {
	var out bytes.Buffer
	code := runWorker(append([]string{"join", "-dir", dir}, args...), nil, &out, &out)
	return code, out.String()
}

func savedWorker(t *testing.T, dir string) workerConfig {
	t.Helper()
	config, err := loadWorker(dir)
	if err != nil {
		t.Fatal(err)
	}
	return config
}

func TestJoiningOverHTTPSWithTheRightFingerprintKeepsTheCA(t *testing.T) {
	ca, address, joins := joinable(t)
	for _, given := range []string{"https://" + address, "http://" + address} {
		dir := t.TempDir()
		// Written as people copy it: lower case, without colons.
		fingerprint := strings.ToLower(strings.ReplaceAll(ca.Fingerprint(), ":", ""))
		if code, out := join(dir, "-ca-fingerprint", fingerprint, given, "code"); code != 0 {
			t.Fatalf("%s: exit %d: %s", given, code, out)
		}
		config := savedWorker(t, dir)
		if config.Server != "https://"+address || config.CA != string(ca.PEM()) || config.Credential != "c1" {
			t.Errorf("%s: saved %+v", given, config)
		}
	}
	if joins.Load() != 2 {
		t.Errorf("%d joins, want 2", joins.Load())
	}
}

func TestAWrongFingerprintIsRefusedBeforeTheCodeIsSent(t *testing.T) {
	_, address, joins := joinable(t)
	other, _ := testCA(t)
	dir := t.TempDir()
	code, out := join(dir, "-ca-fingerprint", other.Fingerprint(), "https://"+address, "code")
	if code == 0 || !strings.Contains(out, "not joining") {
		t.Errorf("exit %d: %s", code, out)
	}
	if joins.Load() != 0 {
		t.Error("the code was sent to a server whose CA didn't match")
	}
	if _, err := os.Stat(filepath.Join(dir, "worker.json")); err == nil {
		t.Error("it saved a worker.json")
	}
}

func TestHTTPSWithoutAFingerprintIsRefusedWithIt(t *testing.T) {
	ca, address, joins := joinable(t)
	code, out := join(t.TempDir(), "https://"+address, "code")
	if code == 0 || !strings.Contains(out, ca.Fingerprint()) || !strings.Contains(out, "-ca-fingerprint") || !strings.Contains(out, "agentry ca") {
		t.Errorf("exit %d: %s", code, out)
	}
	if joins.Load() != 0 {
		t.Error("the code was sent to a server it doesn't trust")
	}
}

// A worker that joined over plain HTTP, run again with the fingerprint (as
// the install command will), moves to https without spending the code.
func TestJoiningAgainWithTheFingerprintMovesAWorkerToHTTPS(t *testing.T) {
	ca, address, joins := joinable(t)
	dir := t.TempDir()
	if err := saveWorker(dir, workerConfig{Server: "http://" + address, Name: "laptop", Credential: "c1"}); err != nil {
		t.Fatal(err)
	}
	code, out := join(dir, "-ca-fingerprint", ca.Fingerprint(), "http://"+address, "code")
	if code != 0 || !strings.Contains(out, "joined https://"+address+" already") {
		t.Fatalf("exit %d: %s", code, out)
	}
	if config := savedWorker(t, dir); config.Server != "https://"+address || config.CA != string(ca.PEM()) {
		t.Errorf("saved %+v", config)
	}
	// Run again with no fingerprint, it keeps the CA it has.
	if code, out := join(dir, "https://"+address, "code"); code != 0 || !strings.Contains(out, "already") {
		t.Errorf("again: exit %d: %s", code, out)
	}
	if joins.Load() != 0 {
		t.Errorf("%d joins: the code was spent", joins.Load())
	}
}

// Without -https, nothing changes: no fingerprint, no CA. A fingerprint
// given for such a server is refused, as it has no CA to check.
func TestAServerOverPlainHTTP(t *testing.T) {
	var joins atomic.Int32
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/worker/join" {
			joins.Add(1)
			json.NewEncoder(w).Encode(map[string]string{"name": "laptop", "credential": "c1"})
			return
		}
		http.NotFound(w, r)
	}))
	defer plain.Close()
	if code, out := join(t.TempDir(), "-ca-fingerprint", "AB:CD", plain.URL, "code"); code == 0 || !strings.Contains(out, "no Agentry CA") {
		t.Errorf("with a fingerprint: exit %d: %s", code, out)
	}
	if joins.Load() != 0 {
		t.Error("the code was sent with a fingerprint that checked nothing")
	}
	dir := t.TempDir()
	if code, out := join(dir, plain.URL, "code"); code != 0 {
		t.Fatalf("exit %d: %s", code, out)
	}
	if config := savedWorker(t, dir); config.Server != plain.URL || config.CA != "" {
		t.Errorf("saved %+v", config)
	}
}
