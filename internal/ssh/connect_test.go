package ssh

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestSSHBaseArgs verifies that sshBaseArgs produces the correct SSH arguments
// with proper count, key path placement, user@ip format, and timeout value.
func TestSSHBaseArgs(t *testing.T) {
	testKeyPath := "/home/vmherd/.ssh/id_ed25519"
	testUser := "vmherd"
	testIP := "192.168.122.10"

	args := sshBaseArgs(testKeyPath, testUser, testIP)

	// Expected layout: 4x("-o", value) + "-i" + keyPath + user@ip = 11 elements.
	const expectedArgCount = 11
	if len(args) != expectedArgCount {
		t.Fatalf("sshBaseArgs returned %d args, want %d: %v", len(args), expectedArgCount, args)
	}

	// Verify ConnectTimeout uses the package constant.
	expectedTimeoutArg := fmt.Sprintf("ConnectTimeout=%d", sshConnectTimeout)
	if args[1] != expectedTimeoutArg {
		t.Errorf("args[1] = %q, want %q", args[1], expectedTimeoutArg)
	}

	// Verify the "-i" flag is at index 8, with the key path at index 9.
	const keyFlagIndex = 8
	const keyPathIndex = 9
	if args[keyFlagIndex] != "-i" {
		t.Errorf("args[%d] = %q, want %q", keyFlagIndex, args[keyFlagIndex], "-i")
	}
	if args[keyPathIndex] != testKeyPath {
		t.Errorf("args[%d] = %q, want key path %q", keyPathIndex, args[keyPathIndex], testKeyPath)
	}

	// Verify user@ip format in the last element.
	expectedUserHost := fmt.Sprintf("%s@%s", testUser, testIP)
	lastArg := args[len(args)-1]
	if lastArg != expectedUserHost {
		t.Errorf("last arg = %q, want %q", lastArg, expectedUserHost)
	}
}

// TestExponentialBackoffConstants verifies that the retry interval constants
// form a valid exponential backoff configuration.
func TestExponentialBackoffConstants(t *testing.T) {
	if initialRetryInterval <= 0 {
		t.Errorf("initialRetryInterval = %s, must be positive", initialRetryInterval)
	}
	if maxRetryInterval < initialRetryInterval {
		t.Errorf("maxRetryInterval (%s) must be >= initialRetryInterval (%s)",
			maxRetryInterval, initialRetryInterval)
	}
	if defaultWaitTimeout < maxRetryInterval {
		t.Errorf("defaultWaitTimeout (%s) must be >= maxRetryInterval (%s)",
			defaultWaitTimeout, maxRetryInterval)
	}

	// Simulate the backoff sequence and ensure it converges to the cap.
	currentInterval := initialRetryInterval
	for i := 0; i < 10; i++ {
		currentInterval *= 2
		if currentInterval > maxRetryInterval {
			currentInterval = maxRetryInterval
		}
	}
	if currentInterval != maxRetryInterval {
		t.Errorf("backoff did not converge to cap: got %s, want %s",
			currentInterval, maxRetryInterval)
	}
}

// TestBackoffSequenceValues verifies the exact sequence of intervals
// produced by the exponential backoff logic.
func TestBackoffSequenceValues(t *testing.T) {
	expectedSequence := []time.Duration{
		1 * time.Second,
		2 * time.Second,
		4 * time.Second,
		8 * time.Second,
		16 * time.Second,
		16 * time.Second, // capped
	}

	currentInterval := initialRetryInterval
	for i, expected := range expectedSequence {
		if currentInterval != expected {
			t.Errorf("step %d: interval = %s, want %s", i, currentInterval, expected)
		}
		currentInterval *= 2
		if currentInterval > maxRetryInterval {
			currentInterval = maxRetryInterval
		}
	}
}

func TestQuoteForShell(t *testing.T) {
	testCases := []struct {
		value      string // raw argument
		wantQuoted string // single-quoted form for a POSIX shell
	}{
		{"plain", `'plain'`},
		{"/a b", `'/a b'`},
		{"it's", `'it'\''s'`},
		{"$HOME `id` \\ \"x\"", "'$HOME `id` \\ \"x\"'"},
		{"", `''`},
	}
	for _, testCase := range testCases {
		if gotQuoted := quoteForShell(testCase.value); gotQuoted != testCase.wantQuoted {
			t.Errorf("quoteForShell(%q) = %s, want %s", testCase.value, gotQuoted, testCase.wantQuoted)
		}
	}
}

func TestRemoteCommandLine(t *testing.T) {
	testCases := []struct {
		command         []string // arguments after 'vmherd ssh <vm> --'
		wantCommandLine string   // command line handed to the remote shell
	}{
		{[]string{"ls | wc -l"}, "ls | wc -l"},
		{[]string{"ls", "/a b"}, `'ls' '/a b'`},
		{[]string{"chmod", "0644", "/tmp/it's here"}, `'chmod' '0644' '/tmp/it'\''s here'`},
	}
	for _, testCase := range testCases {
		if gotCommandLine := remoteCommandLine(testCase.command); gotCommandLine != testCase.wantCommandLine {
			t.Errorf("remoteCommandLine(%q) = %s, want %s", testCase.command, gotCommandLine, testCase.wantCommandLine)
		}
	}
}

// TestRootScriptCommandLineSurvivesShell runs the generated command line
// through a local POSIX shell, with sudo replaced by env, and checks that a
// multi-line script with quotes and && arrives and runs intact.
func TestRootScriptCommandLineSurvivesShell(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}
	script := "set -u\nfirst='it'\"'\"'s'\nsecond=\"$first & more\"\necho \"$second\" && echo done\n"
	commandLine := rootScriptCommandLine(script, nil)
	if !strings.HasPrefix(commandLine, "sudo bash -c ") {
		t.Fatalf("expected a sudo bash -c command line, got %s", commandLine)
	}
	localCommandLine := "env" + strings.TrimPrefix(commandLine, "sudo")

	output, err := exec.Command("sh", "-c", localCommandLine).CombinedOutput()
	if err != nil {
		t.Fatalf("running %s: %v\n%s", localCommandLine, err, output)
	}
	if string(output) != "it's & more\ndone\n" {
		t.Errorf("script output = %q, want %q", output, "it's & more\ndone\n")
	}
}

func TestIsRemoteUploadPath(t *testing.T) {
	testCases := map[string]bool{
		"/tmp/vmherd-upload.Ab3dE5gH9k":  true,
		"/tmp/vmherd-upload.short":       false,
		"/etc/vmherd-upload.Ab3dE5gH9k":  false,
		"/tmp/vmherd-upload.Ab3 E5gH9k":  false,
		"":                               false,
		"/tmp/vmherd-upload.Ab3dE5gH9kX": false,
	}
	for path, want := range testCases {
		if got := isRemoteUploadPath(path); got != want {
			t.Errorf("isRemoteUploadPath(%q) = %v, want %v", path, got, want)
		}
	}
}

// requireGNUInstall skips the test unless install(1) is GNU coreutils, which
// supports -D like the Linux guests do.
func requireGNUInstall(t *testing.T) {
	t.Helper()
	if output, err := exec.Command("install", "--version").CombinedOutput(); err != nil || !strings.Contains(string(output), "GNU") {
		t.Skip("GNU install not available")
	}
}

// runInstallUploadLocally runs installUploadCommandLine in workDir through a
// POSIX shell, without sudo, and returns the error from the shell.
func runInstallUploadLocally(t *testing.T, workDir, uploadName, destination string) error {
	t.Helper()
	commandLine := strings.TrimPrefix(installUploadCommandLine(uploadName, destination, "0644"), "sudo ")
	shellCommand := exec.Command("sh", "-c", commandLine)
	shellCommand.Dir = workDir
	return shellCommand.Run()
}

func TestInstallUploadCommandLineInstallsAndCleansUp(t *testing.T) {
	requireGNUInstall(t)
	workDir := t.TempDir()
	uploadName := ".vmherd-upload-test"
	if err := os.WriteFile(filepath.Join(workDir, uploadName), []byte("hello"), 0o600); err != nil {
		t.Fatalf("writing upload: %v", err)
	}
	destination := filepath.Join(workDir, "new dir", "it's", "file.txt")

	if err := runInstallUploadLocally(t, workDir, uploadName, destination); err != nil {
		t.Fatalf("install command failed: %v", err)
	}
	info, err := os.Stat(destination)
	if err != nil {
		t.Fatalf("destination not created: %v", err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Errorf("destination mode = %o, want 644", info.Mode().Perm())
	}
	if _, err := os.Stat(filepath.Join(workDir, uploadName)); !os.IsNotExist(err) {
		t.Errorf("upload was not removed: %v", err)
	}
}

func TestInstallUploadCommandLineKeepsFailureAndCleansUp(t *testing.T) {
	requireGNUInstall(t)
	workDir := t.TempDir()
	uploadName := ".vmherd-upload-test"
	if err := os.WriteFile(filepath.Join(workDir, uploadName), []byte("hello"), 0o600); err != nil {
		t.Fatalf("writing upload: %v", err)
	}
	blockingFile := filepath.Join(workDir, "not-a-dir")
	if err := os.WriteFile(blockingFile, nil, 0o600); err != nil {
		t.Fatalf("writing blocking file: %v", err)
	}

	if err := runInstallUploadLocally(t, workDir, uploadName, filepath.Join(blockingFile, "file.txt")); err == nil {
		t.Error("expected the install to fail when a parent is a regular file")
	}
	if _, err := os.Stat(filepath.Join(workDir, uploadName)); !os.IsNotExist(err) {
		t.Errorf("upload was not removed after a failed install: %v", err)
	}
}

func TestTunnelArgs(t *testing.T) {
	args := TunnelArgs("/state/id_ed25519", "vmherd", "192.168.150.10", []LocalForward{
		{HostIP: "127.0.0.1", HostPort: 8080, GuestPort: 80},
		{HostIP: "::1", HostPort: 8443, GuestPort: 443},
	})
	joinedArgs := strings.Join(args, " ")
	for _, wantPart := range []string{
		"-N", "ExitOnForwardFailure=yes", "ServerAliveInterval=15", "ServerAliveCountMax=3",
		"-L 127.0.0.1:8080:localhost:80", "-L [::1]:8443:localhost:443", "-i /state/id_ed25519",
	} {
		if !strings.Contains(joinedArgs, wantPart) {
			t.Errorf("tunnel args missing %q: %s", wantPart, joinedArgs)
		}
	}
	if args[len(args)-1] != "vmherd@192.168.150.10" {
		t.Errorf("destination must come last (options after it would be a remote command), got %q", args[len(args)-1])
	}
}

// TestRootScriptCommandLineStopsAtFirstFailure checks the 'vmherd provision'
// path (M1.16): with -e -o pipefail a script whose first command fails exits
// non-zero before its last command; without options plain bash succeeds.
func TestRootScriptCommandLineStopsAtFirstFailure(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}
	script := "false\necho last-command-ran\n"
	for _, testCase := range []struct {
		bashOptions []string
		wantFailed  bool
	}{
		{[]string{"-e", "-o", "pipefail"}, true},
		{nil, false},
	} {
		localCommandLine := "env" + strings.TrimPrefix(rootScriptCommandLine(script, testCase.bashOptions), "sudo")
		output, err := exec.Command("sh", "-c", localCommandLine).CombinedOutput()
		if (err != nil) != testCase.wantFailed || strings.Contains(string(output), "last-command-ran") == testCase.wantFailed {
			t.Errorf("options %q: err = %v, output = %q; want failed = %v", testCase.bashOptions, err, output, testCase.wantFailed)
		}
	}
}
