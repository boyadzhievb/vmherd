package image

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/boyadzhievb/vmherd/internal/config"
)

// CachedPath returns the local cache path for an image entry.
func CachedPath(entry *Entry) string {
	return filepath.Join(config.ImageCacheDir(), entry.Distro+"-"+entry.Codename+"-"+entry.Filename)
}

// IsCached returns true if the image is already downloaded.
func IsCached(entry *Entry) bool {
	_, err := os.Stat(CachedPath(entry))
	return err == nil
}

// EnsureCached downloads and verifies the image if not already cached.
func EnsureCached(entry *Entry) (string, error) {
	cachedPath := CachedPath(entry)
	if IsCached(entry) {
		return cachedPath, nil
	}
	return Download(entry)
}

// Download fetches the cloud image and verifies its SHA256 checksum.
func Download(entry *Entry) (string, error) {
	cacheDir := config.ImageCacheDir()
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", fmt.Errorf("cannot create image cache directory %s: %w", cacheDir, err)
	}

	cachedPath := CachedPath(entry)
	tmpPath := cachedPath + ".tmp"

	fmt.Printf("Downloading %s (%s)...\n", entry.Description, entry.Distro+":"+entry.Codename)
	if err := downloadFile(entry.ImageURL, tmpPath); err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("download failed for %s: %w", entry.Description, err)
	}

	fmt.Println("Verifying SHA256 checksum...")
	expectedHash, err := fetchExpectedChecksum(entry)
	if err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("cannot verify checksum for %s: %w", entry.Description, err)
	}

	actualHash, err := sha256File(tmpPath)
	if err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("cannot compute checksum: %w", err)
	}

	if actualHash != expectedHash {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("SHA256 mismatch for %s:\n  expected: %s\n  got:      %s\n\nThe image may be corrupted. Try 'vmherd pull %s:%s' again",
			entry.Description, expectedHash, actualHash, entry.Distro, entry.Codename)
	}

	fmt.Println("Checksum verified.")

	if err := os.Rename(tmpPath, cachedPath); err != nil {
		return "", fmt.Errorf("cannot move downloaded image into cache: %w", err)
	}

	return cachedPath, nil
}

func downloadFile(url, destPath string) error {
	wgetPath, err := exec.LookPath("wget")
	if err == nil {
		cmd := exec.Command(wgetPath, "-q", "--show-progress", "-O", destPath, url)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}

	curlPath, err := exec.LookPath("curl")
	if err == nil {
		cmd := exec.Command(curlPath, "-fL", "--progress-bar", "-o", destPath, url)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}

	return fmt.Errorf("neither wget nor curl found — install one of them")
}

func fetchExpectedChecksum(entry *Entry) (string, error) {
	tmpFile, err := os.CreateTemp("", "vmherd-checksum-*")
	if err != nil {
		return "", err
	}
	tmpPath := tmpFile.Name()
	_ = tmpFile.Close()
	defer func() { _ = os.Remove(tmpPath) }()

	if err := downloadFile(entry.ChecksumURL, tmpPath); err != nil {
		return "", fmt.Errorf("cannot download checksum file: %w", err)
	}

	data, err := os.ReadFile(tmpPath)
	if err != nil {
		return "", err
	}

	return parseChecksum(string(data), entry.Filename)
}

// parseChecksum extracts the SHA256 hash for a filename from a checksum file.
// Handles both formats:
//
//	"hash *filename" (sha256sum format, used by Ubuntu/Debian)
//	"SHA256 (filename) = hash" (BSD/Fedora format)
func parseChecksum(content, filename string) (string, error) {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if !strings.Contains(line, filename) {
			continue
		}

		if strings.Contains(line, ".manifest") {
			continue
		}

		if strings.Contains(line, " = ") {
			parts := strings.SplitN(line, " = ", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1]), nil
			}
		}

		fields := strings.Fields(line)
		if len(fields) >= 2 {
			return fields[0], nil
		}
	}

	return "", fmt.Errorf("no SHA256 hash found for %q in checksum file", filename)
}

func sha256File(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}
