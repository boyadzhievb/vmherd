package config

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

const (
	// pathHashBytes is how many SHA-256 bytes of the project path are kept (hex doubles it).
	pathHashBytes = 8
	// domainHashLength is how many hex characters of the path hash go into libvirt names.
	domainHashLength = 8
	// maxProjectNamePartLength caps the project part of libvirt domain and disk names.
	maxProjectNamePartLength = 32
	// fallbackProjectNamePart replaces a project name with no usable characters.
	fallbackProjectNamePart = "project"
)

// Project ties together the Herdfile with project-level paths and metadata.
type Project struct {
	Dir      string    // absolute project directory (where the Herdfile lives)
	Name     string    // project directory basename, as typed by the user
	PathHash string    // hex SHA-256 prefix of Dir; keeps libvirt names unique per project
	Herdfile *Herdfile // parsed and validated Herdfile
	StateDir string    // persistent state directory for this project

	legacyDomainVMs map[string]bool // VMs that still use the pre-M0.8 domain name
}

// LoadProject loads the Herdfile from the current directory and returns a Project.
func LoadProject() (*Project, error) {
	dir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("cannot determine working directory: %w", err)
	}

	herdfile, err := Load(dir)
	if err != nil {
		return nil, err
	}

	stateDir, err := resolveStateDir(dir, herdfile.VMNames())
	if err != nil {
		return nil, fmt.Errorf("cannot resolve state directory: %w", err)
	}

	pathHash, err := projectPathHash(dir)
	if err != nil {
		return nil, fmt.Errorf("cannot hash project directory %s: %w", dir, err)
	}

	return &Project{
		Dir:      dir,
		Name:     filepath.Base(dir),
		PathHash: pathHash,
		Herdfile: herdfile,
		StateDir: stateDir,
	}, nil
}

// resolveStateDir determines the persistent state directory for a project
// and makes sure it carries the vmherd marker file.
// Priority: VMHERD_STATE_DIR env var > legacy in-project .vmherd/ > global
// ~/.local/state/vmherd/<hash>. vmNames are the Herdfile VMs, used to vet VMHERD_STATE_DIR.
func resolveStateDir(projectDir string, vmNames []string) (string, error) {
	if envDir := os.Getenv("VMHERD_STATE_DIR"); envDir != "" {
		if err := adoptUserStateDir(envDir, vmNames); err != nil {
			return "", err
		}
		return envDir, nil
	}

	// If .vmherd already exists as a real directory (not a symlink), keep using
	// it for backwards compatibility — don't move state out from under the user.
	localDotDir := filepath.Join(projectDir, ".vmherd")
	if info, err := os.Lstat(localDotDir); err == nil && info.IsDir() {
		return localDotDir, ensureStateDirMarker(localDotDir)
	}

	globalDir, err := globalStateDir(projectDir)
	if err != nil {
		return localDotDir, ensureStateDirMarker(localDotDir)
	}

	if err := os.MkdirAll(globalDir, stateDirPermissions); err != nil {
		return localDotDir, ensureStateDirMarker(localDotDir)
	}

	return globalDir, ensureStateDirMarker(globalDir)
}

// globalStateDir returns $XDG_STATE_HOME/vmherd/<name>-<hash> where the hash
// is derived from the absolute path of the project directory. This avoids
// collisions between projects with the same directory name. Falls back to
// ~/.local/state/vmherd/ when XDG_STATE_HOME is unset.
func globalStateDir(projectDir string) (string, error) {
	pathHash, err := projectPathHash(projectDir)
	if err != nil {
		return "", err
	}
	dirName := filepath.Base(projectDir) + "-" + pathHash

	stateHome := os.Getenv("XDG_STATE_HOME")
	if stateHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		stateHome = filepath.Join(home, ".local", "state")
	}

	return filepath.Join(stateHome, "vmherd", dirName), nil
}

// projectPathHash returns the hex SHA-256 prefix of the absolute project path.
// The state directory and libvirt names both use it, so two projects with the
// same directory name never share state, domains or disks.
func projectPathHash(projectDir string) (string, error) {
	absPath, err := filepath.Abs(projectDir)
	if err != nil {
		return "", fmt.Errorf("resolving absolute path of %s: %w", projectDir, err)
	}
	pathHash := sha256.Sum256([]byte(absPath))
	return hex.EncodeToString(pathHash[:pathHashBytes]), nil
}

// DomainName returns the libvirt domain name for a VM in this project:
// vmherd-<project>-<hash>-<vm>, or the pre-M0.8 name for VMs created under it.
func (p *Project) DomainName(vmName string) string {
	if p.legacyDomainVMs[vmName] {
		return p.LegacyDomainName(vmName)
	}
	return libvirtNamePrefix(p.Name, p.PathHash) + "-" + vmName
}

// LegacyDomainName returns the pre-M0.8 domain name (vmherd-<basename>-<vm>).
// It is not unique across projects; use it only for VMs this project created.
func (p *Project) LegacyDomainName(vmName string) string {
	return fmt.Sprintf("vmherd-%s-%s", p.Name, vmName)
}

// UseLegacyDomainName makes DomainName and DiskPath return the pre-M0.8 names
// for vmName. Call it only after proving the old domain belongs to this project.
func (p *Project) UseLegacyDomainName(vmName string) {
	if p.legacyDomainVMs == nil {
		p.legacyDomainVMs = make(map[string]bool)
	}
	p.legacyDomainVMs[vmName] = true
}

// DropLegacyDomainName switches vmName back to the project-unique name, for
// example after reload destroyed the old domain and recreates it.
func (p *Project) DropLegacyDomainName(vmName string) {
	delete(p.legacyDomainVMs, vmName)
}

// sanitizeProjectName maps a directory name to [a-z0-9-] for libvirt names:
// lowercase, other characters become '-', runs of '-' collapse, ends are
// trimmed and the result is capped. An empty result becomes "project".
func sanitizeProjectName(projectName string) string {
	var builder strings.Builder
	lastWasDash := false
	for _, character := range strings.ToLower(projectName) {
		isAllowed := (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9')
		if isAllowed {
			builder.WriteRune(character)
			lastWasDash = false
			continue
		}
		if !lastWasDash {
			builder.WriteByte('-')
			lastWasDash = true
		}
	}
	sanitized := strings.Trim(builder.String(), "-")
	if len(sanitized) > maxProjectNamePartLength {
		sanitized = strings.TrimRight(sanitized[:maxProjectNamePartLength], "-")
	}
	if sanitized == "" {
		return fallbackProjectNamePart
	}
	return sanitized
}

// DiskPath returns the qcow2 disk path for a VM.
func (p *Project) DiskPath(vmName string) string {
	return fmt.Sprintf("/var/lib/libvirt/images/%s.qcow2", p.DomainName(vmName))
}

// SSHKeyPath returns the path to the project's ed25519 private key.
func (p *Project) SSHKeyPath() string {
	return filepath.Join(p.StateDir, "id_ed25519")
}

// ConsoleLogPath returns the path where serial console output is logged for a VM.
func (p *Project) ConsoleLogPath(vmName string) string {
	return filepath.Join(p.StateDir, vmName, "console.log")
}

// SubnetBase extracts the first three octets from the network subnet.
// e.g. "192.168.122.0/24" -> "192.168.122"
func (p *Project) SubnetBase() (string, error) {
	subnet := p.Herdfile.Network.Subnet
	_, ipNet, err := net.ParseCIDR(subnet)
	if err != nil {
		return "", fmt.Errorf("invalid subnet %q: %w", subnet, err)
	}
	ip := ipNet.IP.To4()
	if ip == nil {
		return "", fmt.Errorf("subnet %q is not IPv4", subnet)
	}
	return fmt.Sprintf("%d.%d.%d", ip[0], ip[1], ip[2]), nil
}

// GatewayIP returns the gateway IP for the network (subnet base + .1).
func (p *Project) GatewayIP() (string, error) {
	base, err := p.SubnetBase()
	if err != nil {
		return "", err
	}
	return base + ".1", nil
}

// ImageCacheDir returns the path where cloud images are cached.
func ImageCacheDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.TempDir()
	}
	return filepath.Join(home, ".vmherd", "images")
}

// exampleHerdfileTemplate is the Herdfile 'vmherd init' writes. The network
// name and subnet are filled in per project so projects don't share a network.
var exampleHerdfileTemplate = template.Must(template.New("Herdfile").Parse(`# Herdfile — vmherd VM definitions
# Docs: https://github.com/boyadzhievb/vmherd

images:
  ubuntu:noble: https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-amd64.img
  ubuntu:resolute: https://cloud-images.ubuntu.com/resolute/current/resolute-server-cloudimg-amd64.img
  fedora:42: https://download.fedoraproject.org/pub/fedora/linux/releases/42/Cloud/x86_64/images/Fedora-Cloud-Base-Generic-42-1.0.x86_64.qcow2

vms:
  - name: ctrl
    image: ubuntu:noble
    cpus: 2
    memory: 4096
    disk: 20G
    network:
      ip: {{.SubnetBase}}.10
    shared_folders:
      - host: .
        guest: /vagrant
    ports:
      - host: 8080
        guest: 80
    provision:
      - type: shell
        inline: |
          apt-get update && apt-get install -y curl

  - name: worker-1
    image: ubuntu:noble
    cpus: 2
    memory: 2048
    disk: 20G
    network:
      ip: {{.SubnetBase}}.20

network:
  name: {{.NetworkName}}
  subnet: {{.SubnetBase}}.0/24
  dns: auto

ssh:
  user: vmherd
`))

// exampleHerdfileValues fills exampleHerdfileTemplate.
type exampleHerdfileValues struct {
	NetworkName string // project-unique libvirt network name
	SubnetBase  string // first three octets of a free /24, e.g. "192.168.150"
}

// ExampleHerdfile returns a sample Herdfile for 'vmherd init' that uses
// networkName and the /24 subnet starting at subnetBase.
func ExampleHerdfile(networkName, subnetBase string) (string, error) {
	var rendered strings.Builder
	if err := exampleHerdfileTemplate.Execute(&rendered, exampleHerdfileValues{NetworkName: networkName, SubnetBase: subnetBase}); err != nil {
		return "", fmt.Errorf("rendering example Herdfile: %w", err)
	}
	return strings.TrimSpace(rendered.String()) + "\n", nil
}

// DefaultNetworkName returns the project-unique libvirt network name that
// 'vmherd init' writes: vmherd-<project>-<hash>, matching the VM domain names.
func DefaultNetworkName(projectDir string) (string, error) {
	pathHash, err := projectPathHash(projectDir)
	if err != nil {
		return "", err
	}
	return libvirtNamePrefix(filepath.Base(projectDir), pathHash), nil
}

// libvirtNamePrefix returns vmherd-<sanitized project>-<short hash>, the
// project-unique start of vmherd's libvirt domain and network names.
func libvirtNamePrefix(projectName, pathHash string) string {
	shortHash := pathHash
	if len(shortHash) > domainHashLength {
		shortHash = shortHash[:domainHashLength]
	}
	return fmt.Sprintf("vmherd-%s-%s", sanitizeProjectName(projectName), shortHash)
}
