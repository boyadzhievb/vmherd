# vmherd

Lightweight VM manager for Linux using libvirt/KVM and official cloud images.

vmherd is a modern, open-source replacement for Vagrant on Linux. It manages local libvirt/KVM virtual machines using official cloud images from distro vendors (Ubuntu, Debian, Fedora, Arch, Rocky, AlmaLinux) with cloud-init provisioning.

**Why vmherd?** Vagrant depends on HCP Vagrant Cloud for box hosting (going EOL Nov 2026). Cloud images from distro vendors are GPG-signed, SHA256-verified, and the same images used by AWS/GCP/Azure. No third-party registry needed.

## Install

### Pre-built binaries

Download the latest release from [GitHub Releases](https://github.com/boyadzhievb/vmherd/releases/latest):

| Platform | Binary |
|----------|--------|
| Linux x86_64 | [vmherd-linux-amd64.tar.gz](https://github.com/boyadzhievb/vmherd/releases/latest/download/vmherd-linux-amd64.tar.gz) |
| Linux ARM64 | [vmherd-linux-arm64.tar.gz](https://github.com/boyadzhievb/vmherd/releases/latest/download/vmherd-linux-arm64.tar.gz) |

```bash
# Example: download and install (amd64)
curl -fsSL https://github.com/boyadzhievb/vmherd/releases/latest/download/vmherd-linux-amd64.tar.gz | tar xz
sudo mv vmherd /usr/local/bin/
```

### From source

```bash
go install github.com/boyadzhievb/vmherd/cmd/vmherd@latest
```

### Build from repo

```bash
git clone https://github.com/boyadzhievb/vmherd.git
cd vmherd
go build -o vmherd ./cmd/vmherd/
sudo mv vmherd /usr/local/bin/
```

### Prerequisites

vmherd requires these packages on your Linux host:

```bash
# Debian/Ubuntu
sudo apt install qemu-kvm libvirt-daemon-system virtinst

# Fedora/RHEL
sudo dnf install qemu-kvm libvirt virt-install

# Arch
sudo pacman -S qemu-full libvirt virt-install
```

Make sure your user is in the `libvirt` group:

```bash
sudo usermod -aG libvirt $USER
# Log out and back in for the group change to take effect
```

## Quick Start

```bash
# Create a Herdfile in your project directory
vmherd init

# Edit the Herdfile to define your VMs, then bring them up
vmherd up

# Check VM status
vmherd status

# SSH into a VM
vmherd ssh ctrl

# Stop all VMs
vmherd down

# Destroy everything (VMs, disks, network, state)
vmherd destroy
```

## Herdfile

vmherd reads a `Herdfile` (YAML) from the current directory to define VMs:

```yaml
vms:
  - name: ctrl
    image: ubuntu:noble
    cpus: 2
    memory: 4096
    disk: 20G
    network:
      ip: 192.168.122.10
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
  dns: auto

ssh:
  user: vmherd
```

### VM fields

| Field | Default | Description |
|-------|---------|-------------|
| `name` | required | VM name (used in all commands) |
| `image` | required | Cloud image in `distro:codename` format |
| `cpus` | 1 | Number of vCPUs |
| `memory` | 1024 | Memory in MB |
| `disk` | 20G | Disk size |
| `network.ip` | — | Static IP address |
| `network.gateway` | subnet .1 | Gateway IP |
| `provision` | — | List of provisioning steps |

### Network fields

| Field | Default | Description |
|-------|---------|-------------|
| `name` | vmherd-{dir} | libvirt network name |
| `subnet` | 192.168.122.0/24 | Network subnet |
| `dns` | auto | DNS server (`auto` reads from host /etc/resolv.conf) |

### SSH fields

| Field | Default | Description |
|-------|---------|-------------|
| `user` | vmherd | SSH user created on all VMs (passwordless sudo) |

## Commands

```
vmherd init                    Create example Herdfile in current directory
vmherd up [name...]            Create and start VMs (all or named)
vmherd down [name...]          Stop VMs gracefully
vmherd destroy [name...]       Stop and remove VMs, disks, and state
vmherd status                  Show VM states in table format
vmherd ssh <name> [-- cmd]     SSH into a VM or run a command
vmherd scp <src> <dest>        Copy files (name:/path syntax)
vmherd list-images             Show available cloud images
vmherd pull <distro:codename>  Download and verify a cloud image
vmherd version                 Show version
```

### Examples

```bash
# Bring up only specific VMs
vmherd up ctrl worker-1

# Run a command on a VM without interactive shell
vmherd ssh ctrl -- uname -a

# Copy a file to a VM
vmherd scp ./config.yaml ctrl:/tmp/config.yaml

# Copy a file from a VM
vmherd scp ctrl:/var/log/syslog ./syslog.txt

# Pre-download an image
vmherd pull debian:bookworm
```

## Supported Images

| Image | Description |
|-------|-------------|
| `ubuntu:noble` | Ubuntu 24.04 LTS |
| `ubuntu:jammy` | Ubuntu 22.04 LTS |
| `ubuntu:focal` | Ubuntu 20.04 LTS |
| `debian:bookworm` | Debian 12 |
| `debian:trixie` | Debian 13 |
| `fedora:41` | Fedora 41 |
| `rocky:9` | Rocky Linux 9 |
| `almalinux:9` | AlmaLinux 9 |
| `arch:latest` | Arch Linux |

All images are downloaded directly from official distro vendors and verified with SHA256 checksums.

## How It Works

1. **Image download** — Official cloud images are fetched from vendor CDNs and cached at `~/.vmherd/images/`. SHA256 checksums are verified against the vendor's published checksums.

2. **SSH key generation** — An ed25519 key pair is generated per project and stored in `.vmherd/` in your project directory.

3. **Cloud-init** — For each VM, vmherd generates cloud-init user-data (SSH key injection, user creation, provisioning), network-config (static IP), and meta-data. These are passed directly to `virt-install --cloud-init` — no external tools needed.

4. **VM creation** — VMs are created via `virt-install` with a qcow2 disk backed by the cached cloud image (copy-on-write, so disk creation is instant).

5. **Idempotent operations** — `vmherd up` creates what's missing, starts what's stopped, and leaves what's running.

## State

| What | Where |
|------|-------|
| Cloud images | `~/.vmherd/images/` |
| SSH keys, cloud-init files | `.vmherd/` in project directory |
| VM disks | `/var/lib/libvirt/images/vmherd-{project}-{name}.qcow2` |
| Networks | libvirt networks prefixed `vmherd-` |

## License

AGPL-3.0. See [LICENSE](LICENSE).
