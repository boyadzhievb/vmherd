package ssh

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

const (
	sshConnectTimeout  = 2
	defaultWaitTimeout = 180 * time.Second
	waitPollInterval   = 2 * time.Second
)

// Connect opens an interactive SSH session to a VM, or runs a command if provided.
func Connect(keyPath, user, ip string, command []string) error {
	args := sshBaseArgs(keyPath, user, ip)

	if len(command) > 0 {
		args = append(args, strings.Join(command, " "))
	}

	cmd := exec.Command("ssh", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// CopyFiles runs scp between the local host and a VM.
func CopyFiles(keyPath, user string, args []string) error {
	scpArgs := []string{
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "LogLevel=ERROR",
		"-i", keyPath,
	}
	scpArgs = append(scpArgs, args...)

	cmd := exec.Command("scp", scpArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// WaitForSSH polls until SSH is reachable on the given IP, or until timeout.
func WaitForSSH(keyPath, user, ip, vmName string) error {
	deadline := time.Now().Add(defaultWaitTimeout)

	for time.Now().Before(deadline) {
		if trySSH(keyPath, user, ip) {
			return nil
		}
		time.Sleep(waitPollInterval)
	}

	return fmt.Errorf("SSH timeout for %s (%s) after %s — the VM may still be booting, try again in a moment",
		vmName, ip, defaultWaitTimeout)
}

func trySSH(keyPath, user, ip string) bool {
	args := sshBaseArgs(keyPath, user, ip)
	args = append(args, "true")

	cmd := exec.Command("ssh", args...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run() == nil
}

func sshBaseArgs(keyPath, user, ip string) []string {
	return []string{
		"-o", fmt.Sprintf("ConnectTimeout=%d", sshConnectTimeout),
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "LogLevel=ERROR",
		"-i", keyPath,
		fmt.Sprintf("%s@%s", user, ip),
	}
}
