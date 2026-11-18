//go:build integration

package e2e

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	// missingShareProjectName is the temporary project of the missing-share
	// case; its domain prefix differs from the distro and scenario projects.
	missingShareProjectName = "e2e-missing-share"
	// missingShareSubnetOctet is the case's /24 (192.168.245.0/24): after the
	// distro table (230+) and the acceptance scenarios (240, 241).
	missingShareSubnetOctet = 245
	// missingShareFolderName is the share's host folder, which is never created.
	missingShareFolderName = "missing"
	// commandFailedExitCode is vmherd's exit code when a command fails.
	commandFailedExitCode = 1
	// projectSSHKeyFileName is the project key vmherd creates in its state dir.
	projectSSHKeyFileName = "id_ed25519"
	// missingShareImageName is the distroTable image the case's Herdfile names.
	missingShareImageName = "ubuntu:noble"
)

// missingShareHerdfileTemplate is one VM sharing a folder that doesn't exist.
// Placeholders: image name, URL, subnet octet, network name, share folder.
const missingShareHerdfileTemplate = `images:
  %[1]s: %[2]s
vms:
  - name: web
    image: %[1]s
    cpus: 1
    memory: 1024
    disk: 10G
    network:
      ip: 192.168.%[3]d.10
    shared_folders:
      - host: ./%[5]s
        guest: /srv/app
network:
  name: %[4]s
  subnet: 192.168.%[3]d.0/24
  dns: auto
ssh:
  user: vmherd
`

// TestUpRefusesMissingShareFolder checks that up fails, naming the missing
// share folder and how to create it, before it creates the SSH key, the
// network, a disk or a domain (M1.21). No image is downloaded.
func TestUpRefusesMissingShareFolder(t *testing.T) {
	ubuntuNoble := distroByImageName(t, missingShareImageName)
	project := e2eProject{
		directory:    filepath.Join(t.TempDir(), missingShareProjectName),
		stateDir:     filepath.Join(t.TempDir(), "state"),
		domainPrefix: "vmherd-" + missingShareProjectName + "-",
		networkName:  "vmherd-" + missingShareProjectName,
	}
	if err := os.MkdirAll(project.directory, 0o755); err != nil {
		t.Fatalf("creating project directory: %v", err)
	}
	herdfile := fmt.Sprintf(missingShareHerdfileTemplate, ubuntuNoble.imageName, ubuntuNoble.imageURL,
		missingShareSubnetOctet, project.networkName, missingShareFolderName)
	if err := os.WriteFile(filepath.Join(project.directory, "Herdfile"), []byte(herdfile), 0o644); err != nil {
		t.Fatalf("writing Herdfile: %v", err)
	}
	// Nothing should be created, but if up does create something (the bug),
	// the failure cleanup destroys it.
	cleanUpNowAndOnFailure(t, project)

	result := project.vmherd(t, longCommandTimeout, "up")
	if result.exitCode != commandFailedExitCode {
		t.Errorf("up exited %d, want %d", result.exitCode, commandFailedExitCode)
	}
	for _, wantFragment := range []string{"./" + missingShareFolderName, "mkdir -p"} {
		if !strings.Contains(result.output, wantFragment) {
			t.Errorf("up output should contain %q, got:\n%s", wantFragment, result.output)
		}
	}
	checkNothingCreated(t, project)
}

// distroByImageName returns the distroTable entry for imageName, or fails.
func distroByImageName(t *testing.T, imageName string) distroCase {
	t.Helper()
	for _, distro := range distroTable {
		if distro.imageName == imageName {
			return distro
		}
	}
	t.Fatalf("image %q is not in distroTable — known: %s", imageName, knownImageNames())
	return distroCase{}
}

// checkNothingCreated checks that the project has no domain, disk, seed ISO,
// libvirt network or SSH key.
func checkNothingCreated(t *testing.T, project e2eProject) {
	t.Helper()
	if domains := project.leftoverDomains(t); len(domains) > 0 {
		t.Errorf("up left domains: %v", domains)
	}
	if files := project.leftoverImageFiles(t); len(files) > 0 {
		t.Errorf("up left disks or seeds in %s: %v", libvirtImageDir, files)
	}
	if project.networkExists(t) {
		t.Errorf("up left network %s", project.networkName)
	}
	keyPath := filepath.Join(project.stateDir, projectSSHKeyFileName)
	if _, err := os.Stat(keyPath); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("up should not create the SSH key %s, stat: %v", keyPath, err)
	}
}
