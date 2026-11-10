package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// fakePrivilegedRead stands in for 'sudo cat': it prints a marker and records
// the path it was asked to read.
func fakePrivilegedRead(requestedPaths *[]string) privilegedReadCommand {
	return func(filePath string) *exec.Cmd {
		*requestedPaths = append(*requestedPaths, filePath)
		return exec.Command("printf", "read-with-sudo")
	}
}

// TestPrintConsoleLogReadable checks that a readable log is copied directly,
// without the privileged reader.
func TestPrintConsoleLogReadable(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "console.log")
	if err := os.WriteFile(logPath, []byte("boot output\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	var requestedPaths []string
	if err := printConsoleLog("ctrl", logPath, &output, fakePrivilegedRead(&requestedPaths)); err != nil {
		t.Fatalf("printConsoleLog: %v", err)
	}
	if output.String() != "boot output\n" || len(requestedPaths) != 0 {
		t.Errorf("output = %q, privileged reads = %q; want the file content and no privileged read", output.String(), requestedPaths)
	}
}

// TestPrintConsoleLogRootOnly checks that a log the user can't open (virtlogd
// writes root:root 0600) is read through the privileged reader.
func TestPrintConsoleLogRootOnly(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can open a 0000 file, so the permission error can't be produced")
	}
	logPath := filepath.Join(t.TempDir(), "console.log")
	if err := os.WriteFile(logPath, []byte("secret"), 0o000); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	var requestedPaths []string
	if err := printConsoleLog("ctrl", logPath, &output, fakePrivilegedRead(&requestedPaths)); err != nil {
		t.Fatalf("printConsoleLog: %v", err)
	}
	if output.String() != "read-with-sudo" || len(requestedPaths) != 1 || requestedPaths[0] != logPath {
		t.Errorf("output = %q, privileged reads = %q; want one privileged read of %s", output.String(), requestedPaths, logPath)
	}

	failingRead := func(string) *exec.Cmd { return exec.Command("false") }
	err := printConsoleLog("ctrl", logPath, &output, failingRead)
	if err == nil || !strings.Contains(err.Error(), "sudo cat "+logPath) {
		t.Errorf("failed privileged read: error = %v, want a hint to run 'sudo cat %s'", err, logPath)
	}
}

// TestPrintConsoleLogMissing checks that a missing log tells the user to
// enable console_log for that VM.
func TestPrintConsoleLogMissing(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "console.log")
	var requestedPaths []string
	err := printConsoleLog("ctrl", logPath, &bytes.Buffer{}, fakePrivilegedRead(&requestedPaths))
	if err == nil || !strings.Contains(err.Error(), "console_log: true") || !strings.Contains(err.Error(), "vmherd reload ctrl") {
		t.Errorf("error = %v, want the console_log hint", err)
	}
	if len(requestedPaths) != 0 {
		t.Errorf("privileged reads = %q, want none for a missing log", requestedPaths)
	}
}
