// Package pinned is how Agentry's own clients reach an Agentry server served
// over https with Agentry's own CA: they trust that CA and nothing else, not
// the system's store. With no CA they are the plain defaults, so a server
// over plain HTTP, or behind a reverse proxy with a public certificate, is
// reached exactly as before.
package pinned

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net/http"
	"time"
)

// TLSConfig trusts only the certificates in caPEM. It is nil with no CA:
// the defaults.
func TLSConfig(caPEM []byte) *tls.Config {
	if len(caPEM) == 0 {
		return nil
	}
	// A CA that doesn't parse leaves the pool empty: every server is
	// refused, never trusted by the system store instead.
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(caPEM)
	return &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
}

// Transport is http.DefaultTransport trusting only caPEM, or the default
// itself with no CA.
func Transport(caPEM []byte) http.RoundTripper {
	config := TLSConfig(caPEM)
	if config == nil {
		return http.DefaultTransport
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = config
	return transport
}

// Client trusts only caPEM, and gives up after timeout (0 for never). With
// no CA it is a plain client.
func Client(caPEM []byte, timeout time.Duration) *http.Client {
	if len(caPEM) == 0 {
		return &http.Client{Timeout: timeout}
	}
	return &http.Client{Transport: Transport(caPEM), Timeout: timeout}
}

// Untrusted says whether err is a server's certificate this client
// doesn't trust, as after the server's CA was regenerated, rather than a
// server it can't reach.
func Untrusted(err error) bool {
	var verification *tls.CertificateVerificationError
	var unknown x509.UnknownAuthorityError
	return errors.As(err, &verification) || errors.As(err, &unknown)
}

// Check says whether caPEM is one CA certificate, as a CA kept to pin
// must be.
func Check(caPEM []byte) error {
	block, rest := pem.Decode(caPEM)
	if block == nil || block.Type != "CERTIFICATE" {
		return errors.New("the CA isn't a PEM certificate")
	}
	if next, _ := pem.Decode(rest); next != nil {
		return errors.New("the CA holds more than one certificate")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return err
	}
	if !cert.IsCA {
		return errors.New("the certificate isn't a CA's")
	}
	return nil
}
