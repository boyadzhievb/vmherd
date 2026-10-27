package image

import (
	"fmt"
	"path"
	"strings"
)

// Entry describes a cloud image resolved from the Herdfile images section.
type Entry struct {
	Distro      string // vendor name (e.g. ubuntu, debian)
	Codename    string // release codename or version (e.g. noble, 41)
	Description string // human-readable label for display
	ImageURL    string // HTTPS URL to the qcow2 cloud image
	ChecksumURL string // HTTPS URL to the vendor checksum file (empty = auto-discover)
	Filename    string // image filename used to match checksum entries
	OSVariant   string // virt-install os-variant hint
}

// Resolve looks up an image spec in the Herdfile images map and constructs an Entry.
func Resolve(imageSpec string, images map[string]string) (*Entry, error) {
	parts := strings.SplitN(imageSpec, ":", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("image must be 'distro:codename' format, got %q", imageSpec)
	}
	distro, codename := parts[0], parts[1]

	if len(images) == 0 {
		return nil, fmt.Errorf("no images defined — add an 'images:' section to your Herdfile:\nimages:\n  %s: <url>\n\nRun 'vmherd init' for an example", imageSpec)
	}

	imageURL, found := images[imageSpec]
	if !found {
		available := availableForDistro(distro, images)
		if len(available) > 0 {
			return nil, fmt.Errorf("image %q not defined in Herdfile images section — available %s: %s\n\nAdd it to your Herdfile:\nimages:\n  %s: <url>",
				imageSpec, distro, strings.Join(available, ", "), imageSpec)
		}
		return nil, fmt.Errorf("image %q not defined in Herdfile images section — add it:\nimages:\n  %s: <url>", imageSpec, imageSpec)
	}

	filename := path.Base(imageURL)
	osVariant := deriveOSVariant(distro, codename)
	description := buildDescription(distro, codename)

	return &Entry{
		Distro:      distro,
		Codename:    codename,
		Description: description,
		ImageURL:    imageURL,
		Filename:    filename,
		OSVariant:   osVariant,
	}, nil
}

// availableForDistro returns codenames defined in the images map for a given distro.
func availableForDistro(distro string, images map[string]string) []string {
	var codenames []string
	prefix := distro + ":"
	for spec := range images {
		if strings.HasPrefix(spec, prefix) {
			codenames = append(codenames, strings.TrimPrefix(spec, prefix))
		}
	}
	return codenames
}

// deriveOSVariant maps a distro:codename to a virt-install os-variant hint.
func deriveOSVariant(distro, codename string) string {
	knownVariants := map[string]string{
		"ubuntu:noble":    "ubuntu24.04",
		"ubuntu:jammy":    "ubuntu22.04",
		"ubuntu:focal":    "ubuntu20.04",
		"ubuntu:resolute": "ubuntu26.04",
		"ubuntu:questing": "ubuntu25.10",
		"ubuntu:stonking": "ubuntu26.10",
		"debian:bookworm": "debian12",
		"debian:trixie":   "debian13",
		"fedora:41":       "fedora41",
		"fedora:42":       "fedora42",
		"rocky:9":         "rocky9",
		"almalinux:9":     "almalinux9",
		"arch:latest":     "archlinux",
	}

	key := distro + ":" + codename
	if variant, found := knownVariants[key]; found {
		return variant
	}

	return "detect=on,require=off"
}

// buildDescription creates a human-readable label from distro and codename.
func buildDescription(distro, codename string) string {
	capitalDistro := strings.ToUpper(distro[:1]) + distro[1:]
	return fmt.Sprintf("%s %s", capitalDistro, codename)
}

// DefaultImages returns the image entries pre-populated in vmherd init.
// Only Ubuntu and Fedora are included — users add other distros themselves.
func DefaultImages() map[string]string {
	return map[string]string{
		"ubuntu:noble":    "https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-amd64.img",
		"ubuntu:resolute": "https://cloud-images.ubuntu.com/resolute/current/resolute-server-cloudimg-amd64.img",
		"fedora:42":       "https://download.fedoraproject.org/pub/fedora/linux/releases/42/Cloud/x86_64/images/Fedora-Cloud-Base-Generic-42-1.0.x86_64.qcow2",
	}
}

// ListAll returns all entries from the provided images map, sorted by key.
func ListAll(images map[string]string) []Entry {
	keys := make([]string, 0, len(images))
	for key := range images {
		keys = append(keys, key)
	}

	sortStrings(keys)

	entries := make([]Entry, 0, len(keys))
	for _, key := range keys {
		parts := strings.SplitN(key, ":", 2)
		if len(parts) != 2 {
			continue
		}
		entries = append(entries, Entry{
			Distro:      parts[0],
			Codename:    parts[1],
			Description: buildDescription(parts[0], parts[1]),
			ImageURL:    images[key],
			Filename:    path.Base(images[key]),
			OSVariant:   deriveOSVariant(parts[0], parts[1]),
		})
	}
	return entries
}

// sortStrings sorts a string slice in place using insertion sort (small lists).
func sortStrings(items []string) {
	for index := 1; index < len(items); index++ {
		current := items[index]
		position := index
		for position > 0 && items[position-1] > current {
			items[position] = items[position-1]
			position--
		}
		items[position] = current
	}
}
