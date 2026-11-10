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
sudo apt install qemu-kvm libvirt-daemon-system

# Fedora/RHEL
sudo dnf install qemu-kvm libvirt

# Arch
sudo pacman -S qemu-full libvirt
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
images:
  ubuntu:noble: https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-amd64.img

vms:
  - name: ctrl
    image: ubuntu:noble
    cpus: 2
    memory: 4096
    disk: 20G
    network:
      ip: 192.168.150.10
    shared_folders:
      - host: .
        guest: /vagrant
    ports:
      - host: 8080
        guest: 80
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
      ip: 192.168.150.20

network:
  name: vmherd-myproject-1a2b3c4d   # vmherd init picks a project-unique name
  subnet: 192.168.150.0/24          # and a /24 no other network or route uses
  dns: auto

ssh:
  user: vmherd
  password: changeme              # optional — enables virsh console login
```

vmherd checks the whole Herdfile before doing anything: unknown keys (typos such as `memroy:`) and invalid values are all reported at once, with line numbers for the YAML-level ones.

### Images

| Field | Default | Description |
|-------|---------|-------------|
| `images` | required | Map of `distro:codename` to the vendor's `https://` image URL. Every VM's `image` must be listed here. `vmherd init` writes entries for the supported images |

### VM fields

| Field | Default | Description |
|-------|---------|-------------|
| `name` | required | VM name (used in all commands) |
| `image` | required | Cloud image in `distro:codename` format, defined under `images` |
| `cpus` | 1 | Number of vCPUs (at least 1) |
| `memory` | 1024 | Memory in MB (at least 256) |
| `disk` | 20G | Disk size |
| `network.ip` | — (DHCP) | Static IPv4 address in the subnet, outside the DHCP range .100–.200 (that range only applies to NAT networks). Omit it to use DHCP: vmherd then reads the VM's address from libvirt's lease table (NAT) or the host's ARP table (bridge) for `ssh`, `scp`, `provision`, port forwards and `status` |
| `network.gateway` | subnet .1 (nat) / `network.gateway` (bridge) | Gateway IPv4 address inside the subnet (static `ip` only; with DHCP the gateway comes from DHCP) |
| `shared_folders` | `[{host: ".", guest: "/vagrant"}]` | Host directories shared via 9p virtio. Files keep the same owner and modes on host and guest: QEMU runs as you for VMs with shares, and the guest SSH user gets your UID. Omit it to get the project at `/vagrant`; set `shared_folders: []` for no shares; an explicit list replaces the default. `vmherd up` prints every share. `host` must resolve (symlinks included) inside the project directory or `$HOME`; set `allow_outside_home: true` on an entry to share another path. `/`, `$HOME` itself and system directories are always refused. Rocky and AlmaLinux kernels have no 9p: set `shared_folders: []` for those VMs (`vmherd up` says so if you don't) |
| `ports` | — | Port forwards (`host`→`guest`), carried by one background `ssh -L` tunnel per VM (TCP only; no firewall rules). `host_ip` defaults to `127.0.0.1` (this machine only); set `0.0.0.0` or a host address to expose a port on the network. Host ports below 1024 run the tunnel via `sudo`. `up`, `restart` and `reload` start the tunnel once SSH is ready; `down` and `destroy` stop it |
| `provision` | — | List of provisioning steps: `type: shell` with an `inline` script, or `type: file` with a project-relative `source` and an absolute `destination` in the VM |

### Network fields

| Field | Default | Description |
|-------|---------|-------------|
| `type` | nat | `nat` (vmherd creates and manages a libvirt NAT network) or `bridge` (VMs attach to an existing host bridge and appear on the LAN; vmherd creates no network and runs no DHCP) |
| `name` | vmherd-{dir} | libvirt network name (nat only). `vmherd init` writes `vmherd-{dir}-{hash}` so projects never share a network. `vmherd destroy` keeps the network while VMs of another project are still attached |
| `subnet` | 192.168.122.0/24 | Network subnet. The default is the same as libvirt's stock `default` network, so set it explicitly; `vmherd init` writes the first free /24 in 192.168.150.0–192.168.249.0, skipping libvirt networks and host interfaces and routes. In bridge mode it defaults to the gateway's /24 |
| `dns` | auto | DNS server: `auto` (read from the host's /etc/resolv.conf) or an IP address |
| `bridge` | — | Bridge mode only (required): the host bridge device to attach VMs to, e.g. `br1`. The bridge must already exist; vmherd never creates or removes host bridges |
| `gateway` | — | Bridge mode only (required): the LAN router's IP the VMs route through, e.g. `192.168.100.1` |

### Bridge mode

With `type: bridge` the VMs join your LAN through an existing host bridge — the equivalent of Vagrant's `public_network`, but attached to a named bridge instead of a guessed interface:

```yaml
network:
  type: bridge
  bridge: br1
  gateway: 192.168.100.1
```

Each VM either gets a static LAN address via cloud-init (`network.ip`, any address in the subnet your LAN's DHCP doesn't hand out) or, with `ip` omitted, asks the LAN's DHCP server. For DHCP VMs, vmherd discovers the address from the host's ARP table (`virsh domifaddr --source arp`), which works because the host sits on the same bridge.

### SSH fields

| Field | Default | Description |
|-------|---------|-------------|
| `user` | vmherd | SSH user created on all VMs (passwordless sudo) |
| `password` | — | Optional password for the user (enables `virsh console` login) |

## Commands

```
vmherd init                    Create example Herdfile in current directory
vmherd up [name...]            Create and start VMs (all or named)
vmherd down [name...]          Stop VMs gracefully
vmherd reload [name...]        Destroy and recreate VMs (preserves network/keys)
vmherd destroy [name...]       Stop and remove VMs, disks, and state
vmherd provision [name...]     Rerun provisioning steps on running VMs
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

# Reload a VM after changing its Herdfile config
vmherd reload ctrl

# Rerun provisioning on running VMs
vmherd provision
```

### Errors and exit codes

Commands that act on several VMs (`up`, `down`, `restart`, `reload`, `destroy`) still handle every VM when one of them fails, then list each VM's error at the end and exit 1. `up` exits 1 when a VM could not be created or started, gets no DHCP lease, never answers SSH, fails a first-boot provisioning step, or its port forwarding fails. If libvirt itself can't be reached, every command stops right away and says so, instead of reporting VMs as "not created".

## Supported Images

| Image | Description |
|-------|-------------|
| `ubuntu:resolute` | Ubuntu 26.04 LTS |
| `ubuntu:noble` | Ubuntu 24.04 LTS |
| `ubuntu:jammy` | Ubuntu 22.04 LTS |
| `ubuntu:focal` | Ubuntu 20.04 LTS |
| `debian:bookworm` | Debian 12 |
| `debian:trixie` | Debian 13 |
| `fedora:45` | Fedora 45 |
| `fedora:44` | Fedora 44 |
| `rocky:9` | Rocky Linux 9 |
| `almalinux:9` | AlmaLinux 9 |
| `arch:latest` | Arch Linux |

All images are downloaded directly from official distro vendors and verified with SHA256 checksums. `vmherd init` writes the download URLs for `ubuntu:noble`, `ubuntu:resolute`, `fedora:44` and `fedora:45`; for the others, add the vendor's image URL under `images:`. If a vendor stops publishing a file (Fedora pins a build number in the file name), `vmherd up`/`pull` say so and point to the directory with the current file name.

## How It Works

1. **Image download** — Official cloud images are fetched from vendor CDNs and cached at `~/.vmherd/images/`. SHA256 checksums are verified against the vendor's published checksums.

2. **SSH key generation** — An ed25519 key pair is generated per project and stored in the project's state directory (see [State](#state)).

3. **Cloud-init** — For each VM, vmherd generates cloud-init user-data (SSH key injection, user creation, provisioning), network-config (static IP), and meta-data. These are packaged into a cidata ISO and attached to the VM — no external tools needed. On first boot the provisioning steps run in order; if one fails, the later steps are skipped (shared folders are still mounted). `vmherd up` and `vmherd reload` wait for this first-boot run to finish (up to 30 minutes) and exit 1 if a step failed, naming the step and showing the end of the guest's `/var/log/cloud-init-output.log`. VMs that were already provisioned are not checked again; rerun steps with `vmherd provision`.

4. **VM creation** — VMs are created by generating libvirt domain XML and defining them via `virsh`, with a qcow2 disk backed by the cached cloud image (copy-on-write, so disk creation is instant).

5. **Idempotent operations** — `vmherd up` creates what's missing, starts what's stopped, resumes what's paused or suspended, and leaves what's running. A crashed VM is forced off and started again.

## State

| What | Where |
|------|-------|
| Cloud images | `~/.vmherd/images/` |
| SSH keys, cloud-init files | `$XDG_STATE_HOME/vmherd/<project>-<hash>/` (default `~/.local/state/vmherd/`); `vmherd info state-dir` prints it, `VMHERD_STATE_DIR` overrides it. Projects that already have an in-project `.vmherd/` keep using it. |
| VM disks | `/var/lib/libvirt/images/vmherd-{project}-{hash}-{name}.qcow2` |
| Networks | libvirt networks prefixed `vmherd-` |

## License

AGPL-3.0. See [LICENSE](LICENSE).
