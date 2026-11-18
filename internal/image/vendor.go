package image

import (
	"fmt"
	"net/http"
)

// This file exports the vendor checks of the download path for the release
// URL checker (internal/imageurls). Each function is a thin wrapper, so the
// checker and 'vmherd pull' read vendor directories the same way.

// ChecksumEntry is one file's hash in a vendor checksum file.
type ChecksumEntry struct {
	Hash      string // hex digest as the vendor publishes it (64 or 128 hex digits)
	Algorithm string // "sha256" or "sha512"
}

// HeadImageURL sends HEAD to an HTTPS image URL, following redirects, and
// returns the final HTTP status code.
func HeadImageURL(httpClient *http.Client, imageURL string) (int, error) {
	return headImageURL(httpClient, imageURL)
}

// IsWithdrawnStatusCode reports whether a vendor's status code means the file
// is no longer published (HTTP 404 or 410), as 'vmherd pull' reads it.
func IsWithdrawnStatusCode(statusCode int) bool {
	return isWithdrawnStatusCode(statusCode)
}

// ChecksumFileURLs reads the HTTPS directory listing at directoryURL and
// returns its checksum files' URLs on the same host, in the order 'vmherd
// pull' tries them (imageFilename's own file first), and the listing's final URL.
func ChecksumFileURLs(httpClient *http.Client, directoryURL, imageFilename string) (checksumURLs []string, listingFinalURL string, err error) {
	checksumURLs, listingFinalURL, err = readDirectoryChecksumLinks(httpClient, directoryURL, imageFilename)
	if err != nil {
		return nil, "", fmt.Errorf("reading directory listing %q: %w", directoryURL, err)
	}
	return checksumURLs, listingFinalURL, nil
}

// FetchChecksumEntries downloads an HTTPS checksum file and returns every file
// it lists by name (sha256sum or BSD format, same algorithm detection as
// 'vmherd pull') and the checksum file's final URL after redirects.
func FetchChecksumEntries(httpClient *http.Client, checksumURL string) (checksumEntries map[string]ChecksumEntry, finalURL string, err error) {
	checksumFileContent, finalURL, err := fetchChecksumFileContent(httpClient, checksumURL)
	if err != nil {
		return nil, "", fmt.Errorf("reading checksum file %q: %w", checksumURL, err)
	}
	parsedEntries := parseChecksumEntries(checksumFileContent)
	checksumEntries = make(map[string]ChecksumEntry, len(parsedEntries))
	for listedFilename, parsedEntry := range parsedEntries {
		checksumEntries[listedFilename] = ChecksumEntry{
			Hash:      parsedEntry.hash,
			Algorithm: resolveChecksumAlgorithm(parsedEntry.algorithm, checksumURL),
		}
	}
	return checksumEntries, finalURL, nil
}
