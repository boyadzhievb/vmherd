//go:build integration

package internal

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/boyadzhievb/vmherd/internal/cloudinit"
	"github.com/boyadzhievb/vmherd/internal/config"
	"github.com/boyadzhievb/vmherd/internal/image"
	"github.com/boyadzhievb/vmherd/internal/network"
	"github.com/boyadzhievb/vmherd/internal/ssh"
	"github.com/boyadzhievb/vmherd/internal/vm"
)

const (
	testNetworkName = "vmherd-itest"
	testSubnetBase  = "192.168.200"
	testVMIP        = "192.168.200.10"
	testVMName      = "integ-vm"
	testSSHUser     = "vmherd"
	testDomainName  = "vmherd-itest-integ-vm"
	testDiskDir     = "/var/lib/libvirt/images"
)

// TestFullVMLifecycle exercises the complete flow:
// pull image → create network → generate cloud-init → create VM → wait for SSH → shutdown → destroy.
func TestFullVMLifecycle(t *testing.T) {
	cleanup(t)
	t.Cleanup(func() { cleanup(t) })

	stateDir := t.TempDir()

	testImages := config.BuiltInImageURLs()
	entry, err := image.Resolve("ubuntu:noble", testImages)
	if err != nil {
		t.Fatalf("cannot resolve image: %v", err)
	}

	imagePath, err := image.EnsureCached(entry)
	if err != nil {
		t.Fatalf("cannot download image: %v", err)
	}
	t.Logf("Image cached at %s", imagePath)

	if err := network.Ensure(stateDir, testNetworkName, testSubnetBase); err != nil {
		t.Fatalf("cannot create network: %v", err)
	}
	t.Log("Network created")

	keyPath, err := ssh.EnsureKeyPair(stateDir, "vmherd-integration-test")
	if err != nil {
		t.Fatalf("cannot generate SSH key: %v", err)
	}

	pubKey, err := ssh.ReadPublicKey(keyPath)
	if err != nil {
		t.Fatalf("cannot read public key: %v", err)
	}

	dnsServer := vm.ResolveDNS("auto", testSubnetBase+".1")

	vmConfig := config.VM{
		Name:   testVMName,
		Image:  "ubuntu:noble",
		CPUs:   1,
		Memory: 1024,
		Disk:   "5G",
		Network: config.VMNetworkConfig{
			IP: testVMIP,
		},
	}

	ciFiles, err := cloudinit.GenerateFiles(
		stateDir, vmConfig,
		testSSHUser, "", strings.TrimSpace(pubKey),
		testSubnetBase, dnsServer, "52:54:00:aa:bb:cc", nil, 0,
	)
	if err != nil {
		t.Fatalf("cannot generate cloud-init files: %v", err)
	}
	t.Log("Cloud-init files generated")

	diskPath := filepath.Join(testDiskDir, testDomainName+".qcow2")

	testMACAddress := "52:54:00:aa:bb:cc"

	seedPath := filepath.Join(testDiskDir, testDomainName+"-seed.iso")
	if err := cloudinit.BuildAndInstallSeed(ciFiles, seedPath); err != nil {
		t.Fatalf("cannot build seed ISO: %v", err)
	}

	if err := vm.Create(vm.CreateOptions{
		VMName:      vmConfig.Name,
		DomainName:  testDomainName,
		DiskPath:    diskPath,
		SeedPath:    seedPath,
		NetworkName: testNetworkName,
		MACAddress:  testMACAddress,
		Memory:      vmConfig.Memory,
		CPUs:        vmConfig.CPUs,
		ImagePath:   imagePath,
		DiskSize:    vmConfig.Disk,
	}); err != nil {
		t.Fatalf("cannot create VM: %v", err)
	}
	t.Log("VM created")

	state, err := vm.State(testDomainName)
	if err != nil {
		t.Fatalf("cannot query VM state: %v", err)
	}
	if state != vm.StateRunning {
		t.Fatalf("expected VM state 'running', got %q", state)
	}

	t.Log("Waiting for SSH...")
	if err := ssh.WaitForSSH(keyPath, testSSHUser, testVMIP, testVMName); err != nil {
		t.Fatalf("SSH not reachable: %v", err)
	}
	t.Log("SSH ready")

	if err := vm.Shutdown(testDomainName); err != nil {
		t.Fatalf("cannot shut down VM: %v", err)
	}
	t.Log("Shutdown signal sent")

	if err := vm.Destroy(testDomainName); err != nil {
		t.Fatalf("cannot destroy VM: %v", err)
	}
	// Destroy keeps the disk (no --remove-all-storage since M1.17), as the
	// CLI does; remove it the same way 'vmherd destroy' does.
	if err := vm.RemoveDisk(diskPath); err != nil {
		t.Fatalf("cannot remove disk: %v", err)
	}

	if state, err := vm.State(testDomainName); err != nil || state != vm.StateNotCreated {
		t.Errorf("VM should not exist after destroy: state %q, error %v", state, err)
	}
	t.Log("VM destroyed")

	if err := network.Destroy(testNetworkName); err != nil {
		t.Fatalf("cannot destroy network: %v", err)
	}
	t.Log("Network destroyed")
}

// cleanup removes any leftover test resources from a previous failed run:
// domain, network, disk and seed ISO. The disk and seed are root-owned, so
// they go through the sudo helpers; missing files are not an error.
func cleanup(t *testing.T) {
	t.Helper()
	if state, err := vm.State(testDomainName); err == nil && state != vm.StateNotCreated {
		// Destroy forces an active domain off before undefining it.
		if err := vm.Destroy(testDomainName); err != nil {
			t.Logf("cleanup: %v", err)
		}
	}
	if err := network.Destroy(testNetworkName); err != nil {
		t.Logf("cleanup: network %s: %v", testNetworkName, err)
	}
	if err := vm.RemoveDisk(filepath.Join(testDiskDir, testDomainName+".qcow2")); err != nil {
		t.Logf("cleanup: %v", err)
	}
	if err := cloudinit.RemoveSeed(filepath.Join(testDiskDir, testDomainName+"-seed.iso")); err != nil {
		t.Logf("cleanup: %v", err)
	}
}
