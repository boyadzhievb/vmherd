package main

import (
	"fmt"
	"time"

	"github.com/boyadzhievb/vmherd/internal/config"
	"github.com/boyadzhievb/vmherd/internal/ssh"
	"github.com/boyadzhievb/vmherd/internal/ui"
	"github.com/boyadzhievb/vmherd/internal/vm"
	"github.com/spf13/cobra"
)

const restartShutdownWait = 5 * time.Second

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
			project, err := config.LoadProject()
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
				waitForSSHInParallel(restartedVMs, keyPath, project.Herdfile.SSH.User)
			}

			ui.Println("Restart complete.")
			return nil
		},
	}
}

// stopAndStartVMs gracefully stops running VMs, waits, force-stops any
// stragglers, then starts all VMs. Returns the list of VMs that were started.
func stopAndStartVMs(project *config.Project, vms []config.VM) ([]config.VM, error) {
	var runningVMs []config.VM
	var stoppedVMs []config.VM

	for _, vmConfig := range vms {
		domainName := project.DomainName(vmConfig.Name)
		state := vm.State(domainName)

		switch state {
		case "running":
			runningVMs = append(runningVMs, vmConfig)
		case "shut off":
			stoppedVMs = append(stoppedVMs, vmConfig)
		default:
			return nil, fmt.Errorf("cannot restart %s: VM does not exist — run 'vmherd up %s' first", vmConfig.Name, vmConfig.Name)
		}
	}

	for _, vmConfig := range runningVMs {
		domainName := project.DomainName(vmConfig.Name)
		ui.Printf("Stopping %s...\n", vmConfig.Name)
		teardownPortForwarding(vmConfig)
		_ = vm.Shutdown(domainName)
	}

	if len(runningVMs) > 0 {
		time.Sleep(restartShutdownWait)

		for _, vmConfig := range runningVMs {
			domainName := project.DomainName(vmConfig.Name)
			if vm.State(domainName) == "running" {
				ui.Printf("Force-stopping %s...\n", vmConfig.Name)
				if err := vm.ForceStop(domainName); err != nil {
					return nil, fmt.Errorf("cannot force-stop %s: %w", vmConfig.Name, err)
				}
			}
		}
	}

	allVMs := append(runningVMs, stoppedVMs...)
	var startedVMs []config.VM

	for _, vmConfig := range allVMs {
		domainName := project.DomainName(vmConfig.Name)
		ui.Printf("Starting %s...\n", vmConfig.Name)
		if err := vm.Start(domainName); err != nil {
			return nil, fmt.Errorf("cannot start %s: %w", vmConfig.Name, err)
		}
		setupPortForwarding(vmConfig)
		startedVMs = append(startedVMs, vmConfig)
	}

	return startedVMs, nil
}
