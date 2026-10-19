package main

import (
	"bytes"
	"crypto/tls"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/tlsca"
)

// tlsConfig serves the certificate issuer hands out, which it issues anew
// when the addresses change or it nears its end, with no restart.
func tlsConfig(issuer *tlsca.Issuer) *tls.Config {
	return &tls.Config{MinVersion: tls.VersionTLS12, GetCertificate: issuer.GetCertificate,
		NextProtos: []string{"h2", "http/1.1"}}
}

// trustPrefix is where a device gets what it needs to trust the CA, over
// plain HTTP too, and without the token: it can't open the https address
// before it trusts the CA.
const trustPrefix = "/trust/"

// frontDoor is the server with -https on. Over TLS it is next. Over plain
// HTTP it is next only for this machine, whose hooks, `agentry mcp`, status
// line and desk call it on loopback, and for trustPrefix; anyone else is
// sent to the same address over https. The CA certificate is at
// /trust/ca.crt (DER, as phones install it) and /trust/ca.pem; its key is
// never served.
func frontDoor(next http.Handler, ca *tlsca.CA) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+trustPrefix+"ca.crt", func(w http.ResponseWriter, r *http.Request) {
		serveCA(w, r, "application/x-x509-ca-cert", "ca.crt", ca.DER())
	})
	mux.HandleFunc("GET "+trustPrefix+"ca.pem", func(w http.ResponseWriter, r *http.Request) {
		serveCA(w, r, "application/x-pem-file", "ca.pem", ca.PEM())
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.TLS != nil || fromLoopback(r) || strings.HasPrefix(r.URL.Path, trustPrefix) {
			next.ServeHTTP(w, r)
			return
		}
		if r.Host == "" {
			http.Error(w, "Agentry is served over https here", http.StatusBadRequest)
			return
		}
		http.Redirect(w, r, "https://"+r.Host+r.URL.RequestURI(), http.StatusPermanentRedirect)
	})
	return mux
}

func serveCA(w http.ResponseWriter, r *http.Request, contentType, name string, body []byte) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", `attachment; filename="agentry-`+name+`"`)
	http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(body))
}

// fromLoopback is a request this machine sent.
func fromLoopback(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	ip, err := netip.ParseAddr(host)
	return err == nil && ip.Unmap().IsLoopback()
}
