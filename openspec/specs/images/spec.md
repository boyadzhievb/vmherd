# images Specification

## Purpose

vmherd boots official cloud images downloaded directly from distro vendors over HTTPS, verified against the vendor's published checksum and cached locally. There is no vmherd image registry.

## Requirements

### Requirement: Image names resolve through the Herdfile
An image name SHALL have the form `distro:codename` and SHALL resolve to the URL given for it under the Herdfile's `images:`. `vmherd pull` and `vmherd list-images` SHALL use the built-in images (ubuntu noble and resolute, fedora 44 and 45) when the Herdfile has no `images:`.

#### Scenario: Defined image
- **WHEN** `ubuntu:noble` is defined under `images:`
- **THEN** it resolves to that URL and the file name at its end
- **Test:** internal/image/TestResolveFromImages

#### Scenario: Unknown image
- **WHEN** an image name is not defined
- **THEN** the error names it, lists the defined images of the same distro, and shows the `images:` entry to add
- **Test:** internal/image/TestResolveUnknownImage

#### Scenario: Malformed name
- **WHEN** the name has no `:`
- **THEN** the error asks for `distro:codename`
- **Test:** internal/image/TestResolveBadFormat

#### Scenario: Listing
- **WHEN** the user runs `vmherd list-images`
- **THEN** every image is listed with a description and whether it is cached
- **Test:** internal/image/TestListAll, internal/image/TestCachedPath, internal/image/TestIsCachedMissing

### Requirement: HTTPS only
vmherd SHALL download images and checksum files only over HTTPS and SHALL refuse any other scheme.

#### Scenario: HTTP URL
- **WHEN** an image URL starts with `http://`
- **THEN** the download is refused with "refusing to download from non-HTTPS URL"
- **Test:** internal/image/TestDownloadFileRejectsNonHTTPS

### Requirement: Withdrawn images are named
Before downloading, vmherd SHALL check the image URL and, when the vendor answers 404 or 410, SHALL say the file is no longer published and point at the vendor directory to find the current file name.

#### Scenario: Vendor removed the file
- **WHEN** the image URL returns HTTP 404
- **THEN** the error names the URL, the status and the directory to look in
- **Test:** internal/image/TestCheckImageURLAvailable

### Requirement: Verified during download
vmherd SHALL find the vendor's checksum file in the image's directory listing (names containing SUM, SHA256 or SHA512), SHALL compute the SHA-256 or SHA-512 hash while downloading, and SHALL move the file into the cache only when the hash matches. Lines for other files, including near-matches of the file name, SHALL never be used.

#### Scenario: Checksum discovery
- **WHEN** the vendor directory links `SHA256SUMS` or `<file>.SHA256` (Arch style)
- **THEN** vmherd finds the right checksum file and skips unsupported algorithms such as MD5
- **Test:** internal/image/TestExtractChecksumLinksArchStyle, internal/image/TestExtractChecksumLinksSumsLists, internal/image/TestDiscoverChecksumArchStyle, internal/image/TestExtractChecksumLinksSkipsUnsupportedAlgorithms

#### Scenario: No checksum available
- **WHEN** the directory links no checksum file, or none contains the image's hash
- **THEN** the download fails and says it can't be verified
- **Test:** internal/image/TestDiscoverChecksumErrorMessages

#### Scenario: Checksum file formats
- **WHEN** the checksum file uses `sha256sum` or BSD format, has comments, blank lines or a manifest entry for a similar name
- **THEN** only the exact file name's hash is taken
- **Test:** internal/image/TestParseChecksumSha256sumFormat, internal/image/TestParseChecksumBSDFormat, internal/image/TestParseChecksumNotFound, internal/image/TestParseChecksumEmptyContent, internal/image/TestParseChecksumCommentOnly, internal/image/TestParseChecksumWhitespaceLines, internal/image/TestParseChecksumExactMatchIgnoresManifest, internal/image/TestParseChecksumExactMatchPreventsSubstring

#### Scenario: Hash mismatch
- **WHEN** the downloaded bytes don't match the vendor hash
- **THEN** nothing is added to the cache and the error shows both hashes and suggests `vmherd pull` again
- **Test:** none yet — can use the TLS test server TestCheckImageURLAvailable already uses

### Requirement: Local cache
Verified images SHALL be cached in `~/.vmherd/images/` as `<distro>-<codename>-<file>`. A cached image SHALL be used without downloading again, and leftover temporary files from interrupted downloads SHALL be removed before the next download.

#### Scenario: Interrupted download
- **WHEN** a `.tmp` file from an interrupted download is in the cache
- **THEN** it is removed before the next download and cached images are kept
- **Test:** internal/image/TestCleanStaleTempFiles, internal/image/TestCleanStaleTempFilesEmptyDir

#### Scenario: Pull
- **WHEN** the user runs `vmherd pull ubuntu:noble`
- **THEN** the image is downloaded, verified and reported as "Image cached at <path>"
- **Test:** none yet — cmd/ code, needs M2.3; the e2e test downloads every image in its table through `up`
