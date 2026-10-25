package config

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
)

// Project ties together the Herdfile with project-level paths and metadata.
type Project struct {
	Dir      string
	Name     string
	Herdfile *Herdfile
	StateDir string
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

	stateDir, err := resolveStateDir(dir)
	if err != nil {
		return nil, fmt.Errorf("cannot resolve state directory: %w", err)
	}

	return &Project{
		Dir:      dir,
		Name:     filepath.Base(dir),
		Herdfile: herdfile,
		StateDir: stateDir,
	}, nil
}

// resolveStateDir determines the persistent state directory for a project.
// Priority: VMHERD_STATE_DIR env var > global ~/.local/state/vmherd/<hash>.
// Falls back to the legacy .vmherd/ in-project directory if it already exists.
func resolveStateDir(projectDir string) (string, error) {
	if envDir := os.Getenv("VMHERD_STATE_DIR"); envDir != "" {
		if err := os.MkdirAll(envDir, 0o755); err != nil {
			return "", fmt.Errorf("cannot create VMHERD_STATE_DIR %s: %w", envDir, err)
		}
		return envDir, nil
	}

	// If .vmherd already exists as a real directory (not a symlink), keep using
	// it for backwards compatibility — don't move state out from under the user.
	localDotDir := filepath.Join(projectDir, ".vmherd")
	if info, err := os.Lstat(localDotDir); err == nil && info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
		return localDotDir, nil
	}

	globalDir, err := globalStateDir(projectDir)
	if err != nil {
		return localDotDir, nil
	}

	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		return localDotDir, nil
	}

	return globalDir, nil
}

// globalStateDir returns $XDG_STATE_HOME/vmherd/<name>-<hash> where the hash
// is derived from the absolute path of the project directory. This avoids
// collisions between projects with the same directory name. Falls back to
// ~/.local/state/vmherd/ when XDG_STATE_HOME is unset.
func globalStateDir(projectDir string) (string, error) {
	absPath, err := filepath.Abs(projectDir)
	if err != nil {
		return "", err
	}

	pathHash := sha256.Sum256([]byte(absPath))
	shortHash := hex.EncodeToString(pathHash[:8])
	dirName := filepath.Base(projectDir) + "-" + shortHash

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

// DomainName returns the libvirt domain name for a VM in this project.
func (p *Project) DomainName(vmName string) string {
	return fmt.Sprintf("vmherd-%s-%s", p.Name, vmName)
}

// DiskPath returns the qcow2 disk path for a VM.
func (p *Project) DiskPath(vmName string) string {
	return fmt.Sprintf("/var/lib/libvirt/images/%s.qcow2", p.DomainName(vmName))
}

// SSHKeyPath returns the path to the project's ed25519 private key.
func (p *Project) SSHKeyPath() string {
	return filepath.Join(p.StateDir, "id_ed25519")
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

// ExampleHerdfile returns a sample Herdfile for 'vmherd init'.
func ExampleHerdfile() string {
	return strings.TrimSpace(`
# Herdfile — vmherd VM definitions
# Docs: https://github.com/boyadzhievb/vmherd

vms:
  - name: ctrl
    image: ubuntu:noble
    cpus: 2
    memory: 4096
    disk: 20G
    network:
      ip: 192.168.122.10
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
      ip: 192.168.122.20

network:
  name: vmherd-default
  subnet: 192.168.122.0/24
  dns: auto

ssh:
  user: vmherd
`) + "\n"
}
