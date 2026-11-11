//go:build integration

// Package e2e drives the vmherd binary the way a user does, once per distro,
// on a host with libvirt/KVM and passwordless sudo (the self-hosted runner).
package e2e

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/boyadzhievb/vmherd/internal/cloudinit"
)

const (
	// binaryOverrideVariable names a prebuilt vmherd binary to test instead of
	// building one (hosts whose Go is older than go.mod).
	binaryOverrideVariable = "VMHERD_E2E_BINARY"
	// imageSelectionVariable is a comma-separated list of image names to run
	// ("ubuntu:noble,rocky:9"); empty or "all" runs the whole table.
	imageSelectionVariable = "VMHERD_E2E_IMAGES"
	// allImagesSelection runs every distro in the table.
	allImagesSelection = "all"
	// longCommandTimeout bounds up, restart and reload: image download, boot
	// and first-boot provisioning of two VMs.
	longCommandTimeout = 25 * time.Minute
	// shortCommandTimeout bounds every other vmherd and helper command.
	shortCommandTimeout = 5 * time.Minute
	// libvirtImageDir holds VM disks and seed ISOs.
	libvirtImageDir = "/var/lib/libvirt/images"
	// firstSubnetOctet is the third octet of the first distro's subnet; each
	// distro gets its own /24 so leftovers of one never block another.
	firstSubnetOctet = 230
	// fixedVMName has a static IP, the default /vagrant share and one step.
	fixedVMName = "fixed"
	// dhcpVMName uses DHCP, has no shares, and a second step that fails.
	dhcpVMName = "dhcp"
	// provisionedMarkerPath is written by the fixed VM's provisioning step.
	provisionedMarkerPath = "/var/tmp/e2e-provisioned"
	// afterFailureMarkerPath would be written after the failing command if
	// shell steps did not stop at the first error (M1.16).
	afterFailureMarkerPath = "/var/tmp/e2e-after-failure"
)

// distroCase is one image in the table and what to expect from it.
type distroCase struct {
	imageName     string // Herdfile image name, distro:codename
	imageURL      string // vendor download URL
	hasNinePShare bool   // false for kernels built without 9p (RHEL family, Debian cloud)
}

// distroTable is every image the e2e test boots. Order fixes each distro's
// subnet, so append new entries at the end.
var distroTable = []distroCase{
	{"ubuntu:noble", "https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-amd64.img", true},
	{"ubuntu:resolute", "https://cloud-images.ubuntu.com/resolute/current/resolute-server-cloudimg-amd64.img", true},
	{"fedora:44", "https://download.fedoraproject.org/pub/fedora/linux/releases/44/Cloud/x86_64/images/Fedora-Cloud-Base-Generic-44-1.7.x86_64.qcow2", true},
	{"fedora:45", "https://download.fedoraproject.org/pub/fedora/linux/releases/45/Cloud/x86_64/images/Fedora-Cloud-Base-Generic-45-1.4.x86_64.qcow2", true},
	// Debian's genericcloud image runs the trimmed cloud kernel, without 9p.
	{"debian:trixie", "https://cloud.debian.org/images/cloud/trixie/latest/debian-13-genericcloud-amd64.qcow2", false},
	{"rocky:9", "https://dl.rockylinux.org/pub/rocky/9/images/x86_64/Rocky-9-GenericCloud-Base.latest.x86_64.qcow2", false},
	{"almalinux:9", "https://repo.almalinux.org/almalinux/9/cloud/x86_64/images/AlmaLinux-9-GenericCloud-latest.x86_64.qcow2", false},
	{"arch:latest", "https://geo.mirror.pkgbuild.com/images/latest/Arch-Linux-x86_64-cloudimg.qcow2", true},
}

// herdfileTemplate is the per-distro project. Placeholders: image name, URL,
// subnet octet (fixed IP, network), network name. The dhcp VM's second step
// fails at its first command, so up and provision must report it.
const herdfileTemplate = `images:
  %[1]s: %[2]s
vms:
  - name: fixed
    image: %[1]s
    cpus: 1
    memory: 1024
    disk: 10G
    network:
      ip: 192.168.%[3]d.10
    provision:
      - type: shell
        inline: echo provisioned > /var/tmp/e2e-provisioned
  - name: dhcp
    image: %[1]s
    cpus: 1
    memory: 1024
    disk: 10G
    shared_folders: []
    provision:
      - type: shell
        inline: echo first step
      - type: shell
        inline: |
          false
          echo should-not-run > /var/tmp/e2e-after-failure
network:
  name: %[4]s
  subnet: 192.168.%[3]d.0/24
  dns: auto
ssh:
  user: vmherd
`

// vmherdBinaryPath is set once by TestMain and only read afterwards.
var vmherdBinaryPath string

// TestMain builds the vmherd binary (or takes VMHERD_E2E_BINARY) before the
// distro subtests run.
func TestMain(m *testing.M) {
	binaryPath, cleanupBuild, err := prepareVMHerdBinary()
	if err != nil {
		fmt.Fprintf(os.Stderr, "e2e: %v\n", err)
		os.Exit(1)
	}
	vmherdBinaryPath = binaryPath
	exitCode := m.Run()
	cleanupBuild()
	os.Exit(exitCode)
}

// prepareVMHerdBinary returns the binary to test and a function that removes
// it when it was built here.
func prepareVMHerdBinary() (string, func(), error) {
	if overridePath := os.Getenv(binaryOverrideVariable); overridePath != "" {
		return overridePath, func() {}, nil
	}
	buildDir, err := os.MkdirTemp("", "vmherd-e2e-bin-")
	if err != nil {
		return "", nil, fmt.Errorf("creating build directory: %w", err)
	}
	binaryPath := filepath.Join(buildDir, "vmherd")
	buildCommand := exec.Command("go", "build", "-o", binaryPath, "../../cmd/vmherd")
	buildCommand.Stdout, buildCommand.Stderr = os.Stdout, os.Stderr
	if err := buildCommand.Run(); err != nil {
		return "", nil, fmt.Errorf("building vmherd: %w — set %s to a prebuilt binary to skip the build", err, binaryOverrideVariable)
	}
	return binaryPath, func() {
		if err := os.RemoveAll(buildDir); err != nil {
			fmt.Fprintf(os.Stderr, "e2e: removing %s: %v\n", buildDir, err)
		}
	}, nil
}

// selectedDistros returns the table entries named in VMHERD_E2E_IMAGES, with
// each entry's subnet octet, or fails on an unknown name.
func selectedDistros(t *testing.T) map[string]int {
	selection := strings.TrimSpace(os.Getenv(imageSelectionVariable))
	octetByImage := make(map[string]int, len(distroTable))
	for index, distro := range distroTable {
		octetByImage[distro.imageName] = firstSubnetOctet + index
	}
	if selection == "" || selection == allImagesSelection {
		return octetByImage
	}
	selected := make(map[string]int)
	for _, imageName := range strings.Split(selection, ",") {
		imageName = strings.TrimSpace(imageName)
		octet, known := octetByImage[imageName]
		if !known {
			t.Fatalf("%s names unknown image %q — known: %s", imageSelectionVariable, imageName, knownImageNames())
		}
		selected[imageName] = octet
	}
	return selected
}

// knownImageNames lists the table's image names for error messages.
func knownImageNames() string {
	names := make([]string, 0, len(distroTable))
	for _, distro := range distroTable {
		names = append(names, distro.imageName)
	}
	return strings.Join(names, ", ")
}

// TestDistroLifecycle runs the full CLI lifecycle for every selected distro,
// one distro at a time (two VMs each).
func TestDistroLifecycle(t *testing.T) {
	selected := selectedDistros(t)
	for _, distro := range distroTable {
		subnetOctet, isSelected := selected[distro.imageName]
		if !isSelected {
			continue
		}
		t.Run(strings.ReplaceAll(distro.imageName, ":", "-"), func(t *testing.T) {
			runDistroLifecycle(t, distro, subnetOctet)
		})
	}
}

// e2eProject is one distro's temporary project and how to reach its VMs.
type e2eProject struct {
	directory    string // project directory holding the Herdfile
	stateDir     string // VMHERD_STATE_DIR for every command
	domainPrefix string // libvirt domain and disk name prefix for this project
	networkName  string // libvirt network name
	fixedIP      string // static address of the fixed VM
}

// newE2EProject writes the distro's Herdfile into a fresh project directory
// and removes leftovers of an earlier, aborted run of the same distro.
func newE2EProject(t *testing.T, distro distroCase, subnetOctet int) e2eProject {
	slug := strings.ReplaceAll(strings.ReplaceAll(distro.imageName, ":", "-"), ".", "-")
	projectName := "e2e-" + slug
	project := e2eProject{
		directory:    filepath.Join(t.TempDir(), projectName),
		stateDir:     filepath.Join(t.TempDir(), "state"),
		domainPrefix: "vmherd-" + projectName + "-",
		networkName:  "vmherd-" + projectName,
		fixedIP:      fmt.Sprintf("192.168.%d.10", subnetOctet),
	}
	if err := os.MkdirAll(project.directory, 0o755); err != nil {
		t.Fatalf("creating project directory: %v", err)
	}
	herdfile := fmt.Sprintf(herdfileTemplate, distro.imageName, distro.imageURL, subnetOctet, project.networkName)
	if err := os.WriteFile(filepath.Join(project.directory, "Herdfile"), []byte(herdfile), 0o644); err != nil {
		t.Fatalf("writing Herdfile: %v", err)
	}
	removeLeftovers(t, project)
	t.Cleanup(func() {
		if t.Failed() {
			if result := project.vmherd(t, shortCommandTimeout, "destroy"); result.exitCode != 0 {
				t.Logf("cleanup: vmherd destroy exited %d:\n%s", result.exitCode, result.output)
			}
			removeLeftovers(t, project)
		}
	})
	return project
}

// runDistroLifecycle is the scenario for one distro; each phase is a helper
// so a failure names the phase.
func runDistroLifecycle(t *testing.T, distro distroCase, subnetOctet int) {
	project := newE2EProject(t, distro, subnetOctet)

	checkFirstUp(t, project, distro)
	checkStatusShowsAddresses(t, project, subnetOctet)
	checkProvisioningResults(t, project)
	if distro.hasNinePShare {
		checkShareBothWays(t, project)
	}
	checkProvisionCommand(t, project)
	checkRestartDownUp(t, project)
	checkDestroyLeavesNothing(t, project)
}

// checkFirstUp creates both VMs. up must exit 1 because the dhcp VM's second
// step fails and name that step; the fixed VM must succeed where the kernel
// has 9p, and up must name the missing 9p where it doesn't.
func checkFirstUp(t *testing.T, project e2eProject, distro distroCase) {
	result := project.vmherd(t, longCommandTimeout, "up")
	if result.exitCode != 1 {
		t.Fatalf("first up: want exit 1 (dhcp step 2 fails), got %d:\n%s", result.exitCode, result.output)
	}
	wantStepLine := cloudinit.FailedStepMarker + " 2 of 2"
	if !strings.Contains(result.output, wantStepLine) {
		t.Errorf("first up: output does not name the failed step (%q):\n%s", wantStepLine, result.output)
	}
	if !distro.hasNinePShare && !strings.Contains(result.output, "no 9p support") {
		t.Errorf("first up: output does not say the kernel has no 9p support:\n%s", result.output)
	}
	if distro.hasNinePShare && strings.Contains(result.output, "no 9p support") {
		t.Errorf("first up: unexpected missing-9p report:\n%s", result.output)
	}
	fixedFailure := fixedVMName + ": first-boot provisioning failed"
	if distro.hasNinePShare && strings.Contains(result.output, fixedFailure) {
		t.Errorf("first up: the fixed VM's first boot should succeed:\n%s", result.output)
	}
}

// checkStatusShowsAddresses checks both VMs run, the fixed VM has its static
// address and the dhcp VM got a lease inside the project subnet.
func checkStatusShowsAddresses(t *testing.T, project e2eProject, subnetOctet int) {
	output := project.mustVMHerd(t, shortCommandTimeout, "status")
	rows := statusRows(output)
	if row := rows[fixedVMName]; !strings.Contains(row, project.fixedIP) || !strings.Contains(row, "running") {
		t.Errorf("status: fixed row should show %s and running, got %q", project.fixedIP, row)
	}
	subnetPrefix := fmt.Sprintf("192.168.%d.", subnetOctet)
	if row := rows[dhcpVMName]; !strings.Contains(row, subnetPrefix) || !strings.Contains(row, "running") {
		t.Errorf("status: dhcp row should show a %sx lease and running, got %q", subnetPrefix, row)
	}
}

// statusRows maps each VM name to its line of 'vmherd status' output.
func statusRows(statusOutput string) map[string]string {
	rows := make(map[string]string)
	for _, line := range strings.Split(statusOutput, "\n") {
		fields := strings.Fields(line)
		if len(fields) > 0 {
			rows[fields[0]] = line
		}
	}
	return rows
}

// checkProvisioningResults checks the fixed VM ran its step over SSH and the
// dhcp VM's failing step stopped at its first command.
func checkProvisioningResults(t *testing.T, project e2eProject) {
	provisioned := project.mustVMHerd(t, shortCommandTimeout, "ssh", fixedVMName, "--", "cat", provisionedMarkerPath)
	if strings.TrimSpace(provisioned) != "provisioned" {
		t.Errorf("fixed VM: %s should contain 'provisioned', got %q", provisionedMarkerPath, provisioned)
	}
	project.mustVMHerd(t, shortCommandTimeout, "ssh", dhcpVMName, "--", "test", "!", "-e", afterFailureMarkerPath)
}

// checkShareBothWays checks the /vagrant share: a host file is visible in the
// guest, and a guest file appears on the host owned by the invoking user.
func checkShareBothWays(t *testing.T, project e2eProject) {
	hostFile := filepath.Join(project.directory, "from-host.txt")
	if err := os.WriteFile(hostFile, []byte("host-content\n"), 0o644); err != nil {
		t.Fatalf("writing %s: %v", hostFile, err)
	}
	seenInGuest := project.mustVMHerd(t, shortCommandTimeout, "ssh", fixedVMName, "--", "cat", "/vagrant/from-host.txt")
	if strings.TrimSpace(seenInGuest) != "host-content" {
		t.Errorf("share: guest read %q from /vagrant/from-host.txt", seenInGuest)
	}
	// One argument is sent as a raw command line, so the redirect runs in the guest.
	project.mustVMHerd(t, shortCommandTimeout, "ssh", fixedVMName, "--", "echo guest-content > /vagrant/from-guest.txt")
	guestFile := filepath.Join(project.directory, "from-guest.txt")
	content, err := os.ReadFile(guestFile)
	if err != nil {
		t.Fatalf("share: reading guest-written file on the host: %v", err)
	}
	if strings.TrimSpace(string(content)) != "guest-content" {
		t.Errorf("share: host read %q from from-guest.txt", content)
	}
	fileInfo, err := os.Stat(guestFile)
	if err != nil {
		t.Fatalf("share: %v", err)
	}
	if ownerUID := fileInfo.Sys().(*syscall.Stat_t).Uid; int(ownerUID) != os.Getuid() {
		t.Errorf("share: guest-written file is owned by UID %d, want the host user %d", ownerUID, os.Getuid())
	}
}

// checkProvisionCommand reruns provisioning: the fixed VM succeeds, the dhcp
// VM fails and the error names step 2.
func checkProvisionCommand(t *testing.T, project e2eProject) {
	project.mustVMHerd(t, shortCommandTimeout, "provision", fixedVMName)
	result := project.vmherd(t, shortCommandTimeout, "provision", dhcpVMName)
	if result.exitCode != 1 {
		t.Errorf("provision dhcp: want exit 1, got %d:\n%s", result.exitCode, result.output)
	}
	if !strings.Contains(result.output, "shell provisioning step 2 failed") {
		t.Errorf("provision dhcp: output does not name step 2:\n%s", result.output)
	}
}

// checkRestartDownUp checks restart, down (both VMs shut off) and up again,
// with the fixed VM's disk contents kept throughout.
func checkRestartDownUp(t *testing.T, project e2eProject) {
	project.mustVMHerdLong(t, "restart")
	project.mustVMHerd(t, shortCommandTimeout, "ssh", fixedVMName, "--", "test", "-e", provisionedMarkerPath)

	project.mustVMHerd(t, shortCommandTimeout, "down")
	rows := statusRows(project.mustVMHerd(t, shortCommandTimeout, "status"))
	for _, vmName := range []string{fixedVMName, dhcpVMName} {
		if !strings.Contains(rows[vmName], "shut off") {
			t.Errorf("after down: %s should be shut off, got %q", vmName, rows[vmName])
		}
	}

	project.mustVMHerdLong(t, "up")
	project.mustVMHerd(t, shortCommandTimeout, "ssh", fixedVMName, "--", "test", "-e", provisionedMarkerPath)
}

// checkDestroyLeavesNothing destroys the project and checks that no domain,
// disk, seed ISO or network of it remains.
func checkDestroyLeavesNothing(t *testing.T, project e2eProject) {
	project.mustVMHerd(t, shortCommandTimeout, "destroy")
	if domains := project.leftoverDomains(t); len(domains) > 0 {
		t.Errorf("destroy left domains: %v", domains)
	}
	if files := project.leftoverImageFiles(t); len(files) > 0 {
		t.Errorf("destroy left disks or seeds in %s: %v", libvirtImageDir, files)
	}
	if project.networkExists(t) {
		t.Errorf("destroy left network %s", project.networkName)
	}
}

// commandResult is a finished command's combined output and exit code.
type commandResult struct {
	output   string // stdout and stderr interleaved
	exitCode int    // process exit code
}

// vmherd runs the binary in the project directory with the project's state
// dir and returns its output and exit code; it fails only if it can't start.
func (project e2eProject) vmherd(t *testing.T, timeout time.Duration, args ...string) commandResult {
	t.Helper()
	commandContext, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	command := exec.CommandContext(commandContext, vmherdBinaryPath, args...)
	command.Dir = project.directory
	command.Env = append(os.Environ(), "VMHERD_STATE_DIR="+project.stateDir)
	var output bytes.Buffer
	command.Stdout, command.Stderr = &output, &output
	runErr := command.Run()
	t.Logf("vmherd %s:\n%s", strings.Join(args, " "), output.String())
	if commandContext.Err() != nil {
		t.Fatalf("vmherd %s did not finish within %s", strings.Join(args, " "), timeout)
	}
	if exitErr, isExitError := runErr.(*exec.ExitError); isExitError {
		return commandResult{output: output.String(), exitCode: exitErr.ExitCode()}
	}
	if runErr != nil {
		t.Fatalf("running vmherd %s: %v", strings.Join(args, " "), runErr)
	}
	return commandResult{output: output.String()}
}

// mustVMHerd runs vmherd and fails the test unless it exits 0.
func (project e2eProject) mustVMHerd(t *testing.T, timeout time.Duration, args ...string) string {
	t.Helper()
	result := project.vmherd(t, timeout, args...)
	if result.exitCode != 0 {
		t.Fatalf("vmherd %s: exit %d:\n%s", strings.Join(args, " "), result.exitCode, result.output)
	}
	return result.output
}

// mustVMHerdLong is mustVMHerd with the timeout for commands that boot VMs.
func (project e2eProject) mustVMHerdLong(t *testing.T, args ...string) string {
	t.Helper()
	return project.mustVMHerd(t, longCommandTimeout, args...)
}

// sudoOutput runs a command with sudo and returns its trimmed stdout.
func sudoOutput(t *testing.T, args ...string) string {
	t.Helper()
	commandContext, cancel := context.WithTimeout(context.Background(), shortCommandTimeout)
	defer cancel()
	output, err := exec.CommandContext(commandContext, "sudo", args...).Output()
	if err != nil {
		t.Fatalf("sudo %s: %v", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(output))
}

// leftoverDomains lists libvirt domains whose name starts with the project prefix.
func (project e2eProject) leftoverDomains(t *testing.T) []string {
	return linesWithPrefix(sudoOutput(t, "virsh", "list", "--all", "--name"), project.domainPrefix)
}

// leftoverImageFiles lists disks and seed ISOs whose name starts with the project prefix.
func (project e2eProject) leftoverImageFiles(t *testing.T) []string {
	return linesWithPrefix(sudoOutput(t, "ls", "-1", libvirtImageDir), project.domainPrefix)
}

// networkExists reports whether the project's libvirt network is defined.
func (project e2eProject) networkExists(t *testing.T) bool {
	for _, networkName := range strings.Split(sudoOutput(t, "virsh", "net-list", "--all", "--name"), "\n") {
		if strings.TrimSpace(networkName) == project.networkName {
			return true
		}
	}
	return false
}

// linesWithPrefix returns the trimmed lines of text that start with prefix.
func linesWithPrefix(text, prefix string) []string {
	var matches []string
	for _, line := range strings.Split(text, "\n") {
		if line = strings.TrimSpace(line); strings.HasPrefix(line, prefix) {
			matches = append(matches, line)
		}
	}
	return matches
}

// removeLeftovers force-removes this project's domains, disks, seeds and
// network from an earlier aborted run. Names are matched by the project
// prefix, which only e2e projects use.
func removeLeftovers(t *testing.T, project e2eProject) {
	t.Helper()
	for _, domainName := range project.leftoverDomains(t) {
		runSudoLogged(t, "virsh", "destroy", domainName)
		runSudoLogged(t, "virsh", "undefine", domainName, "--nvram")
	}
	for _, fileName := range project.leftoverImageFiles(t) {
		runSudoLogged(t, "rm", "-f", filepath.Join(libvirtImageDir, fileName))
	}
	if project.networkExists(t) {
		runSudoLogged(t, "virsh", "net-destroy", project.networkName)
		runSudoLogged(t, "virsh", "net-undefine", project.networkName)
	}
}

// runSudoLogged runs a cleanup command with sudo and logs a failure instead
// of failing the test (a domain may already be shut off, for example).
func runSudoLogged(t *testing.T, args ...string) {
	t.Helper()
	commandContext, cancel := context.WithTimeout(context.Background(), shortCommandTimeout)
	defer cancel()
	if output, err := exec.CommandContext(commandContext, "sudo", args...).CombinedOutput(); err != nil {
		t.Logf("cleanup: sudo %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	} else {
		t.Logf("cleanup: sudo %s", strings.Join(args, " "))
	}
}
