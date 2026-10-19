package main

import (
	"bytes"
	"crypto/tls"
	"net"
	"net/http"
	"net/netip"
	"strconv"
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
// line and desk call it on loopback or on the address it serves (serve
// -addr <LAN IP>); anyone else is sent to the same address over https, if
// the certificate covers it, or gets the trust page saying where to go.
// Under trustPrefix, over either, without the token, is what a device needs
// to trust the CA: the trust page, the CA certificate at /trust/ca.crt (DER,
// as phones install it) and /trust/ca.pem, and an iOS profile holding it;
// the CA's key is never served.
func frontDoor(next http.Handler, door trust) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+trustPrefix+"{$}", func(w http.ResponseWriter, r *http.Request) {
		door.page(w, r, http.StatusOK, "")
	})
	mux.HandleFunc("GET "+strings.TrimSuffix(trustPrefix, "/"), func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, trustPrefix, http.StatusPermanentRedirect)
	})
	mux.HandleFunc("GET "+trustPrefix+"ca.crt", func(w http.ResponseWriter, r *http.Request) {
		serveCA(w, r, "application/x-x509-ca-cert", "ca.crt", door.ca.DER())
	})
	mux.HandleFunc("GET "+trustPrefix+"ca.pem", func(w http.ResponseWriter, r *http.Request) {
		serveCA(w, r, "application/x-pem-file", "ca.pem", door.ca.PEM())
	})
	mux.HandleFunc("GET "+trustPrefix+profileName, func(w http.ResponseWriter, r *http.Request) {
		serveProfile(w, r, door.ca)
	})
	mux.HandleFunc(trustPrefix, http.NotFound) // nothing else is under it
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.TLS != nil || fromThisMachine(r) {
			next.ServeHTTP(w, r)
			return
		}
		if r.Host == "" {
			http.Error(w, "Agentry is served over https here", http.StatusBadRequest)
			return
		}
		if !door.issuer.Covers(hostOf(r.Host)) {
			door.page(w, r, http.StatusMisdirectedRequest, r.Host)
			return
		}
		http.Redirect(w, r, "https://"+httpsHost(r)+r.URL.RequestURI(), http.StatusPermanentRedirect)
	})
	return mux
}

func serveCA(w http.ResponseWriter, r *http.Request, contentType, name string, body []byte) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", `attachment; filename="agentry-`+name+`"`)
	http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(body))
}

// hostOf is a Host header's host, without its port or an IPv6 address's
// brackets.
func hostOf(hostPort string) string {
	if host, _, err := net.SplitHostPort(hostPort); err == nil {
		return host
	}
	return strings.Trim(hostPort, "[]")
}

// httpsHost is the host and port to send a request on to over https: the
// host it asked at, and the port it asked at when that is a port number,
// else the port it arrived on. Nothing else in Host reaches a link.
func httpsHost(r *http.Request) string {
	host := hostOf(r.Host)
	_, port, err := net.SplitHostPort(r.Host)
	if err != nil {
		port = "" // Host has none
	} else if _, err := strconv.ParseUint(port, 10, 16); err != nil {
		port = ""
		if local := arrivedOn(r); local != nil {
			_, port, _ = net.SplitHostPort(local.String())
		}
	}
	switch {
	case port != "":
		return net.JoinHostPort(host, port)
	case strings.Contains(host, ":"):
		return "[" + host + "]" // IPv6
	default:
		return host
	}
}

// arrivedOn is the local address a request arrived on, or nil.
func arrivedOn(r *http.Request) net.Addr {
	local, _ := r.Context().Value(http.LocalAddrContextKey).(net.Addr)
	return local
}

// fromThisMachine is a request this machine sent: from loopback, or from
// the very address it arrived on.
func fromThisMachine(r *http.Request) bool {
	remote, ok := ipOf(r.RemoteAddr)
	if !ok {
		return false
	}
	if remote.IsLoopback() {
		return true
	}
	local := arrivedOn(r)
	if local == nil {
		return false
	}
	arrivedOnIP, ok := ipOf(local.String())
	return ok && remote == arrivedOnIP
}

// ipOf is the IP of a host:port address, IPv4 in IPv6 unmapped.
func ipOf(address string) (netip.Addr, bool) {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return netip.Addr{}, false
	}
	ip, err := netip.ParseAddr(host)
	return ip.Unmap(), err == nil
}
