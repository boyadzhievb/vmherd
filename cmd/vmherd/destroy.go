package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/boyadzhievb/vmherd/internal/config"
	"github.com/boyadzhievb/vmherd/internal/network"
	"github.com/boyadzhievb/vmherd/internal/ui"
	"github.com/boyadzhievb/vmherd/internal/vm"
	"github.com/spf13/cobra"
)

func newDestroyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "destroy [name...]",
		Short: "Stop and remove VMs, disks, and project state (all or named)",
		RunE: func(cmd *cobra.Command, args []string) error {
			project, err := config.LoadProject()
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
				vmStateDir := filepath.Join(project.StateDir, vmConfig.Name)
				_ = os.RemoveAll(vmStateDir)
			}

			if destroyAll {
				if err := network.Destroy(project.Herdfile.Network.Name); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
				}
				if err := os.RemoveAll(project.StateDir); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: cannot remove %s: %v\n", project.StateDir, err)
				}
			}

			ui.Println("Done.")
			return nil
		},
	}
}
