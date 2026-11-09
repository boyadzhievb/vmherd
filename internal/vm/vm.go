package vm

import (
	"crypto/rand"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"

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
	// StateShuttingDown indicates the guest is handling a shutdown request.
	StateShuttingDown VMState = "in shutdown"
)

const (
	// GracefulShutdownTimeout is how long StopGracefully waits for the guest
	// to power off before forcing it off (Vagrant's graceful_halt_timeout).
	GracefulShutdownTimeout = 60 * time.Second
	// shutdownInitialPollInterval is the first wait between state checks while stopping.
	shutdownInitialPollInterval = 1 * time.Second
	// shutdownMaxPollInterval caps the backoff between state checks while stopping.
	shutdownMaxPollInterval = 4 * time.Second
)

// GenerateMACAddress creates a random MAC address with the QEMU OUI prefix (52:54:00).
func GenerateMACAddress() (string, error) {
	randomBytes := make([]byte, 3)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", fmt.Errorf("cannot generate MAC address: %w", err)
	}
	return fmt.Sprintf("52:54:00:%02x:%02x:%02x", randomBytes[0], randomBytes[1], randomBytes[2]), nil
}


const (
	// domifaddrProtocolColumn is the Protocol column in 'virsh domifaddr' output.
	domifaddrProtocolColumn = 2
	// domifaddrAddressColumn is the Address column (CIDR) in 'virsh domifaddr' output.
	domifaddrAddressColumn = 3
	// domifaddrMinimumFields is how many columns an address row has.
	domifaddrMinimumFields = 4
)

// SharedFolderMount describes a host directory shared with the guest via 9p virtio.
type SharedFolderMount struct {
	HostPath  string // absolute path on the host
	GuestPath string // absolute mount path inside the guest
	MountTag  string // 9p mount tag for virtio passthrough
}

// CreateOptions holds everything needed to create a VM: disk, seed ISO, network,
// and optional shared folders. The seed ISO is built by cloudinit.BuildAndInstallSeed
// before calling Create.
type CreateOptions struct {
	VMName           string              // human-readable name from the Herdfile
	DomainName       string              // libvirt domain name (project-scoped)
	DiskPath         string              // path to the VM's qcow2 disk image
	SeedPath         string              // path to the cloud-init seed ISO
	NetworkName      string              // libvirt network to attach the VM to
	MACAddress       string              // assigned MAC address
	Memory           int                 // RAM in megabytes
	CPUs             int                 // virtual CPU count
	ImagePath        string              // path to the backing cloud image
	DiskSize         string              // disk size with unit (e.g. 20G)
	SharedFolders    []SharedFolderMount // 9p host-to-guest directory mounts
	QEMUProcessLabel string              // DAC label "+UID:+GID" QEMU runs as; required when SharedFolders is set
	ConsoleLogPath   string              // if set, serial console output is logged to this file
}

// Create provisions a new VM: creates the backing disk, renders domain XML, and
// defines+starts the domain via virsh. On failure, it cleans up the disk and
// seed ISO so re-running 'vmherd up' works without leftover files.
func Create(opts CreateOptions) error {
	if err := checkDiskPathAvailable(opts.DiskPath, opts.VMName); err != nil {
		return err
	}

	if err := createDisk(opts.DiskPath, opts.ImagePath, opts.DiskSize); err != nil {
		return err
	}

	domainXML, err := RenderDomainXML(opts)
	if err != nil {
		cleanupDiskAndSeed(opts)
		return fmt.Errorf("creating VM %s: %w", opts.VMName, err)
	}

	if err := defineAndStart(opts.DomainName, domainXML); err != nil {
		cleanupDiskAndSeed(opts)
		return fmt.Errorf("creating VM %s: %w — check that libvirtd is running and you have permission", opts.VMName, err)
	}

	return nil
}

// defineAndStart writes domain XML to a temp file, defines the domain via virsh,
// enables autostart, and starts it. The temp file is always removed. If start
// fails after a successful define, the domain is undefined before returning.
func defineAndStart(domainName string, domainXML []byte) error {
	tmpFile, err := os.CreateTemp("", "vmherd-domain-*.xml")
	if err != nil {
		return fmt.Errorf("creating temp file for domain XML: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer func() {
		if removeErr := os.Remove(tmpPath); removeErr != nil && !os.IsNotExist(removeErr) {
			ui.Printf("Warning: could not remove temp XML file %s: %v\n", tmpPath, removeErr)
		}
	}()

	if _, err := tmpFile.Write(domainXML); err != nil {
		if closeErr := tmpFile.Close(); closeErr != nil {
			ui.Printf("Warning: could not close temp XML file %s: %v\n", tmpPath, closeErr)
		}
		return fmt.Errorf("writing domain XML: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("closing domain XML temp file: %w", err)
	}

	if err := libvirt.Virsh("define", tmpPath); err != nil {
		return fmt.Errorf("virsh define failed: %w", err)
	}

	if err := libvirt.Virsh("autostart", domainName); err != nil {
		if undefErr := libvirt.Virsh("undefine", domainName); undefErr != nil {
			ui.Printf("Warning: could not undefine %s after autostart failure: %v\n", domainName, undefErr)
		}
		return fmt.Errorf("virsh autostart failed: %w", err)
	}

	if err := libvirt.Virsh("start", domainName); err != nil {
		if undefErr := libvirt.Virsh("undefine", domainName); undefErr != nil {
			ui.Printf("Warning: could not undefine %s after start failure: %v\n", domainName, undefErr)
		}
		return fmt.Errorf("virsh start failed: %w", err)
	}

	return nil
}

// cleanupDiskAndSeed removes the disk and seed ISO after a failed creation
// attempt so re-running 'vmherd up' can start fresh.
func cleanupDiskAndSeed(opts CreateOptions) {
	if cleanupErr := exec.Command("sudo", "unlink", opts.DiskPath).Run(); cleanupErr != nil {
		ui.Printf("Warning: could not clean up disk %s: %v\n", opts.DiskPath, cleanupErr)
	}
	if cleanupErr := exec.Command("sudo", "rm", "-f", opts.SeedPath).Run(); cleanupErr != nil {
		ui.Printf("Warning: could not clean up seed %s: %v\n", opts.SeedPath, cleanupErr)
	}
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

// StopGracefully asks the guest to shut down (ACPI) and waits up to timeout
// for it to power off, checking with backoff. It forces the VM off if the
// request fails or the timeout passes, and reports whether it had to.
func StopGracefully(domainName string, timeout time.Duration) (bool, error) {
	if err := Shutdown(domainName); err != nil {
		ui.Printf("Warning: %v — forcing it off\n", err)
		return true, forceStopIfRunning(domainName)
	}

	deadline := time.Now().Add(timeout)
	pollInterval := shutdownInitialPollInterval
	for time.Now().Before(deadline) {
		if !isPoweredOn(State(domainName)) {
			return false, nil
		}
		time.Sleep(min(pollInterval, time.Until(deadline)))
		pollInterval = min(pollInterval*2, shutdownMaxPollInterval)
	}
	if !isPoweredOn(State(domainName)) {
		return false, nil
	}
	return true, forceStopIfRunning(domainName)
}

// isPoweredOn reports whether a domain in state still has a running guest,
// including one that is busy shutting down.
func isPoweredOn(state VMState) bool {
	return state == StateRunning || state == StateShuttingDown
}

// forceStopIfRunning powers the domain off unless it already stopped by itself.
func forceStopIfRunning(domainName string) error {
	if !isPoweredOn(State(domainName)) {
		return nil
	}
	if err := ForceStop(domainName); err != nil {
		return fmt.Errorf("cannot force-stop VM %s: %w", domainName, err)
	}
	return nil
}

// ForceStop immediately powers off a VM.
func ForceStop(domainName string) error {
	return libvirt.Virsh("destroy", domainName)
}

// Destroy stops a running VM and undefines its libvirt domain. It does not
// remove the disk or seed ISO — call RemoveDisk and cloudinit.RemoveSeed
// separately, because these files are created outside libvirt's storage pool
// and virsh --remove-all-storage cannot manage them.
func Destroy(domainName string) error {
	state := State(domainName)
	if state == StateRunning {
		if err := ForceStop(domainName); err != nil {
			ui.Printf("Warning: force-stop failed for %s: %v\n", domainName, err)
		}
	}

	if err := libvirt.Virsh("undefine", domainName); err != nil {
		return fmt.Errorf("cannot destroy VM %s: %w", domainName, err)
	}
	return nil
}

// RemoveDisk removes a VM's qcow2 disk via sudo. It is not an error if the
// file does not exist (idempotent for destroy/cleanup).
func RemoveDisk(diskPath string) error {
	checkCmd := exec.Command("sudo", "test", "-e", diskPath)
	if checkCmd.Run() != nil {
		return nil
	}
	unlinkCmd := exec.Command("sudo", "unlink", diskPath)
	unlinkCmd.Stdout = ui.Stdout()
	unlinkCmd.Stderr = ui.Stderr()
	if err := unlinkCmd.Run(); err != nil {
		return fmt.Errorf("removing disk %s: %w", diskPath, err)
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

// MACAddresses returns the lowercase MAC addresses of a domain's network
// interfaces, parsed from 'virsh domiflist'.
func MACAddresses(domainName string) ([]string, error) {
	output, err := exec.Command("sudo", "virsh", "domiflist", domainName).Output()
	if err != nil {
		return nil, fmt.Errorf("listing interfaces of %s: %w — check that libvirtd is running", domainName, err)
	}
	return parseDomainInterfaceMACs(string(output)), nil
}

// parseDomainInterfaceMACs extracts the MAC column from 'virsh domiflist'
// output. Header and separator lines carry no MAC and are skipped.
func parseDomainInterfaceMACs(domiflistOutput string) []string {
	var macAddresses []string
	for _, line := range strings.Split(domiflistOutput, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		lastField := strings.ToLower(fields[len(fields)-1])
		if _, err := net.ParseMAC(lastField); err == nil {
			macAddresses = append(macAddresses, lastField)
		}
	}
	return macAddresses
}

// LeaseAddress returns the IPv4 address libvirt's DHCP server leased to the
// domain, or "" when it has no lease yet (still booting, or not running).
func LeaseAddress(domainName string) (string, error) {
	output, err := exec.Command("sudo", "virsh", "domifaddr", domainName, "--source", "lease").Output()
	if err != nil {
		return "", fmt.Errorf("reading DHCP lease of %s: %w — check that libvirtd is running and the VM exists", domainName, err)
	}
	return parseLeaseIPv4(string(output)), nil
}

// parseLeaseIPv4 returns the first ipv4 address (without prefix length) in
// 'virsh domifaddr' output, or "".
func parseLeaseIPv4(domifaddrOutput string) string {
	for _, line := range strings.Split(domifaddrOutput, "\n") {
		fields := strings.Fields(line)
		if len(fields) < domifaddrMinimumFields || fields[domifaddrProtocolColumn] != "ipv4" {
			continue
		}
		leasedIP, _, err := net.ParseCIDR(fields[domifaddrAddressColumn])
		if err == nil && leasedIP.To4() != nil {
			return leasedIP.String()
		}
	}
	return ""
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
