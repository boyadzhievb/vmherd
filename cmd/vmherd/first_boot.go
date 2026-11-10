package main

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/boyadzhievb/vmherd/internal/cloudinit"
	"github.com/boyadzhievb/vmherd/internal/config"
	"github.com/boyadzhievb/vmherd/internal/ssh"
	"github.com/boyadzhievb/vmherd/internal/ui"
	"golang.org/x/sync/errgroup"
)

const (
	// cloudInitWaitTimeout is how long up and reload wait for a new VM's
	// first-boot cloud-init run (provisioning steps included) to finish.
	cloudInitWaitTimeout = 30 * time.Minute
	// cloudInitLogTailLines is how many lines of the guest's cloud-init
	// output log a provisioning failure shows.
	cloudInitLogTailLines = 20
	// cloudInitLogReadTimeout bounds reading that log excerpt over SSH.
	cloudInitLogReadTimeout = 30 * time.Second
)

// firstBootResult is one VM's first-boot cloud-init outcome.
type firstBootResult struct {
	outcome cloudinit.FirstBootOutcome // done, done with warnings, failed, …
	err     error                      // why the VM failed, nil when it didn't
}

// waitForFirstBootProvisioning waits in parallel for cloud-init on VMs that
// just booted for the first time (SSH is ready). It returns the VMs whose
// provisioning finished and the joined errors of the others.
func waitForFirstBootProvisioning(vms []config.VM, keyPath, sshUser string) ([]config.VM, error) {
	if len(vms) == 0 {
		return nil, nil
	}
	ui.Printf("\nWaiting for first-boot provisioning (cloud-init, up to %s)...\n", cloudInitWaitTimeout)

	results := make([]firstBootResult, len(vms))
	group := new(errgroup.Group)
	for index, vmConfig := range vms {
		group.Go(func() error {
			results[index] = checkFirstBoot(vmConfig, keyPath, sshUser)
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil, fmt.Errorf("waiting for cloud-init: %w", err)
	}

	var provisionedVMs []config.VM
	var failures []error
	for index, vmConfig := range vms {
		result := results[index]
		ui.Printf("  %s... %s\n", vmConfig.Name, result.outcome)
		if result.err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", vmConfig.Name, result.err))
			continue
		}
		if result.outcome == cloudinit.FirstBootDoneWithWarnings {
			ui.Printf("Warning: cloud-init on %s finished with recoverable errors — see 'vmherd ssh %s -- cloud-init status --long'\n",
				vmConfig.Name, vmConfig.Name)
		}
		provisionedVMs = append(provisionedVMs, vmConfig)
	}
	return provisionedVMs, errors.Join(failures...)
}

// checkFirstBoot runs 'cloud-init status --wait' on the VM and turns a
// failure into an error with the failed step and the end of the output log.
func checkFirstBoot(vmConfig config.VM, keyPath, sshUser string) firstBootResult {
	statusOutput, exitCode, err := ssh.RunCommandWithTimeout(keyPath, sshUser, vmConfig.Network.IP,
		cloudinit.WaitForStatusCommand, cloudInitWaitTimeout)
	if err != nil {
		return firstBootResult{outcome: cloudinit.FirstBootFailed,
			err: fmt.Errorf("waiting for first-boot provisioning: %w — check progress with 'vmherd ssh %s -- sudo tail -f %s'",
				err, vmConfig.Name, cloudinit.OutputLogPath)}
	}

	outcome := cloudinit.ClassifyStatusExitCode(exitCode)
	switch outcome {
	case cloudinit.FirstBootDone, cloudinit.FirstBootDoneWithWarnings:
		return firstBootResult{outcome: outcome}
	case cloudinit.FirstBootNoCloudInit:
		return firstBootResult{outcome: outcome,
			err: fmt.Errorf("the guest has no 'cloud-init' command, so vmherd cannot tell whether provisioning ran — use an official cloud image")}
	default:
		return firstBootResult{outcome: outcome, err: describeFirstBootFailure(vmConfig, keyPath, sshUser, statusOutput)}
	}
}

// describeFirstBootFailure builds the error for a failed cloud-init run: the
// failed provisioning step (or cloud-init's own status), the last lines of
// the guest's output log, and where to read the rest.
func describeFirstBootFailure(vmConfig config.VM, keyPath, sshUser, statusOutput string) error {
	tailCommand := fmt.Sprintf("sudo tail -n %d %s", cloudInitLogTailLines, cloudinit.OutputLogPath)
	logExcerpt, tailExitCode, err := ssh.RunCommandWithTimeout(keyPath, sshUser, vmConfig.Network.IP, tailCommand, cloudInitLogReadTimeout)
	if err == nil && tailExitCode != 0 {
		err = fmt.Errorf("'%s' exited with %d", tailCommand, tailExitCode)
	}
	if err != nil {
		logExcerpt = fmt.Sprintf("(cannot read %s: %v)", cloudinit.OutputLogPath, err)
	}

	summary := cloudinit.FailedStepLine(logExcerpt)
	if cloudinit.ReportsMissingNineP(logExcerpt) {
		summary = joinFailureSummaries(summary, missingNinePSummary(vmConfig.Name))
	}
	if summary == "" {
		summary = "cloud-init reported an error:\n" + indentLines(strings.TrimSpace(statusOutput))
	}
	return fmt.Errorf("first-boot provisioning failed: %s\n  last %d lines of %s:\n%s\n  full log: vmherd ssh %s -- sudo cat %s",
		summary, cloudInitLogTailLines, cloudinit.OutputLogPath, indentLines(strings.TrimRight(logExcerpt, "\n")),
		vmConfig.Name, cloudinit.OutputLogPath)
}

// missingNinePSummary explains a share that can't be mounted because the
// guest kernel lacks 9p, and how to bring the VM up without shares.
func missingNinePSummary(vmName string) string {
	return fmt.Sprintf("shared folders can't be mounted: %s's kernel has no 9p support "+
		"(RHEL-family kernels such as Rocky and AlmaLinux are built without it) — "+
		"set 'shared_folders: []' for %s in the Herdfile and run 'vmherd reload %s'", vmName, vmName, vmName)
}

// joinFailureSummaries puts a second failure cause under the first, or
// returns it alone when there is no first one.
func joinFailureSummaries(firstSummary, secondSummary string) string {
	if firstSummary == "" {
		return secondSummary
	}
	return firstSummary + "\n  also: " + secondSummary
}

// indentLines indents every line of text by four spaces, so a multi-line
// guest excerpt stands apart from vmherd's own messages.
func indentLines(text string) string {
	return "    " + strings.ReplaceAll(text, "\n", "\n    ")
}
