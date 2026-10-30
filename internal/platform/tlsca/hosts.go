package tlsca

import (
	"os"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/platform/hostaddrs"
)

// Hosts are the names and addresses this machine is reached on: its
// hostname, hostname.local for devices that find it by multicast DNS (phones
// and Macs on the LAN), and every network address serve prints a link for.
// Issue adds localhost and loopback.
func Hosts() []string {
	var hosts []string
	if hostname, err := os.Hostname(); err == nil && hostname != "" {
		hosts = append(hosts, hostname)
		if !strings.Contains(hostname, ".") {
			hosts = append(hosts, hostname+".local")
		}
	}
	for _, ip := range hostaddrs.Network() {
		hosts = append(hosts, ip.String())
	}
	return hosts
}
