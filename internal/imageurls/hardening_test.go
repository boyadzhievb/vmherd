package imageurls

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// ubuntuImagePath is the recorded Ubuntu image's path on the test server.
const ubuntuImagePath = "/ubuntu/noble/current/noble-server-cloudimg-amd64.img"

// recordedServerWithOverride serves testdata/vendor like newRecordedVendorServer,
// except that requests overrideHandler claims (it returns true) get its answer.
func recordedServerWithOverride(t *testing.T, overrideHandler func(http.ResponseWriter, *http.Request) bool) (*httptest.Server, *Checker) {
	t.Helper()
	fileServer := http.FileServer(http.Dir(recordedVendorDirectory))
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if !overrideHandler(writer, request) {
			fileServer.ServeHTTP(writer, request)
		}
	}))
	t.Cleanup(server.Close)
	return server, newVendorChecker(server.Client().Transport, vendorRequestTimeout)
}

// TestCheckRequiresImageToAnswer checks that the image URL itself must end in
// a 2xx: a 503 for the current or the replacement file fails without a
// rewrite, and a server refusing HEAD is asked again with a 1-byte ranged GET.
func TestCheckRequiresImageToAnswer(t *testing.T) {
	t.Run("current URL answers 503", func(t *testing.T) {
		server, checker := recordedServerWithOverride(t, func(writer http.ResponseWriter, request *http.Request) bool {
			if request.URL.Path != ubuntuImagePath {
				return false
			}
			writer.WriteHeader(http.StatusServiceUnavailable)
			return true
		})
		guideImage := GuideImage{Name: "ubuntu:noble", URL: server.URL + ubuntuImagePath}
		checkResult := checker.CheckImage(guideImage)
		if checkResult.Status != StatusFailed || checkResult.Err == nil || !strings.Contains(checkResult.Err.Error(), "503") {
			t.Errorf("CheckImage = %s, %v; want failed naming HTTP 503", checkResult.Status, checkResult.Err)
		}
	})

	t.Run("replacement answers 503", func(t *testing.T) {
		server, checker := recordedServerWithOverride(t, func(writer http.ResponseWriter, request *http.Request) bool {
			if request.URL.Path != "/fedora/45/"+fedoraNewFilename {
				return false
			}
			writer.WriteHeader(http.StatusServiceUnavailable)
			return true
		})
		oldURL := server.URL + "/fedora/45/" + fedoraOldFilename
		guideContent := guideWithRows(GuideImage{Name: "fedora:45", URL: oldURL})
		repositoryDirectory := writeTestRepository(t, testRepositoryFiles{GuideRelativePath: guideContent})
		var runOutput bytes.Buffer
		if err := checker.Run(repositoryDirectory, false, &runOutput); err == nil || !strings.Contains(runOutput.String(), "FAILED") {
			t.Errorf("Run = %v, want FAILED for a replacement answering 503:\n%s", err, runOutput.String())
		}
		if readTestRepositoryFile(t, repositoryDirectory, GuideRelativePath) != guideContent {
			t.Errorf("Run rewrote the guide to a replacement that answers 503")
		}
	})

	t.Run("HEAD refused, ranged GET answers", func(t *testing.T) {
		var rangeHeaders []string
		var rangeHeadersLock sync.Mutex
		server, checker := recordedServerWithOverride(t, func(writer http.ResponseWriter, request *http.Request) bool {
			if request.URL.Path != ubuntuImagePath {
				return false
			}
			if request.Method == http.MethodHead {
				writer.WriteHeader(http.StatusMethodNotAllowed)
				return true
			}
			rangeHeadersLock.Lock()
			rangeHeaders = append(rangeHeaders, request.Header.Get("Range"))
			rangeHeadersLock.Unlock()
			writer.WriteHeader(http.StatusPartialContent)
			return true
		})
		checkResult := checker.CheckImage(GuideImage{Name: "ubuntu:noble", URL: server.URL + ubuntuImagePath})
		if checkResult.Status != StatusCurrent {
			t.Errorf("CheckImage = %s, %v; want current after the ranged GET", checkResult.Status, checkResult.Err)
		}
		if strings.Join(rangeHeaders, ",") != "bytes=0-0" {
			t.Errorf("GET Range headers = %q, want one bytes=0-0", rangeHeaders)
		}
	})
}

// TestStaleOutputNamesFinalListingAndChecksumURLs checks that a replacement's
// report shows where the listing and checksum file really came from after
// redirects (Fedora's download host redirects to mirrors).
func TestStaleOutputNamesFinalListingAndChecksumURLs(t *testing.T) {
	server, checker := recordedServerWithOverride(t, func(writer http.ResponseWriter, request *http.Request) bool {
		switch {
		case strings.HasPrefix(request.URL.Path, "/redirector/45/") && !strings.HasSuffix(request.URL.Path, ".qcow2"):
			http.Redirect(writer, request, "/fedora/45/"+strings.TrimPrefix(request.URL.Path, "/redirector/45/"), http.StatusFound)
			return true
		case strings.HasPrefix(request.URL.Path, "/redirector/45/"):
			request.URL.Path = "/fedora/45/" + strings.TrimPrefix(request.URL.Path, "/redirector/45/")
		}
		return false
	})
	guideContent := guideWithRows(GuideImage{Name: "fedora:45", URL: server.URL + "/redirector/45/" + fedoraOldFilename})
	repositoryDirectory := writeTestRepository(t, testRepositoryFiles{GuideRelativePath: guideContent})

	var runOutput bytes.Buffer
	if err := checker.Run(repositoryDirectory, true, &runOutput); err == nil {
		t.Fatalf("Run --check succeeded on a stale URL:\n%s", runOutput.String())
	}
	for _, wantURL := range []string{server.URL + "/fedora/45/", server.URL + "/fedora/45/Fedora-Cloud-45-1.5-x86_64-CHECKSUM"} {
		if !strings.Contains(runOutput.String(), wantURL+"\n") {
			t.Errorf("STALE output does not name the final URL %s:\n%s", wantURL, runOutput.String())
		}
	}
}

// testServerOriginPlaceholder in an inline body is replaced by the test
// server's own https://host:port, for absolute links.
const testServerOriginPlaceholder = "{{origin}}"

// newInlineVendorServer serves fixed bodies by cleaned path, counts the
// requests per path, and answers 404 to anything else.
func newInlineVendorServer(t *testing.T, bodiesByPath map[string]string) (*httptest.Server, *Checker, func(string) int) {
	t.Helper()
	requestCounts := make(map[string]int)
	var requestCountsLock sync.Mutex
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		cleanedPath := path.Clean(request.URL.Path)
		if strings.HasSuffix(request.URL.Path, "/") && cleanedPath != "/" {
			cleanedPath += "/"
		}
		requestCountsLock.Lock()
		requestCounts[cleanedPath]++
		requestCountsLock.Unlock()
		responseBody, known := bodiesByPath[cleanedPath]
		if !known {
			writer.WriteHeader(http.StatusNotFound)
			return
		}
		responseBody = strings.ReplaceAll(responseBody, testServerOriginPlaceholder, "https://"+request.Host)
		if _, writeError := fmt.Fprint(writer, responseBody); writeError != nil {
			t.Errorf("writing test response for %s: %v", cleanedPath, writeError)
		}
	}))
	t.Cleanup(server.Close)
	countRequests := func(requestPath string) int {
		requestCountsLock.Lock()
		defer requestCountsLock.Unlock()
		return requestCounts[requestPath]
	}
	return server, newVendorChecker(server.Client().Transport, vendorRequestTimeout), countRequests
}

// TestChecksumLinksOutsideDirectoryIgnored checks that checksum files linked
// from another directory (../, percent-encoded ../, an absolute URL elsewhere
// on the host) can't vouch for an image, although the server would serve them.
func TestChecksumLinksOutsideDirectoryIgnored(t *testing.T) {
	checksumListingImage := ubuntuImageHash + " *noble-server-cloudimg-amd64.img\n"
	for _, outsideLink := range []string{
		"../44/SHA256SUMS",
		"%2e%2e%2f44%2fSHA256SUMS",
		"..%2F44%2FSHA256SUMS",
		testServerOriginPlaceholder + "/other/SHA256SUMS",
	} {
		t.Run(outsideLink, func(t *testing.T) {
			server, checker, countRequests := newInlineVendorServer(t, map[string]string{
				"/images/45/": `<a href="` + outsideLink + `">x</a>`,
				"/images/45/noble-server-cloudimg-amd64.img": "",
				"/images/44/SHA256SUMS":                      checksumListingImage,
				"/other/SHA256SUMS":                          checksumListingImage,
			})
			checkResult := checker.CheckImage(GuideImage{Name: "ubuntu:noble", URL: server.URL + "/images/45/noble-server-cloudimg-amd64.img"})
			if checkResult.Status != StatusFailed || checkResult.Err == nil || !strings.Contains(checkResult.Err.Error(), "no checksum file") {
				t.Errorf("CheckImage = %s, %v; want failed: no checksum file in the image's own directory", checkResult.Status, checkResult.Err)
			}
			if fetchedOutside := countRequests("/images/44/SHA256SUMS") + countRequests("/other/SHA256SUMS"); fetchedOutside != 0 {
				t.Errorf("fetched a checksum file outside the image's directory %d times", fetchedOutside)
			}
		})
	}

	directoryURL := "https://vendor.test/images/45/"
	for _, linkedURL := range []string{
		directoryURL + "%2e%2e%2f44%2fSHA256SUMS",
		directoryURL + "sub%2FSHA256SUMS",
		directoryURL + "..\\44\\SHA256SUMS",
		directoryURL + "SHA256SUMS?x=1",
		"https://other.test/images/45/SHA256SUMS",
	} {
		if keptURLs := checksumURLsInDirectory([]string{linkedURL}, directoryURL); len(keptURLs) != 0 {
			t.Errorf("checksumURLsInDirectory kept %q, want it rejected", linkedURL)
		}
	}
	if keptURLs := checksumURLsInDirectory([]string{directoryURL + "SHA256SUMS"}, directoryURL); len(keptURLs) != 1 {
		t.Errorf("checksumURLsInDirectory rejected %sSHA256SUMS", directoryURL)
	}
}

// TestChecksumFileFetchFailureSkipped checks that one checksum file that
// can't be fetched doesn't fail the image when another lists it, and that the
// failure names the skipped files when none does.
func TestChecksumFileFetchFailureSkipped(t *testing.T) {
	imageListing := ubuntuImageHash + " *noble-server-cloudimg-amd64.img\n"
	server, checker, _ := newInlineVendorServer(t, map[string]string{
		"/images/": `<a href="a-SHA256SUMS">x</a> <a href="b-SHA256SUMS">x</a>`,
		"/images/noble-server-cloudimg-amd64.img": "",
		"/images/b-SHA256SUMS":                    imageListing,
		"/broken/":                                `<a href="a-SHA256SUMS">x</a> <a href="b-SHA256SUMS">x</a>`,
		"/broken/noble-server-cloudimg-amd64.img": "",
	})

	checkResult := checker.CheckImage(GuideImage{Name: "ubuntu:noble", URL: server.URL + "/images/noble-server-cloudimg-amd64.img"})
	if checkResult.Status != StatusCurrent {
		t.Errorf("CheckImage with one unreadable checksum file = %s, %v; want current from the other", checkResult.Status, checkResult.Err)
	}

	checkResult = checker.CheckImage(GuideImage{Name: "ubuntu:noble", URL: server.URL + "/broken/noble-server-cloudimg-amd64.img"})
	if checkResult.Status != StatusFailed || checkResult.Err == nil {
		t.Fatalf("CheckImage with no readable checksum file = %s; want failed", checkResult.Status)
	}
	for _, skippedURL := range []string{server.URL + "/broken/a-SHA256SUMS", server.URL + "/broken/b-SHA256SUMS"} {
		if !strings.Contains(checkResult.Err.Error(), skippedURL) {
			t.Errorf("error = %v, want it to name the skipped %s", checkResult.Err, skippedURL)
		}
	}
}

// TestOwnChecksumFileReadFirstUnderCap checks that the image's own
// <image>.SHA256 file is read first even when more than
// maxChecksumFilesPerDirectory other checksum files are linked before it.
func TestOwnChecksumFileReadFirstUnderCap(t *testing.T) {
	imageFilename := "Arch-Linux-x86_64-cloudimg.qcow2"
	bodiesByPath := map[string]string{
		"/images/" + imageFilename:             "",
		"/images/" + imageFilename + ".SHA256": ubuntuImageHash + "  " + imageFilename + "\n",
	}
	var listingText strings.Builder
	for checksumFileIndex := 0; checksumFileIndex < maxChecksumFilesPerDirectory+1; checksumFileIndex++ {
		otherFilename := fmt.Sprintf("Arch-Linux-x86_64-other-%02d.qcow2.SHA256", checksumFileIndex)
		fmt.Fprintf(&listingText, `<a href="%s">x</a> `, otherFilename)
		bodiesByPath["/images/"+otherFilename] = strings.Repeat("cd", 32) + "  other.qcow2\n"
	}
	fmt.Fprintf(&listingText, `<a href="%s.SHA256">x</a>`, imageFilename)
	bodiesByPath["/images/"] = listingText.String()
	server, checker, countRequests := newInlineVendorServer(t, bodiesByPath)

	checkResult := checker.CheckImage(GuideImage{Name: "arch:latest", URL: server.URL + "/images/" + imageFilename})
	if checkResult.Status != StatusCurrent || checkResult.Checksum.Hash != ubuntuImageHash {
		t.Errorf("CheckImage = %s, %v, %+v; want current with the own file's hash", checkResult.Status, checkResult.Err, checkResult.Checksum)
	}
	if ownFileRequests := countRequests("/images/" + imageFilename + ".SHA256"); ownFileRequests != 1 {
		t.Errorf("own checksum file fetched %d times, want 1", ownFileRequests)
	}
	for requestPath := range bodiesByPath {
		if strings.Contains(requestPath, "-other-") && countRequests(requestPath) != 0 {
			t.Errorf("%s was fetched before the image's own checksum file settled the check", requestPath)
		}
	}
}

// TestFindReplacementKeepsFixedDigitRuns checks that digit runs that belong to
// the architecture, the format or the release never count as a newer build,
// and that an absurdly long build number is not taken.
func TestFindReplacementKeepsFixedDigitRuns(t *testing.T) {
	for _, candidateFilename := range []string{
		"Fedora-Cloud-Base-Generic-45-1.4.x99_64.qcow2",
		"Fedora-Cloud-Base-Generic-45-1.4.x86_64.qcow3",
		"Fedora-Cloud-Base-Generic-46-1.4.x86_64.qcow2",
		"Fedora-Cloud-Base-Generic-45-1." + strings.Repeat("9", 100) + ".x86_64.qcow2",
	} {
		if gotFilename, err := selectNewerBuild(fedoraOldFilename, fedoraTestDirectoryURL, []string{candidateFilename}); !errors.Is(err, errNoNewerBuild) {
			t.Errorf("selectNewerBuild(%s) = %q, %v; want errNoNewerBuild", candidateFilename, gotFilename, err)
		}
	}
	ubuntuDirectoryURL := "https://vendor.test/noble/current/"
	if gotFilename, err := selectNewerBuild("noble-server-cloudimg-amd64.img", ubuntuDirectoryURL, []string{"noble-server-cloudimg-amd65.img"}); !errors.Is(err, errNoNewerBuild) {
		t.Errorf("selectNewerBuild(amd65) = %q, %v; want errNoNewerBuild", gotFilename, err)
	}
	if gotFilename, err := selectNewerBuild(fedoraOldFilename, fedoraTestDirectoryURL, []string{fedoraNewFilename}); err != nil || gotFilename != fedoraNewFilename {
		t.Errorf("selectNewerBuild(1.5) = %q, %v; want %s", gotFilename, err, fedoraNewFilename)
	}
}

// TestRewriteLeavesLongerURLs checks that the old URL is replaced only where
// it ends, not inside a longer URL that starts with it.
func TestRewriteLeavesLongerURLs(t *testing.T) {
	oldURL := "https://vendor.test/noble/noble-server-cloudimg-amd64.img"
	newURL := "https://vendor.test/noble/noble-server-cloudimg-amd64-v2.img"
	guideContent := "| `ubuntu:noble` | " + oldURL + " | |\nManifest: " + oldURL + ".manifest\n" +
		"See (" + oldURL + ") and <" + oldURL + "> and `" + oldURL + "`\n" + oldURL
	repositoryDirectory := writeTestRepository(t, testRepositoryFiles{GuideRelativePath: guideContent})

	if _, err := RewriteURL(repositoryDirectory, oldURL, newURL); err != nil {
		t.Fatalf("RewriteURL: %v", err)
	}
	wantContent := "| `ubuntu:noble` | " + newURL + " | |\nManifest: " + oldURL + ".manifest\n" +
		"See (" + newURL + ") and <" + newURL + "> and `" + newURL + "`\n" + newURL
	if gotContent := readTestRepositoryFile(t, repositoryDirectory, GuideRelativePath); gotContent != wantContent {
		t.Errorf("guide after rewrite =\n%s\nwant\n%s", gotContent, wantContent)
	}
}

// TestRewriteNamesFilesWrittenBeforeFailure checks that when a later file
// can't be written, the error names the files already rewritten.
func TestRewriteNamesFilesWrittenBeforeFailure(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: a read-only directory doesn't stop root from writing, so the failed write can't be provoked")
	}
	oldURL := "https://vendor.test/45/" + fedoraOldFilename
	repositoryDirectory := writeTestRepository(t, testRepositoryFiles{
		"internal/config/images.go": "\"" + oldURL + "\"\n",
		GuideRelativePath:           "| `fedora:45` | " + oldURL + " | |\n",
		"test/e2e/e2e_test.go":      "\"" + oldURL + "\"\n",
	})
	readOnlyDirectory := filepath.Join(repositoryDirectory, "test/e2e")
	if err := os.Chmod(readOnlyDirectory, 0o555); err != nil {
		t.Fatalf("making %s read-only: %v", readOnlyDirectory, err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(readOnlyDirectory, 0o755); err != nil {
			t.Errorf("restoring %s: %v", readOnlyDirectory, err)
		}
	})

	_, err := RewriteURL(repositoryDirectory, oldURL, "https://vendor.test/45/"+fedoraNewFilename)
	if err == nil {
		t.Fatal("RewriteURL succeeded although test/e2e is read-only")
	}
	for _, wantText := range []string{"test/e2e/e2e_test.go", "already rewritten: internal/config/images.go, docs/images.md"} {
		if !strings.Contains(err.Error(), wantText) {
			t.Errorf("error = %v, want it to contain %q", err, wantText)
		}
	}
}

// TestVendorClientRefusesUnsafeRedirects checks the checker's client: a
// redirect to http:// and a chain longer than maxVendorRedirects both fail.
func TestVendorClientRefusesUnsafeRedirects(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case strings.HasPrefix(request.URL.Path, "/to-http/"):
			http.Redirect(writer, request, "http://"+strings.TrimPrefix(server.URL, "https://")+"/plain.qcow2", http.StatusFound)
		case strings.HasPrefix(request.URL.Path, "/hop/"):
			var hopNumber int
			if _, scanError := fmt.Sscanf(request.URL.Path, "/hop/%d/", &hopNumber); scanError != nil {
				t.Errorf("reading hop number from %s: %v", request.URL.Path, scanError)
			}
			http.Redirect(writer, request, fmt.Sprintf("/hop/%d/image.qcow2", hopNumber+1), http.StatusFound)
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	checker := newVendorChecker(server.Client().Transport, vendorRequestTimeout)

	testCases := []struct {
		caseName    string
		imageURL    string
		wantInError string
	}{
		{"redirect to http", server.URL + "/to-http/image.qcow2", "non-HTTPS"},
		{"too many hops", server.URL + "/hop/0/image.qcow2", fmt.Sprintf("stopped after %d redirects", maxVendorRedirects)},
	}
	for _, testCase := range testCases {
		t.Run(testCase.caseName, func(t *testing.T) {
			checkResult := checker.CheckImage(GuideImage{Name: "fedora:45", URL: testCase.imageURL})
			if checkResult.Status != StatusFailed || checkResult.Err == nil || !strings.Contains(checkResult.Err.Error(), testCase.wantInError) {
				t.Errorf("CheckImage = %s, %v; want failed with %q", checkResult.Status, checkResult.Err, testCase.wantInError)
			}
		})
	}
}

// TestChecksumLinksDeduplicatedAndCapped checks that a checksum file linked
// several times is read once, and that no more than
// maxChecksumFilesPerDirectory checksum files are read from one directory.
func TestChecksumLinksDeduplicatedAndCapped(t *testing.T) {
	linkedChecksumFileCount := maxChecksumFilesPerDirectory + 4
	bodiesByPath := map[string]string{"/images/unlisted.qcow2": ""}
	var listingText strings.Builder
	listingText.WriteString(`<a href="SHA256SUMS">x</a> <a href="SHA256SUMS">x</a> <a href="SHA256SUMS">x</a>`)
	bodiesByPath["/images/SHA256SUMS"] = strings.Repeat("ab", 32) + " *other.qcow2\n"
	for checksumFileIndex := 1; checksumFileIndex < linkedChecksumFileCount; checksumFileIndex++ {
		checksumFilename := fmt.Sprintf("part-%02d.SHA256SUMS", checksumFileIndex)
		fmt.Fprintf(&listingText, ` <a href="%s">x</a>`, checksumFilename)
		bodiesByPath["/images/"+checksumFilename] = strings.Repeat("cd", 32) + " *other.qcow2\n"
	}
	bodiesByPath["/images/"] = listingText.String()
	server, checker, countRequests := newInlineVendorServer(t, bodiesByPath)

	checker.CheckImage(GuideImage{Name: "test:unlisted", URL: server.URL + "/images/unlisted.qcow2"})

	if sumsRequests := countRequests("/images/SHA256SUMS"); sumsRequests != 1 {
		t.Errorf("SHA256SUMS, linked 3 times, was fetched %d times, want 1", sumsRequests)
	}
	fetchedChecksumFiles := 0
	for requestPath := range bodiesByPath {
		if strings.HasSuffix(requestPath, "SHA256SUMS") {
			fetchedChecksumFiles += min(countRequests(requestPath), 1)
		}
	}
	if fetchedChecksumFiles != maxChecksumFilesPerDirectory {
		t.Errorf("fetched %d checksum files, want the cap of %d", fetchedChecksumFiles, maxChecksumFilesPerDirectory)
	}
}

// TestReplacementWithSkippedChecksumFileFails checks that a replacement is
// not taken while any checksum file of the directory couldn't be read: the
// skipped file might list a competing build. The image fails, nothing is
// rewritten, and the error names the skipped file and says to rerun.
func TestReplacementWithSkippedChecksumFileFails(t *testing.T) {
	server, checker, _ := newInlineVendorServer(t, map[string]string{
		"/images/45/":                     `<a href="a-SHA256SUMS">x</a> <a href="b-SHA256SUMS">x</a>`,
		"/images/45/b-SHA256SUMS":         fedoraNewHash + "  " + fedoraNewFilename + "\n",
		"/images/45/" + fedoraNewFilename: "",
	})
	oldURL := server.URL + "/images/45/" + fedoraOldFilename
	guideContent := guideWithRows(GuideImage{Name: "fedora:45", URL: oldURL})
	repositoryDirectory := writeTestRepository(t, testRepositoryFiles{GuideRelativePath: guideContent})

	var runOutput bytes.Buffer
	if err := checker.Run(repositoryDirectory, false, &runOutput); err == nil {
		t.Fatalf("Run succeeded with a skipped checksum file:\n%s", runOutput.String())
	}
	if !strings.Contains(runOutput.String(), "FAILED") || strings.Contains(runOutput.String(), "STALE") {
		t.Errorf("output does not report FAILED (and only FAILED) for fedora:45:\n%s", runOutput.String())
	}
	for _, wantText := range []string{server.URL + "/images/45/a-SHA256SUMS", "run the release again"} {
		if !strings.Contains(runOutput.String(), wantText) {
			t.Errorf("output lacks %q:\n%s", wantText, runOutput.String())
		}
	}
	if readTestRepositoryFile(t, repositoryDirectory, GuideRelativePath) != guideContent {
		t.Errorf("Run rewrote the guide although a checksum file was skipped")
	}
}
