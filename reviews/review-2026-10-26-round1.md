# Code Review — Round 1 (2026-10-26)

**Scope:** Full project — security, code quality, functionality
**Status:** ALL FIXED in commit `08c342f`
**Findings:** 26 total (6 critical/high, 12 medium, 8 low)

---

## Critical / High

| # | Severity | Finding | File(s) | Status |
|---|----------|---------|---------|--------|
| 1 | Critical | Shared folder path traversal — host path not validated, can mount `/etc` | `config/parse.go` | FIXED |
| 2 | Critical | Provision file path traversal — source can escape project dir | `config/parse.go` | FIXED |
| 3 | High | No VM name validation — special chars allow shell injection via domain names | `config/parse.go` | FIXED |
| 4 | High | Network name not validated — XML injection possible via libvirt network XML | `config/parse.go` | FIXED |
| 5 | High | Multi-line runcmd broken — newline splitting creates invalid cloud-init | `cloudinit/cloudinit.go` | FIXED |
| 6 | High | Cloud-init files written 0644 — readable by all users, contains SSH keys | `cloudinit/cloudinit.go` | FIXED |

## Medium

| # | Severity | Finding | File(s) | Status |
|---|----------|---------|---------|--------|
| 7 | Medium | No IP address validation — malformed IPs accepted | `config/parse.go` | FIXED |
| 8 | Medium | No disk size validation — invalid formats accepted | `config/parse.go` | FIXED |
| 9 | Medium | Download-then-hash instead of streaming hash verification | `image/download.go` | FIXED |
| 10 | Medium | Orphaned disk on virt-install failure — no cleanup | `vm/vm.go` | FIXED |
| 11 | Medium | Error message uses domain name instead of VM name | `vm/vm.go` | FIXED |
| 12 | Medium | Port forwarding errors swallowed as warnings with no early backend check | `portfwd/portfwd.go`, `cmd/` | FIXED |
| 13 | Medium | IP outside subnet not detected | `config/parse.go` | FIXED |
| 14 | Medium | Duplicate IP addresses not detected | `config/parse.go` | FIXED |
| 15 | Medium | `vmherd down` swallows shutdown and force-stop errors | `cmd/down.go` | FIXED |
| 16 | Medium | SSH/SCP don't check VM state before connecting | `cmd/ssh_cmd.go`, `cmd/scp.go` | FIXED |
| 17 | Medium | Fixed 2s SSH polling instead of exponential backoff | `ssh/connect.go` | FIXED |
| 18 | Medium | Missing doc comments on ~20 functions | multiple | FIXED |

## Low

| # | Severity | Finding | File(s) | Status |
|---|----------|---------|---------|--------|
| 19 | Low | Missing inline comments on exported struct fields | multiple | FIXED |
| 20 | Low | `vmherd status` output not sorted — non-deterministic | `cmd/status.go` | FIXED |
| 21 | Low | HTTPS not enforced for image download URLs | `image/download.go` | FIXED |
| 22 | Low | SSH key permissions not explicitly set after generation | `ssh/keygen.go` | FIXED |
| 23 | Low | Stale .tmp files not cleaned up in image cache | `image/download.go` | FIXED |
| 24 | Low | Destroy doesn't clean per-VM cloud-init state directory | `cmd/destroy.go` | FIXED |
| 25 | Low | Provision file chmod 755 — should be 644 for non-scripts | `cmd/provision.go` | FIXED |
| 26 | Low | UI quiet flag uses non-atomic global bool | `ui/ui.go` | FIXED |
