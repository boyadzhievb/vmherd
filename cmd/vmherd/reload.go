package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/boyadzhievb/vmherd/internal/cloudinit"
	"github.com/boyadzhievb/vmherd/internal/config"
	"github.com/boyadzhievb/vmherd/internal/ssh"
	"github.com/boyadzhievb/vmherd/internal/ui"
	"github.com/boyadzhievb/vmherd/internal/vm"
	"github.com/spf13/cobra"
)

// newReloadCmd creates the 'reload' command that destroys and recreates VMs.
func newReloadCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "reload [name...]",
		Short: "Destroy and recreate VMs from the current Herdfile (all or named)",
		Long: `Reload destroys the specified VMs and recreates them from the current Herdfile.
Network and SSH keys are preserved. This is useful after changing VM configuration.

Examples:
  vmherd reload
  vmherd reload ctrl worker-1`,
		RunE: func(cmd *cobra.Command, args []string) error {
			project, err := loadProjectWithDomains()
			if err != nil {
				return err
			}

			if project.Herdfile.SSH.Password != "" {
				ui.Println("Warning: Herdfile contains a plaintext password — avoid committing this file to version control")
			}

			vmsToReload, err := config.FilterVMs(project.Herdfile, args)
			if err != nil {
				return err
			}

			resolvedImages, err := checkReloadPrerequisites(vmsToReload, project.Herdfile.Images)
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

			if err := ensureProjectNetwork(project); err != nil {
				return err
			}

			var recreatedVMs []config.VM
			var reloadFailures []error
			for _, vmConfig := range vmsToReload {
				err := reloadSingleVM(project, vmConfig, resolvedImages[vmConfig.Image],
					strings.TrimSpace(pubKey), gatewayIP, dnsServer)
				if err != nil {
					reloadFailures = append(reloadFailures, fmt.Errorf("%s: %w", vmConfig.Name, err))
					continue
				}
				recreatedVMs = append(recreatedVMs, vmConfig)
			}

			recreatedVMNames := make(map[string]bool, len(recreatedVMs))
			for _, vmConfig := range recreatedVMs {
				recreatedVMNames[vmConfig.Name] = true
			}
			_, readyErr := waitForVMsReady(project, recreatedVMs, keyPath, recreatedVMNames)
			if err := incompleteCommandError("reload", errors.Join(append(reloadFailures, readyErr)...)); err != nil {
				return err
			}

			ui.Println("Reload complete.")
			return nil
		},
	}
}

// checkReloadPrerequisites downloads and verifies every image the VMs need and
// checks the host identity used for shared folders, before reload removes
// anything: a VM is never destroyed when its replacement can't be created.
func checkReloadPrerequisites(vmsToReload []config.VM, images map[string]string) (map[string]resolvedImage, error) {
	resolvedImages, err := resolveAndCacheImages(vmsToReload, images)
	if err != nil {
		return nil, fmt.Errorf("%w\n\nreload stopped before changing any VM — the existing VMs and their disks are untouched", err)
	}
	for _, vmConfig := range vmsToReload {
		if len(vmConfig.SharedFolders) == 0 {
			continue
		}
		if _, err := vm.DetectHostIdentity(); err != nil {
			return nil, fmt.Errorf("preparing shared folders for %s: %w\n\nreload stopped before changing any VM — "+
				"the existing VMs and their disks are untouched", vmConfig.Name, err)
		}
		break
	}
	return resolvedImages, nil
}

// reloadSingleVM stops and destroys a VM (disk and seed included) and creates
// it again from the current Herdfile under the project-unique domain name.
// img must already be downloaded and verified (checkReloadPrerequisites).
func reloadSingleVM(project *config.Project, vmConfig config.VM, img resolvedImage, pubKey, defaultGateway, dnsServer string) error {
	if err := stopPortForwarding(project, vmConfig); err != nil {
		return err
	}
	if err := shutdownAndDestroy(project.DomainName(vmConfig.Name), vmConfig.Name); err != nil {
		return err
	}
	if err := vm.RemoveDisk(project.DiskPath(vmConfig.Name)); err != nil {
		return fmt.Errorf("cannot remove old disk: %w", err)
	}
	if err := cloudinit.RemoveSeed(project.SeedPath(vmConfig.Name)); err != nil {
		return fmt.Errorf("cannot remove old seed: %w", err)
	}

	// The old domain is gone, so recreate under the project-unique name.
	project.DropLegacyDomainName(vmConfig.Name)
	return createSingleVM(project, vmConfig, img, pubKey, defaultGateway, dnsServer)
}

// shutdownAndDestroy gracefully stops a VM (forcing only after the shutdown
// timeout) and destroys it.
func shutdownAndDestroy(domainName, vmName string) error {
	state, err := vm.State(domainName)
	if err != nil {
		return fmt.Errorf("cannot reload %s: %w", vmName, err)
	}
	if state == vm.StateNotCreated {
		ui.Printf("%s does not exist, will create fresh.\n", vmName)
		return nil
	}

	if state != vm.StateStopped {
		ui.Printf("Stopping %s...\n", vmName)
		stopOutcome, err := vm.StopGracefully(domainName, vm.GracefulShutdownTimeout)
		if err != nil {
			return fmt.Errorf("cannot stop %s before reload: %w", vmName, err)
		}
		reportForcedStop(vmName, stopOutcome)
	}

	ui.Printf("Destroying %s...\n", vmName)
	if err := vm.Destroy(domainName); err != nil {
		return fmt.Errorf("cannot destroy %s before reload: %w — try 'vmherd destroy %s' manually", vmName, err, vmName)
	}
	return nil
}
