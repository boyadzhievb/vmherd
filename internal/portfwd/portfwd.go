package portfwd

import (
	"fmt"
	"net"
	"os/exec"
	"strconv"

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
)

// Rule describes a single port-forwarding mapping from a host port to a guest port.
type Rule struct {
	HostPort  int
	GuestIP   string
	GuestPort int
	Protocol  string // "tcp" or "udp"
}

// Setup installs iptables DNAT rules so traffic to host ports reaches guest VMs.
// Both PREROUTING (external traffic) and OUTPUT (localhost traffic) chains are configured.
func Setup(rules []Rule) error {
	for _, rule := range rules {
		normalizedRule := normalizeProtocol(rule)
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

func runIptables(args ...string) error {
	cmd := exec.Command("sudo", append([]string{iptablesCommand}, args...)...)
	cmd.Stdout = ui.Stdout()
	cmd.Stderr = ui.Stderr()
	return cmd.Run()
}
