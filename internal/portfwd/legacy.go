package portfwd

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/boyadzhievb/vmherd/internal/ssh"
	"github.com/boyadzhievb/vmherd/internal/ui"
)

// legacyIptablesSearchPaths are where iptables lives on hosts where it isn't in
// the user's PATH (Debian keeps it in /usr/sbin).
var legacyIptablesSearchPaths = []string{"/usr/sbin/iptables", "/sbin/iptables"}

// RemoveLegacyRules deletes the iptables DNAT rules vmherd v0.1.x added for
// these forwards to guestIP. Left in place they catch the traffic before it
// reaches the SSH tunnel. Only rules exactly in the v0.1.x format are touched;
// a host without iptables can't have any.
func RemoveLegacyRules(guestIP string, forwards []ssh.LocalForward) error {
	if guestIP == "" || len(forwards) == 0 || !iptablesInstalled() {
		return nil
	}
	ruleListing, err := exec.Command("sudo", "iptables", "-t", "nat", "-S").Output()
	if err != nil {
		return fmt.Errorf("listing NAT rules to remove old vmherd v0.1.x port forwards: %w", err)
	}

	for _, ruleSpec := range matchingLegacyRules(string(ruleListing), guestIP, forwards) {
		deleteArgs := append([]string{"iptables", "-t", "nat", "-D"}, strings.Fields(strings.TrimPrefix(ruleSpec, "-A "))...)
		if output, err := exec.Command("sudo", deleteArgs...).CombinedOutput(); err != nil {
			return fmt.Errorf("removing old vmherd v0.1.x rule %q: %w: %s — remove it with 'sudo iptables -t nat %s'",
				ruleSpec, err, strings.TrimSpace(string(output)), strings.Replace(ruleSpec, "-A ", "-D ", 1))
		}
		ui.Printf("Removed old vmherd v0.1.x port-forward rule: %s\n", ruleSpec)
	}
	return nil
}

// matchingLegacyRules returns the 'iptables -S' lines that are exactly the
// PREROUTING and OUTPUT DNAT rules v0.1.x created for forwards to guestIP.
// Duplicates are kept, since v0.1.x could add the OUTPUT rule twice.
func matchingLegacyRules(ruleListing, guestIP string, forwards []ssh.LocalForward) []string {
	legacyRules := make(map[string]bool)
	for _, forward := range forwards {
		destination := guestIP + ":" + strconv.Itoa(forward.GuestPort)
		hostPort := strconv.Itoa(forward.HostPort)
		legacyRules["-A PREROUTING -p tcp -m tcp --dport "+hostPort+" -j DNAT --to-destination "+destination] = true
		legacyRules["-A OUTPUT -d 127.0.0.1/32 -p tcp -m tcp --dport "+hostPort+" -j DNAT --to-destination "+destination] = true
	}

	var matchingRules []string
	for _, line := range strings.Split(ruleListing, "\n") {
		if ruleSpec := strings.TrimSpace(line); legacyRules[ruleSpec] {
			matchingRules = append(matchingRules, ruleSpec)
		}
	}
	return matchingRules
}

// iptablesInstalled reports whether the iptables command exists on the host.
func iptablesInstalled() bool {
	if _, err := exec.LookPath("iptables"); err == nil {
		return true
	}
	for _, candidatePath := range legacyIptablesSearchPaths {
		if _, err := exec.LookPath(candidatePath); err == nil {
			return true
		}
	}
	return false
}
