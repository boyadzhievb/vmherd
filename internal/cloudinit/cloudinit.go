package cloudinit

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/boyadzhievb/vmherd/internal/config"
	"gopkg.in/yaml.v3"
)

// Files holds the paths to generated cloud-init configuration files.
type Files struct {
	UserData      string // path to the user-data file
	MetaData      string // path to the meta-data file
	NetworkConfig string // path to the network-config file
}

// SharedMount describes a 9p mount to configure in cloud-init.
type SharedMount struct {
	MountTag  string // 9p mount tag matching the virtio device
	GuestPath string // absolute mount path inside the guest
}

// GenerateFiles creates the cloud-init configuration files for a VM.
// sshUserUID sets the SSH user's UID (0 keeps the guest's default allocation).
func GenerateFiles(stateDir string, vm config.VM, sshUser, sshPassword, sshPubKey, subnetBase, dnsServer, macAddress string, sharedMounts []SharedMount, sshUserUID int) (*Files, error) {
	seedDir := filepath.Join(stateDir, vm.Name)
	if err := os.MkdirAll(seedDir, 0o700); err != nil {
		return nil, fmt.Errorf("cannot create cloud-init directory for %s: %w", vm.Name, err)
	}

	gateway := vm.Network.Gateway
	if gateway == "" {
		gateway = subnetBase + ".1"
	}

	if err := writeUserData(seedDir, vm, sshUser, sshPassword, sshPubKey, sharedMounts, sshUserUID); err != nil {
		return nil, err
	}
	if err := writeNetworkConfig(seedDir, vm.Network.IP, gateway, dnsServer, macAddress); err != nil {
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

// writeUserData generates the cloud-init user-data file with users, packages, and provisioning.
// A non-zero sshUserUID pins the SSH user's UID (matching the host user for shared folders).
func writeUserData(seedDir string, vm config.VM, sshUser, userPassword, sshPubKey string, sharedMounts []SharedMount, sshUserUID int) error {
	var builder strings.Builder
	builder.WriteString("#cloud-config\n")
	fmt.Fprintf(&builder, "hostname: %s\n", vm.Name)
	builder.WriteString("manage_etc_hosts: true\n")
	builder.WriteString("users:\n")
	fmt.Fprintf(&builder, "  - name: %s\n", sshUser)
	if sshUserUID != 0 {
		fmt.Fprintf(&builder, "    uid: %d\n", sshUserUID)
	}
	builder.WriteString("    sudo: ALL=(ALL) NOPASSWD:ALL\n")
	builder.WriteString("    shell: /bin/bash\n")
	if userPassword != "" {
		hashedPassword, err := hashPassword(userPassword)
		if err != nil {
			return err
		}
		builder.WriteString("    lock_passwd: false\n")
		fmt.Fprintf(&builder, "    passwd: %s\n", hashedPassword)
	}
	builder.WriteString("    ssh_authorized_keys:\n")
	fmt.Fprintf(&builder, "      - %s\n", sshPubKey)
	builder.WriteString("ssh_pwauth: false\n")
	builder.WriteString("package_update: false\n")
	builder.WriteString("package_upgrade: false\n")

	hasRuncmdSection := false
	if len(vm.Provision) > 0 {
		wroteRuncmds, err := writeProvisionEntries(&builder, vm.Provision)
		if err != nil {
			return err
		}
		hasRuncmdSection = wroteRuncmds
	}

	if len(sharedMounts) > 0 {
		writeSharedMountEntries(&builder, sharedMounts, hasRuncmdSection)
	}

	return os.WriteFile(filepath.Join(seedDir, "user-data"), []byte(builder.String()), 0o600)
}

// writeProvisionEntries appends cloud-init write_files and runcmd entries for provisioning steps.
// Returns true if a runcmd section was written (so shared mounts can append to it).
func writeProvisionEntries(builder *strings.Builder, provisions []config.Provision) (bool, error) {
	var runcmds []string
	var writeFiles []config.Provision

	for _, step := range provisions {
		switch step.Type {
		case config.ProvisionShell:
			if step.Inline != "" {
				runcmds = append(runcmds, step.Inline)
			}
		case config.ProvisionFile:
			writeFiles = append(writeFiles, step)
		}
	}

	if len(writeFiles) > 0 {
		builder.WriteString("write_files:\n")
		for _, wf := range writeFiles {
			content, err := os.ReadFile(wf.Source)
			if err != nil {
				return false, fmt.Errorf("cannot read provisioning file %s: %w", wf.Source, err)
			}
			encoded := base64.StdEncoding.EncodeToString(content)
			fmt.Fprintf(builder, "  - path: %s\n", wf.Destination)
			builder.WriteString("    encoding: b64\n")
			fmt.Fprintf(builder, "    content: %s\n", encoded)
			builder.WriteString("    permissions: '0644'\n")
		}
	}

	writeRuncmdEntries(builder, runcmds)
	return len(runcmds) > 0, nil
}

// writeRuncmdEntries appends cloud-init runcmd entries for shell provisioning steps.
func writeRuncmdEntries(builder *strings.Builder, runcmds []string) {
	if len(runcmds) == 0 {
		return
	}
	builder.WriteString("runcmd:\n")
	for _, cmd := range runcmds {
		trimmedCommand := strings.TrimSpace(cmd)
		fmt.Fprintf(builder, "  - [\"/bin/bash\", \"-c\", %q]\n", trimmedCommand)
	}
}

// sharedMountOptions are the guest mount options for 9p shared folders. nofail
// keeps the guest booting if a share is missing; noatime avoids a host write on
// every read.
const sharedMountOptions = "trans=virtio,version=9p2000.L,noatime,nofail"

// writeSharedMountEntries appends runcmd and mounts entries for 9p shared folders.
func writeSharedMountEntries(builder *strings.Builder, sharedMounts []SharedMount, hasProvisionRuncmds bool) {
	if !hasProvisionRuncmds {
		builder.WriteString("runcmd:\n")
	}
	for _, mount := range sharedMounts {
		mkdirCommand := fmt.Sprintf("mkdir -p %s", mount.GuestPath)
		mountCommand := fmt.Sprintf("mount -t 9p -o %s %s %s", sharedMountOptions, mount.MountTag, mount.GuestPath)
		fmt.Fprintf(builder, "  - [\"/bin/bash\", \"-c\", %q]\n", mkdirCommand)
		fmt.Fprintf(builder, "  - [\"/bin/bash\", \"-c\", %q]\n", mountCommand)
	}
	builder.WriteString("mounts:\n")
	for _, mount := range sharedMounts {
		fmt.Fprintf(builder, "  - [%s, %s, 9p, \"%s\", \"0\", \"0\"]\n", mount.MountTag, mount.GuestPath, sharedMountOptions)
	}
}

// writeNetworkConfig generates the cloud-init network-config file. With an
// ipAddress the interface gets that static address; without one it uses
// DHCP keyed by MAC (the lease 'virsh domifaddr' looks up). Either way DNS is
// dnsServer (derived from the host), never the DHCP-provided server.
func writeNetworkConfig(seedDir, ipAddress, gateway, dnsServer, macAddress string) error {
	addressing := fmt.Sprintf(`    dhcp4: false
    addresses:
      - %s/24
    routes:
      - to: default
        via: %s
`, ipAddress, gateway)
	if ipAddress == "" {
		addressing = `    dhcp4: true
    dhcp-identifier: mac
    dhcp4-overrides:
      use-dns: false
`
	}

	content := fmt.Sprintf(`version: 2
ethernets:
  primary:
    match:
      macaddress: "%s"
%s    nameservers:
      addresses:
        - %s
`, macAddress, addressing, dnsServer)

	return os.WriteFile(filepath.Join(seedDir, "network-config"), []byte(content), 0o600)
}

// savedNetworkConfig is the part of a generated network-config that
// ReadSavedMACAddress needs.
type savedNetworkConfig struct {
	Ethernets map[string]struct {
		Match struct {
			MACAddress string `yaml:"macaddress"` // MAC the VM was created with
		} `yaml:"match"` // interface match rules
	} `yaml:"ethernets"` // interfaces keyed by cloud-init id
}

// ReadSavedMACAddress returns the MAC address that GenerateFiles wrote for
// vmName in stateDir. It returns "" and no error when no network-config exists,
// meaning this project never created the VM.
func ReadSavedMACAddress(stateDir, vmName string) (string, error) {
	networkConfigPath := filepath.Join(stateDir, vmName, "network-config")
	content, err := os.ReadFile(networkConfigPath)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", networkConfigPath, err)
	}

	var networkConfig savedNetworkConfig
	if err := yaml.Unmarshal(content, &networkConfig); err != nil {
		return "", fmt.Errorf("parsing %s: %w — delete it and run 'vmherd reload %s' to regenerate it", networkConfigPath, err, vmName)
	}
	for _, ethernet := range networkConfig.Ethernets {
		if ethernet.Match.MACAddress != "" {
			return strings.ToLower(ethernet.Match.MACAddress), nil
		}
	}
	return "", nil
}

// writeMetaData generates the cloud-init meta-data file with instance identity.
func writeMetaData(seedDir, vmName string) error {
	content := fmt.Sprintf("instance-id: %s\nlocal-hostname: %s\n", vmName, vmName)
	return os.WriteFile(filepath.Join(seedDir, "meta-data"), []byte(content), 0o600)
}

// hashPassword generates a SHA-512 password hash using openssl.
func hashPassword(plaintext string) (string, error) {
	cmd := exec.Command("openssl", "passwd", "-6", "-stdin")
	cmd.Stdin = strings.NewReader(plaintext)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("cannot hash password: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}
