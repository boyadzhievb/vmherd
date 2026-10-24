package vm

import (
	"crypto/rand"
	"fmt"
	"os/exec"
	"strings"

	"github.com/boyadzhievb/vmherd/internal/ui"
)

// GenerateMACAddress creates a random MAC address with the QEMU OUI prefix (52:54:00).
func GenerateMACAddress() (string, error) {
	randomBytes := make([]byte, 3)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", fmt.Errorf("cannot generate MAC address: %w", err)
	}
	return fmt.Sprintf("52:54:00:%02x:%02x:%02x", randomBytes[0], randomBytes[1], randomBytes[2]), nil
}

// SharedFolderMount describes a host directory shared with the guest via 9p virtio.
type SharedFolderMount struct {
	HostPath  string
	GuestPath string
	MountTag  string
}

// CreateOptions holds everything needed to create a VM via virt-install.
type CreateOptions struct {
	DomainName         string
	DiskPath           string
	CloudInitUserData  string
	CloudInitMetaData  string
	CloudInitNetConfig string
	NetworkName        string
	MACAddress         string
	Memory             int
	CPUs               int
	OSVariant          string
	ImagePath          string
	DiskSize           string
	SharedFolders      []SharedFolderMount
}

// Create provisions a new VM: creates the backing disk and runs virt-install.
func Create(opts CreateOptions) error {
	if err := createDisk(opts.DiskPath, opts.ImagePath, opts.DiskSize); err != nil {
		return err
	}

	args := []string{
		"virt-install",
		"--name", opts.DomainName,
		"--memory", fmt.Sprintf("%d", opts.Memory),
		"--vcpus", fmt.Sprintf("%d", opts.CPUs),
		"--disk", fmt.Sprintf("path=%s,format=qcow2", opts.DiskPath),
		"--cloud-init", fmt.Sprintf("user-data=%s,meta-data=%s,network-config=%s",
			opts.CloudInitUserData, opts.CloudInitMetaData, opts.CloudInitNetConfig),
		"--network", fmt.Sprintf("network=%s,mac=%s", opts.NetworkName, opts.MACAddress),
		"--os-variant", opts.OSVariant,
		"--graphics", "none",
		"--noautoconsole",
		"--import",
		"--autostart",
	}

	for _, sf := range opts.SharedFolders {
		args = append(args,
			"--filesystem", fmt.Sprintf("source.dir=%s,target.dir=%s,accessmode=mapped", sf.HostPath, sf.MountTag),
		)
	}

	cmd := exec.Command("sudo", args...)
	cmd.Stdout = ui.Stdout()
	cmd.Stderr = ui.Stderr()

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("virt-install failed for %s: %w — check that libvirtd is running and you have permission", opts.DomainName, err)
	}

	return nil
}

// Start boots a stopped VM.
func Start(domainName string) error {
	if err := virsh("start", domainName); err != nil {
		return fmt.Errorf("cannot start VM %s: %w", domainName, err)
	}
	return nil
}

// Shutdown sends a graceful shutdown signal to a VM.
func Shutdown(domainName string) error {
	if err := virsh("shutdown", domainName); err != nil {
		return fmt.Errorf("cannot shut down VM %s: %w", domainName, err)
	}
	return nil
}

// ForceStop immediately powers off a VM.
func ForceStop(domainName string) error {
	return virsh("destroy", domainName)
}

// Destroy removes a VM and its storage.
func Destroy(domainName string) error {
	state := State(domainName)
	if state == "running" {
		_ = ForceStop(domainName)
	}

	if err := virsh("undefine", domainName, "--remove-all-storage"); err != nil {
		return fmt.Errorf("cannot destroy VM %s: %w", domainName, err)
	}
	return nil
}

// Exists returns true if the VM domain is defined in libvirt.
func Exists(domainName string) bool {
	cmd := exec.Command("sudo", "virsh", "dominfo", domainName)
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run() == nil
}

// State returns the current state of a VM (running, shut off, etc).
// Returns "not created" if the VM doesn't exist.
func State(domainName string) string {
	out, err := exec.Command("sudo", "virsh", "domstate", domainName).Output()
	if err != nil {
		return "not created"
	}
	return strings.TrimSpace(string(out))
}

func createDisk(diskPath, backingImage, diskSize string) error {
	cmd := exec.Command("sudo", "qemu-img", "create",
		"-f", "qcow2",
		"-b", backingImage,
		"-F", "qcow2",
		diskPath, diskSize,
	)
	cmd.Stdout = ui.Stdout()
	cmd.Stderr = ui.Stderr()

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("cannot create disk %s: %w", diskPath, err)
	}
	return nil
}

func virsh(args ...string) error {
	cmd := exec.Command("sudo", append([]string{"virsh"}, args...)...)
	cmd.Stdout = ui.Stdout()
	cmd.Stderr = ui.Stderr()
	return cmd.Run()
}
