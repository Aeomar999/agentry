package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/client"
	"gitlab.com/dev.chempa/agentry/internal/tlsca"
)

// agentry ca shows the CA's fingerprint; -regenerate makes a new CA and
// says what that means for devices.
func TestCARegenerate(t *testing.T) {
	dataDir := t.TempDir()
	old, err := tlsca.Open(tlsca.Dir(dataDir))
	if err != nil {
		t.Fatal(err)
	}
	var out, errs bytes.Buffer
	if code := runCA([]string{"-data", dataDir}, nil, &out, &errs); code != 0 || !strings.Contains(out.String(), old.Fingerprint()) {
		t.Fatalf("agentry ca: %d %q %q", code, out.String(), errs.String())
	}
	out.Reset()
	if code := runCA([]string{"-data", dataDir, "-regenerate"}, nil, &out, &errs); code != 0 {
		t.Fatalf("agentry ca -regenerate: %d %q", code, errs.String())
	}
	renewed, err := tlsca.Open(tlsca.Dir(dataDir))
	if err != nil {
		t.Fatal(err)
	}
	if renewed.Fingerprint() == old.Fingerprint() {
		t.Error("the CA is the same one")
	}
	for _, want := range []string{"no longer trust", "/trust/ca.crt", renewed.Fingerprint()} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("said %q, want %q in it", out.String(), want)
		}
	}
}

// While the server for the data folder answers, -regenerate refuses and
// leaves the CA alone.
func TestCARegenerateRefusesWhileTheServerRuns(t *testing.T) {
	dataDir := t.TempDir()
	ca, err := tlsca.Open(tlsca.Dir(dataDir))
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("{}")) }))
	defer server.Close()
	token, err := keptToken(filepath.Join(dataDir, "ui-token"))
	if err != nil || token == "" {
		t.Fatal(err)
	}
	if err := client.Save(dataDir, client.Where{URL: server.URL, Page: server.URL}); err != nil {
		t.Fatal(err)
	}
	var out, errs bytes.Buffer
	if code := runCA([]string{"-data", dataDir, "-regenerate"}, nil, &out, &errs); code != 1 || !strings.Contains(errs.String(), "Stop it first") {
		t.Errorf("agentry ca -regenerate: %d %q", code, errs.String())
	}
	same, err := tlsca.Open(tlsca.Dir(dataDir))
	if err != nil || same.Fingerprint() != ca.Fingerprint() {
		t.Errorf("the CA changed while the server ran: %v", err)
	}
}
