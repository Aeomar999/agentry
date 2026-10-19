package tlsca

import (
	"crypto/tls"
	"crypto/x509"
	"net"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// clock and addresses the tests move by hand.
type world struct {
	mu    sync.Mutex
	now   time.Time
	hosts []string
	asked int // times Hosts was called
}

func (w *world) Now() time.Time {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.now
}

func (w *world) Hosts() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.asked++
	return slices.Clone(w.hosts)
}

// current is the hosts list, read without counting as asked.
func (w *world) current() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return slices.Clone(w.hosts)
}

func (w *world) timesAsked() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.asked
}

func (w *world) set(now time.Time, hosts ...string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.now, w.hosts = now, hosts
}

func newIssuer(t *testing.T) (*Issuer, *world) {
	ca, err := open(t.TempDir(), "box")
	if err != nil {
		t.Fatal(err)
	}
	w := &world{now: time.Now(), hosts: []string{"box", "192.168.1.20"}}
	return NewIssuer(ca, w.Hosts, w.Now), w
}

func certificate(t *testing.T, i *Issuer) *tls.Certificate {
	t.Helper()
	cert, err := i.GetCertificate(&tls.ClientHelloInfo{})
	if err != nil {
		t.Fatal(err)
	}
	return cert
}

func TestTheSameCertificateWhileNothingChanges(t *testing.T) {
	i, w := newIssuer(t)
	first := certificate(t, i)
	w.set(w.Now().Add(300*24*time.Hour), "192.168.1.20", "BOX", "localhost") // same hosts, written otherwise
	if again := certificate(t, i); again != first {
		t.Errorf("issued again: serial %v, then %v", first.Leaf.SerialNumber, again.Leaf.SerialNumber)
	}
}

func TestANewCertificateWhenTheAddressesChange(t *testing.T) {
	i, w := newIssuer(t)
	first := certificate(t, i)
	w.set(w.Now().Add(hostsFresh), "box", "192.168.1.20", "100.64.0.7") // joined a private network
	second := certificate(t, i)
	if second == first || second.Leaf.SerialNumber.Cmp(first.Leaf.SerialNumber) == 0 {
		t.Fatal("no new certificate for a new address")
	}
	if _, err := second.Leaf.Verify(x509.VerifyOptions{DNSName: "100.64.0.7", Roots: i.ca.Pool()}); err != nil {
		t.Errorf("the new address: %v", err)
	}
	w.set(w.Now().Add(hostsFresh), "box") // left the LAN
	if third := certificate(t, i); third == second || slices.ContainsFunc(third.Leaf.IPAddresses, func(ip net.IP) bool { return ip.String() == "192.168.1.20" }) {
		t.Errorf("still names an address that is gone: %v", third.Leaf.IPAddresses)
	}
}

func TestTheHostsAreAskedForAtMostEvery30Seconds(t *testing.T) {
	i, w := newIssuer(t)
	certificate(t, i)
	start := w.Now()
	w.set(start.Add(hostsFresh-time.Second), "box", "192.168.1.20", "100.64.0.7")
	if cert := certificate(t, i); w.timesAsked() != 1 || cert.Leaf.VerifyHostname("100.64.0.7") == nil {
		t.Errorf("asked %d times within %v", w.timesAsked(), hostsFresh)
	}
	w.set(start.Add(hostsFresh), w.current()...)
	if cert := certificate(t, i); cert.Leaf.VerifyHostname("100.64.0.7") != nil {
		t.Errorf("a new address isn't named %v after it came: %v", hostsFresh, cert.Leaf.IPAddresses)
	}
}

func TestANewCertificateNearItsEnd(t *testing.T) {
	i, w := newIssuer(t)
	first := certificate(t, i)
	end := first.Leaf.NotAfter
	w.set(end.Add(-renewBefore-time.Minute), w.current()...)
	if certificate(t, i) != first {
		t.Fatal("renewed more than 30 days before the end")
	}
	w.set(end.Add(-renewBefore+time.Minute), w.current()...)
	second := certificate(t, i)
	if second == first || !second.Leaf.NotAfter.After(end) {
		t.Errorf("not renewed within 30 days of the end: ends %v", second.Leaf.NotAfter)
	}
}

func TestANewCertificateWhenTheClockWentBack(t *testing.T) {
	i, w := newIssuer(t)
	first := certificate(t, i)
	w.set(first.Leaf.NotBefore, w.current()...)
	if certificate(t, i) != first {
		t.Fatal("issued again at the certificate's start")
	}
	w.set(first.Leaf.NotBefore.Add(-time.Second), w.current()...)
	second := certificate(t, i)
	if second == first || second.Leaf.NotBefore.After(w.Now()) {
		t.Errorf("served a certificate that begins at %v, at %v", second.Leaf.NotBefore, w.Now())
	}
}

// Run with -race: handshakes ask for the certificate at once, while the
// addresses change under them.
func TestConcurrentHandshakes(t *testing.T) {
	i, w := newIssuer(t)
	var wg sync.WaitGroup
	for n := range 16 {
		wg.Go(func() {
			for k := range 20 {
				if n == 0 && k%5 == 0 {
					w.set(w.Now().Add(hostsFresh), "box", "10.0.0."+string(rune('1'+k/5)))
				}
				if cert, err := i.GetCertificate(&tls.ClientHelloInfo{}); err != nil || cert.Leaf == nil {
					t.Error(err)
				}
			}
		})
	}
	wg.Wait()
}

// A listing that started first but ends last holds the older addresses, and
// is ignored.
func TestTheNewestHostListWins(t *testing.T) {
	ca, err := open(t.TempDir(), "box")
	if err != nil {
		t.Fatal(err)
	}
	asked, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	hosts := func() []string {
		if calls.Add(1) == 1 {
			close(asked)
			<-release
			return []string{"box", "192.168.1.20"} // as it was before the address came
		}
		return []string{"box", "192.168.1.20", "100.64.0.7"}
	}
	now := time.Now()
	i := NewIssuer(ca, hosts, func() time.Time { return now })
	slow := make(chan *tls.Certificate)
	go func() {
		cert, err := i.Certificate()
		if err != nil {
			t.Error(err)
		}
		slow <- cert
	}()
	<-asked
	newer := certificate(t, i)
	close(release)
	older := <-slow
	for name, cert := range map[string]*tls.Certificate{"the faster call": newer, "the slower call": older, "a later call": certificate(t, i)} {
		if cert == nil || cert.Leaf.VerifyHostname("100.64.0.7") != nil {
			t.Errorf("%s got a certificate for the older list", name)
		}
	}
}

// Handshakes that come while the hosts are being listed again keep the old
// list instead of each listing them too.
func TestHandshakesDuringAListingKeepTheOldList(t *testing.T) {
	ca, err := open(t.TempDir(), "box")
	if err != nil {
		t.Fatal(err)
	}
	asked, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	hosts := func() []string {
		n := calls.Add(1)
		if n == 1 {
			return []string{"box", "192.168.1.20"}
		}
		if n == 2 {
			close(asked)
		}
		<-release
		return []string{"box", "192.168.1.20", "100.64.0.7"}
	}
	start := time.Now()
	var now atomic.Pointer[time.Time]
	now.Store(&start)
	i := NewIssuer(ca, hosts, func() time.Time { return *now.Load() })
	certificate(t, i)
	later := start.Add(hostsFresh)
	now.Store(&later)
	slow := make(chan *tls.Certificate)
	go func() {
		cert, err := i.Certificate()
		if err != nil {
			t.Error(err)
		}
		slow <- cert
	}()
	<-asked
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if cert := certificate(t, i); cert.Leaf.VerifyHostname("100.64.0.7") == nil {
				t.Error("a handshake during the listing got the new list before it landed")
			}
		})
	}
	wg.Wait()
	if n := calls.Load(); n != 2 {
		t.Errorf("hosts called %d times, want 2: once at the start, once after %v", n, hostsFresh)
	}
	close(release)
	for name, cert := range map[string]*tls.Certificate{"the listing call": <-slow, "a later call": certificate(t, i)} {
		if cert.Leaf.VerifyHostname("100.64.0.7") != nil {
			t.Errorf("%s got a certificate for the old list", name)
		}
	}
}

// A client that trusts only the CA completes a handshake with a server the
// Issuer serves, by name and by address.
func TestAClientTrustingTheCAConnects(t *testing.T) {
	i, _ := newIssuer(t)
	listener, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{GetCertificate: i.GetCertificate})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			conn.(*tls.Conn).Handshake()
			conn.Close()
		}
	}()
	_, port, _ := net.SplitHostPort(listener.Addr().String())
	for _, name := range []string{"localhost", "127.0.0.1"} {
		conn, err := tls.Dial("tcp", net.JoinHostPort("127.0.0.1", port), &tls.Config{RootCAs: i.ca.Pool(), ServerName: name})
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		conn.Close()
	}
	if conn, err := tls.Dial("tcp", listener.Addr().String(), &tls.Config{ServerName: "localhost"}); err == nil {
		conn.Close()
		t.Error("a client trusting only the system's CAs connected")
	}
}

func TestTheIssuerLeavesOutHostsOutsideTheCA(t *testing.T) {
	i, w := newIssuer(t)
	w.set(w.Now(), "box", "renamed", "192.168.1.20", "203.0.113.9")
	cert := certificate(t, i)
	if want := []string{"box", "localhost"}; !slices.Equal(sorted(cert.Leaf.DNSNames), want) {
		t.Errorf("DNS names %v", cert.Leaf.DNSNames)
	}
	for _, host := range []string{"box", "192.168.1.20", "localhost"} {
		if _, err := cert.Leaf.Verify(x509.VerifyOptions{DNSName: host, Roots: i.ca.Pool()}); err != nil {
			t.Errorf("%s: %v", host, err)
		}
	}
	for _, host := range []string{"renamed", "203.0.113.9"} {
		if err := cert.Leaf.VerifyHostname(host); err == nil {
			t.Errorf("the certificate names %s, which the CA may not vouch for", host)
		}
	}
	if !slices.Equal(i.leftOut, []string{"203.0.113.9", "renamed"}) {
		t.Errorf("left out %v", i.leftOut)
	}
}
