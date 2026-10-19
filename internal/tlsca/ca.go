// Package tlsca is Agentry's own certificate authority, for serving over
// HTTPS with a certificate devices trust: the user trusts the CA once per
// device, as with mkcert, and the server certificate it signs names every
// address the server is reached on.
//
// The CA is name constrained, and the constraints are critical: it may only
// vouch for localhost, this machine's hostname (and hostname.local), loopback
// and the private address ranges (LAN, Hawser and Tailscale). Its key lives
// in the data folder (0600, in a 0700 folder agents are kept away from), but
// an agent runs as the same user as the server, and no check of the commands
// it runs can be complete. With the constraints, a key that leaks anyway
// can't be used to pass as any site on the public web to the devices that
// trust the CA.
//
// The trade-off: the hostname is fixed when the CA is made. After the machine
// is renamed, the Issuer leaves the new name out of the server certificate
// (addresses still work) until the CA is regenerated, and every device then
// trusts the new CA once more. A public address, as on a VPS, is never named:
// put a reverse proxy that does TLS in front instead.
package tlsca

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Folder is where the CA lives in the data folder.
const Folder = "tls"

const (
	keyFile    = "ca.key"
	certFile   = "ca.crt"
	caLifetime = 10 * 365 * 24 * time.Hour
)

// CA is Agentry's certificate authority. Its private key stays in the
// package: only the certificate is handed out.
type CA struct {
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
}

// Dir is the CA's folder in a data folder.
func Dir(dataDir string) string {
	return filepath.Join(dataDir, Folder)
}

// privateRanges are the addresses the CA may vouch for: loopback, the
// private LAN ranges, the shared range Hawser and Tailscale give out, and
// IPv6 unique local addresses (Tailscale's among them). hostaddrs.Network
// leaves out link-local addresses, so they aren't here.
var privateRanges = []string{
	"127.0.0.0/8", "::1/128",
	"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16",
	"100.64.0.0/10",
	"fd00::/8",
}

// Open loads the CA from dir, or makes one there on first use, for this
// machine's hostname. It refuses a folder or key file that is a symbolic
// link, a key that others than its owner may read, and a CA without name
// constraints.
func Open(dir string) (*CA, error) {
	hostname, _ := os.Hostname()
	return open(dir, hostname)
}

func open(dir, hostname string) (*CA, error) {
	if err := checkFolder(dir); err != nil {
		return nil, err
	}
	keyPath, certPath := filepath.Join(dir, keyFile), filepath.Join(dir, certFile)
	_, keyErr := os.Lstat(keyPath)
	_, certErr := os.Lstat(certPath)
	switch {
	case errors.Is(keyErr, fs.ErrNotExist) && errors.Is(certErr, fs.ErrNotExist):
		return create(dir, hostname)
	case errors.Is(keyErr, fs.ErrNotExist) || errors.Is(certErr, fs.ErrNotExist):
		return nil, fmt.Errorf("the CA in %s is incomplete: regenerate it", dir)
	}
	return load(keyPath, certPath)
}

// checkFolder refuses a CA folder that is a link or not a folder, and closes
// one open to others. It is tightened rather than refused: the key in it is
// checked on its own, and a folder made by hand or under a loose umask
// shouldn't stop the server. Windows keeps no Unix permissions.
func checkFolder(dir string) error {
	info, err := os.Lstat(dir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil // create makes it
	case err != nil:
		return err
	case info.Mode()&fs.ModeSymlink != 0:
		return fmt.Errorf("%s is a symbolic link: the CA's folder must be a plain folder", dir)
	case !info.IsDir():
		return fmt.Errorf("%s is not a folder", dir)
	case runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0:
		return os.Chmod(dir, 0o700)
	}
	return nil
}

// Regenerate removes the CA from dir, so the next Open makes a new one.
// Devices that trusted the old CA no longer trust the server.
func Regenerate(dir string) error {
	for _, name := range []string{keyFile, certFile} {
		if err := os.Remove(filepath.Join(dir, name)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	return nil
}

// PEM is the CA certificate, PEM encoded, as devices install it.
func (ca *CA) PEM() []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.cert.Raw})
}

// DER is the CA certificate's DER bytes.
func (ca *CA) DER() []byte {
	return ca.cert.Raw
}

// Fingerprint is the CA certificate's SHA-256, as browsers show it:
// upper-case hex bytes joined by colons.
func (ca *CA) Fingerprint() string {
	sum := sha256.Sum256(ca.cert.Raw)
	parts := make([]string, len(sum))
	for i, b := range sum {
		parts[i] = fmt.Sprintf("%02X", b)
	}
	return strings.Join(parts, ":")
}

// Pool holds only the CA, for clients that trust it and nothing else.
func (ca *CA) Pool() *x509.CertPool {
	pool := x509.NewCertPool()
	pool.AddCert(ca.cert)
	return pool
}

// Permits is true when the CA's name constraints let it vouch for host, a
// DNS name or an IP address written as normalHosts writes it.
func (ca *CA) Permits(host string) bool {
	if ip := net.ParseIP(host); ip != nil {
		for _, r := range ca.cert.PermittedIPRanges {
			if r.Contains(ip) {
				return true
			}
		}
		return false
	}
	for _, domain := range ca.cert.PermittedDNSDomains {
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return true
		}
	}
	return false
}

// constrained is true for a CA whose critical name constraints permit some
// names and some addresses, and so nothing else.
func constrained(cert *x509.Certificate) bool {
	return cert.PermittedDNSDomainsCritical && len(cert.PermittedDNSDomains) > 0 && len(cert.PermittedIPRanges) > 0
}

// permittedDomains are the DNS names a CA made on hostname may vouch for,
// each with its subdomains.
func permittedDomains(hostname string) []string {
	domains := []string{"localhost"}
	if hostname = strings.ToLower(strings.TrimSuffix(hostname, ".")); hostname != "" && hostname != "localhost" {
		domains = append(domains, hostname)
		if !strings.Contains(hostname, ".") {
			domains = append(domains, hostname+".local")
		}
	}
	return domains
}

func permittedRanges() []*net.IPNet {
	ranges := make([]*net.IPNet, len(privateRanges))
	for i, cidr := range privateRanges {
		_, ranges[i], _ = net.ParseCIDR(cidr)
	}
	return ranges
}

func create(dir, hostname string) (*CA, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber:          randomSerial(),
		Subject:               pkix.Name{CommonName: fmt.Sprintf("Agentry local CA (%s)", hostname), Organization: []string{"Agentry"}},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(caLifetime),
		IsCA:                  true,
		BasicConstraintsValid: true,
		MaxPathLen:            0,
		MaxPathLenZero:        true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,

		PermittedDNSDomainsCritical: true,
		PermittedDNSDomains:         permittedDomains(hostname),
		PermittedIPRanges:           permittedRanges(),
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, err
	}
	// The key is written before the certificate, so a CA with a certificate
	// always has its key; one left without a certificate is reported as incomplete.
	if err := writeNew(filepath.Join(dir, keyFile), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		return nil, err
	}
	if err := writeNew(filepath.Join(dir, certFile), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644); err != nil {
		return nil, err
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}
	return &CA{cert: cert, key: key}, nil
}

// writeNew writes a file that must not exist yet, with its permissions set
// as it is made.
func writeNew(path string, data []byte, mode os.FileMode) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func load(keyPath, certPath string) (*CA, error) {
	keyBlock, err := readKey(keyPath)
	if err != nil {
		return nil, err
	}
	parsed, err := x509.ParsePKCS8PrivateKey(keyBlock)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", keyPath, err)
	}
	key, ok := parsed.(*ecdsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("%s: not an ECDSA key", keyPath)
	}
	certBlock, err := readPEM(certPath, "CERTIFICATE")
	if err != nil {
		return nil, err
	}
	cert, err := x509.ParseCertificate(certBlock)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", certPath, err)
	}
	if !cert.IsCA {
		return nil, fmt.Errorf("%s: not a CA certificate", certPath)
	}
	if !constrained(cert) {
		return nil, fmt.Errorf("%s has no name constraints, so a leaked key could pass as any site: regenerate the CA", certPath)
	}
	if !key.PublicKey.Equal(cert.PublicKey) {
		return nil, fmt.Errorf("%s doesn't belong to %s: regenerate the CA", keyPath, certPath)
	}
	return &CA{cert: cert, key: key}, nil
}

// readKey reads the CA key's PEM block from the file it checks, refusing a
// key a link could point anywhere, or that others may read. Windows doesn't
// keep Unix permissions, so there only the link counts.
func readKey(path string) ([]byte, error) {
	f, err := openKey(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a plain file", path)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("%s may be read by others (%v): only its owner may (chmod 600)", path, info.Mode().Perm())
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	return decodePEM(path, data, "PRIVATE KEY")
}

func linkedKeyError(path string) error {
	return fmt.Errorf("%s is a symbolic link: the CA key must be a plain file", path)
}

func readPEM(path, kind string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return decodePEM(path, data, kind)
}

func decodePEM(path string, data []byte, kind string) ([]byte, error) {
	block, _ := pem.Decode(data)
	if block == nil || block.Type != kind {
		return nil, fmt.Errorf("%s: no %s in it", path, kind)
	}
	return block.Bytes, nil
}

func randomSerial() *big.Int {
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	return serial
}
