// Package tlsca is Agentry's own certificate authority, for serving over
// HTTPS with a certificate devices trust: the user trusts the CA once per
// device, as with mkcert, and the server certificate it signs names every
// address the server is reached on.
//
// The CA is not name constrained. Constraints are fixed when a device trusts
// the CA, but the addresses are not: a new LAN, a new private network or a
// renamed machine would then need every device to trust a new CA. The CA's
// key instead never leaves the data folder (0600, in a 0700 folder agents
// may not touch), and Regenerate replaces it in one step.
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

// Open loads the CA from dir, or makes one there on first use. It refuses a
// key file that is a symbolic link or that others than its owner may read.
func Open(dir string) (*CA, error) {
	keyPath, certPath := filepath.Join(dir, keyFile), filepath.Join(dir, certFile)
	_, keyErr := os.Lstat(keyPath)
	_, certErr := os.Lstat(certPath)
	switch {
	case errors.Is(keyErr, fs.ErrNotExist) && errors.Is(certErr, fs.ErrNotExist):
		return create(dir)
	case errors.Is(keyErr, fs.ErrNotExist) || errors.Is(certErr, fs.ErrNotExist):
		return nil, fmt.Errorf("the CA in %s is incomplete: regenerate it", dir)
	}
	return load(keyPath, certPath)
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

func create(dir string) (*CA, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	hostname, _ := os.Hostname()
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
