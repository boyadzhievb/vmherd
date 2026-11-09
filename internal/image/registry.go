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
	description := buildDescription(distro, codename)

	return &Entry{
		Distro:      distro,
		Codename:    codename,
		Description: description,
		ImageURL:    imageURL,
		Filename:    filename,
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

// buildDescription creates a human-readable label from distro and codename.
func buildDescription(distro, codename string) string {
	capitalDistro := strings.ToUpper(distro[:1]) + distro[1:]
	return fmt.Sprintf("%s %s", capitalDistro, codename)
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
