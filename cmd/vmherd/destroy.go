package main

import (
	"fmt"
	"os"
	"strings"

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

			for _, vmConfig := range vms {
				domainName := project.DomainName(vmConfig.Name)

				if !vm.Exists(domainName) {
					ui.Printf("%s does not exist, skipping.\n", vmConfig.Name)
					continue
				}

				ui.Printf("Destroying %s...\n", vmConfig.Name)
				teardownPortForwarding(vmConfig)
				if err := vm.Destroy(domainName); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
				}
				if err := config.RemoveVMStateDir(project.StateDir, vmConfig.Name); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
				}
			}

			if destroyAll {
				if err := destroyNetworkUnlessShared(project.Herdfile.Network.Name); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
				}
				if err := config.RemoveProjectStateDir(project.StateDir, project.Herdfile.VMNames()); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
				}
			}

			ui.Println("Done.")
			return nil
		},
	}
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
