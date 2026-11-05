package ssh

import (
	"errors"
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
	// remoteUploadTemplate is the mktemp template for provision uploads on the VM.
	remoteUploadTemplate = "/tmp/vmherd-upload.XXXXXXXXXX"
	// tunnelKeepaliveIntervalSeconds is how often a port-forward tunnel checks the VM is alive.
	tunnelKeepaliveIntervalSeconds = 15
	// tunnelKeepaliveMaxMissed is how many missed keepalives end a tunnel (about 45s).
	tunnelKeepaliveMaxMissed = 3
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
// private temp file from 'mktemp' (not the home directory, which may not be
// writable), then 'sudo install' moves it into place.
func InstallFileAsRoot(keyPath, user, ip, localPath, destination, fileMode string) error {
	uploadPath, err := createRemoteUploadFile(keyPath, user, ip)
	if err != nil {
		return err
	}

	remoteUploadTarget := fmt.Sprintf("%s@%s:%s", user, ip, uploadPath)
	if err := CopyFiles(keyPath, user, []string{localPath, remoteUploadTarget}); err != nil {
		return errors.Join(fmt.Errorf("uploading %s to %s: %w", localPath, ip, err), removeRemoteUploadFile(keyPath, user, ip, uploadPath))
	}

	installArgs := append(sshBaseArgs(keyPath, user, ip), installUploadCommandLine(uploadPath, destination, fileMode))
	cmd := exec.Command("ssh", installArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("installing %s as root with mode %s: %w", destination, fileMode, err)
	}
	return nil
}

// createRemoteUploadFile runs 'mktemp' on the VM and returns the path of the
// new empty file, which only the SSH user can read or write.
func createRemoteUploadFile(keyPath, user, ip string) (string, error) {
	mktempArgs := append(sshBaseArgs(keyPath, user, ip), "mktemp "+quoteForShell(remoteUploadTemplate))
	output, err := exec.Command("ssh", mktempArgs...).Output()
	if err != nil {
		return "", fmt.Errorf("creating an upload file on %s with mktemp: %w", ip, err)
	}
	uploadPath := strings.TrimSpace(string(output))
	if !isRemoteUploadPath(uploadPath) {
		return "", fmt.Errorf("mktemp on %s returned an unexpected path %q", ip, uploadPath)
	}
	return uploadPath, nil
}

// isRemoteUploadPath reports whether path looks like a file mktemp made from
// remoteUploadTemplate, so a garbled reply is never used as a scp target.
func isRemoteUploadPath(path string) bool {
	prefix := strings.TrimRight(remoteUploadTemplate, "X")
	return strings.HasPrefix(path, prefix) && len(path) == len(remoteUploadTemplate) && !strings.ContainsAny(path, " \t\n'\"")
}

// removeRemoteUploadFile deletes an upload file after a failed copy.
func removeRemoteUploadFile(keyPath, user, ip, uploadPath string) error {
	removeArgs := append(sshBaseArgs(keyPath, user, ip), "rm -f -- "+quoteForShell(uploadPath))
	if err := exec.Command("ssh", removeArgs...).Run(); err != nil {
		return fmt.Errorf("removing upload file %s on %s: %w", uploadPath, ip, err)
	}
	return nil
}

// installUploadCommandLine returns the remote command line that installs the
// uploaded file at destination (creating parent directories) and always
// removes the upload, keeping the install's exit status.
func installUploadCommandLine(uploadPath, destination, fileMode string) string {
	quotedUpload := quoteForShell(uploadPath)
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

// LocalForward is one 'ssh -L' mapping from a host address and port to a port
// on the VM's own localhost.
type LocalForward struct {
	HostIP    string // host address to listen on (127.0.0.1, 0.0.0.0, ::1, …)
	HostPort  int    // port to listen on
	GuestPort int    // port on the VM's localhost to connect to
}

// TunnelArgs returns the ssh arguments (without "ssh") for a background tunnel
// carrying forwards to a VM. The tunnel exits if any port can't be bound or the
// VM stops answering, so a dead tunnel is never mistaken for a working one.
func TunnelArgs(keyPath, user, ip string, forwards []LocalForward) []string {
	args := []string{
		"-N",
		"-o", "ExitOnForwardFailure=yes",
		"-o", fmt.Sprintf("ServerAliveInterval=%d", tunnelKeepaliveIntervalSeconds),
		"-o", fmt.Sprintf("ServerAliveCountMax=%d", tunnelKeepaliveMaxMissed),
	}
	for _, forward := range forwards {
		args = append(args, "-L", fmt.Sprintf("%s:localhost:%d", forward.ListenSpec(), forward.GuestPort))
	}
	return append(args, sshBaseArgs(keyPath, user, ip)...)
}

// ListenSpec returns the host side of the forward as written in 'ssh -L':
// "127.0.0.1:8080", or "[::1]:8080" for IPv6.
func (localForward LocalForward) ListenSpec() string {
	bindAddress := localForward.HostIP
	if strings.Contains(bindAddress, ":") {
		bindAddress = "[" + bindAddress + "]"
	}
	return fmt.Sprintf("%s:%d", bindAddress, localForward.HostPort)
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
