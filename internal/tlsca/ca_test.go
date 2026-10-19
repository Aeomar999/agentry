package tlsca

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestTheCAIsMadeOnceAndReused(t *testing.T) {
	dir := filepath.Join(t.TempDir(), Folder)
	first, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	hostname, _ := os.Hostname()
	cert := first.cert
	if cert.Subject.CommonName != "Agentry local CA ("+hostname+")" {
		t.Errorf("CN = %q", cert.Subject.CommonName)
	}
	if !cert.IsCA || !cert.MaxPathLenZero || cert.MaxPathLen != 0 {
		t.Errorf("IsCA %v, MaxPathLen %d (zero %v)", cert.IsCA, cert.MaxPathLen, cert.MaxPathLenZero)
	}
	if years := cert.NotAfter.Sub(cert.NotBefore).Hours() / 24 / 365; years < 9.9 || years > 10.1 {
		t.Errorf("lifetime %.1f years", years)
	}
	if runtime.GOOS != "windows" {
		for path, want := range map[string]os.FileMode{dir: 0o700, filepath.Join(dir, keyFile): 0o600} {
			if info, err := os.Stat(path); err != nil || info.Mode().Perm() != want {
				t.Errorf("%s: %v, %v; want %v", path, info.Mode().Perm(), err, want)
			}
		}
	}

	again, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if again.Fingerprint() != first.Fingerprint() || !again.key.Equal(first.key) {
		t.Error("a second Open made a new CA")
	}
}

func TestAKeyOthersMayReadIsRefused(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows keeps no Unix permissions")
	}
	dir := t.TempDir()
	if _, err := Open(dir); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []os.FileMode{0o644, 0o640, 0o604} {
		os.Chmod(filepath.Join(dir, keyFile), mode)
		if _, err := Open(dir); err == nil || !strings.Contains(err.Error(), "chmod 600") {
			t.Errorf("mode %v: %v", mode, err)
		}
	}
	os.Chmod(filepath.Join(dir, keyFile), 0o400)
	if _, err := Open(dir); err != nil {
		t.Errorf("a key only its owner reads: %v", err)
	}
}

func TestAKeyThatIsALinkIsRefused(t *testing.T) {
	dir, elsewhere := t.TempDir(), t.TempDir()
	if _, err := Open(elsewhere); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{keyFile, certFile} {
		data, _ := os.ReadFile(filepath.Join(elsewhere, name))
		os.WriteFile(filepath.Join(dir, name), data, 0o600)
	}
	os.Remove(filepath.Join(dir, keyFile))
	if err := os.Symlink(filepath.Join(elsewhere, keyFile), filepath.Join(dir, keyFile)); err != nil {
		t.Skip("no symbolic links here:", err)
	}
	if _, err := Open(dir); err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Errorf("a linked key: %v", err)
	}
}

func TestAKeyThatIsNotAFileIsRefused(t *testing.T) {
	dir := t.TempDir()
	if _, err := Open(dir); err != nil {
		t.Fatal(err)
	}
	os.Remove(filepath.Join(dir, keyFile))
	os.Mkdir(filepath.Join(dir, keyFile), 0o700)
	if _, err := Open(dir); err == nil || !strings.Contains(err.Error(), "not a plain file") {
		t.Errorf("a folder for a key: %v", err)
	}
}

func TestAFolderThatIsALinkOrAFileIsRefused(t *testing.T) {
	elsewhere := t.TempDir()
	if _, err := Open(elsewhere); err != nil {
		t.Fatal(err)
	}
	linked := filepath.Join(t.TempDir(), Folder)
	if err := os.Symlink(elsewhere, linked); err != nil {
		t.Skip("no symbolic links here:", err)
	}
	if _, err := Open(linked); err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Errorf("a linked folder: %v", err)
	}

	file := filepath.Join(t.TempDir(), Folder)
	os.WriteFile(file, nil, 0o600)
	if _, err := Open(file); err == nil || !strings.Contains(err.Error(), "not a folder") {
		t.Errorf("a file for a folder: %v", err)
	}
}

func TestAFolderOpenToOthersIsClosed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows keeps no Unix permissions")
	}
	dir := filepath.Join(t.TempDir(), Folder)
	if _, err := Open(dir); err != nil {
		t.Fatal(err)
	}
	os.Chmod(dir, 0o755)
	if _, err := Open(dir); err != nil {
		t.Fatal(err)
	}
	if info, _ := os.Stat(dir); info.Mode().Perm() != 0o700 {
		t.Errorf("the folder is %v after Open, want 0700", info.Mode().Perm())
	}
}

func TestAHalfMadeOrMismatchedCAIsRefused(t *testing.T) {
	dir := t.TempDir()
	if _, err := Open(dir); err != nil {
		t.Fatal(err)
	}
	os.Remove(filepath.Join(dir, certFile))
	if _, err := Open(dir); err == nil || !strings.Contains(err.Error(), "incomplete") {
		t.Errorf("a key with no certificate: %v", err)
	}

	other := t.TempDir()
	if _, err := Open(other); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(other, certFile))
	os.WriteFile(filepath.Join(dir, certFile), data, 0o644)
	if _, err := Open(dir); err == nil || !strings.Contains(err.Error(), "doesn't belong") {
		t.Errorf("another CA's certificate: %v", err)
	}
}

func TestACorruptCertificateSaysToRegenerate(t *testing.T) {
	dir := t.TempDir()
	if _, err := Open(dir); err != nil {
		t.Fatal(err)
	}
	certPath := filepath.Join(dir, certFile)
	for name, data := range map[string][]byte{
		"not PEM":       []byte("garbage"),
		"not a cert":    pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("garbage")}),
		"the wrong PEM": pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: []byte("garbage")}),
	} {
		os.WriteFile(certPath, data, 0o644)
		if _, err := Open(dir); err == nil || !strings.Contains(err.Error(), certFile+" is corrupt") || !strings.Contains(err.Error(), "regenerate the CA") {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestRegenerateMakesANewCA(t *testing.T) {
	dir := t.TempDir()
	old, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := Regenerate(dir); err != nil {
		t.Fatal(err)
	}
	fresh, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Fingerprint() == old.Fingerprint() {
		t.Error("the same CA after Regenerate")
	}
	if err := Regenerate(t.TempDir()); err != nil {
		t.Errorf("regenerating where there is no CA: %v", err)
	}
}

func TestTheCertificateIsHandedOutInEachForm(t *testing.T) {
	ca, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(ca.PEM()), "-----BEGIN CERTIFICATE-----\n") || strings.Contains(string(ca.PEM()), "PRIVATE") {
		t.Errorf("PEM:\n%s", ca.PEM())
	}
	sum := sha256.Sum256(ca.DER())
	want := strings.ToUpper(hex.EncodeToString(sum[:]))
	if got := strings.ReplaceAll(ca.Fingerprint(), ":", ""); got != want || len(ca.Fingerprint()) != 32*3-1 {
		t.Errorf("Fingerprint = %s, want the SHA-256 %s", ca.Fingerprint(), want)
	}
}

func TestTheCAIsNameConstrained(t *testing.T) {
	ca, err := open(t.TempDir(), "Box")
	if err != nil {
		t.Fatal(err)
	}
	cert := ca.cert
	if !cert.PermittedDNSDomainsCritical {
		t.Error("the name constraints aren't critical")
	}
	if want := []string{"localhost", "box", "box.local"}; !slices.Equal(cert.PermittedDNSDomains, want) {
		t.Errorf("DNS names %v, want %v", cert.PermittedDNSDomains, want)
	}
	var ranges []string
	for _, r := range cert.PermittedIPRanges {
		ranges = append(ranges, r.String())
	}
	if !slices.Equal(ranges, privateRanges) {
		t.Errorf("IP ranges %v, want %v", ranges, privateRanges)
	}
	if dotted, _ := open(t.TempDir(), "box.lan"); !slices.Equal(dotted.cert.PermittedDNSDomains, []string{"localhost", "box.lan"}) {
		t.Errorf("a dotted hostname: %v", dotted.cert.PermittedDNSDomains)
	}
}

// Every host the machine prints a link for is one the CA may vouch for.
func TestALeafForEveryHostVerifies(t *testing.T) {
	ca, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	hosts := normalHosts(Hosts())
	cert, err := ca.Issue(hosts, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, host := range hosts {
		if net.ParseIP(host) != nil && !ca.Permits(host) {
			t.Logf("%s is outside the private ranges: left out", host)
			continue
		}
		if _, err := cert.Leaf.Verify(x509.VerifyOptions{DNSName: host, Roots: ca.Pool()}); err != nil {
			t.Errorf("%s: %v", host, err)
		}
	}
}

func serial(t *testing.T) *big.Int {
	t.Helper()
	serial, err := randomSerial()
	if err != nil {
		t.Fatal(err)
	}
	return serial
}

// A leaf the CA key signs for a public name or address is refused by Go's
// verifier, as by browsers: the constraints make a leaked key useless there.
func TestALeafForThePublicWebIsRefused(t *testing.T) {
	ca, err := open(t.TempDir(), "box")
	if err != nil {
		t.Fatal(err)
	}
	for _, host := range []string{"example.com", "8.8.8.8", "box.example.com", "notbox", "2001:4860:4860::8888"} {
		template := &x509.Certificate{
			SerialNumber: serial(t),
			NotBefore:    time.Now().Add(-time.Hour),
			NotAfter:     time.Now().Add(time.Hour),
			ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		}
		if ip := net.ParseIP(host); ip != nil {
			template.IPAddresses = []net.IP{ip}
		} else {
			template.DNSNames = []string{host}
		}
		key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		der, err := x509.CreateCertificate(rand.Reader, template, ca.cert, &key.PublicKey, ca.key)
		if err != nil {
			t.Fatal(err)
		}
		leaf, _ := x509.ParseCertificate(der)
		_, err = leaf.Verify(x509.VerifyOptions{DNSName: host, Roots: ca.Pool()})
		if !errors.As(err, new(x509.CertificateInvalidError)) || err.(x509.CertificateInvalidError).Reason != x509.CANotAuthorizedForThisName {
			t.Errorf("%s: %v, want a name constraint error", host, err)
		}
	}
}

func TestACAWithoutConstraintsIsRefused(t *testing.T) {
	dir := t.TempDir()
	ca, err := open(dir, "box")
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber:          serial(t),
		Subject:               ca.cert.Subject,
		NotBefore:             ca.cert.NotBefore,
		NotAfter:              ca.cert.NotAfter,
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &ca.key.PublicKey, ca.key)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, certFile), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644)
	if _, err := Open(dir); err == nil || !strings.Contains(err.Error(), "no name constraints") || !strings.Contains(err.Error(), "regenerate") {
		t.Errorf("a CA without constraints: %v", err)
	}
}
