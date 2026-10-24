# Changelog

All notable releases are documented here.

---

## v0.1.4

**2026-10-24** — Shared folders, port forwarding, reload, and provision commands

- Add 9p virtio shared folders — host directories mounted into guests (default `./` → `/vagrant`)
- Add port forwarding via iptables DNAT — map host ports to guest ports (TCP/UDP)
- Add `vmherd reload [name...]` — destroy and recreate VMs preserving network and SSH keys
- Add `vmherd provision [name...]` — rerun shell/file provisioning on running VMs via SSH
- Add `SharedFolder` and `PortForward` config types with validation (port ranges, duplicate detection, absolute guest paths)
- Port forwarding integrated into `up`, `down`, `destroy`, and `reload` lifecycle commands
- Consolidated port forwarding helpers into shared module

---

## v0.1.3

**2026-10-24** — Idempotent network handling, docs update, disable CI cache on self-hosted runner

- Make network ensure idempotent — `net-define` and `net-start` non-fatal when network already exists
- Use `virsh net-list` with exact name matching instead of `net-info` for reliable checks
- Remove `cloud-image-utils` from README prerequisites
- Update CLAUDE.md to reflect removed dependencies
- Add full changelog entries for all releases
- Disable Go module cache upload in integration CI (self-hosted runner has local cache)

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
