package cloudinit

import (
	"fmt"
	"strings"
)

// WaitForStatusCommand blocks on the guest until cloud-init has finished,
// prints the detailed status (including module errors) and exits with its
// result (see ClassifyStatusExitCode).
const WaitForStatusCommand = "cloud-init status --wait --long"

// OutputLogPath is where cloud-init on the guest logs the output of runcmd
// (and so of the provisioning steps).
const OutputLogPath = "/var/log/cloud-init-output.log"

// FirstBootOutcome is the result of a guest's first-boot cloud-init run.
type FirstBootOutcome string

const (
	// FirstBootDone means cloud-init finished without errors.
	FirstBootDone FirstBootOutcome = "done"
	// FirstBootDoneWithWarnings means cloud-init finished with recoverable
	// errors (for example deprecated config keys); provisioning ran.
	FirstBootDoneWithWarnings FirstBootOutcome = "done with warnings"
	// FirstBootFailed means a cloud-init module failed, including a failed
	// provisioning step (the runcmd script exits 1).
	FirstBootFailed FirstBootOutcome = "failed"
	// FirstBootNoCloudInit means the guest has no cloud-init command.
	FirstBootNoCloudInit FirstBootOutcome = "cloud-init not found"
)

const (
	// statusExitDone is 'cloud-init status' exit code for a clean run.
	statusExitDone = 0
	// statusExitRecoverable is the exit code (cloud-init 23.4+) for a run
	// that finished with recoverable errors only.
	statusExitRecoverable = 2
	// shellCommandNotFoundExitCode is what the guest's shell returns when
	// the cloud-init command doesn't exist.
	shellCommandNotFoundExitCode = 127
)

// ClassifyStatusExitCode maps the exit code of WaitForStatusCommand to a
// FirstBootOutcome. Exit code 1 (critical error) and codes cloud-init doesn't
// document count as failed.
func ClassifyStatusExitCode(exitCode int) FirstBootOutcome {
	switch exitCode {
	case statusExitDone:
		return FirstBootDone
	case statusExitRecoverable:
		return FirstBootDoneWithWarnings
	case shellCommandNotFoundExitCode:
		return FirstBootNoCloudInit
	default:
		// 1 (critical error) and any code cloud-init doesn't document.
		return FirstBootFailed
	}
}

// FailureMarkerSearchCommand returns a guest shell command that prints every
// line of logPath the runcmd script wrote to report a failure (failed step,
// missing 9p). The whole log is searched: on many distros later boot output
// (SSH host keys) pushes those lines out of the log's tail. Like grep, it
// exits 1 when there are none.
func FailureMarkerSearchCommand(logPath string) string {
	return fmt.Sprintf("grep -F -e %s -e %s -- %s",
		quoteForShell(FailedStepMarker+" "), quoteForShell(MissingNinePMarker), quoteForShell(logPath))
}

// FailedStepLine returns the line the runcmd script printed for a failed
// provisioning step ("vmherd: provisioning step 2 of 3 (exit 1) failed; …"),
// or "" when the log excerpt has none (another cloud-init module failed).
func FailedStepLine(logExcerpt string) string {
	for _, line := range strings.Split(logExcerpt, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), FailedStepMarker+" ") {
			return strings.TrimSpace(line)
		}
	}
	return ""
}

// ReportsMissingNineP reports whether the log excerpt contains the line the
// runcmd script prints when the guest kernel has no 9p support.
func ReportsMissingNineP(logExcerpt string) bool {
	for _, line := range strings.Split(logExcerpt, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), MissingNinePMarker) {
			return true
		}
	}
	return false
}
