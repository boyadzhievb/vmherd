package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/boyadzhievb/vmherd/internal/cloudinit"
	"github.com/boyadzhievb/vmherd/internal/config"
	"github.com/boyadzhievb/vmherd/internal/image"
	"github.com/boyadzhievb/vmherd/internal/network"
	"github.com/boyadzhievb/vmherd/internal/portfwd"
	"github.com/boyadzhievb/vmherd/internal/ssh"
	"github.com/boyadzhievb/vmherd/internal/ui"
	"github.com/boyadzhievb/vmherd/internal/vm"
	"github.com/spf13/cobra"
)

const reloadShutdownWait = 5 * time.Second

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
			project, err := config.LoadProject()
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

			if hasPortForwards(vmsToReload) {
				if err := portfwd.CheckBackend(); err != nil {
					return err
				}
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

			for _, vmConfig := range vmsToReload {
				domainName := project.DomainName(vmConfig.Name)

				teardownPortForwarding(vmConfig)

				if err := shutdownAndDestroy(domainName, vmConfig.Name); err != nil {
					return err
				}

				if err := createVM(project, vmConfig, domainName, pubKey, subnetBase, dnsServer); err != nil {
					return err
				}

				setupPortForwarding(vmConfig)
			}

			ui.Println("\nWaiting for SSH on reloaded VMs...")
			for _, vmConfig := range vmsToReload {
				if vmConfig.Network.IP == "" {
					continue
				}
				ui.Printf("  %s (%s)...", vmConfig.Name, vmConfig.Network.IP)
				if err := ssh.WaitForSSH(keyPath, project.Herdfile.SSH.User, vmConfig.Network.IP, vmConfig.Name); err != nil {
					ui.Println(" timeout")
					fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
					continue
				}
				ui.Println(" ready")
			}

			ui.Println("Reload complete.")
			return nil
		},
	}
}

// shutdownAndDestroy gracefully stops a VM, waits, then force-stops and destroys it.
func shutdownAndDestroy(domainName, vmName string) error {
	if !vm.Exists(domainName) {
		ui.Printf("%s does not exist, will create fresh.\n", vmName)
		return nil
	}

	currentState := vm.State(domainName)
	if currentState == vm.StateRunning {
		ui.Printf("Stopping %s...\n", vmName)
		if err := vm.Shutdown(domainName); err != nil {
			ui.Printf("Graceful shutdown failed for %s, force-stopping...\n", vmName)
		}
		time.Sleep(reloadShutdownWait)

		if vm.State(domainName) == vm.StateRunning {
			if err := vm.ForceStop(domainName); err != nil {
				return fmt.Errorf("cannot force-stop %s before reload: %w", vmName, err)
			}
		}
	}

	ui.Printf("Destroying %s...\n", vmName)
	if err := vm.Destroy(domainName); err != nil {
		return fmt.Errorf("cannot destroy %s before reload: %w — try 'vmherd destroy %s' manually", vmName, err, vmName)
	}
	return nil
}

// createVM resolves the image, generates cloud-init files, and creates a new VM.
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

	sharedMounts := buildSharedMounts(project.Dir, vmConfig)

	macAddress, err := vm.GenerateMACAddress()
	if err != nil {
		return err
	}

	ciFiles, err := cloudinit.GenerateFiles(
		project.StateDir, vmConfig,
		project.Herdfile.SSH.User, project.Herdfile.SSH.Password, strings.TrimSpace(pubKey),
		subnetBase, dnsServer, macAddress, buildCloudInitMounts(sharedMounts),
	)
	if err != nil {
		return err
	}

	var consoleLogPath string
	if vmConfig.ConsoleLog {
		consoleLogPath = project.ConsoleLogPath(vmConfig.Name)
	}

	if err := vm.Create(vm.CreateOptions{
		VMName:             vmConfig.Name,
		DomainName:         domainName,
		DiskPath:           project.DiskPath(vmConfig.Name),
		CloudInitUserData:  ciFiles.UserData,
		CloudInitMetaData:  ciFiles.MetaData,
		CloudInitNetConfig: ciFiles.NetworkConfig,
		NetworkName:        project.Herdfile.Network.Name,
		MACAddress:         macAddress,
		Memory:             vmConfig.Memory,
		CPUs:               vmConfig.CPUs,
		OSVariant:          entry.OSVariant,
		ImagePath:          imagePath,
		DiskSize:           vmConfig.Disk,
		SharedFolders:      sharedMounts,
		ConsoleLogPath:     consoleLogPath,
	}); err != nil {
		return err
	}

	ui.Printf("%s created.\n", vmConfig.Name)
	return nil
}
