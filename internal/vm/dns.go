package vm

import (
	"os"
	"strings"
)

// ResolveDNS returns the DNS server to use for VMs.
// If "auto", it reads the first nameserver from /etc/resolv.conf.
// Falls back to the gateway IP if resolv.conf can't be parsed.
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
			if len(fields) >= 2 {
				return fields[1]
			}
		}
	}

	return gatewayIP
}
