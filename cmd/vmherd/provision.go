package main

import (
	"fmt"

	"github.com/boyadzhievb/vmherd/internal/config"
	"github.com/boyadzhievb/vmherd/internal/ssh"
	"github.com/boyadzhievb/vmherd/internal/ui"
	"github.com/boyadzhievb/vmherd/internal/vm"
	"github.com/spf13/cobra"
)

// newProvisionCmd creates the 'provision' command that reruns provisioning on running VMs.
func newProvisionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "provision [name...]",
		Short: "Rerun provisioning steps on running VMs (all or named)",
		Long: `Run all provisioning steps defined in the Herdfile on running VMs via SSH.
Shell steps are executed remotely. File steps are copied via scp.

Examples:
  vmherd provision
  vmherd provision ctrl`,
		RunE: func(cmd *cobra.Command, args []string) error {
			project, err := config.LoadProject()
			if err != nil {
				return err
			}

			vmsToProvision, err := config.FilterVMs(project.Herdfile, args)
			if err != nil {
				return err
			}

			keyPath := project.SSHKeyPath()
			sshUser := project.Herdfile.SSH.User

			for _, vmConfig := range vmsToProvision {
				if err := provisionVM(project, vmConfig, keyPath, sshUser); err != nil {
					return err
				}
			}

			ui.Println("Provisioning complete.")
			return nil
		},
	}
}

// provisionVM executes all provisioning steps on a single running VM.
func provisionVM(project *config.Project, vmConfig config.VM, keyPath, sshUser string) error {
	domainName := project.DomainName(vmConfig.Name)
	currentState := vm.State(domainName)

	if currentState != "running" {
		return fmt.Errorf("VM %q is not running (state: %s) — run 'vmherd up %s' first", vmConfig.Name, currentState, vmConfig.Name)
	}

	if len(vmConfig.Provision) == 0 {
		ui.Printf("%s: no provisioning steps defined, skipping.\n", vmConfig.Name)
		return nil
	}

	if vmConfig.Network.IP == "" {
		return fmt.Errorf("VM %q has no IP address — cannot provision without network access", vmConfig.Name)
	}

	totalSteps := len(vmConfig.Provision)
	ui.Printf("Provisioning %s (%d steps)...\n", vmConfig.Name, totalSteps)

	for stepIndex, step := range vmConfig.Provision {
		stepNumber := stepIndex + 1

		switch step.Type {
		case "shell":
			if err := provisionShellStep(keyPath, sshUser, vmConfig, step, stepNumber, totalSteps); err != nil {
				return err
			}
		case "file":
			if err := provisionFileStep(keyPath, sshUser, vmConfig, step, stepNumber, totalSteps); err != nil {
				return err
			}
		default:
			return fmt.Errorf("VM %q: unknown provisioning type %q in step %d — supported types: shell, file",
				vmConfig.Name, step.Type, stepNumber)
		}
	}

	ui.Printf("%s: all %d steps completed.\n", vmConfig.Name, totalSteps)
	return nil
}

// provisionShellStep runs an inline shell command on the VM via SSH.
func provisionShellStep(keyPath, sshUser string, vmConfig config.VM, step config.Provision, stepNumber, totalSteps int) error {
	if step.Inline == "" {
		ui.Printf("  step %d/%d: shell (empty, skipping)\n", stepNumber, totalSteps)
		return nil
	}

	ui.Printf("  step %d/%d: shell...", stepNumber, totalSteps)

	command := []string{"bash", "-c", step.Inline}
	if err := ssh.Connect(keyPath, sshUser, vmConfig.Network.IP, command); err != nil {
		ui.Println(" failed")
		return fmt.Errorf("VM %q: shell provisioning step %d failed: %w", vmConfig.Name, stepNumber, err)
	}

	ui.Println(" done")
	return nil
}

// provisionFileStep copies a file to the VM via scp, then sets it executable.
func provisionFileStep(keyPath, sshUser string, vmConfig config.VM, step config.Provision, stepNumber, totalSteps int) error {
	if step.Source == "" || step.Destination == "" {
		return fmt.Errorf("VM %q: file provisioning step %d requires both 'source' and 'destination'", vmConfig.Name, stepNumber)
	}

	ui.Printf("  step %d/%d: file %s → %s...", stepNumber, totalSteps, step.Source, step.Destination)

	remoteDestination := fmt.Sprintf("%s@%s:%s", sshUser, vmConfig.Network.IP, step.Destination)
	if err := ssh.CopyFiles(keyPath, sshUser, []string{step.Source, remoteDestination}); err != nil {
		ui.Println(" failed")
		return fmt.Errorf("VM %q: file copy step %d failed: %w — check that %s exists", vmConfig.Name, stepNumber, err, step.Source)
	}

	chmodCommand := []string{"chmod", "0644", step.Destination}
	if err := ssh.Connect(keyPath, sshUser, vmConfig.Network.IP, chmodCommand); err != nil {
		ui.Println(" chmod failed")
		return fmt.Errorf("VM %q: cannot set permissions on %s: %w", vmConfig.Name, step.Destination, err)
	}

	ui.Println(" done")
	return nil
}
