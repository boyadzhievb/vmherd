package main

import (
	"fmt"

	"github.com/boyadzhievb/vmherd/internal/config"
	"github.com/boyadzhievb/vmherd/internal/ssh"
	"github.com/boyadzhievb/vmherd/internal/ui"
	"github.com/boyadzhievb/vmherd/internal/vm"
	"github.com/spf13/cobra"
)

// newRestartCmd creates the 'restart' command that stops and starts VMs.
func newRestartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "restart [name...]",
		Short: "Stop and start VMs (all or named)",
		Long: `Restart gracefully stops the specified VMs, then starts them again.
Unlike reload, restart preserves the VM disk and configuration — it is a
simple reboot. VMs that are not running are started directly.

Examples:
  vmherd restart
  vmherd restart ctrl worker-1`,
		RunE: func(cmd *cobra.Command, args []string) error {
			project, err := loadProjectWithDomains()
			if err != nil {
				return err
			}

			vmsToRestart, err := config.FilterVMs(project.Herdfile, args)
			if err != nil {
				return err
			}

			keyPath, err := ssh.EnsureKeyPair(project.StateDir, "vmherd-"+project.Name)
			if err != nil {
				return err
			}

			restartedVMs, err := stopAndStartVMs(project, vmsToRestart)
			if err != nil {
				return err
			}

			if len(restartedVMs) > 0 {
				restartedVMs = waitForAddresses(project, restartedVMs)
				waitForSSHInParallel(restartedVMs, keyPath, project.Herdfile.SSH.User)
				startPortForwardingForVMs(project, restartedVMs)
			}

			ui.Println("Restart complete.")
			return nil
		},
	}
}

// stopAndStartVMs gracefully stops running VMs (in parallel, forcing only
// after the shutdown timeout), then starts all VMs. Returns the started VMs.
func stopAndStartVMs(project *config.Project, vms []config.VM) ([]config.VM, error) {
	var runningVMs []config.VM
	var stoppedVMs []config.VM

	for _, vmConfig := range vms {
		domainName := project.DomainName(vmConfig.Name)
		state, err := vm.State(domainName)
		if err != nil {
			return nil, fmt.Errorf("cannot restart %s: %w", vmConfig.Name, err)
		}

		switch state {
		case vm.StateNotCreated:
			return nil, fmt.Errorf("cannot restart %s: VM does not exist — run 'vmherd up %s' first", vmConfig.Name, vmConfig.Name)
		case vm.StateStopped:
			stoppedVMs = append(stoppedVMs, vmConfig)
		default:
			// Running, paused, crashed, sleeping or shutting down:
			// stopVMsGracefully brings each of these to power-off.
			runningVMs = append(runningVMs, vmConfig)
		}
	}

	if err := stopVMsGracefully(project, runningVMs); err != nil {
		return nil, fmt.Errorf("cannot restart: %w", err)
	}

	allVMs := append(runningVMs, stoppedVMs...)
	var startedVMs []config.VM

	for _, vmConfig := range allVMs {
		domainName := project.DomainName(vmConfig.Name)
		ui.Printf("Starting %s...\n", vmConfig.Name)
		if err := vm.Start(domainName); err != nil {
			return nil, fmt.Errorf("cannot start %s: %w", vmConfig.Name, err)
		}
		startedVMs = append(startedVMs, vmConfig)
	}

	return startedVMs, nil
}
