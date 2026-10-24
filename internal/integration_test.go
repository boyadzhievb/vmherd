//go:build integration

package internal

import (
	"os"
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
	testNetworkName = "vmherd-integration-test"
	testSubnetBase  = "192.168.200"
	testVMIP        = "192.168.200.10"
	testVMName      = "integ-vm"
	testSSHUser     = "vmherd"
	testDomainName  = "vmherd-integration-test-integ-vm"
	testDiskDir     = "/var/lib/libvirt/images"
)

// TestFullVMLifecycle exercises the complete flow:
// pull image → create network → generate cloud-init → create VM → wait for SSH → shutdown → destroy.
func TestFullVMLifecycle(t *testing.T) {
	cleanup(t)
	t.Cleanup(func() { cleanup(t) })

	stateDir := t.TempDir()

	entry, err := image.Resolve("ubuntu:noble")
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
		testSSHUser, strings.TrimSpace(pubKey),
		testSubnetBase, dnsServer,
	)
	if err != nil {
		t.Fatalf("cannot generate cloud-init files: %v", err)
	}
	t.Log("Cloud-init files generated")

	diskPath := filepath.Join(testDiskDir, testDomainName+".qcow2")

	if err := vm.Create(vm.CreateOptions{
		DomainName:         testDomainName,
		DiskPath:           diskPath,
		CloudInitUserData:  ciFiles.UserData,
		CloudInitMetaData:  ciFiles.MetaData,
		CloudInitNetConfig: ciFiles.NetworkConfig,
		NetworkName:        testNetworkName,
		Memory:             vmConfig.Memory,
		CPUs:               vmConfig.CPUs,
		OSVariant:          entry.OSVariant,
		ImagePath:          imagePath,
		DiskSize:           vmConfig.Disk,
	}); err != nil {
		t.Fatalf("cannot create VM: %v", err)
	}
	t.Log("VM created")

	if !vm.Exists(testDomainName) {
		t.Fatal("VM should exist after creation")
	}
	if state := vm.State(testDomainName); state != "running" {
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

	if vm.Exists(testDomainName) {
		t.Error("VM should not exist after destroy")
	}
	t.Log("VM destroyed")

	if err := network.Destroy(testNetworkName); err != nil {
		t.Fatalf("cannot destroy network: %v", err)
	}
	t.Log("Network destroyed")
}

// cleanup removes any leftover test resources from a previous failed run.
func cleanup(t *testing.T) {
	t.Helper()
	if vm.Exists(testDomainName) {
		_ = vm.ForceStop(testDomainName)
		_ = vm.Destroy(testDomainName)
	}
	_ = network.Destroy(testNetworkName)
	diskPath := filepath.Join(testDiskDir, testDomainName+".qcow2")
	_ = os.Remove(diskPath)
}
