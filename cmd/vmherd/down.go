package main

import (
	"time"

	"github.com/boyadzhievb/vmherd/internal/config"
	"github.com/boyadzhievb/vmherd/internal/ui"
	"github.com/boyadzhievb/vmherd/internal/vm"
	"github.com/spf13/cobra"
)

const gracefulShutdownWait = 5 * time.Second

func newDownCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "down [name...]",
		Short: "Stop VMs gracefully (all or named)",
		RunE: func(cmd *cobra.Command, args []string) error {
			project, err := config.LoadProject()
			if err != nil {
				return err
			}

			vms, err := config.FilterVMs(project.Herdfile, args)
			if err != nil {
				return err
			}

			for _, vmConfig := range vms {
				domainName := project.DomainName(vmConfig.Name)
				state := vm.State(domainName)

				if state != "running" {
					ui.Printf("%s is not running (state: %s).\n", vmConfig.Name, state)
					continue
				}

				ui.Printf("Stopping %s...\n", vmConfig.Name)
				teardownPortForwarding(vmConfig)
				if err := vm.Shutdown(domainName); err != nil {
					ui.Printf("Warning: graceful shutdown failed for %s: %v\n", vmConfig.Name, err)
				}
			}

			time.Sleep(gracefulShutdownWait)

			for _, vmConfig := range vms {
				domainName := project.DomainName(vmConfig.Name)
				if vm.State(domainName) == "running" {
					ui.Printf("Force-stopping %s...\n", vmConfig.Name)
					if err := vm.ForceStop(domainName); err != nil {
						ui.Printf("Warning: force-stop failed for %s: %v\n", vmConfig.Name, err)
					}
				}
			}

			ui.Println("All VMs stopped.")
			return nil
		},
	}
}
