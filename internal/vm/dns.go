package vm

import (
	"net"
	"os"
	"strings"
)

// ResolveDNS returns the DNS server to use for VMs.
// If "auto", it reads the first non-loopback nameserver from /etc/resolv.conf.
// Loopback addresses (127.x.x.x, ::1) are skipped because they point to a
// host-local DNS cache that is unreachable from inside the VM.
// Falls back to the gateway IP (the libvirt dnsmasq instance).
func ResolveDNS(dnsConfig, gatewayIP string) string {
	if dnsConfig != "auto" && dnsConfig != "" {
		return dnsConfig
	}

	data, err := os.ReadFile("/etc/resolv.conf")
	if err != nil {
		return gatewayIP
	}

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "nameserver") {
			fields := strings.Fields(line)
			if len(fields) >= 2 && !isLoopback(fields[1]) {
				return fields[1]
			}
		}
	}

	return gatewayIP
}

// isLoopback returns true if the address is a loopback IP (127.0.0.0/8 or ::1).
func isLoopback(address string) bool {
	ip := net.ParseIP(address)
	return ip != nil && ip.IsLoopback()
}
