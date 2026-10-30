package hostaddrs

import "testing"

func TestNetworkLeavesOutLoopbackAndLinkLocal(t *testing.T) {
	for _, ip := range Network() {
		if ip.To4() == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
			t.Errorf("Network has %s", ip)
		}
	}
}

func TestIsBridge(t *testing.T) {
	for name, want := range map[string]bool{"docker0": true, "br-1a2b": true, "veth12": true, "eth0": false, "wlan0": false, "tailscale0": false} {
		if IsBridge(name) != want {
			t.Errorf("IsBridge(%s) = %v", name, !want)
		}
	}
}
