# Design

## Context

- `vmherd init` renders `exampleHerdfileTemplate` (`internal/config/project.go`) with the 4 images from `BuiltInImages()` (`internal/config/images.go`). The template's only pointer to docs is `# Docs: https://github.com/boyadzhievb/vmherd`.
- The README image list (around line 249) names 10 images and says "add the vendor's image URL under `images:`" without saying where to look.
- `test/e2e/e2e_test.go` (tag `integration`) holds 8 booted URLs, including the 4 from `init` again. Debian, Rocky, Alma and Arch appear nowhere else.
- `internal/image/download.go` already finds a vendor's checksum file by scraping the image's directory (`scrapeChecksumURLsFromDirectory`, `extractChecksumLinksFromHTML`) and parses `sha256sum` and BSD formats (`parseChecksumWithAlgorithm`). `checkImageURLAvailable` already treats 404/410 as "withdrawn". All of these are unexported.
- `scripts/release.sh` checks the tag format, the clean tree and that the tag doesn't exist, then writes CHANGELOG.md, commits, and tags. `release.yml` runs on tag push and builds `./cmd/vmherd` only.

## Goals / Non-Goals

**Goals:** a user who opens a fresh Herdfile knows where to find an image URL. The URLs vmherd writes and documents don't go stale between releases without anyone noticing. A stale URL is fixed by a reviewed, ordinary commit.

**Non-Goals:** an image list file, a scheduled CI check, bot branches or pull requests, tokens, any runtime URL lookup in `vmherd`, new images in `init`, an HTTP timeout for `vmherd pull` (M3.1).

## Decisions

1. **The guide's table is the list the checker reads.** `docs/images.md` (its own file, not a README section, by the user's decision of 2026-11-18) ends with a Markdown table `| Image | URL | Notes |` under `## Booted URLs`. The checker parses only the rows of that table. A consistency test (`test/tooling/TestImageURLListsAgree`) requires every `BuiltInImages()` URL and every e2e table URL to appear there under the same name. The e2e table is read from the source text with a regular expression over its `{"name", "https://…", bool}` rows, because the file is behind the `integration` tag. So the guide is a superset, and checking it checks everything vmherd ships.
   - *Alternative rejected:* a new `images.yaml`. The user ruled out a list file on 2026-11-18.

2. **Candidates come from the vendor's checksum file, not the HTML listing.** A replacement must be a file the vendor publishes a hash for, so the new URL can always be verified by `vmherd pull`. The checker reuses `internal/image`'s discovery and parsing; the needed functions get exported wrappers instead of being copied (CLAUDE.md: no copy-paste duplication).
   - *Rework after review (2026-11-18):* the checker reads only checksum links that name a file **directly in the image's directory**. The link, resolved against the directory URL, must be HTTPS on the same host with no query or fragment. Its percent-decoded path must already be clean (`path.Clean` changes nothing), must have no backslash, and its parent must be the directory's path. Links to `../44/…`, `%2e%2e%2f44%2f…` or `https://<host>/other/…` are ignored (second rework, 2026-11-18). A checksum file that can't be fetched is recorded and skipped, as `vmherd pull` does. A current URL listed in a readable file stays OK. A replacement is never taken while any checksum file of the directory was skipped, because the skipped file might list a competing build: the image is FAILED, nothing is rewritten for it, and the error names the skipped files and says to run the release again when the vendor answers (finding T, user's decision 2026-11-18). Each checksum URL is read once, and at most `maxChecksumFilesPerDirectory` (8) per directory, in `vmherd pull`'s preference order (the image's own checksum file first). Only 64- or 128-digit hex hashes count, for `vmherd pull` too, and a listing or checksum file over 1 MiB fails instead of being read cut off (shared reader, so `pull` gets it too).
   - For a replacement, the report shows the final URL (after redirects) of the directory listing and of the checksum file the new name and hash came from, so the reviewer sees which mirror vouched for it. Fedora's download host redirects the `CHECKSUM` file to third-party mirrors. Verifying the vendors' signatures stays with M3.3.

3. **"Same file, newer build" rule.** The old file name is split into digit runs and non-digit runs. Its **build numbers** are the digit runs that are neither inside an architecture or format word (`x86_64`, `amd64`, `aarch64`, `arm64`, `qcow2`) nor equal to a path segment of the image's directory URL. That second rule keeps the release fixed: `45` in `…/releases/45/Cloud/…`, `9` in `…/rocky/9/…`. Every other run, digit or not, is fixed. A candidate matches when it has the same runs, identical fixed runs, and build numbers of at most `maxBuildNumberDigits` (14, a date-time stamp) digits. Among the matches, the highest by numeric comparison of the build numbers, left to right, wins, and it must be higher than the old one. (Rework after review, 2026-11-18: before, every digit run counted, so `x99_64`, `qcow3` or `Generic-46` would have been "newer".) A digit run that only looks like a release is also held fixed: a build number equal to the release number (e.g. a hypothetical `…-45-45.x86_64…`) is frozen. That can only turn a real update into "gone", which fails safe. Debian is the known exception: its file names hold the release number (`debian-13-…`) while the path holds the codename (`trixie/latest/`), so `13` counts as a build number there. This is harmless because Debian's `latest/` file names never change, so the checker never looks for a Debian replacement. If none match, the image is "gone". A tie, or several matches in a directory where the old file is still listed but no longer passes, means "ambiguous". Both fail loudly with the directory to look in. Example: `Fedora-Cloud-Base-Generic-45-1.4.x86_64.qcow2` matches `…-45-1.5.x86_64.qcow2`, but not `…-Base-UEFI-UKI-45-1.5…`, a `.raw.xz`, `…-46-1.5…`, `…x99_64…` or `….qcow3`.
   - Ubuntu `current/`, Debian `latest/`, Rocky/Alma `latest` and Arch `latest` names don't change, so in practice only Fedora is rewritten. The rule stays generic, so a new pinned vendor needs no code change.
   - The new URL is always the old URL's directory plus the new file name: same host, same path. The checker never follows the vendor to another host or directory. A release that moved directories, such as Fedora 44 moving to archives, is "gone".

4. **Rewrite is a literal string replacement** of the old URL with the new one, only where the old URL ends at a delimiter (`"`, whitespace, `|`, `)`, `>`, a backtick, or the end of the file), so a longer URL such as `….img.manifest` is left alone (rework after review, 2026-11-18). The fixed list of files: `internal/config/images.go`, `docs/images.md`, `test/e2e/e2e_test.go`. Each file is written to a temp file in the same directory and renamed into place. A file that doesn't contain the old URL is skipped, except `docs/images.md`, which must contain it, since that's where the URL came from. The checker prints `git diff --stat` advice and never runs git itself. If a later file can't be written, the error names the files already rewritten.

5. **The checker is a separate developer tool**, `cmd/image-urls-check/` (thin main: flags, output, exit code) over `internal/imageurls/` (parse table, check, find replacement, rewrite). It isn't built into release archives: `release.yml` builds only `./cmd/vmherd`, so the single-binary principle is unchanged. Flags: `--check` (report only) and `--repo <dir>` (default `.`). It uses its own `http.Client` with a named timeout constant (`vendorRequestTimeout = 30 * time.Second`) and HTTPS only. Redirects must stay on HTTPS and are capped at `maxVendorRedirects` (10); tests inject the test server's transport into this client.
   - *Rework after review (2026-11-18):* the checker requires a **final 2xx** for the current URL and for a replacement. It sends HEAD through the same helper as `vmherd pull` (`image.HeadImageURL`), and on 405 or 501 it falls back to `GET` with `Range: bytes=0-0` (200 or 206 passes). Any other non-2xx answer except 404/410 fails, naming the URL and status. `vmherd pull` keeps its lenient rule, where an answer other than 404/410 is not held against the URL.

6. **`release.sh` order:** argument and tag checks, clean tree, tag doesn't exist, then `go run ./cmd/image-urls-check`, then the changelog, commit and tag. The URL step runs after the clean-tree check, so a rewrite leaves a dirty tree that blocks the next run until the user commits. On a non-zero exit, the script prints "image URLs changed or failed — review `git diff`, commit, and run the release again" and exits 1 before touching CHANGELOG.md.

7. **`release.yml`** gets a job `image-urls` (ubuntu-latest, checkout, setup-go, `go run ./cmd/image-urls-check --check`), and `build` gets `needs: image-urls`. If it fails, the tag already exists. Per the version rule (never retag), the log says to fix the URL and release the next patch version.

8. **Herdfile comment** directly above `images:`:
   ```
   # Images: each VM's image must be listed here with the vendor's download URL.
   # To find the URL for another distro or release, see
   # https://github.com/boyadzhievb/vmherd/blob/main/docs/images.md
   ```
   `TestImageURLGuideLinkMatchesFile` takes the path after `/blob/main/` from the template's output and checks that the file exists in the repo.

## Failure cases

| Case | What the checker does |
|---|---|
| All URLs current | One OK line per image, exit 0; release goes on |
| Vendor host down, timeout, TLS error, 5xx (also on the image URL alone) | Fails naming the URL and error; rewrites nothing for it; exit 1 |
| Replacement URL answers non-2xx | Fails naming the replacement and status; no rewrite |
| Checksum link outside the image's directory (also percent-encoded) | Ignored; if none is left, fails "links no checksum file in that directory" |
| One checksum file can't be fetched | Recorded and skipped; the others are read. A current URL listed in a readable file is OK. A stale URL fails (no rewrite), even when a readable file lists a replacement, naming the skipped files and saying to run the release again |
| Listing or checksum file over 1 MiB | Fails ("larger than … bytes"); also for `vmherd pull` |
| URL 404/410, one newer match in checksum file | Rewrites the URL in the listed files, prints old → new + SHA256; exit 1 (release stops for review) |
| URL answers but checksum file no longer lists it | Same as 404: look for a replacement |
| No match / tie / directory gone | Fails naming image, URL and directory; no rewrite for it; exit 1 |
| No checksum file found in directory | Fails ("can't verify"), no rewrite |
| Several images, some rewritten, some failed | Writes the replacements it found, reports every failure, exit 1. The diff then holds only verified replacements |
| Guide table missing or empty | Fails: "no image table under '## Booted URLs' in docs/images.md" |
| Old URL not in docs/images.md at rewrite time | Fails for that image, writes nothing for it |
| `--check` | Same checks; never writes; exit 1 on any stale or failed URL |
| Run in CI after tagging fails | Job fails before build; no release published; log says to release the next patch |

Nothing is removed: the rewrite only changes text in the working tree. Cached images and users' Herdfiles are untouched, and the old URL stays in git history until the user commits the change (rule from M1.19).

## Risks / Trade-offs

- **The hash comes from the vendor host itself.** A compromised vendor directory could offer a bad "newer build". Mitigations: same host and directory only, HTTPS only, the change lands only through the maintainer's reviewed commit, and the nightly e2e run boots the new URL after it's committed.
- **Vendor layouts change.** The match rule fails rather than guesses, and the release stops, which is the intended behavior.
- **`release.sh` now needs network and Go.** Both are already needed to build and push a release.
- **The e2e table is read by regular expression.** If its row format changes, `TestImageURLListsAgree` fails with "found no e2e URLs", so it doesn't pass silently.
