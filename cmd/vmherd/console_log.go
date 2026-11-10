package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/boyadzhievb/vmherd/internal/config"
	"github.com/spf13/cobra"
)

// newConsoleLogCmd returns the 'vmherd console-log' subcommand for viewing serial console output.
func newConsoleLogCmd() *cobra.Command {
	consoleLogCmd := &cobra.Command{
		Use:   "console-log <name>",
		Short: "Print the serial console log for a VM",
		Long: `Print the serial console output captured during VM boot and runtime.
Requires console_log: true in the Herdfile for the target VM.
libvirt's log daemon writes the log as root-only; vmherd reads it with sudo.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			project, err := config.LoadProject()
			if err != nil {
				return err
			}
			vmName := args[0]
			return printConsoleLog(vmName, project.ConsoleLogPath(vmName), os.Stdout, sudoCatCommand)
		},
	}

	return consoleLogCmd
}

// privilegedReadCommand builds the command that prints a root-only file.
type privilegedReadCommand func(filePath string) *exec.Cmd

// sudoCatCommand prints a file with 'sudo cat', attached to the terminal so
// sudo can ask for a password.
func sudoCatCommand(filePath string) *exec.Cmd {
	sudoCat := exec.Command("sudo", "cat", "--", filePath)
	sudoCat.Stdin = os.Stdin
	sudoCat.Stderr = os.Stderr
	return sudoCat
}

// printConsoleLog copies a VM's serial console log to output. virtlogd
// (libvirt, running as root) creates the file root:root 0600, so a permission
// error falls back to readCommand (sudo) instead of loosening the file's mode.
func printConsoleLog(vmName, logPath string, output io.Writer, readCommand privilegedReadCommand) error {
	logFile, openError := os.Open(logPath)
	switch {
	case os.IsNotExist(openError):
		return fmt.Errorf("no console log for %q at %s — set console_log: true for this VM in the Herdfile and run 'vmherd reload %s'",
			vmName, logPath, vmName)
	case os.IsPermission(openError):
		privilegedRead := readCommand(logPath)
		privilegedRead.Stdout = output
		if runError := privilegedRead.Run(); runError != nil {
			return fmt.Errorf("reading console log %s with sudo: %w — libvirt writes it as root-only; check your sudo access or run 'sudo cat %s'",
				logPath, runError, logPath)
		}
		return nil
	case openError != nil:
		return fmt.Errorf("cannot open console log %s: %w", logPath, openError)
	}
	defer func() { _ = logFile.Close() }()

	if _, copyError := io.Copy(output, logFile); copyError != nil {
		return fmt.Errorf("reading console log %s: %w", logPath, copyError)
	}
	return nil
}
