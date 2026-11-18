## Purpose

Keep the image URLs that vmherd writes in `vmherd init` and documents in `docs/images.md` current. A release checks them before tagging and rewrites stale ones locally for the maintainer to review. Nothing is committed or published automatically.

## ADDED Requirements

### Requirement: One documented list of booted URLs
The image guide `docs/images.md` SHALL have a table of image URLs, and every URL that `vmherd init` writes or the e2e distro test boots SHALL appear in it with the same image name.

#### Scenario: Lists agree
- **WHEN** `go test ./...` runs
- **THEN** it fails if a URL from `vmherd init`'s images or from the e2e distro table is missing from the guide's table, naming the image and the file it came from
- **Test:** test/tooling/TestImageURLListsAgree

### Requirement: Release checks every URL
Before a release is tagged, every URL in the guide's table SHALL be checked. A URL passes when it answers over HTTPS and the vendor's checksum file in the same directory lists its file name.

#### Scenario: All current
- **WHEN** the maintainer runs `scripts/release.sh v0.5.0 "…"` and every URL passes
- **THEN** the checker prints one OK line per image and the release goes on to the changelog and tag as before
- **Test:** internal/imageurls/TestCheckReportsCurrentURL, internal/imageurls/TestOwnChecksumFileReadFirstUnderCap (checker); `release.sh` step: none yet, shell script, shown by the dry run in task 4.4

#### Scenario: Network or vendor failure
- **WHEN** a vendor host doesn't answer within the timeout, or answers with an error other than 404/410
- **THEN** the check fails naming the URL and the error, rewrites nothing, and the release stops before tagging
- **Test:** internal/imageurls/TestCheckFailsOnVendorError, internal/imageurls/TestCheckRequiresImageToAnswer, internal/imageurls/TestVendorClientRefusesUnsafeRedirects

### Requirement: Stale URLs are rewritten locally for review
When a URL is gone (404 or 410) or its checksum file no longer lists the file name, the checker SHALL look for the current file in the same vendor directory. If it finds exactly one, it SHALL replace the old URL with the new one in the files that hold it, print the old URL, new URL, SHA256 and the final URLs of the listing and checksum file that vouched for it, and exit non-zero so the release stops before tagging. It SHALL never commit, tag or push.

#### Scenario: Fedora moved to a new build
- **WHEN** `Fedora-Cloud-Base-Generic-45-1.4.x86_64.qcow2` is gone and the directory's checksum file lists `Fedora-Cloud-Base-Generic-45-1.5.x86_64.qcow2`
- **THEN** the URL is replaced in `internal/config/images.go`, `docs/images.md` and `test/e2e/e2e_test.go`, the output shows `fedora:45` old → new with its SHA256 and the final URLs (after redirects) of the directory listing and the checksum file it came from, and says to review, commit and run the release again, and no tag is made
- **Test:** internal/imageurls/TestFindReplacementPicksNewerBuild, internal/imageurls/TestFindReplacementKeepsFixedDigitRuns, internal/imageurls/TestRewriteReplacesURLInFiles, internal/imageurls/TestRewriteLeavesLongerURLs, internal/imageurls/TestRewriteNamesFilesWrittenBeforeFailure, internal/imageurls/TestStaleOutputNamesFinalListingAndChecksumURLs

#### Scenario: No replacement found
- **WHEN** a URL is stale and the directory has no file of the same pattern, several that tie, or the directory itself is gone
- **THEN** the check fails naming the image, the URL and the directory to look in, and rewrites nothing for that image
- **Test:** internal/imageurls/TestFindReplacementGone, internal/imageurls/TestFindReplacementAmbiguous, internal/imageurls/TestChecksumLinksOutsideDirectoryIgnored, internal/imageurls/TestChecksumLinksDeduplicatedAndCapped, internal/imageurls/TestChecksumFileFetchFailureSkipped

#### Scenario: A checksum file can't be read
- **WHEN** a URL is stale, a readable checksum file lists a newer build, and another checksum file in the same directory can't be read
- **THEN** the image is reported FAILED, not STALE: nothing is rewritten for it, and the error names the skipped checksum files and says to run the release again when the vendor answers. A current URL listed in a readable checksum file stays OK even if other checksum files were skipped
- **Test:** internal/imageurls/TestReplacementWithSkippedChecksumFileFails, internal/imageurls/TestChecksumFileFetchFailureSkipped

#### Scenario: Check-only mode
- **WHEN** the checker runs with `--check` and a URL is stale
- **THEN** it reports the stale URL and the replacement it would use, writes no file, and exits non-zero
- **Test:** internal/imageurls/TestCheckOnlyWritesNothing

### Requirement: Release workflow re-checks
The release workflow SHALL run the checker in check-only mode before building, and SHALL publish nothing when it fails.

#### Scenario: URL went stale after tagging
- **WHEN** a tag is pushed and a URL fails the check
- **THEN** the release job fails before the build, no GitHub release is created, and the log names the image and says to fix it and release the next patch version
- **Test:** none yet — GitHub workflow; shown once by the run in task 4.5
