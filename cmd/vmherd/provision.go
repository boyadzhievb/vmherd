package main

import (
	"fmt"

	"github.com/boyadzhievb/vmherd/internal/config"
	"github.com/boyadzhievb/vmherd/internal/ssh"
	"github.com/boyadzhievb/vmherd/internal/ui"
	"github.com/boyadzhievb/vmherd/internal/vm"
	"github.com/spf13/cobra"
)

// provisionedFileMode is the mode set on files copied by 'type: file' steps.
const provisionedFileMode = "0644"

// newProvisionCmd creates the 'provision' command that reruns provisioning on running VMs.
func newProvisionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "provision [name...]",
		Short: "Rerun provisioning steps on running VMs (all or named)",
		Long: `Run all provisioning steps defined in the Herdfile on running VMs via SSH.
Shell steps run as root. File steps are copied via scp and installed as root
with mode 0644, matching first boot.

Examples:
  vmherd provision
  vmherd provision ctrl`,
		RunE: func(cmd *cobra.Command, args []string) error {
			project, err := loadProjectWithDomains()
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
	currentState, err := vm.State(domainName)
	if err != nil {
		return fmt.Errorf("provisioning %s: %w", vmConfig.Name, err)
	}
	if !currentState.IsRunning() {
		return fmt.Errorf("VM %q is not running (state: %s) — run 'vmherd up %s' first", vmConfig.Name, currentState, vmConfig.Name)
	}

	if len(vmConfig.Provision) == 0 {
		ui.Printf("%s: no provisioning steps defined, skipping.\n", vmConfig.Name)
		return nil
	}

	vmConfig, err = withCurrentAddress(project, vmConfig)
	if err != nil {
		return err
	}

	totalSteps := len(vmConfig.Provision)
	ui.Printf("Provisioning %s (%d steps)...\n", vmConfig.Name, totalSteps)

	for stepIndex, step := range vmConfig.Provision {
		stepNumber := stepIndex + 1

		switch step.Type {
		case config.ProvisionShell:
			if err := provisionShellStep(keyPath, sshUser, vmConfig, step, stepNumber, totalSteps); err != nil {
				return err
			}
		case config.ProvisionFile:
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

// provisionShellStep runs an inline shell script on the VM as root via SSH,
// matching first-boot runcmd, which also runs as root.
func provisionShellStep(keyPath, sshUser string, vmConfig config.VM, step config.Provision, stepNumber, totalSteps int) error {
	if step.Inline == "" {
		ui.Printf("  step %d/%d: shell (empty, skipping)\n", stepNumber, totalSteps)
		return nil
	}

	ui.Printf("  step %d/%d: shell...", stepNumber, totalSteps)

	if err := ssh.RunScriptAsRoot(keyPath, sshUser, vmConfig.Network.IP, step.Inline); err != nil {
		ui.Println(" failed")
		return fmt.Errorf("VM %q: shell provisioning step %d failed: %w", vmConfig.Name, stepNumber, err)
	}

	ui.Println(" done")
	return nil
}

// provisionFileStep copies a file to the VM as root:root with
// provisionedFileMode, the same result as first-boot write_files. It can
// replace those files and write to root-owned destinations like /etc.
func provisionFileStep(keyPath, sshUser string, vmConfig config.VM, step config.Provision, stepNumber, totalSteps int) error {
	if step.Source == "" || step.Destination == "" {
		return fmt.Errorf("VM %q: file provisioning step %d requires both 'source' and 'destination'", vmConfig.Name, stepNumber)
	}

	ui.Printf("  step %d/%d: file %s → %s...", stepNumber, totalSteps, step.Source, step.Destination)

	if err := ssh.InstallFileAsRoot(keyPath, sshUser, vmConfig.Network.IP, step.Source, step.Destination, provisionedFileMode); err != nil {
		ui.Println(" failed")
		return fmt.Errorf("VM %q: file provisioning step %d failed: %w — check that %s exists and the VM is reachable", vmConfig.Name, stepNumber, err, step.Source)
	}

	ui.Println(" done")
	return nil
}
