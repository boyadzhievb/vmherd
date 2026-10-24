# Changelog

All notable releases are documented here.

---

## v0.1.2

**2026-10-24** — Replace cloud-localds with virt-install --cloud-init, add test suite and integration CI

- Remove `cloud-image-utils` / `cloud-localds` dependency — cloud-init files are now passed directly to `virt-install --cloud-init`
- Add unit tests for `ui`, `cloudinit`, `vm/dns`, and `ssh/keygen` packages
- Add integration tests (gated with `-tags integration`) for full VM lifecycle on libvirt
- Add self-hosted runner CI workflow for integration tests on testbed
- Truncate libvirt bridge names to 15 chars (Linux kernel limit)
- Bump Go to 1.27.3 (fix crypto/x509 vulnerability GO-2026-6809)

---

## v0.1.1

**2026-10-24** — Add --quiet/-q flag and replace wget/curl with native Go HTTP

- Add `--quiet` / `-q` global flag to suppress status output
- Replace `wget` with native Go `net/http` for image downloads (one fewer external dependency)
- Add progress display for downloads with percentage and MB counter

---

## v0.1.0

**2026-10-24** — Initial release

- Declarative VM management via `Herdfile` (YAML)
- Built-in registry of 9 official cloud images (Ubuntu, Debian, Fedora, Rocky, AlmaLinux, Arch)
- SHA256 checksum verification for all downloaded images
- Cloud-init provisioning: SSH key injection, shell commands, file transfers
- Static IP assignment via cloud-init network-config
- DNS auto-detection from host `/etc/resolv.conf`
- Full CLI: `init`, `up`, `down`, `destroy`, `status`, `ssh`, `scp`, `list-images`, `pull`, `version`
- ed25519 SSH key generation per project

---
