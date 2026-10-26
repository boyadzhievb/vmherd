package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadValidHerdfile(t *testing.T) {
	dir := t.TempDir()
	herdfile := `
vms:
  - name: ctrl
    image: ubuntu:noble
    cpus: 2
    memory: 4096
    disk: 20G
    network:
      ip: 192.168.122.10
  - name: worker
    image: ubuntu:noble
    network:
      ip: 192.168.122.20

network:
  name: test-net
  subnet: 192.168.122.0/24

ssh:
  user: testuser
`
	if err := os.WriteFile(filepath.Join(dir, HerdfileName), []byte(herdfile), 0o644); err != nil {
		t.Fatal(err)
	}

	hf, err := Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(hf.VMs) != 2 {
		t.Fatalf("expected 2 VMs, got %d", len(hf.VMs))
	}

	if hf.VMs[0].Name != "ctrl" {
		t.Errorf("expected VM name 'ctrl', got %q", hf.VMs[0].Name)
	}
	if hf.VMs[0].CPUs != 2 {
		t.Errorf("expected 2 CPUs, got %d", hf.VMs[0].CPUs)
	}
	if hf.VMs[0].Memory != 4096 {
		t.Errorf("expected 4096 memory, got %d", hf.VMs[0].Memory)
	}

	if hf.VMs[1].CPUs != 1 {
		t.Errorf("expected default 1 CPU for worker, got %d", hf.VMs[1].CPUs)
	}
	if hf.VMs[1].Memory != 1024 {
		t.Errorf("expected default 1024 memory for worker, got %d", hf.VMs[1].Memory)
	}
	if hf.VMs[1].Disk != "20G" {
		t.Errorf("expected default disk '20G', got %q", hf.VMs[1].Disk)
	}

	if hf.SSH.User != "testuser" {
		t.Errorf("expected SSH user 'testuser', got %q", hf.SSH.User)
	}
	if hf.Network.Name != "test-net" {
		t.Errorf("expected network name 'test-net', got %q", hf.Network.Name)
	}
}

func TestLoadDefaultValues(t *testing.T) {
	dir := t.TempDir()
	herdfile := `
vms:
  - name: minimal
    image: debian:bookworm
    network:
      ip: 192.168.122.5
`
	if err := os.WriteFile(filepath.Join(dir, HerdfileName), []byte(herdfile), 0o644); err != nil {
		t.Fatal(err)
	}

	hf, err := Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if hf.SSH.User != "vmherd" {
		t.Errorf("expected default SSH user 'vmherd', got %q", hf.SSH.User)
	}
	if hf.Network.Subnet != "192.168.122.0/24" {
		t.Errorf("expected default subnet, got %q", hf.Network.Subnet)
	}
	if hf.Network.DNS != "auto" {
		t.Errorf("expected default DNS 'auto', got %q", hf.Network.DNS)
	}
	if hf.VMs[0].CPUs != 1 {
		t.Errorf("expected default 1 CPU, got %d", hf.VMs[0].CPUs)
	}
	if hf.VMs[0].Memory != 1024 {
		t.Errorf("expected default 1024 MB memory, got %d", hf.VMs[0].Memory)
	}
}

func TestLoadMissingHerdfile(t *testing.T) {
	dir := t.TempDir()
	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error for missing Herdfile")
	}
}

func TestValidateNoVMs(t *testing.T) {
	dir := t.TempDir()
	herdfile := `vms: []`
	if err := os.WriteFile(filepath.Join(dir, HerdfileName), []byte(herdfile), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error for empty VMs")
	}
}

func TestValidateDuplicateNames(t *testing.T) {
	dir := t.TempDir()
	herdfile := `
vms:
  - name: web
    image: ubuntu:noble
  - name: web
    image: ubuntu:noble
`
	if err := os.WriteFile(filepath.Join(dir, HerdfileName), []byte(herdfile), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error for duplicate VM names")
	}
}

func TestValidateMissingImage(t *testing.T) {
	dir := t.TempDir()
	herdfile := `
vms:
  - name: broken
`
	if err := os.WriteFile(filepath.Join(dir, HerdfileName), []byte(herdfile), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error for missing image")
	}
}

func TestValidateBadImageFormat(t *testing.T) {
	dir := t.TempDir()
	herdfile := `
vms:
  - name: broken
    image: ubuntu
`
	if err := os.WriteFile(filepath.Join(dir, HerdfileName), []byte(herdfile), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error for bad image format")
	}
}

func TestDefaultSharedFolders(t *testing.T) {
	dir := t.TempDir()
	herdfile := `
vms:
  - name: web
    image: ubuntu:noble
    network:
      ip: 192.168.122.5
`
	if err := os.WriteFile(filepath.Join(dir, HerdfileName), []byte(herdfile), 0o644); err != nil {
		t.Fatal(err)
	}

	hf, err := Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(hf.VMs[0].SharedFolders) != 1 {
		t.Fatalf("expected 1 default shared folder, got %d", len(hf.VMs[0].SharedFolders))
	}
	if hf.VMs[0].SharedFolders[0].Host != "." {
		t.Errorf("expected default host '.', got %q", hf.VMs[0].SharedFolders[0].Host)
	}
	if hf.VMs[0].SharedFolders[0].Guest != "/vagrant" {
		t.Errorf("expected default guest '/vagrant', got %q", hf.VMs[0].SharedFolders[0].Guest)
	}
}

func TestExplicitSharedFolders(t *testing.T) {
	dir := t.TempDir()
	herdfile := `
vms:
  - name: web
    image: ubuntu:noble
    shared_folders:
      - host: ./src
        guest: /app
`
	if err := os.WriteFile(filepath.Join(dir, HerdfileName), []byte(herdfile), 0o644); err != nil {
		t.Fatal(err)
	}

	hf, err := Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(hf.VMs[0].SharedFolders) != 1 {
		t.Fatalf("expected 1 shared folder, got %d", len(hf.VMs[0].SharedFolders))
	}
	if hf.VMs[0].SharedFolders[0].Host != "./src" {
		t.Errorf("expected host './src', got %q", hf.VMs[0].SharedFolders[0].Host)
	}
	if hf.VMs[0].SharedFolders[0].Guest != "/app" {
		t.Errorf("expected guest '/app', got %q", hf.VMs[0].SharedFolders[0].Guest)
	}
}

func TestSharedFolderRelativeGuestPath(t *testing.T) {
	dir := t.TempDir()
	herdfile := `
vms:
  - name: web
    image: ubuntu:noble
    shared_folders:
      - host: .
        guest: relative/path
`
	if err := os.WriteFile(filepath.Join(dir, HerdfileName), []byte(herdfile), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error for relative guest path")
	}
}

func TestPortForwardDefaults(t *testing.T) {
	dir := t.TempDir()
	herdfile := `
vms:
  - name: web
    image: ubuntu:noble
    ports:
      - host: 8080
        guest: 80
`
	if err := os.WriteFile(filepath.Join(dir, HerdfileName), []byte(herdfile), 0o644); err != nil {
		t.Fatal(err)
	}

	hf, err := Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(hf.VMs[0].Ports) != 1 {
		t.Fatalf("expected 1 port forward, got %d", len(hf.VMs[0].Ports))
	}
	if hf.VMs[0].Ports[0].Protocol != "tcp" {
		t.Errorf("expected default protocol 'tcp', got %q", hf.VMs[0].Ports[0].Protocol)
	}
	if hf.VMs[0].Ports[0].Host != 8080 {
		t.Errorf("expected host port 8080, got %d", hf.VMs[0].Ports[0].Host)
	}
	if hf.VMs[0].Ports[0].Guest != 80 {
		t.Errorf("expected guest port 80, got %d", hf.VMs[0].Ports[0].Guest)
	}
}

func TestPortForwardExplicitProtocol(t *testing.T) {
	dir := t.TempDir()
	herdfile := `
vms:
  - name: web
    image: ubuntu:noble
    ports:
      - host: 5353
        guest: 53
        protocol: udp
`
	if err := os.WriteFile(filepath.Join(dir, HerdfileName), []byte(herdfile), 0o644); err != nil {
		t.Fatal(err)
	}

	hf, err := Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if hf.VMs[0].Ports[0].Protocol != "udp" {
		t.Errorf("expected protocol 'udp', got %q", hf.VMs[0].Ports[0].Protocol)
	}
}

func TestPortForwardInvalidHostPort(t *testing.T) {
	dir := t.TempDir()
	herdfile := `
vms:
  - name: web
    image: ubuntu:noble
    ports:
      - host: 0
        guest: 80
`
	if err := os.WriteFile(filepath.Join(dir, HerdfileName), []byte(herdfile), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error for invalid host port")
	}
}

func TestPortForwardInvalidGuestPort(t *testing.T) {
	dir := t.TempDir()
	herdfile := `
vms:
  - name: web
    image: ubuntu:noble
    ports:
      - host: 8080
        guest: 70000
`
	if err := os.WriteFile(filepath.Join(dir, HerdfileName), []byte(herdfile), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error for invalid guest port")
	}
}

func TestPortForwardInvalidProtocol(t *testing.T) {
	dir := t.TempDir()
	herdfile := `
vms:
  - name: web
    image: ubuntu:noble
    ports:
      - host: 8080
        guest: 80
        protocol: sctp
`
	if err := os.WriteFile(filepath.Join(dir, HerdfileName), []byte(herdfile), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error for invalid protocol")
	}
}

func TestPortForwardDuplicateHostPort(t *testing.T) {
	dir := t.TempDir()
	herdfile := `
vms:
  - name: web
    image: ubuntu:noble
    ports:
      - host: 8080
        guest: 80
  - name: api
    image: ubuntu:noble
    ports:
      - host: 8080
        guest: 3000
`
	if err := os.WriteFile(filepath.Join(dir, HerdfileName), []byte(herdfile), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error for duplicate host port across VMs")
	}
}

func TestFilterVMs(t *testing.T) {
	hf := &Herdfile{
		VMs: []VM{
			{Name: "ctrl", Image: "ubuntu:noble"},
			{Name: "worker-1", Image: "ubuntu:noble"},
			{Name: "worker-2", Image: "ubuntu:noble"},
		},
	}

	all, err := FilterVMs(hf, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("expected 3 VMs, got %d", len(all))
	}

	subset, err := FilterVMs(hf, []string{"ctrl", "worker-2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(subset) != 2 {
		t.Errorf("expected 2 VMs, got %d", len(subset))
	}
	if subset[0].Name != "ctrl" || subset[1].Name != "worker-2" {
		t.Errorf("unexpected VMs: %v", subset)
	}

	_, err = FilterVMs(hf, []string{"nonexistent"})
	if err == nil {
		t.Fatal("expected error for unknown VM name")
	}
}

func TestValidateVMNameSpecialChars(t *testing.T) {
	dir := t.TempDir()
	herdfile := `
vms:
  - name: ";rm -rf /"
    image: ubuntu:noble
`
	if err := os.WriteFile(filepath.Join(dir, HerdfileName), []byte(herdfile), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error for VM name with special characters")
	}
	if !strings.Contains(err.Error(), "invalid") {
		t.Errorf("expected error to mention 'invalid', got: %v", err)
	}
}

func TestValidateVMNameTooLong(t *testing.T) {
	dir := t.TempDir()
	longName := strings.Repeat("a", 64)
	herdfile := fmt.Sprintf(`
vms:
  - name: %s
    image: ubuntu:noble
`, longName)
	if err := os.WriteFile(filepath.Join(dir, HerdfileName), []byte(herdfile), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error for VM name exceeding 63 characters")
	}
	if !strings.Contains(err.Error(), "invalid") {
		t.Errorf("expected error to mention 'invalid', got: %v", err)
	}
}

func TestValidateVMNameStartsWithHyphen(t *testing.T) {
	dir := t.TempDir()
	herdfile := `
vms:
  - name: -bad
    image: ubuntu:noble
`
	if err := os.WriteFile(filepath.Join(dir, HerdfileName), []byte(herdfile), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error for VM name starting with hyphen")
	}
	if !strings.Contains(err.Error(), "invalid") {
		t.Errorf("expected error to mention 'invalid', got: %v", err)
	}
}

func TestValidateVMNameValid(t *testing.T) {
	validNames := []string{"ctrl", "worker-1", "a"}
	for _, name := range validNames {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			herdfile := fmt.Sprintf(`
vms:
  - name: %s
    image: ubuntu:noble
network:
  name: test-net
  subnet: 192.168.122.0/24
`, name)
			if err := os.WriteFile(filepath.Join(dir, HerdfileName), []byte(herdfile), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := Load(dir)
			if err != nil {
				t.Fatalf("valid VM name %q should not produce error, got: %v", name, err)
			}
		})
	}
}

func TestValidateNetworkNameSpecialChars(t *testing.T) {
	dir := t.TempDir()
	herdfile := `
vms:
  - name: web
    image: ubuntu:noble
network:
  name: "</name>"
  subnet: 192.168.122.0/24
`
	if err := os.WriteFile(filepath.Join(dir, HerdfileName), []byte(herdfile), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error for network name with XML injection characters")
	}
	if !strings.Contains(err.Error(), "network name") {
		t.Errorf("expected error about network name, got: %v", err)
	}
}

func TestValidateIPAddressInvalid(t *testing.T) {
	dir := t.TempDir()
	herdfile := `
vms:
  - name: web
    image: ubuntu:noble
    network:
      ip: not-an-ip
network:
  name: test-net
  subnet: 192.168.122.0/24
`
	if err := os.WriteFile(filepath.Join(dir, HerdfileName), []byte(herdfile), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error for invalid IP address")
	}
	if !strings.Contains(err.Error(), "not a valid IPv4") {
		t.Errorf("expected error about invalid IPv4, got: %v", err)
	}
}

func TestValidateIPAddressDuplicate(t *testing.T) {
	dir := t.TempDir()
	herdfile := `
vms:
  - name: web
    image: ubuntu:noble
    network:
      ip: 192.168.122.10
  - name: api
    image: ubuntu:noble
    network:
      ip: 192.168.122.10
network:
  name: test-net
  subnet: 192.168.122.0/24
`
	if err := os.WriteFile(filepath.Join(dir, HerdfileName), []byte(herdfile), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error for duplicate IP address across VMs")
	}
	if !strings.Contains(err.Error(), "already used by VM") {
		t.Errorf("expected error about duplicate IP, got: %v", err)
	}
}

func TestValidateIPOutsideSubnet(t *testing.T) {
	dir := t.TempDir()
	herdfile := `
vms:
  - name: web
    image: ubuntu:noble
    network:
      ip: 192.168.1.5
network:
  name: test-net
  subnet: 10.0.0.0/24
`
	if err := os.WriteFile(filepath.Join(dir, HerdfileName), []byte(herdfile), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error for IP outside network subnet")
	}
	if !strings.Contains(err.Error(), "outside the network subnet") {
		t.Errorf("expected error about IP outside subnet, got: %v", err)
	}
}

func TestValidateDiskSizeInvalid(t *testing.T) {
	invalidSizes := []struct {
		size string
		desc string
	}{
		{"20X", "invalid unit suffix"},
		{"abc", "non-numeric value"},
	}
	for _, tc := range invalidSizes {
		t.Run(tc.desc, func(t *testing.T) {
			dir := t.TempDir()
			herdfile := fmt.Sprintf(`
vms:
  - name: web
    image: ubuntu:noble
    disk: %s
network:
  name: test-net
  subnet: 192.168.122.0/24
`, tc.size)
			if err := os.WriteFile(filepath.Join(dir, HerdfileName), []byte(herdfile), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := Load(dir)
			if err == nil {
				t.Fatalf("expected error for invalid disk size %q", tc.size)
			}
			if !strings.Contains(err.Error(), "disk size") {
				t.Errorf("expected error about disk size, got: %v", err)
			}
		})
	}
}

func TestValidateDiskSizeValid(t *testing.T) {
	validSizes := []string{"20G", "100M", "1T"}
	for _, size := range validSizes {
		t.Run(size, func(t *testing.T) {
			dir := t.TempDir()
			herdfile := fmt.Sprintf(`
vms:
  - name: web
    image: ubuntu:noble
    disk: %s
network:
  name: test-net
  subnet: 192.168.122.0/24
`, size)
			if err := os.WriteFile(filepath.Join(dir, HerdfileName), []byte(herdfile), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := Load(dir)
			if err != nil {
				t.Fatalf("valid disk size %q should not produce error, got: %v", size, err)
			}
		})
	}
}

func TestValidateSharedFolderPathTraversal(t *testing.T) {
	dir := t.TempDir()
	herdfile := `
vms:
  - name: web
    image: ubuntu:noble
    shared_folders:
      - host: /etc/shadow
        guest: /mnt/data
network:
  name: test-net
  subnet: 192.168.122.0/24
`
	if err := os.WriteFile(filepath.Join(dir, HerdfileName), []byte(herdfile), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error for shared folder pointing to /etc/shadow")
	}
	if !strings.Contains(err.Error(), "forbidden system directory") {
		t.Errorf("expected error about forbidden system directory, got: %v", err)
	}
}

func TestValidateSharedFolderRelativeTraversal(t *testing.T) {
	dir := t.TempDir()
	// Use enough ../ segments to reach the filesystem root from any temp directory depth.
	herdfile := `
vms:
  - name: web
    image: ubuntu:noble
    shared_folders:
      - host: ../../../../../../../../../../../../etc/shadow
        guest: /mnt/data
network:
  name: test-net
  subnet: 192.168.122.0/24
`
	if err := os.WriteFile(filepath.Join(dir, HerdfileName), []byte(herdfile), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error for shared folder with path traversal to /etc/shadow")
	}
	if !strings.Contains(err.Error(), "forbidden system directory") {
		t.Errorf("expected error about forbidden system directory, got: %v", err)
	}
}

func TestValidateProvisionSourceAbsolute(t *testing.T) {
	dir := t.TempDir()
	herdfile := `
vms:
  - name: web
    image: ubuntu:noble
    provision:
      - type: file
        source: /etc/shadow
        destination: /tmp/shadow
network:
  name: test-net
  subnet: 192.168.122.0/24
`
	if err := os.WriteFile(filepath.Join(dir, HerdfileName), []byte(herdfile), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error for absolute provision source path")
	}
	if !strings.Contains(err.Error(), "must be a relative path") {
		t.Errorf("expected error about relative path requirement, got: %v", err)
	}
}

func TestValidateProvisionSourceTraversal(t *testing.T) {
	dir := t.TempDir()
	herdfile := `
vms:
  - name: web
    image: ubuntu:noble
    provision:
      - type: file
        source: ../../../etc/passwd
        destination: /tmp/passwd
network:
  name: test-net
  subnet: 192.168.122.0/24
`
	if err := os.WriteFile(filepath.Join(dir, HerdfileName), []byte(herdfile), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error for provision source escaping project directory")
	}
	if !strings.Contains(err.Error(), "escapes project directory") {
		t.Errorf("expected error about escaping project directory, got: %v", err)
	}
}

func TestValidateSSHUserInvalid(t *testing.T) {
	invalidUsers := []struct {
		user string
		desc string
	}{
		{"root\ninjection", "newline injection"},
		{"; rm -rf /", "command injection"},
		{"UPPERCASE", "uppercase letters"},
		{strings.Repeat("a", 33), "exceeds 32 characters"},
		{"123start", "starts with digit"},
		{"user name", "contains space"},
		{"user.name", "contains dot"},
	}
	for _, tc := range invalidUsers {
		t.Run(tc.desc, func(t *testing.T) {
			dir := t.TempDir()
			herdfile := fmt.Sprintf(`
vms:
  - name: web
    image: ubuntu:noble
network:
  name: test-net
  subnet: 192.168.122.0/24
ssh:
  user: "%s"
`, tc.user)
			if err := os.WriteFile(filepath.Join(dir, HerdfileName), []byte(herdfile), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := Load(dir)
			if err == nil {
				t.Fatalf("expected error for invalid SSH user %q (%s)", tc.user, tc.desc)
			}
			if !strings.Contains(err.Error(), "ssh.user") {
				t.Errorf("expected error about ssh.user, got: %v", err)
			}
		})
	}
}

func TestValidateSSHUserValid(t *testing.T) {
	validUsers := []string{"vmherd", "test-user", "_service", "a", "user_123"}
	for _, user := range validUsers {
		t.Run(user, func(t *testing.T) {
			dir := t.TempDir()
			herdfile := fmt.Sprintf(`
vms:
  - name: web
    image: ubuntu:noble
network:
  name: test-net
  subnet: 192.168.122.0/24
ssh:
  user: %s
`, user)
			if err := os.WriteFile(filepath.Join(dir, HerdfileName), []byte(herdfile), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := Load(dir)
			if err != nil {
				t.Fatalf("valid SSH user %q should not produce error, got: %v", user, err)
			}
		})
	}
}

func TestValidateSubnetNon24Rejected(t *testing.T) {
	dir := t.TempDir()
	herdfile := `
vms:
  - name: web
    image: ubuntu:noble
network:
  name: test-net
  subnet: 10.0.0.0/16
`
	if err := os.WriteFile(filepath.Join(dir, HerdfileName), []byte(herdfile), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error for non-/24 subnet")
	}
	if !strings.Contains(err.Error(), "only /24 subnets are supported") {
		t.Errorf("expected error about /24 requirement, got: %v", err)
	}
	if !strings.Contains(err.Error(), "/16") {
		t.Errorf("expected error to mention the actual prefix /16, got: %v", err)
	}
}

func TestValidateIPInDHCPPoolRejected(t *testing.T) {
	dir := t.TempDir()
	herdfile := `
vms:
  - name: web
    image: ubuntu:noble
    network:
      ip: 192.168.122.150
network:
  name: test-net
  subnet: 192.168.122.0/24
`
	if err := os.WriteFile(filepath.Join(dir, HerdfileName), []byte(herdfile), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error for IP in DHCP pool range")
	}
	if !strings.Contains(err.Error(), "DHCP pool range") {
		t.Errorf("expected error about DHCP pool range, got: %v", err)
	}
}

func TestValidateIPOutsideDHCPPoolAccepted(t *testing.T) {
	dir := t.TempDir()
	herdfile := `
vms:
  - name: web
    image: ubuntu:noble
    network:
      ip: 192.168.122.10
network:
  name: test-net
  subnet: 192.168.122.0/24
`
	if err := os.WriteFile(filepath.Join(dir, HerdfileName), []byte(herdfile), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(dir)
	if err != nil {
		t.Fatalf("IP outside DHCP pool should be accepted, got: %v", err)
	}
}
