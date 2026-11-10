package network

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/boyadzhievb/vmherd/internal/libvirt"
	"github.com/boyadzhievb/vmherd/internal/ui"
)

const (
	// freeSubnetPoolFirstOctet is the third octet of the first 192.168.X.0/24 FindFreeSubnet tries.
	freeSubnetPoolFirstOctet = 150
	// freeSubnetPoolLastOctet is the third octet of the last 192.168.X.0/24 FindFreeSubnet tries.
	freeSubnetPoolLastOctet = 249
	// procRouteTablePath is the Linux kernel's IPv4 routing table.
	procRouteTablePath = "/proc/net/route"
	// routeTableDestinationColumn is the Destination column in /proc/net/route.
	routeTableDestinationColumn = 1
	// routeTableMaskColumn is the Mask column in /proc/net/route.
	routeTableMaskColumn = 7
	// domiflistTypeColumn is the Type column in 'virsh domiflist' output.
	domiflistTypeColumn = 1
	// domiflistSourceColumn is the Source column in 'virsh domiflist' output.
	domiflistSourceColumn = 2
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
			return fmt.Errorf("subnet %s overlaps with existing libvirt network %q (%s) — change 'subnet' in your Herdfile and the VMs' ip addresses to match%s",
				ourCIDR, existingName, existingCIDR, freeSubnetHint(existingSubnets))
		}
	}

	return nil
}

// freeSubnetHint suggests a free subnet for the overlap error, or returns ""
// when none can be found.
func freeSubnetHint(libvirtSubnets map[string]string) string {
	hostCIDRs, err := hostNetworkCIDRs()
	if err != nil {
		return ""
	}
	freeSubnetBase, err := firstFreeSubnet(append(hostCIDRs, mapValues(libvirtSubnets)...))
	if err != nil {
		return ""
	}
	return fmt.Sprintf("; %s.0/24 is free", freeSubnetBase)
}

// FindFreeSubnet returns the base (e.g. "192.168.150") of the first /24 in
// the vmherd pool that overlaps no libvirt network, host interface or host
// route. 'vmherd init' writes it into new Herdfiles.
func FindFreeSubnet() (string, error) {
	libvirtSubnets, err := listNetworkSubnets()
	if err != nil {
		return "", fmt.Errorf("finding a free subnet: %w", err)
	}
	hostCIDRs, err := hostNetworkCIDRs()
	if err != nil {
		return "", fmt.Errorf("finding a free subnet: %w", err)
	}
	return firstFreeSubnet(append(hostCIDRs, mapValues(libvirtSubnets)...))
}

// firstFreeSubnet returns the base of the first pool /24 that overlaps none
// of usedCIDRs.
func firstFreeSubnet(usedCIDRs []string) (string, error) {
	for thirdOctet := freeSubnetPoolFirstOctet; thirdOctet <= freeSubnetPoolLastOctet; thirdOctet++ {
		candidateBase := fmt.Sprintf("192.168.%d", thirdOctet)
		if !overlapsAny(candidateBase+".0/24", usedCIDRs) {
			return candidateBase, nil
		}
	}
	return "", fmt.Errorf("no free /24 between 192.168.%d.0 and 192.168.%d.0 — set 'subnet' in your Herdfile by hand",
		freeSubnetPoolFirstOctet, freeSubnetPoolLastOctet)
}

// overlapsAny reports whether candidateCIDR overlaps any of usedCIDRs.
func overlapsAny(candidateCIDR string, usedCIDRs []string) bool {
	for _, usedCIDR := range usedCIDRs {
		if subnetsOverlap(candidateCIDR, usedCIDR) {
			return true
		}
	}
	return false
}

// hostNetworkCIDRs returns the IPv4 networks of the host's interfaces and
// routes, so a free subnet doesn't clash with the LAN, a VPN or a container
// bridge. The default route is skipped because it overlaps everything.
func hostNetworkCIDRs() ([]string, error) {
	interfaceAddresses, err := net.InterfaceAddrs()
	if err != nil {
		return nil, fmt.Errorf("listing host interface addresses: %w", err)
	}
	var hostCIDRs []string
	for _, interfaceAddress := range interfaceAddresses {
		if ipNetwork, isIPNetwork := interfaceAddress.(*net.IPNet); isIPNetwork && ipNetwork.IP.To4() != nil {
			hostCIDRs = append(hostCIDRs, ipNetwork.String())
		}
	}

	routeTable, err := os.ReadFile(procRouteTablePath)
	if errors.Is(err, os.ErrNotExist) {
		return hostCIDRs, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", procRouteTablePath, err)
	}
	return append(hostCIDRs, parseRouteTableCIDRs(string(routeTable))...), nil
}

// parseRouteTableCIDRs extracts destination CIDRs from /proc/net/route, whose
// Destination and Mask columns are little-endian hex. Default routes are skipped.
func parseRouteTableCIDRs(routeTable string) []string {
	var routeCIDRs []string
	for _, line := range strings.Split(routeTable, "\n")[1:] {
		fields := strings.Fields(line)
		if len(fields) <= routeTableMaskColumn {
			continue
		}
		destination, destinationErr := parseLittleEndianIPv4(fields[routeTableDestinationColumn])
		mask, maskErr := parseLittleEndianIPv4(fields[routeTableMaskColumn])
		if destinationErr != nil || maskErr != nil {
			continue
		}
		prefixLength, _ := net.IPMask(mask).Size()
		if prefixLength == 0 {
			continue
		}
		routeCIDRs = append(routeCIDRs, fmt.Sprintf("%s/%d", destination, prefixLength))
	}
	return routeCIDRs
}

// parseLittleEndianIPv4 decodes an 8-digit little-endian hex IPv4 address
// as written in /proc/net/route.
func parseLittleEndianIPv4(hexAddress string) (net.IP, error) {
	addressBytes, err := hex.DecodeString(hexAddress)
	if err != nil || len(addressBytes) != net.IPv4len {
		return nil, fmt.Errorf("invalid route table address %q", hexAddress)
	}
	return net.IPv4(addressBytes[3], addressBytes[2], addressBytes[1], addressBytes[0]).To4(), nil
}

// mapValues returns the values of a string map, in no particular order.
func mapValues(stringMap map[string]string) []string {
	values := make([]string, 0, len(stringMap))
	for _, value := range stringMap {
		values = append(values, value)
	}
	return values
}

// AttachedDomains returns the sorted names of libvirt domains (running or not)
// with an interface on networkName.
func AttachedDomains(networkName string) ([]string, error) {
	domainList, err := exec.Command("sudo", "virsh", "list", "--all", "--name").Output()
	if err != nil {
		return nil, fmt.Errorf("listing libvirt domains: %w — check that libvirtd is running", err)
	}
	var attachedDomains []string
	for _, line := range strings.Split(string(domainList), "\n") {
		domainName := strings.TrimSpace(line)
		if domainName == "" {
			continue
		}
		interfaceList, err := exec.Command("sudo", "virsh", "domiflist", domainName).Output()
		if err != nil {
			return nil, fmt.Errorf("listing interfaces of domain %s: %w", domainName, err)
		}
		if domainUsesNetwork(string(interfaceList), networkName) {
			attachedDomains = append(attachedDomains, domainName)
		}
	}
	sort.Strings(attachedDomains)
	return attachedDomains, nil
}

// domainUsesNetwork reports whether 'virsh domiflist' output has an interface
// of type network whose source is networkName.
func domainUsesNetwork(domiflistOutput, networkName string) bool {
	for _, line := range strings.Split(domiflistOutput, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= domiflistSourceColumn+1 && fields[domiflistTypeColumn] == "network" && fields[domiflistSourceColumn] == networkName {
			return true
		}
	}
	return false
}

// Ensure creates the libvirt network if it doesn't exist, starts it, and enables autostart.
func Ensure(stateDir, networkName, subnetBase string) error {
	if exists(networkName) {
		if !isActive(networkName) {
			if err := libvirt.Virsh("net-start", networkName); err != nil {
				return fmt.Errorf("cannot start network %s: %w", networkName, err)
			}
		}
		if err := libvirt.Virsh("net-autostart", networkName); err != nil {
			ui.Printf("Warning: could not set autostart for network %s: %v\n", networkName, err)
		}
		return nil
	}

	if err := checkSubnetConflict(networkName, subnetBase); err != nil {
		return err
	}

	bridgeName := bridgeNameForNetwork(networkName)

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
	if err := os.WriteFile(xmlPath, []byte(netXML), 0o600); err != nil {
		return fmt.Errorf("cannot write network XML: %w", err)
	}

	if err := libvirt.Virsh("net-define", xmlPath); err != nil {
		if !exists(networkName) {
			return fmt.Errorf("cannot define network %s: %w — is libvirtd running?", networkName, err)
		}
	}
	if err := libvirt.Virsh("net-start", networkName); err != nil {
		if !isActive(networkName) {
			return fmt.Errorf("cannot start network %s: %w", networkName, err)
		}
	}
	if err := libvirt.Virsh("net-autostart", networkName); err != nil {
		ui.Printf("Warning: could not set autostart for network %s: %v\n", networkName, err)
	}

	ui.Printf("Network %s created.\n", networkName)
	return nil
}

// sysClassNetPath is where the Linux kernel exposes network interfaces.
const sysClassNetPath = "/sys/class/net"

// EnsureBridgeExists verifies that bridgeName is an existing bridge device on
// this host. vmherd never creates or removes host bridges; it only attaches
// VMs to one that is already configured.
func EnsureBridgeExists(bridgeName string) error {
	if _, err := os.Stat(filepath.Join(sysClassNetPath, bridgeName)); err != nil {
		return fmt.Errorf("host bridge %q does not exist — create it first (e.g. with nmcli or systemd-networkd) or fix 'network.bridge' in your Herdfile", bridgeName)
	}
	if _, err := os.Stat(filepath.Join(sysClassNetPath, bridgeName, "bridge")); err != nil {
		return fmt.Errorf("host interface %q exists but is not a bridge — point 'network.bridge' at a bridge device such as br0", bridgeName)
	}
	return nil
}

// Destroy removes a libvirt network.
func Destroy(networkName string) error {
	if !exists(networkName) {
		return nil
	}
	if err := libvirt.Virsh("net-destroy", networkName); err != nil {
		ui.Printf("Warning: could not stop network %s: %v\n", networkName, err)
	}
	if err := libvirt.Virsh("net-undefine", networkName); err != nil {
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

const maxBridgeNameLength = 15

// bridgeNameHashLength is how many hex characters of the network-name hash end
// a shortened bridge name.
const bridgeNameHashLength = 8

// bridgeNameForNetwork returns the Linux bridge name for a new libvirt network.
// Names that fit the 15-char limit are used as is. Longer names become their
// first characters plus a hash of the full name, so two long names that share
// a prefix (vmherd-web-1a2b… and vmherd-web-9f8e…) still get different bridges.
func bridgeNameForNetwork(networkName string) string {
	if len(networkName) <= maxBridgeNameLength {
		return networkName
	}
	nameHash := sha256.Sum256([]byte(networkName))
	hashSuffix := hex.EncodeToString(nameHash[:])[:bridgeNameHashLength]
	prefixLength := maxBridgeNameLength - bridgeNameHashLength - 1
	return networkName[:prefixLength] + "-" + hashSuffix
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
// already claimed by another libvirt network, for example one created by an
// older vmherd that truncated long names to 15 characters.
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
				"both networks would use the same 15-char bridge. "+
				"Rename your network in the Herdfile (network.name) to avoid this collision",
				bridgeName, existingNetworkName)
		}
	}

	return nil
}
