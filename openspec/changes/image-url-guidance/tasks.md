# Tasks

## 1. Failing tests first

- [ ] 1.1 Add `internal/config/TestExampleHerdfilePointsToImageURLGuide`: the lines directly above `images:` are the comment from design decision 8, with the link `https://github.com/boyadzhievb/vmherd/blob/main/docs/images.md`, and the output still passes validation. Run it against the unchanged template and record here how it fails.
- [ ] 1.2 Add `test/tooling/image_urls_test.go` (no build tag) with `TestImageURLGuideLinkMatchesFile` (the path in the template's link exists in the repo), `TestReadmeLinksImageGuide` (README.md links `docs/images.md` in the image section and in the `images` row of the Herdfile reference table) and `TestImageURLListsAgree` (every `BuiltInImages()` URL and every e2e table URL, read from `test/e2e/e2e_test.go`'s source text, appears in `docs/images.md`'s table under the same name; fails with "found no e2e URLs" if the regular expression matches nothing). Run them and record the failures (expected: no `docs/images.md`).
- [ ] 1.3 Add `internal/imageurls/` tests on recorded vendor responses in `testdata/` (a TLS `httptest` server, as `TestCheckImageURLAvailable` uses): `TestCheckReportsCurrentURL`, `TestCheckFailsOnVendorError` (timeout and 5xx), `TestFindReplacementPicksNewerBuild` (Fedora 45 1.4 gone, 1.5 in `CHECKSUM`; the UEFI-UKI and `.raw.xz` lines are not picked), `TestFindReplacementGone`, `TestFindReplacementAmbiguous`, `TestRewriteReplacesURLInFiles` (all three files, the file that lacks the URL skipped, `docs/images.md` required), `TestCheckOnlyWritesNothing`. Record that they fail to compile before the package exists.

## 2. Herdfile comment and image guide

- [ ] 2.1 Change `exampleHerdfileTemplate` in `internal/config/project.go` to put the comment from design decision 8 above `images:`. Verify: 1.1 passes, `TestExampleHerdfileIsValidYAML` and `TestExampleHerdfilePassesValidation` still pass.
- [ ] 2.2 Add `docs/images.md`: per distro the vendor's image directory, which file to pick (cloud or generic variant, amd64, qcow2), the traps (Debian `genericcloud`, Rocky and Alma have no 9p, so set `shared_folders: []`; Fedora pins a build number), and the table `| Image | URL | Notes |` under `## Booted URLs` with the 8 booted URLs. In README.md, reference it in two places: the paragraph under the image list ("To find the URL for an image, see [docs/images.md](docs/images.md)") and the `images` row of the Herdfile reference table. Verify: 1.2 passes.

## 3. Release URL checker

- [ ] 3.1 Export what the checker needs from `internal/image` (checksum-file discovery in a directory, parsing a checksum file into name → hash, the 404/410 check) as thin wrappers over the existing functions; no logic copied. Verify: `go test ./internal/image/` unchanged.
- [ ] 3.2 Implement `internal/imageurls/`: parse the guide's table, check each URL, find a replacement by the rule in design decision 3, rewrite by decision 4. Named constant `vendorRequestTimeout`. Verify: 1.3 passes.
- [ ] 3.3 Add `cmd/image-urls-check/main.go` (`--check`, `--repo`), with output as in the spec scenarios and exit 1 on any stale or failed URL.
- [ ] 3.4 Add the step to `scripts/release.sh` after the "tag already exists" check and before the changelog (design decision 6), with the review message.
- [ ] 3.5 Add the `image-urls` job to `.github/workflows/release.yml` and `needs: image-urls` on `build` (design decision 7), with actions pinned by SHA like the existing steps.

## 4. Checks, review, result

- [ ] 4.1 `gofmt -l .` empty, `go vet ./...` and `go vet -tags integration ./...` clean, `golangci-lint run` 0 issues, `go test -count=1 ./...` passes, `openspec validate image-url-guidance --strict` passes.
- [ ] 4.2 Run `go run ./cmd/image-urls-check --check` on the Mac against the real vendors and record the output (expect 8 OK lines, or a real stale Fedora URL to record).
- [ ] 4.3 Independent review with `code-reviewer` and `security-audit` (downloads, rewriting files, release script). Record each finding as fixed or rejected with the reason.
- [ ] 4.4 Dry run of `release.sh` on a scratch branch with one `docs/images.md` and `images.go` URL changed to a gone Fedora build: it stops before the changelog, the diff shows the rewrite back to the current build, and no tag exists. Record the output, then discard the branch's changes.
- [ ] 4.5 The `image-urls` job runs on the next real release tag. Record the run link here (not shown before then, because `release.yml` only runs on tag push). The e2e distro test needs no change and no testbed run, since no URL changes in this change unless 4.2 finds a stale one.
- [ ] 4.6 Tick M3.11 in milestone.md with its Result note and update the M3 done count in the Progress table, in the same commit.
