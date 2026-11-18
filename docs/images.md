# Finding an image URL

vmherd has no image registry. Every image a VM uses is a `KEY: REAL_URL` entry under `images:` in the Herdfile, and vmherd downloads exactly that URL:

```yaml
images:
  ubuntu:noble: https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-amd64.img
```

The key is your name for the image (`distro:codename` form). The URL is the vendor's own download link. vmherd finds the vendor's checksum file in the same directory and checks the download against it, so take the URL from the vendor's image directory, not from a mirror list or a blog post.

## Which file to pick

Vendors publish several files per release. Pick the one that is:

- the **cloud** or **generic cloud** variant, which has cloud-init (not the variants for a specific cloud such as EC2, Azure or GCE),
- for **amd64** (also called x86_64),
- in **qcow2** format (Ubuntu's `.img` files are qcow2 too).

The URLs that vmherd has booted are in the [table at the end](#booted-urls).

## Ubuntu

- Directory: `https://cloud-images.ubuntu.com/<codename>/current/`, e.g. https://cloud-images.ubuntu.com/noble/current/
- File: `<codename>-server-cloudimg-amd64.img`
- The name stays the same for the life of the release; `current/` always holds the newest build.

## Debian

- Directory: `https://cloud.debian.org/images/cloud/<codename>/latest/`, e.g. https://cloud.debian.org/images/cloud/trixie/latest/
- File: `debian-<version>-genericcloud-amd64.qcow2`, e.g. `debian-13-genericcloud-amd64.qcow2`
- Trap: `genericcloud` runs Debian's trimmed cloud kernel, which has no 9p. Set `shared_folders: []` for these VMs, or `vmherd up` stops and tells you to.
- If you want the `/vagrant` share: `generic` (e.g. `debian-13-generic-amd64.qcow2`) runs Debian's full kernel, which has 9p. vmherd has not booted it yet.
- Don't pick `nocloud`: it has no cloud-init, so vmherd can't set up the VM.

## Fedora

- Directory: `https://download.fedoraproject.org/pub/fedora/linux/releases/<version>/Cloud/x86_64/images/`, e.g. https://download.fedoraproject.org/pub/fedora/linux/releases/45/Cloud/x86_64/images/
- File: `Fedora-Cloud-Base-Generic-<version>-<build>.x86_64.qcow2`, e.g. `Fedora-Cloud-Base-Generic-45-1.N.x86_64.qcow2`, where `1.N` is the build
- Trap: the file name pins the build number. When Fedora replaces the file, or moves an old release to its archive, the URL stops working. `vmherd up` and `vmherd pull` then say so and name the directory; open it and copy the current file name.
- Pick `Generic` … `.qcow2`. Not `UEFI-UKI` (UEFI-only), not the Generic `.raw.xz`, and not the AmazonEC2, Azure or GCE images.

## Arch Linux

- Directory: https://geo.mirror.pkgbuild.com/images/latest/
- File: `Arch-Linux-x86_64-cloudimg.qcow2`
- Don't pick `basic`: it has no cloud-init.

## Rocky Linux

- Directory: `https://dl.rockylinux.org/pub/rocky/<major>/images/x86_64/`, e.g. https://dl.rockylinux.org/pub/rocky/9/images/x86_64/
- File: `Rocky-<major>-GenericCloud-Base.latest.x86_64.qcow2`
- Don't pick the `EC2` images. `GenericCloud-LVM` is the same image with an LVM root; not booted by vmherd.
- Trap: Rocky's kernel has no 9p. Set `shared_folders: []` for these VMs.

## AlmaLinux

- Directory: `https://repo.almalinux.org/almalinux/<major>/cloud/x86_64/images/`, e.g. https://repo.almalinux.org/almalinux/9/cloud/x86_64/images/
- File: `AlmaLinux-<major>-GenericCloud-latest.x86_64.qcow2`
- Trap: AlmaLinux's kernel has no 9p. Set `shared_folders: []` for these VMs.

## Booted URLs

vmherd's tests boot these URLs. `vmherd init` writes the Ubuntu and Fedora ones. Before each release, `cmd/image-urls-check` checks every URL in this table and updates stale ones.

| Image | URL | Notes |
|-------|-----|-------|
| `ubuntu:noble` | https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-amd64.img | written by `vmherd init` |
| `ubuntu:resolute` | https://cloud-images.ubuntu.com/resolute/current/resolute-server-cloudimg-amd64.img | written by `vmherd init` |
| `fedora:44` | https://download.fedoraproject.org/pub/fedora/linux/releases/44/Cloud/x86_64/images/Fedora-Cloud-Base-Generic-44-1.7.x86_64.qcow2 | written by `vmherd init`; build number pinned |
| `fedora:45` | https://download.fedoraproject.org/pub/fedora/linux/releases/45/Cloud/x86_64/images/Fedora-Cloud-Base-Generic-45-1.4.x86_64.qcow2 | written by `vmherd init`; build number pinned |
| `debian:trixie` | https://cloud.debian.org/images/cloud/trixie/latest/debian-13-genericcloud-amd64.qcow2 | no 9p: set `shared_folders: []` |
| `rocky:9` | https://dl.rockylinux.org/pub/rocky/9/images/x86_64/Rocky-9-GenericCloud-Base.latest.x86_64.qcow2 | no 9p: set `shared_folders: []` |
| `almalinux:9` | https://repo.almalinux.org/almalinux/9/cloud/x86_64/images/AlmaLinux-9-GenericCloud-latest.x86_64.qcow2 | no 9p: set `shared_folders: []` |
| `arch:latest` | https://geo.mirror.pkgbuild.com/images/latest/Arch-Linux-x86_64-cloudimg.qcow2 | |
