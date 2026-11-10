package cloudinit

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/boyadzhievb/vmherd/internal/config"
	"gopkg.in/yaml.v3"
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

	files, err := GenerateFiles(stateDir, vmConfig, "testuser", "", "ssh-ed25519 AAAA testkey", "192.168.122.1", "8.8.8.8", "52:54:00:aa:bb:cc", nil, 0)
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

	files, err := GenerateFiles(stateDir, vmConfig, "vmherd", "", "ssh-ed25519 AAAA mykey", "10.0.0.1", "1.1.1.1", "52:54:00:aa:bb:cc", nil, 0)
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

	files, err := GenerateFiles(stateDir, vmConfig, "vmherd", "", "ssh-ed25519 AAAA key", "192.168.122.1", "8.8.8.8", "52:54:00:aa:bb:cc", nil, 0)
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
		"to: 0.0.0.0/0",
		"via: 192.168.122.1",
		"- 8.8.8.8",
	}
	// cloud-init's own v2 parser (EL, no netplan) rejects netplan's "default".
	if strings.Contains(content, "to: default") {
		t.Errorf("network-config uses netplan-only 'to: default':\n%s", content)
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

	files, err := GenerateFiles(stateDir, vmConfig, "vmherd", "", "ssh-ed25519 AAAA key", "10.0.0.1", "1.1.1.1", "52:54:00:aa:bb:cc", nil, 0)
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

	files, err := GenerateFiles(stateDir, vmConfig, "vmherd", "", "ssh-ed25519 AAAA key", "10.0.0.1", "1.1.1.1", "52:54:00:aa:bb:cc", nil, 0)
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

	files, err := GenerateFiles(stateDir, vmConfig, "vmherd", "", "ssh-ed25519 AAAA key", "10.0.0.1", "1.1.1.1", "52:54:00:aa:bb:cc", nil, 0)
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

	files, err := GenerateFiles(stateDir, vmConfig, "vmherd", "", "ssh-ed25519 AAAA key", "10.0.0.1", "1.1.1.1", "52:54:00:aa:bb:cc", nil, 0)
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
	if writeFiles := readWriteFiles(t, files.UserData); len(writeFiles) != 1 || writeFiles[0].Permissions != "0644" {
		t.Errorf("write_files permissions must be the string 0644, got %+v", writeFiles)
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

	_, err := GenerateFiles(stateDir, vmConfig, "vmherd", "", "ssh-ed25519 AAAA key", "10.0.0.1", "1.1.1.1", "52:54:00:aa:bb:cc", nil, 0)
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

	files, err := GenerateFiles(stateDir, vmConfig, "vmherd", "", "ssh-ed25519 AAAA key", "192.168.122.1", "8.8.8.8", "52:54:00:aa:bb:cc", sharedMounts, 1001)
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
		"mounts:",
		"  - name: vmherd\n    uid: 1001\n",
	}

	for _, fragment := range expectedFragments {
		if !strings.Contains(userDataContent, fragment) {
			t.Errorf("user-data missing expected fragment %q", fragment)
		}
	}
	assertRuncmdHasLines(t, files.UserData,
		bashCommandLine("mkdir -p '/vagrant'"),
		bashCommandLine("mountpoint -q '/vagrant' || mount -t 9p -o trans=virtio,version=9p2000.L,noatime,nofail vmherd_share_0 '/vagrant'"))
	wantMount := []string{"vmherd_share_0", "/vagrant", "9p", "trans=virtio,version=9p2000.L,noatime,nofail", "0", "0"}
	if mounts := readMounts(t, files.UserData); len(mounts) != 1 || !slices.Equal(mounts[0], wantMount) {
		t.Errorf("mounts = %q, want [%q]", mounts, wantMount)
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

	files, err := GenerateFiles(stateDir, vmConfig, "vmherd", "", "ssh-ed25519 AAAA key", "192.168.122.1", "8.8.8.8", "52:54:00:aa:bb:cc", nil, 0)
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

	files, err := GenerateFiles(stateDir, vmConfig, "vmherd", "", "ssh-ed25519 AAAA key", "192.168.122.1", "8.8.8.8", "52:54:00:aa:bb:cc", sharedMounts, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	userData, err := os.ReadFile(files.UserData)
	if err != nil {
		t.Fatalf("cannot read user-data: %v", err)
	}
	userDataContent := string(userData)

	assertRuncmdHasLines(t, files.UserData,
		bashCommandLine("mkdir -p '/vagrant'"),
		bashCommandLine("mkdir -p '/data'"))

	expectedFragments := []string{
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

	files, err := GenerateFiles(stateDir, vmConfig, "vmherd", "", "ssh-ed25519 AAAA key", "192.168.122.1", "8.8.8.8", "52:54:00:aa:bb:cc", sharedMounts, 0)
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

	// The home fix runs before provisioning (which may use ~), the share
	// fallback mounts after it, and the failed-step check last.
	wantRuncmdLines := []string{
		"vmherd_failed_step=",
		bashCommandLine(`chown vmherd: "$(getent passwd vmherd | cut -d: -f6)"`),
		guardedStepLine("1 of 2", shellStep("apt-get update")),
		bashCommandLine(ninePSupportCheckCommand()),
		bashCommandLine("mkdir -p '/vagrant'"),
		bashCommandLine("mountpoint -q '/vagrant' || mount -t 9p -o trans=virtio,version=9p2000.L,noatime,nofail vmherd_share_0 '/vagrant'"),
		failedStepCheckLine(),
	}
	if gotRuncmdLines := readRuncmdLines(t, files.UserData); !slices.Equal(gotRuncmdLines, wantRuncmdLines) {
		t.Errorf("runcmd lines =\n%s\nwant\n%s", strings.Join(gotRuncmdLines, "\n"), strings.Join(wantRuncmdLines, "\n"))
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

	files, err := GenerateFiles(stateDir, vmConfig, "vmherd", "", "ssh-ed25519 AAAA key", "10.0.0.1", "1.1.1.1", "52:54:00:aa:bb:cc", nil, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(files.UserData)
	if err != nil {
		t.Fatalf("cannot read user-data: %v", err)
	}
	content := string(data)

	// The whole script is one runcmd entry run by bash, not one entry per line.
	assertRuncmdHasLines(t, files.UserData, guardedStepLine("1 of 1", shellStep(multiLineScript)))

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

	files, err := GenerateFiles(stateDir, vmConfig, "vmherd", "", "ssh-ed25519 AAAA key", "10.0.0.1", "1.1.1.1", "52:54:00:aa:bb:cc", nil, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(files.UserData)
	if err != nil {
		t.Fatalf("cannot read user-data: %v", err)
	}
	content := string(data)

	assertRuncmdHasLines(t, files.UserData, guardedStepLine("1 of 1", shellStep("apt-get update")))
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

			files, err := GenerateFiles(stateDir, vmConfig, "vmherd", "", "ssh-ed25519 AAAA key", "10.0.0.1", "1.1.1.1", "52:54:00:aa:bb:cc", nil, 0)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			data, err := os.ReadFile(files.UserData)
			if err != nil {
				t.Fatalf("cannot read user-data: %v", err)
			}
			content := string(data)

			// Decoding user-data must give back exactly the guarded line, so the
			// YAML-special characters stayed inside the entry.
			assertRuncmdHasLines(t, files.UserData, guardedStepLine("1 of 1", shellStep(tc.command)))

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

	files, err := GenerateFiles(stateDir, vmConfig, "vmherd", "", "ssh-ed25519 AAAA key", "10.0.0.1", "1.1.1.1", "52:54:00:aa:bb:cc", nil, 0)
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

	files, err := GenerateFiles(stateDir, vmConfig, "vmherd", "", "ssh-ed25519 AAAA key", "10.0.0.1", "1.1.1.1", "52:54:00:aa:bb:cc", nil, 0)
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

// TestGenerateFilesWithoutUIDLeavesDefault verifies that sshUserUID 0 writes no uid line,
// so the guest allocates the SSH user's UID itself.
func TestGenerateFilesWithoutUIDLeavesDefault(t *testing.T) {
	vmConfig := config.VM{Name: "web", Image: "ubuntu:noble", CPUs: 1, Memory: 1024, Disk: "10G",
		Network: config.VMNetworkConfig{IP: "192.168.122.10"}}
	files, err := GenerateFiles(t.TempDir(), vmConfig, "vmherd", "", "ssh-ed25519 AAAA key", "192.168.122.1", "8.8.8.8", "52:54:00:aa:bb:cc", nil, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	userData, err := os.ReadFile(files.UserData)
	if err != nil {
		t.Fatalf("cannot read user-data: %v", err)
	}
	if strings.Contains(string(userData), "uid:") {
		t.Errorf("expected no uid line when sshUserUID is 0, got:\n%s", userData)
	}
}

func TestReadSavedMACAddressRoundTrip(t *testing.T) {
	stateDir := t.TempDir()
	seedDir := filepath.Join(stateDir, "ctrl")
	if err := os.MkdirAll(seedDir, 0o700); err != nil {
		t.Fatalf("creating seed dir: %v", err)
	}
	if err := writeNetworkConfig(seedDir, "192.168.122.10", "192.168.122.1", "192.168.122.1", "52:54:00:AB:cd:01"); err != nil {
		t.Fatalf("writing network-config: %v", err)
	}

	macAddress, err := ReadSavedMACAddress(stateDir, "ctrl")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if macAddress != "52:54:00:ab:cd:01" {
		t.Errorf("expected 52:54:00:ab:cd:01, got %q", macAddress)
	}
}

func TestReadSavedMACAddressMissing(t *testing.T) {
	macAddress, err := ReadSavedMACAddress(t.TempDir(), "ctrl")
	if err != nil {
		t.Fatalf("missing network-config should not be an error: %v", err)
	}
	if macAddress != "" {
		t.Errorf("expected empty MAC, got %q", macAddress)
	}
}

func TestReadSavedMACAddressCorrupt(t *testing.T) {
	stateDir := t.TempDir()
	seedDir := filepath.Join(stateDir, "ctrl")
	if err := os.MkdirAll(seedDir, 0o700); err != nil {
		t.Fatalf("creating seed dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(seedDir, "network-config"), []byte("ethernets: [unclosed"), 0o600); err != nil {
		t.Fatalf("writing corrupt network-config: %v", err)
	}

	if _, err := ReadSavedMACAddress(stateDir, "ctrl"); err == nil {
		t.Error("expected an error for corrupt network-config")
	}
}

func TestGenerateFilesNetworkConfigDHCP(t *testing.T) {
	stateDir := t.TempDir()
	vmConfig := config.VM{Name: "dhcpvm", Image: "ubuntu:noble"}

	files, err := GenerateFiles(stateDir, vmConfig, "vmherd", "", "ssh-ed25519 AAAA key", "192.168.150", "192.168.150.1", "52:54:00:aa:bb:cc", nil, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, err := os.ReadFile(files.NetworkConfig)
	if err != nil {
		t.Fatalf("cannot read network-config: %v", err)
	}
	content := string(data)

	for _, fragment := range []string{"dhcp4: true", "dhcp-identifier: mac", "use-dns: false", `macaddress: "52:54:00:aa:bb:cc"`, "- 192.168.150.1"} {
		if !strings.Contains(content, fragment) {
			t.Errorf("DHCP network-config missing %q:\n%s", fragment, content)
		}
	}
	for _, forbidden := range []string{"/24", "routes:", "dhcp4: false"} {
		if strings.Contains(content, forbidden) {
			t.Errorf("DHCP network-config must not contain %q:\n%s", forbidden, content)
		}
	}

	var parsed map[string]any
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		t.Errorf("DHCP network-config is not valid YAML: %v", err)
	}
	macAddress, err := ReadSavedMACAddress(stateDir, "dhcpvm")
	if err != nil || macAddress != "52:54:00:aa:bb:cc" {
		t.Errorf("ReadSavedMACAddress = %q, %v; want the MAC from the DHCP config", macAddress, err)
	}
}

func TestGenerateFilesWithoutSharesHasNoHomeFixOrRuncmd(t *testing.T) {
	stateDir := t.TempDir()
	files, err := GenerateFiles(stateDir, config.VM{Name: "plain", Image: "ubuntu:noble"}, "vmherd", "", "ssh-ed25519 AAAA key", "192.168.150", "192.168.150.1", "52:54:00:aa:bb:cc", nil, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	userData, err := os.ReadFile(files.UserData)
	if err != nil {
		t.Fatalf("cannot read user-data: %v", err)
	}
	for _, forbidden := range []string{"runcmd:", "chown", "mounts:"} {
		if strings.Contains(string(userData), forbidden) {
			t.Errorf("user-data without shares or provisioning must not contain %q:\n%s", forbidden, userData)
		}
	}
}

func TestSharedMountRuncmdsQuoteGuestPath(t *testing.T) {
	runcmds := sharedMountRuncmds([]SharedMount{{MountTag: "vmherd_share_0", GuestPath: "/home/me/my app's"}})
	wantMount := `mountpoint -q '/home/me/my app'\''s' || mount -t 9p -o ` + sharedMountOptions + ` vmherd_share_0 '/home/me/my app'\''s'`
	if len(runcmds) != 3 || runcmds[0] != ninePSupportCheckCommand() || runcmds[1] != `mkdir -p '/home/me/my app'\''s'` || runcmds[2] != wantMount {
		t.Errorf("sharedMountRuncmds() = %q", runcmds)
	}
}

// decodedUserData holds the user-data sections the tests inspect.
type decodedUserData struct {
	Runcmd     []string         `yaml:"runcmd"`      // runcmd script lines
	Mounts     [][]string       `yaml:"mounts"`      // fstab-style mount entries
	WriteFiles []writeFileEntry `yaml:"write_files"` // file provisioning entries
}

// decodeUserData parses the user-data file as YAML.
func decodeUserData(t *testing.T, userDataPath string) decodedUserData {
	t.Helper()
	userData, err := os.ReadFile(userDataPath)
	if err != nil {
		t.Fatalf("cannot read user-data: %v", err)
	}
	var cloudConfig decodedUserData
	if err := yaml.Unmarshal(userData, &cloudConfig); err != nil {
		t.Fatalf("user-data is not valid YAML: %v\n%s", err, userData)
	}
	return cloudConfig
}

// readRuncmdLines decodes the user-data file and returns its runcmd entries.
func readRuncmdLines(t *testing.T, userDataPath string) []string {
	t.Helper()
	return decodeUserData(t, userDataPath).Runcmd
}

// readMounts decodes the user-data file and returns its mounts entries.
func readMounts(t *testing.T, userDataPath string) [][]string {
	t.Helper()
	return decodeUserData(t, userDataPath).Mounts
}

// readWriteFiles decodes the user-data file and returns its write_files entries.
func readWriteFiles(t *testing.T, userDataPath string) []writeFileEntry {
	t.Helper()
	return decodeUserData(t, userDataPath).WriteFiles
}

// assertRuncmdHasLines fails the test unless every wanted line is one of the
// user-data's runcmd entries, exactly.
func assertRuncmdHasLines(t *testing.T, userDataPath string, wantedLines ...string) {
	t.Helper()
	runcmdLines := readRuncmdLines(t, userDataPath)
	for _, wantedLine := range wantedLines {
		if !slices.Contains(runcmdLines, wantedLine) {
			t.Errorf("runcmd has no entry %q; entries:\n%s", wantedLine, strings.Join(runcmdLines, "\n"))
		}
	}
}

// TestYAMLSpecialCharactersInPaths checks that guest paths and destinations
// with YAML-special characters decode back unchanged (they used to be written
// with a raw %s, so ", " or "]" broke the mounts and write_files entries).
func TestYAMLSpecialCharactersInPaths(t *testing.T) {
	trickyPath := "/srv/a, b]: #c 'd\" e"
	sourceFile := filepath.Join(t.TempDir(), "setup.sh")
	if err := os.WriteFile(sourceFile, []byte("echo hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	vmConfig := config.VM{
		Name:      "tricky",
		Image:     "ubuntu:noble",
		Network:   config.VMNetworkConfig{IP: "10.0.0.5"},
		Provision: []config.Provision{{Type: config.ProvisionFile, Source: sourceFile, Destination: trickyPath}},
	}
	sharedMounts := []SharedMount{{MountTag: "vmherd_share_0", GuestPath: trickyPath}}

	files, err := GenerateFiles(t.TempDir(), vmConfig, "vmherd", "", "ssh-ed25519 AAAA key", "10.0.0.1", "1.1.1.1", "52:54:00:aa:bb:cc", sharedMounts, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	userData := decodeUserData(t, files.UserData)
	if len(userData.Mounts) != 1 || userData.Mounts[0][1] != trickyPath {
		t.Errorf("mount point = %q, want %q", userData.Mounts, trickyPath)
	}
	if len(userData.WriteFiles) != 1 || userData.WriteFiles[0].Path != trickyPath {
		t.Errorf("write_files path = %+v, want %q", userData.WriteFiles, trickyPath)
	}
}
