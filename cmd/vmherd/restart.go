package main

import (
	"errors"
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

			restartedVMs, restartErr, err := stopAndStartVMs(project, vmsToRestart)
			if err != nil {
				return err
			}
			_, readyErr := waitForVMsReady(project, restartedVMs, keyPath)
			if err := incompleteCommandError("restart", errors.Join(restartErr, readyErr)); err != nil {
				return err
			}

			ui.Println("Restart complete.")
			return nil
		},
	}
}

// stopAndStartVMs gracefully stops powered-on VMs (in parallel, forcing only
// after the shutdown timeout), then starts every VM that is off. It returns
// the started VMs and the joined errors of VMs that could not be stopped or
// started; err is set only when nothing was attempted.
func stopAndStartVMs(project *config.Project, vms []config.VM) (startedVMs []config.VM, vmFailures error, err error) {
	poweredOnVMs, stoppedVMs, err := splitVMsByPower(project, vms)
	if err != nil {
		return nil, nil, err
	}

	nowStoppedVMs, stopErr := stopVMsGracefully(project, poweredOnVMs)
	failures := []error{stopErr}

	for _, vmConfig := range append(nowStoppedVMs, stoppedVMs...) {
		ui.Printf("Starting %s...\n", vmConfig.Name)
		if err := vm.Start(project.DomainName(vmConfig.Name)); err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", vmConfig.Name, err))
			continue
		}
		startedVMs = append(startedVMs, vmConfig)
	}
	return startedVMs, errors.Join(failures...), nil
}

// splitVMsByPower sorts VMs into those still powered on in any way (running,
// paused, crashed, sleeping, shutting down) and those shut off. A VM that
// doesn't exist is an error, before anything is stopped.
func splitVMsByPower(project *config.Project, vms []config.VM) (poweredOnVMs, stoppedVMs []config.VM, err error) {
	for _, vmConfig := range vms {
		state, err := vm.State(project.DomainName(vmConfig.Name))
		if err != nil {
			return nil, nil, fmt.Errorf("cannot restart %s: %w", vmConfig.Name, err)
		}
		switch state {
		case vm.StateNotCreated:
			return nil, nil, fmt.Errorf("cannot restart %s: VM does not exist — run 'vmherd up %s' first", vmConfig.Name, vmConfig.Name)
		case vm.StateStopped:
			stoppedVMs = append(stoppedVMs, vmConfig)
		default:
			poweredOnVMs = append(poweredOnVMs, vmConfig)
		}
	}
	return poweredOnVMs, stoppedVMs, nil
}
