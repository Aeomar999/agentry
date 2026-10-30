// Package hostaddrs finds the addresses other machines reach this one on:
// what serve prints as links and what its certificate names.
package hostaddrs

import (
	"net"
	"strings"
)

// Network is this machine's own IPv4 addresses other machines reach, on the
// LAN or a private network (Hawser, Tailscale): every address of an
// interface that is up, leaving out loopback, link-local addresses and the
// bridges containers use, which no other machine reaches.
func Network() []net.IP {
	interfaces, _ := net.Interfaces()
	var ips []net.IP
	for _, i := range interfaces {
		if i.Flags&net.FlagUp == 0 || i.Flags&net.FlagLoopback != 0 || IsBridge(i.Name) {
			continue
		}
		addresses, _ := i.Addrs()
		for _, address := range addresses {
			if ip, ok := address.(*net.IPNet); ok && ip.IP.To4() != nil && !ip.IP.IsLinkLocalUnicast() {
				ips = append(ips, ip.IP)
			}
		}
	}
	return ips
}

// IsBridge is true for the interfaces containers and virtual machines use.
func IsBridge(name string) bool {
	for _, prefix := range []string{"docker", "br-", "veth", "virbr", "cni", "flannel", "vEthernet"} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}
