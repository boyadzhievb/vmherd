# Proposal

Implements **M3.11 — Herdfile comment that points to an image URL guide on GitHub; release checks the URLs** (milestone.md, M3).

## Why

Users have no way to find the right URL for an image. `vmherd init` writes URLs for 4 images (ubuntu noble and resolute, fedora 44 and 45). The README names 10 images and says "add the vendor's image URL" for the other 6, without saying where to look or which file to pick. Vendors publish several variants: Debian `genericcloud` has no 9p (found in M1.20), and Rocky offers both `GenericCloud` and `EC2`. The URLs CI has booted for Debian, Rocky, Alma and Arch exist only in the e2e test table. Fedora's file names pin a build number, so the URLs `init` writes go stale without anyone noticing until a user's `vmherd up` fails.

## What Changes

- `vmherd init` writes a comment above `images:` that says where and how to find an image URL. It links to a new page, `docs/images.md`, on the vmherd GitHub page (https://github.com/boyadzhievb/vmherd/blob/main/docs/images.md). The guide is its own file, not a README section.
- `docs/images.md` is the guide. For each distro it names the vendor's image directory and the file to pick (cloud or generic variant, amd64, qcow2), lists the known traps (Debian `genericcloud`, Rocky and Alma have no 9p; Fedora pins a build number), and gives a table of the URLs vmherd has booted: the 4 that `init` writes plus Debian, Rocky, Alma and Arch from the e2e table.
- A test keeps the three lists in step. Every URL in `BuiltInImages` and in the e2e distro table must appear in the guide's table.
- A release URL checker, `cmd/image-urls-check`, is a developer tool that is not part of the `vmherd` binary. For every URL in the guide's table, it checks that the URL answers and that the vendor's checksum file lists the file name.
  - If a URL is stale, it finds the current file in the vendor's directory and replaces the old URL with the new one in `internal/config/images.go`, `docs/images.md` and `test/e2e/e2e_test.go`. It prints the old URL, the new URL and the SHA256, then exits non-zero.
  - It never guesses: if the directory is unreadable, it finds no candidate, or several candidates tie, it fails and names the URL.
  - With `--check`, it only reports and never writes.
- `scripts/release.sh` runs the checker after its clean-tree check and before it writes the changelog or the tag. If the checker rewrote any file, the script stops before tagging: the user reviews the diff, commits it, and runs the release again.
- `release.yml` runs the checker with `--check` before it builds, as a safety net.

Out of scope, by the user's decision (2026-11-18): an image list file such as `images.yaml`, a daily or scheduled CI check, bot branches or pull requests, any commit or token used by automation, and any runtime lookup of URLs by `vmherd` itself (CLAUDE.md principle 1). It also doesn't add new images to `init`, change the image download path (M3.1), or add `Herd.lock` (M6.1).

**How Vagrant does it:** `vagrant box add ubuntu/noble` looks the name up in HCP Vagrant Cloud, so users never see a URL. The cost is depending on that registry, which reaches EOL in November 2026. Box authors keep their boxes current there. vmherd has no registry on purpose, so it tells users where each vendor publishes its images and keeps its own example URLs current at release time.

## Capabilities

### New Capabilities
- `release-image-urls`: the release-time check of the image URLs vmherd ships and documents, and the local rewrite of stale ones for review.

### Modified Capabilities
- `herdfile`: the Example Herdfile requirement gains the comment that points to `docs/images.md`.

## Impact

- `internal/config/project.go`: the `exampleHerdfileTemplate` comment.
- New `docs/images.md`: the guide and URL table. `README.md` references it under its image list and in the `images` row of its Herdfile reference table.
- New `cmd/image-urls-check/` (main) and `internal/imageurls/` (check, find replacement, rewrite), with unit tests on recorded vendor listings in `testdata/`. This reuses `internal/image`'s checksum-file discovery and parsing (exported where needed) instead of copying it.
- New untagged consistency test, which reads the e2e table's URLs from the source text because that file is behind the `integration` tag.
- `scripts/release.sh` gets a new step; `.github/workflows/release.yml` gets a new job that `build` depends on.
- `milestone.md`: M3.11 links this change.
