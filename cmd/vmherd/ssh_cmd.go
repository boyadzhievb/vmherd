package main

import (
	"github.com/boyadzhievb/vmherd/internal/config"
	"github.com/boyadzhievb/vmherd/internal/ssh"
	"github.com/spf13/cobra"
)

// newSSHCmd creates the 'ssh' command for connecting to a VM.
func newSSHCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ssh <name> [-- command...]",
		Short: "SSH into a VM or run a command on it",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			vmName := args[0]

			project, err := loadProjectWithDomains()
			if err != nil {
				return err
			}

			vms, err := config.FilterVMs(project.Herdfile, []string{vmName})
			if err != nil {
				return err
			}
			if err := requireRunningVM(project, vms[0].Name); err != nil {
				return err
			}

			targetVM, err := withCurrentAddress(project, vms[0])
			if err != nil {
				return err
			}

			keyPath := project.SSHKeyPath()

			var command []string
			if cmd.ArgsLenAtDash() >= 0 && cmd.ArgsLenAtDash() < len(args) {
				command = args[cmd.ArgsLenAtDash():]
			} else if len(args) > 1 {
				command = args[1:]
			}

			return ssh.Connect(keyPath, project.Herdfile.SSH.User, targetVM.Network.IP, command)
		},
	}
}
