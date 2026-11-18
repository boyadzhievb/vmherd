# Design

## Context

- `validateSharedFolders` (`internal/config/parse.go`) resolves each share's host path at load time with `resolveSharedFolderHostPath` → `resolveSymlinksAllowMissing`. A missing folder resolves to its would-be path, which is stored in `SharedFolder.ResolvedHost`. That is deliberate, so a deleted folder doesn't break `status`, `destroy`, `down` or `ssh`.
- `up` (`cmd/vmherd/up.go`) does, in order: load, `FilterVMs`, gateway, DNS, `ssh.EnsureKeyPair`, `ensureProjectNetwork`, then `bringUpVMs`. That starts existing domains and creates missing ones (images, disk, seed, `virt-install`). A missing share folder first fails in QEMU at domain start.
- `reload` (`cmd/vmherd/reload.go`) runs `checkReloadPrerequisites` (images, host identity) before `shutdownAndDestroy`, following the M1.19 rule: never destroy before the replacement can be created.

## Goals / Non-Goals

**Goals:** a missing share folder is named before vmherd changes anything, for both new and existing VMs, and every missing folder is reported at once.

**Non-Goals:** creating folders (`create: true`, which the user rejected), checking at load time, changing the `virsh start` hint, checking guest paths.

## Decisions

1. **One pure check in `internal/config`:** `CheckSharedFolderHostPathsExist(vms []VM) error`. For each VM in order, and each share in order, it calls `os.Stat(ResolvedHost)`. Not existing → "VM %q: shared folder host path %q (%s) does not exist — create it with 'mkdir -p %s' or remove the share from the Herdfile". Not a directory → "VM %q: shared folder host path %q (%s) is not a directory — 9p shares a folder; point 'host' at a folder or remove the share". Any other stat error (e.g. permission denied) is wrapped with the path. All problems are joined with `errors.Join`, matching load's "every problem at once". The check lives in config because it only reads the parsed shares, and it needs no libvirt, so it is unit-tested without M2.3.
   - *Alternative rejected:* checking in `validateSharedFolders` at load time. That would break `destroy` and `status` on a project whose folder was deleted.
2. **`up` calls it right after `FilterVMs`**, before the gateway lookup, the key pair and the network. It covers every selected VM, including ones that already exist or are running. A stopped VM with a deleted folder would fail to start the same way, and a running one has a share that no longer exists on the host. Checking all of them keeps the rule simple: `up` never touches anything while a selected VM's folder is missing.
3. **`reload` calls it at the start of `checkReloadPrerequisites`**, before the image downloads. It fails fast, and its error gets the same "reload stopped before changing any VM" suffix as the other prerequisite errors.
4. **Output.** The error goes through the existing command error path, so it is printed once and the exit code is 1. Errors on the Herdfile path use the `host` value as written, and the resolved path is added in parentheses, so a `../shared` share is still recognisable.

## Failure cases

| Case | What vmherd does |
|---|---|
| One VM, folder missing | `up`/`reload` exit 1 naming VM, path, resolved path, `mkdir -p` hint; nothing created or destroyed |
| Several VMs, several missing | One error lists every missing folder in Herdfile order; nothing changes for any VM |
| `up web` with `db`'s folder missing | Passes: only the selected VMs are checked |
| Host path is a file | Fails with "is not a directory" |
| Folder unreadable (stat permission error) | Fails naming the path and the OS error |
| Folder deleted after the check, before boot | QEMU's error as today (race, accepted) |
| `status`, `destroy`, `down`, `ssh` with a missing folder | Unchanged: load accepts it |
| `shared_folders: []` | Nothing to check |

Nothing is removed: the check runs before any change, so no VM, disk, network or key is created or destroyed when it fails (M1.19 rule).

## Risks / Trade-offs

- A running VM whose folder was deleted now makes `up` (on that VM) fail instead of saying "already running". That's intended: the share is broken, and the fix is the same.
- The race between the check and the boot stays, as in Vagrant.
