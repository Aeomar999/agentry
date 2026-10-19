package main

import (
	"crypto/tls"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/pinned"
	"gitlab.com/dev.chempa/agentry/internal/tlsca"
)

// A machine joining a server served over https with Agentry's own CA, as a
// worker or as Claude Code's connection, must trust that CA before it sends
// the code. It can't check the CA by the connection, which the CA itself
// vouches for, so it checks it by the fingerprint `agentry ca` shows on the
// server, given as -ca-fingerprint.

// caPath is where a server served with -https hands out its CA, to anyone.
const caPath = trustPrefix + "ca.pem"

// trustTimeout is how long joining waits for the server to answer its
// trust check.
const trustTimeout = 15 * time.Second

// trustedServer is a server a code may be sent to: its address, https when
// it is served so, and the CA it is pinned to, nil when it needs none.
type trustedServer struct {
	server string
	ca     []byte
}

// trustServer works out how to reach server, checking its CA by
// fingerprint. With a fingerprint, the server's CA is fetched (over plain
// HTTP from an http address, as /trust/ is served so) and must match it;
// an http address then becomes https, as the server would redirect it.
// Without one, an https address must be trusted by this machine, as one
// behind a reverse proxy with a public certificate is; if it isn't, it is
// refused with the fingerprint of the CA the server offered. Each request
// gives up after timeout.
func trustServer(server, fingerprint string, timeout time.Duration) (trustedServer, error) {
	server = strings.TrimSuffix(server, "/")
	if fingerprint == "" {
		if !strings.HasPrefix(server, "https://") {
			return trustedServer{server: server}, nil
		}
		return trustedServer{server: server}, trustedWithoutCA(server, timeout)
	}
	ca, err := fetchCA(server, timeout)
	if err != nil {
		return trustedServer{}, err
	}
	if got := tlsca.FingerprintOf(derOf(ca)); normalFingerprint(got) != normalFingerprint(fingerprint) {
		return trustedServer{}, fmt.Errorf("the server's CA has fingerprint %s, not the %s given, so it isn't trusted. "+
			"Check it with `agentry ca` on the server", got, fingerprint)
	}
	secure, err := httpsOf(server)
	if err != nil {
		return trustedServer{}, err
	}
	trusted := trustedServer{server: secure, ca: ca}
	return trusted, servedWithCA(trusted, timeout)
}

// httpsOf is server at https, where one served with Agentry's CA is
// reached whether it was given as http or https, in any case.
func httpsOf(server string) (string, error) {
	u, err := url.Parse(server)
	if err != nil {
		return "", err
	}
	if scheme := strings.ToLower(u.Scheme); scheme != "http" && scheme != "https" {
		return "", fmt.Errorf("%s isn't an http or https address", server)
	}
	u.Scheme = "https"
	return u.String(), nil
}

// servedWithCA checks that the server answers with a certificate its CA
// issued. One behind a reverse proxy with a public certificate may hand
// out the right CA and still not be served with it.
func servedWithCA(trusted trustedServer, timeout time.Duration) error {
	response, err := pinned.Client(trusted.ca, timeout).Get(trusted.server + caPath)
	if pinned.Untrusted(err) {
		return fmt.Errorf("%s hands out Agentry's CA but answers with another certificate, as a reverse proxy with its own does: "+
			"run this again without -ca-fingerprint", trusted.server)
	}
	if err != nil {
		return err
	}
	response.Body.Close()
	return nil
}

// trustedWithoutCA checks that this machine trusts server's certificate as
// it is, sending nothing but the request for the CA.
func trustedWithoutCA(server string, timeout time.Duration) error {
	response, err := (&http.Client{Timeout: timeout}).Get(server + caPath)
	if err == nil {
		response.Body.Close()
		return nil
	}
	if !pinned.Untrusted(err) {
		return err
	}
	if ca, fetchErr := fetchCA(server, timeout); fetchErr == nil {
		offered := tlsca.FingerprintOf(derOf(ca))
		return fmt.Errorf("%s is served with its own CA, SHA-256 %s. Check that against `agentry ca` on the server, "+
			"then run this again with -ca-fingerprint %s", server, offered, offered)
	}
	return fmt.Errorf("%s: %w (to pin Agentry's own CA, run this again with -ca-fingerprint from `agentry ca` on the server)", server, err)
}

// fetchCA gets the CA server offers, without trusting the connection: the
// fingerprint check is what makes it trusted.
func fetchCA(server string, timeout time.Duration) ([]byte, error) {
	unchecked := &http.Client{Timeout: timeout,
		Transport:     &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}, // checked by fingerprint
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := unchecked.Get(server + caPath)
	if err != nil {
		return nil, fmt.Errorf("getting the server's CA: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s has no Agentry CA to check (%s for %s): is it served with -https?", server, response.Status, caPath)
	}
	ca, err := io.ReadAll(io.LimitReader(response.Body, 64<<10))
	if err != nil {
		return nil, fmt.Errorf("getting the server's CA: %w", err)
	}
	if err := pinned.Check(ca); err != nil {
		return nil, fmt.Errorf("the server's %s: %w", caPath, err)
	}
	return ca, nil
}

// derOf is the certificate in a PEM that pinned.Check took.
func derOf(caPEM []byte) []byte {
	block, _ := pem.Decode(caPEM)
	return block.Bytes
}

// normalFingerprint compares fingerprints however they are written: any
// case, with or without colons or spaces.
func normalFingerprint(f string) string {
	return strings.ToUpper(strings.NewReplacer(":", "", " ", "", "-", "").Replace(strings.TrimSpace(f)))
}
