package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const HerdfileName = "Herdfile"

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
	}
}

func validate(herdfile *Herdfile) error {
	if len(herdfile.VMs) == 0 {
		return fmt.Errorf("%s must define at least one VM under 'vms:'", HerdfileName)
	}

	names := make(map[string]bool)
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
