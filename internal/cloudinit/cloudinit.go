package cloudinit

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/boyadzhievb/vmherd/internal/config"
)

// Files holds the paths to generated cloud-init configuration files.
type Files struct {
	UserData      string
	MetaData      string
	NetworkConfig string
}

// GenerateFiles creates the cloud-init configuration files for a VM.
func GenerateFiles(stateDir string, vm config.VM, sshUser, sshPubKey, subnetBase, dnsServer string) (*Files, error) {
	seedDir := filepath.Join(stateDir, vm.Name)
	if err := os.MkdirAll(seedDir, 0o755); err != nil {
		return nil, fmt.Errorf("cannot create cloud-init directory for %s: %w", vm.Name, err)
	}

	gateway := vm.Network.Gateway
	if gateway == "" {
		gateway = subnetBase + ".1"
	}

	if err := writeUserData(seedDir, vm, sshUser, sshPubKey); err != nil {
		return nil, err
	}
	if err := writeNetworkConfig(seedDir, vm.Network.IP, gateway, dnsServer); err != nil {
		return nil, err
	}
	if err := writeMetaData(seedDir, vm.Name); err != nil {
		return nil, err
	}

	return &Files{
		UserData:      filepath.Join(seedDir, "user-data"),
		MetaData:      filepath.Join(seedDir, "meta-data"),
		NetworkConfig: filepath.Join(seedDir, "network-config"),
	}, nil
}

func writeUserData(seedDir string, vm config.VM, sshUser, sshPubKey string) error {
	var builder strings.Builder
	builder.WriteString("#cloud-config\n")
	fmt.Fprintf(&builder, "hostname: %s\n", vm.Name)
	builder.WriteString("manage_etc_hosts: true\n")
	builder.WriteString("users:\n")
	fmt.Fprintf(&builder, "  - name: %s\n", sshUser)
	builder.WriteString("    sudo: ALL=(ALL) NOPASSWD:ALL\n")
	builder.WriteString("    shell: /bin/bash\n")
	builder.WriteString("    ssh_authorized_keys:\n")
	fmt.Fprintf(&builder, "      - %s\n", sshPubKey)
	builder.WriteString("ssh_pwauth: false\n")
	builder.WriteString("package_update: false\n")
	builder.WriteString("package_upgrade: false\n")

	if len(vm.Provision) > 0 {
		var runcmds []string
		var writeFiles []config.Provision

		for _, step := range vm.Provision {
			switch step.Type {
			case "shell":
				if step.Inline != "" {
					runcmds = append(runcmds, step.Inline)
				}
			case "file":
				writeFiles = append(writeFiles, step)
			}
		}

		if len(writeFiles) > 0 {
			builder.WriteString("write_files:\n")
			for _, wf := range writeFiles {
				content, err := os.ReadFile(wf.Source)
				if err != nil {
					return fmt.Errorf("cannot read provisioning file %s: %w", wf.Source, err)
				}
				encoded := base64.StdEncoding.EncodeToString(content)
				fmt.Fprintf(&builder, "  - path: %s\n", wf.Destination)
				builder.WriteString("    encoding: b64\n")
				fmt.Fprintf(&builder, "    content: %s\n", encoded)
				builder.WriteString("    permissions: '0755'\n")
			}
		}

		if len(runcmds) > 0 {
			builder.WriteString("runcmd:\n")
			for _, cmd := range runcmds {
				for _, line := range strings.Split(strings.TrimSpace(cmd), "\n") {
					fmt.Fprintf(&builder, "  - %s\n", line)
				}
			}
		}
	}

	return os.WriteFile(filepath.Join(seedDir, "user-data"), []byte(builder.String()), 0o644)
}

func writeNetworkConfig(seedDir, ipAddress, gateway, dnsServer string) error {
	content := fmt.Sprintf(`version: 2
ethernets:
  enp1s0:
    dhcp4: false
    addresses:
      - %s/24
    routes:
      - to: default
        via: %s
    nameservers:
      addresses:
        - %s
`, ipAddress, gateway, dnsServer)

	return os.WriteFile(filepath.Join(seedDir, "network-config"), []byte(content), 0o644)
}

func writeMetaData(seedDir, vmName string) error {
	content := fmt.Sprintf("instance-id: %s\nlocal-hostname: %s\n", vmName, vmName)
	return os.WriteFile(filepath.Join(seedDir, "meta-data"), []byte(content), 0o644)
}
