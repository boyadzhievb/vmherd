package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/boyadzhievb/vmherd/internal/cloudinit"
	"github.com/boyadzhievb/vmherd/internal/config"
	"github.com/boyadzhievb/vmherd/internal/network"
	"github.com/boyadzhievb/vmherd/internal/ui"
	"github.com/boyadzhievb/vmherd/internal/vm"
	"github.com/spf13/cobra"
)

// newDestroyCmd creates the 'destroy' command that removes VMs and their disks.
func newDestroyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "destroy [name...]",
		Short: "Stop and remove VMs, disks, and project state (all or named)",
		RunE: func(cmd *cobra.Command, args []string) error {
			project, err := loadProjectWithDomains()
			if err != nil {
				return err
			}

			vms, err := config.FilterVMs(project.Herdfile, args)
			if err != nil {
				return err
			}

			destroyAll := len(args) == 0

			var destroyFailures []error
			for _, vmConfig := range vms {
				state, err := vm.State(project.DomainName(vmConfig.Name))
				if err != nil {
					return fmt.Errorf("cannot destroy %s: %w", vmConfig.Name, err)
				}
				if state == vm.StateNotCreated {
					ui.Printf("%s does not exist, skipping.\n", vmConfig.Name)
					continue
				}
				ui.Printf("Destroying %s...\n", vmConfig.Name)
				if err := destroySingleVM(project, vmConfig); err != nil {
					destroyFailures = append(destroyFailures, fmt.Errorf("%s: %w", vmConfig.Name, err))
				}
			}

			if destroyAll {
				if err := destroyNetworkUnlessShared(project.Herdfile.Network.Name); err != nil {
					destroyFailures = append(destroyFailures, err)
				}
				if err := config.RemoveProjectStateDir(project.StateDir, project.Herdfile.VMNames()); err != nil {
					destroyFailures = append(destroyFailures, err)
				}
			}
			if err := incompleteCommandError("destroy", errors.Join(destroyFailures...)); err != nil {
				return err
			}

			ui.Println("Done.")
			return nil
		},
	}
}

// destroySingleVM stops a VM's port forwarding, removes its libvirt domain,
// disk, seed ISO and state directory. Every step is attempted even when an
// earlier one fails; the failures are returned joined.
func destroySingleVM(project *config.Project, vmConfig config.VM) error {
	return errors.Join(
		stopPortForwarding(project, vmConfig),
		vm.Destroy(project.DomainName(vmConfig.Name)),
		vm.RemoveDisk(project.DiskPath(vmConfig.Name)),
		cloudinit.RemoveSeed(project.SeedPath(vmConfig.Name)),
		config.RemoveVMStateDir(project.StateDir, vmConfig.Name),
	)
}

// destroyNetworkUnlessShared removes the project's libvirt network only when
// no VM is attached any more. Another project's VMs on the same network would
// lose connectivity, so in that case, or when the check fails, it is kept.
func destroyNetworkUnlessShared(networkName string) error {
	attachedDomains, err := network.AttachedDomains(networkName)
	if err != nil {
		return fmt.Errorf("keeping network %s: cannot check which VMs still use it: %w", networkName, err)
	}
	if len(attachedDomains) > 0 {
		ui.Printf("Keeping network %s: still used by %s. Remove it with 'sudo virsh net-destroy %s && sudo virsh net-undefine %s' once they are gone.\n",
			networkName, strings.Join(attachedDomains, ", "), networkName, networkName)
		return nil
	}
	return network.Destroy(networkName)
}
