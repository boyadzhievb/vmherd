# Code Review — Round 2 (2026-10-26)

**Scope:** Full project — security, code quality, functionality (post round-1 fixes)
**Status:** ALL FIXED (pending commit)
**Findings:** 23 total (1 critical, 5 high, 11 medium, 6 low)

---

## Critical

| # | Severity | Finding | File(s) | Status |
|---|----------|---------|---------|--------|
| 1 | Critical | Single-line runcmd YAML injection — commands with `: `, `#`, `[` produce malformed cloud-init | `cloudinit/cloudinit.go:117` | FIXED |

## High

| # | Severity | Finding | File(s) | Status |
|---|----------|---------|---------|--------|
| 2 | High | YAML injection via unvalidated SSH user field — newlines inject cloud-init directives | `cloudinit/cloudinit.go:63`, `config/parse.go` | FIXED |
| 3 | High | `vmherd up` re-adds port forwarding for running VMs — breaks idempotency | `cmd/up.go:110` | FIXED |
| 4 | High | Checksum substring matching can return wrong hash for similar filenames | `image/download.go:188` | FIXED |
| 5 | High | Raw strings for VM states — 15 occurrences across 8 files, no typed constants | `vm/vm.go`, `cmd/*.go` | FIXED |
| 6 | High | Raw strings for provisioner types — 5 occurrences, no typed constants | `config/config.go`, `cmd/provision.go`, `cloudinit/cloudinit.go` | FIXED |

## Medium

| # | Severity | Finding | File(s) | Status |
|---|----------|---------|---------|--------|
| 7 | Medium | HTTPS not enforced for checksum URL fetch (only image URL is checked) | `image/download.go:158` | FIXED |
| 8 | Medium | Hardcoded /24 subnet assumption — /16 silently becomes /24 | `config/project.go`, `network/network.go`, `cloudinit/cloudinit.go` | FIXED |
| 9 | Medium | State directory created with 0755 — should be 0700 (holds SSH keys) | `config/project.go:51,69` | FIXED |
| 10 | Medium | `vmherd down` always sleeps 5s even when no VMs were running | `cmd/down.go:45` | FIXED |
| 11 | Medium | Orphaned iptables rules after crash — no cleanup or idempotent teardown | `portfwd/portfwd.go` | FIXED |
| 12 | Medium | write_files 0755 but provision.go uses 0644 — inconsistent permissions | `cloudinit/cloudinit.go:106`, `cmd/provision.go:126` | FIXED |
| 13 | Medium | Static IPs within DHCP pool range (.100-.200) not rejected | `config/parse.go` | FIXED |
| 14 | Medium | `CheckConflict` is dead code — exported, tested, never called | `portfwd/portfwd.go:99-128` | FIXED |
| 15 | Medium | 6 remaining swallowed errors (`_ = fn()`) | `restart.go`, `vm.go`, `network.go`, `download.go` | FIXED |
| 16 | Medium | Duplicate `virsh()` helper — identical in network.go and vm.go | `network/network.go:235`, `vm/vm.go:176` | FIXED |
| 17 | Medium | Missing doc comments on 11 cmd/ functions | `cmd/vmherd/*.go` | FIXED |

## Low

| # | Severity | Finding | File(s) | Status |
|---|----------|---------|---------|--------|
| 18 | Low | Function length violations — newReloadCmd (88 lines), writeUserData (82 lines) | `cmd/reload.go`, `cloudinit/cloudinit.go` | FIXED |
| 19 | Low | No warning when Herdfile contains plaintext password | `config/config.go:52` | FIXED |
| 20 | Low | Shared folder fallback `chmod o+rwx` overly permissive, no prominent warning | `vm/shared.go:23` | FIXED |
| 21 | Low | No size limit on checksum file download — OOM possible | `image/download.go:168` | FIXED |
| 22 | Low | Network XML written at 0644 — inconsistent with 0600 cloud-init files | `network/network.go:173` | FIXED |
| 23 | Low | Redundant `Lstat` + `ModeSymlink` check — `IsDir()` already handles symlinks | `config/project.go:60` | FIXED |
