package main

import (
	"fmt"
	"io"
	"os"

	"github.com/boyadzhievb/vmherd/internal/config"
	"github.com/spf13/cobra"
)

// newConsoleLogCmd returns the 'vmherd console-log' subcommand for viewing serial console output.
func newConsoleLogCmd() *cobra.Command {
	consoleLogCmd := &cobra.Command{
		Use:   "console-log <name>",
		Short: "Print the serial console log for a VM",
		Long: `Print the serial console output captured during VM boot and runtime.
Requires console_log: true in the Herdfile for the target VM.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			project, err := config.LoadProject()
			if err != nil {
				return err
			}

			vmName := args[0]
			logPath := project.ConsoleLogPath(vmName)

			logFile, openError := os.Open(logPath)
			if openError != nil {
				if os.IsNotExist(openError) {
					return fmt.Errorf("no console log for %q — ensure console_log: true is set in the Herdfile and the VM has been created", vmName)
				}
				return fmt.Errorf("cannot open console log: %w", openError)
			}
			defer func() { _ = logFile.Close() }()

			if _, copyError := io.Copy(os.Stdout, logFile); copyError != nil {
				return fmt.Errorf("cannot read console log: %w", copyError)
			}

			return nil
		},
	}

	return consoleLogCmd
}
