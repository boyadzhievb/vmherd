# vmherd — Lightweight VM Manager for Linux

## What It Is

**vmherd** is a modern, open-source replacement for Vagrant on Linux. It manages local libvirt/KVM virtual machines using official cloud images from distro vendors (Ubuntu, Fedora, Debian, Arch, etc.) with cloud-init provisioning.

**Why it exists**: Vagrant depends on HCP Vagrant Cloud for box hosting (going EOL Nov 2026). Cloud images from distro vendors are GPG-signed, SHA256-verified, and the same images used by AWS/GCP/Azure. No third-party registry needed.

**Language**: Go. **License**: AGPL-3.0.

---

## Core Design Principles

1. **No external registry.** Images come directly from distro vendors (cloud-images.ubuntu.com, cloud.debian.org, etc.).
2. **Declarative.** A `Herdfile` (YAML) defines VMs — name, image, resources, network, provisioning.
3. **Idempotent operations.** `vmherd up` creates what's missing, starts what's stopped, leaves what's running.
4. **Minimal dependencies.** Only requires: libvirt + qemu-kvm + virt-install.
5. **Single binary.** `go build` produces one binary, no plugins.
6. **SSH-first.** Every VM gets an ed25519 key pair. `vmherd ssh <name>` just works.

---

## CLI Design

```
vmherd init                    # create example Herdfile in current directory
vmherd up [name...]            # create and start VMs (all or named)
vmherd down [name...]          # stop VMs gracefully
vmherd destroy [name...]       # stop and remove VMs + disks
vmherd status                  # show VM states in table format
vmherd ssh <name> [-- cmd]     # SSH into a VM or run command
vmherd scp <src> <dest>        # copy files (name:/path syntax)
vmherd list-images             # show available cloud images
vmherd pull <distro>           # download and verify a cloud image
vmherd version                 # show version
```

**Naming rule**: Use self-explanatory English. Not K8s jargon, not Vagrant jargon where a better word exists.

---

## Herdfile Format

```yaml
# Herdfile
vms:
  - name: ctrl
    image: ubuntu:noble          # resolves to cloud-images.ubuntu.com/noble/current/...
    cpus: 2
    memory: 4096                 # MB
    disk: 20G
    network:
      ip: 192.168.122.10
      gateway: 192.168.122.1
    provision:
      - type: shell
        inline: |
          apt-get update && apt-get install -y curl
      - type: file
        source: ./setup.sh
        destination: /tmp/setup.sh

  - name: worker-1
    image: ubuntu:noble
    cpus: 2
    memory: 2048
    disk: 20G
    network:
      ip: 192.168.122.20

network:
  name: vmherd-default
  subnet: 192.168.122.0/24
  dns: auto                     # derive from host /etc/resolv.conf

ssh:
  user: vmherd                  # default SSH user created on all VMs
```

---

## Image Registry (Built-In)

vmherd ships with a built-in registry of official cloud image URLs. No external service needed.

| Distro | Source | Format |
|--------|--------|--------|
| Ubuntu | cloud-images.ubuntu.com | qcow2 |
| Debian | cloud.debian.org/images/cloud | qcow2 |
| Fedora | download.fedoraproject.org/pub/fedora/linux/releases | qcow2 |
| Arch | geo.mirror.pkgbuild.com/images | qcow2 |
| Rocky | dl.rockylinux.org/pub/rocky | qcow2 |
| AlmaLinux | repo.almalinux.org/almalinux | qcow2 |

All images verified via SHA256 checksums from the vendor. GPG verification where the vendor provides signed checksums.

---

## Architecture

```
Herdfile (YAML) → vmherd CLI → libvirt API (via virsh/virt-install) → KVM VMs
                              → cloud-init (user-data, network-config, meta-data)
                              → cloud images (cached locally, SHA256 verified)
```

### State Management

- Image cache: `~/.vmherd/images/`
- VM state per project: `$XDG_STATE_HOME/vmherd/<project>-<hash>/` (~/.local/state/vmherd/ by default). Override with `VMHERD_STATE_DIR` env var. Query path with `vmherd info ssh-key` or `vmherd info state-dir`.
- Disk images: `/var/lib/libvirt/images/vmherd-<project>-<name>.qcow2`
- Networks: libvirt networks prefixed `vmherd-`

### Provisioning via cloud-init

- SSH key injection (ed25519, generated per project)
- Static IP assignment via network-config
- Shell provisioning via runcmd
- File provisioning via write_files
- DNS from host `/etc/resolv.conf` — never hardcode external DNS (air-gap support)

---

## Code Style Rules

- **Comment every function.** Every exported and unexported function must have a doc comment (max 4 lines).
- **Use long descriptive variable names.** `imageCache` not `ic`, `networkName` not `nn`, `vmDefinition` not `vd`.
- **Document variables.** Struct fields must have inline comments. Named constants/maps should have comments when role isn't obvious.
- **Descriptive function names.** `downloadAndVerifyImage` not `getImage`, `createCloudInitISO` not `makeISO`, `waitForSSHReady` not `waitSSH`.
- **Receiver names match the type.** `imageRegistry` not `r`, `vmManager` not `m`, `herdConfig` not `c`.
- **Error messages must be actionable.** `"cannot find image 'ubuntu:noble': run 'vmherd pull ubuntu:noble'"` not `"image not found"`. Include the failed value and a recovery hint.
- **Wrap errors with context.** `fmt.Errorf("creating VM %s: %w", vmName, err)` — never return bare errors from called functions.

### Anti-Pattern Rules

- **No dead code.** Every exported type/function/constant must have a caller.
- **No magic numbers.** Timeouts, thresholds, defaults → package-level named constants. `const sshConnectTimeout = 180 * time.Second` not `180`.
- **Function length limit: 80 lines.** Break long functions into sub-functions. Excludes test functions.
- **No copy-paste duplication.** Same pattern 3+ places → shared helper. Near-duplicate blocks >10 lines → refactor.
- **Typed enums over raw strings.** VM states (`running`, `stopped`, `creating`), image formats, provisioner types → typed string constants.
- **No shell-out when a library exists.** Prefer `libvirt-go` bindings over `exec.Command("virsh", ...)` where stable bindings exist. Shell-out is acceptable for `virt-install` (no Go equivalent).
- **No swallowed errors.** Never `_ = someFunc()` for functions that return errors. Log or return every error.
- **No global mutable state.** Pass dependencies explicitly. No package-level `var` that gets mutated at runtime.

### Data Structures & Algorithms

- **Maps for O(1) lookups.** `map[string]ImageSource` for distro→URL registry, `map[string]VMState` for tracking VM states. Never linear scan a slice when a map fits.
- **Slices for ordered iteration.** VM creation order matters (network first, then VMs). Sort deterministically when source is a map.
- **Deterministic output.** `vmherd status` must produce same table for same state. Sort VM names before displaying.
- **Concurrent VM operations.** `vmherd up` with multiple VMs → create in parallel with `errgroup`. Bounded concurrency (e.g., 4 VMs at a time). Collect errors, don't fail-fast on first.
- **Streaming hash verification.** SHA256 verify during download (`io.TeeReader` into `sha256.New()`), not download-then-hash. Saves a full read pass.
- **Template-based generation.** cloud-init YAML via `text/template`, not string concatenation. Avoids injection and escaping bugs.
- **Retry with backoff for SSH.** Exponential backoff (1s, 2s, 4s...) up to a ceiling, not fixed-interval polling. Total timeout as named constant.
- **Config validation at parse time.** Validate Herdfile completely before any VM operation. Fail early with all errors, not one at a time.
- **Structured state files.** JSON for `.vmherd/state.json` (machine-readable, diffable). Never invent a custom format.

---

## Project Structure

```
cmd/vmherd/         # CLI entry point (cobra or similar)
internal/
  config/           # Herdfile parsing and validation
  image/            # Image registry, download, verification
  vm/               # VM lifecycle (create, start, stop, destroy)
  network/          # libvirt network management
  cloudinit/        # cloud-init file generation
  ssh/              # SSH key generation and connection
reference/          # CCattler's original bash script (design reference)
```

---

## Reference Material

- `reference/libvirt-vm-ccattler.sh` — The working bash implementation from CCattler that vmherd replaces. Shows the full lifecycle: image download, SHA256 verify, cloud-init ISO creation, virt-install, SSH wait. Use as behavioral reference, not code to port.
- Ubuntu cloud images: https://cloud-images.ubuntu.com/
- cloud-init docs: https://cloudinit.readthedocs.io/
- libvirt API: https://libvirt.org/

---

## Development Workflow

1. `go test ./...` must pass before any commit.
2. `gofmt -w .` + `golangci-lint run` before committing.
3. No `rm -rf` without explicit user permission.
4. No hardcoded DNS servers — always derive from host.
5. Error messages shown to user explicitly, never swallowed.
6. Explain approach before implementing. No opportunistic changes.

---

## Competitive Landscape

| Tool | Status | Why vmherd is different |
|------|--------|------------------------|
| Vagrant | CLI stays, registry EOL Nov 2026 | No registry dependency, official images |
| Multipass | Ubuntu-only, Canonical-controlled | Multi-distro, AGPL, community-driven |
| Lima | macOS-focused, complex | Linux-native, simple, libvirt/KVM |
| virt-manager | GUI, no declarative config | CLI-first, declarative Herdfile |
| Terraform + libvirt | Heavy, HCL, provider maintenance | Single-purpose, YAML, zero plugins |
