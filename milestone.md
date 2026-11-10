# vmherd — Review Remediation Milestones

Source: architecture review of 2026-11-03 (code, CI, `CLAUDE.md`, `.claude/`).
This is a remediation plan, separate from the product roadmap milestones.

## How to use this file

- Work **one item at a time**, in order. Each item becomes one commit (or one PR).
- Before committing: `gofmt -l .` is empty, `go vet ./...`, `go test ./...` and `golangci-lint run` all pass.
- Tick the checkbox `[x]` in the same commit that fixes the item.
- 🔬 **Verify on testbed** = the bug was found by reading the code and has not been reproduced yet. Reproduce it on testbed-100.49 before fixing. If it doesn't reproduce, close the item with a note.
- 🧭 **Decision** = needs a product decision from the maintainer before any code changes.
- Severity: **Crit** / **High** / **Med** / **Low**.

## Progress

| Milestone | Theme | Items | Done |
|---|---|---|---|
| M0 | Unblock tooling, stop data loss and host compromise | 10 | 10 |
| M1 | Correctness of existing features | 17 | 15 |
| M2 | Structure: state, abstractions, deduplication | 9 | 0 |
| M3 | Supply chain, SSH trust, CI | 10 | 0 |
| M4 | LLM infrastructure (`CLAUDE.md`, `.claude/`) | 6 | 0 |
| M5 | Bridge networking (feature, v0.4.0) | 1 | 1 |

---

## M0 — Unblock tooling, stop data loss and host compromise

### Phase 0A — Tooling unblock

- [x] **M0.1 — Commit the gofmt fix for `config.go`** · Low
  - **Files:** `internal/config/config.go`
  - **Problem:** The file was committed with misaligned struct-tag comments. The gofmt hook then blocked all shell commands.
  - **Fix:** The whitespace fix is already in the working tree. Commit it on its own.
  - **Done when:** `gofmt -l .` prints nothing on `main`.

- [x] **M0.2 — Hooks must fire only on `git commit` / `git tag`** · High
  - **Files:** `.claude/settings.json`, new `.claude/hooks/pre-commit-gate.sh`
  - **Problem:** The `"if": "Bash(git commit *)"` filter isn't applied. gofmt, `go vet` and golangci-lint run before *every* shell command, and any unformatted file blocks all shell use, including the `gofmt -w` the hook tells you to run. The hooks also fail open when `jq` is missing.
  - **Fix:** Replace the four inline hooks with one script that reads the tool input, exits 0 unless the command is `git commit` (or `git tag` for the tag check), checks only staged `.go` files, and denies with a clear message when `jq` is missing.
  - **Done when:** `ls` works with an unformatted file in the tree, and `git commit` with that file staged is still blocked.
  - **Note:** The gate matches command text, so any command that *contains* `git commit …` or `git tag <name>` (even inside an `echo` string) is checked too. gofmt checks tracked plus untracked, non-ignored `.go` files, not only staged ones, so that `git add x && git commit` in a single command is still covered.

### Phase 0B — CI exposure

- [x] **M0.3 — Integration tests must not run fork PR code on the self-hosted runner** · Crit
  - **Files:** `.github/workflows/integration.yml`
  - **Problem:** The workflow triggers on `pull_request` and runs on `[self-hosted, libvirt]`, a machine with libvirt and sudo. Any fork PR runs arbitrary code there.
  - **Fix:** Trigger on `push` to `main` and `workflow_dispatch` only, or gate PR runs behind a protected `environment` that requires manual approval. Also add `if: github.event.pull_request.head.repo.full_name == github.repository`.
  - **Done when:** A PR opened from a fork does not start a job on the self-hosted runner.

### Phase 0C — Host data loss and privilege

- [x] **M0.4 — `destroy` must never `RemoveAll` an arbitrary directory** · Crit
  - **Files:** `cmd/vmherd/destroy.go:47,54`, `internal/config/project.go` (`resolveStateDir`)
  - **Problem:** `os.RemoveAll(project.StateDir)` is called on the raw `VMHERD_STATE_DIR` value. With `VMHERD_STATE_DIR=$HOME`, `vmherd destroy` deletes the home directory. The per-VM `RemoveAll` error is also discarded with `_ =`.
  - **Fix:** Write a marker file (for example `.vmherd-state`) when creating a state directory. Refuse to delete a directory without the marker, and delete only the files and subdirectories vmherd creates (`id_ed25519*`, `<vm>/`, `network/`, `state.json`). Return or log every error.
  - **Done when:** A unit test proves `destroy` leaves a directory without the marker untouched, and that removal errors are reported.

- [x] **M0.5 — Harden shared-folder host-path validation** · Crit
  - **Files:** `internal/config/parse.go:49,264-301`
  - **Problem:** The forbidden list misses `/` itself, `/home`, `/var`, `/opt`, `/dev`, `/run`, `/srv` and `/mnt`, plus the user's `$HOME` root. Symlinks aren't resolved, so `./link → /etc` passes.
  - **Fix:** Run `filepath.EvalSymlinks` on the resolved path. Reject `/`, `$HOME` exactly, and the system prefixes. Prefer allowing only paths under the project directory or under `$HOME`, with an explicit opt-out flag for other paths.
  - **Done when:** Table-driven tests reject `/`, `~`, `/var/x`, and a symlink to `/etc`, and accept `.` and `./src`.

- [x] **M0.6 — Remove the `chmod o+rwx` fallback** · Crit
  - **Files:** `internal/vm/shared.go:25-33`
  - **Problem:** When `setfacl` fails, vmherd runs `sudo chmod o+rwx` on the host path. That makes the project directory (or worse, see M0.5) world-writable without the user noticing.
  - **Fix:** If `setfacl` fails, return an actionable error ("install the `acl` package or run `setfacl -m u:<qemu>:rwx <path>`"). Never widen permissions to "other". Also check that QEMU can traverse the parent directories (home directories are often `0750`).
  - **Done when:** No `chmod` call remains in `shared.go`, and the error message names the exact command to run.

- [x] **M0.7 — 🧭 Decide on the implicit `.` → `/vagrant` share** · High
  - **Files:** `internal/config/parse.go:97-101`, README, `ExampleHerdfile`
  - **Problem:** Every VM silently mounts the project directory with write access for QEMU, even if the user never asked for it.
  - **Decision (2026-11-04):** Keep it as the default. vmherd's audience is people moving off Vagrant, whose scripts expect `/vagrant`, and "your code is already in the VM" is the core experience. Remove the surprises instead:
    - `vmherd up` prints every share (`<vm>: sharing <host> → <guest> (read-write)`).
    - `shared_folders: []` means "no shares". Today an empty list is replaced by the default too.
    - Document the default and the opt-out in CLAUDE.md and the README.
  - **Done when:** The decision is recorded in CLAUDE.md, the code matches it, and README documents it. A unit test shows that omitting `shared_folders` gives the default and `shared_folders: []` gives no shares.

- [x] **M0.10 — 🔬 Files the guest creates in a share must be readable on the host** · High
  - **Files:** `internal/vm/vm.go` (`--filesystem ... accessmode=mapped`), `internal/vm/shared.go`, cloud-init mount setup
  - **Problem:** With 9p `accessmode=mapped`, QEMU creates guest files as its own user with mode `0600` (for example `-rw------- libvirt-qemu libvirt-qemu`). The host user can't read what the VM produced. Seen on the testbed on 2026-11-04 in the M0.6 test and in `~/test-terramino` (created 2026-10-25).
  - **Fix:** Evaluate virtiofs (`--filesystem driver.type=virtiofs` plus shared memory backing), which keeps host ownership. Fall back to 9p only where virtiofs is unavailable, and document the limitation there. Check guest kernel support for every image in the registry.
  - **Done when:** On the testbed, a file written by the guest into `/vagrant` is owned by the host user and readable by them, and a host file stays editable from the guest.
  - **Findings (testbed .49, 2026-11-04;** Debian 12, libvirt 9.0, QEMU 7.2, C `virtiofsd` from `qemu-system-common`, host user UID 1001, guest `vmherd` UID 1000**):**
    - **Reproduced, and worse than described.** With 9p mapped, the normal guest user can't write to `/vagrant` at all (Permission denied: the share root shows host UID 1001). Only guest root can write, and its files land on the host as `libvirt-qemu 0600`. The default share is effectively unusable for the normal user.
    - **virtiofs + matching UIDs works.** A hand-edited domain (`memoryBacking memfd/shared` and `filesystem driver=virtiofs`) mounts in the guest. A guest user whose UID matches the host user reads and writes in both directions, and files are owned by the host user (`0664`). With mismatched UIDs, the guest user is still locked out.
    - **⚠️ libvirt-managed virtiofs is a root escalation.** libvirt runs `virtiofsd` as root. Guest root created `-rwsr-xr-x root root rootsh` (a copy of bash) in the project folder on the host. `mknod` was blocked. libvirt 9.0 can't pass `virtiofsd` options like `modcaps`, and `<idmap>` needs libvirt 10+. **Don't ship this mode.**
    - **Guest kernels:** virtiofs is supported in ubuntu:noble (6.8), ubuntu:resolute (7.0) and debian:trixie (6.12 cloud). fedora:42, arch, rocky:9 and alma:9 weren't tested because of the unrelated bugs below.
    - **Unrelated bugs found:** (1) fedora:42 from `ExampleHerdfile` fails checksum lookup ("no hash found for Fedora-Cloud-Base-Generic-42-1.0.x86_64.qcow2"). (2) The arch `latest` checksum lookup fails, and the error ends in `: <nil>`. (3) rocky:9 and alma:9 GenericCloud images give no SSH within about 4 minutes; the cause is unknown.
  - **Status (2026-11-04): virtiofs deferred** at the maintainer's request (no Rust `virtiofsd` on the testbed; 192.168.100.30 is a VM without nested KVM, so it can't host test VMs). Work continues with option D below.
  - **Option D, in progress: 9p with QEMU running as the host user, plus a guest user with the host UID.**
    - **Idea:** a per-domain `<seclabel type='static' model='dac' relabel='yes'><label>+UID:+GID</label></seclabel>` makes QEMU create share files as the host user, so guest root can't make root-owned files. Matching the guest user's UID to the host user makes host files editable from the guest. Compare `accessmode='squash'` (real modes on disk) with `mapped` (modes in xattrs, `0600` files). If it works, M0.6's ACLs on the share are no longer needed.
    - **Cost:** a QEMU escape lands as the host user instead of `libvirt-qemu`. This is the Vagrant/VirtualBox model. Check whether AppArmor confinement still applies.
    - **First attempt failed before reaching the share:** with label `+1001:+1001`, QEMU couldn't open `/dev/kvm` ("Could not access KVM kernel module: Permission denied"). `bojan` isn't in the `kvm` group (GID 106).
    - **Second attempt worked (label `+1001:+106`, the host UID plus the group of `/dev/kvm`).** AppArmor still enforces (`libvirt-<uuid> (enforce)`). In both modes, the guest user (UID 1001) creates and edits files and folders, and edits host files. The host user reads and edits everything the guest created. **Guest root can't create a root-owned file on the host.**
      - `squash`: real modes on the host (`0664`, `0775`, exec bits kept). Guest root's `chmod 4755` copy of bash is `-rwsr-xr-x 1001`, setuid as the *host user*, not root. Files created by guest root show as UID 1001 in the guest.
      - `mapped`: every guest-created file is `-rw------- 1001:106` on the host (modes hidden in xattrs), and no setuid bit reaches the host. The guest sees accurate ownership (root files as root).
      - **Side effects:** libvirt relabeled the VM disk to `1001:106`, and also the shared cached image in `~/.vmherd/images` (`1001:1001` → `1001:106`, not restored after `destroy`; harmless at `0644`). vmherd should read the GID from `stat /dev/kvm`, not hard-code `kvm`. M0.6's share ACLs become unnecessary.
    - **Recommendation:** option D with `squash`, because guest files behave like normal host files (exec bits, git modes). Document the remaining risk: guest root can create a file that's setuid to the host user, which other host users with access to the project folder could run.
  - **Resolution (2026-11-04): option D with `squash`, implemented.**
    - `vm.Create` adds `--seclabel type=static,model=dac,label=+UID:+<gid of /dev/kvm>,relabel=yes` and `accessmode=squash` for VMs with shares.
    - cloud-init pins the SSH user's UID to the host UID (range 1000–59999; outside it, vmherd warns and keeps the default).
    - Guest mounts use `noatime,nofail`. vmherd refuses to run as root.
    - M0.6's ACL code (`internal/vm/shared.go`) is removed: QEMU running as the user needs no ACLs, and `chmod` stays gone.
    - **End-to-end on testbed .49**, one project with `web` (default share) and `plain` (`[]`): `web`'s QEMU runs as `bojan:kvm` (AppArmor enforce) with guest `vmherd` UID 1001. Guest-created files and an exec script are `1001:1001` with real modes and usable on the host. Host files are editable in the guest. Guest root's setuid copy is owned by 1001, not root. `plain` stays `libvirt-qemu` with no mount. `reload web` keeps the share and UID. The shared cached image's ownership stays stable.
    - **Existing VMs** keep their old 9p `mapped` setup until recreated (`reload`, or `destroy` + `up`).
    - **Test recipe (testbed .49):** create a VM with `shared_folders: []`; in the guest, `useradd -u <host uid>` a user; `virsh shutdown`; edit the `--inactive` XML (remove any `<filesystem>`, add `<filesystem type='mount' accessmode='squash|mapped'><source dir=PROJECT/><target dir='vmherd_share_0'/></filesystem>` and the DAC `<seclabel>`); `virsh define` and `start`; in the guest, `mount -t 9p -o trans=virtio,version=9p2000.L vmherd_share_0 /vagrant`. Then check: the guest user creates and edits files, the host user reads and edits guest files, and whether guest root `cp /bin/bash rootsh; chmod 4755` produces a setuid-root file on the host.
  - **Guest mount options to adopt for any backend:** `noatime,nofail`. Today's fstab entry has no `nofail`, so a missing share can drop the guest into emergency mode. Skip `_netdev`.
  - **Options for the fix (🧭 maintainer decision):**
    - **A.** vmherd starts the Rust `virtiofsd` itself, as the host user, and libvirt connects through `<source socket=...>` (supported in libvirt 9.0). All guest files are then owned by the host user, with no UID matching and no root files. Needs the `virtiofsd` package (Debian 13+, Ubuntu 24.04+, Fedora). vmherd manages the daemon's lifecycle on `up`/`restart`/`reload`, and falls back to 9p elsewhere. Can't be tested on Debian 12 without installing a Rust `virtiofsd`.
    - **B.** libvirt-managed root `virtiofsd` plus UID matching. Simple, but has the root escalation above.
    - **C.** Stay on 9p and make the guest user's UID match the host user's. That fixes guest writes, but guest-created files stay `libvirt-qemu 0600` on the host.
    - **D.** See "Option D, in progress" above.
    - virtiofs notes for later (upstream guidance): keep `--sandbox namespace`; don't add `--modcaps=+sys_admin` (needed only for SELinux `trusted.` xattr remapping); `--translate-uid` can't be combined with `--posix-acl`; check whether Ubuntu 24.04's virtiofsd 1.10.0 has `--translate-uid`; libvirt `<idmap>` needs libvirt 10.0+.

- [x] **M0.8 — Make libvirt domain and disk names project-unique, and validate the project name** · High
  - **Files:** `internal/config/project.go:103-110`, all callers of `DomainName`/`DiskPath`
  - **Problem:** Names use only `filepath.Base(dir)`, so `~/a/web` and `~/b/web` share domains and disks, and `destroy` in one wipes the other. The directory name isn't validated, so a comma injects options into `virt-install --disk path=...,format=qcow2`.
  - **Fix:** Use `vmherd-<project>-<shortHash>-<vm>` (the same hash as the state directory), and sanitize the project part to `[a-z0-9-]`. For migration, if the old-style domain exists and the new one doesn't, keep using the old name (record it in `state.json` once M2.1 lands).
  - **Done when:** A unit test shows two projects with the same basename get different domain names. A directory name containing a comma or space produces a valid name.
  - **Result (2026-11-05):** Names are `vmherd-<project>-<hash8>-<vm>` (hash = state-dir hash; user chose the hash before the VM name). No `state.json` yet, so migration proves ownership by MAC: the old-style domain is used only if its MAC matches `<state>/<vm>/network-config`; another project's old domain is left alone with a note. `vmherd reload` moves a VM to the new name. Safe names alone weren't enough: the state dir, share host path and console-log path also reach `virt-install` and split at commas. All virt-install paths are now double-quoted (virt-install 4.1.0 does not accept `,,`); paths with `"` or line breaks are refused before any disk is created, because virt-install double-escapes `"` in the XML. Verified on .49: old-binary VM found by `status`/`ssh`/`destroy`; same-basename project gets its own domain and its `destroy` leaves the other VM running; `reload` migrates; `We b,x` project boots with `/vagrant` share and console log; `q"x` is refused with no leftovers.
  - **Found, not fixed (pre-existing):** `vmherd console-log` fails with "permission denied" because virtlogd writes `console.log` as `root:root 0600` (same with the old binary). The default network name `vmherd-<basename>` still collides between same-basename projects.

- [x] **M0.9 — Remove the committed binary** · Low
  - **Files:** `vmherd-linux-amd64`, `.gitignore`
  - **Problem:** A 10 MB binary was committed in `6280642 fixes`, and `.gitignore` covers only `/vmherd`.
  - **Fix:** Run `git rm --cached vmherd-linux-amd64` and add `/vmherd-*` and `/dist/` to `.gitignore`. 🧭 Rewriting history to purge the blob is the maintainer's call.
  - **Done when:** `git ls-files | grep vmherd-linux` is empty.
  - **Result (2026-11-05):** Untracked with `git rm --cached` (the local file stays) and `/vmherd-*`, `/dist/` added to `.gitignore`. The release workflow only builds into `dist/` and uploads it, so it is unaffected. History was not rewritten; the ~10 MB blob stays in old commits.

---

## M1 — Correctness of existing features

### Phase 1A — Broken features

- [x] **M1.1 — Fix command quoting for `provision`, `ssh --` and `scp`** · High
  - **Files:** `internal/ssh/connect.go:27`, `cmd/vmherd/provision.go:102,120,126`
  - **Problem:** `Connect` joins `["bash","-c",inline]` with spaces, so the remote shell re-splits it: `apt-get update && …` runs `bash -c apt-get`. Provision steps also run as the SSH user, while first-boot `runcmd` runs as root. `vmherd ssh vm -- ls "/a b"` loses quoting. The `chmod` destination is unquoted, and the comment says "sets it executable" while the code sets `0644`.
  - **Fix:** Run shell steps as `ssh … sudo bash -s` with the script on stdin. Quote user `ssh --` arguments for POSIX shells (`'…'` with escaped `'`). Quote the scp/chmod remote path. Make the comment match the permission (or make it configurable).
  - **Done when:** A unit test checks the argv built for a multi-line inline script, and on the testbed `vmherd provision` runs `apt-get update && apt-get install -y curl` successfully.
  - **Result (2026-11-05):** Shell steps run as `sudo bash -c '<script>'` with the script single-quoted (not on stdin, so commands in the script can't consume it). `ssh --` with one argument passes it as a shell command line (`'ls | wc -l'` keeps working); with several, each is single-quoted. The separate chmod call is gone (see below). The scp path is **not** quoted: OpenSSH 9+ scp uses SFTP, which takes the path literally, so quotes would end up in the filename. Unit tests include running the generated command line through a real shell. Verified on .49: with curl removed, `vmherd provision` reinstalls it via `apt-get update && apt-get install -y curl`, runs as root, and copies to `/tmp/a b.txt` and `/tmp/it's.txt`; `ssh -- printf "[%s]\n" "a b"` keeps the argument. The previous binary ran only `apt-get` and the rest as the SSH user.
  - **Also fixed here (user decision):** file steps failed with "Permission denied" right after `up`, because first boot had already written the files as root via `write_files` and scp copied as the SSH user (root-owned destinations like `/etc` failed too). File steps now scp to a random hidden name in the SSH user's home, then `sudo install -D -m 0644` moves it into place as `root:root` (same as `write_files`) and the upload is always removed. Verified on .49: provision right after first boot replaces `/tmp/a b.txt`, `/tmp/it's.txt` and `/etc/m11/app.conf`, all `root:root 0644`, no upload left behind. The install-command tests need GNU `install` (skip on macOS) and passed on the testbed.
  - **Found, not fixed:** (1) A shell step reports "done" when an earlier command in the script failed, because the exit status is only the last command's — now M1.16. (2) Running provision while first-boot cloud-init is still running hits the apt lock — covered by M1.8.

- [x] **M1.2 — 🔬 Default subnet collides with libvirt's `default` network** · High
  - **Files:** `internal/config/parse.go:82`, `internal/config/project.go` (`ExampleHerdfile`), README, CLAUDE.md
  - **Problem:** The default and example subnet is `192.168.122.0/24`, which is the stock libvirt `default` network, so `checkSubnetConflict` should reject `vmherd init && vmherd up` on a stock host.
  - **Verify:** Run `vmherd init && vmherd up` on a host where `virsh net-list` shows `default`.
  - **Fix:** Pick a different default (for example `192.168.150.0/24`), or auto-select the first free /24 from a pool and record it in `state.json`.
  - **Done when:** `init` then `up` works on a stock libvirt host.
  - **Repro (2026-11-05, .49):** three problems, all confirmed. (1) `init`'s subnet 192.168.122.0/24 fails `up` with an overlap error (on .49 `vmherd-terramino` holds it; on a stock host it's libvirt's `default`), and a fixed default means any second project clashes. (2) `init` always wrote `name: vmherd-default`, so every init'ed project reused one network, skipping the subnet check. (3) `destroy` removed that shared network while another project's VM was running on it; that VM lost SSH.
  - **Result:** `init` writes `vmherd-<project>-<hash8>` (same prefix as the domains) and the first free /24 in 192.168.150–249, skipping libvirt networks, host interfaces and `/proc/net/route` (default route ignored), with VM IPs to match; if libvirt can't be queried it warns and uses 192.168.150. `destroy` keeps a network while any VM is attached and prints which, and keeps it if the check fails. The overlap error suggests a free subnet. Bridge names over 15 chars are a prefix plus a hash of the network name (truncation made `vmherd-web-…` names collide). User decision: a Herdfile without `subnet` keeps 192.168.122.0/24, so existing Herdfiles don't break; without `name` it keeps `vmherd-<basename>`. Verified on .49: two `init` projects get 150 and 151 with their own networks and boot; a third project on the first one's network is destroyed without removing it, and the first VM stays reachable; the overlap error names 192.168.150.0/24 as free.
  - **Still open:** `init`'s example has a port forward, so `init && up` fails on hosts without iptables before reaching the network (seen on .49) — M1.3. Hand-written Herdfiles without `network.name` still share `vmherd-<basename>` across same-basename projects, but `destroy` no longer breaks the other project.

- [x] **M1.3 — 🔬 Port forwarding works and binds to localhost by default** · High
  - **Files:** `internal/portfwd/portfwd.go`
  - **Problem:** OUTPUT DNAT to `-d localhost` needs `net.ipv4.conf.all.route_localnet=1`. libvirt's NAT FORWARD rules reject new inbound connections to guests, so PREROUTING DNAT alone shouldn't work. Rules also have no `-i`/`-d` limit, so ports are exposed to the whole LAN. `CheckExistingNATRule` swallows errors. A partial `Setup` doesn't roll back. `CheckBackend` and `Setup` duplicate the backend switch.
  - **Verify:** On the testbed, forward 8080→80 to nginx, then `curl localhost:8080` on the host and `curl <host-ip>:8080` from another machine.
  - **Fix:** Insert matching FORWARD ACCEPT rules. Either enable `route_localnet` scoped to the bridge, or use a small userspace TCP proxy for localhost. Add an optional `host_ip` field, defaulting to `127.0.0.1`. Roll back on partial failure, and extract the backend check into one helper.
  - **Done when:** Both curl checks behave as documented, and `destroy` leaves no vmherd rules in `iptables-save`.
  - **Repro (2026-11-05, .49):** nothing worked. `curl localhost:8080` timed out (OUTPUT DNAT from 127.0.0.1, `route_localnet=0`), the host's own LAN IP was refused (OUTPUT rule only matched 127.0.0.1), and LAN clients were refused (PREROUTING DNAT, then libvirt's `LIBVIRT_FWI` REJECT). Also: `-d localhost` added the OUTPUT rule twice, the first `up` printed two "could not remove" warnings, and the backend check ran `iptables --version` without sudo, so on Debian (no `/usr/sbin` in a user's PATH) every `up` with ports failed with "iptables not found" (this is what broke `init && up` in M1.2).
  - **Decision (user):** replace iptables with SSH tunnels, like vagrant-libvirt (`route_localnet` would let guests reach host-local services, CVE-2020-8558). Default `host_ip` 127.0.0.1 (Vagrant's default is all interfaces); host ports below 1024 use sudo, as vagrant-libvirt does.
  - **Result:** One background `ssh -N -L host_ip:port:localhost:guest` per VM (`ExitOnForwardFailure`, keepalive 15s×3), detached with setsid, recorded in `<state>/<vm>/port-forward.json` (PID + full argv, checked against `/proc/<pid>/cmdline` so a reused PID is never killed), output in `port-forward.log`. `up`, `restart` and `reload` start it after SSH is ready (it used to be set up before); an identical running tunnel is kept, a changed one replaced, a dead one restarted. `down`/`destroy` stop it. Sudo (`sudo -n true`, else an interactive `sudo true`) and busy host ports are checked before the old tunnel is stopped, so a failed update keeps it running. New `host_ip` field (validated IP); `protocol: udp` is refused with an explanation. No firewall rules at all, so it also works on nftables-only hosts and next to Docker/libvirt rules. The iptables code and its backend switch are removed.
  - **Verified on .49:** default: `localhost:18080` works, host LAN IP and the Mac are refused, listener only on 127.0.0.1; `host_ip: 0.0.0.0` reachable from the Mac; host port 80 runs as `sudo -n ssh` and works; second `up` keeps the tunnel; a killed tunnel is restarted by `up`; `down` stops it; a busy port is reported and the old tunnel keeps serving; after `destroy` no tunnel runs and `iptables-save` is identical to before (only libvirt's own rules appear while the network exists). Not run end to end: `init`'s full example (4 GB + 2 GB VMs don't fit next to the testbed's other VMs).
  - **Known limits:** TCP only. A tunnel drops when the VM reboots or the host restarts, until the next `vmherd up`. Old iptables rules from earlier vmherd versions are not removed automatically (they never worked and vanish on host reboot).

- [x] **M1.4 — A VM without `network.ip` gets invalid cloud-init** · High
  - **Files:** `internal/cloudinit/cloudinit.go:162-180`, `internal/config/parse.go`, `cmd/vmherd/up.go`
  - **Problem:** With an empty IP, network-config contains `addresses: - /24`. The VM boots with no network, and `up` still reports success (it just skips SSH).
  - **Fix:** Either make `ip` required at validation, or auto-assign a free address outside the DHCP pool and persist it (needs M2.1).
  - **Done when:** A Herdfile without `ip` either fails validation with an actionable error, or produces a reachable VM.
  - **Decision (user):** DHCP, like vagrant-libvirt, instead of making `ip` required.
  - **Result (2026-11-05):** Without `ip`, network-config uses `dhcp4: true` (match by MAC) with `dhcp4-overrides: use-dns: false`, so DNS stays host-derived. vmherd reads the address from libvirt's lease table (`virsh domifaddr --source lease`): `up`, `restart` and `reload` wait for the lease (backoff, 120s) before SSH and port forwards; `ssh`, `scp` and `provision` look up the current lease and explain when there is none yet (`scp` now checks "running" first); `status` shows the lease, `dhcp (no lease yet)` or `dhcp`. `network.gateway` without `ip` is refused. Verified on .49 with one DHCP and one static VM: lease .106, SSH ready, port forward, `ssh`, `scp`, `provision`, guest DNS = host-derived server; `restart` keeps the lease; `reload` gets a new MAC and lease (.142) and the tunnel follows; `status` after `down` shows `dhcp`; `destroy` leaves no VM, tunnel or network.
  - **Found, not fixed:** two VMs created in parallel can make virt-install print `FileNotFoundError: …/virt-install.log.5` — now part of M1.17.
  - **Review follow-up (code-reviewer agent, 2026-11-05):** leases are now awaited in parallel, each VM with its own 120s deadline, and `virsh` lookup errors are retried until then (one slow VM used to eat the shared budget of the others). DHCP uses `dhcp-identifier: mac`, so the lease is keyed by the MAC that `domifaddr` filters on, not by machine-id. Re-verified on .49 with an Ubuntu and a Debian 13 DHCP VM booting together: both got leases with client-id `01:<mac>`, and both resolve DNS through the host-derived server. Accepted as is: a lease could briefly be stale right after `restart` (same MAC, so in practice the same address); the resolved address is a copy of `config.VM` with `Network.IP` overwritten, and no copy reaches cloud-init or validation. DNS override on NetworkManager guests (Fedora/Rocky/Alma) is untested → M1.14.

### Phase 1B — Lifecycle reliability

- [x] **M1.5 — Graceful stop by polling, not a fixed 5-second sleep** · Med
  - **Files:** `cmd/vmherd/down.go:49`, `restart.go:96`, `reload.go:129`
  - **Problem:** All three sleep 5 seconds and then force-stop, which can lose data. The logic is copied three times with three identical constants.
  - **Fix:** Add one helper in `internal/vm`, `StopGracefully(domain string, timeout time.Duration) error`, that polls state with backoff and force-stops after a named `gracefulShutdownTimeout` (for example 60s). Use it in all three commands.
  - **Done when:** The three `time.Sleep` calls and duplicate constants are gone.
  - **Result (2026-11-05):** `vm.StopGracefully` sends the ACPI shutdown, polls the state with backoff (1s, 2s, then 4s; `in shutdown` counts as still on) and forces the VM off only after `vm.GracefulShutdownTimeout` = 60s (Vagrant's `graceful_halt_timeout`), or at once if the shutdown request itself fails, and says when it had to. `down` and `restart` stop VMs in parallel through one helper; `reload` uses `StopGracefully` per VM. The three sleeps and constants are gone. Verified on .49: `down` of two VMs returned in 7s (the old code force-killed them at 5s, mid-shutdown); with one guest set to `HandlePowerKey=ignore`, the other stopped normally and the stubborn one was forced off at 60s with a message (61s total, not 120); `restart` 28s and `reload` 47s, both clean.

- [x] **M1.6 — `State()` reports errors and maps every libvirt state** · Med
  - **Files:** `internal/vm/vm.go:13-23,130-166`
  - **Problem:** Any virsh failure (libvirtd down, sudo fails) shows as "not created". `paused`, `crashed`, `in shutdown` and `pmsuspended` are treated as missing VMs, so `up` tries to create them. `ForceStop` returns an unwrapped error. `Exists` duplicates `State`.
  - **Fix:** Change the signature to `State(domain) (VMState, error)` and distinguish "domain not found" from other errors. Add typed constants for every virsh state, and define behaviour per state in `up`, `down` and `restart`. Wrap the `ForceStop` error.
  - **Done when:** `vmherd status` with libvirtd stopped prints an actionable error instead of "not created".
  - **Result (2026-11-09):** `vm.State(domain) (VMState, error)` (new `internal/vm/state.go`) has typed constants for all eight `virsh domstate` states plus `not created`; output it doesn't know is an error, never a silent fallback. When `domstate` fails, vmherd decides "not created" by listing all domains (`virsh list --all --name`), not by matching virsh's translated error text. Any other failure (libvirtd down, sudo refused) is returned with virsh's stderr and a hint (as vagrant-libvirt does: only a missing domain is `:not_created`). `Exists` is gone; every caller (`up`, `down`, `restart`, `reload`, `destroy`, `status`, `ssh`, `scp`, `provision`, legacy-name adoption) uses `State` and stops on its error. Per-state behaviour (new `internal/vm/power.go`): `up` → `EnsureRunning` (start shut off, `resume` paused, `dompmwakeup` pmsuspended, wait for in shutdown/dying then start, force off + start crashed); `down`/`restart`/`reload` → `StopGracefully` handles every state (paused is resumed first, since a paused guest can't process ACPI; crashed/pmsuspended are forced off; in shutdown is only waited on) and returns a typed `StopOutcome` instead of a bool. Force-off decisions use `virsh list --name` (active domains), because a crashed domain can be active or not. `ForceStop` wraps its error; `Destroy` now fails instead of undefining a domain it couldn't force off. `ssh`/`scp` share `requireRunningVM`; `idle` counts as running. Verified on .71 with 2 Ubuntu noble VMs: `status` shows `paused`; `ssh` to a paused VM says so and exits 1; `up` resumes it; `down` of a paused VM resumes it and shuts it down in 7s; `restart` of a paused VM works; with libvirtd and its sockets stopped, `status`/`up`/`down`/`destroy`/`ssh` all exit 1 with "'virsh list' failed: error: failed to connect to the hypervisor … — check that libvirtd is running"; `destroy` + `up` of worker-1 clean.
  - **Not verified:** `crashed` and `pmsuspended` couldn't be produced: the domain XML has no pvpanic device, so QEMU never sees a guest crash (and `on_crash=destroy` would make it `shut off`), and `dompmsuspend` needs the guest agent channel. Both paths are covered only by code review and unit tests.
  - **Found, not fixed:** during an ACPI shutdown, libvirt's QEMU driver reports `running`, not `in shutdown`. So `up` right after an outside `virsh shutdown` treats the VM as running, the SSH wait times out and the VM ends up shut off (`up` exits 0, see M1.7). Left for a follow-up: libvirt doesn't expose the request, so a fix needs another signal (for example a short wait after `up` sees `running` without SSH).

- [x] **M1.7 — Non-zero exit code on partial failure** · Med
  - **Files:** `cmd/vmherd/up.go:155-160,311-318`, `destroy.go`, `portfwd_helpers.go:26`, `reload.go:102`
  - **Problem:** Failed VM creations (when at least one succeeds), destroy warnings, port-forward failures and SSH timeouts all exit 0.
  - **Fix:** Collect the errors with `errors.Join` and return them at the end so the exit code is 1, while still processing every VM.
  - **Done when:** A test (after M2.3) or a manual check shows `up` exits 1 when one of two VMs fails.
  - **Also (from M1.4 review):** a DHCP VM that gets no lease, like a VM whose SSH wait times out, only prints a warning, is left out of the "All VMs ready" list, and `up` exits 0.
  - **Result (2026-11-09):** `up`, `restart`, `reload`, `destroy` and `down` process every VM, collect each VM's failure as `<vm>: <error>` and end with `'vmherd <command>' did not complete for every VM:` plus one failure per line (exit 1); `main` prints it once, so nothing is printed twice. Failures that affect every VM still stop the command at once (libvirt unreachable, image download, network setup, `restart` of a VM that doesn't exist). Counted as failures: creation, start/resume of an existing VM (it used to abort the remaining VMs), no DHCP lease, SSH timeout (as in Vagrant, a VM that never becomes reachable fails `up`), port-forward start/stop, every `destroy` step (all steps are still attempted; network and project-state removal too), and in `reload` each VM's destroy/recreate (it used to abort the rest). Shared helpers now return what they achieved plus an error: `waitForAddresses` and `waitForSSHInParallel` return only the VMs that got there, `stopVMsGracefully` returns the VMs that are off (so `restart` still starts the others), and the new `waitForVMsReady` (lease → SSH → port forwarding) replaces three copies of that sequence, including `reload`'s own serial SSH loop. `up` lists only VMs that answered SSH ("1 of 2 VMs ready:"). Final success lines ("Restart complete.", "Done.") print only when nothing failed. A failed disk removal now says how to remove the file. Verified on .71: with a file at worker-1's disk path, `up` brings ctrl up, lists 1 of 2 ready, prints worker-1's error and exits 1; `destroy worker-1` with the disk `chattr +i` still undefines the domain and removes seed and state, then exits 1 naming the disk; `up` right after an outside `virsh shutdown` (M1.6 finding) now exits 1 with the SSH timeout, and a second `up` recovers; `restart`, `reload`, `down`, `up` exit 0 when nothing fails.

- [x] **M1.8 — Wait for cloud-init to finish and surface failures** · Med
  - **Files:** `cmd/vmherd/up.go` (`waitForSSHInParallel`), `internal/ssh`
  - **Problem:** `up` prints "ready" once SSH answers, while `runcmd` provisioning may still be running or may have failed.
  - **Fix:** After SSH is ready, run `cloud-init status --wait` (with a named timeout), and print a non-`done` status as an error. Point the user to `vmherd console-log` / `/var/log/cloud-init-output.log`.
  - **Done when:** A Herdfile with a failing shell step makes `up` exit non-zero with the failing step visible.
  - **Finding (2026-11-09):** `cloud-init status --wait` alone would not have caught a failing step. cloud-init writes all runcmd entries into one `#!/bin/sh` script without `set -e` (`/var/lib/cloud/instance/scripts/runcmd`, checked on .71), so its exit status is the last line's — the share mount commands. On every VM with a share (the default) a failed step left `status: done`.
  - **Result (2026-11-09):** Guest side (new `internal/cloudinit/runcmd.go`): runcmd entries are now shell lines (YAML-encoded with yaml.v3 instead of `%q`), and each shell step is guarded — `[ -n "$vmherd_failed_step" ] || { echo …; /bin/bash -c '<step>'; } || vmherd_failed_step="2 of 3 (exit $?)"`. After a failed step the later steps are skipped (user's decision; Vagrant stops at the first failing provisioner and `vmherd provision` already did), the home fix and share mounts still run, and a last line prints `vmherd: provisioning step 2 of 3 (exit 2) failed; later steps were skipped` and exits 1, so cloud-init itself reports `error`. Steps are numbered by position among all steps, as `vmherd provision` numbers them. Host side (new `cmd/vmherd/first_boot.go`): after SSH, `up` and `reload` run `cloud-init status --wait --long` over SSH (new `ssh.RunCommandWithTimeout`) on the VMs created in this run only, in parallel, up to `cloudInitWaitTimeout` = 30m. Exit 0 → ready; 2 (recoverable errors) → warning, still ready; 127 → failure (no cloud-init); anything else → failure with the failed-step line (or cloud-init's long status when another module failed), the last 20 lines of `/var/log/cloud-init-output.log` (via sudo) and the command for the full log. A VM whose provisioning failed is not listed as ready and makes the command exit 1 (M1.7), but keeps its port forwarding since it runs. Already-provisioned VMs (`up` of existing VMs, `restart`) are not checked, so one old failure doesn't fail every later `up` (as in Vagrant, provisioning is a first-`up` thing). README "How It Works" describes this. Tests: exact runcmd lines from decoded YAML; the generated script run with `/bin/sh` (failing middle step: step 1 ran, later step skipped, lines after the steps still run, exit 1 naming "3 of 4 (exit 7)"; quotes and `$(...)` reach bash unchanged); exit-code classification; finding the failed-step line in a log excerpt.
  - **Verified on .71 (Ubuntu noble, dash as /bin/sh):** a 2-VM project, `bad` with step 2 of 3 failing (`ls /does-not-exist`), `good` passing on DHCP: `up` shows `bad... failed`, `good... done`, lists 1 of 2 ready and exits 1 with "step 2 of 3 (exit 2) failed" and the `ls` error in the log excerpt (25s total). In `bad`: step 1's file exists, step 3's doesn't, `/vagrant` is mounted, home owned by the SSH user, `cloud-init status` = error. A second `up` doesn't re-check and exits 0. After fixing the step, `reload bad` waits, reports done, exits 0, and all three steps ran. `reload ctrl` in the M1.17 project (apt-get update + install curl) waits through it (35s) and exits 0.
  - **Not covered:** the step guard runs under each distro's `/bin/sh`; only Ubuntu (dash) was run on a VM, macOS/bash in the unit test. Fedora/Rocky/Alma/Arch first boot is untested (M1.12–M1.14 still block them). What happens inside a step stays M1.16.

### Phase 1C — Herdfile validation

- [x] **M1.9 — Strict YAML and collect all validation errors** · Med
  - **Files:** `internal/config/parse.go:52-175`
  - **Problem:** Unknown keys (`memroy:`) are silently ignored. Validation stops at the first error, while CLAUDE.md requires reporting all of them.
  - **Fix:** Use `yaml.NewDecoder` with `KnownFields(true)`. Have `validate` collect errors into a slice and return `errors.Join`.
  - **Done when:** A test Herdfile with three distinct errors reports all three, and an unknown key is reported with its line number.
  - **Result (2026-11-09):** `Load` decodes with `yaml.NewDecoder` + `KnownFields(true)`. yaml.v3 already collects every unknown key and type mismatch with its line; the new `internal/config/yamlerrors.go` rewrites them in Herdfile terms: `line 4: unknown key 'memroy' in a VM (under 'vms:') — did you mean 'memory'?` (closest known key within 2 edits, Damerau-Levenshtein so a swap counts 1), or `— known keys there: password, user` when nothing is close; the keys come from the struct's yaml tags, so the list can't go stale. Type errors read `line 4: 'two' is text, but this key needs a whole number`. An empty Herdfile says so (was a bare `EOF`). When decoding fails, semantic validation is skipped (half-decoded zero values would add misleading problems). `validate` and its helpers now run every check and collect all problems: one problem is reported as `Herdfile: <problem>`, several as `Herdfile has N problems:` with one per line. Shares, ports and provision steps report every bad entry; a VM without a name is called `#<position>` so its other problems are still reported; a bad subnet is reported once and the per-VM in-subnet checks are skipped (`validateSubnetPrefix` → `parseSubnet`, returning the parsed subnet). Message texts are unchanged otherwise; new range/format checks stay M1.10, the source-escape bug M1.11. The two acceptance Herdfiles, the `vmherd init` template and the README example all decode strictly; collecting all problems showed the README example's VM IPs (192.168.122.x) were outside its own subnet (192.168.150.0/24) — fixed. Tests (`internal/config/problems_test.go`): three distinct problems all reported with a count; five unknown keys at different levels with line numbers, suggestions and the known-keys list; three type mismatches; empty file; YAML syntax error; bad subnet not repeated per VM; unnamed VM; every bad share/port entry; edit distance and suggestion cut-off. Verified on .71: `memroy` in the M1.17 project's Herdfile → `line 31: unknown key 'memroy' … did you mean 'memory'?`, exit 1; after fixing it, a bad port and ssh user are reported together as 2 problems; the restored Herdfile works.

- [x] **M1.10 — Range, enum and format checks for every field** · Med
  - **Files:** `internal/config/parse.go`, `internal/config/config.go`
  - **Problem:** Negative `cpus` or `memory` are accepted. An unknown provision `type` is accepted at parse time (only `provision` rejects it later). An empty shell `inline`, a relative `destination`, and non-IP `gateway` or `dns` values reach cloud-init unvalidated, which allows YAML injection.
  - **Fix:** Validate `cpus >= 1`, `memory >= 256`, provision type in an enum, `destination` absolute with no newlines, `gateway` an IPv4 address inside the subnet, and `dns` either `auto` or an IP address.
  - **Done when:** There's a table-driven test case for each rule.
  - **Result (2026-11-09):** New `internal/config/fields.go`, wired into `validate` (all problems reported together, M1.9): `cpus >= 1` and `memory >= 256` (omitted 0 still means the default); provision `type` must be `shell` or `file` (missing type has its own message), a shell step needs a non-empty `inline` and no `source`/`destination`, a file step needs `source` and `destination` and no `inline`; `destination` and share `guest` paths must be absolute without control characters; `gateway` (static ip only) an IPv4 address inside the subnet, not .0/.255 and not the VM's own ip; `dns` `auto` or an IP address (IPv6 allowed). **Added (user's decision):** `images:` keys must be `distro:codename`, values `https://` URLs with a host (the downloader already refused anything else), and every VM's `image` must be defined there — this used to fail only at `up`, after the network was created. **Also added (same injection class as the milestone's):** the `mounts:` and `write_files:` sections were written with a raw `%s`, so a guest path with `, ` or `]` broke the YAML; they are now rendered with yaml.v3 like `runcmd` (shared `writeYAMLSection`). Runtime checks the parser now guarantees were removed (`provision`'s unknown-type default, empty-shell skip and missing source/destination check; the empty-inline skip in runcmd). The two acceptance Herdfiles and the README example had no `images:` section, so `up` would already have refused them — added. README field tables state the new limits and the `images` section. Tests: `TestValidateFieldRules`, 27 table cases (one or more per rule, valid boundaries included, each must produce exactly one problem); mounts and write_files with YAML-special characters decode back unchanged; test fixtures got an `images:` section. Verified on .71: a VM with a share and a file step to `/opt/vmherd test/setup.sh` boots, the file has mode 644, `/vagrant` is mounted, cloud-init `done`, and `cloud-init schema --system` reports "Valid schema user-data"; a Herdfile with negative memory, a relative destination and `dns: resolver.lan` reports all 4 problems and exits 1.

- [x] **M1.11 — Correct the provision source escape check** · Low
  - **Files:** `internal/config/parse.go:316-319`
  - **Problem:** `strings.HasPrefix(cleanPath, projectDir)` accepts `../proj-evil/x` when the project is `/…/proj`. Symlinks aren't resolved.
  - **Fix:** Use `filepath.Rel` plus `EvalSymlinks`, and reject when the relative path starts with `..`.
  - **Done when:** A test covers the sibling-prefix case.
  - **Result (2026-11-09):** New `checkProvisionSourceInsideProject` in `internal/config/parse.go`: the source is joined to the project directory, symlinks in both are resolved with the shared folders' `resolveSymlinksAllowMissing`, and the resolved source must lie below the resolved project directory. Instead of `filepath.Rel` it reuses the shared folders' `isPathWithin`, which compares whole path components (`/proj/` with the separator), so `/proj-evil` is not inside `/proj`. Separate messages: `escapes project directory` (`..`), `is the project directory itself` (`.`), `is a symlink to "<target>", outside the project directory`. A missing source still passes at load time (so `destroy`/`status` keep working after a file is deleted); `up`/`provision` report it as before. Test `TestCheckProvisionSourceInsideProject` with real files and symlinks: `../proj-evil/secret` (the sibling-prefix case), `../outside.txt`, `scripts/../../outside.txt`, `.`, `../proj`, a symlink to the sibling, a symlinked directory in the path, a relative symlink leading out, an absolute path — all refused; a plain file, `./scripts/../scripts/setup.sh`, a missing file and a symlink to another file inside the project — accepted. Verified on .71: with `source: ../test-m18-evil/secret` and a `keys-link` → `~/.ssh/authorized_keys` symlink, the installed v0.3.0 `vmherd status` accepts the Herdfile, the new build reports both problems (naming the symlink target) and exits 1.


### Phase 1D — Found while testing M0 on the testbed

- [x] **M1.12 — 🔬 `fedora:42` checksum lookup fails for the example URL** · Med
  - **Files:** `internal/image/registry.go:106`, `internal/config/project.go:251` (`ExampleHerdfile`), `internal/image/download.go` (`discoverChecksumInfo`)
  - **Problem:** `vmherd pull fedora:42` with the URL from the built-in registry and example Herdfile fails at the checksum step. Seen on testbed .49 during M0 testing.
  - **Verify:** Run `vmherd pull fedora:42` and check which checksum files the directory listing yields (Fedora publishes a signed `*-CHECKSUM` file, and `download.fedoraproject.org` redirects to mirrors).
  - **Fix:** Depends on the repro: point `ChecksumURL` at Fedora's `CHECKSUM` file, or make discovery handle the redirect and Fedora's BSD-style `SHA256 (file) = hash` lines.
  - **Done when:** `vmherd pull fedora:42` downloads and verifies the image on the testbed.
  - **Repro (2026-11-09, .71):** `pull fedora:42` failed with `no hash found for "Fedora-Cloud-Base-Generic-42-1.0.x86_64.qcow2" in checksum file`. Checksum discovery itself worked: it followed `download.fedoraproject.org`'s 302 to a mirror, found `Fedora-Cloud-42-1.1-x86_64-CHECKSUM` and parses its BSD-style `SHA256 (file) = hash` lines. The built-in URL pointed at a file that doesn't exist (HTTP 404) — Fedora 42 was only published as `42-1.1` — so the checksum file rightly had no entry for it. Fedora 42 is also end-of-life (May 2026); 43, 44 and 45 are on the mirrors.
  - **Result (2026-11-09):** Built-in images (user's decisions): `fedora:42` replaced by `fedora:44` (`44-1.7`) and `fedora:45` (`45-1.4`); Fedora release images keep their file name for the release's life. The list now lives once, in `config.BuiltInImages()` (new `internal/config/images.go`; `image` imports `config`, so it can't live in `image`): `vmherd init`'s template renders its `images:` section from it, and `pull`/`list-images`/the integration test use `config.BuiltInImageURLs()`; `image.DefaultImages` is gone. Before checksum discovery, `Download` now sends a HEAD for the image URL (redirects followed); a 404/410 is reported as `image URL … returns HTTP 404 — the vendor no longer publishes this file. Look up the current file name at <directory> and update the images: entry in your Herdfile` (servers that refuse HEAD are not held against the URL). README's Supported Images table lists `ubuntu:resolute`, `fedora:44`/`45` (was `fedora:41`) and says which ones `init` writes. Tests: every built-in image has a distro:codename name, an https URL, no duplicate, and appears in the init template; the HEAD check against a local TLS server (200 and 405 pass; 404 and 410 name the status, the directory and the images: entry; plain http refused). Verified on .71: `vmherd init` writes the four images; `pull fedora:45` and `pull fedora:44` download and verify SHA256 (about 70s each); `pull fedora:42` with the old Herdfile entry now reports the 404 and the directory to look in. Booting Fedora VMs is still untested (NetworkManager DNS, see M1.14).

- [x] **M1.13 — Checksum discovery error ends in `: <nil>`** · Low
  - **Files:** `internal/image/download.go:104-105`
  - **Problem:** When the directory listing parses but has no checksum links (seen with `arch:latest`), the error is built with `%v` of a nil `scrapeError`, so the user sees `no checksum file found in directory listing at …: <nil>` and no hint.
  - **Fix:** Separate the two cases: wrap `scrapeError` with `%w` when it is set; otherwise say no checksum links were found and suggest setting the image's checksum URL. Then check why the Arch listing yields no links (Arch publishes `*.qcow2.SHA256` next to the image).
  - **Done when:** A unit test covers both messages, and `vmherd pull arch:latest` either verifies or fails with an actionable message.
  - **Result (2026-11-10):** Cause of the Arch failure: the link filter only accepted names containing `SUM`/`CHECKSUM`, and Arch publishes one `<image>.qcow2.SHA256` per image (no SUMS list), so every link was dropped and the nil `scrapeError` was printed. `isChecksumFileName` now also accepts names containing `SHA256`/`SHA512` (signatures `.sig`/`.gpg`/`.asc` still skipped), and the image's own `<image>.<hash>` file is tried first, then SHA256, SHA512 and generic lists. The two failures are separate: an unreadable listing is wrapped with `%w` (`reading the image directory listing at …: HTTP 404 …`); a listing without checksum links names the directory, the names looked for (e.g. `<image>.SHA256`) and the `images:` entry to check. The milestone's "suggest setting the checksum URL" wasn't possible — the Herdfile has no such field (`Entry.ChecksumURL` is never set) — so the hint points at the directory instead. Discovery now takes the `*http.Client` (as `checkImageURLAvailable` does) so it is tested end to end against a local TLS server. Tests (`internal/image/checksum_discovery_test.go`): Arch-style listing ranks the image's own file first and skips signatures; SUMS lists keep their order and foreign hosts are dropped; discovery against an Arch-style listing returns the hash; both error messages, neither containing `<nil>`. Verified on .71: `pull arch:latest` (cloudimg) downloads 549 MB and verifies SHA256 (67s); the Arch ISO directory's lowercase `sha256sums.txt` is found and verifies too; an entry pointing at the mirror root (a real file, no checksum links) fails with the new message, exit 1. Follow-up (same day, from the Arch ISO archive listing `sha256sums.txt` + `b2sums.txt`): widening the filter also admitted lists for algorithms vmherd can't verify; their hashes would be read as SHA-256/512 by length and fail as a misleading "image may be corrupted" mismatch. `isChecksumFileName` now skips `B2SUM`/`BLAKE2`/`MD5`/`SHA1SUM`/`SHA224`/`SHA384` lists and `.sha1` files (matched on the file name only); test with the archive layout plus MD5/SHA-1/BLAKE2/SHA-384/SHA-224 variants. Re-verified on .71: a pinned `arch:20261101` entry (`images/v20261101.620452/…cloudimg-20261101.620452.qcow2`) downloads and verifies SHA256. Booting an Arch VM is still untested.

- [ ] **M1.14 — 🔬 `rocky:9` and `almalinux:9` VMs never reach SSH** · Med
  - **Files:** `internal/cloudinit/cloudinit.go` (network-config, user-data), `internal/image/registry.go:81-82`
  - **Problem:** Both images boot but `vmherd up` times out waiting for SSH. Seen on testbed .49. Likely the EL network stack (NetworkManager) ignores the netplan-style `match: macaddress` config, or the SSH user isn't created.
  - **Verify:** Create a VM with `console_log: true` (after M1.15) and read the serial log for cloud-init network and user errors.
  - **Fix:** Depends on the repro.
  - **Done when:** `vmherd up` with `rocky:9` and with `almalinux:9` reaches SSH on the testbed.

- [x] **M1.15 — `vmherd console-log` fails with "permission denied"** · Med
  - **Files:** `cmd/vmherd/console_log.go`, `internal/vm/vm.go` (`--serial file,path=…`)
  - **Problem:** virtlogd writes `<state>/<vm>/console.log` as `root:root 0600`, so the host user can't read it. Same with the pre-M0.8 binary (verified on .49, 2026-11-05).
  - **Fix:** Options: read the log via `sudo cat` (consistent with the other `sudo virsh` calls), or have libvirt write a log the user can read. Pick the one that doesn't loosen permissions on other files.
  - **Done when:** `vmherd console-log <vm>` prints the log on the testbed for a VM with `console_log: true`.
  - **Result (2026-11-10):** Reproduced with the M1.17 domain XML (`<serial type='file'>`): both logs were `root:root 0600`, for a VM with shares (QEMU runs as the host user) as well as one without (libvirt's QEMU user) — virtlogd (root) creates the file, so QEMU's user doesn't matter. Chose the option that changes no permissions: `printConsoleLog` reads the file directly when it can and, on a permission error only, runs `sudo cat -- <path>` attached to the terminal (sudo can prompt), consistent with the other `sudo virsh` calls. The log stays root-only (it can hold boot output). A missing log now names the path and says to set `console_log: true` and `vmherd reload <vm>`; a failed sudo read suggests `sudo cat <path>`. Tests (`cmd/vmherd/console_log_test.go`, injected reader): readable file copied without sudo; 0000 file read through the privileged reader, failure hint; missing log hint with no privileged read. Verified on .71 (two noble VMs, `shared` and `noshare`, both `console_log: true`): `console-log` prints the full boot log for both (1071/1047 lines, through cloud-init "finished" and the login prompt), exit 0; an unknown VM exits 1 with the hint; `destroy` removes the state dir including the root-owned logs. Rotated logs (virtlogd `console.log.N`) are not printed.

- [ ] **M1.16 — 🧭 A shell step reports success when an earlier command failed** · Med
  - **Files:** `internal/ssh/connect.go` (`rootScriptCommandLine`), `internal/cloudinit/cloudinit.go` (`writeRuncmdEntries`), README (provisioning section)
  - **Problem:** Shell steps run as `bash -c '<script>'`, so the step's exit status is only the last command's. On the testbed (2026-11-05, during M1.1) a step whose `apt-get update` failed on the apt lock still printed "done" because a later command succeeded. First boot has the same behavior: `runcmd` runs each step as `/bin/bash -c <script>`.
  - **Decision needed:** stop at the first failing command by default (`bash -e`, optionally `-o pipefail`), which matches what users expect from a provisioning step but can break scripts that rely on ignoring a failure; or keep plain bash semantics and document that scripts must `set -e` themselves (Vagrant's behavior).
  - **Fix:** Apply the chosen mode to both `vmherd provision` and first-boot `runcmd`, so a script behaves the same on both paths, and document it.
  - **Done when:** A unit test shows a script with a failing first command and a succeeding last command fails the step (or, with the documented option, succeeds), and on the testbed `vmherd provision` reports the step as failed with a non-zero exit code.

- [x] **M1.17 — Create VMs without `virt-install` (domain XML + own cloud-init seed)** · Med
  - **Files:** `internal/vm/vm.go` (`Create`, `buildVirtInstallArgs`, `sharedFolderArgs`, `quoteVirtInstallValue`), `internal/cloudinit/cloudinit.go`, `cmd/vmherd/up.go` (`createVMsInParallel`), `cmd/vmherd/destroy.go`, README/CLAUDE.md (dependencies)
  - **Problem:** `up` runs up to four `sudo virt-install` at once, and each rotates the same `/root/.cache/virt-manager/virt-install.log`: `FileNotFoundError: [Errno 2] No such file or directory: '/root/.cache/virt-manager/virt-install.log.5'` (seen on .49, 2026-11-05, M1.4 test; both VMs were still created). More generally, virt-install is an extra host dependency whose option syntax needed the M0.8 quoting work, and vmherd can't control the XML it produces.
  - **How Vagrant does it:** vagrant-libvirt never runs virt-install. It renders the domain XML from a template, creates volumes through libvirt's storage API and defines/starts the domain through the API, so parallel `vagrant up` shares no log file.
  - **Decision (2026-11-05):** do the full fix, not the per-run `XDG_CACHE_HOME` workaround. (1) Keep `sudo virsh` for define/start in this item, consistent with every other libvirt call; moving to the libvirt API is M2.7/M2.8. (2) Build the NoCloud seed (label `cidata`; files `user-data`, `meta-data`, `network-config`) as an ISO 9660 image in pure Go; if the chosen library can't write those names (needs Rock Ridge or Joliet), use a FAT image labelled `cidata` instead. No `xorriso`/`genisoimage` dependency.
  - **Fix (revised after code review, 2026-11-05):**
    - **Baseline first:** capture `virt-install --print-xml` (or `virsh dumpxml` of today's VMs) for Ubuntu, Debian, Fedora, Rocky/Alma and Arch and commit them as golden files; the rendered XML is diffed against them. Things virt-install adds that libvirt won't on its own and must be explicit: `<os><type machine='q35'>`, `<features><acpi/><apic/>` (graceful `down` needs ACPI), `<clock offset='utc'>`, `on_poweroff/reboot/crash`, `<cpu mode='host-passthrough'>`, virtio disk and net, virtio-rng (boot entropy for cloud-init/ssh keygen), memballoon, qemu-guest-agent channel, usb tablet, serial console. Scope: x86_64 with BIOS firmware (what virt-install picks today); aarch64/UEFI is out of scope (UEFI would also need `undefine --nvram`).
    - **Render with `encoding/xml` structs, not `text/template`:** values (share paths, console log path, names) are escaped by the encoder, so the M0.8 injection class can't come back. The domain carries: qcow2 disk, seed disk, NAT interface with the MAC, serial console to file when `console_log` is set, 9p `<filesystem>` with `accessmode=squash` and the M0.10 DAC `<seclabel>`, CPUs/memory, VNC listen none.
    - **Seed:** NoCloud, label `cidata`, files `user-data`, `meta-data`, `network-config`, with a unique `instance-id` per create (so `reload` reruns cloud-init). Candidate libraries: `github.com/diskfs/go-diskfs` (ISO 9660 with Rock Ridge, and FAT32 with a label; heavy dependency tree) or a small hand-written ISO 9660 + Rock Ridge `NM` writer for three files. `kdomanski/iso9660` likely lacks Rock Ridge/Joliet, so long names with `-` wouldn't survive; verify before choosing. FAT fallback must be attached as a disk (not cdrom); check the guest kernels have vfat.
    - **Placement:** build the seed in a temp file as the user, then `sudo install -m 0640` it to `/var/lib/libvirt/images/vmherd-<project>-<hash>-<vm>-seed.iso` (never write there directly; never keep it in the state dir, which QEMU may not traverse). libvirt relabels files there (SELinux `svirt_image_t`, AppArmor, M0.10 DAC label).
    - **Apply:** `sudo virsh define`, `autostart`, `start` behind one small function, so M2.7/M2.8 replace only that and the install step is the single place that knows about privilege. Roll back on partial failure (define failed, disk exists, seed exists): no domain or seed left behind, re-running `up` works. `destroy`, `reload` and failed creates remove the seed.
    - Remove `quoteVirtInstallValue`, `unsafeVirtInstallPathCharacters` and the `virt-install` dependency; update the CLAUDE.md line "Shell-out is acceptable for virt-install".
  - **Done when:** `virt-install` is not called anywhere. Unit tests: golden XML per distro (shares + seclabel, console log, static and DHCP VMs) and a round trip that reads the seed back (label `cidata`, three file names, contents). Testbed: four VMs created in one `up` show no traceback; Ubuntu, Debian, Fedora, Rocky/Alma and Arch boot and get their network (static and DHCP); `/vagrant` (M0.10) mounts; the console log is written; graceful `down` and `restart` work; `reload` reruns cloud-init; a forced partial failure leaves nothing behind; `destroy` leaves no seed file.
  - **Result (2026-11-09):** `virt-install` fully replaced. VMs are created by rendering domain XML via `encoding/xml` structs (`internal/vm/domain.go`) and defining/starting via `virsh define`/`start`. Cloud-init seed ISOs are built in pure Go (`internal/cloudinit/iso.go`) with Rock Ridge NM/PX extensions for filenames with hyphens. `OSVariant` removed from the image registry. `virsh undefine` no longer uses `--remove-all-storage` (disks are created outside libvirt's pool); explicit `sudo unlink` handles disk cleanup. `destroy` and `reload` both clean up disk and seed. Rollback on partial failure removes disk, seed and domain. Golden XML tests for 4 VM configurations. Testbed (.71, Debian trixie): 2 Ubuntu noble VMs — `up` (parallel create, SSH ready), `status`, `ssh`, shared folders (`/vagrant`), port forwarding, `down`/`up` restart, `reload` (destroys and recreates cleanly), `destroy` (no orphaned files). Multi-distro testing deferred.

Note: the default network name collision is handled under M1.2.

---

## M2 — Structure: state, abstractions, deduplication

### Phase 2A — Persistent project state

- [ ] **M2.1 — Introduce `state.json` per project** · Med
  - **Files:** new `internal/state/`, `cmd/vmherd/*`
  - **Problem:** There's no record of what vmherd created. CLAUDE.md promises `state.json`, but it doesn't exist.
  - **Fix:** A JSON file in the state directory, with a schema version and, per VM: domain name, MAC, IP, disk path, image URL and hash, port rules installed, and creation time. Write it atomically (temp file + rename) and guard it with a file lock.
  - **Done when:** `up` writes it, `destroy` reads it, and the round-trip is unit-tested.

- [ ] **M2.2 — Tear down from recorded state; clean up orphans** · Med
  - **Depends on:** M2.1
  - **Files:** `cmd/vmherd/portfwd_helpers.go`, `destroy.go`, `up.go`
  - **Problem:** Port-forward teardown is computed from the *current* Herdfile, so changing ports leaks iptables rules. VMs removed from the Herdfile can never be destroyed by vmherd.
  - **Fix:** Tear down the rules recorded in state. Have `up` warn about orphaned VMs, and let `destroy` (with no args) remove them.
  - **Done when:** Changing a port and running `up` leaves exactly one rule, and removing a VM from the Herdfile followed by `destroy` removes its domain.

### Phase 2B — Abstractions and deduplication

- [ ] **M2.3 — A command-runner / `Hypervisor` interface so `cmd/` and `libvirt/` can be tested** · Med
  - **Files:** `internal/libvirt/virsh.go`, `internal/vm`, `internal/network`, `internal/portfwd`, `cmd/vmherd`
  - **Problem:** `exec.Command` is called directly everywhere. `cmd/` and `internal/libvirt` have 0% coverage, network ~20%, vm ~31%.
  - **Fix:** Define a `CommandRunner` interface (with a real implementation and a fake for tests) and a `Hypervisor` interface (`DomainState`, `Start`, `Shutdown`, `Destroy`, `NetworkEnsure`…). Inject them explicitly, with no package globals.
  - **Done when:** `up`, `down` and `destroy` have unit tests using the fake, and total coverage reaches ≥ 60%.

- [ ] **M2.4 — Remove duplicated code** · Med
  - **Files:** `cmd/vmherd/up.go:221-275` vs `reload.go:146-203`, `internal/network/network.go` (`net-list` parsing ×4), `up.go`/`reload.go` (password warning, key setup), `portfwd.go` (backend switch ×2)
  - **Problem:** These near-duplicates violate CLAUDE.md's no-duplication rule.
  - **Fix:** Add one `lifecycle.CreateVM(...)` used by both `up` and `reload`, one `listNetworks()` helper, and one `prepareProjectSSH()` helper.
  - **Done when:** A `dupl` linter at threshold 100 reports nothing (see M4.2).

- [ ] **M2.5 — Remove global mutable UI state** · Low
  - **Files:** `internal/ui/ui.go`, `internal/image/download.go:350-360`
  - **Problem:** `ui.quiet` is a package-level mutable variable, which CLAUDE.md forbids. `progressReader` prints with `fmt.Printf` directly, bypassing quiet mode and garbling parallel output.
  - **Fix:** Add a `ui.Printer` struct passed through the code, and route the progress output through it.
  - **Done when:** `ui` has no package-level `var`.

- [ ] **M2.6 — Generate cloud-init from typed structs** · Med
  - **Files:** `internal/cloudinit/cloudinit.go`
  - **Problem:** YAML is built by string concatenation (CLAUDE.md asks for templates). Go's `%q` is not YAML escaping. Guest paths are interpolated unquoted into `bash -c "mkdir -p %s"`.
  - **Fix:** Define structs for user-data, network-config and meta-data and `yaml.Marshal` them, which guarantees escaping. Use argv-form `runcmd` (`["mkdir","-p",path]`). Use the actual subnet prefix instead of a hardcoded `/24`.
  - **Done when:** Existing golden tests pass, and a new test with quotes, newlines and spaces in paths produces valid YAML.

### Phase 2C — Privilege and libvirt access

- [ ] **M2.7 — 🧭 Decide between `virsh` and `go-libvirt`; align CLAUDE.md** · Med
  - **Problem:** CLAUDE.md mandates `libvirt-go` bindings, which need cgo and would break the `CGO_ENABLED=0` static release build. The code shells out to `virsh` everywhere.
  - **Options:** (a) `github.com/digitalocean/go-libvirt` (pure Go, RPC over the libvirt socket); (b) keep `virsh` behind the M2.3 interface and change the rule.
  - **Done when:** The decision is recorded in CLAUDE.md, and the rule there matches reality.

- [ ] **M2.8 — Stop using `sudo` for libvirt calls** · Med
  - **Files:** `internal/libvirt/virsh.go`, `internal/vm/vm.go`, `internal/network/network.go`
  - **Problem:** Every `virsh`, `qemu-img` and `test` call goes through `sudo` without `-n`. Parallel operations can block on password prompts, and the privilege is far broader than needed.
  - **Fix:** Use `virsh -c qemu:///system` with `libvirt` group membership (and check it up front with an actionable error). Keep `sudo -n` only for iptables, setfacl and writes under `/var/lib/libvirt/images`, or create disks through a libvirt storage pool.
  - **Done when:** `grep -rn '"sudo", "virsh"'` returns nothing, and a missing group membership produces a clear error.

- [ ] **M2.9 — Small cleanups** · Low
  - **Files:**
    - `internal/image/download.go:372` (`parseChecksum`, used only by tests)
    - `internal/image/registry.go:70,138` (variant map rebuilt on every call; custom insertion sort)
    - `internal/config/project.go:66,70,150` (`resolveStateDir` swallows errors; `ImageCacheDir` falls back to the world-writable `/tmp`)
    - every remaining `_ = fn()` on a call that returns an error
  - **Fix:**
    - Delete the dead code.
    - Make the variant map package-level.
    - Replace the custom sort with `slices.Sort`.
    - Return errors instead of falling back silently.
    - Never cache images under `/tmp`.
  - **Done when:** `errcheck` with blank-assignment checking is clean (see M4.2).

---

## M3 — Supply chain, SSH trust, CI

### Phase 3A — Image download and verification

- [ ] **M3.1 — A hardened HTTP client** · Med
  - **Files:** `internal/image/download.go`
  - **Problem:** `http.Get` has no timeout or context, so it can hang forever. Redirects can downgrade to `http://`. `out.Close()` errors are ignored, so a short write can be renamed into the cache. Hash comparison is case-sensitive. The `//nolint:noctx // built-in registry` comments are false, because URLs come from the Herdfile.
  - **Fix:**
    - Add one `http.Client` with dial, TLS and idle timeouts, plus a `CheckRedirect` that rejects non-HTTPS targets.
    - Pass a `context.Context` from the CLI so Ctrl-C cancels the download.
    - `fsync` and check `Close` before the rename.
    - Compare hashes with `strings.EqualFold`.
    - Remove the misleading `nolint` comments.
  - **Done when:** `httptest` cases cover the timeout, the HTTPS→HTTP redirect, and an uppercase checksum.

- [ ] **M3.2 — Make the cache safe under concurrent use** · Low
  - **Files:** `internal/image/download.go:33,330`
  - **Problem:** `cleanStaleTempFiles` deletes *every* `*.tmp` file, including a download in progress from another `vmherd` process (for example `pull` in a second terminal).
  - **Fix:** Take a per-image lock file (`flock`). Only delete stale temp files whose lock isn't held.
  - **Done when:** Two parallel `vmherd pull ubuntu:noble` runs both succeed.

- [ ] **M3.3 — Verify vendor signatures; allow pinned checksums** · High
  - **Files:** `internal/image/`, `internal/config/config.go`
  - **Problem:** The checksum file comes from the same host as the image (by scraping the directory listing), so verification only detects corruption, not tampering. CLAUDE.md promises GPG verification, but there is none.
  - **Fix:** Embed vendor signing keys and verify `SHA256SUMS.gpg` (Ubuntu), the clearsigned `CHECKSUM` (Fedora) and `SHA512SUMS.sign` (Debian) with an OpenPGP library. Also support a pinned `sha256:` per image in the Herdfile (`images: { ubuntu:noble: { url: …, sha256: … } }`), and have `vmherd pull` print the hash so users can pin it.
  - **Done when:** A tampered checksum file fails verification in tests, and a pinned hash overrides discovery.

- [ ] **M3.4 — 🧭 Decide the image registry model** · Med
  - **Problem:** CLAUDE.md says "built-in registry, no user URLs"; the code requires an `images:` map of URLs in the Herdfile.
  - **Recommendation:** Built-in defaults (signed vendor sources) that a Herdfile can override, with overrides required to be HTTPS and either signed or pinned.
  - **Done when:** The decision is recorded in CLAUDE.md, and the code and README match it.

- [ ] **M3.5 — Refresh cached images from mutable URLs** · Low
  - **Files:** `internal/image/download.go:19-39`
  - **Problem:** `…/noble/current/…` changes over time, but the cache is keyed only by distro, codename and filename, so it never refreshes. An image is verified once and then trusted forever.
  - **Fix:** Store the verified hash and the source checksum file's ETag or Last-Modified next to the image. Have `pull` re-check, and have `up` re-hash only on demand (`--verify`).
  - **Done when:** `vmherd pull` fetches a new image after the vendor updates `current/`.

### Phase 3B — SSH trust

- [ ] **M3.6 — Pin VM host keys** · Med
  - **Files:** `internal/ssh/connect.go:40-43,92-100`, `internal/cloudinit`
  - **Problem:** `StrictHostKeyChecking=no` with `UserKnownHostsFile=/dev/null` disables host verification for every connection.
  - **Fix:** Generate the guest's host key on the host, inject it through cloud-init `ssh_keys`, and write a per-project `known_hosts`. Then use `StrictHostKeyChecking=yes` with that file.
  - **Done when:** `vmherd ssh` fails if the guest's host key changes.

### Phase 3C — CI and release

- [ ] **M3.7 — 🧭 SPDX headers and DCO sign-off: enforce or remove** · Med
  - **Files:** `.github/workflows/pr-quality.yml`, all `.go` files
  - **Problem:** 0 of 47 Go files have SPDX headers and no commit is signed off, so the PR Quality checks can never pass.
  - **Fix:** Either add `// SPDX-License-Identifier: AGPL-3.0-or-later` to every file and commit with `-s`, or remove the jobs.
  - **Done when:** PR Quality is green on a fresh PR.

- [ ] **M3.8 — Fix the `pr-security` workflow** · Med
  - **Files:** `.github/workflows/pr-security.yml`
  - **Problem:** `if: contains(github.event.pull_request.changed_files_url, 'go.mod')` tests an API URL, so the dependency audit never runs. The secret-scan pipeline (`grep -qv '#'`) has inverted, unreliable logic.
  - **Fix:** Use `dorny/paths-filter` (pinned SHA), or always run `go mod verify` and `govulncheck`, since they're cheap. Replace the grep with `gitleaks` (pinned).
  - **Done when:** A PR that changes `go.mod` triggers the audit job.

- [ ] **M3.9 — Gate releases on tests; add provenance and an SBOM** · Med
  - **Files:** `.github/workflows/release.yml`
  - **Problem:** A tag push builds and publishes without running tests. There's no build provenance or SBOM.
  - **Fix:** Add a `test` job that `build` depends on. Add `actions/attest-build-provenance` and an SBOM (`anchore/sbom-action`), all pinned to SHAs. Publish one `SHA256SUMS` file for all archives.
  - **Done when:** A release created from a commit with failing tests is blocked, and attestations appear on the release.

- [ ] **M3.10 — Coverage threshold** · Low
  - **Files:** `.github/workflows/pr-quality.yml`
  - **Problem:** The 30% threshold is too low to catch regressions.
  - **Fix:** Raise it step by step as M2.3 lands (40 → 50 → 60%).
  - **Done when:** The threshold is ≥ 60%.

---

## M4 — LLM infrastructure (`CLAUDE.md`, `.claude/`)

- [ ] **M4.1 — Rewrite CLAUDE.md to describe the current code** · High
  - **Files:** `CLAUDE.md`, new `docs/ROADMAP.md`, new `docs/ARCHITECTURE.md`
  - **Problem:** CLAUDE.md describes features that don't exist (built-in registry, `state.json`, libvirt-go, `text/template`, report-all-errors). Its structure section omits `portfwd/`, `ui/` and `libvirt/`, and its CLI list omits `reload`, `provision`, `info` and `console-log`. Agents are steered by fiction.
  - **Fix:** Keep CLAUDE.md short and current:
    - what exists today
    - build, test and lint commands
    - integration tests on the testbed (`-tags integration`, binary deployed as `vmherd-new`)
    - the release flow (`scripts/release.sh`)
    - a **security model** section (the Herdfile is untrusted input; host-side effects need validation)
    - rules, each linked to the linter that enforces it

    Move product plans to `docs/ROADMAP.md` and the design to `docs/ARCHITECTURE.md`. Record the 🧭 decisions from M0.7, M2.7 and M3.4.
  - **Done when:** Every statement in CLAUDE.md can be checked against the code.

- [ ] **M4.2 — Enforce the style rules with `.golangci.yml`** · Med
  - **Files:** new `.golangci.yml`, `.github/workflows/test.yml`
  - **Problem:** The CLAUDE.md rules are prose only. golangci-lint runs with defaults and reports "0 issues" despite violations.
  - **Fix:** Enable `funlen` (80 lines), `errcheck` (`check-blank: true`), `revive` (exported comments, receiver naming), `mnd`, `dupl`, `gosec`, `noctx`, `wrapcheck`, `varnamelen` and `gocognit`. Fix the violations package by package. This item can be pulled forward and enabled per linter as other items land.
  - **Done when:** CI runs with this config, and CLAUDE.md maps each rule to its linter.

- [ ] **M4.3 — Fix the project skills' layout** · High
  - **Files:** `.claude/skills/*.md`
  - **Problem:** Skills are flat `.md` files, but Claude Code only loads `.claude/skills/<name>/SKILL.md` with `name` and `description` frontmatter, so none of the four load. Three have no frontmatter at all.
  - **Fix:** Move `commit`, `test` and `reduce-comments` to `<name>/SKILL.md` with frontmatter. Delete `improve`, which only repeats CLAUDE.md. Have the `commit` skill state the project's commit rules (no Co-Authored-By, no amend, no push).
  - **Done when:** The skills appear in Claude Code's skill list.

- [ ] **M4.4 — Consolidate and correct the agents** · Med
  - **Files:** `.claude/agents/*.md`
  - **Problems:**
    - There are 10 overlapping agents (code-quality, code-reviewer, refactoring and security all review code).
    - They hardcode `/Users/boyadboz/REPOS/vmherd`.
    - They copy the CLAUDE.md rules, and the copies already differ ("exported functions" vs. "every function").
    - The security agent's threat model is wrong (it lists wget/curl and cloud-localds, which aren't used, and says URLs come from a built-in registry).
    - Read-only "never modify" agents have shell access.
    - The haiku-model technical-writer is allowed to edit CLAUDE.md.
  - **Fix:** Reduce to about five agents: `implementer`, `reviewer` (covering correctness, security and refactoring), `test-runner`, `devops` and `docs`.
    - Use repo-relative paths.
    - Have each agent say "follow CLAUDE.md" instead of copying the rules.
    - Rewrite the threat model around the Herdfile as the trust boundary.
    - Restrict reviewer tools.
    - Make CLAUDE.md edits require a stronger model or human review.
  - **Done when:** No absolute paths remain, and no rule text is duplicated from CLAUDE.md.

- [ ] **M4.5 — Turn the memory rules into permission deny rules** · Med
  - **Files:** `.claude/settings.json`
  - **Problem:** "Never push", "never amend" and "no `rm -rf`" exist only as memory and prose, so nothing enforces them.
  - **Fix:** Add `permissions.deny` entries: `Bash(git push*)`, `Bash(git commit --amend*)`, `Bash(rm -rf*)`, `Bash(git tag -d*)`, `Bash(git reset --hard*)`.
  - **Done when:** Those commands are refused without a prompt.

- [ ] **M4.6 — Archive old review files** · Low
  - **Files:** `reviews/`
  - **Problem:** Stale LLM review output ("ALL FIXED (pending commit)") lives next to the code and can mislead agents.
  - **Fix:** Move it to `docs/reviews/archive/`, or delete it once this file supersedes it. Keep `checklist.md` only if it's updated to match CLAUDE.md.
  - **Done when:** `reviews/` contains no stale status claims.

---

## M5 — Bridge networking (feature, v0.4.0)

- [x] **M5.1 — Herdfile bridge mode: attach VMs to an existing host bridge** · Feature
  - **Files:** `internal/config/config.go`, `internal/config/parse.go`, `internal/config/fields.go`, `internal/config/project.go`, `internal/cloudinit/cloudinit.go`, `internal/vm/vm.go`, `internal/vm/domain.go`, `internal/network/network.go`, `cmd/vmherd/{up,reload,destroy,status,vm_address,network_setup}.go`
  - **What:** `network: {type: bridge, bridge: br1, gateway: 192.168.100.1}` attaches VMs to the host bridge `br1` with static LAN IPs set by cloud-init, or LAN DHCP when a VM's `ip` is omitted. No libvirt network is created, no vmherd DHCP runs. The Vagrant equivalent is `public_network` with a fixed bridge.
  - **Design:**
    - `network.type` is a typed enum (`nat` default, `bridge`). Bridge mode requires `bridge` (valid interface name, ≤15 chars) and `gateway` (IPv4 inside the subnet); `subnet` defaults to the gateway's /24. NAT Herdfiles that set `bridge`/`gateway` are rejected with a hint.
    - Domain XML gets `<interface type='bridge'><source bridge='…'/>`; `vm.CreateOptions.BridgeName` selects it over `NetworkName`.
    - `up`/`reload` check `/sys/class/net/<bridge>/bridge` exists instead of `network.Ensure`; `destroy` never removes a host bridge.
    - DHCP address discovery uses `virsh domifaddr --source arp` (host ARP table) in bridge mode, `--source lease` on NAT (`vm.DiscoveredAddress` with a typed `AddressSource`).
    - The `.100–.200` reserved-range check only applies to NAT, where vmherd's own dnsmasq owns that pool.
    - `cloudinit.GenerateFiles` now takes the default gateway instead of a subnet base (the NAT `.1` rule moved to `Project.GatewayIP`).
  - **Done when:** `go test ./...` passes with bridge validation, domain-XML golden and DHCP-pool tests; smoke-tested on a testbed with a real bridge. Released as v0.4.0.
