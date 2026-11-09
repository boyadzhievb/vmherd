package cloudinit

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

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

	if err := writeProvisionFiles(&builder, vm.Provision); err != nil {
		return err
	}

	// Order matters: the home fix must run before provisioning uses ~, and the
	// share fallback mounts come after provisioning (see buildRuncmdLines).
	if err := writeRuncmdSection(&builder, buildRuncmdLines(vm.Provision, sshUser, sharedMounts)); err != nil {
		return err
	}
	if err := writeMountsEntries(&builder, sharedMounts); err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(seedDir, "user-data"), []byte(builder.String()), 0o600)
}

// writeFileEntry is one cloud-init write_files entry for a file provisioning step.
type writeFileEntry struct {
	Path        string `yaml:"path"`        // absolute destination in the guest
	Encoding    string `yaml:"encoding"`    // always b64, so any file content is safe
	Content     string `yaml:"content"`     // base64 of the source file
	Permissions string `yaml:"permissions"` // octal mode string, e.g. '0644'
}

// provisionedFileEncoding and provisionedFilePermissions are the write_files
// encoding and mode of first-boot file steps.
const (
	provisionedFileEncoding    = "b64"
	provisionedFilePermissions = "0644"
)

// writeProvisionFiles appends cloud-init write_files entries for file
// provisioning steps, YAML-encoded by yaml.v3 so no path can break out of its
// entry. Shell steps go to runcmd (see buildRuncmdLines).
func writeProvisionFiles(builder *strings.Builder, provisions []config.Provision) error {
	var writeFiles []writeFileEntry
	for _, step := range provisions {
		if step.Type != config.ProvisionFile {
			continue
		}
		content, err := os.ReadFile(step.Source)
		if err != nil {
			return fmt.Errorf("cannot read provisioning file %s: %w", step.Source, err)
		}
		writeFiles = append(writeFiles, writeFileEntry{
			Path:        step.Destination,
			Encoding:    provisionedFileEncoding,
			Content:     base64.StdEncoding.EncodeToString(content),
			Permissions: provisionedFilePermissions,
		})
	}
	return writeYAMLSection(builder, "write_files", writeFiles)
}

// homeOwnershipCommand returns a runcmd that gives the SSH user its home
// directory back. cloud-init creates share mount points (for example
// /home/vagrant/app) before it creates the user, so the home directory ends
// up root-owned. Only the directory itself is changed, not its contents.
func homeOwnershipCommand(sshUser string) string {
	return fmt.Sprintf(`chown %s: "$(getent passwd %s | cut -d: -f6)"`, sshUser, sshUser)
}

// sharedMountOptions are the guest mount options for 9p shared folders. nofail
// keeps the guest booting if a share is missing; noatime avoids a host write on
// every read.
const sharedMountOptions = "trans=virtio,version=9p2000.L,noatime,nofail"

// sharedMountRuncmds returns the commands that create each share's mount
// point and mount it if the mounts module didn't already (a second mount
// would fail and mark the whole cloud-init run as failed).
func sharedMountRuncmds(sharedMounts []SharedMount) []string {
	var runcmds []string
	for _, mount := range sharedMounts {
		quotedGuestPath := quoteForShell(mount.GuestPath)
		runcmds = append(runcmds,
			fmt.Sprintf("mkdir -p %s", quotedGuestPath),
			fmt.Sprintf("mountpoint -q %s || mount -t 9p -o %s %s %s", quotedGuestPath, sharedMountOptions, mount.MountTag, quotedGuestPath))
	}
	return runcmds
}

// writeMountsEntries appends the cloud-init mounts section (fstab entries:
// device, mount point, type, options, dump, pass), YAML-encoded by yaml.v3 so
// no guest path can break out of its entry.
func writeMountsEntries(builder *strings.Builder, sharedMounts []SharedMount) error {
	mountEntries := make([][]string, 0, len(sharedMounts))
	for _, mount := range sharedMounts {
		mountEntries = append(mountEntries, []string{mount.MountTag, mount.GuestPath, "9p", sharedMountOptions, "0", "0"})
	}
	return writeYAMLSection(builder, "mounts", mountEntries)
}

// quoteForShell wraps value in single quotes for a POSIX shell. Each embedded
// single quote closes the quoting, adds a backslash-escaped quote and reopens.
func quoteForShell(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
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
// The instance-id includes a Unix timestamp so cloud-init treats each create
// (and each reload) as a new instance and reruns all modules.
func writeMetaData(seedDir, vmName string) error {
	timestamp := time.Now().Unix()
	content := fmt.Sprintf("instance-id: %s-%d\nlocal-hostname: %s\n", vmName, timestamp, vmName)
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
