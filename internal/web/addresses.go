package web

import (
	"encoding/pem"
	"net"
	"net/http"
	"slices"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/tlsca"
)

// The addresses other machines can reach this server at, for the install
// commands the Workers and Integrations pages hand out. The page is often
// open at localhost, which no other machine reaches, so its own address is
// not enough: the commands come for every address, with one picked.

// address is one way to reach the server, and what the page calls it.
type address struct {
	URL   string
	Label string
}

const thisComputerOnly = "This computer only"

// addresses are the server's addresses, in order: the public URL, each
// network address, the one this request came to if it isn't listed, and
// this computer's own last. The second result is the one to show first: the
// public URL, else for a page open on this computer the first network
// address, else the address the page is open at.
func (s *Server) addresses(r *http.Request) ([]address, string) {
	var list []address
	add := func(url, label string) {
		if url != "" && !slices.ContainsFunc(list, func(a address) bool { return a.URL == url }) {
			list = append(list, address{URL: url, Label: label})
		}
	}
	public := strings.TrimSuffix(s.config.PublicURL, "/")
	add(public, "Public address")
	for _, url := range s.config.NetworkURLs {
		add(url, "On your network")
	}
	here, local := requestURL(r), isLoopback(r.Host)
	if !local {
		add(here, "The address this page is open at")
	}
	thisComputer := s.config.LocalURL
	if local {
		thisComputer = here // the page reaches it there, so the commands will too
	}
	add(thisComputer, thisComputerOnly)

	switch {
	case public != "":
		return list, public
	case local && len(s.config.NetworkURLs) > 0:
		return list, s.config.NetworkURLs[0]
	default:
		return list, here
	}
}

// requestURL is the address this request came to.
func requestURL(r *http.Request) string {
	scheme := "http"
	if overHTTPS(r) {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

// overHTTPS is a request that came over https, here or to a proxy in front.
func overHTTPS(r *http.Request) bool {
	return r.TLS != nil || forwardedProto(r) == "https"
}

// forwardedProto is how the client reached the reverse proxy that passed r
// on, in lower case, or "" for a request no proxy marked. Proxies say it
// in X-Forwarded-Proto, in Forwarded (RFC 7239: the first element's
// proto), or with X-Forwarded-Ssl: on.
func forwardedProto(r *http.Request) string {
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		first, _, _ := strings.Cut(proto, ",")
		return strings.ToLower(strings.TrimSpace(first))
	}
	if forwarded := r.Header.Get("Forwarded"); forwarded != "" {
		first, _, _ := strings.Cut(forwarded, ",")
		for pair := range strings.SplitSeq(first, ";") {
			name, value, _ := strings.Cut(strings.TrimSpace(pair), "=")
			if strings.EqualFold(name, "proto") {
				return strings.ToLower(strings.Trim(value, `"`))
			}
		}
	}
	switch strings.ToLower(r.Header.Get("X-Forwarded-Ssl")) {
	case "on":
		return "https"
	case "off":
		return "http"
	}
	return ""
}

// isLoopback is a host, with or without its port, only this computer reaches.
func isLoopback(host string) bool {
	if name, _, err := net.SplitHostPort(host); err == nil {
		host = name
	}
	host = strings.Trim(host, "[]")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// installCommands are the install commands for the script at path with
// code, for every address, and the default one at the top level as before.
func (s *Server) installCommands(r *http.Request, path, code string) map[string]any {
	list, chosen := s.addresses(r)
	answer := s.commandsAt(r, chosen, path, code)
	all := make([]map[string]any, 0, len(list))
	for _, a := range list {
		each := s.commandsAt(r, a.URL, path, code)
		each["url"], each["label"] = a.URL, a.Label
		all = append(all, each)
	}
	answer["addresses"], answer["address"] = all, chosen
	return answer
}

// commandsAt are the commands for the script at url, pinned to the
// server's own CA over https where it serves url with it.
func (s *Server) commandsAt(r *http.Request, url, path, code string) map[string]any {
	if !s.servedWithOwnCA(r, url) {
		return commands(url, path, code, "")
	}
	if rest, plain := strings.CutPrefix(url, "http://"); plain {
		url = "https://" + rest
	}
	return commands(url, path, code, s.caFingerprint())
}

// servedWithOwnCA is whether url, the server's address r came to or one it
// was offered, is served with Agentry's own CA (-https): every address but
// the public URL, or one a reverse proxy in front passed r on from. A proxy
// marks r (forwardedProto), even one reaching Agentry over https itself,
// as it shows its own certificate. A client faking the header only
// gets commands that trust the system's CAs instead. Plain HTTP this machine
// sends (localhost) counts, as https reaches it too.
func (s *Server) servedWithOwnCA(r *http.Request, url string) bool {
	switch {
	case s.config.CA == "" || url == strings.TrimSuffix(s.config.PublicURL, "/"):
		return false
	case url == requestURL(r):
		return forwardedProto(r) == ""
	default:
		return true
	}
}

// caFingerprint is the SHA-256 of the server's own CA, as `agentry ca`
// shows it.
func (s *Server) caFingerprint() string {
	block, _ := pem.Decode([]byte(s.config.CA))
	if block == nil {
		return ""
	}
	return tlsca.FingerprintOf(block.Bytes)
}
