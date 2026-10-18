package web

import (
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

// The install commands come for every address the server has, in order,
// with the one another machine most likely reaches picked first.
func TestAddresses(t *testing.T) {
	network := []string{"http://192.168.1.5:7800", "http://10.0.0.2:7800"}
	everywhere := Config{NetworkURLs: network, LocalURL: "http://localhost:7800"}
	for _, c := range []struct {
		name   string
		config Config
		host   string // the page is open at
		want   []address
		chosen string
	}{
		{"open at localhost, on every interface", everywhere, "localhost:7800",
			[]address{{network[0], "On your network"}, {network[1], "On your network"}, {"http://localhost:7800", thisComputerOnly}}, network[0]},
		{"open at a network address", everywhere, "10.0.0.2:7800",
			[]address{{network[0], "On your network"}, {network[1], "On your network"}, {"http://localhost:7800", thisComputerOnly}}, network[1]},
		{"open at a name", everywhere, "agentry.lan:7800",
			[]address{{network[0], "On your network"}, {network[1], "On your network"},
				{"http://agentry.lan:7800", "The address this page is open at"}, {"http://localhost:7800", thisComputerOnly}}, "http://agentry.lan:7800"},
		{"a public URL", Config{PublicURL: "https://agentry.example.com/", NetworkURLs: network, LocalURL: "http://localhost:7800"}, "127.0.0.1:7800",
			[]address{{"https://agentry.example.com", "Public address"}, {network[0], "On your network"}, {network[1], "On your network"},
				{"http://127.0.0.1:7800", thisComputerOnly}}, "https://agentry.example.com"},
		{"listening on localhost only", Config{LocalURL: "http://localhost:7800"}, "127.0.0.1:7800",
			[]address{{"http://127.0.0.1:7800", thisComputerOnly}}, "http://127.0.0.1:7800"},
		{"listening on localhost only, open at localhost", Config{LocalURL: "http://localhost:7800"}, "localhost:7800",
			[]address{{"http://localhost:7800", thisComputerOnly}}, "http://localhost:7800"},
	} {
		request := httptest.NewRequest("POST", "/api/workers/invite", nil)
		request.Host = c.host
		list, chosen := (&Server{config: c.config}).addresses(request)
		if !reflect.DeepEqual(list, c.want) || chosen != c.chosen {
			t.Errorf("%s:\n got %v, %s\nwant %v, %s", c.name, list, chosen, c.want, c.chosen)
		}
	}
}

func TestIsLoopback(t *testing.T) {
	for host, want := range map[string]bool{"localhost:7800": true, "LOCALHOST": true, "127.0.0.1:7800": true, "[::1]:7800": true,
		"127.0.0.2": true, "192.168.1.5:7800": false, "agentry.lan": false, "[fe80::1]:7800": false} {
		if isLoopback(host) != want {
			t.Errorf("isLoopback(%s) = %v", host, !want)
		}
	}
}

// Each address has its own commands; the top-level ones are the picked address's.
func TestInstallCommands(t *testing.T) {
	request := httptest.NewRequest("POST", "/api/workers/invite", nil)
	request.Host = "localhost:7800"
	server := &Server{config: Config{NetworkURLs: []string{"http://192.168.1.5:7800"}, LocalURL: "http://localhost:7800"}}
	answer := server.installCommands(request, "/worker/install", "c0de")
	if answer["command"] != "curl -fsSL 'http://192.168.1.5:7800/worker/install.sh?code=c0de' | sh" || answer["address"] != "http://192.168.1.5:7800" {
		t.Fatalf("the default: %v", answer)
	}
	all := answer["addresses"].([]map[string]any)
	if len(all) != 2 || all[1]["url"] != "http://localhost:7800" || all[1]["label"] != thisComputerOnly ||
		!strings.Contains(all[1]["windowsCommand"].(string), "http://localhost:7800/worker/install.ps1?code=c0de") {
		t.Fatalf("every address: %v", all)
	}
}
