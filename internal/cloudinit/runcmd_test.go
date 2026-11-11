package cloudinit

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/boyadzhievb/vmherd/internal/config"
)

// runRuncmdScript runs runcmdLines the way cloud-init does (one '#!/bin/sh'
// script, no 'set -e') and returns its exit code and combined output.
func runRuncmdScript(t *testing.T, runcmdLines []string) (int, string) {
	t.Helper()
	scriptPath := filepath.Join(t.TempDir(), "runcmd")
	script := "#!/bin/sh\n" + strings.Join(runcmdLines, "\n") + "\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command("/bin/sh", scriptPath).CombinedOutput()
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		return exitError.ExitCode(), string(output)
	}
	if err != nil {
		t.Fatalf("cannot run runcmd script: %v", err)
	}
	return 0, string(output)
}

// TestRuncmdScriptSkipsStepsAfterFailure checks that a failed step skips the
// later steps, that lines after the steps (share mounts) still run, and that
// the script exits 1 naming the failed step.
func TestRuncmdScriptSkipsStepsAfterFailure(t *testing.T) {
	markerDir := t.TempDir()
	provisions := []config.Provision{
		{Type: config.ProvisionShell, Inline: "touch '" + markerDir + "/step1'"},
		{Type: config.ProvisionFile, Source: "./setup.sh", Destination: "/tmp/setup.sh"},
		{Type: config.ProvisionShell, Inline: "exit 7"},
		{Type: config.ProvisionShell, Inline: "touch '" + markerDir + "/step4'"},
	}
	runcmdLines := buildRuncmdLines(provisions, "vmherd", nil)
	// Stand in for the share mounts, which run between the steps and the check.
	afterStepsLine := "touch '" + markerDir + "/after-steps'"
	lastIndex := len(runcmdLines) - 1
	runcmdLines = append(runcmdLines[:lastIndex:lastIndex], afterStepsLine, runcmdLines[lastIndex])

	exitCode, output := runRuncmdScript(t, runcmdLines)

	if exitCode != 1 {
		t.Errorf("exit code = %d, want 1; output:\n%s", exitCode, output)
	}
	if !strings.Contains(output, FailedStepMarker+" 3 of 4 (exit 7) failed") {
		t.Errorf("output does not name step 3 of 4 with exit 7:\n%s", output)
	}
	for markerName, wantExists := range map[string]bool{"step1": true, "step4": false, "after-steps": true} {
		_, err := os.Stat(filepath.Join(markerDir, markerName))
		if gotExists := err == nil; gotExists != wantExists {
			t.Errorf("marker %s exists = %v, want %v; output:\n%s", markerName, gotExists, wantExists, output)
		}
	}
}

// TestRuncmdScriptSucceedsWhenAllStepsPass checks the script exits 0 and
// prints no failure line when every step passes, even if a step's own last
// command is the only one that counts (M1.16 covers errexit inside a step).
func TestRuncmdScriptSucceedsWhenAllStepsPass(t *testing.T) {
	provisions := []config.Provision{
		{Type: config.ProvisionShell, Inline: "true"},
		{Type: config.ProvisionShell, Inline: "echo 'quotes '\"'\"' and $HOME stay literal: $(echo no)'"},
	}
	exitCode, output := runRuncmdScript(t, buildRuncmdLines(provisions, "vmherd", nil))

	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0; output:\n%s", exitCode, output)
	}
	if strings.Contains(output, FailedStepMarker) {
		t.Errorf("unexpected failure line in output:\n%s", output)
	}
	if !strings.Contains(output, "vmherd: running provisioning step 2 of 2") {
		t.Errorf("output does not announce step 2 of 2:\n%s", output)
	}
	if !strings.Contains(output, "quotes ' and $HOME stay literal: $(echo no)") {
		t.Errorf("step script was not passed to bash unchanged:\n%s", output)
	}
}

// TestBuildRuncmdLinesWithoutShellSteps checks that VMs without shell steps
// get no failure tracking lines, and VMs without anything get no runcmd.
func TestBuildRuncmdLinesWithoutShellSteps(t *testing.T) {
	if runcmdLines := buildRuncmdLines(nil, "vmherd", nil); len(runcmdLines) != 0 {
		t.Errorf("buildRuncmdLines() without steps or shares = %q, want none", runcmdLines)
	}
	sharedMounts := []SharedMount{{MountTag: "vmherd_share_0", GuestPath: "/vagrant"}}
	for _, runcmdLine := range buildRuncmdLines(nil, "vmherd", sharedMounts) {
		if strings.Contains(runcmdLine, failedStepVariable) {
			t.Errorf("share-only runcmd tracks failed steps: %q", runcmdLine)
		}
	}
}

func TestClassifyStatusExitCode(t *testing.T) {
	testCases := map[int]FirstBootOutcome{
		0:   FirstBootDone,
		1:   FirstBootFailed,
		2:   FirstBootDoneWithWarnings,
		127: FirstBootNoCloudInit,
		3:   FirstBootFailed,
	}
	for exitCode, wantOutcome := range testCases {
		if gotOutcome := ClassifyStatusExitCode(exitCode); gotOutcome != wantOutcome {
			t.Errorf("ClassifyStatusExitCode(%d) = %q, want %q", exitCode, gotOutcome, wantOutcome)
		}
	}
}

// TestFailedStepLineFromScriptOutput checks that the line the runcmd script
// prints for a failed step is found in a log excerpt.
func TestFailedStepLineFromScriptOutput(t *testing.T) {
	provisions := []config.Provision{{Type: config.ProvisionShell, Inline: "echo working; false"}}
	_, output := runRuncmdScript(t, buildRuncmdLines(provisions, "vmherd", nil))
	logExcerpt := "Cloud-init v. 26.1 running 'modules:final'\n" + output + "Cloud-init v. 26.1 finished\n"

	wantLine := FailedStepMarker + " 1 of 1 (exit 1) failed; later steps were skipped"
	if gotLine := FailedStepLine(logExcerpt); gotLine != wantLine {
		t.Errorf("FailedStepLine() = %q, want %q", gotLine, wantLine)
	}
	if gotLine := FailedStepLine("Cloud-init v. 26.1 running 'modules:final'\nok\n"); gotLine != "" {
		t.Errorf("FailedStepLine() without a failed step = %q, want empty", gotLine)
	}
}

// TestReportsMissingNineP checks that the marker printed by the 9p check is
// found in a cloud-init log excerpt, and that ordinary mount errors are not
// mistaken for it.
func TestReportsMissingNineP(t *testing.T) {
	logExcerpt := "Cloud-init v. 24.4 running 'modules:final'\n" +
		MissingNinePMarker + ", which this guest kernel does not have\n" +
		"mount: /vagrant: unknown filesystem type '9p'.\n"
	if !ReportsMissingNineP(logExcerpt) {
		t.Errorf("ReportsMissingNineP() = false for a log with the marker")
	}
	if ReportsMissingNineP("mount: /vagrant: special device vmherd_share_0 does not exist.\n") {
		t.Errorf("ReportsMissingNineP() = true for a log without the marker")
	}
	if !strings.Contains(ninePSupportCheckCommand(), MissingNinePMarker) {
		t.Errorf("ninePSupportCheckCommand() = %q, does not print the marker", ninePSupportCheckCommand())
	}
}

// TestNoNinePCheckWithoutShares checks that VMs without shares get no 9p
// check, so images without 9p still provision cleanly.
func TestNoNinePCheckWithoutShares(t *testing.T) {
	if runcmds := sharedMountRuncmds(nil); len(runcmds) != 0 {
		t.Errorf("sharedMountRuncmds(nil) = %q, want none", runcmds)
	}
}

// shellStep returns a shell provisioning step with the default options.
func shellStep(script string) config.Provision {
	return config.Provision{Type: config.ProvisionShell, Inline: script}
}

// TestShellStepStopsAtFirstFailure checks M1.16: a script whose first command
// fails and whose last succeeds fails the step by default (bash -e -o
// pipefail, also for a failing command inside a pipe), and succeeds with
// errexit: false, as plain bash (Vagrant) would.
func TestShellStepStopsAtFirstFailure(t *testing.T) {
	plainBash := false
	for _, testCase := range []struct {
		name       string
		step       config.Provision
		wantFailed bool
	}{
		{"default, failing first command", shellStep("false\necho last-command-ran"), true},
		{"default, failure inside a pipe", shellStep("false | cat\necho last-command-ran"), true},
		{"errexit false, plain bash", config.Provision{Type: config.ProvisionShell, Inline: "false\necho last-command-ran", Errexit: &plainBash}, false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			exitCode, output := runRuncmdScript(t, buildRuncmdLines([]config.Provision{testCase.step}, "vmherd", nil))
			stepFailed := strings.Contains(output, FailedStepMarker)
			if stepFailed != testCase.wantFailed || (exitCode != 0) != testCase.wantFailed {
				t.Errorf("exit %d, step failed = %v, want failed = %v; output:\n%s", exitCode, stepFailed, testCase.wantFailed, output)
			}
			if ranLastCommand := strings.Contains(output, "last-command-ran"); ranLastCommand == testCase.wantFailed {
				t.Errorf("last command ran = %v, want %v; output:\n%s", ranLastCommand, !testCase.wantFailed, output)
			}
		})
	}
}

// TestFailureMarkerSearchFindsLinesOutsideTheTail runs the search command on a
// log whose failure lines are followed by more boot output than the tail
// shows, as on Debian, Fedora and Arch, and on a log without failures.
func TestFailureMarkerSearchFindsLinesOutsideTheTail(t *testing.T) {
	failedStepLine := FailedStepMarker + " 2 of 2 (exit 1) failed; later steps were skipped"
	missingNinePLine := MissingNinePMarker + ", which this guest kernel does not have"
	var logContent strings.Builder
	logContent.WriteString("Cloud-init v. 25.1 running 'modules:final'\n")
	logContent.WriteString("vmherd: running provisioning step 2 of 2\n")
	logContent.WriteString(missingNinePLine + "\n" + failedStepLine + "\n")
	for lineNumber := 0; lineNumber < 50; lineNumber++ {
		logContent.WriteString("|  ssh host key randomart  |\n")
	}
	logPath := filepath.Join(t.TempDir(), "cloud-init-output.log")
	if err := os.WriteFile(logPath, []byte(logContent.String()), 0o600); err != nil {
		t.Fatalf("writing log: %v", err)
	}

	output, err := exec.Command("sh", "-c", FailureMarkerSearchCommand(logPath)).Output()
	if err != nil {
		t.Fatalf("search command: %v", err)
	}
	if gotLine := FailedStepLine(string(output)); gotLine != failedStepLine {
		t.Errorf("FailedStepLine(search output) = %q, want %q", gotLine, failedStepLine)
	}
	if !ReportsMissingNineP(string(output)) {
		t.Errorf("search output %q does not report the missing 9p", output)
	}
	if strings.Contains(string(output), "running provisioning step") {
		t.Errorf("search output %q includes the step start line", output)
	}

	cleanLogPath := filepath.Join(t.TempDir(), "clean.log")
	if err := os.WriteFile(cleanLogPath, []byte("Cloud-init v. 25.1 finished\n"), 0o600); err != nil {
		t.Fatalf("writing log: %v", err)
	}
	err = exec.Command("sh", "-c", FailureMarkerSearchCommand(cleanLogPath)).Run()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
		t.Errorf("search on a log without failure lines: want exit 1, got %v", err)
	}
}
