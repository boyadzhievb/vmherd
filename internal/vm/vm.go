package vm

import (
	"crypto/rand"
	"fmt"
	"os/exec"
	"strings"

	"github.com/boyadzhievb/vmherd/internal/libvirt"
	"github.com/boyadzhievb/vmherd/internal/ui"
)

// VMState represents the lifecycle state of a virtual machine.
type VMState string

const (
	// StateRunning indicates the VM is powered on and active.
	StateRunning VMState = "running"
	// StateStopped indicates the VM exists but is powered off.
	StateStopped VMState = "shut off"
	// StateNotCreated indicates the VM does not exist in libvirt.
	StateNotCreated VMState = "not created"
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
	HostPath  string // absolute path on the host
	GuestPath string // absolute mount path inside the guest
	MountTag  string // 9p mount tag for virtio passthrough
}

// CreateOptions holds everything needed to create a VM via virt-install.
type CreateOptions struct {
	VMName             string              // human-readable name from the Herdfile
	DomainName         string              // libvirt domain name (project-scoped)
	DiskPath           string              // path to the VM's qcow2 disk image
	CloudInitUserData  string              // path to cloud-init user-data file
	CloudInitMetaData  string              // path to cloud-init meta-data file
	CloudInitNetConfig string              // path to cloud-init network-config file
	NetworkName        string              // libvirt network to attach the VM to
	MACAddress         string              // assigned MAC address
	Memory             int                 // RAM in megabytes
	CPUs               int                 // virtual CPU count
	OSVariant          string              // virt-install os-variant hint
	ImagePath          string              // path to the backing cloud image
	DiskSize           string              // disk size with unit (e.g. 20G)
	SharedFolders      []SharedFolderMount // 9p host-to-guest directory mounts
}

// Create provisions a new VM: creates the backing disk and runs virt-install.
func Create(opts CreateOptions) error {
	if err := checkDiskPathAvailable(opts.DiskPath, opts.VMName); err != nil {
		return err
	}

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
		if err := ensureSharedFolderAccess(sf.HostPath); err != nil {
			return err
		}
		args = append(args,
			"--filesystem", fmt.Sprintf("source.dir=%s,target.dir=%s,accessmode=mapped", sf.HostPath, sf.MountTag),
		)
	}

	cmd := exec.Command("sudo", args...)
	cmd.Stdout = ui.Stdout()
	cmd.Stderr = ui.Stderr()

	if err := cmd.Run(); err != nil {
		if cleanupErr := exec.Command("sudo", "unlink", opts.DiskPath).Run(); cleanupErr != nil {
			ui.Printf("Warning: could not clean up disk %s: %v\n", opts.DiskPath, cleanupErr)
		}
		return fmt.Errorf("virt-install failed for %s: %w — check that libvirtd is running and you have permission", opts.DomainName, err)
	}

	return nil
}

// Start boots a stopped VM.
func Start(domainName string) error {
	if err := libvirt.Virsh("start", domainName); err != nil {
		return fmt.Errorf("cannot start VM %s: %w", domainName, err)
	}
	return nil
}

// Shutdown sends a graceful shutdown signal to a VM.
func Shutdown(domainName string) error {
	if err := libvirt.Virsh("shutdown", domainName); err != nil {
		return fmt.Errorf("cannot shut down VM %s: %w", domainName, err)
	}
	return nil
}

// ForceStop immediately powers off a VM.
func ForceStop(domainName string) error {
	return libvirt.Virsh("destroy", domainName)
}

// Destroy removes a VM and its storage.
func Destroy(domainName string) error {
	state := State(domainName)
	if state == StateRunning {
		if err := ForceStop(domainName); err != nil {
			ui.Printf("Warning: force-stop failed for %s: %v\n", domainName, err)
		}
	}

	if err := libvirt.Virsh("undefine", domainName, "--remove-all-storage"); err != nil {
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
// Returns StateNotCreated if the VM doesn't exist.
func State(domainName string) VMState {
	out, err := exec.Command("sudo", "virsh", "domstate", domainName).Output()
	if err != nil {
		return StateNotCreated
	}
	return VMState(strings.TrimSpace(string(out)))
}

// checkDiskPathAvailable verifies that no file already exists at the target disk
// path. An existing file likely belongs to a stale or orphaned VM and should not
// be silently overwritten.
func checkDiskPathAvailable(diskPath, vmName string) error {
	cmd := exec.Command("sudo", "test", "-e", diskPath)
	if cmd.Run() == nil {
		return fmt.Errorf("disk %s already exists — it may belong to a previous VM. "+
			"Run 'vmherd destroy %s' to clean up, or remove the file manually with: sudo rm %s",
			diskPath, vmName, diskPath)
	}
	return nil
}

// createDisk creates a copy-on-write qcow2 disk backed by the cloud image.
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
