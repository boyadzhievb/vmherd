package network

import (
	"encoding/xml"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/boyadzhievb/vmherd/internal/ui"
)

// networkXML represents the top-level <network> element from virsh net-dumpxml.
type networkXML struct {
	XMLName xml.Name       `xml:"network"`
	Name    string         `xml:"name"`
	IPs     []networkIPXML `xml:"ip"`
}

// networkIPXML represents an <ip> element inside a libvirt network definition.
type networkIPXML struct {
	Address string `xml:"address,attr"`
	Netmask string `xml:"netmask,attr"`
	Prefix  string `xml:"prefix,attr"`
}

// listNetworkSubnets returns a map of libvirt network name → CIDR string
// for every defined network on the host. Skips networks whose XML cannot be parsed.
func listNetworkSubnets() (map[string]string, error) {
	out, err := exec.Command("sudo", "virsh", "net-list", "--all", "--name").Output()
	if err != nil {
		return nil, fmt.Errorf("cannot list libvirt networks: %w", err)
	}

	subnetsByName := make(map[string]string)
	for _, line := range strings.Split(string(out), "\n") {
		networkName := strings.TrimSpace(line)
		if networkName == "" {
			continue
		}

		xmlOut, err := exec.Command("sudo", "virsh", "net-dumpxml", networkName).Output()
		if err != nil {
			continue
		}

		var netDef networkXML
		if err := xml.Unmarshal(xmlOut, &netDef); err != nil {
			continue
		}

		if len(netDef.IPs) == 0 {
			continue
		}

		cidr := ipAndMaskToCIDR(netDef.IPs[0])
		if cidr != "" {
			subnetsByName[networkName] = cidr
		}
	}

	return subnetsByName, nil
}

// ipAndMaskToCIDR converts an <ip> element's address and netmask (or prefix)
// into a network CIDR string like "192.168.122.0/24".
func ipAndMaskToCIDR(ipElem networkIPXML) string {
	parsedIP := net.ParseIP(ipElem.Address)
	if parsedIP == nil {
		return ""
	}

	var ipMask net.IPMask
	if ipElem.Netmask != "" {
		maskIP := net.ParseIP(ipElem.Netmask)
		if maskIP == nil {
			return ""
		}
		ipMask = net.IPMask(maskIP.To4())
	} else if ipElem.Prefix != "" {
		var prefixLen int
		if _, err := fmt.Sscanf(ipElem.Prefix, "%d", &prefixLen); err != nil {
			return ""
		}
		ipMask = net.CIDRMask(prefixLen, 32)
	} else {
		return ""
	}

	networkAddress := parsedIP.Mask(ipMask)
	prefixSize, _ := ipMask.Size()
	return fmt.Sprintf("%s/%d", networkAddress, prefixSize)
}

// subnetsOverlap returns true if two CIDR ranges share any addresses.
func subnetsOverlap(cidrA, cidrB string) bool {
	_, netA, errA := net.ParseCIDR(cidrA)
	_, netB, errB := net.ParseCIDR(cidrB)
	if errA != nil || errB != nil {
		return false
	}

	return netA.Contains(netB.IP) || netB.Contains(netA.IP)
}

// checkSubnetConflict verifies that the requested subnet does not overlap with
// any existing libvirt network (other than our own). Returns an actionable error
// if a conflict is found.
func checkSubnetConflict(networkName, subnetBase string) error {
	existingSubnets, err := listNetworkSubnets()
	if err != nil {
		return err
	}

	ourCIDR := subnetBase + ".0/24"

	for existingName, existingCIDR := range existingSubnets {
		if existingName == networkName {
			continue
		}
		if subnetsOverlap(ourCIDR, existingCIDR) {
			return fmt.Errorf("subnet %s overlaps with existing libvirt network %q (%s) — change 'subnet' in your Herdfile to avoid conflicts",
				ourCIDR, existingName, existingCIDR)
		}
	}

	return nil
}

// Ensure creates the libvirt network if it doesn't exist, starts it, and enables autostart.
func Ensure(stateDir, networkName, subnetBase string) error {
	if exists(networkName) {
		if !isActive(networkName) {
			if err := virsh("net-start", networkName); err != nil {
				return fmt.Errorf("cannot start network %s: %w", networkName, err)
			}
		}
		_ = virsh("net-autostart", networkName)
		return nil
	}

	if err := checkSubnetConflict(networkName, subnetBase); err != nil {
		return err
	}

	bridgeName := truncateBridgeName(networkName)

	if err := checkBridgeNameConflict(networkName, bridgeName); err != nil {
		return err
	}

	ui.Printf("Creating libvirt network: %s (%s.0/24)\n", networkName, subnetBase)

	netXML := fmt.Sprintf(`<network>
  <name>%s</name>
  <forward mode='nat'/>
  <bridge name='%s' stp='on' delay='0'/>
  <ip address='%s.1' netmask='255.255.255.0'>
    <dhcp>
      <range start='%s.100' end='%s.200'/>
    </dhcp>
  </ip>
</network>`, networkName, bridgeName, subnetBase, subnetBase, subnetBase)

	xmlDir := filepath.Join(stateDir, "network")
	if err := os.MkdirAll(xmlDir, 0o755); err != nil {
		return fmt.Errorf("cannot create state directory: %w", err)
	}

	xmlPath := filepath.Join(xmlDir, networkName+".xml")
	if err := os.WriteFile(xmlPath, []byte(netXML), 0o644); err != nil {
		return fmt.Errorf("cannot write network XML: %w", err)
	}

	if err := virsh("net-define", xmlPath); err != nil {
		if !exists(networkName) {
			return fmt.Errorf("cannot define network %s: %w — is libvirtd running?", networkName, err)
		}
	}
	if err := virsh("net-start", networkName); err != nil {
		if !isActive(networkName) {
			return fmt.Errorf("cannot start network %s: %w", networkName, err)
		}
	}
	_ = virsh("net-autostart", networkName)

	ui.Printf("Network %s created.\n", networkName)
	return nil
}

// Destroy removes a libvirt network.
func Destroy(networkName string) error {
	if !exists(networkName) {
		return nil
	}
	_ = virsh("net-destroy", networkName)
	if err := virsh("net-undefine", networkName); err != nil {
		return fmt.Errorf("cannot remove network %s: %w", networkName, err)
	}
	ui.Printf("Network %s removed.\n", networkName)
	return nil
}

// exists checks whether a libvirt network with the given name is defined.
func exists(networkName string) bool {
	out, err := exec.Command("sudo", "virsh", "net-list", "--all", "--name").Output()
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.TrimSpace(line) == networkName {
			return true
		}
	}
	return false
}

// isActive checks whether a libvirt network is currently started.
func isActive(networkName string) bool {
	out, err := exec.Command("sudo", "virsh", "net-list", "--name").Output()
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.TrimSpace(line) == networkName {
			return true
		}
	}
	return false
}

// virsh runs a virsh subcommand with sudo and streams output to the UI.
func virsh(args ...string) error {
	cmd := exec.Command("sudo", append([]string{"virsh"}, args...)...)
	cmd.Stdout = ui.Stdout()
	cmd.Stderr = ui.Stderr()
	return cmd.Run()
}

const maxBridgeNameLength = 15

// truncateBridgeName ensures the bridge name fits within the Linux 15-char limit.
func truncateBridgeName(networkName string) string {
	if len(networkName) <= maxBridgeNameLength {
		return networkName
	}
	return networkName[:maxBridgeNameLength]
}

// bridgeXML represents the <bridge> element inside a libvirt network definition.
type bridgeXML struct {
	Name string `xml:"name,attr"`
}

// networkBridgeXML is a minimal parse of virsh net-dumpxml for bridge name extraction.
type networkBridgeXML struct {
	XMLName xml.Name  `xml:"network"`
	Bridge  bridgeXML `xml:"bridge"`
}

// checkBridgeNameConflict verifies that the bridge name we plan to use is not
// already claimed by another libvirt network. Two long network names that share
// the same first 15 characters would collide after truncation.
func checkBridgeNameConflict(networkName, bridgeName string) error {
	out, err := exec.Command("sudo", "virsh", "net-list", "--all", "--name").Output()
	if err != nil {
		return nil
	}

	for _, line := range strings.Split(string(out), "\n") {
		existingNetworkName := strings.TrimSpace(line)
		if existingNetworkName == "" || existingNetworkName == networkName {
			continue
		}

		xmlOut, err := exec.Command("sudo", "virsh", "net-dumpxml", existingNetworkName).Output()
		if err != nil {
			continue
		}

		var netDef networkBridgeXML
		if err := xml.Unmarshal(xmlOut, &netDef); err != nil {
			continue
		}

		if netDef.Bridge.Name == bridgeName {
			return fmt.Errorf("bridge name %q conflicts with existing libvirt network %q — "+
				"both network names truncate to the same 15-char bridge. "+
				"Rename your network in the Herdfile (network.name) to avoid this collision",
				bridgeName, existingNetworkName)
		}
	}

	return nil
}
