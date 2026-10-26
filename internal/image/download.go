package image

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/boyadzhievb/vmherd/internal/config"
	"github.com/boyadzhievb/vmherd/internal/ui"
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
// Cleans up stale .tmp files from interrupted downloads before proceeding.
func EnsureCached(entry *Entry) (string, error) {
	cleanStaleTempFiles(config.ImageCacheDir())
	cachedPath := CachedPath(entry)
	if IsCached(entry) {
		return cachedPath, nil
	}
	return Download(entry)
}

// Download fetches the cloud image and verifies its SHA256 checksum.
// The checksum is computed during download using streaming hash verification,
// avoiding a second read pass over multi-GB images.
func Download(entry *Entry) (string, error) {
	cacheDir := config.ImageCacheDir()
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", fmt.Errorf("cannot create image cache directory %s: %w", cacheDir, err)
	}

	cachedPath := CachedPath(entry)
	tmpPath := cachedPath + ".tmp"

	ui.Printf("Downloading %s (%s)...\n", entry.Description, entry.Distro+":"+entry.Codename)
	actualHash, err := downloadFile(entry.ImageURL, tmpPath)
	if err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("download failed for %s: %w", entry.Description, err)
	}

	ui.Println("Verifying SHA256 checksum...")
	expectedHash, err := fetchExpectedChecksum(entry)
	if err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("cannot verify checksum for %s: %w", entry.Description, err)
	}

	if actualHash != expectedHash {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("SHA256 mismatch for %s:\n  expected: %s\n  got:      %s\n\nThe image may be corrupted. Try 'vmherd pull %s:%s' again",
			entry.Description, expectedHash, actualHash, entry.Distro, entry.Codename)
	}

	ui.Println("Checksum verified.")

	if err := os.Rename(tmpPath, cachedPath); err != nil {
		return "", fmt.Errorf("cannot move downloaded image into cache: %w", err)
	}

	return cachedPath, nil
}

// downloadFile fetches a URL to destPath, computing a SHA256 hash during download.
// Returns the hex-encoded SHA256 digest of the downloaded content.
func downloadFile(imageURL, destPath string) (string, error) {
	if !strings.HasPrefix(imageURL, "https://") {
		return "", fmt.Errorf("refusing to download from non-HTTPS URL: %s — all image URLs must use HTTPS", imageURL)
	}

	resp, err := http.Get(imageURL) //nolint:noctx // URLs come from the built-in registry, not user input
	if err != nil {
		return "", fmt.Errorf("HTTP request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d from %s", resp.StatusCode, imageURL)
	}

	out, err := os.Create(destPath)
	if err != nil {
		return "", fmt.Errorf("cannot create %s: %w", destPath, err)
	}
	defer func() { _ = out.Close() }()

	var reader io.Reader = resp.Body
	if !ui.IsQuiet() && resp.ContentLength > 0 {
		reader = &progressReader{
			reader: resp.Body,
			total:  resp.ContentLength,
		}
	}

	hasher := sha256.New()
	teeReader := io.TeeReader(reader, hasher)

	if _, err := io.Copy(out, teeReader); err != nil {
		return "", fmt.Errorf("download interrupted: %w", err)
	}

	if !ui.IsQuiet() && resp.ContentLength > 0 {
		fmt.Println()
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// cleanStaleTempFiles removes leftover .tmp files from interrupted downloads.
func cleanStaleTempFiles(cacheDir string) {
	matches, err := filepath.Glob(filepath.Join(cacheDir, "*.tmp"))
	if err != nil {
		return
	}
	for _, staleFile := range matches {
		ui.Printf("Removing stale download: %s\n", filepath.Base(staleFile))
		_ = os.Remove(staleFile)
	}
}

type progressReader struct {
	reader  io.Reader
	total   int64
	current int64
	lastPct int
}

// Read wraps the underlying reader and prints download progress to stdout.
func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.reader.Read(p)
	pr.current += int64(n)
	pct := int(pr.current * 100 / pr.total)
	if pct != pr.lastPct {
		pr.lastPct = pct
		fmt.Printf("\r  %3d%% (%d / %d MB)", pct, pr.current/(1024*1024), pr.total/(1024*1024))
	}
	return n, err
}

// fetchExpectedChecksum downloads the vendor checksum file and extracts the expected hash.
func fetchExpectedChecksum(entry *Entry) (string, error) {
	resp, err := http.Get(entry.ChecksumURL) //nolint:gosec // URLs come from the built-in registry
	if err != nil {
		return "", fmt.Errorf("cannot download checksum file: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d fetching checksum from %s", resp.StatusCode, entry.ChecksumURL)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("cannot read checksum response: %w", err)
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
