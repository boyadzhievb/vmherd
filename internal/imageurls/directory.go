package imageurls

import (
	"errors"
	"net/url"
	"path"
	"strings"

	"github.com/boyadzhievb/vmherd/internal/image"
)

// listedChecksum is one file a directory's checksum files list.
type listedChecksum struct {
	checksumEntry    image.ChecksumEntry // the file's hash
	checksumFinalURL string              // final URL of the checksum file that listed it first
}

// directoryChecksumListing is what the checksum files of one image directory list.
type directoryChecksumListing struct {
	listingFinalURL string                    // final URL (after redirects) of the directory listing
	listedFiles     map[string]listedChecksum // every listed file by name; the first checksum file wins
	skippedFiles    []string                  // why each checksum file that couldn't be read was skipped
}

// readDirectoryChecksums reads the checksum files linked directly in an
// image's directory: duplicates once, at most maxChecksumFilesPerDirectory.
// With stopWhenListed it stops at the first that lists imageFilename. A file
// that can't be read is recorded in skippedFiles and skipped, as 'vmherd pull' does.
func (checker *Checker) readDirectoryChecksums(directoryURL, imageFilename string, stopWhenListed bool) (*directoryChecksumListing, error) {
	linkedChecksumURLs, listingFinalURL, listingError := image.ChecksumFileURLs(checker.httpClient, directoryURL, imageFilename)
	if listingError != nil {
		return nil, listingError
	}
	checksumURLs := checksumURLsInDirectory(linkedChecksumURLs, directoryURL)
	if len(checksumURLs) == 0 {
		return nil, errors.New("the directory listing links no checksum file in that directory, so no URL in it can be verified")
	}

	directoryChecksums := &directoryChecksumListing{listingFinalURL: listingFinalURL, listedFiles: make(map[string]listedChecksum)}
	for _, checksumURL := range checksumURLs {
		checksumEntries, checksumFinalURL, fetchError := image.FetchChecksumEntries(checker.httpClient, checksumURL)
		if fetchError != nil {
			directoryChecksums.skippedFiles = append(directoryChecksums.skippedFiles, fetchError.Error())
			continue
		}
		for listedFilename, checksumEntry := range checksumEntries {
			if _, alreadyListed := directoryChecksums.listedFiles[listedFilename]; !alreadyListed {
				directoryChecksums.listedFiles[listedFilename] = listedChecksum{checksumEntry: checksumEntry, checksumFinalURL: checksumFinalURL}
			}
		}
		if _, imageListed := directoryChecksums.listedFiles[imageFilename]; imageListed && stopWhenListed {
			break
		}
	}
	return directoryChecksums, nil
}

// checksumURLsInDirectory keeps, in order, the checksum URLs that name a file
// directly in directoryURL, each once, and at most maxChecksumFilesPerDirectory
// of them (see isDirectlyInDirectory).
func checksumURLsInDirectory(linkedChecksumURLs []string, directoryURL string) []string {
	alreadyKept := make(map[string]bool)
	var keptURLs []string
	for _, linkedURL := range linkedChecksumURLs {
		if len(keptURLs) == maxChecksumFilesPerDirectory {
			break
		}
		if alreadyKept[linkedURL] || !isDirectlyInDirectory(linkedURL, directoryURL) {
			continue
		}
		alreadyKept[linkedURL] = true
		keptURLs = append(keptURLs, linkedURL)
	}
	return keptURLs
}

// isDirectlyInDirectory reports whether a checksum URL names a file directly
// in directoryURL: same HTTPS host, no query or fragment, and a decoded path
// that is already clean, has no backslash, and whose parent is the directory.
// Percent-encoded '../' or '/' therefore can't reach another directory.
func isDirectlyInDirectory(linkedURL, directoryURL string) bool {
	parsedLink, linkError := url.Parse(linkedURL)
	parsedDirectory, directoryError := url.Parse(directoryURL)
	if linkError != nil || directoryError != nil {
		return false
	}
	if parsedLink.Scheme != "https" || parsedLink.Host != parsedDirectory.Host || parsedLink.RawQuery != "" || parsedLink.Fragment != "" {
		return false
	}
	decodedPath := parsedLink.Path
	if strings.Contains(decodedPath, "\\") || path.Clean(decodedPath) != decodedPath {
		return false
	}
	return path.Dir(decodedPath) == path.Clean(parsedDirectory.Path) && path.Base(decodedPath) != ""
}

// skippedFilesNote returns, for an error message, the checksum files that
// were skipped because they couldn't be read, or "" if none was.
func (directoryChecksums *directoryChecksumListing) skippedFilesNote() string {
	if len(directoryChecksums.skippedFiles) == 0 {
		return ""
	}
	return " (checksum files skipped because they couldn't be read: " + strings.Join(directoryChecksums.skippedFiles, "; ") + ")"
}
