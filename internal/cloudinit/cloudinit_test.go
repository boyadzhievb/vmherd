package cloudinit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/boyadzhievb/vmherd/internal/config"
)

func TestGenerateFilesCreatesAllFiles(t *testing.T) {
	stateDir := t.TempDir()
	vmConfig := config.VM{
		Name:  "test-vm",
		Image: "ubuntu:noble",
		Network: config.VMNetworkConfig{
			IP: "192.168.122.10",
		},
	}

	files, err := GenerateFiles(stateDir, vmConfig, "testuser", "ssh-ed25519 AAAA testkey", "192.168.122", "8.8.8.8", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, path := range []string{files.UserData, files.MetaData, files.NetworkConfig} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("expected file %s to exist: %v", path, err)
		}
	}
}

func TestGenerateFilesUserDataContent(t *testing.T) {
	stateDir := t.TempDir()
	vmConfig := config.VM{
		Name:  "myvm",
		Image: "ubuntu:noble",
		Network: config.VMNetworkConfig{
			IP: "10.0.0.5",
		},
	}

	files, err := GenerateFiles(stateDir, vmConfig, "vmherd", "ssh-ed25519 AAAA mykey", "10.0.0", "1.1.1.1", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(files.UserData)
	if err != nil {
		t.Fatalf("cannot read user-data: %v", err)
	}
	content := string(data)

	expectedFragments := []string{
		"#cloud-config",
		"hostname: myvm",
		"name: vmherd",
		"ssh-ed25519 AAAA mykey",
		"ssh_pwauth: false",
	}

	for _, fragment := range expectedFragments {
		if !strings.Contains(content, fragment) {
			t.Errorf("user-data missing %q", fragment)
		}
	}
}

func TestGenerateFilesNetworkConfigContent(t *testing.T) {
	stateDir := t.TempDir()
	vmConfig := config.VM{
		Name:  "netvm",
		Image: "ubuntu:noble",
		Network: config.VMNetworkConfig{
			IP: "192.168.122.50",
		},
	}

	files, err := GenerateFiles(stateDir, vmConfig, "vmherd", "ssh-ed25519 AAAA key", "192.168.122", "8.8.8.8", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(files.NetworkConfig)
	if err != nil {
		t.Fatalf("cannot read network-config: %v", err)
	}
	content := string(data)

	expectedFragments := []string{
		"version: 2",
		"192.168.122.50/24",
		"via: 192.168.122.1",
		"- 8.8.8.8",
	}

	for _, fragment := range expectedFragments {
		if !strings.Contains(content, fragment) {
			t.Errorf("network-config missing %q", fragment)
		}
	}
}

func TestGenerateFilesCustomGateway(t *testing.T) {
	stateDir := t.TempDir()
	vmConfig := config.VM{
		Name:  "gwvm",
		Image: "ubuntu:noble",
		Network: config.VMNetworkConfig{
			IP:      "10.0.0.5",
			Gateway: "10.0.0.254",
		},
	}

	files, err := GenerateFiles(stateDir, vmConfig, "vmherd", "ssh-ed25519 AAAA key", "10.0.0", "1.1.1.1", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(files.NetworkConfig)
	if err != nil {
		t.Fatalf("cannot read network-config: %v", err)
	}

	if !strings.Contains(string(data), "via: 10.0.0.254") {
		t.Error("network-config should use custom gateway 10.0.0.254")
	}
}

func TestGenerateFilesMetaDataContent(t *testing.T) {
	stateDir := t.TempDir()
	vmConfig := config.VM{
		Name:  "metavm",
		Image: "ubuntu:noble",
		Network: config.VMNetworkConfig{
			IP: "10.0.0.5",
		},
	}

	files, err := GenerateFiles(stateDir, vmConfig, "vmherd", "ssh-ed25519 AAAA key", "10.0.0", "1.1.1.1", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(files.MetaData)
	if err != nil {
		t.Fatalf("cannot read meta-data: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "instance-id: metavm") {
		t.Error("meta-data missing instance-id")
	}
	if !strings.Contains(content, "local-hostname: metavm") {
		t.Error("meta-data missing local-hostname")
	}
}

func TestGenerateFilesWithShellProvisioning(t *testing.T) {
	stateDir := t.TempDir()
	vmConfig := config.VM{
		Name:  "provvm",
		Image: "ubuntu:noble",
		Network: config.VMNetworkConfig{
			IP: "10.0.0.5",
		},
		Provision: []config.Provision{
			{Type: "shell", Inline: "apt-get update"},
			{Type: "shell", Inline: "apt-get install -y curl"},
		},
	}

	files, err := GenerateFiles(stateDir, vmConfig, "vmherd", "ssh-ed25519 AAAA key", "10.0.0", "1.1.1.1", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(files.UserData)
	if err != nil {
		t.Fatalf("cannot read user-data: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "runcmd:") {
		t.Error("user-data missing runcmd section")
	}
	if !strings.Contains(content, "apt-get update") {
		t.Error("user-data missing first runcmd")
	}
	if !strings.Contains(content, "apt-get install -y curl") {
		t.Error("user-data missing second runcmd")
	}
}

func TestGenerateFilesWithFileProvisioning(t *testing.T) {
	stateDir := t.TempDir()

	sourceFile := filepath.Join(t.TempDir(), "setup.sh")
	if err := os.WriteFile(sourceFile, []byte("#!/bin/bash\necho hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	vmConfig := config.VM{
		Name:  "filevm",
		Image: "ubuntu:noble",
		Network: config.VMNetworkConfig{
			IP: "10.0.0.5",
		},
		Provision: []config.Provision{
			{Type: "file", Source: sourceFile, Destination: "/tmp/setup.sh"},
		},
	}

	files, err := GenerateFiles(stateDir, vmConfig, "vmherd", "ssh-ed25519 AAAA key", "10.0.0", "1.1.1.1", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(files.UserData)
	if err != nil {
		t.Fatalf("cannot read user-data: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "write_files:") {
		t.Error("user-data missing write_files section")
	}
	if !strings.Contains(content, "path: /tmp/setup.sh") {
		t.Error("user-data missing file destination")
	}
	if !strings.Contains(content, "encoding: b64") {
		t.Error("user-data missing base64 encoding marker")
	}
}

func TestGenerateFilesWithMissingSourceFile(t *testing.T) {
	stateDir := t.TempDir()
	vmConfig := config.VM{
		Name:  "badvm",
		Image: "ubuntu:noble",
		Network: config.VMNetworkConfig{
			IP: "10.0.0.5",
		},
		Provision: []config.Provision{
			{Type: "file", Source: "/nonexistent/file.sh", Destination: "/tmp/file.sh"},
		},
	}

	_, err := GenerateFiles(stateDir, vmConfig, "vmherd", "ssh-ed25519 AAAA key", "10.0.0", "1.1.1.1", nil)
	if err == nil {
		t.Fatal("expected error for missing source file")
	}
}

func TestGenerateFilesPathsAreCorrect(t *testing.T) {
	stateDir := t.TempDir()
	vmConfig := config.VM{
		Name:  "pathvm",
		Image: "ubuntu:noble",
		Network: config.VMNetworkConfig{
			IP: "10.0.0.5",
		},
	}

	files, err := GenerateFiles(stateDir, vmConfig, "vmherd", "ssh-ed25519 AAAA key", "10.0.0", "1.1.1.1", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedDir := filepath.Join(stateDir, "pathvm")
	if files.UserData != filepath.Join(expectedDir, "user-data") {
		t.Errorf("unexpected user-data path: %s", files.UserData)
	}
	if files.MetaData != filepath.Join(expectedDir, "meta-data") {
		t.Errorf("unexpected meta-data path: %s", files.MetaData)
	}
	if files.NetworkConfig != filepath.Join(expectedDir, "network-config") {
		t.Errorf("unexpected network-config path: %s", files.NetworkConfig)
	}
}
