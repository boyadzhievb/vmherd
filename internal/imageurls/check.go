package imageurls

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/boyadzhievb/vmherd/internal/image"
)

// vendorRequestTimeout bounds every request to a vendor host, so a release
// stops with an error instead of hanging on a silent mirror.
const vendorRequestTimeout = 30 * time.Second

// maxVendorRedirects is how many redirects a vendor request may follow (the
// net/http default); Fedora's download host redirects to a mirror.
const maxVendorRedirects = 10

// maxChecksumFilesPerDirectory caps how many checksum files the checker reads
// from one directory listing, in 'vmherd pull' preference order (the image's
// own checksum file first), so a hostile listing can't fan out requests.
const maxChecksumFilesPerDirectory = 8

// URLStatus is the outcome of checking one guide URL.
type URLStatus string

const (
	// StatusCurrent: the URL answers and its directory's checksum file lists it.
	StatusCurrent URLStatus = "current"
	// StatusStale: the URL is gone or unlisted, and one newer build replaces it.
	StatusStale URLStatus = "stale"
	// StatusFailed: the URL could not be checked, or has no single replacement.
	StatusFailed URLStatus = "failed"
)

// CheckResult is what checking one guide URL found.
type CheckResult struct {
	Image          GuideImage          // the guide row that was checked
	Status         URLStatus           // current, stale or failed
	Checksum       image.ChecksumEntry // current: the URL's hash; stale: the replacement's hash
	StaleReason    string              // stale: why the URL no longer passes
	ReplacementURL string              // stale: the newer build in the same directory
	ListingURL     string              // stale: final URL (after redirects) of the directory listing read
	ChecksumURL    string              // stale: final URL of the checksum file that lists the replacement
	Err            error               // failed: what went wrong and where to look
}

// Checker checks guide URLs against the vendors' directories over HTTPS.
type Checker struct {
	httpClient *http.Client // vendor client: bounded timeout, HTTPS-only redirects
}

// NewChecker returns a Checker whose requests time out after
// vendorRequestTimeout and never follow a redirect away from HTTPS.
func NewChecker() *Checker {
	return newVendorChecker(http.DefaultTransport, vendorRequestTimeout)
}

// newVendorChecker returns a Checker with the vendor redirect policy over the
// given transport and request timeout; tests pass a test server's transport.
func newVendorChecker(transport http.RoundTripper, requestTimeout time.Duration) *Checker {
	return &Checker{httpClient: &http.Client{Transport: transport, Timeout: requestTimeout, CheckRedirect: refuseNonHTTPSRedirect}}
}

// refuseNonHTTPSRedirect stops a vendor redirect to a non-HTTPS URL, or after
// maxVendorRedirects hops.
func refuseNonHTTPSRedirect(redirectRequest *http.Request, previousRequests []*http.Request) error {
	if redirectRequest.URL.Scheme != "https" {
		return fmt.Errorf("refusing redirect to non-HTTPS URL %s: vendor URLs must stay on HTTPS", redirectRequest.URL)
	}
	if len(previousRequests) >= maxVendorRedirects {
		return fmt.Errorf("stopped after %d redirects at %s", maxVendorRedirects, redirectRequest.URL)
	}
	return nil
}

// CheckImage checks one guide URL: it must answer 2xx and be listed in its
// directory's checksum file. If it is gone or unlisted, CheckImage looks for
// the newer build in the same directory (see selectNewerBuild).
func (checker *Checker) CheckImage(guideImage GuideImage) CheckResult {
	directoryURL, imageFilename, splitError := splitImageURL(guideImage.URL)
	if splitError != nil {
		return failedResult(guideImage, fmt.Errorf("%s: %w", guideImage.Name, splitError))
	}

	statusCode, probeError := checker.probeImageURL(guideImage.URL)
	if probeError != nil {
		return failedResult(guideImage, fmt.Errorf("%s: checking %s: %w", guideImage.Name, guideImage.URL, probeError))
	}
	imageWithdrawn := image.IsWithdrawnStatusCode(statusCode)
	if !imageWithdrawn && !isSuccessStatusCode(statusCode) {
		return failedResult(guideImage, fmt.Errorf("%s: %s answers HTTP %d — the vendor's server failed or refused the request; "+
			"run the check again later, or look for the current file in %s and update its row in %s",
			guideImage.Name, guideImage.URL, statusCode, directoryURL, GuideRelativePath))
	}

	directoryChecksums, listError := checker.readDirectoryChecksums(directoryURL, imageFilename, !imageWithdrawn)
	if listError != nil {
		return failedResult(guideImage, fmt.Errorf("%s: checking %s: %w — look for the current file in %s and update its row in %s",
			guideImage.Name, guideImage.URL, listError, directoryURL, GuideRelativePath))
	}

	listedImage, imageListed := directoryChecksums.listedFiles[imageFilename]
	if imageListed && !imageWithdrawn {
		return CheckResult{Image: guideImage, Status: StatusCurrent, Checksum: listedImage.checksumEntry}
	}

	staleReason := fmt.Sprintf("no checksum file in %s lists %s", directoryURL, imageFilename)
	if imageWithdrawn {
		staleReason = fmt.Sprintf("the URL answers HTTP %d: the vendor no longer publishes the file", statusCode)
	}
	return checker.findReplacement(guideImage, directoryURL, imageFilename, directoryChecksums, staleReason)
}

// findReplacement picks the newer build of a stale URL's file from the files
// its directory's checksum files list, and checks that the new URL answers 2xx.
// While any checksum file of the directory was skipped, no replacement is taken.
func (checker *Checker) findReplacement(guideImage GuideImage, directoryURL, imageFilename string,
	directoryChecksums *directoryChecksumListing, staleReason string) CheckResult {
	listedFilenames := make([]string, 0, len(directoryChecksums.listedFiles))
	for listedFilename := range directoryChecksums.listedFiles {
		listedFilenames = append(listedFilenames, listedFilename)
	}
	sort.Strings(listedFilenames)

	replacementFilename, selectError := selectNewerBuild(imageFilename, directoryURL, listedFilenames)
	if selectError != nil {
		return failedResult(guideImage, fmt.Errorf("%s: %s is stale (%s), and the checksum files of %s give no single replacement: %w%s — "+
			"look for the current file there and update its row in %s",
			guideImage.Name, guideImage.URL, staleReason, directoryURL, selectError, directoryChecksums.skippedFilesNote(), GuideRelativePath))
	}

	replacementURL := directoryURL + replacementFilename
	if len(directoryChecksums.skippedFiles) > 0 {
		return failedResult(guideImage, fmt.Errorf("%s: %s is stale (%s); %s would replace it, but a skipped checksum file of %s "+
			"might list another build%s — run the release again when the vendor answers, or look for the current file there and update its row in %s",
			guideImage.Name, guideImage.URL, staleReason, replacementURL, directoryURL, directoryChecksums.skippedFilesNote(), GuideRelativePath))
	}
	statusCode, probeError := checker.probeImageURL(replacementURL)
	if probeError == nil && !isSuccessStatusCode(statusCode) {
		probeError = fmt.Errorf("it answers HTTP %d", statusCode)
	}
	if probeError != nil {
		return failedResult(guideImage, fmt.Errorf("%s: %s is stale (%s), and its replacement %s does not answer: %w%s — "+
			"look for the current file in %s and update its row in %s",
			guideImage.Name, guideImage.URL, staleReason, replacementURL, probeError, directoryChecksums.skippedFilesNote(), directoryURL, GuideRelativePath))
	}

	replacementListing := directoryChecksums.listedFiles[replacementFilename]
	return CheckResult{
		Image:          guideImage,
		Status:         StatusStale,
		Checksum:       replacementListing.checksumEntry,
		StaleReason:    staleReason,
		ReplacementURL: replacementURL,
		ListingURL:     directoryChecksums.listingFinalURL,
		ChecksumURL:    replacementListing.checksumFinalURL,
	}
}

// splitImageURL returns an HTTPS image URL's directory (with trailing slash)
// and file name.
func splitImageURL(imageURL string) (directoryURL, imageFilename string, err error) {
	if !strings.HasPrefix(imageURL, "https://") {
		return "", "", fmt.Errorf("image URL %s does not use HTTPS: use the vendor's https:// URL in %s", imageURL, GuideRelativePath)
	}
	lastSlashIndex := strings.LastIndex(imageURL, "/")
	if lastSlashIndex == len(imageURL)-1 || lastSlashIndex < len("https://") {
		return "", "", fmt.Errorf("image URL %s names no file: give the full URL of the image file in %s", imageURL, GuideRelativePath)
	}
	return imageURL[:lastSlashIndex+1], imageURL[lastSlashIndex+1:], nil
}

// failedResult returns a failed CheckResult for a guide row.
func failedResult(guideImage GuideImage, checkError error) CheckResult {
	return CheckResult{Image: guideImage, Status: StatusFailed, Err: checkError}
}
