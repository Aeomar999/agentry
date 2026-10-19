package pinned

import (
	"crypto/tls"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/tlsca"
)

// agentryServer serves over TLS with the leaf issuer hands out, as `agentry
// serve -https` does. (httptest's StartTLS would serve its own leaf.)
func agentryServer(t *testing.T, issuer *tlsca.Issuer) *httptest.Server {
	t.Helper()
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "agentry")
	}))
	server.Listener = tls.NewListener(server.Listener, &tls.Config{GetCertificate: issuer.GetCertificate})
	server.Start()
	server.URL = "https://" + server.Listener.Addr().String()
	t.Cleanup(server.Close)
	return server
}

func newCA(t *testing.T) *tlsca.CA {
	t.Helper()
	ca, err := tlsca.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return ca
}

func get(client *http.Client, url string) error {
	response, err := client.Get(url)
	if err != nil {
		return err
	}
	response.Body.Close()
	return nil
}

func TestAPinnedClientReachesTheServerItsCAVouchesFor(t *testing.T) {
	ca := newCA(t)
	server := agentryServer(t, tlsca.NewIssuer(ca, func() []string { return nil }, time.Now))
	if err := get(Client(ca.PEM(), 5*time.Second), server.URL); err != nil {
		t.Fatal(err)
	}
}

// The system store is told to trust the other server's CA (SSL_CERT_FILE,
// which Go reads on Linux), and the pinned client still refuses it.
func TestAPinnedClientRefusesAnotherCAEvenOneTheSystemTrusts(t *testing.T) {
	ours, theirs := newCA(t), newCA(t)
	server := agentryServer(t, tlsca.NewIssuer(theirs, func() []string { return nil }, time.Now))
	systemStore := filepath.Join(t.TempDir(), "system.pem")
	if err := os.WriteFile(systemStore, theirs.PEM(), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SSL_CERT_FILE", systemStore)
	t.Setenv("SSL_CERT_DIR", t.TempDir())
	if err := get(&http.Client{Transport: &http.Transport{}}, server.URL); err != nil {
		t.Skipf("this system's store doesn't take SSL_CERT_FILE: %v", err)
	}
	if err := get(Client(ours.PEM(), 5*time.Second), server.URL); err == nil {
		t.Fatal("the pinned client trusted a server another CA vouches for")
	}
}

func TestAPinnedClientRefusesEverythingWhenItsCAIsUnreadable(t *testing.T) {
	ca := newCA(t)
	server := agentryServer(t, tlsca.NewIssuer(ca, func() []string { return nil }, time.Now))
	if err := get(Client([]byte("not a certificate"), 5*time.Second), server.URL); err == nil {
		t.Fatal("an unreadable CA fell back to trusting the server")
	}
}

// A server another CA vouches for is untrusted, however the error is
// wrapped. One that isn't there is not, and nor is a certificate from our
// CA for another name or out of date: the CA wasn't regenerated.
func TestUntrusted(t *testing.T) {
	ours, theirs := newCA(t), newCA(t)
	server := agentryServer(t, tlsca.NewIssuer(theirs, func() []string { return nil }, time.Now))
	refused := get(Client(ours.PEM(), 5*time.Second), server.URL)
	if !Untrusted(refused) || !Untrusted(fmt.Errorf("link: %w", refused)) {
		t.Errorf("not untrusted: %v", refused)
	}
	gone := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	gone.Close()
	if unreachable := get(Client(ours.PEM(), 5*time.Second), "https://"+gone.Listener.Addr().String()); Untrusted(unreachable) {
		t.Errorf("untrusted: %v", unreachable)
	}

	ourServer := agentryServer(t, tlsca.NewIssuer(ours, func() []string { return nil }, time.Now))
	config := TLSConfig(ours.PEM())
	config.ServerName = "elsewhere.test"
	otherName := get(&http.Client{Transport: &http.Transport{TLSClientConfig: config}, Timeout: 5 * time.Second}, ourServer.URL)
	if otherName == nil || Untrusted(otherName) {
		t.Errorf("another name: %v", otherName)
	}

	old, err := ours.Issue(nil, time.Now().Add(-2*397*24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	expiredServer := httptest.NewUnstartedServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	expiredServer.TLS = &tls.Config{Certificates: []tls.Certificate{*old}}
	expiredServer.StartTLS()
	t.Cleanup(expiredServer.Close)
	expired := get(Client(ours.PEM(), 5*time.Second), expiredServer.URL)
	if expired == nil || Untrusted(expired) {
		t.Errorf("expired: %v", expired)
	}
}

func TestWithNoCAItIsThePlainDefault(t *testing.T) {
	if TLSConfig(nil) != nil || Transport(nil) != http.DefaultTransport {
		t.Error("no CA should be the defaults")
	}
	plain := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer plain.Close()
	if err := get(Client(nil, time.Second), plain.URL); err != nil {
		t.Error(err)
	}
}

// The leaf changes when the machine's addresses do; the CA doesn't, so a
// client that pins it carries on.
func TestTheClientKeepsWorkingWhenTheLeafIsIssuedAgain(t *testing.T) {
	ca := newCA(t)
	var mu sync.Mutex
	hosts, now := []string{"192.168.1.20"}, time.Now()
	issuer := tlsca.NewIssuer(ca,
		func() []string { mu.Lock(); defer mu.Unlock(); return hosts },
		func() time.Time { mu.Lock(); defer mu.Unlock(); return now })
	server := agentryServer(t, issuer)
	first, err := issuer.Certificate()
	if err != nil {
		t.Fatal(err)
	}
	client := Client(ca.PEM(), 5*time.Second)
	if err := get(client, server.URL); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	hosts, now = []string{"10.0.0.9"}, now.Add(time.Minute)
	mu.Unlock()
	second, err := issuer.Certificate()
	if err != nil {
		t.Fatal(err)
	}
	if second.Leaf.SerialNumber.Cmp(first.Leaf.SerialNumber) == 0 {
		t.Fatal("the leaf wasn't issued again")
	}
	client.CloseIdleConnections() // a new handshake, which sees the new leaf
	if err := get(client, server.URL); err != nil {
		t.Fatalf("after the new leaf: %v", err)
	}
}

func TestCheck(t *testing.T) {
	ca := newCA(t)
	if err := Check(ca.PEM()); err != nil {
		t.Errorf("the CA: %v", err)
	}
	for name, bad := range map[string][]byte{
		"empty":    nil,
		"not PEM":  []byte("hello"),
		"two":      append(ca.PEM(), newCA(t).PEM()...),
		"a key":    []byte("-----BEGIN PRIVATE KEY-----\nAAAA\n-----END PRIVATE KEY-----\n"),
		"not a CA": leafPEM(t, ca),
	} {
		if Check(bad) == nil {
			t.Errorf("%s: taken as a CA", name)
		}
	}
}

func leafPEM(t *testing.T, ca *tlsca.CA) []byte {
	t.Helper()
	cert, err := ca.Issue(nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Certificate[0]})
}
