package main

import (
	"net"
	"strings"
	"testing"
)

// Hooks and the desk call the server on loopback when it listens on every
// address, as it does in a container.
func TestLocalURL(t *testing.T) {
	for addr, want := range map[string]string{
		"0.0.0.0:7800":   "http://127.0.0.1:7800",
		"127.0.0.1:7801": "http://127.0.0.1:7801",
		"[::]:7802":      "http://127.0.0.1:7802",
	} {
		tcp, _ := net.ResolveTCPAddr("tcp", addr)
		if got := localURL(tcp); got != want {
			t.Errorf("localURL(%s) = %s, want %s", addr, got, want)
		}
	}
}

// Listening on every address, serve prints a link for each of this machine's
// own addresses other machines reach; listening on one, it prints none.
func TestNetworkURLs(t *testing.T) {
	one, _ := net.ResolveTCPAddr("tcp", "127.0.0.1:7800")
	if urls := networkURLs(one); urls != nil {
		t.Errorf("on one address: %v", urls)
	}
	every, _ := net.ResolveTCPAddr("tcp", "0.0.0.0:7800")
	for _, url := range networkURLs(every) {
		if !strings.HasPrefix(url, "http://") || !strings.HasSuffix(url, ":7800") || strings.Contains(url, "127.0.0.1") {
			t.Errorf("a network URL: %s", url)
		}
	}
	for name, want := range map[string]bool{"docker0": true, "br-1a2b": true, "veth12": true, "eth0": false, "wlan0": false} {
		if isBridge(name) != want {
			t.Errorf("isBridge(%s) = %v", name, !want)
		}
	}
}
