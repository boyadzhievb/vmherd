package image

import (
	"os"
	"path/filepath"
	"strings"
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

// TestParseChecksumEmptyContent verifies that an empty checksum file returns an error.
func TestParseChecksumEmptyContent(t *testing.T) {
	_, err := parseChecksum("", "file.img")
	if err == nil {
		t.Fatal("expected error for empty checksum content")
	}
}

// TestParseChecksumCommentOnly verifies that a checksum file with only comments returns an error.
func TestParseChecksumCommentOnly(t *testing.T) {
	commentOnlyContent := "# comment\n# another"
	_, err := parseChecksum(commentOnlyContent, "file.img")
	if err == nil {
		t.Fatal("expected error when checksum file contains only comments")
	}
}

// TestParseChecksumWhitespaceLines verifies that blank lines are skipped and a valid checksum is still found.
func TestParseChecksumWhitespaceLines(t *testing.T) {
	contentWithBlanks := "\n\n  \nabc999def888  *cloud-image.img\n\n"
	expectedHash := "abc999def888"

	hash, err := parseChecksum(contentWithBlanks, "cloud-image.img")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hash != expectedHash {
		t.Errorf("expected %s, got %s", expectedHash, hash)
	}
}

// TestParseChecksumSkipsManifest verifies that lines containing ".manifest" are ignored.
func TestParseChecksumSkipsManifest(t *testing.T) {
	contentWithManifest := `manifesthash  *noble-server-cloudimg-amd64.img.manifest
realhash12345  *noble-server-cloudimg-amd64.img`

	hash, err := parseChecksum(contentWithManifest, "noble-server-cloudimg-amd64.img")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hash != "realhash12345" {
		t.Errorf("expected realhash12345, got %s", hash)
	}
}

// TestCachedPath verifies that CachedPath includes the distro, codename, and filename.
func TestCachedPath(t *testing.T) {
	entry := &Entry{
		Distro:   "ubuntu",
		Codename: "noble",
		Filename: "noble-server-cloudimg-amd64.img",
	}

	cachedPath := CachedPath(entry)

	if !strings.Contains(cachedPath, "ubuntu") {
		t.Errorf("CachedPath %q does not contain distro 'ubuntu'", cachedPath)
	}
	if !strings.Contains(cachedPath, "noble") {
		t.Errorf("CachedPath %q does not contain codename 'noble'", cachedPath)
	}
	if !strings.Contains(cachedPath, "noble-server-cloudimg-amd64.img") {
		t.Errorf("CachedPath %q does not contain filename 'noble-server-cloudimg-amd64.img'", cachedPath)
	}
}

// TestIsCachedMissing verifies that IsCached returns false for a non-existent entry.
func TestIsCachedMissing(t *testing.T) {
	nonexistentEntry := &Entry{
		Distro:   "nonexistent",
		Codename: "fake",
		Filename: "does-not-exist.qcow2",
	}

	if IsCached(nonexistentEntry) {
		t.Error("expected IsCached to return false for a non-existent image")
	}
}

// TestSha256File verifies that sha256File returns the correct hex digest for known content.
func TestSha256File(t *testing.T) {
	tempDir := t.TempDir()
	testFilePath := filepath.Join(tempDir, "testfile.txt")

	knownContent := []byte("hello")
	// SHA256("hello") = 2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824
	expectedSHA256 := "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"

	if err := os.WriteFile(testFilePath, knownContent, 0o644); err != nil {
		t.Fatalf("cannot write temp file: %v", err)
	}

	actualHash, err := sha256File(testFilePath)
	if err != nil {
		t.Fatalf("sha256File returned unexpected error: %v", err)
	}

	if actualHash != expectedSHA256 {
		t.Errorf("sha256File hash mismatch:\n  expected: %s\n  got:      %s", expectedSHA256, actualHash)
	}
}
