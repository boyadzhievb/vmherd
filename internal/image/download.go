package image

import (
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"hash"
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

// checksumInfo holds a discovered checksum: the algorithm, expected hash, and
// source URL. The algorithm name ("sha256" or "sha512") determines which
// streaming hash is used during download verification.
type checksumInfo struct {
	algorithm    string // "sha256" or "sha512"
	expectedHash string
	sourceURL    string
}

// Download fetches the cloud image and verifies its checksum. The checksum
// algorithm (SHA-256 or SHA-512) is auto-discovered from the vendor's
// checksum file. The hash is computed during download using streaming
// verification, avoiding a second read pass over multi-GB images.
func Download(entry *Entry) (string, error) {
	cacheDir := config.ImageCacheDir()
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", fmt.Errorf("cannot create image cache directory %s: %w", cacheDir, err)
	}

	cachedPath := CachedPath(entry)
	tmpPath := cachedPath + ".tmp"

	if err := checkImageURLAvailable(http.DefaultClient, entry.ImageURL); err != nil {
		return "", fmt.Errorf("cannot download %s: %w", entry.Description, err)
	}

	checksum, discoverError := discoverChecksumInfo(http.DefaultClient, entry)
	if discoverError != nil {
		return "", fmt.Errorf("cannot find checksum for %s: %w", entry.Description, discoverError)
	}

	ui.Printf("Downloading %s (%s)...\n", entry.Description, entry.Distro+":"+entry.Codename)
	actualHash, downloadError := downloadFileWithHash(entry.ImageURL, tmpPath, checksum.algorithm)
	if downloadError != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("download failed for %s: %w", entry.Description, downloadError)
	}

	ui.Printf("Verifying %s checksum...\n", strings.ToUpper(checksum.algorithm))

	if actualHash != checksum.expectedHash {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("%s mismatch for %s:\n  expected: %s\n  got:      %s\n\nThe image may be corrupted. Try 'vmherd pull %s:%s' again",
			strings.ToUpper(checksum.algorithm), entry.Description, checksum.expectedHash, actualHash, entry.Distro, entry.Codename)
	}

	ui.Println("Checksum verified.")

	if err := os.Rename(tmpPath, cachedPath); err != nil {
		return "", fmt.Errorf("cannot move downloaded image into cache: %w", err)
	}

	return cachedPath, nil
}

// checkImageURLAvailable asks the vendor (HEAD, redirects followed) whether
// the image file exists, so a stale URL is reported as such instead of as a
// missing checksum. Servers that don't answer HEAD are not held against the URL.
func checkImageURLAvailable(httpClient *http.Client, imageURL string) error {
	if !strings.HasPrefix(imageURL, "https://") {
		return fmt.Errorf("refusing to download from non-HTTPS URL: %s — all image URLs must use HTTPS", imageURL)
	}
	response, err := httpClient.Head(imageURL) //nolint:noctx // URLs come from the Herdfile images section, validated as https
	if err != nil {
		return fmt.Errorf("cannot reach %s: %w — check the network connection and the URL", imageURL, err)
	}
	if closeErr := response.Body.Close(); closeErr != nil {
		return fmt.Errorf("closing HEAD response from %s: %w", imageURL, closeErr)
	}
	switch response.StatusCode {
	case http.StatusNotFound, http.StatusGone:
		directoryURL := imageURL[:strings.LastIndex(imageURL, "/")+1]
		return fmt.Errorf("image URL %s returns HTTP %d — the vendor no longer publishes this file. "+
			"Look up the current file name at %s and update the images: entry in your Herdfile",
			imageURL, response.StatusCode, directoryURL)
	}
	return nil
}

// discoverChecksumInfo finds the checksum for an image entry. If the entry has
// an explicit ChecksumURL, it is used directly. Otherwise the function scrapes
// the image directory listing for checksum files (see isChecksumFileName),
// then fetches them in preference order until one lists the image.
func discoverChecksumInfo(httpClient *http.Client, entry *Entry) (*checksumInfo, error) {
	if entry.ChecksumURL != "" {
		return fetchChecksumFromURL(httpClient, entry.ChecksumURL, entry.Filename)
	}

	baseURL := entry.ImageURL[:strings.LastIndex(entry.ImageURL, "/")+1]

	checksumURLs, scrapeError := scrapeChecksumURLsFromDirectory(httpClient, baseURL, entry.Filename)
	if scrapeError != nil {
		return nil, fmt.Errorf("reading the image directory listing at %s: %w", baseURL, scrapeError)
	}
	if len(checksumURLs) == 0 {
		return nil, fmt.Errorf("the directory listing at %s links no checksum file (looked for names containing "+
			"SUM, SHA256 or SHA512, e.g. SHA256SUMS or %s.SHA256), so the download can't be verified — "+
			"open that page and check that the images: entry in your Herdfile points at the vendor's image directory",
			baseURL, entry.Filename)
	}

	var lastError error
	for _, candidateURL := range checksumURLs {
		result, fetchError := fetchChecksumFromURL(httpClient, candidateURL, entry.Filename)
		if fetchError != nil {
			lastError = fetchError
			continue
		}
		return result, nil
	}

	return nil, fmt.Errorf("no checksum file at %s contained hash for %q: %w", baseURL, entry.Filename, lastError)
}

// scrapeChecksumURLsFromDirectory fetches an HTTP directory listing and returns
// the URLs of its checksum files in preference order (see
// extractChecksumLinksFromHTML). imageFilename ranks the image's own file first.
func scrapeChecksumURLsFromDirectory(httpClient *http.Client, baseURL, imageFilename string) ([]string, error) {
	if !strings.HasPrefix(baseURL, "https://") {
		return nil, fmt.Errorf("directory URL must use HTTPS: %s", baseURL)
	}

	resp, err := httpClient.Get(baseURL) //nolint:noctx // URLs come from the Herdfile images section, validated as https
	if err != nil {
		return nil, fmt.Errorf("cannot fetch directory listing: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d fetching directory listing from %s", resp.StatusCode, baseURL)
	}

	const maxDirectorySize = 1 << 20
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxDirectorySize))
	if err != nil {
		return nil, fmt.Errorf("cannot read directory listing: %w", err)
	}

	return extractChecksumLinksFromHTML(string(data), baseURL, imageFilename), nil
}

// checksumSignatureSuffixes are detached-signature extensions; such files sit
// next to checksum files but hold no hashes.
var checksumSignatureSuffixes = []string{".GPG", ".SIG", ".ASC"}

// unsupportedHashFileMarkers name checksum files for algorithms vmherd can't
// verify (BLAKE2, MD5, SHA-1, SHA-224, SHA-384). Their hashes would be taken
// for SHA-256/512 by length and fail as a misleading "image corrupted" mismatch.
var unsupportedHashFileMarkers = []string{"B2SUM", "BLAKE2", "MD5", "SHA1SUM", "SHA224", "SHA384"}

// isChecksumFileName reports whether an upper-cased link names a checksum file:
// a SUMS/CHECKSUM list (Ubuntu, Debian, Fedora, Rocky) or a per-image
// <image>.SHA256 file (Arch). Signatures and unsupported algorithms are excluded.
func isChecksumFileName(upperLink string) bool {
	upperFileName := upperLink[strings.LastIndex(upperLink, "/")+1:]
	for _, signatureSuffix := range checksumSignatureSuffixes {
		if strings.HasSuffix(upperFileName, signatureSuffix) {
			return false
		}
	}
	for _, unsupportedMarker := range unsupportedHashFileMarkers {
		if strings.Contains(upperFileName, unsupportedMarker) {
			return false
		}
	}
	if strings.HasSuffix(upperFileName, ".SHA1") {
		return false
	}
	return strings.Contains(upperFileName, "SUM") || strings.Contains(upperFileName, "SHA256") || strings.Contains(upperFileName, "SHA512")
}

// extractHrefTargets returns the href="..." targets of an HTML page in order.
func extractHrefTargets(htmlContent string) []string {
	var linkTargets []string
	searchContent := htmlContent
	for {
		hrefIndex := strings.Index(searchContent, "href=\"")
		if hrefIndex == -1 {
			return linkTargets
		}
		searchContent = searchContent[hrefIndex+6:]
		quoteEnd := strings.Index(searchContent, "\"")
		if quoteEnd == -1 {
			return linkTargets
		}
		linkTargets = append(linkTargets, searchContent[:quoteEnd])
		searchContent = searchContent[quoteEnd:]
	}
}

// extractChecksumLinksFromHTML parses href attributes from an HTML directory
// listing and returns absolute URLs for checksum files: the image's own
// <image>.<hash> file first, then SHA256, SHA512 and generic lists. Only links
// on the listing's HTTPS host are kept, so a tampered listing can't redirect
// verification to another server.
func extractChecksumLinksFromHTML(htmlContent, baseURL, imageFilename string) []string {
	baseHost := extractHostFromURL(baseURL)
	upperImagePrefix := strings.ToUpper(imageFilename) + "."
	var ownFileURLs, sha256URLs, sha512URLs, genericURLs []string

	for _, linkTarget := range extractHrefTargets(htmlContent) {
		upperLink := strings.ToUpper(linkTarget)
		if !isChecksumFileName(upperLink) {
			continue
		}

		absoluteURL := linkTarget
		if !strings.HasPrefix(linkTarget, "http") {
			absoluteURL = baseURL + linkTarget
		}
		if !strings.HasPrefix(absoluteURL, "https://") || extractHostFromURL(absoluteURL) != baseHost {
			continue
		}

		switch {
		case imageFilename != "" && strings.HasPrefix(upperLink, upperImagePrefix):
			ownFileURLs = append(ownFileURLs, absoluteURL)
		case strings.Contains(upperLink, "SHA256"):
			sha256URLs = append(sha256URLs, absoluteURL)
		case strings.Contains(upperLink, "SHA512"):
			sha512URLs = append(sha512URLs, absoluteURL)
		default:
			genericURLs = append(genericURLs, absoluteURL)
		}
	}

	result := append(ownFileURLs, sha256URLs...)
	result = append(result, sha512URLs...)
	return append(result, genericURLs...)
}

// extractHostFromURL returns the host portion of an HTTPS URL.
func extractHostFromURL(rawURL string) string {
	withoutScheme := strings.TrimPrefix(rawURL, "https://")
	slashIndex := strings.Index(withoutScheme, "/")
	if slashIndex == -1 {
		return withoutScheme
	}
	return withoutScheme[:slashIndex]
}

// fetchChecksumFromURL downloads a checksum file and extracts the expected
// hash for the given filename. The algorithm is determined from three sources
// in priority order: BSD-format line prefix, hex digest length, URL path name.
func fetchChecksumFromURL(httpClient *http.Client, checksumURL string, filename string) (*checksumInfo, error) {
	if !strings.HasPrefix(checksumURL, "https://") {
		return nil, fmt.Errorf("checksum URL must use HTTPS: %s", checksumURL)
	}

	resp, err := httpClient.Get(checksumURL) //nolint:noctx // URLs come from the Herdfile images section, validated as https
	if err != nil {
		return nil, fmt.Errorf("cannot download checksum file: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d fetching checksum from %s", resp.StatusCode, checksumURL)
	}

	const maxChecksumFileSize = 1 << 20
	limitedReader := io.LimitReader(resp.Body, maxChecksumFileSize)

	data, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, fmt.Errorf("cannot read checksum response: %w", err)
	}

	parseResult, parseError := parseChecksumWithAlgorithm(string(data), filename)
	if parseError != nil {
		return nil, parseError
	}

	algorithm := parseResult.algorithm
	if algorithm == "" {
		algorithm = inferAlgorithmFromURLPath(checksumURL)
	}
	if algorithm == "" {
		algorithm = "sha256"
	}

	return &checksumInfo{
		algorithm:    algorithm,
		expectedHash: parseResult.hash,
		sourceURL:    checksumURL,
	}, nil
}

// inferAlgorithmFromURLPath determines the hash algorithm from the checksum
// file URL path name (e.g. "SHA512SUMS" → sha512).
func inferAlgorithmFromURLPath(checksumURL string) string {
	upperURL := strings.ToUpper(checksumURL)
	if strings.Contains(upperURL, "SHA512") {
		return "sha512"
	}
	if strings.Contains(upperURL, "SHA256") {
		return "sha256"
	}
	return ""
}

// newHashForAlgorithm returns a new hash.Hash for the given algorithm name.
func newHashForAlgorithm(algorithm string) hash.Hash {
	if algorithm == "sha512" {
		return sha512.New()
	}
	return sha256.New()
}

// downloadFileWithHash fetches a URL to destPath, computing a streaming hash.
// Returns the hex-encoded digest of the downloaded content.
func downloadFileWithHash(imageURL, destPath string, algorithm string) (string, error) {
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

	hasher := newHashForAlgorithm(algorithm)
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
		if err := os.Remove(staleFile); err != nil {
			ui.Printf("Warning: could not remove stale temp file %s: %v\n", staleFile, err)
		}
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

// checksumParseResult holds the hash and the algorithm detected from file content.
type checksumParseResult struct {
	hash      string
	algorithm string // "sha256", "sha512", or "" if not determinable from content
}

// parseChecksum extracts the hash for a filename from a checksum file.
// Uses exact filename matching to prevent substring collisions. Handles both
// "hash *filename" (coreutils format) and "ALGO (filename) = hash" (BSD format).
// For BSD format, the algorithm is extracted from the line prefix.
func parseChecksum(content, filename string) (string, error) {
	result, err := parseChecksumWithAlgorithm(content, filename)
	if err != nil {
		return "", err
	}
	return result.hash, nil
}

// parseChecksumWithAlgorithm extracts the hash and detected algorithm from a
// checksum file. BSD-format lines embed the algorithm name (e.g. "SHA256 (...) = hash").
func parseChecksumWithAlgorithm(content, filename string) (*checksumParseResult, error) {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "-----") || strings.HasPrefix(line, "Hash:") {
			continue
		}

		// BSD format: "SHA256 (filename) = hash" or "SHA512 (filename) = hash"
		if strings.Contains(line, "("+filename+")") && strings.Contains(line, " = ") {
			parts := strings.SplitN(line, " = ", 2)
			if len(parts) == 2 {
				algorithm := inferAlgorithmFromBSDPrefix(line)
				return &checksumParseResult{
					hash:      strings.TrimSpace(parts[1]),
					algorithm: algorithm,
				}, nil
			}
		}

		// coreutils format: "hash  filename" or "hash *filename"
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			candidateFilename := strings.TrimPrefix(fields[1], "*")
			if candidateFilename == filename {
				algorithm := inferAlgorithmFromHashLength(fields[0])
				return &checksumParseResult{
					hash:      fields[0],
					algorithm: algorithm,
				}, nil
			}
		}
	}

	return nil, fmt.Errorf("no hash found for %q in checksum file", filename)
}

// inferAlgorithmFromBSDPrefix extracts the algorithm from a BSD-format checksum
// line prefix like "SHA256 (...)" or "SHA512 (...)".
func inferAlgorithmFromBSDPrefix(line string) string {
	upperLine := strings.ToUpper(line)
	if strings.HasPrefix(upperLine, "SHA512") {
		return "sha512"
	}
	if strings.HasPrefix(upperLine, "SHA256") {
		return "sha256"
	}
	return ""
}

// inferAlgorithmFromHashLength guesses the algorithm from hex digest length.
func inferAlgorithmFromHashLength(hexDigest string) string {
	switch len(hexDigest) {
	case 64:
		return "sha256"
	case 128:
		return "sha512"
	default:
		return ""
	}
}
