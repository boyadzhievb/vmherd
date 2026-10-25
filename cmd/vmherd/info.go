package main

import (
	"fmt"

	"github.com/boyadzhievb/vmherd/internal/config"
	"github.com/spf13/cobra"
)

// newInfoCmd returns the 'vmherd info' subcommand for querying project state.
func newInfoCmd() *cobra.Command {
	infoCmd := &cobra.Command{
		Use:   "info",
		Short: "Show project state paths and configuration",
		Long:  "Print paths to project state (SSH key, state directory) for use in scripts and CI.",
	}

	infoCmd.AddCommand(
		newInfoSSHKeyCmd(),
		newInfoStateDirCmd(),
	)

	return infoCmd
}

// newInfoSSHKeyCmd returns 'vmherd info ssh-key' — prints the SSH private key path.
func newInfoSSHKeyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ssh-key",
		Short: "Print the path to the project SSH private key",
		RunE: func(cmd *cobra.Command, args []string) error {
			project, err := config.LoadProject()
			if err != nil {
				return err
			}
			fmt.Println(project.SSHKeyPath())
			return nil
		},
	}
}

// newInfoStateDirCmd returns 'vmherd info state-dir' — prints the state directory.
func newInfoStateDirCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "state-dir",
		Short: "Print the path to the project state directory",
		RunE: func(cmd *cobra.Command, args []string) error {
			project, err := config.LoadProject()
			if err != nil {
				return err
			}
			fmt.Println(project.StateDir)
			return nil
		},
	}
}
