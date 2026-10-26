# Per-Feature Review Checklist

Use this when implementing or reviewing changes in a specific area. Check the relevant rows before marking work as done.

---

## config/ (Herdfile parsing and validation)

- [ ] New fields have validation in `parse.go` (fail early with actionable error)
- [ ] String fields that reach shell commands or YAML output are regex-validated
- [ ] Paths are checked for traversal (no escape from project dir, no system dirs)
- [ ] Numeric fields have range checks (ports 1-65535, CPUs > 0, memory > 0)
- [ ] Duplicate detection for any new uniqueness constraints (names, IPs, ports)
- [ ] Defaults applied in `applyDefaults()` before validation runs

## cloudinit/ (cloud-init generation)

- [ ] All user input interpolated with `%q` or validated before embedding in YAML
- [ ] runcmd entries use `["/bin/bash", "-c", %q]` format (never bare strings)
- [ ] Generated files written with 0600 permissions, directories with 0700
- [ ] No hardcoded DNS servers (derive from host `/etc/resolv.conf`)
- [ ] write_files permissions match what `provision.go` uses (0644 default)

## image/ (download and verification)

- [ ] All URLs enforce HTTPS (both image and checksum URLs)
- [ ] Checksum matching uses exact filename comparison (not substring)
- [ ] Hash computed during download via `io.TeeReader` (streaming, not download-then-hash)
- [ ] Response bodies have size limits (`io.LimitReader`)
- [ ] Temp files cleaned up on failure and on startup

## vm/ (VM lifecycle)

- [ ] Disk cleaned up on virt-install failure (`sudo unlink`)
- [ ] Errors from cleanup operations logged as warnings (not swallowed with `_`)
- [ ] Error messages use VM name (not domain name) for user-facing output
- [ ] New `CreateOptions` fields documented with inline comments

## network/ (libvirt network management)

- [ ] Subnet conflicts checked against existing libvirt networks
- [ ] Bridge name collisions detected before creation
- [ ] Network XML written with 0600 permissions
- [ ] Autostart errors logged (not swallowed)

## portfwd/ (port forwarding)

- [ ] Teardown called before setup (idempotent pattern)
- [ ] Firewall backend checked early (before any VM work)
- [ ] Existing NAT rule conflicts detected before adding rules
- [ ] Setup and teardown are symmetric (same chains, same rules)

## ssh/ (SSH key management and connection)

- [ ] Private key permissions explicitly set to 0600
- [ ] State directory permissions set to 0700
- [ ] Retry uses exponential backoff (not fixed interval)
- [ ] Timeout is a named constant

## cmd/ (CLI commands)

- [ ] VM state checked before operations that require running VMs (ssh, scp, provision)
- [ ] Every new function has a doc comment
- [ ] Errors from shutdown/force-stop logged as warnings (not swallowed)
- [ ] Port forwarding set up only for VMs with IP and port config
- [ ] Plaintext password in Herdfile triggers a warning

## General (applies to all changes)

- [ ] No raw string literals for VM states — use `vm.StateRunning`, `vm.StateStopped`, etc.
- [ ] No raw string literals for provisioner types — use `config.ProvisionShell`, `config.ProvisionFile`
- [ ] No magic numbers — timeouts, thresholds, limits are named constants
- [ ] No `_ = fn()` for error-returning functions — log or return
- [ ] No function over 80 lines (excluding tests)
- [ ] Exported struct fields have inline comments
- [ ] `go test ./...` passes, `golangci-lint run` clean
