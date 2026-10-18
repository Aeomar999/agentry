package web

import (
	"net"
	"net/http"
	"slices"
	"strings"
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
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return scheme + "://" + r.Host
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
	answer := commands(chosen, path, code)
	all := make([]map[string]any, 0, len(list))
	for _, a := range list {
		each := commands(a.URL, path, code)
		each["url"], each["label"] = a.URL, a.Label
		all = append(all, each)
	}
	answer["addresses"], answer["address"] = all, chosen
	return answer
}
