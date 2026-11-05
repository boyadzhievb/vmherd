package ssh

import (
	"crypto/rand"
	"encoding/hex"
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
	// uploadNameRandomBytes is the randomness in provision upload file names.
	uploadNameRandomBytes = 8
)

// Connect opens an interactive SSH session to a VM, or runs a command if provided.
// See remoteCommandLine for how command reaches the remote shell.
func Connect(keyPath, user, ip string, command []string) error {
	args := sshBaseArgs(keyPath, user, ip)

	if len(command) > 0 {
		args = append(args, remoteCommandLine(command))
	}

	cmd := exec.Command("ssh", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// RunScriptAsRoot runs a shell script on a VM as root via 'sudo bash -c'.
// The script travels as one quoted argument, so the remote shell can't split
// it, and stdin stays free for commands in the script.
func RunScriptAsRoot(keyPath, user, ip, script string) error {
	args := append(sshBaseArgs(keyPath, user, ip), rootScriptCommandLine(script))

	cmd := exec.Command("ssh", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// InstallFileAsRoot copies localPath to destination on a VM as root:root with
// fileMode, like cloud-init write_files does on first boot. scp uploads to a
// random name in the SSH user's home, then 'sudo install' moves it into place,
// so root-owned destinations and files from first boot can be replaced.
func InstallFileAsRoot(keyPath, user, ip, localPath, destination, fileMode string) error {
	uploadName, err := randomUploadName()
	if err != nil {
		return err
	}

	remoteUploadTarget := fmt.Sprintf("%s@%s:%s", user, ip, uploadName)
	if err := CopyFiles(keyPath, user, []string{localPath, remoteUploadTarget}); err != nil {
		return fmt.Errorf("uploading %s to %s: %w", localPath, ip, err)
	}

	installArgs := append(sshBaseArgs(keyPath, user, ip), installUploadCommandLine(uploadName, destination, fileMode))
	cmd := exec.Command("ssh", installArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("installing %s as root with mode %s: %w", destination, fileMode, err)
	}
	return nil
}

// randomUploadName returns a hidden, unguessable file name for a provision
// upload in the SSH user's home directory.
func randomUploadName() (string, error) {
	randomBytes := make([]byte, uploadNameRandomBytes)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", fmt.Errorf("generating upload file name: %w", err)
	}
	return ".vmherd-upload-" + hex.EncodeToString(randomBytes), nil
}

// installUploadCommandLine returns the remote command line that installs the
// uploaded file at destination (creating parent directories) and always
// removes the upload, keeping the install's exit status.
func installUploadCommandLine(uploadName, destination, fileMode string) string {
	quotedUpload := quoteForShell(uploadName)
	return fmt.Sprintf("sudo install -D -m %s -- %s %s; installStatus=$?; rm -f -- %s; exit $installStatus",
		quoteForShell(fileMode), quotedUpload, quoteForShell(destination), quotedUpload)
}

// rootScriptCommandLine returns the remote command line that runs script as root.
func rootScriptCommandLine(script string) string {
	return "sudo bash -c " + quoteForShell(script)
}

// remoteCommandLine turns 'vmherd ssh -- ...' arguments into the command line
// ssh hands to the remote shell. A single argument is used as is, so a shell
// command line like 'ls | wc -l' works. Several arguments are each quoted, so
// each reaches the remote program as exactly one argument (ls "/a b").
func remoteCommandLine(command []string) string {
	if len(command) == 1 {
		return command[0]
	}
	quotedArguments := make([]string, len(command))
	for index, argument := range command {
		quotedArguments[index] = quoteForShell(argument)
	}
	return strings.Join(quotedArguments, " ")
}

// quoteForShell wraps value in single quotes for a POSIX shell. Each embedded
// single quote closes the quoting, adds a backslash-escaped quote and reopens.
func quoteForShell(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
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
