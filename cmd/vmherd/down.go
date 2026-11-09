package main

import (
	"github.com/boyadzhievb/vmherd/internal/config"
	"github.com/boyadzhievb/vmherd/internal/ui"
	"github.com/boyadzhievb/vmherd/internal/vm"
	"github.com/spf13/cobra"
)

// newDownCmd creates the 'down' command that gracefully stops VMs.
func newDownCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "down [name...]",
		Short: "Stop VMs gracefully (all or named)",
		RunE: func(cmd *cobra.Command, args []string) error {
			project, err := loadProjectWithDomains()
			if err != nil {
				return err
			}

			vms, err := config.FilterVMs(project.Herdfile, args)
			if err != nil {
				return err
			}

			var runningVMs []config.VM
			for _, vmConfig := range vms {
				state, err := vm.State(project.DomainName(vmConfig.Name))
				if err != nil {
					return err
				}
				if state == vm.StateStopped || state == vm.StateNotCreated {
					ui.Printf("%s is not running (state: %s).\n", vmConfig.Name, state)
					continue
				}
				runningVMs = append(runningVMs, vmConfig)
			}

			if _, err := stopVMsGracefully(project, runningVMs); err != nil {
				return incompleteCommandError("down", err)
			}
			ui.Println("All VMs stopped.")
			return nil
		},
	}
}
