package imageurls

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// recordedVendorDirectory holds the recorded vendor directory listings,
// checksum files and (empty) image files the test server serves.
const recordedVendorDirectory = "testdata/vendor"

// testRequestTimeout is the client timeout of the vendor-timeout test: short,
// because the test server never answers.
const testRequestTimeout = 200 * time.Millisecond

// fedoraOldFilename is the Fedora 45 build the tests treat as published before.
const fedoraOldFilename = "Fedora-Cloud-Base-Generic-45-1.4.x86_64.qcow2"

// fedoraNewFilename is the newer Fedora 45 build in the recorded CHECKSUM files.
const fedoraNewFilename = "Fedora-Cloud-Base-Generic-45-1.5.x86_64.qcow2"

// fedoraNewHash is the SHA256 the recorded CHECKSUM files give fedoraNewFilename.
const fedoraNewHash = "3333333333333333333333333333333333333333333333333333333333333333"

// fedoraTestDirectoryURL is a Fedora 45 image directory for selection tests
// that need no server; its path holds the release number 45 and x86_64.
const fedoraTestDirectoryURL = "https://vendor.test/pub/fedora/linux/releases/45/Cloud/x86_64/images/"

// ubuntuImageHash is the SHA256 the recorded SHA256SUMS gives the noble image.
const ubuntuImageHash = "1111111111111111111111111111111111111111111111111111111111111111"

// newRecordedVendorServer serves testdata/vendor over TLS: a directory answers
// with its recorded index.html, a missing file with 404.
func newRecordedVendorServer(t *testing.T) (*httptest.Server, *Checker) {
	t.Helper()
	server := httptest.NewTLSServer(http.FileServer(http.Dir(recordedVendorDirectory)))
	t.Cleanup(server.Close)
	return server, newVendorChecker(server.Client().Transport, vendorRequestTimeout)
}

// testRepositoryFiles are the files a test repository holds, by relative path.
type testRepositoryFiles map[string]string

// writeTestRepository writes files into a new temporary repository directory.
func writeTestRepository(t *testing.T, repositoryFiles testRepositoryFiles) string {
	t.Helper()
	repositoryDirectory := t.TempDir()
	for relativePath, fileContent := range repositoryFiles {
		absolutePath := filepath.Join(repositoryDirectory, relativePath)
		if err := os.MkdirAll(filepath.Dir(absolutePath), 0o755); err != nil {
			t.Fatalf("creating directory for %s: %v", relativePath, err)
		}
		if err := os.WriteFile(absolutePath, []byte(fileContent), 0o644); err != nil {
			t.Fatalf("writing %s: %v", relativePath, err)
		}
	}
	return repositoryDirectory
}

// readTestRepositoryFile returns one file of a test repository.
func readTestRepositoryFile(t *testing.T, repositoryDirectory, relativePath string) string {
	t.Helper()
	fileContent, err := os.ReadFile(filepath.Join(repositoryDirectory, relativePath))
	if err != nil {
		t.Fatalf("reading %s: %v", relativePath, err)
	}
	return string(fileContent)
}

// guideWithRows returns an image guide whose Booted URLs table has the rows.
func guideWithRows(guideImages ...GuideImage) string {
	var guideText strings.Builder
	guideText.WriteString("# Images\n\nSome text.\n\n## Booted URLs\n\n| Image | URL | Notes |\n|---|---|---|\n")
	for _, guideImage := range guideImages {
		guideText.WriteString("| `" + guideImage.Name + "` | " + guideImage.URL + " | note |\n")
	}
	return guideText.String()
}

// TestParseGuideTable checks that the rows under '## Booted URLs' are read
// and that a guide without that table fails with the heading named.
func TestParseGuideTable(t *testing.T) {
	guideImages, err := ParseGuideTable(guideWithRows(
		GuideImage{Name: "ubuntu:noble", URL: "https://example.test/a.img"},
		GuideImage{Name: "fedora:45", URL: "https://example.test/b.qcow2"},
	))
	if err != nil {
		t.Fatalf("ParseGuideTable: %v", err)
	}
	if len(guideImages) != 2 || guideImages[0].Name != "ubuntu:noble" || guideImages[1].URL != "https://example.test/b.qcow2" {
		t.Errorf("guide images = %+v, want ubuntu:noble and fedora:45", guideImages)
	}

	for _, guideContent := range []string{"# Images\n\nNo table here.\n", "## Booted URLs\n\n| Image | URL | Notes |\n|---|---|---|\n"} {
		if _, err := ParseGuideTable(guideContent); err == nil || !strings.Contains(err.Error(), "no image table under '## Booted URLs' in docs/images.md") {
			t.Errorf("ParseGuideTable(%q) error = %v, want the missing-table error", guideContent, err)
		}
	}
}

// TestCheckReportsCurrentURL checks that a URL that answers and is listed in
// its directory's checksum file is current, and that Run prints one OK line.
func TestCheckReportsCurrentURL(t *testing.T) {
	server, checker := newRecordedVendorServer(t)
	guideImage := GuideImage{Name: "ubuntu:noble", URL: server.URL + "/ubuntu/noble/current/noble-server-cloudimg-amd64.img"}

	checkResult := checker.CheckImage(guideImage)
	if checkResult.Status != StatusCurrent || checkResult.Err != nil {
		t.Fatalf("CheckImage status = %s, error = %v, want current", checkResult.Status, checkResult.Err)
	}
	if checkResult.Checksum.Hash != ubuntuImageHash || checkResult.Checksum.Algorithm != "sha256" {
		t.Errorf("checksum = %+v, want sha256 %s", checkResult.Checksum, ubuntuImageHash)
	}

	repositoryDirectory := writeTestRepository(t, testRepositoryFiles{GuideRelativePath: guideWithRows(guideImage)})
	var runOutput bytes.Buffer
	if err := checker.Run(repositoryDirectory, false, &runOutput); err != nil {
		t.Fatalf("Run: %v\n%s", err, runOutput.String())
	}
	if !strings.Contains(runOutput.String(), "OK") || !strings.Contains(runOutput.String(), "ubuntu:noble") {
		t.Errorf("Run output has no OK line for ubuntu:noble:\n%s", runOutput.String())
	}
}

// TestCheckFailsOnVendorError checks that a 5xx answer and a timeout fail the
// check naming the URL, and that Run then writes nothing.
func TestCheckFailsOnVendorError(t *testing.T) {
	brokenServer := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(brokenServer.Close)
	silentServer := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		<-request.Context().Done()
	}))
	t.Cleanup(silentServer.Close)

	testCases := []struct {
		caseName    string
		checker     *Checker
		imageURL    string
		wantInError string
	}{
		{"5xx", newVendorChecker(brokenServer.Client().Transport, vendorRequestTimeout), brokenServer.URL + "/fedora/45/" + fedoraOldFilename, "503"},
		{"timeout", newVendorChecker(silentServer.Client().Transport, testRequestTimeout), silentServer.URL + "/fedora/45/" + fedoraOldFilename, "Timeout"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.caseName, func(t *testing.T) {
			guideImage := GuideImage{Name: "fedora:45", URL: testCase.imageURL}
			checkResult := testCase.checker.CheckImage(guideImage)
			if checkResult.Status != StatusFailed || checkResult.Err == nil {
				t.Fatalf("CheckImage status = %s, error = %v, want failed", checkResult.Status, checkResult.Err)
			}
			if !strings.Contains(checkResult.Err.Error(), testCase.imageURL) || !strings.Contains(checkResult.Err.Error(), testCase.wantInError) {
				t.Errorf("error = %v, want it to name %s and %q", checkResult.Err, testCase.imageURL, testCase.wantInError)
			}

			guideContent := guideWithRows(guideImage)
			imagesGoContent := "URL: \"" + testCase.imageURL + "\"\n"
			repositoryDirectory := writeTestRepository(t, testRepositoryFiles{GuideRelativePath: guideContent, "internal/config/images.go": imagesGoContent})
			var runOutput bytes.Buffer
			if err := testCase.checker.Run(repositoryDirectory, false, &runOutput); err == nil {
				t.Errorf("Run succeeded on a failing vendor:\n%s", runOutput.String())
			}
			if readTestRepositoryFile(t, repositoryDirectory, GuideRelativePath) != guideContent ||
				readTestRepositoryFile(t, repositoryDirectory, "internal/config/images.go") != imagesGoContent {
				t.Errorf("Run changed files although the check failed")
			}
		})
	}
}

// TestFindReplacementPicksNewerBuild checks that a gone or no longer listed
// Fedora 1.4 is replaced by the Generic 1.5 qcow2 from the same directory,
// never by the UEFI-UKI or raw.xz variants.
func TestFindReplacementPicksNewerBuild(t *testing.T) {
	server, checker := newRecordedVendorServer(t)
	for _, scenarioDirectory := range []string{"/fedora/45/", "/fedora-unlisted/45/"} {
		t.Run(scenarioDirectory, func(t *testing.T) {
			checkResult := checker.CheckImage(GuideImage{Name: "fedora:45", URL: server.URL + scenarioDirectory + fedoraOldFilename})
			if checkResult.Status != StatusStale || checkResult.Err != nil {
				t.Fatalf("CheckImage status = %s, error = %v, want stale with a replacement", checkResult.Status, checkResult.Err)
			}
			if wantURL := server.URL + scenarioDirectory + fedoraNewFilename; checkResult.ReplacementURL != wantURL {
				t.Errorf("replacement = %s, want %s", checkResult.ReplacementURL, wantURL)
			}
			if checkResult.Checksum.Hash != fedoraNewHash {
				t.Errorf("replacement hash = %s, want %s", checkResult.Checksum.Hash, fedoraNewHash)
			}
		})
	}

	selectionCases := []struct {
		oldFilename     string
		listedFilenames []string
		wantFilename    string
	}{
		{fedoraOldFilename, []string{"Fedora-Cloud-Base-UEFI-UKI-45-1.6.x86_64.qcow2", "Fedora-Cloud-Base-Generic-45-1.5.x86_64.raw.xz", fedoraNewFilename}, fedoraNewFilename},
		{fedoraOldFilename, []string{"Fedora-Cloud-Base-Generic-45-1.5.x86_64.qcow2", "Fedora-Cloud-Base-Generic-45-1.10.x86_64.qcow2"}, "Fedora-Cloud-Base-Generic-45-1.10.x86_64.qcow2"},
		{fedoraOldFilename, []string{fedoraOldFilename, fedoraNewFilename}, fedoraNewFilename},
	}
	for _, selectionCase := range selectionCases {
		gotFilename, err := selectNewerBuild(selectionCase.oldFilename, fedoraTestDirectoryURL, selectionCase.listedFilenames)
		if err != nil || gotFilename != selectionCase.wantFilename {
			t.Errorf("selectNewerBuild(%s, %v) = %q, %v; want %s", selectionCase.oldFilename, selectionCase.listedFilenames, gotFilename, err, selectionCase.wantFilename)
		}
	}
}

// TestFindReplacementGone checks that no match, a directory that is gone and
// a directory without a checksum file each fail naming the image, URL and
// directory, without a replacement.
func TestFindReplacementGone(t *testing.T) {
	server, checker := newRecordedVendorServer(t)
	for _, scenarioDirectory := range []string{"/fedora-gone/45/", "/fedora/44/", "/fedora-nochecksum/45/"} {
		t.Run(scenarioDirectory, func(t *testing.T) {
			imageURL := server.URL + scenarioDirectory + fedoraOldFilename
			checkResult := checker.CheckImage(GuideImage{Name: "fedora:45", URL: imageURL})
			if checkResult.Status != StatusFailed || checkResult.ReplacementURL != "" {
				t.Fatalf("CheckImage status = %s, replacement = %q, want failed without replacement", checkResult.Status, checkResult.ReplacementURL)
			}
			errorText := checkResult.Err.Error()
			if !strings.Contains(errorText, "fedora:45") || !strings.Contains(errorText, imageURL) || !strings.Contains(errorText, server.URL+scenarioDirectory+" ") {
				t.Errorf("error = %s, want it to name fedora:45, %s and the directory", errorText, imageURL)
			}
		})
	}

	if _, err := selectNewerBuild(fedoraOldFilename, fedoraTestDirectoryURL, []string{"Fedora-Cloud-Base-Generic-45-1.3.x86_64.qcow2", fedoraOldFilename}); !errors.Is(err, errNoNewerBuild) {
		t.Errorf("selectNewerBuild with only an older build: error = %v, want errNoNewerBuild", err)
	}
}

// TestFindReplacementAmbiguous checks that two equal builds, or several newer
// ones next to the still-listed old file, fail instead of guessing.
func TestFindReplacementAmbiguous(t *testing.T) {
	server, checker := newRecordedVendorServer(t)
	for _, scenarioDirectory := range []string{"/fedora-tie/45/", "/fedora-old-listed/45/"} {
		t.Run(scenarioDirectory, func(t *testing.T) {
			imageURL := server.URL + scenarioDirectory + fedoraOldFilename
			checkResult := checker.CheckImage(GuideImage{Name: "fedora:45", URL: imageURL})
			if checkResult.Status != StatusFailed || checkResult.ReplacementURL != "" {
				t.Fatalf("CheckImage status = %s, replacement = %q, want failed without replacement", checkResult.Status, checkResult.ReplacementURL)
			}
			if !errors.Is(checkResult.Err, errAmbiguousBuild) || !strings.Contains(checkResult.Err.Error(), imageURL) {
				t.Errorf("error = %v, want errAmbiguousBuild naming %s", checkResult.Err, imageURL)
			}
		})
	}
}

// TestRewriteReplacesURLInFiles checks the literal rewrite: all three target
// files holding the old URL are changed, a target file without it is skipped,
// and a guide without it fails with nothing written.
func TestRewriteReplacesURLInFiles(t *testing.T) {
	oldURL := "https://vendor.test/45/" + fedoraOldFilename
	newURL := "https://vendor.test/45/" + fedoraNewFilename
	repositoryDirectory := writeTestRepository(t, testRepositoryFiles{
		"internal/config/images.go": "{Name: \"fedora:45\", URL: \"" + oldURL + "\"},\n",
		GuideRelativePath:           "| `fedora:45` | " + oldURL + " | pinned |\n",
		"test/e2e/e2e_test.go":      "{\"fedora:45\", \"" + oldURL + "\", true},\n",
	})

	rewrittenFiles, err := RewriteURL(repositoryDirectory, oldURL, newURL)
	if err != nil {
		t.Fatalf("RewriteURL: %v", err)
	}
	if strings.Join(rewrittenFiles, ",") != "internal/config/images.go,docs/images.md,test/e2e/e2e_test.go" {
		t.Errorf("rewritten files = %v, want images.go, docs/images.md and e2e_test.go", rewrittenFiles)
	}
	for _, relativePath := range []string{"internal/config/images.go", GuideRelativePath, "test/e2e/e2e_test.go"} {
		fileContent := readTestRepositoryFile(t, repositoryDirectory, relativePath)
		if strings.Contains(fileContent, oldURL) || !strings.Contains(fileContent, newURL) {
			t.Errorf("%s still holds the old URL or lacks the new one:\n%s", relativePath, fileContent)
		}
	}

	imagesGoWithoutURL := "{Name: \"ubuntu:noble\", URL: \"https://other.test/a.img\"},\n"
	repositoryWithSkippedFile := writeTestRepository(t, testRepositoryFiles{
		"internal/config/images.go": imagesGoWithoutURL,
		GuideRelativePath:           "| `fedora:45` | " + oldURL + " | pinned |\n",
	})
	rewrittenFiles, err = RewriteURL(repositoryWithSkippedFile, oldURL, newURL)
	if err != nil || strings.Join(rewrittenFiles, ",") != GuideRelativePath {
		t.Errorf("RewriteURL with the URL only in the guide = %v, %v; want only %s", rewrittenFiles, err, GuideRelativePath)
	}
	if readTestRepositoryFile(t, repositoryWithSkippedFile, "internal/config/images.go") != imagesGoWithoutURL {
		t.Errorf("RewriteURL changed images.go, which lacks the old URL")
	}

	imagesGoContent := "{Name: \"fedora:45\", URL: \"" + oldURL + "\"},\n"
	repositoryWithoutGuideURL := writeTestRepository(t, testRepositoryFiles{
		"internal/config/images.go": imagesGoContent,
		GuideRelativePath:           "| `fedora:45` | https://vendor.test/other.qcow2 | |\n",
	})
	if _, err := RewriteURL(repositoryWithoutGuideURL, oldURL, newURL); err == nil || !strings.Contains(err.Error(), GuideRelativePath) {
		t.Errorf("RewriteURL with the URL missing from the guide: error = %v, want one naming %s", err, GuideRelativePath)
	}
	if readTestRepositoryFile(t, repositoryWithoutGuideURL, "internal/config/images.go") != imagesGoContent {
		t.Errorf("RewriteURL wrote images.go although the guide lacked the URL")
	}
}

// TestCheckOnlyWritesNothing checks that --check reports the stale URL and
// its replacement, writes no file and fails; without --check, Run rewrites.
func TestCheckOnlyWritesNothing(t *testing.T) {
	server, checker := newRecordedVendorServer(t)
	oldURL := server.URL + "/fedora/45/" + fedoraOldFilename
	newURL := server.URL + "/fedora/45/" + fedoraNewFilename
	guideContent := guideWithRows(GuideImage{Name: "fedora:45", URL: oldURL})
	imagesGoContent := "{Name: \"fedora:45\", URL: \"" + oldURL + "\"},\n"
	repositoryDirectory := writeTestRepository(t, testRepositoryFiles{GuideRelativePath: guideContent, "internal/config/images.go": imagesGoContent})

	var checkOutput bytes.Buffer
	if err := checker.Run(repositoryDirectory, true, &checkOutput); err == nil {
		t.Errorf("Run --check succeeded on a stale URL:\n%s", checkOutput.String())
	}
	for _, wantText := range []string{"fedora:45", oldURL, newURL, fedoraNewHash} {
		if !strings.Contains(checkOutput.String(), wantText) {
			t.Errorf("--check output lacks %q:\n%s", wantText, checkOutput.String())
		}
	}
	if readTestRepositoryFile(t, repositoryDirectory, GuideRelativePath) != guideContent ||
		readTestRepositoryFile(t, repositoryDirectory, "internal/config/images.go") != imagesGoContent {
		t.Errorf("Run --check changed files")
	}

	var rewriteOutput bytes.Buffer
	if err := checker.Run(repositoryDirectory, false, &rewriteOutput); err == nil {
		t.Errorf("Run succeeded although it rewrote a stale URL:\n%s", rewriteOutput.String())
	}
	if !strings.Contains(readTestRepositoryFile(t, repositoryDirectory, "internal/config/images.go"), newURL) {
		t.Errorf("Run without --check did not rewrite images.go:\n%s", rewriteOutput.String())
	}
	if !strings.Contains(rewriteOutput.String(), "git diff") {
		t.Errorf("Run output does not tell the maintainer to review the diff:\n%s", rewriteOutput.String())
	}
}
