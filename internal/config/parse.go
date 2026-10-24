package config

import (
	"fmt"
	"os"
	"path/filepath"
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
)

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

	if err := validate(&herdfile); err != nil {
		return nil, err
	}

	return &herdfile, nil
}

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

func validate(herdfile *Herdfile) error {
	if len(herdfile.VMs) == 0 {
		return fmt.Errorf("%s must define at least one VM under 'vms:'", HerdfileName)
	}

	names := make(map[string]bool)
	usedHostPorts := make(map[string]string)

	for _, vm := range herdfile.VMs {
		if vm.Name == "" {
			return fmt.Errorf("every VM must have a 'name'")
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

		if err := validateSharedFolders(vm); err != nil {
			return err
		}

		if err := validatePortForwards(vm, usedHostPorts); err != nil {
			return err
		}
	}

	return nil
}

func validateSharedFolders(vm VM) error {
	for _, sharedFolder := range vm.SharedFolders {
		if sharedFolder.Host == "" {
			return fmt.Errorf("VM %q: shared_folders entry has empty 'host' path", vm.Name)
		}
		if sharedFolder.Guest == "" {
			return fmt.Errorf("VM %q: shared_folders entry has empty 'guest' path", vm.Name)
		}
		if !strings.HasPrefix(sharedFolder.Guest, "/") {
			return fmt.Errorf("VM %q: shared_folders guest path %q must be absolute (start with /)", vm.Name, sharedFolder.Guest)
		}
	}
	return nil
}

func validatePortForwards(vm VM, usedHostPorts map[string]string) error {
	for _, portForward := range vm.Ports {
		if portForward.Host < minPort || portForward.Host > maxPort {
			return fmt.Errorf("VM %q: host port %d is out of range (%d-%d)", vm.Name, portForward.Host, minPort, maxPort)
		}
		if portForward.Guest < minPort || portForward.Guest > maxPort {
			return fmt.Errorf("VM %q: guest port %d is out of range (%d-%d)", vm.Name, portForward.Guest, minPort, maxPort)
		}
		if portForward.Protocol != "tcp" && portForward.Protocol != "udp" {
			return fmt.Errorf("VM %q: port protocol must be 'tcp' or 'udp', got %q", vm.Name, portForward.Protocol)
		}

		portKey := fmt.Sprintf("%d/%s", portForward.Host, portForward.Protocol)
		if existingVM, taken := usedHostPorts[portKey]; taken {
			return fmt.Errorf("VM %q: host port %s is already forwarded by VM %q", vm.Name, portKey, existingVM)
		}
		usedHostPorts[portKey] = vm.Name
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
