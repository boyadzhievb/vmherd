package config

// BuiltInImage is one cloud image vmherd knows without a Herdfile: what
// 'vmherd init' writes into the images section, and what 'pull' and
// 'list-images' use when there is no Herdfile.
type BuiltInImage struct {
	Name string // image name in distro:codename form, e.g. ubuntu:noble
	URL  string // vendor download URL (https); pinned file names stay valid for the release's lifetime
}

// BuiltInImages returns the built-in images in the order 'vmherd init' writes
// them. Fedora release images keep their exact file name for the life of the
// release; check the release's Cloud/x86_64/images/ directory when adding one.
func BuiltInImages() []BuiltInImage {
	return []BuiltInImage{
		{Name: "ubuntu:noble", URL: "https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-amd64.img"},
		{Name: "ubuntu:resolute", URL: "https://cloud-images.ubuntu.com/resolute/current/resolute-server-cloudimg-amd64.img"},
		{Name: "fedora:44", URL: "https://download.fedoraproject.org/pub/fedora/linux/releases/44/Cloud/x86_64/images/Fedora-Cloud-Base-Generic-44-1.7.x86_64.qcow2"},
		{Name: "fedora:45", URL: "https://download.fedoraproject.org/pub/fedora/linux/releases/45/Cloud/x86_64/images/Fedora-Cloud-Base-Generic-45-1.4.x86_64.qcow2"},
	}
}

// BuiltInImageURLs returns the built-in images as a Herdfile-style images map
// (name → URL).
func BuiltInImageURLs() map[string]string {
	builtInImages := BuiltInImages()
	imageURLs := make(map[string]string, len(builtInImages))
	for _, builtInImage := range builtInImages {
		imageURLs[builtInImage.Name] = builtInImage.URL
	}
	return imageURLs
}
