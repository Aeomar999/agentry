package tlsca

import (
	"crypto/tls"
	"slices"
	"sync"
	"time"
)

// renewBefore is how long before it expires a server certificate is
// replaced.
const renewBefore = 30 * 24 * time.Hour

// Issuer hands TLS the server certificate, issuing a new one when the
// addresses change or the one it has nears its end, so the server never
// restarts for it.
//
// The certificate is kept in memory only. A new one each start costs a
// millisecond, clients trust the CA rather than the certificate, and one
// fewer private key sits on disk.
type Issuer struct {
	ca    *CA
	hosts func() []string
	now   func() time.Time

	mu     sync.Mutex
	cert   *tls.Certificate
	issued []string // the normal hosts cert names
}

// NewIssuer issues for the hosts hosts returns, asked on each handshake, at
// the time now gives.
func NewIssuer(ca *CA, hosts func() []string, now func() time.Time) *Issuer {
	return &Issuer{ca: ca, hosts: hosts, now: now}
}

// GetCertificate is for tls.Config.GetCertificate.
func (i *Issuer) GetCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	return i.Certificate()
}

// Certificate is the current server certificate, issued anew if it doesn't
// name the hosts there are now or expires within renewBefore.
func (i *Issuer) Certificate() (*tls.Certificate, error) {
	hosts, now := normalHosts(i.hosts()), i.now() // outside the lock: hosts may be slow
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.cert != nil && slices.Equal(i.issued, hosts) && now.Before(i.cert.Leaf.NotAfter.Add(-renewBefore)) {
		return i.cert, nil
	}
	cert, err := i.ca.Issue(hosts, now)
	if err != nil {
		return nil, err
	}
	i.cert, i.issued = cert, hosts
	return cert, nil
}
