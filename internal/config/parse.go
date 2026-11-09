package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	HerdfileName = "Herdfile"

	minPort = 1
	maxPort = 65535

	defaultPortProtocol      = "tcp"
	defaultPortHostIP        = "127.0.0.1" // forwarded ports are local-only unless host_ip says otherwise
	defaultSharedFolderHost  = "."
	defaultSharedFolderGuest = "/vagrant"

	// maxResourceNameLength is the maximum length for VM names.
	maxResourceNameLength = 63
	// maxNetworkNameLength is the maximum length for network names.
	maxNetworkNameLength = 253
	// maxSSHUserLength is the maximum length for SSH usernames (Linux limit).
	maxSSHUserLength = 32

	// dhcpPoolStart is the first last-octet in the DHCP range (.100-.200).
	dhcpPoolStart = 100
	// dhcpPoolEnd is the last last-octet in the DHCP range (.100-.200).
	dhcpPoolEnd = 200
)

// resourceNamePattern matches valid VM and network names: alphanumeric with hyphens,
// starting and ending with an alphanumeric character.
var resourceNamePattern = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?$`)

// diskSizePattern matches valid disk size specifications: digits followed by a unit suffix.
var diskSizePattern = regexp.MustCompile(`^[0-9]+[KMGT]$`)

// sshUserPattern matches valid Linux usernames: starts with lowercase letter or underscore,
// followed by lowercase letters, digits, hyphens, or underscores.
var sshUserPattern = regexp.MustCompile(`^[a-z_][a-z0-9_-]*$`)

// Load reads and parses a Herdfile from the given directory.
func Load(dir string) (*Herdfile, error) {
	path := filepath.Join(dir, HerdfileName)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w — run 'vmherd init' to create one", HerdfileName, err)
	}

	herdfile, err := decodeHerdfile(data)
	if err != nil {
		return nil, err
	}

	applyDefaults(herdfile, dir)

	if err := validate(herdfile, dir); err != nil {
		return nil, err
	}

	return herdfile, nil
}

// decodeHerdfile parses Herdfile YAML strictly: unknown keys and wrongly typed
// values are all reported with their line numbers. Semantic validation is
// skipped when decoding fails, since half-decoded zero values would only add
// misleading problems.
func decodeHerdfile(data []byte) (*Herdfile, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)

	var herdfile Herdfile
	if err := decoder.Decode(&herdfile); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("%s is empty — define at least one VM under 'vms:' (run 'vmherd init' in an empty directory for an example)", HerdfileName)
		}
		return nil, herdfileProblemsError(describeYAMLDecodeError(err))
	}
	return &herdfile, nil
}

// herdfileProblemsError combines Herdfile problems into one error: a single
// problem as is, several as a counted list with one problem per line.
func herdfileProblemsError(problems []error) error {
	switch len(problems) {
	case 0:
		return nil
	case 1:
		return fmt.Errorf("%s: %w", HerdfileName, problems[0])
	}
	listedProblems := make([]error, len(problems))
	for index, problem := range problems {
		listedProblems[index] = fmt.Errorf("  - %w", problem)
	}
	return fmt.Errorf("%s has %d problems:\n%w", HerdfileName, len(problems), errors.Join(listedProblems...))
}

// applyDefaults fills in zero-value fields with sensible defaults.
func applyDefaults(herdfile *Herdfile, dir string) {
	if herdfile.SSH.User == "" {
		herdfile.SSH.User = "vmherd"
	}
	if herdfile.Network.Name == "" {
		herdfile.Network.Name = "vmherd-" + filepath.Base(dir)
	}
	if herdfile.Network.Subnet == "" {
		herdfile.Network.Subnet = "192.168.122.0/24"
	}
	if herdfile.Network.DNS == "" {
		herdfile.Network.DNS = "auto"
	}
	for i := range herdfile.VMs {
		if herdfile.VMs[i].CPUs == 0 {
			herdfile.VMs[i].CPUs = 1
		}
		if herdfile.VMs[i].Memory == 0 {
			herdfile.VMs[i].Memory = 1024
		}
		if herdfile.VMs[i].Disk == "" {
			herdfile.VMs[i].Disk = "20G"
		}
		// Omitted (nil) gets the project share; an explicit 'shared_folders: []'
		// is non-nil and means "no shares".
		if herdfile.VMs[i].SharedFolders == nil {
			herdfile.VMs[i].SharedFolders = []SharedFolder{
				{Host: defaultSharedFolderHost, Guest: defaultSharedFolderGuest},
			}
		}
		for j := range herdfile.VMs[i].Ports {
			if herdfile.VMs[i].Ports[j].Protocol == "" {
				herdfile.VMs[i].Ports[j].Protocol = defaultPortProtocol
			}
			if herdfile.VMs[i].Ports[j].HostIP == "" {
				herdfile.VMs[i].Ports[j].HostIP = defaultPortHostIP
			}
		}
	}
}

// validate checks the Herdfile for semantic errors after defaults are applied.
// Every check runs; all problems are reported together.
func validate(herdfile *Herdfile, projectDir string) error {
	var problems []error
	if len(herdfile.VMs) == 0 {
		problems = append(problems, fmt.Errorf("define at least one VM under 'vms:'"))
	}

	// A bad subnet is reported once here; the VMs' in-subnet checks then skip.
	subnetCIDR, err := parseSubnet(herdfile.Network.Subnet)
	if err != nil {
		problems = append(problems, err)
	}

	usedNames := make(map[string]bool)
	usedHostPorts := make(map[string]string)
	usedIPs := make(map[string]string) // ip → vm name
	for vmIndex, vmConfig := range herdfile.VMs {
		problems = append(problems, validateVMIdentity(&vmConfig, vmIndex, usedNames)...)
		if err := validateDiskSize(vmConfig.Disk, vmConfig.Name); err != nil {
			problems = append(problems, err)
		}
		problems = append(problems, validateVMIPAddress(vmConfig, subnetCIDR, usedIPs)...)
		// The Herdfile's VM shares SharedFolders' backing array with vmConfig,
		// so the resolved host paths reach the Herdfile.
		problems = append(problems, validateSharedFolders(vmConfig, projectDir)...)
		problems = append(problems, validatePortForwards(vmConfig, usedHostPorts)...)
		problems = append(problems, validateProvisionSteps(vmConfig, projectDir)...)
	}

	if err := validateNetworkName(herdfile.Network.Name); err != nil {
		problems = append(problems, err)
	}
	if err := validateSSHUser(herdfile.SSH.User); err != nil {
		problems = append(problems, err)
	}
	return herdfileProblemsError(problems)
}

// validateVMIdentity checks a VM's name (present, valid, unique) and image.
// A VM without a name gets "#<position>" as its name in this and later
// messages, so its other problems can still be reported.
func validateVMIdentity(vmConfig *VM, vmIndex int, usedNames map[string]bool) []error {
	var problems []error
	switch {
	case vmConfig.Name == "":
		vmConfig.Name = fmt.Sprintf("#%d", vmIndex+1)
		problems = append(problems, fmt.Errorf("VM %s under 'vms:' has no 'name'", vmConfig.Name))
	case !isValidResourceName(vmConfig.Name, maxResourceNameLength):
		problems = append(problems, fmt.Errorf("VM name %q is invalid — use only letters, digits, and hyphens (max 63 chars, must start and end with alphanumeric)", vmConfig.Name))
	case usedNames[vmConfig.Name]:
		problems = append(problems, fmt.Errorf("duplicate VM name: %q", vmConfig.Name))
	}
	usedNames[vmConfig.Name] = true

	if vmConfig.Image == "" {
		return append(problems, fmt.Errorf("VM %q must have an 'image' (e.g. ubuntu:noble)", vmConfig.Name))
	}
	distro, codename, found := strings.Cut(vmConfig.Image, ":")
	if !found || distro == "" || codename == "" {
		problems = append(problems, fmt.Errorf("VM %q: image must be 'distro:codename' format (e.g. ubuntu:noble), got %q", vmConfig.Name, vmConfig.Image))
	}
	return problems
}

// parseSubnet parses the network subnet and ensures it uses a /24 prefix.
// The entire codebase (network XML, cloud-init, DHCP range) assumes /24;
// broader subnet support is planned but not yet implemented.
func parseSubnet(subnet string) (*net.IPNet, error) {
	_, subnetCIDR, err := net.ParseCIDR(subnet)
	if err != nil {
		return nil, fmt.Errorf("network subnet %q is not valid CIDR: %w", subnet, err)
	}
	ones, _ := subnetCIDR.Mask.Size()
	if ones != 24 {
		return nil, fmt.Errorf("only /24 subnets are supported, got /%d in %q — broader subnet support is planned", ones, subnet)
	}
	return subnetCIDR, nil
}

// isValidResourceName checks that a name contains only alphanumerics and hyphens,
// starts and ends with an alphanumeric character, and does not exceed maxLength.
func isValidResourceName(name string, maxLength int) bool {
	if len(name) > maxLength {
		return false
	}
	return resourceNamePattern.MatchString(name)
}

// validateNetworkName checks the network name for valid characters and length.
func validateNetworkName(networkName string) error {
	if !isValidResourceName(networkName, maxNetworkNameLength) {
		return fmt.Errorf("network name %q is invalid — use only letters, digits, and hyphens (max 253 chars, must start and end with alphanumeric)", networkName)
	}
	return nil
}

// validateSSHUser checks that the SSH username is a valid Linux username.
func validateSSHUser(user string) error {
	if len(user) > maxSSHUserLength {
		return fmt.Errorf("ssh.user %q exceeds maximum length of %d characters", user, maxSSHUserLength)
	}
	if !sshUserPattern.MatchString(user) {
		return fmt.Errorf("ssh.user %q is invalid — must start with lowercase letter or underscore, contain only lowercase letters, digits, hyphens, and underscores", user)
	}
	return nil
}

// validateDiskSize checks that the disk field uses a valid size format like "20G" or "100M".
func validateDiskSize(diskSize string, vmName string) error {
	if !diskSizePattern.MatchString(diskSize) {
		return fmt.Errorf("VM %q: disk size %q is invalid — use a number followed by K, M, G, or T (e.g. '20G')", vmName, diskSize)
	}
	return nil
}

// validateVMIPAddress validates a VM's IP: correct format, no duplicates,
// within subnetCIDR (skipped when the subnet itself is invalid and nil). No
// ip means DHCP, which must not have a gateway override.
func validateVMIPAddress(vmConfig VM, subnetCIDR *net.IPNet, usedIPs map[string]string) []error {
	if vmConfig.Network.IP == "" {
		if vmConfig.Network.Gateway != "" {
			return []error{fmt.Errorf("VM %q: network.gateway is set but network.ip is not — without an ip the VM uses DHCP, which provides the gateway; add an ip or remove the gateway",
				vmConfig.Name)}
		}
		return nil
	}

	parsedIP := net.ParseIP(vmConfig.Network.IP)
	if parsedIP == nil || parsedIP.To4() == nil {
		return []error{fmt.Errorf("VM %q: IP address %q is not a valid IPv4 address", vmConfig.Name, vmConfig.Network.IP)}
	}

	var problems []error
	if existingVM, taken := usedIPs[vmConfig.Network.IP]; taken {
		problems = append(problems, fmt.Errorf("VM %q: IP address %s is already used by VM %q", vmConfig.Name, vmConfig.Network.IP, existingVM))
	} else {
		usedIPs[vmConfig.Network.IP] = vmConfig.Name
	}
	if subnetCIDR != nil && !subnetCIDR.Contains(parsedIP) {
		problems = append(problems, fmt.Errorf("VM %q: IP address %s is outside the network subnet %s", vmConfig.Name, vmConfig.Network.IP, subnetCIDR))
	}
	lastOctet := int(parsedIP.To4()[3])
	if lastOctet >= dhcpPoolStart && lastOctet <= dhcpPoolEnd {
		problems = append(problems, fmt.Errorf("VM %q: IP address %s is in the DHCP pool range (.%d-.%d) — use an address outside this range to avoid collisions with DHCP leases",
			vmConfig.Name, vmConfig.Network.IP, dhcpPoolStart, dhcpPoolEnd))
	}
	return problems
}

// validateSharedFolders checks every shared folder for empty paths, an
// absolute guest path and the host-path policy, and records each resolved
// host path in ResolvedHost (the slice shares its backing array with the Herdfile).
func validateSharedFolders(vmConfig VM, projectDir string) []error {
	var problems []error
	for folderIndex, sharedFolder := range vmConfig.SharedFolders {
		if err := validateSharedFolderGuest(vmConfig.Name, sharedFolder.Guest); err != nil {
			problems = append(problems, err)
		}
		if sharedFolder.Host == "" {
			problems = append(problems, fmt.Errorf("VM %q: shared_folders entry has empty 'host' path", vmConfig.Name))
			continue
		}
		resolvedHostPath, err := resolveSharedFolderHostPath(sharedFolder.Host, projectDir, sharedFolder.AllowOutsideHome)
		if err != nil {
			problems = append(problems, fmt.Errorf("VM %q: shared_folders host path %q %w", vmConfig.Name, sharedFolder.Host, err))
			continue
		}
		vmConfig.SharedFolders[folderIndex].ResolvedHost = resolvedHostPath
	}
	return problems
}

// validateSharedFolderGuest checks that a share's guest path is set and absolute.
func validateSharedFolderGuest(vmName, guestPath string) error {
	if guestPath == "" {
		return fmt.Errorf("VM %q: shared_folders entry has empty 'guest' path", vmName)
	}
	if !strings.HasPrefix(guestPath, "/") {
		return fmt.Errorf("VM %q: shared_folders guest path %q must be absolute (start with /)", vmName, guestPath)
	}
	return nil
}

// validateProvisionSteps checks that every file-type provision source is
// relative and does not escape the project directory.
func validateProvisionSteps(vmConfig VM, projectDir string) []error {
	var problems []error
	for _, step := range vmConfig.Provision {
		if step.Type != ProvisionFile || step.Source == "" {
			continue
		}
		if filepath.IsAbs(step.Source) {
			problems = append(problems, fmt.Errorf("VM %q: provision source %q must be a relative path — use a path relative to your project directory", vmConfig.Name, step.Source))
			continue
		}
		cleanPath := filepath.Clean(filepath.Join(projectDir, step.Source))
		if !strings.HasPrefix(cleanPath, projectDir) {
			problems = append(problems, fmt.Errorf("VM %q: provision source %q escapes project directory — provision files must be inside the project", vmConfig.Name, step.Source))
		}
	}
	return problems
}

// validatePortForwards checks every port forward's ranges, protocol and host
// address, and host port uniqueness across VMs.
func validatePortForwards(vmConfig VM, usedHostPorts map[string]string) []error {
	var problems []error
	for _, portForward := range vmConfig.Ports {
		problems = append(problems, validatePortForward(vmConfig.Name, portForward)...)

		// Keyed by port alone: 0.0.0.0:8080 and 127.0.0.1:8080 can't both be bound.
		portKey := fmt.Sprintf("%d/%s", portForward.Host, portForward.Protocol)
		if existingVM, taken := usedHostPorts[portKey]; taken {
			problems = append(problems, fmt.Errorf("VM %q: host port %s is already forwarded by VM %q", vmConfig.Name, portKey, existingVM))
			continue
		}
		usedHostPorts[portKey] = vmConfig.Name
	}
	return problems
}

// validatePortForward checks one port forward's ports, protocol and host_ip.
func validatePortForward(vmName string, portForward PortForward) []error {
	var problems []error
	if portForward.Host < minPort || portForward.Host > maxPort {
		problems = append(problems, fmt.Errorf("VM %q: host port %d is out of range (%d-%d)", vmName, portForward.Host, minPort, maxPort))
	}
	if portForward.Guest < minPort || portForward.Guest > maxPort {
		problems = append(problems, fmt.Errorf("VM %q: guest port %d is out of range (%d-%d)", vmName, portForward.Guest, minPort, maxPort))
	}
	if portForward.Protocol != defaultPortProtocol {
		problems = append(problems, fmt.Errorf("VM %q: port %d uses protocol %q, but only 'tcp' is supported — vmherd forwards ports over SSH, which carries TCP only",
			vmName, portForward.Host, portForward.Protocol))
	}
	if net.ParseIP(portForward.HostIP) == nil {
		problems = append(problems, fmt.Errorf("VM %q: port %d has host_ip %q, which is not an IP address — use 127.0.0.1 (this machine only), 0.0.0.0 (all interfaces) or one of the host's addresses",
			vmName, portForward.Host, portForward.HostIP))
	}
	return problems
}

// FilterVMs returns only the VMs matching the given names.
// Returns all VMs if names is empty.
func FilterVMs(herdfile *Herdfile, names []string) ([]VM, error) {
	if len(names) == 0 {
		return herdfile.VMs, nil
	}

	known := make(map[string]VM)
	for _, vm := range herdfile.VMs {
		known[vm.Name] = vm
	}

	var result []VM
	for _, name := range names {
		vm, ok := known[name]
		if !ok {
			available := make([]string, len(herdfile.VMs))
			for i, v := range herdfile.VMs {
				available[i] = v.Name
			}
			return nil, fmt.Errorf("unknown VM %q — available: %s", name, strings.Join(available, ", "))
		}
		result = append(result, vm)
	}
	return result, nil
}
