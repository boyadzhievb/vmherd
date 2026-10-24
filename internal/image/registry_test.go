package image

import (
	"testing"
)

func TestResolveKnownImages(t *testing.T) {
	tests := []struct {
		spec       string
		wantDistro string
	}{
		{"ubuntu:noble", "ubuntu"},
		{"ubuntu:jammy", "ubuntu"},
		{"debian:bookworm", "debian"},
		{"fedora:41", "fedora"},
		{"rocky:9", "rocky"},
		{"almalinux:9", "almalinux"},
		{"arch:latest", "arch"},
	}

	for _, tt := range tests {
		entry, err := Resolve(tt.spec)
		if err != nil {
			t.Errorf("Resolve(%q): unexpected error: %v", tt.spec, err)
			continue
		}
		if entry.Distro != tt.wantDistro {
			t.Errorf("Resolve(%q).Distro = %q, want %q", tt.spec, entry.Distro, tt.wantDistro)
		}
		if entry.ImageURL == "" {
			t.Errorf("Resolve(%q).ImageURL is empty", tt.spec)
		}
		if entry.ChecksumURL == "" {
			t.Errorf("Resolve(%q).ChecksumURL is empty", tt.spec)
		}
		if entry.OSVariant == "" {
			t.Errorf("Resolve(%q).OSVariant is empty", tt.spec)
		}
	}
}

func TestResolveUnknownImage(t *testing.T) {
	_, err := Resolve("ubuntu:nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown codename")
	}

	_, err = Resolve("nonexistent:thing")
	if err == nil {
		t.Fatal("expected error for unknown distro")
	}
}

func TestResolveBadFormat(t *testing.T) {
	_, err := Resolve("ubuntu")
	if err == nil {
		t.Fatal("expected error for bad format")
	}
}

func TestListAll(t *testing.T) {
	entries := ListAll()
	if len(entries) == 0 {
		t.Fatal("expected at least one image in registry")
	}

	seen := make(map[string]bool)
	for _, entry := range entries {
		key := entry.Distro + ":" + entry.Codename
		if seen[key] {
			t.Errorf("duplicate entry: %s", key)
		}
		seen[key] = true
	}
}

func TestParseChecksumSha256sumFormat(t *testing.T) {
	content := `abc123def456  *noble-server-cloudimg-amd64.img
deadbeef1234  *noble-server-cloudimg-amd64.manifest`

	hash, err := parseChecksum(content, "noble-server-cloudimg-amd64.img")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hash != "abc123def456" {
		t.Errorf("expected abc123def456, got %s", hash)
	}
}

func TestParseChecksumBSDFormat(t *testing.T) {
	content := `SHA256 (Fedora-Cloud-Base-Generic-41-1.4.x86_64.qcow2) = abc123hash456`

	hash, err := parseChecksum(content, "Fedora-Cloud-Base-Generic-41-1.4.x86_64.qcow2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hash != "abc123hash456" {
		t.Errorf("expected abc123hash456, got %s", hash)
	}
}

func TestParseChecksumNotFound(t *testing.T) {
	content := `abc123  *other-file.img`

	_, err := parseChecksum(content, "missing-file.img")
	if err == nil {
		t.Fatal("expected error for missing filename in checksum")
	}
}
