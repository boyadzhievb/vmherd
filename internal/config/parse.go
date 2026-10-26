package config

import (
	"fmt"
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
	defaultSharedFolderHost  = "."
	defaultSharedFolderGuest = "/vagrant"

	// maxResourceNameLength is the maximum length for VM names.
	maxResourceNameLength = 63
	// maxNetworkNameLength is the maximum length for network names.
	maxNetworkNameLength = 253
)

// resourceNamePattern matches valid VM and network names: alphanumeric with hyphens,
// starting and ending with an alphanumeric character.
var resourceNamePattern = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?$`)

// diskSizePattern matches valid disk size specifications: digits followed by a unit suffix.
var diskSizePattern = regexp.MustCompile(`^[0-9]+[KMGT]$`)

// forbiddenPathPrefixes lists system directories that shared folder host paths must not resolve into.
var forbiddenPathPrefixes = []string{"/etc", "/root", "/var/lib", "/usr", "/bin", "/sbin", "/sys", "/proc", "/boot"}

// Load reads and parses a Herdfile from the given directory.
func Load(dir string) (*Herdfile, error) {
	path := filepath.Join(dir, HerdfileName)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w — run 'vmherd init' to create one", HerdfileName, err)
	}

	var herdfile Herdfile
	if err := yaml.Unmarshal(data, &herdfile); err != nil {
		return nil, fmt.Errorf("invalid %s syntax: %w", HerdfileName, err)
	}

	applyDefaults(&herdfile, dir)

	if err := validate(&herdfile, dir); err != nil {
		return nil, err
	}

	return &herdfile, nil
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
		if len(herdfile.VMs[i].SharedFolders) == 0 {
			herdfile.VMs[i].SharedFolders = []SharedFolder{
				{Host: defaultSharedFolderHost, Guest: defaultSharedFolderGuest},
			}
		}
		for j := range herdfile.VMs[i].Ports {
			if herdfile.VMs[i].Ports[j].Protocol == "" {
				herdfile.VMs[i].Ports[j].Protocol = defaultPortProtocol
			}
		}
	}
}

// validate checks the Herdfile for semantic errors after defaults are applied.
func validate(herdfile *Herdfile, projectDir string) error {
	if len(herdfile.VMs) == 0 {
		return fmt.Errorf("%s must define at least one VM under 'vms:'", HerdfileName)
	}

	names := make(map[string]bool)
	usedHostPorts := make(map[string]string)
	usedIPs := make(map[string]string) // ip → vm name

	for _, vm := range herdfile.VMs {
		if vm.Name == "" {
			return fmt.Errorf("every VM must have a 'name'")
		}
		if !isValidResourceName(vm.Name, maxResourceNameLength) {
			return fmt.Errorf("VM name %q is invalid — use only letters, digits, and hyphens (max 63 chars, must start and end with alphanumeric)", vm.Name)
		}
		if names[vm.Name] {
			return fmt.Errorf("duplicate VM name: %q", vm.Name)
		}
		names[vm.Name] = true

		if vm.Image == "" {
			return fmt.Errorf("VM %q must have an 'image' (e.g. ubuntu:noble)", vm.Name)
		}

		parts := strings.SplitN(vm.Image, ":", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return fmt.Errorf("VM %q: image must be 'distro:codename' format (e.g. ubuntu:noble), got %q", vm.Name, vm.Image)
		}

		if err := validateDiskSize(vm.Disk, vm.Name); err != nil {
			return err
		}

		if err := validateVMIPAddress(vm, herdfile.Network.Subnet, usedIPs); err != nil {
			return err
		}

		if err := validateSharedFolders(vm, projectDir); err != nil {
			return err
		}

		if err := validatePortForwards(vm, usedHostPorts); err != nil {
			return err
		}

		if err := validateProvisionSteps(vm, projectDir); err != nil {
			return err
		}
	}

	if err := validateNetworkName(herdfile.Network.Name); err != nil {
		return err
	}

	return nil
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

// validateDiskSize checks that the disk field uses a valid size format like "20G" or "100M".
func validateDiskSize(diskSize string, vmName string) error {
	if !diskSizePattern.MatchString(diskSize) {
		return fmt.Errorf("VM %q: disk size %q is invalid — use a number followed by K, M, G, or T (e.g. '20G')", vmName, diskSize)
	}
	return nil
}

// validateVMIPAddress validates a VM's IP: correct format, no duplicates, within subnet.
func validateVMIPAddress(vmConfig VM, subnet string, usedIPs map[string]string) error {
	if vmConfig.Network.IP == "" {
		return nil
	}

	parsedIP := net.ParseIP(vmConfig.Network.IP)
	if parsedIP == nil || parsedIP.To4() == nil {
		return fmt.Errorf("VM %q: IP address %q is not a valid IPv4 address", vmConfig.Name, vmConfig.Network.IP)
	}

	if existingVM, taken := usedIPs[vmConfig.Network.IP]; taken {
		return fmt.Errorf("VM %q: IP address %s is already used by VM %q", vmConfig.Name, vmConfig.Network.IP, existingVM)
	}

	_, subnetCIDR, err := net.ParseCIDR(subnet)
	if err != nil {
		return fmt.Errorf("network subnet %q is not valid CIDR: %w", subnet, err)
	}
	if !subnetCIDR.Contains(parsedIP) {
		return fmt.Errorf("VM %q: IP address %s is outside the network subnet %s", vmConfig.Name, vmConfig.Network.IP, subnet)
	}

	usedIPs[vmConfig.Network.IP] = vmConfig.Name
	return nil
}

// validateSharedFolders checks shared folder paths for emptiness, absolute guest paths,
// and forbidden system directory targets.
func validateSharedFolders(vmConfig VM, projectDir string) error {
	for _, sharedFolder := range vmConfig.SharedFolders {
		if sharedFolder.Host == "" {
			return fmt.Errorf("VM %q: shared_folders entry has empty 'host' path", vmConfig.Name)
		}
		if sharedFolder.Guest == "" {
			return fmt.Errorf("VM %q: shared_folders entry has empty 'guest' path", vmConfig.Name)
		}
		if !strings.HasPrefix(sharedFolder.Guest, "/") {
			return fmt.Errorf("VM %q: shared_folders guest path %q must be absolute (start with /)", vmConfig.Name, sharedFolder.Guest)
		}

		resolvedHostPath := resolveHostPath(sharedFolder.Host, projectDir)
		if isForbiddenSystemPath(resolvedHostPath) {
			return fmt.Errorf("VM %q: shared_folders host path %q resolves to forbidden system directory %q — choose a path outside system directories", vmConfig.Name, sharedFolder.Host, resolvedHostPath)
		}
	}
	return nil
}

// resolveHostPath converts a host path to an absolute path for validation.
// Relative paths are joined with the project directory.
func resolveHostPath(hostPath string, projectDir string) string {
	if filepath.IsAbs(hostPath) {
		return filepath.Clean(hostPath)
	}
	return filepath.Clean(filepath.Join(projectDir, hostPath))
}

// isForbiddenSystemPath checks whether a resolved path falls under any forbidden system directory.
func isForbiddenSystemPath(resolvedPath string) bool {
	for _, prefix := range forbiddenPathPrefixes {
		if resolvedPath == prefix || strings.HasPrefix(resolvedPath, prefix+"/") {
			return true
		}
	}
	return false
}

// validateProvisionSteps checks that file-type provision sources are relative
// and do not escape the project directory.
func validateProvisionSteps(vmConfig VM, projectDir string) error {
	for _, step := range vmConfig.Provision {
		if step.Type != "file" {
			continue
		}
		if step.Source == "" {
			continue
		}
		if filepath.IsAbs(step.Source) {
			return fmt.Errorf("VM %q: provision source %q must be a relative path — use a path relative to your project directory", vmConfig.Name, step.Source)
		}
		cleanPath := filepath.Clean(filepath.Join(projectDir, step.Source))
		if !strings.HasPrefix(cleanPath, projectDir) {
			return fmt.Errorf("VM %q: provision source %q escapes project directory — provision files must be inside the project", vmConfig.Name, step.Source)
		}
	}
	return nil
}

// validatePortForwards checks port ranges, protocols, and host port uniqueness across VMs.
func validatePortForwards(vmConfig VM, usedHostPorts map[string]string) error {
	for _, portForward := range vmConfig.Ports {
		if portForward.Host < minPort || portForward.Host > maxPort {
			return fmt.Errorf("VM %q: host port %d is out of range (%d-%d)", vmConfig.Name, portForward.Host, minPort, maxPort)
		}
		if portForward.Guest < minPort || portForward.Guest > maxPort {
			return fmt.Errorf("VM %q: guest port %d is out of range (%d-%d)", vmConfig.Name, portForward.Guest, minPort, maxPort)
		}
		if portForward.Protocol != "tcp" && portForward.Protocol != "udp" {
			return fmt.Errorf("VM %q: port protocol must be 'tcp' or 'udp', got %q", vmConfig.Name, portForward.Protocol)
		}

		portKey := fmt.Sprintf("%d/%s", portForward.Host, portForward.Protocol)
		if existingVM, taken := usedHostPorts[portKey]; taken {
			return fmt.Errorf("VM %q: host port %s is already forwarded by VM %q", vmConfig.Name, portKey, existingVM)
		}
		usedHostPorts[portKey] = vmConfig.Name
	}
	return nil
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
