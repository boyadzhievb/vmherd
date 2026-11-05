# Changelog

All notable releases are documented here.

---

## v0.2.0

**2026-11-05** — Safety fixes, working port forwarding and provisioning, DHCP VMs

This release fixes data-loss and host-safety problems and several features that did not work in v0.1.x. It has breaking changes; read **Upgrading** first.

### Upgrading from v0.1.x
- **Port forwards** now run over one SSH tunnel per VM instead of iptables, and listen on `127.0.0.1` by default. Set `host_ip: 0.0.0.0` (or a host address) on a port to expose it on the network. `protocol: udp` is refused (SSH carries TCP only). Host ports below 1024 use `sudo`. The iptables DNAT rules v0.1.x created for a VM's ports are removed automatically when v0.2.0 starts or stops that VM's port forwarding (only rules exactly in the v0.1.x format are touched); left in place they would catch the traffic before the tunnel.
- **Shell provision steps run as root** (like first-boot `runcmd`); **file steps are installed as `root:root 0644`**.
- **New libvirt names:** VMs and disks are `vmherd-<project>-<hash>-<vm>`. Existing VMs keep their old name and keep working; `vmherd reload` moves a VM to the new name.
- **Shared folders:** for VMs with shares, QEMU runs as your user (9p `accessmode=squash`) and the guest SSH user gets your UID, so files have the same owner and modes on both sides. vmherd refuses to run as root. Existing VMs get this after `vmherd reload`.
- **The default `/vagrant` share is announced** by `vmherd up`; `shared_folders: []` now means no shares.
- **A VM without `network.ip` uses DHCP** (it used to boot with no network). `network.gateway` without `network.ip` is refused.
- Project paths and share paths containing `"` or a line break are refused (virt-install can't carry them).

### Fixed
- `destroy` only removes a state directory that carries the vmherd marker, and only files vmherd created.
- Shared-folder host paths must resolve inside the project or `$HOME` (`allow_outside_home: true` for others); `/`, `$HOME` and system directories are refused. The `chmod o+rwx` fallback is gone.
- Guest-created files in shares are readable on the host; guest root can't create root-owned files there.
- Projects with the same directory name no longer share VMs, disks or networks, and a comma in the directory name can't inject virt-install options.
- `provision` runs multi-line shell scripts intact (`apt-get update && …` used to run only `apt-get`), and file steps work right after `up`.
- `vmherd ssh <vm> -- cmd args` keeps quoted arguments.
- A shared folder mounted under the SSH user's home (for example `/home/vagrant/app`) no longer leaves the home directory owned by root.
- cloud-init no longer reports `status: error` on every VM with a shared folder (a second, redundant mount failed).
- `vmherd init` writes a project-unique network name and a free subnet (192.168.150–249) instead of `vmherd-default` on libvirt's `192.168.122.0/24`; `destroy` keeps a network other VMs still use; the subnet-overlap error suggests a free subnet.
- Port forwarding works: `localhost`, LAN (with `host_ip`), and the iptables-not-in-PATH failure on Debian is gone.
- `down`, `restart` and `reload` wait up to 60s for a clean shutdown (in parallel) instead of forcing VMs off after 5s.
- Integration tests no longer run fork-PR code on the self-hosted runner; the committed release binary is removed.

### Known issues
- Only Ubuntu and Debian images are tested end to end. `fedora:42` fails at the checksum step, the `arch:latest` checksum error is unclear, and `rocky:9` / `almalinux:9` never reach SSH.
- `vmherd console-log` fails with "permission denied" (the log is written as root).
- A shell provision step reports success if a later command in the script succeeds after an earlier one failed.
- `provision` right after `up` can hit the apt lock while first-boot cloud-init is still running.
- Files created in a shared folder get the group of `/dev/kvm` (usually `kvm`) on the host; owner and modes match.
- `up` exits 0 when a VM's SSH wait or DHCP lease times out (it prints a warning).
- Image checksums are fetched from the vendor's server but not yet GPG-verified, and release builds are not yet gated on tests.

---

## v0.1.15

**2026-10-27** — Redesing registry

---

## v0.1.14

**2026-10-27** — Autodicovers sums

---

## v0.1.12

**2026-10-26** — Fix release script cross-platform compatibility

---

## v0.1.11

**2026-10-26** — Fix 49 review findings across two comprehensive code reviews

### Round 1 (26 findings)
- Validate VM names, network names, IPs, disk sizes, and port forwards at parse time
- Block path traversal in shared folders and provision file sources
- Fix multi-line shell provisioning (wrap in `bash -c` instead of splitting)
- Tighten cloud-init and SSH key file permissions (0600/0700)
- Stream SHA256 hash during image download instead of download-then-hash
- Enforce HTTPS for image URLs and clean stale `.tmp` files
- Clean up orphaned disks on virt-install failure
- Check VM state before ssh/scp, log shutdown errors instead of swallowing
- Replace fixed 2s SSH polling with exponential backoff (1s→16s cap)
- Sort VMs alphabetically in status output for deterministic display
- Replace global mutable bool with `atomic.Bool` in ui package

### Round 2 (23 findings)
- Fix single-line runcmd YAML injection (use `bash -c %q` for all entries)
- Validate `ssh.user` field to prevent cloud-init YAML injection
- Add `VMState` and `ProvisionType` typed constants, replace 19 raw string comparisons
- Fix port forwarding idempotency with teardown-before-setup pattern
- Use exact filename matching in checksum parsing (not substring)
- Enforce HTTPS for checksum URLs, add 1MB size limit on checksum downloads
- Restrict subnet validation to /24, reject static IPs in DHCP pool range
- Tighten state directory (0700) and network XML (0600) permissions
- Skip 5s sleep in `vmherd down` when no VMs were running
- Extract shared `virsh()` helper to `internal/libvirt` package
- Log all previously swallowed errors as warnings
- Remove dead `CheckConflict` function
- Add review tracking files and per-feature checklist

---

## v0.1.10

**2026-10-26** — Add restart command and host collision safety checks

- Add `vmherd restart` command: gracefully stop and start VMs without destroying them (preserves disk and config, unlike `reload`)
- Detect subnet conflicts: before creating a libvirt network, check all existing networks for overlapping subnets (handles mixed prefix lengths)
- Detect iptables NAT rule conflicts: before adding DNAT port-forward rules, check for existing rules on the same port to avoid collisions with Docker, other VMs, etc.
- Detect firewall backend: identify iptables / iptables-nft / pure nftables and error clearly on unsupported setups
- Detect disk path collisions: refuse to overwrite existing qcow2 files from stale or orphaned VMs
- Detect bridge name collisions: catch when two long network names truncate to the same 15-char Linux bridge name
- 28 new unit tests covering subnet overlap, iptables output parsing, bridge truncation, and CIDR conversion

---

## v0.1.9

**2026-10-25** — 9p shared folder fixes and test coverage

- Fix 9p shared folder permissions: set ACL on host directory so QEMU process (libvirt-qemu) can write via `setfacl`, with `chmod` fallback
- Fix git clone in 9p shared folders: use `git -c safe.directory=` instead of `git config --global` (root's gitconfig may not exist during cloud-init)
- Fix install script idempotency: check for `docker-compose.yml` instead of `.git` (stale `.git` persists on host across VM recreations)
- Fix `git checkout -B` to handle pre-existing branches from partial runs
- Expand test coverage: new tests for MAC address generation, QEMU user detection, SSH args, cloud-init shared mounts, config edge cases, and image registry helpers

---

## v0.1.8

**2026-10-25** — Parallel VM creation and DNS loopback fix

- Create VMs in parallel with bounded concurrency (up to 4 at a time) via `errgroup`
- Wait for SSH readiness on all VMs concurrently instead of sequentially
- Collect all creation errors instead of failing on the first — partial success is reported
- Fix DNS resolution: skip loopback nameservers (127.0.0.1, 127.0.0.53) from host `/etc/resolv.conf` and fall back to the libvirt gateway (dnsmasq). Fixes provisioning failures on hosts running local DNS caches (systemd-resolved, NetworkManager dnsmasq)
- Add Terramino acceptance test Herdfiles (single-VM and multi-VM) based on HashiCorp's learn-vagrant-get-started tutorial
- Add `golang.org/x/sync` dependency

---

## v0.1.7

**2026-10-25** — Persistent XDG state directory and info command

- Store per-project state in `$XDG_STATE_HOME/vmherd/` (~/.local/state/vmherd/) instead of `.vmherd/` in the project directory. Survives `git checkout` and CI workspace wipes.
- Backwards compatible: existing `.vmherd/` directories continue to work.
- Override with `VMHERD_STATE_DIR` environment variable.
- Add `vmherd info ssh-key` — prints the SSH private key path for the current project.
- Add `vmherd info state-dir` — prints the state directory path for the current project.

---

## v0.1.6

**2026-10-24** — MAC address based network config

- Generate random MAC address (QEMU OUI 52:54:00) per VM at creation time
- Match NIC by MAC address in cloud-init netplan instead of hardcoded interface name
- Fixes network not activating when shared folders shift NIC to different PCI slot (enp2s0 vs enp1s0)

---

## v0.1.5

**2026-10-24** — Console password support

- Add `password` field to Herdfile `ssh:` section for setting a user password via cloud-init
- Password is SHA-512 hashed via openssl and injected with `lock_passwd: false`
- Enables `virsh console` login without affecting SSH key-only authentication
- Fix test compilation after GenerateFiles signature change

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
