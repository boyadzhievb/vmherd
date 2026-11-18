package image

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/boyadzhievb/vmherd/internal/config"
)

func TestResolveFromImages(t *testing.T) {
	images := map[string]string{
		"ubuntu:noble":    "https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-amd64.img",
		"debian:bookworm": "https://cloud.debian.org/images/cloud/bookworm/latest/debian-12-nocloud-amd64.qcow2",
		"fedora:45":       "https://download.fedoraproject.org/pub/fedora/linux/releases/45/Cloud/x86_64/images/Fedora-Cloud-Base-Generic-45-1.4.x86_64.qcow2",
	}

	tests := []struct {
		spec       string
		wantDistro string
	}{
		{"ubuntu:noble", "ubuntu"},
		{"debian:bookworm", "debian"},
		{"fedora:45", "fedora"},
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
	images := config.BuiltInImageURLs()
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

func TestParseChecksumSha256sumFormat(t *testing.T) {
	content := `a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1  *noble-server-cloudimg-amd64.img
deadbeef1234  *noble-server-cloudimg-amd64.manifest`

	hash, err := parseChecksum(content, "noble-server-cloudimg-amd64.img")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hash != "a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1" {
		t.Errorf("expected a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1, got %s", hash)
	}
}

func TestParseChecksumBSDFormat(t *testing.T) {
	content := `SHA256 (Fedora-Cloud-Base-Generic-41-1.4.x86_64.qcow2) = b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2`

	hash, err := parseChecksum(content, "Fedora-Cloud-Base-Generic-41-1.4.x86_64.qcow2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hash != "b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2" {
		t.Errorf("expected b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2, got %s", hash)
	}
}

func TestParseChecksumNotFound(t *testing.T) {
	content := `5c5c5c5c5c5c5c5c5c5c5c5c5c5c5c5c5c5c5c5c5c5c5c5c5c5c5c5c5c5c5c5c  *other-file.img`

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
	contentWithBlanks := "\n\n  \nc3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3  *cloud-image.img\n\n"
	expectedHash := "c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3c3"

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
	contentWithManifest := `d4d4d4d4d4d4d4d4d4d4d4d4d4d4d4d4d4d4d4d4d4d4d4d4d4d4d4d4d4d4d4d4  *noble-server-cloudimg-amd64.img.manifest
e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5  *noble-server-cloudimg-amd64.img`

	hash, err := parseChecksum(contentWithManifest, "noble-server-cloudimg-amd64.img")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hash != "e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5" {
		t.Errorf("expected e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5e5, got %s", hash)
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
			content:  "f6f6f6f6f6f6f6f6f6f6f6f6f6f6f6f6f6f6f6f6f6f6f6f6f6f6f6f6f6f6f6f6  *amd64-disk-kvm.img\n",
			filename: "amd64.img",
			wantErr:  true,
		},
		{
			name:     "substring filename does not match longer BSD entry",
			content:  "SHA256 (amd64-disk-kvm.img) = f6f6f6f6f6f6f6f6f6f6f6f6f6f6f6f6f6f6f6f6f6f6f6f6f6f6f6f6f6f6f6f6\n",
			filename: "amd64.img",
			wantErr:  true,
		},
		{
			name:     "exact match found among similar filenames in sha256sum format",
			content:  "0707070707070707070707070707070707070707070707070707070707070707  *noble.img-kvm\n1818181818181818181818181818181818181818181818181818181818181818  *noble.img\n2929292929292929292929292929292929292929292929292929292929292929  *noble.img.manifest\n",
			filename: "noble.img",
			wantHash: "1818181818181818181818181818181818181818181818181818181818181818",
		},
		{
			name:     "exact match found among similar filenames in BSD format",
			content:  "SHA256 (noble.img-kvm) = 0707070707070707070707070707070707070707070707070707070707070707\nSHA256 (noble.img) = 1818181818181818181818181818181818181818181818181818181818181818\nSHA256 (noble.img.manifest) = 2929292929292929292929292929292929292929292929292929292929292929\n",
			filename: "noble.img",
			wantHash: "1818181818181818181818181818181818181818181818181818181818181818",
		},
		{
			name:     "star prefix stripped for exact comparison",
			content:  "3a3a3a3a3a3a3a3a3a3a3a3a3a3a3a3a3a3a3a3a3a3a3a3a3a3a3a3a3a3a3a3a  *cloud-image.qcow2\n",
			filename: "cloud-image.qcow2",
			wantHash: "3a3a3a3a3a3a3a3a3a3a3a3a3a3a3a3a3a3a3a3a3a3a3a3a3a3a3a3a3a3a3a3a",
		},
		{
			name:     "no star prefix still matches exactly",
			content:  "4b4b4b4b4b4b4b4b4b4b4b4b4b4b4b4b4b4b4b4b4b4b4b4b4b4b4b4b4b4b4b4b  cloud-image.qcow2\n",
			filename: "cloud-image.qcow2",
			wantHash: "4b4b4b4b4b4b4b4b4b4b4b4b4b4b4b4b4b4b4b4b4b4b4b4b4b4b4b4b4b4b4b4b",
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

// TestCheckImageURLAvailable checks that a missing image (404/410) is reported
// as a stale URL with the directory to look in, while existing files and
// servers that refuse HEAD pass.
func TestCheckImageURLAvailable(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/images/current.qcow2":
			writer.WriteHeader(http.StatusOK)
		case "/images/gone.qcow2":
			writer.WriteHeader(http.StatusGone)
		case "/images/no-head.qcow2":
			writer.WriteHeader(http.StatusMethodNotAllowed)
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	for _, availablePath := range []string{"/images/current.qcow2", "/images/no-head.qcow2"} {
		if err := checkImageURLAvailable(server.Client(), server.URL+availablePath); err != nil {
			t.Errorf("%s: unexpected error: %v", availablePath, err)
		}
	}
	for stalePath, wantStatus := range map[string]string{"/images/old-1.0.qcow2": "HTTP 404", "/images/gone.qcow2": "HTTP 410"} {
		err := checkImageURLAvailable(server.Client(), server.URL+stalePath)
		if err == nil {
			t.Fatalf("%s: expected an error", stalePath)
		}
		for _, fragment := range []string{wantStatus, "no longer publishes this file", server.URL + "/images/", "images: entry"} {
			if !strings.Contains(err.Error(), fragment) {
				t.Errorf("%s: error does not mention %q: %v", stalePath, fragment, err)
			}
		}
	}
	if err := checkImageURLAvailable(server.Client(), "http://example.org/x.img"); err == nil || !strings.Contains(err.Error(), "non-HTTPS") {
		t.Errorf("plain http URL: error = %v, want a non-HTTPS refusal", err)
	}
}
