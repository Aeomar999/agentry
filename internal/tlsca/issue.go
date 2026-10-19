package tlsca

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"net"
	"os"
	"slices"
	"strings"
	"time"
)

// leafLifetime stays well inside the 825 days Apple allows a server
// certificate from a CA the user installed; 397 is what public CAs may issue.
const leafLifetime = 397 * 24 * time.Hour

// always are the names every server certificate has, for pages and
// commands on the server's own machine.
var always = []string{"localhost", "127.0.0.1", "::1"}

// Issue makes a server certificate for hosts, DNS names and IP addresses,
// signed by the CA and valid from now. It always names localhost,
// 127.0.0.1 and ::1 too, and leaves out hosts the CA's name constraints
// don't permit, which clients would refuse the certificate for.
func (ca *CA) Issue(hosts []string, now time.Time) (*tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	hostname, _ := os.Hostname()
	template := &x509.Certificate{
		SerialNumber: randomSerial(),
		Subject:      pkix.Name{CommonName: "Agentry (" + hostname + ")", Organization: []string{"Agentry"}},
		NotBefore:    now.Add(-time.Hour), // a device whose clock is a little behind
		NotAfter:     notAfter(now, ca.cert.NotAfter),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	for _, host := range normalHosts(hosts) {
		if !ca.Permits(host) {
			continue
		}
		if ip := net.ParseIP(host); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
		} else {
			template.DNSNames = append(template.DNSNames, host)
		}
	}
	der, err := x509.CreateCertificate(rand.Reader, template, ca.cert, &key.PublicKey, ca.key)
	if err != nil {
		return nil, err
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}
	return &tls.Certificate{Certificate: [][]byte{der, ca.cert.Raw}, PrivateKey: key, Leaf: leaf}, nil
}

// notAfter is a leaf's end: leafLifetime from now, but no later than the
// CA's own.
func notAfter(now, caEnd time.Time) time.Time {
	if end := now.Add(leafLifetime); end.Before(caEnd) {
		return end
	}
	return caEnd
}

// normalHosts is hosts with the ones every certificate names, each once, in
// order: lower-case names and IPs written the one way Go writes them.
func normalHosts(hosts []string) []string {
	var normal []string
	for _, host := range append(slices.Clone(always), hosts...) {
		host = strings.TrimSpace(host)
		if ip := net.ParseIP(host); ip != nil {
			host = ip.String()
		} else {
			host = strings.ToLower(strings.TrimSuffix(host, "."))
		}
		if host != "" {
			normal = append(normal, host)
		}
	}
	slices.Sort(normal)
	return slices.Compact(normal)
}
