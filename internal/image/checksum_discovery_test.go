package image

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// archImageFilename is the Arch cloud image name used by the discovery tests.
const archImageFilename = "Arch-Linux-x86_64-cloudimg.qcow2"

// archStyleListing mirrors geo.mirror.pkgbuild.com/images/latest/: one
// <image>.SHA256 file per image, signatures next to them, no SUMS list.
const archStyleListing = `<a href="../">../</a>
<a href="Arch-Linux-x86_64-basic.qcow2">x</a>
<a href="Arch-Linux-x86_64-basic.qcow2.SHA256">x</a>
<a href="Arch-Linux-x86_64-basic.qcow2.sig">x</a>
<a href="Arch-Linux-x86_64-cloudimg.qcow2">x</a>
<a href="Arch-Linux-x86_64-cloudimg.qcow2.SHA256">x</a>
<a href="Arch-Linux-x86_64-cloudimg.qcow2.SHA256.sig">x</a>
<a href="Arch-Linux-x86_64-cloudimg.qcow2.sig">x</a>`

// TestExtractChecksumLinksArchStyle checks that per-image .SHA256 files are
// found, the image's own file is tried first and signatures are skipped.
func TestExtractChecksumLinksArchStyle(t *testing.T) {
	baseURL := "https://mirror.example/images/latest/"
	links := extractChecksumLinksFromHTML(archStyleListing, baseURL, archImageFilename)
	want := []string{
		baseURL + "Arch-Linux-x86_64-cloudimg.qcow2.SHA256",
		baseURL + "Arch-Linux-x86_64-basic.qcow2.SHA256",
	}
	if strings.Join(links, "\n") != strings.Join(want, "\n") {
		t.Errorf("links = %q, want %q", links, want)
	}
}

// TestExtractChecksumLinksSumsLists checks the existing SUMS/CHECKSUM listings
// (Ubuntu, Debian, Fedora) keep their order and foreign hosts are dropped.
func TestExtractChecksumLinksSumsLists(t *testing.T) {
	baseURL := "https://mirror.example/dir/"
	listing := `<a href="CHECKSUM">x</a><a href="SHA512SUMS">x</a><a href="SHA256SUMS">x</a>
<a href="SHA256SUMS.gpg">x</a><a href="https://evil.example/SHA256SUMS">x</a><a href="image.qcow2">x</a>`
	links := extractChecksumLinksFromHTML(listing, baseURL, "image.qcow2")
	want := []string{baseURL + "SHA256SUMS", baseURL + "SHA512SUMS", baseURL + "CHECKSUM"}
	if strings.Join(links, "\n") != strings.Join(want, "\n") {
		t.Errorf("links = %q, want %q", links, want)
	}
}

// newChecksumTestServer serves an image directory listing at /images/ and the
// Arch image's .SHA256 file holding expectedHash.
func newChecksumTestServer(t *testing.T, listing, expectedHash string) *httptest.Server {
	t.Helper()
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var writeError error
		switch request.URL.Path {
		case "/images/":
			_, writeError = fmt.Fprint(writer, listing)
		case "/images/" + archImageFilename + ".SHA256":
			_, writeError = fmt.Fprintf(writer, "%s  %s\n", expectedHash, archImageFilename)
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
		if writeError != nil {
			t.Errorf("writing test response for %s: %v", request.URL.Path, writeError)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

// TestDiscoverChecksumArchStyle checks discovery end to end against an
// Arch-style listing: the hash comes from the image's own .SHA256 file.
func TestDiscoverChecksumArchStyle(t *testing.T) {
	expectedHash := strings.Repeat("ab", 32)
	server := newChecksumTestServer(t, archStyleListing, expectedHash)
	entry := &Entry{ImageURL: server.URL + "/images/" + archImageFilename, Filename: archImageFilename}

	checksum, err := discoverChecksumInfo(server.Client(), entry)
	if err != nil {
		t.Fatalf("discoverChecksumInfo: %v", err)
	}
	if checksum.expectedHash != expectedHash || checksum.algorithm != "sha256" {
		t.Errorf("checksum = %+v, want sha256 %s", checksum, expectedHash)
	}
}

// TestDiscoverChecksumErrorMessages checks both failure messages: a listing
// without checksum links, and a listing that can't be read (wrapped, no <nil>).
func TestDiscoverChecksumErrorMessages(t *testing.T) {
	server := newChecksumTestServer(t, `<a href="only-image.qcow2">x</a>`, "")

	noLinksEntry := &Entry{ImageURL: server.URL + "/images/only-image.qcow2", Filename: "only-image.qcow2"}
	_, noLinksError := discoverChecksumInfo(server.Client(), noLinksEntry)
	if noLinksError == nil {
		t.Fatal("listing without checksum links: expected an error")
	}
	for _, fragment := range []string{"links no checksum file", server.URL + "/images/", "only-image.qcow2.SHA256", "images: entry"} {
		if !strings.Contains(noLinksError.Error(), fragment) {
			t.Errorf("no-links error does not mention %q: %v", fragment, noLinksError)
		}
	}

	unreadableEntry := &Entry{ImageURL: server.URL + "/missing/x.qcow2", Filename: "x.qcow2"}
	_, unreadableError := discoverChecksumInfo(server.Client(), unreadableEntry)
	if unreadableError == nil {
		t.Fatal("unreadable listing: expected an error")
	}
	for _, fragment := range []string{"reading the image directory listing", "HTTP 404"} {
		if !strings.Contains(unreadableError.Error(), fragment) {
			t.Errorf("unreadable-listing error does not mention %q: %v", fragment, unreadableError)
		}
	}
	for _, failure := range []error{noLinksError, unreadableError} {
		if strings.Contains(failure.Error(), "<nil>") {
			t.Errorf("error ends in <nil>: %v", failure)
		}
	}
}

// TestExtractChecksumLinksSkipsUnsupportedAlgorithms checks that lists for
// algorithms vmherd can't verify are never tried, using the Arch ISO archive
// layout (sha256sums.txt next to b2sums.txt) plus MD5/SHA-1 variants.
func TestExtractChecksumLinksSkipsUnsupportedAlgorithms(t *testing.T) {
	baseURL := "https://archive.example/iso/2026.11.01/"
	listing := `<a href="archlinux-x86_64.iso">x</a><a href="b2sums.txt">x</a><a href="sha256sums.txt">x</a>
<a href="MD5SUMS">x</a><a href="SHA1SUMS">x</a><a href="archlinux-x86_64.iso.sha1">x</a><a href="BLAKE2SUMS">x</a>
<a href="SHA384SUMS">x</a><a href="SHA224SUMS">x</a>`
	links := extractChecksumLinksFromHTML(listing, baseURL, "archlinux-x86_64.iso")
	want := []string{baseURL + "sha256sums.txt"}
	if strings.Join(links, "\n") != strings.Join(want, "\n") {
		t.Errorf("links = %q, want %q", links, want)
	}
}
