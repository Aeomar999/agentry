package web

import (
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/core/httpapi"
)

// Signing in a device that doesn't have the token, such as the installed
// app, which opens at "/" without the printed link. A device that is signed
// in asks for a one-time code; the new one sends it to /api/signin and gets
// the token back, with the cookie. Pasting the token itself works there too.
// Codes live in memory: they last minutes, so a restart losing them is fine.

const (
	wrongCode  = "that code is wrong, used or expired: make a new one on a device that is signed in"
	wrongToken = "wrong token: paste the link `agentry serve` printed"
)

// issueSigninCode gives a signed-in device a code for another one, with the
// server's addresses, so the page can draw a QR code of <address>/?signin=<code>.
// An address served with the server's own CA has its trust page too, for a
// device to trust the CA first. expiresIn is in seconds: the page counts
// down from it, since its own clock may not agree with expiresAt.
func (s *Server) issueSigninCode(w http.ResponseWriter, r *http.Request) {
	code, expires, err := s.signins.issue()
	if err != nil {
		httpapi.Failed(w, err)
		return
	}
	list, chosen := s.addresses(r)
	addresses := make([]map[string]any, 0, len(list))
	for _, a := range list {
		address := map[string]any{"url": a.URL, "label": a.Label}
		if trust := s.trustAt(r, a.URL); trust != nil {
			address["trust"] = trust
		}
		addresses = append(addresses, address)
	}
	httpapi.Reply(w, map[string]any{"code": code, "expiresAt": expires, "expiresIn": int(codeLifetime / time.Second),
		"addresses": addresses, "address": chosen})
}

// signIn swaps a code, or the token pasted in, for the token and the cookie.
// It needs no token: it is how a device gets one. Failed attempts are
// limited, so codes can't be guessed.
func (s *Server) signIn(w http.ResponseWriter, r *http.Request) {
	var request struct{ Code, Token string }
	if !httpapi.Decode(w, r, &request) {
		return
	}
	withCode := request.Token == ""
	if withCode && strings.TrimSpace(request.Code) == "" {
		http.Error(w, "send a code or the token", http.StatusBadRequest)
		return
	}
	try, wait := s.signins.try(clientAddress(r), withCode)
	if wait > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(wait.Seconds()))))
		http.Error(w, "too many failed tries: wait a minute and try again", http.StatusTooManyRequests)
		return
	}

	signedIn, refusal := s.signins.redeem(request.Code), wrongCode
	if !withCode {
		signedIn, refusal = s.isUIToken(request.Token), wrongToken
	}
	if !signedIn {
		http.Error(w, refusal, http.StatusUnauthorized)
		return
	}
	s.signins.succeeded(try)
	setUICookie(w, r, s.config.UIToken)
	httpapi.Reply(w, map[string]string{"token": s.config.UIToken})
}

// signOut forgets the cookie. The page forgets its own copy of the token.
func (s *Server) signOut(w http.ResponseWriter, r *http.Request) {
	clearUICookie(w, r)
	httpapi.Reply(w, map[string]string{})
}

// clientAddress is the address a request came from, without its port.
// Behind a proxy every client shares the proxy's: a forwarded header could
// be made up, and it would let one client look like many.
func clientAddress(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
