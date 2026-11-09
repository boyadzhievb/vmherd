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

			var recreatedVMs []config.VM
			var reloadFailures []error
			for _, vmConfig := range vmsToReload {
				if err := reloadSingleVM(project, vmConfig, pubKey, subnetBase, dnsServer); err != nil {
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

// reloadSingleVM stops and destroys a VM (disk and seed included) and creates
// it again from the current Herdfile under the project-unique domain name.
func reloadSingleVM(project *config.Project, vmConfig config.VM, pubKey, subnetBase, dnsServer string) error {
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
	return createVM(project, vmConfig, project.DomainName(vmConfig.Name), pubKey, subnetBase, dnsServer)
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

// createVM resolves the image, generates cloud-init files, builds the seed ISO,
// and creates a new VM.
func createVM(project *config.Project, vmConfig config.VM, domainName, pubKey, subnetBase, dnsServer string) error {
	entry, err := image.Resolve(vmConfig.Image, project.Herdfile.Images)
	if err != nil {
		return err
	}

	imagePath, err := image.EnsureCached(entry)
	if err != nil {
		return err
	}

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
		project.Herdfile.SSH.User, project.Herdfile.SSH.Password, strings.TrimSpace(pubKey),
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
		ImagePath:        imagePath,
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
