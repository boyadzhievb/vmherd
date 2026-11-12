# provisioning Specification

## Purpose

Herdfile `provision` steps set a VM up: shell scripts run as root and files are copied in. They run on the VM's first boot through cloud-init, and again on demand with `vmherd provision`. A failed step is always named; unlike Vagrant's plain bash, scripts stop at their first failing command.

## Requirements

### Requirement: Step types
A step SHALL be `type: shell` with an `inline` script, or `type: file` with a project-relative `source` and an absolute `destination`. Fields of the other type, an empty script, or an unknown type SHALL be refused naming the VM and step number.

#### Scenario: Invalid steps
- **WHEN** a step has no type, an unknown type, an empty script, a shell step with `destination`, or a file step without `source` or with a relative `destination`
- **THEN** loading fails naming `VM "<vm>": provision step <n>` and what to change
- **Test:** internal/config/TestValidateFieldRules

#### Scenario: File source outside the project
- **WHEN** a file step's source is absolute, contains `..` out of the project, is the project directory, or is a symlink pointing outside
- **THEN** loading fails: provision files must be inside the project
- **Test:** internal/config/TestValidateProvisionSourceAbsolute, internal/config/TestValidateProvisionSourceTraversal, internal/config/TestCheckProvisionSourceInsideProject

### Requirement: Shell steps stop at the first error
Every shell step SHALL run as root with `bash -e -o pipefail`; `errexit: false` on a step SHALL run it with plain bash. `errexit` on a file step SHALL be refused.

#### Scenario: Strict by default
- **WHEN** a step's script is `false; touch /after`
- **THEN** the step fails and `/after` is not created
- **Test:** internal/cloudinit/TestShellStepStopsAtFirstFailure, internal/ssh/TestRootScriptCommandLineStopsAtFirstFailure, test/e2e/TestDistroLifecycle (checkProvisioningResults)

#### Scenario: Opt out
- **WHEN** a step sets `errexit: false`
- **THEN** it runs without `-e -o pipefail`
- **Test:** internal/config/TestProvisionErrexit

#### Scenario: Scripts survive quoting
- **WHEN** a script has several lines, quotes, `$` or YAML special characters
- **THEN** it runs exactly as written
- **Test:** internal/cloudinit/TestMultiLineInlineCommandWrappedInBash, internal/cloudinit/TestSingleLineCommandWrappedInBash, internal/cloudinit/TestYAMLSpecialCharactersInCommand, internal/ssh/TestRootScriptCommandLineSurvivesShell, internal/ssh/TestQuoteForShell

### Requirement: First-boot provisioning
On a VM's first boot, file steps SHALL be written by cloud-init as root:root with mode 0644 and shell steps SHALL run in Herdfile order, numbered by their position among all steps. After a failed step the later steps SHALL be skipped and cloud-init SHALL end in error. `up` and `reload` SHALL wait up to 30 minutes for cloud-init on new VMs only.

#### Scenario: Steps in cloud-init
- **WHEN** a VM has shell and file steps
- **THEN** its user-data runs the shell steps in order and writes the files with the right owner and mode
- **Test:** internal/cloudinit/TestGenerateFilesWithShellProvisioning, internal/cloudinit/TestGenerateFilesWithFileProvisioning, internal/cloudinit/TestGenerateFilesWithCombinedProvisioning, internal/cloudinit/TestGenerateFilesWithMissingSourceFile, internal/cloudinit/TestBuildRuncmdLinesWithoutShellSteps

#### Scenario: Later steps are skipped
- **WHEN** step 2 of 3 fails at first boot
- **THEN** step 3 does not run and the guest log has "vmherd: provisioning step 2 of 3 (exit <code>) failed; later steps were skipped"
- **Test:** internal/cloudinit/TestRuncmdScriptSkipsStepsAfterFailure, internal/cloudinit/TestRuncmdScriptSucceedsWhenAllStepsPass

#### Scenario: up names the failed step
- **WHEN** a first-boot step fails
- **THEN** up exits 1, names the VM and the step, shows the last 20 lines of the guest's `/var/log/cloud-init-output.log`, and prints the command to read the whole log
- **Test:** internal/cloudinit/TestFailedStepLineFromScriptOutput, internal/cloudinit/TestFailureMarkerSearchFindsLinesOutsideTheTail, test/e2e/TestDistroLifecycle (checkFirstUp)

#### Scenario: cloud-init outcomes
- **WHEN** `cloud-init status --wait` exits 0, 2 (recoverable errors) or 1, or the guest has no cloud-init
- **THEN** vmherd reports done, done with a warning and the command to inspect it, failed, or "use an official cloud image"
- **Test:** internal/cloudinit/TestClassifyStatusExitCode

#### Scenario: Restart does not reprovision
- **WHEN** an existing VM is started by up or restart
- **THEN** vmherd doesn't wait for cloud-init and the steps don't run again
- **Test:** test/e2e/TestDistroLifecycle (checkRestartDownUp)

### Requirement: vmherd provision
`vmherd provision [vm...]` SHALL run every step of each named running VM over SSH as root, in order: shell steps with the same bash options as first boot, file steps copied with scp and installed root:root 0644. It SHALL stop at the first failed step, naming the VM and step number, and SHALL refuse a VM that is not running.

#### Scenario: Rerun succeeds
- **WHEN** the user runs `vmherd provision <vm>` on a running VM whose steps succeed
- **THEN** it prints each step as done and exits 0
- **Test:** test/e2e/TestDistroLifecycle (checkProvisionCommand)

#### Scenario: Rerun fails
- **WHEN** step 2 of a VM fails
- **THEN** provision exits 1 with `VM "<vm>": shell provisioning step 2 failed`
- **Test:** test/e2e/TestDistroLifecycle (checkProvisionCommand)

#### Scenario: File step
- **WHEN** a file step targets a root-owned directory such as `/etc`
- **THEN** the file is installed there as root:root 0644 and the temporary upload is removed even when installing fails
- **Test:** internal/ssh/TestInstallUploadCommandLineInstallsAndCleansUp, internal/ssh/TestInstallUploadCommandLineKeepsFailureAndCleansUp, internal/ssh/TestIsRemoteUploadPath

#### Scenario: VM not running
- **WHEN** the VM is shut off
- **THEN** provision fails with "is not running … run 'vmherd up <vm>' first"
- **Test:** none yet — cmd/ code, needs M2.3

#### Scenario: Several VMs, one fails
- **WHEN** `vmherd provision` runs on all VMs and the first VM's step fails
- **THEN** provision stops there and the later VMs are not provisioned (unlike up, which attempts every VM)
- **Test:** none yet — cmd/ code, needs M2.3
