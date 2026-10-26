package ssh

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

const (
	// sshConnectTimeout is the per-attempt TCP connect timeout in seconds.
	sshConnectTimeout = 2
	// initialRetryInterval is the first wait between SSH retry attempts.
	initialRetryInterval = 1 * time.Second
	// maxRetryInterval caps exponential backoff between SSH retry attempts.
	maxRetryInterval = 16 * time.Second
	// defaultWaitTimeout is the total time WaitForSSH will keep retrying.
	defaultWaitTimeout = 180 * time.Second
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
// Uses exponential backoff (1s, 2s, 4s, … capped at 16s) between attempts.
func WaitForSSH(keyPath, user, ip, vmName string) error {
	startTime := time.Now()
	currentInterval := initialRetryInterval

	for {
		if trySSH(keyPath, user, ip) {
			return nil
		}
		if time.Since(startTime) > defaultWaitTimeout {
			break
		}
		time.Sleep(currentInterval)
		currentInterval *= 2
		if currentInterval > maxRetryInterval {
			currentInterval = maxRetryInterval
		}
	}

	return fmt.Errorf("SSH timeout for %s (%s) after %s — the VM may still be booting, try again in a moment",
		vmName, ip, defaultWaitTimeout)
}

// trySSH attempts a single SSH connection and returns true if it succeeds.
func trySSH(keyPath, user, ip string) bool {
	args := sshBaseArgs(keyPath, user, ip)
	args = append(args, "true")

	cmd := exec.Command("ssh", args...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run() == nil
}

// sshBaseArgs returns the common SSH flags used for all VM connections.
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
