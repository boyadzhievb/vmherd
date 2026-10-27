package image

import (
	"fmt"
	"strings"
)

// Entry describes a cloud image available in the built-in registry.
type Entry struct {
	Distro      string // vendor name (e.g. ubuntu, debian)
	Codename    string // release codename or version (e.g. noble, 41)
	Description string // human-readable label for display
	ImageURL    string // HTTPS URL to the qcow2 cloud image
	ChecksumURL string // HTTPS URL to the vendor checksum file
	Filename    string // image filename used to match checksum entries
	OSVariant   string // virt-install os-variant hint
}

// Resolve looks up a distro:codename image spec in the built-in registry.
func Resolve(imageSpec string) (*Entry, error) {
	parts := strings.SplitN(imageSpec, ":", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("image must be 'distro:codename' format, got %q", imageSpec)
	}
	distro, codename := parts[0], parts[1]
	key := distro + ":" + codename

	entry, ok := registry[key]
	if !ok {
		available := availableForDistro(distro)
		if len(available) > 0 {
			return nil, fmt.Errorf("unknown image %q — available %s versions: %s\n\nRun 'vmherd list-images' to see all options",
				imageSpec, distro, strings.Join(available, ", "))
		}
		return nil, fmt.Errorf("unknown distro %q — run 'vmherd list-images' to see available images", distro)
	}
	return &entry, nil
}

// ListAll returns all entries in the built-in registry, ordered by distro.
func ListAll() []Entry {
	return registryOrdered
}

func availableForDistro(distro string) []string {
	var codenames []string
	for _, entry := range registryOrdered {
		if entry.Distro == distro {
			codenames = append(codenames, entry.Codename)
		}
	}
	return codenames
}

var registry = map[string]Entry{}

var registryOrdered = []Entry{
	{
		Distro:      "ubuntu",
		Codename:    "noble",
		Description: "Ubuntu 24.04 LTS (Noble Numbat)",
		ImageURL:    "https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-amd64.img",
		Filename:    "noble-server-cloudimg-amd64.img",
		OSVariant:   "ubuntu24.04",
	},
	{
		Distro:      "ubuntu",
		Codename:    "jammy",
		Description: "Ubuntu 22.04 LTS (Jammy Jellyfish)",
		ImageURL:    "https://cloud-images.ubuntu.com/jammy/current/jammy-server-cloudimg-amd64.img",
		Filename:    "jammy-server-cloudimg-amd64.img",
		OSVariant:   "ubuntu22.04",
	},
	{
		Distro:      "ubuntu",
		Codename:    "focal",
		Description: "Ubuntu 20.04 LTS (Focal Fossa)",
		ImageURL:    "https://cloud-images.ubuntu.com/focal/current/focal-server-cloudimg-amd64.img",
		Filename:    "focal-server-cloudimg-amd64.img",
		OSVariant:   "ubuntu20.04",
	},
	{
		Distro:      "debian",
		Codename:    "bookworm",
		Description: "Debian 12 (Bookworm)",
		ImageURL:    "https://cloud.debian.org/images/cloud/bookworm/latest/debian-12-generic-amd64.qcow2",
		Filename:    "debian-12-generic-amd64.qcow2",
		OSVariant:   "debian12",
	},
	{
		Distro:      "debian",
		Codename:    "trixie",
		Description: "Debian 13 (Trixie)",
		ImageURL:    "https://cloud.debian.org/images/cloud/trixie/latest/debian-13-generic-amd64.qcow2",
		Filename:    "debian-13-generic-amd64.qcow2",
		OSVariant:   "debian13",
	},
	{
		Distro:      "fedora",
		Codename:    "41",
		Description: "Fedora 41 Cloud",
		ImageURL:    "https://download.fedoraproject.org/pub/fedora/linux/releases/41/Cloud/x86_64/images/Fedora-Cloud-Base-Generic-41-1.4.x86_64.qcow2",
		Filename:    "Fedora-Cloud-Base-Generic-41-1.4.x86_64.qcow2",
		OSVariant:   "fedora41",
	},
	{
		Distro:      "rocky",
		Codename:    "9",
		Description: "Rocky Linux 9",
		ImageURL:    "https://dl.rockylinux.org/pub/rocky/9/images/x86_64/Rocky-9-GenericCloud-Base.latest.x86_64.qcow2",
		Filename:    "Rocky-9-GenericCloud-Base.latest.x86_64.qcow2",
		OSVariant:   "rocky9",
	},
	{
		Distro:      "almalinux",
		Codename:    "9",
		Description: "AlmaLinux 9",
		ImageURL:    "https://repo.almalinux.org/almalinux/9/cloud/x86_64/images/AlmaLinux-9-GenericCloud-latest.x86_64.qcow2",
		Filename:    "AlmaLinux-9-GenericCloud-latest.x86_64.qcow2",
		OSVariant:   "almalinux9",
	},
	{
		Distro:      "arch",
		Codename:    "latest",
		Description: "Arch Linux (latest)",
		ImageURL:    "https://geo.mirror.pkgbuild.com/images/latest/Arch-Linux-x86_64-cloudimg.qcow2",
		ChecksumURL: "https://geo.mirror.pkgbuild.com/images/latest/Arch-Linux-x86_64-cloudimg.qcow2.SHA256",
		Filename:    "Arch-Linux-x86_64-cloudimg.qcow2",
		OSVariant:   "archlinux",
	},
}

func init() {
	for _, entry := range registryOrdered {
		registry[entry.Distro+":"+entry.Codename] = entry
	}
}
