package tlsca

import (
	"crypto/x509"
	"slices"
	"testing"
	"time"
)

func TestTheCertificateNamesEveryHostAndVerifies(t *testing.T) {
	ca, err := open(t.TempDir(), "Workbench")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	cert, err := ca.Issue([]string{"Workbench", "workbench.local", "192.168.1.20", "100.64.0.7", "fd7a:115c::1"}, now)
	if err != nil {
		t.Fatal(err)
	}
	leaf := cert.Leaf
	if want := []string{"localhost", "workbench", "workbench.local"}; !slices.Equal(sorted(leaf.DNSNames), want) {
		t.Errorf("DNS names %v, want %v", leaf.DNSNames, want)
	}
	var ips []string
	for _, ip := range leaf.IPAddresses {
		ips = append(ips, ip.String())
	}
	if want := []string{"100.64.0.7", "127.0.0.1", "192.168.1.20", "::1", "fd7a:115c::1"}; !slices.Equal(sorted(ips), want) {
		t.Errorf("IPs %v, want %v", ips, want)
	}
	if !slices.Equal(leaf.ExtKeyUsage, []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}) || leaf.IsCA {
		t.Errorf("ExtKeyUsage %v, IsCA %v", leaf.ExtKeyUsage, leaf.IsCA)
	}
	if days := leaf.NotAfter.Sub(now).Hours() / 24; days > 397 || days < 396 {
		t.Errorf("lasts %.1f days", days)
	}
	for _, name := range []string{"localhost", "workbench", "workbench.local", "WORKBENCH", "127.0.0.1", "::1", "192.168.1.20", "100.64.0.7", "fd7a:115c::1"} {
		if _, err := leaf.Verify(x509.VerifyOptions{DNSName: name, Roots: ca.Pool(), KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := leaf.Verify(x509.VerifyOptions{DNSName: "example.com", Roots: ca.Pool()}); err == nil {
		t.Error("it verifies for a name it doesn't have")
	}
	if len(cert.Certificate) != 2 || !slices.Equal(cert.Certificate[1], ca.DER()) {
		t.Error("the chain should be the leaf, then the CA")
	}
}

func TestALeafNeverOutlivesTheCA(t *testing.T) {
	ca, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cert, err := ca.Issue(nil, ca.cert.NotAfter.Add(-24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if !cert.Leaf.NotAfter.Equal(ca.cert.NotAfter) {
		t.Errorf("leaf ends %v, CA %v", cert.Leaf.NotAfter, ca.cert.NotAfter)
	}
}

func TestNormalHosts(t *testing.T) {
	got := normalHosts([]string{" Box.Lan. ", "", "127.0.0.1", "0:0:0:0:0:0:0:1", "10.0.0.2", "box.lan"})
	want := []string{"10.0.0.2", "127.0.0.1", "::1", "box.lan", "localhost"}
	if !slices.Equal(got, want) {
		t.Errorf("normalHosts = %v, want %v", got, want)
	}
}

func sorted(s []string) []string {
	return slices.Sorted(slices.Values(s))
}
