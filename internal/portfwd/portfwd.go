package portfwd

import (
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"strings"

	"github.com/boyadzhievb/vmherd/internal/ui"
)

const (
	iptablesCommand  = "iptables"
	natTable         = "nat"
	preroutingChain  = "PREROUTING"
	outputChain      = "OUTPUT"
	dnatTarget       = "DNAT"
	localhostAddress = "localhost"
	minPort          = 1
	maxPort          = 65535
	defaultProtocol  = "tcp"

	backendIptables    = "iptables"
	backendIptablesNft = "iptables-nft"
	backendNftables    = "nftables"
	backendNone        = "none"
)

// Rule describes a single port-forwarding mapping from a host port to a guest port.
type Rule struct {
	HostPort  int    // port number on the host
	GuestIP   string // VM IP address to forward to
	GuestPort int    // port number inside the guest
	Protocol  string // "tcp" or "udp"
}

// Setup installs iptables DNAT rules so traffic to host ports reaches guest VMs.
// Both PREROUTING (external traffic) and OUTPUT (localhost traffic) chains are configured.
// Checks for firewall backend compatibility and existing NAT rule conflicts first.
func Setup(rules []Rule) error {
	if len(rules) == 0 {
		return nil
	}

	firewallBackend := detectFirewallBackend()
	switch firewallBackend {
	case backendNftables:
		return fmt.Errorf("this host uses nftables without iptables compatibility — install iptables-nft or configure port forwarding manually using nft")
	case backendNone:
		return fmt.Errorf("iptables not found — install iptables or iptables-nft for port forwarding")
	}

	for _, rule := range rules {
		normalizedRule := normalizeProtocol(rule)

		if err := CheckExistingNATRule(normalizedRule.HostPort, normalizedRule.Protocol); err != nil {
			return err
		}

		destination := net.JoinHostPort(normalizedRule.GuestIP, strconv.Itoa(normalizedRule.GuestPort))

		if err := iptablesAppend(preroutingChain, normalizedRule, destination); err != nil {
			return fmt.Errorf("cannot add PREROUTING rule for port %d→%s:%d: %w — check that iptables is available and you have sudo access",
				normalizedRule.HostPort, normalizedRule.GuestIP, normalizedRule.GuestPort, err)
		}

		if err := iptablesAppendOutput(normalizedRule, destination); err != nil {
			return fmt.Errorf("cannot add OUTPUT rule for port %d→%s:%d: %w",
				normalizedRule.HostPort, normalizedRule.GuestIP, normalizedRule.GuestPort, err)
		}

		ui.Printf("Port forward: %s/%d → %s:%d\n",
			normalizedRule.Protocol, normalizedRule.HostPort,
			normalizedRule.GuestIP, normalizedRule.GuestPort)
	}
	return nil
}

// Teardown removes previously installed iptables DNAT rules.
// Errors are logged but do not stop removal of remaining rules.
func Teardown(rules []Rule) {
	for _, rule := range rules {
		normalizedRule := normalizeProtocol(rule)
		destination := net.JoinHostPort(normalizedRule.GuestIP, strconv.Itoa(normalizedRule.GuestPort))

		if err := iptablesDelete(preroutingChain, normalizedRule, destination); err != nil {
			ui.Printf("Warning: could not remove PREROUTING rule for port %d: %v\n", normalizedRule.HostPort, err)
		}

		if err := iptablesDeleteOutput(normalizedRule, destination); err != nil {
			ui.Printf("Warning: could not remove OUTPUT rule for port %d: %v\n", normalizedRule.HostPort, err)
		}
	}
}

// CheckConflict verifies a host port is not already in use.
// Returns an actionable error if the port is bound.
func CheckConflict(hostPort int, protocol string) error {
	if hostPort < minPort || hostPort > maxPort {
		return fmt.Errorf("port %d is out of range (%d–%d)", hostPort, minPort, maxPort)
	}

	if protocol == "" {
		protocol = defaultProtocol
	}

	address := fmt.Sprintf(":%d", hostPort)

	switch protocol {
	case "tcp":
		listener, err := net.Listen("tcp", address)
		if err != nil {
			return fmt.Errorf("host port %d/%s is already in use — choose a different port or stop the process using it", hostPort, protocol)
		}
		_ = listener.Close()
	case "udp":
		packetConn, err := net.ListenPacket("udp", address)
		if err != nil {
			return fmt.Errorf("host port %d/%s is already in use — choose a different port or stop the process using it", hostPort, protocol)
		}
		_ = packetConn.Close()
	default:
		return fmt.Errorf("unsupported protocol %q — use 'tcp' or 'udp'", protocol)
	}

	return nil
}

// normalizeProtocol returns a copy of the rule with an empty protocol defaulted to "tcp".
func normalizeProtocol(rule Rule) Rule {
	if rule.Protocol == "" {
		rule.Protocol = defaultProtocol
	}
	return rule
}

// iptablesAppend adds a PREROUTING DNAT rule for external traffic.
func iptablesAppend(chain string, rule Rule, destination string) error {
	return runIptables("-t", natTable, "-A", chain,
		"-p", rule.Protocol,
		"--dport", strconv.Itoa(rule.HostPort),
		"-j", dnatTarget,
		"--to-destination", destination)
}

// iptablesAppendOutput adds an OUTPUT DNAT rule for localhost traffic.
func iptablesAppendOutput(rule Rule, destination string) error {
	return runIptables("-t", natTable, "-A", outputChain,
		"-p", rule.Protocol,
		"--dport", strconv.Itoa(rule.HostPort),
		"-d", localhostAddress,
		"-j", dnatTarget,
		"--to-destination", destination)
}

// iptablesDelete removes a PREROUTING DNAT rule.
func iptablesDelete(chain string, rule Rule, destination string) error {
	return runIptables("-t", natTable, "-D", chain,
		"-p", rule.Protocol,
		"--dport", strconv.Itoa(rule.HostPort),
		"-j", dnatTarget,
		"--to-destination", destination)
}

// iptablesDeleteOutput removes an OUTPUT DNAT rule.
func iptablesDeleteOutput(rule Rule, destination string) error {
	return runIptables("-t", natTable, "-D", outputChain,
		"-p", rule.Protocol,
		"--dport", strconv.Itoa(rule.HostPort),
		"-d", localhostAddress,
		"-j", dnatTarget,
		"--to-destination", destination)
}

// CheckBackend verifies that a compatible firewall backend (iptables or iptables-nft)
// is available. Call once before any port forwarding operations to fail early.
func CheckBackend() error {
	backend := detectFirewallBackend()
	switch backend {
	case backendNftables:
		return fmt.Errorf("this host uses nftables without iptables compatibility — install iptables-nft or configure port forwarding manually using nft")
	case backendNone:
		return fmt.Errorf("iptables not found — install iptables or iptables-nft for port forwarding")
	}
	return nil
}

// detectFirewallBackend determines which firewall tool is available on the host.
// Returns backendIptables, backendIptablesNft, backendNftables, or backendNone.
func detectFirewallBackend() string {
	versionOutput, err := exec.Command(iptablesCommand, "--version").CombinedOutput()
	if err == nil {
		versionString := string(versionOutput)
		if strings.Contains(versionString, "nf_tables") {
			return backendIptablesNft
		}
		return backendIptables
	}

	if _, lookupErr := exec.LookPath("nft"); lookupErr == nil {
		return backendNftables
	}

	return backendNone
}

// CheckExistingNATRule checks whether an iptables NAT PREROUTING rule already
// exists for the given host port and protocol. Returns an actionable error if
// a conflicting rule is found. Returns nil if iptables cannot be queried.
func CheckExistingNATRule(hostPort int, protocol string) error {
	if protocol == "" {
		protocol = defaultProtocol
	}

	listOutput, err := exec.Command("sudo", iptablesCommand,
		"-t", natTable, "-L", preroutingChain, "-n").CombinedOutput()
	if err != nil {
		return nil
	}

	if existingNATRuleConflicts(string(listOutput), hostPort, protocol) {
		return fmt.Errorf("host port %d/%s already has a NAT rule — another VM or service (e.g. Docker, libvirt) may be using it. Remove the conflicting rule or choose a different host port",
			hostPort, protocol)
	}

	return nil
}

// existingNATRuleConflicts parses iptables NAT listing output and returns true
// if an existing rule targets the given host port and protocol.
func existingNATRuleConflicts(iptablesOutput string, hostPort int, protocol string) bool {
	portToken := fmt.Sprintf("dpt:%d", hostPort)

	for _, line := range strings.Split(iptablesOutput, "\n") {
		trimmedLine := strings.TrimSpace(line)
		if trimmedLine == "" || strings.HasPrefix(trimmedLine, "Chain") || strings.HasPrefix(trimmedLine, "num") || strings.HasPrefix(trimmedLine, "target") {
			continue
		}

		if !strings.Contains(trimmedLine, portToken) {
			continue
		}

		tokenEndIndex := strings.Index(trimmedLine, portToken) + len(portToken)
		if tokenEndIndex < len(trimmedLine) {
			nextChar := trimmedLine[tokenEndIndex]
			if nextChar >= '0' && nextChar <= '9' {
				continue
			}
		}

		lineFields := strings.Fields(trimmedLine)
		if containsProtocol(lineFields, protocol) {
			return true
		}
	}

	return false
}

// containsProtocol checks whether any field in the iptables output line matches
// the given protocol. Handles both numbered and unnumbered output formats.
func containsProtocol(fields []string, protocol string) bool {
	for _, field := range fields {
		if strings.EqualFold(field, protocol) {
			return true
		}
	}
	return false
}

// runIptables executes an iptables command with sudo and streams output to the UI.
func runIptables(args ...string) error {
	cmd := exec.Command("sudo", append([]string{iptablesCommand}, args...)...)
	cmd.Stdout = ui.Stdout()
	cmd.Stderr = ui.Stderr()
	return cmd.Run()
}
