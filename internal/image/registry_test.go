package image

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveFromImages(t *testing.T) {
	images := map[string]string{
		"ubuntu:noble":    "https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-amd64.img",
		"debian:bookworm": "https://cloud.debian.org/images/cloud/bookworm/latest/debian-12-nocloud-amd64.qcow2",
		"fedora:42":       "https://download.fedoraproject.org/pub/fedora/linux/releases/42/Cloud/x86_64/images/Fedora-Cloud-Base-Generic-42-1.0.x86_64.qcow2",
	}

	tests := []struct {
		spec       string
		wantDistro string
	}{
		{"ubuntu:noble", "ubuntu"},
		{"debian:bookworm", "debian"},
		{"fedora:42", "fedora"},
	}

	for _, tt := range tests {
		entry, err := Resolve(tt.spec, images)
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
		if entry.OSVariant == "" {
			t.Errorf("Resolve(%q).OSVariant is empty", tt.spec)
		}
		if entry.Filename == "" {
			t.Errorf("Resolve(%q).Filename is empty", tt.spec)
		}
	}
}

func TestResolveUnknownImage(t *testing.T) {
	images := map[string]string{
		"ubuntu:noble": "https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-amd64.img",
	}

	_, err := Resolve("ubuntu:nonexistent", images)
	if err == nil {
		t.Fatal("expected error for unknown codename")
	}

	_, err = Resolve("nonexistent:thing", images)
	if err == nil {
		t.Fatal("expected error for unknown distro")
	}
}

func TestResolveBadFormat(t *testing.T) {
	_, err := Resolve("ubuntu", nil)
	if err == nil {
		t.Fatal("expected error for bad format")
	}
}

func TestListAll(t *testing.T) {
	images := DefaultImages()
	entries := ListAll(images)
	if len(entries) == 0 {
		t.Fatal("expected at least one image from defaults")
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

func TestDefaultImages(t *testing.T) {
	defaults := DefaultImages()
	if len(defaults) == 0 {
		t.Fatal("DefaultImages returned empty map")
	}
	if _, found := defaults["ubuntu:noble"]; !found {
		t.Error("DefaultImages missing ubuntu:noble")
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

// TestParseChecksumExactMatchIgnoresManifest verifies that exact matching
// prevents ".manifest" suffix lines from matching the base filename.
func TestParseChecksumExactMatchIgnoresManifest(t *testing.T) {
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

// TestParseChecksumExactMatchPreventsSubstring verifies that a filename that is
// a substring of another filename does not incorrectly match the longer name.
func TestParseChecksumExactMatchPreventsSubstring(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		filename string
		wantHash string
		wantErr  bool
	}{
		{
			name:     "substring filename does not match longer sha256sum entry",
			content:  "wronghash123  *amd64-disk-kvm.img\n",
			filename: "amd64.img",
			wantErr:  true,
		},
		{
			name:     "substring filename does not match longer BSD entry",
			content:  "SHA256 (amd64-disk-kvm.img) = wronghash123\n",
			filename: "amd64.img",
			wantErr:  true,
		},
		{
			name:     "exact match found among similar filenames in sha256sum format",
			content:  "wronghash  *noble.img-kvm\ncorrecthash  *noble.img\nanotherhash  *noble.img.manifest\n",
			filename: "noble.img",
			wantHash: "correcthash",
		},
		{
			name:     "exact match found among similar filenames in BSD format",
			content:  "SHA256 (noble.img-kvm) = wronghash\nSHA256 (noble.img) = correcthash\nSHA256 (noble.img.manifest) = anotherhash\n",
			filename: "noble.img",
			wantHash: "correcthash",
		},
		{
			name:     "star prefix stripped for exact comparison",
			content:  "starhash  *cloud-image.qcow2\n",
			filename: "cloud-image.qcow2",
			wantHash: "starhash",
		},
		{
			name:     "no star prefix still matches exactly",
			content:  "plainhash  cloud-image.qcow2\n",
			filename: "cloud-image.qcow2",
			wantHash: "plainhash",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash, err := parseChecksum(tt.content, tt.filename)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error but got hash %q", hash)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if hash != tt.wantHash {
				t.Errorf("expected hash %q, got %q", tt.wantHash, hash)
			}
		})
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

// TestCleanStaleTempFiles verifies that leftover .tmp files are removed from the cache directory.
func TestCleanStaleTempFiles(t *testing.T) {
	tempDir := t.TempDir()

	staleTmpPath := filepath.Join(tempDir, "ubuntu-noble-image.qcow2.tmp")
	if err := os.WriteFile(staleTmpPath, []byte("stale partial download"), 0o644); err != nil {
		t.Fatalf("cannot create stale .tmp file: %v", err)
	}

	// Create a real cached file that should NOT be removed.
	realImagePath := filepath.Join(tempDir, "ubuntu-noble-image.qcow2")
	if err := os.WriteFile(realImagePath, []byte("real image"), 0o644); err != nil {
		t.Fatalf("cannot create real image file: %v", err)
	}

	cleanStaleTempFiles(tempDir)

	if _, err := os.Stat(staleTmpPath); !os.IsNotExist(err) {
		t.Errorf("stale .tmp file should have been removed, but still exists: %s", staleTmpPath)
	}

	if _, err := os.Stat(realImagePath); err != nil {
		t.Errorf("real image file should not have been removed: %v", err)
	}
}

// TestCleanStaleTempFilesEmptyDir verifies that cleanup is a no-op on an empty directory.
func TestCleanStaleTempFilesEmptyDir(t *testing.T) {
	tempDir := t.TempDir()
	// Should not panic or error on empty directory.
	cleanStaleTempFiles(tempDir)
}

// TestDownloadFileRejectsNonHTTPS verifies that downloadFile refuses non-HTTPS URLs.
func TestDownloadFileRejectsNonHTTPS(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{"plain HTTP", "http://example.com/image.qcow2"},
		{"FTP", "ftp://example.com/image.qcow2"},
		{"empty URL", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			destPath := filepath.Join(t.TempDir(), "output.qcow2")
			_, err := downloadFileWithHash(tt.url, destPath, "sha256")
			if err == nil {
				t.Fatalf("expected error for non-HTTPS URL %q, got nil", tt.url)
			}
			if !strings.Contains(err.Error(), "non-HTTPS") {
				t.Errorf("error should mention non-HTTPS, got: %v", err)
			}
		})
	}
}
