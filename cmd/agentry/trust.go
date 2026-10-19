package main

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"html/template"
	"net/http"
	"net/netip"
	"net/url"
	"slices"
	"strings"
	"time"

	texttemplate "text/template"

	"gitlab.com/dev.chempa/agentry/internal/tlsca"
)

// The "Trust this device" page and what it hands out, under trustPrefix.

// trust is what the front door needs to help a device trust the CA and
// send it on to https.
type trust struct {
	ca        *tlsca.CA
	issuer    *tlsca.Issuer // what the server certificate covers
	addresses []string      // the https addresses serve prints
}

//go:embed trust.html
var trustHTML string

// trustPage is one self-contained page, with no script and nothing else to
// load: a device that doesn't trust the CA yet gets nothing else over
// plain HTTP.
var trustPage = template.Must(template.New("trust").Parse(trustHTML))

// trustView is what the trust page shows. It holds nothing secret.
type trustView struct {
	Name, Fingerprint string
	Addresses         []string // the https addresses to open once the CA is trusted
	Uncovered         string   // the address the page was asked at, when the certificate doesn't name it
}

// page writes the trust page with status, saying so when it was asked at
// uncovered, a host the certificate doesn't name.
func (door trust) page(w http.ResponseWriter, r *http.Request, status int, uncovered string) {
	view := trustView{Name: door.ca.Name(), Fingerprint: door.ca.Fingerprint(),
		Addresses: door.httpsAddresses(r.Host), Uncovered: uncovered}
	var page bytes.Buffer
	if err := trustPage.Execute(&page, view); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(status)
	w.Write(page.Bytes())
}

// httpsAddresses are the https addresses the certificate names: the one
// asked at first, then those serve prints. This machine's own (localhost)
// are left out unless asked at, as no other device reaches them.
func (door trust) httpsAddresses(askedAt string) []string {
	var addresses []string
	if askedAt != "" && door.issuer.Covers(hostOf(askedAt)) {
		addresses = append(addresses, "https://"+askedAt)
	}
	for _, address := range door.addresses {
		parsed, err := url.Parse(address)
		if err != nil || parsed.Scheme != "https" || onlyThisMachine(parsed.Hostname()) || !door.issuer.Covers(parsed.Hostname()) {
			continue
		}
		if !slices.Contains(addresses, address) {
			addresses = append(addresses, address)
		}
	}
	return addresses
}

// onlyThisMachine is a host no other device reaches.
func onlyThisMachine(host string) bool {
	ip, err := netip.ParseAddr(host)
	return strings.EqualFold(host, "localhost") || err == nil && ip.IsLoopback()
}

// profileName is the iOS configuration profile's name under trustPrefix.
const profileName = "agentry.mobileconfig"

// profile is an iOS configuration profile that installs the CA as a root,
// which Certificate Trust Settings then fully trusts. Its identifiers and
// UUIDs come from the CA, so downloading it again replaces the one
// installed rather than adding a second. It is unsigned, so iOS marks it
// Not Verified; the fingerprint answers that. Every value goes through xml.
var profile = texttemplate.Must(texttemplate.New("profile").Funcs(texttemplate.FuncMap{"xml": xmlText}).Parse(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>PayloadContent</key>
	<array>
		<dict>
			<key>PayloadCertificateFileName</key>
			<string>agentry-ca.crt</string>
			<key>PayloadContent</key>
			<data>{{xml .Certificate}}</data>
			<key>PayloadDescription</key>
			<string>Trusts the CA that signs this Agentry server's https certificate.</string>
			<key>PayloadDisplayName</key>
			<string>{{xml .Name}}</string>
			<key>PayloadIdentifier</key>
			<string>{{xml .Identifier}}.root</string>
			<key>PayloadType</key>
			<string>com.apple.security.root</string>
			<key>PayloadUUID</key>
			<string>{{xml .RootUUID}}</string>
			<key>PayloadVersion</key>
			<integer>1</integer>
		</dict>
	</array>
	<key>PayloadDescription</key>
	<string>Agentry's own CA, SHA-256 {{xml .Fingerprint}}</string>
	<key>PayloadDisplayName</key>
	<string>{{xml .Name}}</string>
	<key>PayloadIdentifier</key>
	<string>{{xml .Identifier}}</string>
	<key>PayloadType</key>
	<string>Configuration</string>
	<key>PayloadUUID</key>
	<string>{{xml .UUID}}</string>
	<key>PayloadVersion</key>
	<integer>1</integer>
</dict>
</plist>
`))

// profileView is what profile is filled with.
type profileView struct {
	Name, Fingerprint, Certificate string
	Identifier, UUID, RootUUID     string
}

func serveProfile(w http.ResponseWriter, r *http.Request, ca *tlsca.CA) {
	sum := sha256.Sum256(ca.DER())
	view := profileView{Name: ca.Name(), Fingerprint: ca.Fingerprint(), Certificate: base64.StdEncoding.EncodeToString(ca.DER()),
		Identifier: "agentry.ca." + hex.EncodeToString(sum[:8]), UUID: uuidOf("profile", ca.DER()), RootUUID: uuidOf("root", ca.DER())}
	var body bytes.Buffer
	if err := profile.Execute(&body, view); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/x-apple-aspen-config")
	w.Header().Set("Content-Disposition", `attachment; filename="`+profileName+`"`)
	http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(body.Bytes()))
}

// uuidOf is a UUID (version 8, made from a hash) that is the same for the
// same label and certificate, and differs for another.
func uuidOf(label string, der []byte) string {
	sum := sha256.Sum256(append([]byte(label+"\x00"), der...))
	sum[6] = sum[6]&0x0f | 0x80 // version 8
	sum[8] = sum[8]&0x3f | 0x80 // RFC 9562 variant
	return fmt.Sprintf("%X-%X-%X-%X-%X", sum[0:4], sum[4:6], sum[6:8], sum[8:10], sum[10:16])
}

// xmlText is text escaped for XML character data.
func xmlText(text string) string {
	var escaped strings.Builder
	xml.EscapeText(&escaped, []byte(text))
	return escaped.String()
}
