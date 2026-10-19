package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/pinned"
	"gitlab.com/dev.chempa/agentry/internal/tlsca"
)

// A worker joining a server served over https with Agentry's own CA must
// trust that CA before it sends the code. It can't check the CA by the
// connection, which the CA itself vouches for, so it checks it by the
// fingerprint `agentry ca` shows on the server, given as -ca-fingerprint.

// caPath is where a server served with -https hands out its CA, to anyone.
const caPath = trustPrefix + "ca.pem"

// joining is the server a worker joins: its address, https when it is
// served so, and the CA it is pinned to, nil when it needs none.
type joining struct {
	server string
	ca     []byte
}

// toJoin works out how to reach server, checking its CA by fingerprint.
// With a fingerprint, the server's CA is fetched (over plain HTTP from an
// http address, as /trust/ is served so) and must match it; an http
// address then becomes https, as the server would redirect it. Without
// one, an https address must be trusted by this machine, as one behind a
// reverse proxy with a public certificate is; if it isn't, the join is
// refused with the fingerprint of the CA the server offered.
func toJoin(server, fingerprint string) (joining, error) {
	server = strings.TrimSuffix(server, "/")
	if fingerprint == "" {
		if !strings.HasPrefix(server, "https://") {
			return joining{server: server}, nil
		}
		return joining{server: server}, trustedWithoutCA(server)
	}
	ca, err := fetchCA(server)
	if err != nil {
		return joining{}, err
	}
	if got := tlsca.FingerprintOf(derOf(ca)); normalFingerprint(got) != normalFingerprint(fingerprint) {
		return joining{}, fmt.Errorf("the server's CA has fingerprint %s, not the %s given: not joining. "+
			"Check it with `agentry ca` on the server", got, fingerprint)
	}
	return joining{server: "https://" + strings.TrimPrefix(strings.TrimPrefix(server, "https://"), "http://"), ca: ca}, nil
}

// trustedWithoutCA checks that this machine trusts server's certificate as
// it is, sending nothing but the request for the CA.
func trustedWithoutCA(server string) error {
	response, err := (&http.Client{Timeout: 15 * time.Second}).Get(server + caPath)
	if err == nil {
		response.Body.Close()
		return nil
	}
	var unknown x509.UnknownAuthorityError
	if !errors.As(err, &unknown) {
		return err
	}
	if ca, fetchErr := fetchCA(server); fetchErr == nil {
		offered := tlsca.FingerprintOf(derOf(ca))
		return fmt.Errorf("%s is served with its own CA, SHA-256 %s. Check that against `agentry ca` on the server, "+
			"then join with -ca-fingerprint %s", server, offered, offered)
	}
	return fmt.Errorf("%s: %w (to pin Agentry's own CA, join with -ca-fingerprint from `agentry ca` on the server)", server, err)
}

// fetchCA gets the CA server offers, without trusting the connection: the
// fingerprint check is what makes it trusted.
func fetchCA(server string) ([]byte, error) {
	unchecked := &http.Client{Timeout: 15 * time.Second,
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
