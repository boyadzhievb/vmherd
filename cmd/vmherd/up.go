package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/boyadzhievb/vmherd/internal/cloudinit"
	"github.com/boyadzhievb/vmherd/internal/config"
	"github.com/boyadzhievb/vmherd/internal/image"
	"github.com/boyadzhievb/vmherd/internal/network"
	"github.com/boyadzhievb/vmherd/internal/ssh"
	"github.com/boyadzhievb/vmherd/internal/ui"
	"github.com/boyadzhievb/vmherd/internal/vm"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

// maxParallelVMCreations limits how many VMs are created concurrently.
const maxParallelVMCreations = 4

// vmCreateResult tracks the outcome of a single VM creation attempt.
type vmCreateResult struct {
	vmConfig config.VM
	err      error
}

// newUpCmd creates the 'up' command that creates and starts VMs.
func newUpCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "up [name...]",
		Short: "Create and start VMs (all or named)",
		RunE: func(cmd *cobra.Command, args []string) error {
			project, err := loadProjectWithDomains()
			if err != nil {
				return err
			}

			if project.Herdfile.SSH.Password != "" {
				ui.Println("Warning: Herdfile contains a plaintext password — avoid committing this file to version control")
			}

			vms, err := config.FilterVMs(project.Herdfile, args)
			if err != nil {
				return err
			}

			subnetBase, err := project.SubnetBase()
			if err != nil {
				return err
			}

			gatewayIP, err := project.GatewayIP()
			if err != nil {
				return err
			}

			dnsServer := vm.ResolveDNS(project.Herdfile.Network.DNS, gatewayIP)

			keyPath, err := ssh.EnsureKeyPair(project.StateDir, "vmherd-"+project.Name)
			if err != nil {
				return err
			}

			pubKey, err := ssh.ReadPublicKey(keyPath)
			if err != nil {
				return err
			}

			if err := network.Ensure(project.StateDir, project.Herdfile.Network.Name, subnetBase); err != nil {
				return err
			}

			activeVMs, bringUpErr, err := bringUpVMs(project, vms, strings.TrimSpace(pubKey), subnetBase, dnsServer)
			if err != nil {
				return err
			}

			readyVMs, readyErr := waitForVMsReady(project, activeVMs, keyPath)
			printReadyVMs(readyVMs, keyPath, project.Herdfile.SSH.User, len(vms))
			return incompleteCommandError("up", errors.Join(bringUpErr, readyErr))
		},
	}
}

// bringUpVMs starts or resumes existing VMs, then creates missing VMs in
// parallel. Every VM is attempted: it returns the VMs that are now booted and
// the joined errors of those that failed. err is set only for failures that
// stop all VMs (libvirt unreachable, image download).
func bringUpVMs(project *config.Project, vms []config.VM, pubKey, subnetBase, dnsServer string) (activeVMs []config.VM, vmFailures error, err error) {
	var vmsToCreate []config.VM
	var failures []error

	for _, vmConfig := range vms {
		domainName := project.DomainName(vmConfig.Name)
		state, err := vm.State(domainName)
		if err != nil {
			return nil, nil, err
		}

		if state == vm.StateNotCreated {
			vmsToCreate = append(vmsToCreate, vmConfig)
			continue
		}
		if state.IsRunning() {
			ui.Printf("%s is already running.\n", vmConfig.Name)
		} else {
			ui.Println(startExistingVMMessage(vmConfig.Name, state))
			if err := vm.EnsureRunning(domainName, state); err != nil {
				failures = append(failures, fmt.Errorf("%s: %w", vmConfig.Name, err))
				continue
			}
		}
		activeVMs = append(activeVMs, vmConfig)
	}

	if len(vmsToCreate) == 0 {
		return activeVMs, errors.Join(failures...), nil
	}

	resolvedImages, err := resolveAndCacheImages(vmsToCreate, project.Herdfile.Images)
	if err != nil {
		return nil, nil, err
	}

	results := createVMsInParallel(project, vmsToCreate, resolvedImages, pubKey, subnetBase, dnsServer)
	for _, result := range results {
		if result.err != nil {
			ui.Printf("%s: creation failed (error below).\n", result.vmConfig.Name)
			failures = append(failures, fmt.Errorf("%s: creating VM: %w", result.vmConfig.Name, result.err))
			continue
		}
		activeVMs = append(activeVMs, result.vmConfig)
	}
	return activeVMs, errors.Join(failures...), nil
}

// printReadyVMs lists the VMs that answered SSH with their ssh command line,
// saying how many of the requested VMs that is when some are missing.
func printReadyVMs(readyVMs []config.VM, keyPath, sshUser string, requestedVMCount int) {
	if len(readyVMs) == 0 {
		return
	}
	if len(readyVMs) == requestedVMCount {
		ui.Println("\nAll VMs ready:")
	} else {
		ui.Printf("\n%d of %d VMs ready:\n", len(readyVMs), requestedVMCount)
	}
	for _, vmConfig := range readyVMs {
		ui.Printf("  %-20s %s   ssh -i %s %s@%s\n",
			vmConfig.Name, vmConfig.Network.IP, keyPath, sshUser, vmConfig.Network.IP)
	}
}

// startExistingVMMessage says what 'up' does to bring an existing, not
// running VM in state back up.
func startExistingVMMessage(vmName string, state vm.VMState) string {
	switch state {
	case vm.StatePaused:
		return fmt.Sprintf("Resuming paused %s...", vmName)
	case vm.StatePMSuspended:
		return fmt.Sprintf("Waking suspended %s...", vmName)
	case vm.StateShuttingDown, vm.StateDying:
		return fmt.Sprintf("%s is shutting down; waiting for it to power off, then starting it...", vmName)
	case vm.StateCrashed:
		return fmt.Sprintf("Warning: %s crashed; forcing it off and starting it again...", vmName)
	default:
		return fmt.Sprintf("Starting %s...", vmName)
	}
}

// resolvedImage holds a pre-resolved and pre-downloaded image entry.
type resolvedImage struct {
	entry     *image.Entry
	cachePath string
}

// resolveAndCacheImages downloads all unique images needed by the VMs to create.
// Runs serially to avoid concurrent download races on the same image file.
func resolveAndCacheImages(vmsToCreate []config.VM, images map[string]string) (map[string]resolvedImage, error) {
	resolved := make(map[string]resolvedImage)
	for _, vmConfig := range vmsToCreate {
		if _, ok := resolved[vmConfig.Image]; ok {
			continue
		}
		entry, err := image.Resolve(vmConfig.Image, images)
		if err != nil {
			return nil, err
		}
		cachePath, err := image.EnsureCached(entry)
		if err != nil {
			return nil, err
		}
		resolved[vmConfig.Image] = resolvedImage{entry: entry, cachePath: cachePath}
	}
	return resolved, nil
}

// createVMsInParallel creates multiple VMs concurrently with bounded concurrency.
// All VMs are attempted regardless of individual failures.
func createVMsInParallel(
	project *config.Project,
	vmsToCreate []config.VM,
	resolvedImages map[string]resolvedImage,
	pubKey, subnetBase, dnsServer string,
) []vmCreateResult {
	results := make([]vmCreateResult, len(vmsToCreate))
	group := new(errgroup.Group)
	group.SetLimit(maxParallelVMCreations)

	for i, vmConfig := range vmsToCreate {
		results[i].vmConfig = vmConfig

		group.Go(func() error {
			results[i].err = createSingleVM(
				project, vmConfig, resolvedImages[vmConfig.Image],
				pubKey, subnetBase, dnsServer,
			)
			return nil
		})
	}

	_ = group.Wait()
	return results
}

// createSingleVM generates cloud-init config, builds the seed ISO, and creates a VM.
func createSingleVM(
	project *config.Project,
	vmConfig config.VM,
	img resolvedImage,
	pubKey, subnetBase, dnsServer string,
) error {
	domainName := project.DomainName(vmConfig.Name)

	ui.Printf("Creating %s (%s, %dMB, %d CPUs)...\n",
		vmConfig.Name, vmConfig.Image, vmConfig.Memory, vmConfig.CPUs)

	sharedMounts := buildSharedMounts(vmConfig)
	announceSharedMounts(vmConfig.Name, sharedMounts)
	shareIdentity, err := resolveShareIdentity(vmConfig.Name, sharedMounts)
	if err != nil {
		return err
	}

	macAddress, err := vm.GenerateMACAddress()
	if err != nil {
		return err
	}

	ciFiles, err := cloudinit.GenerateFiles(
		project.StateDir, vmConfig,
		project.Herdfile.SSH.User, project.Herdfile.SSH.Password, pubKey,
		subnetBase, dnsServer, macAddress, buildCloudInitMounts(sharedMounts), shareIdentity.sshUserUID,
	)
	if err != nil {
		return err
	}

	seedPath := project.SeedPath(vmConfig.Name)
	if err := cloudinit.BuildAndInstallSeed(ciFiles, seedPath); err != nil {
		return err
	}

	var consoleLogPath string
	if vmConfig.ConsoleLog {
		consoleLogPath = project.ConsoleLogPath(vmConfig.Name)
	}

	if err := vm.Create(vm.CreateOptions{
		VMName:           vmConfig.Name,
		DomainName:       domainName,
		DiskPath:         project.DiskPath(vmConfig.Name),
		SeedPath:         seedPath,
		NetworkName:      project.Herdfile.Network.Name,
		MACAddress:       macAddress,
		Memory:           vmConfig.Memory,
		CPUs:             vmConfig.CPUs,
		ImagePath:        img.cachePath,
		DiskSize:         vmConfig.Disk,
		SharedFolders:    sharedMounts,
		QEMUProcessLabel: shareIdentity.qemuProcessLabel,
		ConsoleLogPath:   consoleLogPath,
	}); err != nil {
		return err
	}

	ui.Printf("%s created.\n", vmConfig.Name)
	return nil
}

// buildSharedMounts converts Herdfile shared folder config into vm.SharedFolderMount
// structs using the host paths resolved and checked during Herdfile validation.
func buildSharedMounts(vmConfig config.VM) []vm.SharedFolderMount {
	var mounts []vm.SharedFolderMount
	for i, sf := range vmConfig.SharedFolders {
		mountTag := fmt.Sprintf("vmherd_share_%d", i)
		mounts = append(mounts, vm.SharedFolderMount{
			HostPath:  sf.ResolvedHost,
			GuestPath: sf.Guest,
			MountTag:  mountTag,
		})
	}
	return mounts
}

// shareIdentity says who QEMU runs as and which UID the guest SSH user gets
// for a VM with shared folders. The zero value means "no shares, defaults".
type shareIdentity struct {
	qemuProcessLabel string // libvirt DAC label "+UID:+GID" for the QEMU process
	sshUserUID       int    // guest SSH user UID matching the host user, 0 = guest default
}

// resolveShareIdentity runs QEMU as the host user (with the /dev/kvm group) for
// VMs with shared folders, and gives the guest SSH user the host UID so files
// have the same owner on both sides. VMs without shares keep libvirt's defaults.
func resolveShareIdentity(vmName string, sharedMounts []vm.SharedFolderMount) (shareIdentity, error) {
	if len(sharedMounts) == 0 {
		return shareIdentity{}, nil
	}
	hostIdentity, err := vm.DetectHostIdentity()
	if err != nil {
		return shareIdentity{}, fmt.Errorf("preparing shared folders for %s: %w", vmName, err)
	}
	guestUID, uidUsable := hostIdentity.GuestUserUID()
	if !uidUsable {
		ui.Printf("Warning: host UID %d is outside the regular user range, so the SSH user in %s keeps its default UID; "+
			"only root in the guest can write to shared folders — set 'shared_folders: []' if you don't need them\n",
			hostIdentity.UserID, vmName)
	}
	return shareIdentity{qemuProcessLabel: hostIdentity.QEMUProcessLabel(), sshUserUID: guestUID}, nil
}

// announceSharedMounts prints every host directory shared into a VM, so the
// default project share and any extra shares are never a surprise. One
// Printf call keeps the lines together when VMs are created in parallel.
func announceSharedMounts(vmName string, sharedMounts []vm.SharedFolderMount) {
	if len(sharedMounts) == 0 {
		return
	}
	var announcement strings.Builder
	for _, sharedMount := range sharedMounts {
		fmt.Fprintf(&announcement, "  %s: sharing %s → %s (read-write)\n", vmName, sharedMount.HostPath, sharedMount.GuestPath)
	}
	ui.Printf("%s", announcement.String())
}

// buildCloudInitMounts converts vm.SharedFolderMount into cloudinit.SharedMount
// for cloud-init 9p mount configuration.
func buildCloudInitMounts(mounts []vm.SharedFolderMount) []cloudinit.SharedMount {
	var ciMounts []cloudinit.SharedMount
	for _, m := range mounts {
		ciMounts = append(ciMounts, cloudinit.SharedMount{
			MountTag:  m.MountTag,
			GuestPath: m.GuestPath,
		})
	}
	return ciMounts
}
