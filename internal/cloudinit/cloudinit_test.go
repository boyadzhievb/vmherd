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

	files, err := GenerateFiles(stateDir, vmConfig, "testuser", "", "ssh-ed25519 AAAA testkey", "192.168.122", "8.8.8.8", "52:54:00:aa:bb:cc", nil)
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

	files, err := GenerateFiles(stateDir, vmConfig, "vmherd", "", "ssh-ed25519 AAAA mykey", "10.0.0", "1.1.1.1", "52:54:00:aa:bb:cc", nil)
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

	files, err := GenerateFiles(stateDir, vmConfig, "vmherd", "", "ssh-ed25519 AAAA key", "192.168.122", "8.8.8.8", "52:54:00:aa:bb:cc", nil)
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

	files, err := GenerateFiles(stateDir, vmConfig, "vmherd", "", "ssh-ed25519 AAAA key", "10.0.0", "1.1.1.1", "52:54:00:aa:bb:cc", nil)
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

	files, err := GenerateFiles(stateDir, vmConfig, "vmherd", "", "ssh-ed25519 AAAA key", "10.0.0", "1.1.1.1", "52:54:00:aa:bb:cc", nil)
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

	files, err := GenerateFiles(stateDir, vmConfig, "vmherd", "", "ssh-ed25519 AAAA key", "10.0.0", "1.1.1.1", "52:54:00:aa:bb:cc", nil)
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

	files, err := GenerateFiles(stateDir, vmConfig, "vmherd", "", "ssh-ed25519 AAAA key", "10.0.0", "1.1.1.1", "52:54:00:aa:bb:cc", nil)
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
	if !strings.Contains(content, "permissions: '0644'") {
		t.Error("user-data should set write_files permissions to 0644")
	}
	if strings.Contains(content, "permissions: '0755'") {
		t.Error("user-data should not use overly permissive 0755 for write_files")
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

	_, err := GenerateFiles(stateDir, vmConfig, "vmherd", "", "ssh-ed25519 AAAA key", "10.0.0", "1.1.1.1", "52:54:00:aa:bb:cc", nil)
	if err == nil {
		t.Fatal("expected error for missing source file")
	}
}

// TestGenerateFilesWithSharedMounts verifies that shared mount entries produce runcmd and mounts sections in user-data.
func TestGenerateFilesWithSharedMounts(t *testing.T) {
	stateDir := t.TempDir()
	vmConfig := config.VM{
		Name:   "test-vm",
		Image:  "ubuntu:noble",
		CPUs:   1,
		Memory: 1024,
		Disk:   "10G",
		Network: config.VMNetworkConfig{
			IP: "192.168.122.10",
		},
	}

	sharedMounts := []SharedMount{
		{MountTag: "vmherd_share_0", GuestPath: "/vagrant"},
	}

	files, err := GenerateFiles(stateDir, vmConfig, "vmherd", "", "ssh-ed25519 AAAA key", "192.168.122", "8.8.8.8", "52:54:00:aa:bb:cc", sharedMounts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	userData, err := os.ReadFile(files.UserData)
	if err != nil {
		t.Fatalf("cannot read user-data: %v", err)
	}
	userDataContent := string(userData)

	expectedFragments := []string{
		"runcmd:",
		"mkdir -p /vagrant",
		"mount -t 9p",
		"vmherd_share_0",
		"mounts:",
	}

	for _, fragment := range expectedFragments {
		if !strings.Contains(userDataContent, fragment) {
			t.Errorf("user-data missing expected fragment %q", fragment)
		}
	}
}

// TestGenerateFilesWithCombinedProvisioning verifies that both shell and file provisioning steps coexist in user-data.
func TestGenerateFilesWithCombinedProvisioning(t *testing.T) {
	stateDir := t.TempDir()

	sourceFile := filepath.Join(t.TempDir(), "install.sh")
	sourceFileContent := "#!/bin/bash\necho installed"
	if err := os.WriteFile(sourceFile, []byte(sourceFileContent), 0o644); err != nil {
		t.Fatal(err)
	}

	vmConfig := config.VM{
		Name:   "test-vm",
		Image:  "ubuntu:noble",
		CPUs:   1,
		Memory: 1024,
		Disk:   "10G",
		Network: config.VMNetworkConfig{
			IP: "192.168.122.10",
		},
		Provision: []config.Provision{
			{Type: "shell", Inline: "apt-get update && apt-get install -y curl"},
			{Type: "file", Source: sourceFile, Destination: "/tmp/install.sh"},
		},
	}

	files, err := GenerateFiles(stateDir, vmConfig, "vmherd", "", "ssh-ed25519 AAAA key", "192.168.122", "8.8.8.8", "52:54:00:aa:bb:cc", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	userData, err := os.ReadFile(files.UserData)
	if err != nil {
		t.Fatalf("cannot read user-data: %v", err)
	}
	userDataContent := string(userData)

	expectedFragments := []string{
		"write_files:",
		"path: /tmp/install.sh",
		"encoding: b64",
		"runcmd:",
		"apt-get update && apt-get install -y curl",
	}

	for _, fragment := range expectedFragments {
		if !strings.Contains(userDataContent, fragment) {
			t.Errorf("user-data missing expected fragment %q", fragment)
		}
	}
}

// TestGenerateFilesWithMultipleSharedMounts verifies that multiple shared mount entries each appear in mounts and runcmd.
func TestGenerateFilesWithMultipleSharedMounts(t *testing.T) {
	stateDir := t.TempDir()
	vmConfig := config.VM{
		Name:   "test-vm",
		Image:  "ubuntu:noble",
		CPUs:   1,
		Memory: 1024,
		Disk:   "10G",
		Network: config.VMNetworkConfig{
			IP: "192.168.122.10",
		},
	}

	sharedMounts := []SharedMount{
		{MountTag: "vmherd_share_0", GuestPath: "/vagrant"},
		{MountTag: "vmherd_share_1", GuestPath: "/data"},
	}

	files, err := GenerateFiles(stateDir, vmConfig, "vmherd", "", "ssh-ed25519 AAAA key", "192.168.122", "8.8.8.8", "52:54:00:aa:bb:cc", sharedMounts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	userData, err := os.ReadFile(files.UserData)
	if err != nil {
		t.Fatalf("cannot read user-data: %v", err)
	}
	userDataContent := string(userData)

	expectedFragments := []string{
		"mkdir -p /vagrant",
		"mkdir -p /data",
		"vmherd_share_0",
		"vmherd_share_1",
		"mount -t 9p",
		"mounts:",
	}

	for _, fragment := range expectedFragments {
		if !strings.Contains(userDataContent, fragment) {
			t.Errorf("user-data missing expected fragment %q", fragment)
		}
	}

	// Verify both mount tags appear in the mounts section by counting occurrences of the mount line pattern.
	mountTagZeroCount := strings.Count(userDataContent, "vmherd_share_0")
	mountTagOneCount := strings.Count(userDataContent, "vmherd_share_1")
	if mountTagZeroCount < 2 {
		t.Errorf("expected vmherd_share_0 to appear in both runcmd and mounts, found %d occurrences", mountTagZeroCount)
	}
	if mountTagOneCount < 2 {
		t.Errorf("expected vmherd_share_1 to appear in both runcmd and mounts, found %d occurrences", mountTagOneCount)
	}
}

// TestGenerateFilesWithSharedMountsAndProvisioning verifies that provisioning steps and shared mounts coexist correctly.
func TestGenerateFilesWithSharedMountsAndProvisioning(t *testing.T) {
	stateDir := t.TempDir()

	sourceFile := filepath.Join(t.TempDir(), "config.sh")
	sourceFileContent := "#!/bin/bash\necho configured"
	if err := os.WriteFile(sourceFile, []byte(sourceFileContent), 0o644); err != nil {
		t.Fatal(err)
	}

	vmConfig := config.VM{
		Name:   "test-vm",
		Image:  "ubuntu:noble",
		CPUs:   1,
		Memory: 1024,
		Disk:   "10G",
		Network: config.VMNetworkConfig{
			IP: "192.168.122.10",
		},
		Provision: []config.Provision{
			{Type: "shell", Inline: "apt-get update"},
			{Type: "file", Source: sourceFile, Destination: "/tmp/config.sh"},
		},
	}

	sharedMounts := []SharedMount{
		{MountTag: "vmherd_share_0", GuestPath: "/vagrant"},
	}

	files, err := GenerateFiles(stateDir, vmConfig, "vmherd", "", "ssh-ed25519 AAAA key", "192.168.122", "8.8.8.8", "52:54:00:aa:bb:cc", sharedMounts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	userData, err := os.ReadFile(files.UserData)
	if err != nil {
		t.Fatalf("cannot read user-data: %v", err)
	}
	userDataContent := string(userData)

	expectedFragments := []string{
		// File provisioning produces write_files section.
		"write_files:",
		"path: /tmp/config.sh",
		"encoding: b64",
		// Shell provisioning and shared mount commands share the runcmd section.
		"runcmd:",
		"apt-get update",
		"mkdir -p /vagrant",
		"mount -t 9p",
		"vmherd_share_0",
		// Shared mounts produce the mounts section.
		"mounts:",
	}

	for _, fragment := range expectedFragments {
		if !strings.Contains(userDataContent, fragment) {
			t.Errorf("user-data missing expected fragment %q", fragment)
		}
	}

	// Verify runcmd appears exactly once (provisioning and mounts share one section).
	runcmdCount := strings.Count(userDataContent, "runcmd:")
	if runcmdCount != 1 {
		t.Errorf("expected exactly 1 runcmd section, found %d", runcmdCount)
	}
}

// TestMultiLineInlineCommandWrappedInBash verifies that multi-line shell commands
// are wrapped in a bash -c invocation instead of being split into separate runcmd lines.
func TestMultiLineInlineCommandWrappedInBash(t *testing.T) {
	stateDir := t.TempDir()
	multiLineScript := "if true; then\n  echo yes\nfi"
	vmConfig := config.VM{
		Name:  "multiline-vm",
		Image: "ubuntu:noble",
		Network: config.VMNetworkConfig{
			IP: "10.0.0.5",
		},
		Provision: []config.Provision{
			{Type: "shell", Inline: multiLineScript},
		},
	}

	files, err := GenerateFiles(stateDir, vmConfig, "vmherd", "", "ssh-ed25519 AAAA key", "10.0.0", "1.1.1.1", "52:54:00:aa:bb:cc", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(files.UserData)
	if err != nil {
		t.Fatalf("cannot read user-data: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, `["/bin/bash", "-c",`) {
		t.Error("multi-line command should be wrapped in bash -c invocation")
	}

	// The individual lines of the script must NOT appear as separate runcmd entries.
	lines := strings.Split(content, "\n")
	separateIfLine := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "- if true; then" {
			separateIfLine = true
		}
	}
	if separateIfLine {
		t.Error("multi-line command was split into separate runcmd entries instead of being wrapped")
	}
}

// TestSingleLineCommandWrappedInBash verifies that single-line shell commands
// are wrapped in bash -c to prevent YAML injection from special characters.
func TestSingleLineCommandWrappedInBash(t *testing.T) {
	stateDir := t.TempDir()
	vmConfig := config.VM{
		Name:  "singleline-vm",
		Image: "ubuntu:noble",
		Network: config.VMNetworkConfig{
			IP: "10.0.0.5",
		},
		Provision: []config.Provision{
			{Type: "shell", Inline: "apt-get update"},
		},
	}

	files, err := GenerateFiles(stateDir, vmConfig, "vmherd", "", "ssh-ed25519 AAAA key", "10.0.0", "1.1.1.1", "52:54:00:aa:bb:cc", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(files.UserData)
	if err != nil {
		t.Fatalf("cannot read user-data: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, `["/bin/bash", "-c",`) {
		t.Error("single-line command should be wrapped in bash -c invocation")
	}
	if !strings.Contains(content, "apt-get update") {
		t.Error("single-line command content should be present")
	}
}

// TestYAMLSpecialCharactersInCommand verifies that commands containing YAML-special
// characters (colon-space, hash, brackets) are safely quoted via bash -c wrapping.
func TestYAMLSpecialCharactersInCommand(t *testing.T) {
	testCases := []struct {
		name    string
		command string
	}{
		{name: "colon space", command: "echo hello: world"},
		{name: "hash comment", command: "echo hello # this is not a comment"},
		{name: "square bracket", command: "echo [array]"},
		{name: "curly brace", command: "echo {mapping}"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			stateDir := t.TempDir()
			vmConfig := config.VM{
				Name:  "yaml-test",
				Image: "ubuntu:noble",
				Network: config.VMNetworkConfig{
					IP: "10.0.0.5",
				},
				Provision: []config.Provision{
					{Type: "shell", Inline: tc.command},
				},
			}

			files, err := GenerateFiles(stateDir, vmConfig, "vmherd", "", "ssh-ed25519 AAAA key", "10.0.0", "1.1.1.1", "52:54:00:aa:bb:cc", nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			data, err := os.ReadFile(files.UserData)
			if err != nil {
				t.Fatalf("cannot read user-data: %v", err)
			}
			content := string(data)

			if !strings.Contains(content, `["/bin/bash", "-c",`) {
				t.Errorf("command with %s should be wrapped in bash -c", tc.name)
			}

			// The command must NOT appear as a bare YAML string (  - <command>).
			bareEntry := "  - " + tc.command + "\n"
			if strings.Contains(content, bareEntry) {
				t.Errorf("command with %s should not appear as bare YAML string", tc.name)
			}
		})
	}
}

// TestCloudInitFilePermissions verifies that generated cloud-init files are
// created with restrictive permissions (0600) since they contain sensitive data.
func TestCloudInitFilePermissions(t *testing.T) {
	stateDir := t.TempDir()
	vmConfig := config.VM{
		Name:  "perms-vm",
		Image: "ubuntu:noble",
		Network: config.VMNetworkConfig{
			IP: "10.0.0.5",
		},
	}

	files, err := GenerateFiles(stateDir, vmConfig, "vmherd", "", "ssh-ed25519 AAAA key", "10.0.0", "1.1.1.1", "52:54:00:aa:bb:cc", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedPermission := os.FileMode(0o600)
	filesToCheck := map[string]string{
		"user-data":      files.UserData,
		"meta-data":      files.MetaData,
		"network-config": files.NetworkConfig,
	}

	for name, path := range filesToCheck {
		fileInfo, err := os.Stat(path)
		if err != nil {
			t.Fatalf("cannot stat %s at %s: %v", name, path, err)
		}
		actualPermission := fileInfo.Mode().Perm()
		if actualPermission != expectedPermission {
			t.Errorf("%s has permission %o, expected %o", name, actualPermission, expectedPermission)
		}
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

	files, err := GenerateFiles(stateDir, vmConfig, "vmherd", "", "ssh-ed25519 AAAA key", "10.0.0", "1.1.1.1", "52:54:00:aa:bb:cc", nil)
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
