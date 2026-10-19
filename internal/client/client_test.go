package client

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/tlsca"
)

// overHTTPS is a server with a leaf its own CA issued, as `agentry serve
// -https` serves, and that CA's certificate.
func overHTTPS(t *testing.T) (url string, caPEM string) {
	t.Helper()
	ca, err := tlsca.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		w.Write([]byte(`{"ok":true}`))
	}))
	issuer := tlsca.NewIssuer(ca, func() []string { return nil }, time.Now)
	server.Listener = tls.NewListener(server.Listener, &tls.Config{GetCertificate: issuer.GetCertificate})
	server.Start()
	t.Cleanup(server.Close)
	return "https://" + server.Listener.Addr().String(), string(ca.PEM())
}

func TestLocalReachesAnHTTPSServerWithTheCAItSaved(t *testing.T) {
	url, ca := overHTTPS(t)
	dataDir := t.TempDir()
	if err := Save(dataDir, Where{URL: url, Page: url, HTTPS: true, CA: ca}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "ui-token"), []byte("secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	local, err := Local(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	var reply struct{ OK bool }
	if err := local.Call("GET", "/api/config", nil, &reply); err != nil || !reply.OK {
		t.Fatalf("got %+v, %v", reply, err)
	}
	local.CA = ""
	if err := local.Call("GET", "/api/config", nil, nil); err == nil {
		t.Error("without the CA the server was trusted")
	}
}

func TestAConnectionCarriesItsCA(t *testing.T) {
	url, ca := overHTTPS(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir()) // where macOS keeps it
	t.Setenv("AppData", t.TempDir())
	if err := SaveConnection(Connection{URL: url, Credential: "secret", CA: ca}); err != nil {
		t.Fatal(err)
	}
	connected, ok := Connected()
	if !ok || connected.CA != ca {
		t.Fatalf("connected %+v, %v", connected, ok)
	}
	if err := connected.Call("GET", "/api/config", nil, nil); err != nil {
		t.Error(err)
	}
}

// A connection kept before servers had a CA reads as one without.
func TestAConnectionWithoutACAIsPlain(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("AppData", t.TempDir())
	path := ConnectionFile()
	os.MkdirAll(filepath.Dir(path), 0o700)
	if err := os.WriteFile(path, []byte(`{"url":"http://box:7800","credential":"c"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	connected, ok := Connected()
	if !ok || connected.CA != "" || connected.URL != "http://box:7800" {
		t.Errorf("connected %+v, %v", connected, ok)
	}
}
