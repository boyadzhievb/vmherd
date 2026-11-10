package cloudinit

import (
	"fmt"
	"strings"

	"github.com/boyadzhievb/vmherd/internal/config"
	"gopkg.in/yaml.v3"
)

// cloud-init writes every runcmd string entry verbatim as one line of a single
// '#!/bin/sh' script (/var/lib/cloud/instance/scripts/runcmd) without 'set -e',
// so the script's exit status is only its last line's. vmherd therefore tracks
// provisioning failures itself in a shell variable of that script.

// failedStepVariable is the runcmd-script variable that holds the first failed
// provisioning step ("2 of 3 (exit 1)"), empty while all steps succeed.
const failedStepVariable = "vmherd_failed_step"

// FailedStepMarker starts the line the runcmd script prints to stderr (and
// so to /var/log/cloud-init-output.log) when a provisioning step failed.
const FailedStepMarker = "vmherd: provisioning step"

// buildRuncmdLines returns the runcmd script lines in order: reset the failure
// variable, give the SSH user its home back (shares only), run the shell
// provisioning steps, mount the shares, and exit 1 if a step failed. After a
// failed step the later steps are skipped, but the share mounts still run.
func buildRuncmdLines(provisions []config.Provision, sshUser string, sharedMounts []SharedMount) []string {
	stepLines := provisionStepLines(provisions)

	var runcmdLines []string
	if len(stepLines) > 0 {
		runcmdLines = append(runcmdLines, failedStepVariable+"=")
	}
	if len(sharedMounts) > 0 {
		runcmdLines = append(runcmdLines, bashCommandLine(homeOwnershipCommand(sshUser)))
	}
	runcmdLines = append(runcmdLines, stepLines...)
	for _, mountCommand := range sharedMountRuncmds(sharedMounts) {
		runcmdLines = append(runcmdLines, bashCommandLine(mountCommand))
	}
	if len(stepLines) > 0 {
		runcmdLines = append(runcmdLines, failedStepCheckLine())
	}
	return runcmdLines
}

// provisionStepLines returns one guarded runcmd line per shell step (Herdfile
// validation rejects empty ones). Steps are numbered by their position among
// all provisioning steps, as 'vmherd provision' numbers them.
func provisionStepLines(provisions []config.Provision) []string {
	var stepLines []string
	for stepIndex, step := range provisions {
		if step.Type != config.ProvisionShell {
			continue
		}
		stepLabel := fmt.Sprintf("%d of %d", stepIndex+1, len(provisions))
		stepLines = append(stepLines, guardedStepLine(stepLabel, step))
	}
	return stepLines
}

// guardedStepLine runs the step's script with bash (and the step's options,
// see config.Provision.BashOptions) unless an earlier step failed, and
// records stepLabel plus the exit code in failedStepVariable if it fails.
func guardedStepLine(stepLabel string, step config.Provision) string {
	return fmt.Sprintf(`[ -n "$%s" ] || { echo %s; %s; } || %s="%s (exit $?)"`,
		failedStepVariable,
		quoteForShell(fmt.Sprintf("vmherd: running provisioning step %s", stepLabel)),
		bashCommandLineWithOptions(step.BashOptions(), step.Inline),
		failedStepVariable, stepLabel)
}

// failedStepCheckLine ends the runcmd script with exit 1 when a step failed,
// so cloud-init reports status "error", and names the step on stderr.
func failedStepCheckLine() string {
	return fmt.Sprintf(`[ -z "$%s" ] || { echo "%s $%s failed; later steps were skipped" >&2; exit 1; }`,
		failedStepVariable, FailedStepMarker, failedStepVariable)
}

// bashCommandLine returns a shell line that runs script with bash, the way
// cloud-init renders a ["/bin/bash", "-c", script] runcmd entry.
func bashCommandLine(script string) string {
	return bashCommandLineWithOptions(nil, script)
}

// bashCommandLineWithOptions returns a shell line that runs script with bash
// and the given options (e.g. -e -o pipefail) before -c.
func bashCommandLineWithOptions(bashOptions []string, script string) string {
	optionsPrefix := ""
	if len(bashOptions) > 0 {
		optionsPrefix = strings.Join(bashOptions, " ") + " "
	}
	return "/bin/bash " + optionsPrefix + "-c " + quoteForShell(strings.TrimSpace(script))
}

// writeRuncmdSection appends the runcmd section to user-data.
func writeRuncmdSection(builder *strings.Builder, runcmdLines []string) error {
	return writeYAMLSection(builder, "runcmd", runcmdLines)
}

// writeYAMLSection appends "<key>:" with entries to user-data, YAML-encoded by
// yaml.v3 so no value can break out of its entry. Nothing is written when
// entries is empty.
func writeYAMLSection[Entry any](builder *strings.Builder, key string, entries []Entry) error {
	if len(entries) == 0 {
		return nil
	}
	encodedSection, err := yaml.Marshal(map[string][]Entry{key: entries})
	if err != nil {
		return fmt.Errorf("encoding cloud-init %s section: %w", key, err)
	}
	builder.Write(encodedSection)
	return nil
}
