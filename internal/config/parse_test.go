package config

import (
	"os"
	"path/filepath"
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
      ip: 10.0.0.5
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
