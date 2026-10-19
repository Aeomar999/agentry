package tlsca

import (
	"crypto/tls"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"
)

// renewBefore is how long before it expires a server certificate is
// replaced.
const renewBefore = 30 * 24 * time.Hour

// hostsFresh is how long a list of hosts is used before they are asked for
// again: Hosts lists the network interfaces, too slow for every handshake.
const hostsFresh = 30 * time.Second

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

	mu           sync.Mutex
	listed       []string  // the normal hosts the CA permits, as last asked
	listedAt     time.Time // when they were asked
	listingSince time.Time // when the listing still running started; zero when none runs
	asks         uint64    // listings started, in order
	listedBy     uint64    // the listing listed came from
	leftOut      []string  // the hosts last reported as outside the CA's constraints
	cert         *tls.Certificate
	issued       []string // the normal hosts cert names
}

// NewIssuer issues for the hosts hosts returns, asked at most every
// hostsFresh, at the time now gives.
func NewIssuer(ca *CA, hosts func() []string, now func() time.Time) *Issuer {
	return &Issuer{ca: ca, hosts: hosts, now: now}
}

// GetCertificate is for tls.Config.GetCertificate.
func (i *Issuer) GetCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	return i.Certificate()
}

// Certificate is the current server certificate, issued anew if it doesn't
// name the hosts there are now or expires within renewBefore. Hosts the CA
// may not vouch for are left out, and reported once each time they change.
func (i *Issuer) Certificate() (*tls.Certificate, error) {
	now := i.now()
	i.listHosts(now)
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.cert != nil && slices.Equal(i.issued, i.listed) && i.inTime(now) {
		return i.cert, nil
	}
	// Issued under the lock for the list kept, so a certificate for an
	// older list never replaces one for a newer.
	cert, err := i.ca.Issue(i.listed, now)
	if err != nil {
		return nil, err
	}
	i.cert, i.issued = cert, i.listed
	return cert, nil
}

// inTime is whether the certificate kept may still be served at now: it has
// begun, as it hasn't after the clock went back, and isn't within
// renewBefore of its end.
func (i *Issuer) inTime(now time.Time) bool {
	return !now.Before(i.cert.Leaf.NotBefore) && now.Before(i.cert.Leaf.NotAfter.Add(-renewBefore))
}

// listHosts asks for the hosts again once the list is hostsFresh old, or
// from a time after now, as after the clock went back. While a listing runs,
// callers keep the old list rather than each listing again. When two
// listings race, the one started last is kept, however slow it was.
func (i *Issuer) listHosts(now time.Time) {
	i.mu.Lock()
	fresh := !i.listedAt.IsZero() && (recent(i.listedAt, now) || recent(i.listingSince, now))
	if fresh {
		i.mu.Unlock()
		return
	}
	i.asks++
	ask := i.asks
	i.listingSince = now
	i.mu.Unlock()
	hosts, leftOut := i.split(normalHosts(i.hosts())) // outside the lock: hosts may be slow
	i.mu.Lock()
	defer i.mu.Unlock()
	if ask == i.asks {
		i.listingSince = time.Time{} // no listing started after this one runs
	}
	if ask < i.listedBy {
		return // a listing started after this one is already kept
	}
	i.listed, i.listedAt, i.listedBy = hosts, now, ask
	if !slices.Equal(i.leftOut, leftOut) {
		i.leftOut = leftOut
		if len(leftOut) > 0 {
			slog.Warn("the HTTPS certificate leaves out hosts the CA may not vouch for; `agentry ca regenerate` makes a CA for this machine's current hostname, and a public address needs a reverse proxy that does TLS",
				"hosts", strings.Join(leftOut, ", "))
		}
	}
}

// recent is whether since is a time no later than now and less than
// hostsFresh before it.
func recent(since, now time.Time) bool {
	return !since.IsZero() && !now.Before(since) && now.Sub(since) < hostsFresh
}

// split parts normal hosts into those the CA permits and those it doesn't.
func (i *Issuer) split(hosts []string) (permitted, leftOut []string) {
	for _, host := range hosts {
		if i.ca.Permits(host) {
			permitted = append(permitted, host)
		} else {
			leftOut = append(leftOut, host)
		}
	}
	return permitted, leftOut
}
