package launch

import (
	"net/url"
	"strings"
)

// Destination is where a window showing Agentry's page sends a link.
type Destination int

const (
	// Nowhere is a link opened in neither: a local file, a script, or one
	// that doesn't parse.
	Nowhere Destination = iota
	// Window is a link on Agentry's own origin, opened in the window.
	Window
	// Browser is a link that leaves Agentry, opened in the system browser.
	Browser
)

// Route is where link goes from a window showing the page at origin. Only
// the same scheme, host and port stay in the window; another web page,
// even on the same host at another port, and mailto: go to the browser.
func Route(origin, link string) Destination {
	to, err := url.Parse(link)
	if err != nil {
		return Nowhere
	}
	switch strings.ToLower(to.Scheme) {
	case "mailto":
		return Browser
	case "http", "https":
	default:
		return Nowhere
	}
	if to.Host == "" {
		return Nowhere
	}
	from, err := url.Parse(origin)
	if err == nil && sameOrigin(from, to) {
		return Window
	}
	return Browser
}

func sameOrigin(a, b *url.URL) bool {
	return strings.EqualFold(a.Scheme, b.Scheme) &&
		strings.EqualFold(a.Hostname(), b.Hostname()) &&
		portOf(a) == portOf(b)
}

// portOf is the url's port, or its scheme's when it names none.
func portOf(u *url.URL) string {
	if port := u.Port(); port != "" {
		return port
	}
	if strings.EqualFold(u.Scheme, "https") {
		return "443"
	}
	return "80"
}
