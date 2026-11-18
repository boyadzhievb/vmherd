# Proposal

Implements **M1.21 — `up` doesn't check that a share's host folder exists** (milestone.md, M1).

## Why

Loading a Herdfile accepts a share whose host folder doesn't exist yet. That is on purpose (`resolveSymlinksAllowMissing`), so `status` and `destroy` still work after a folder is deleted. But nothing checks the folder later. `vmherd up` defines the domain, QEMU then fails with `cannot initialize fsdev 'fsdev-fs0': failed to open '…/terramino-go': No such file or directory`, and vmherd adds `virsh start failed: exit status 1 — check that libvirtd is running and you have permission`. That hint points at the wrong thing. Found in M6.9 (task 2.5) with a fresh copy of the Terramino single scenario.

## What Changes

- `vmherd up` checks every share's host folder of the VMs it was asked to bring up, before it creates the SSH key, the network, any disk or any domain.
- `vmherd reload` makes the same check before it destroys anything, together with its existing image and host-identity checks.
- `vmherd restart` makes the same check before it stops anything (added after review, 2026-11-18): without it, restart stops a running VM and then fails to start it with the libvirtd hint, leaving it stopped.
- A missing folder, or a path that exists but isn't a folder, fails the command with exit 1. The error names the VM, the share's host path as written in the Herdfile and the resolved path, and how to fix it: create the folder (`mkdir -p '<path>'`, the path single-quoted for the shell) or remove the share from the Herdfile. When the path is missing because a symlink on it (the share itself or a parent) points at a missing target, the error says so and asks to create the target or fix the link instead. Every problem across all VMs is reported at once.
- The two Terramino Herdfiles keep their `mkdir -p` step but drop "needed until M1.21", since creating the folder stays the user's job.
- README: the `shared_folders` row says the host folder must exist.

Out of scope:
- Vagrant's `create: true`. The user chose a check only (option a, 2026-11-18), so vmherd never creates folders on its own.
- Checking at Herdfile load time. Loading must keep accepting missing folders so that `status`, `destroy`, `down` and `ssh` work on a project whose folder was deleted.
- Rewording the `virsh start failed` hint in `internal/vm`. Once the check runs first, that hint is no longer reached for a missing share. A folder deleted between the check and the boot is not worth a special case.

**How Vagrant does it:** before booting, Vagrant refuses with "The host path of the shared folder is missing: <path>", unless the folder has `create: true`, in which case it creates it. vmherd matches the refusal and leaves out `create: true`.

## Capabilities

### New Capabilities
None.

### Modified Capabilities
- `shared-folders`: a new requirement, "Host folders must exist before VMs are created or started".

## Impact

- `internal/config/sharedpath.go`: a new exported check over a list of VMs, which only needs `os.Stat`. It is unit-testable without libvirt.
- `cmd/vmherd/up.go`, `cmd/vmherd/reload.go` and `cmd/vmherd/restart.go` call it before any change.
- `test/e2e/`: a new case, `TestUpRefusesMissingShareFolder`, runs the real binary and checks that nothing is left behind. It runs in the normal integration CI, including PRs.
- `test/acceptance/terramino-*/Herdfile` comments, and `README.md`.
- `milestone.md`: M1.21 links this change.
