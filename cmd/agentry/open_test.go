package main

import (
	"bytes"
	"errors"
	"net"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/client"
)

// Start Agentry prints a signed-in link for this computer, the public
// address and each network address, then opens this computer's in the browser.
func TestOpenPagePrintsEveryLink(t *testing.T) {
	var opened []string
	saved := openBrowser
	openBrowser = func(link string) error { opened = append(opened, link); return nil }
	t.Cleanup(func() { openBrowser = saved })

	every, _ := net.ResolveTCPAddr("tcp", "0.0.0.0:7901")
	network := networkURLs(every)
	var out bytes.Buffer
	c := client.Client{Where: client.Where{URL: "http://127.0.0.1:7901", Page: "https://agentry.example.com/", Listen: "0.0.0.0:7901"}, Token: "abc"}
	if code := openPage(c, &out, &out); code != 0 {
		t.Fatalf("exit %d", code)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	want := []string{
		"Agentry is running. These links sign you in, so share them only with people you trust:",
		"  This computer:   http://localhost:7901/?token=abc",
		"  Public address:  https://agentry.example.com/?token=abc",
	}
	for _, other := range network {
		want = append(want, "  On your network: "+other+"/?token=abc")
	}
	want = append(want, "Opened the first one in your browser. You can close this window: Agentry keeps running; Stop Agentry stops it.")
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Errorf("printed:\n%s\nwant:\n%s", out.String(), strings.Join(want, "\n"))
	}
	if len(opened) != 1 || opened[0] != "http://localhost:7901/?token=abc" {
		t.Errorf("opened %v", opened)
	}
}

// Listening on this computer only, or from a server.json that doesn't say,
// it prints only this computer's link; without a browser it says to open it.
func TestOpenPageOnThisComputerOnly(t *testing.T) {
	saved := openBrowser
	openBrowser = func(string) error { return errors.New("no browser") }
	t.Cleanup(func() { openBrowser = saved })

	for _, listen := range []string{"127.0.0.1:7901", ""} {
		var out bytes.Buffer
		c := client.Client{Where: client.Where{URL: "http://127.0.0.1:7901", Page: "http://127.0.0.1:7901", Listen: listen}, Token: "abc"}
		openPage(c, &out, &out)
		want := "Agentry is running. These links sign you in, so share them only with people you trust:\n" +
			"  This computer:   http://localhost:7901/?token=abc\n" +
			"Open the first link in your browser. Agentry keeps running; Stop Agentry stops it.\n"
		if out.String() != want {
			t.Errorf("listening on %q, printed:\n%s", listen, out.String())
		}
	}
}
