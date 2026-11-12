# vm-lifecycle Specification

## Purpose

`up`, `down`, `restart`, `reload`, `destroy` and `status` create, start, stop, rebuild and remove a project's VMs. Each command is idempotent, works on all VMs or the named ones, and tries every VM even when one fails.

## Requirements

### Requirement: Every VM is attempted
A lifecycle command acting on several VMs SHALL attempt every VM, SHALL report each VM that failed with its name, and SHALL exit 1 with "'vmherd <command>' did not complete for every VM" when any failed.

#### Scenario: One VM fails during up
- **WHEN** `vmherd up` creates two VMs and one VM's provisioning step fails
- **THEN** the other VM is created and ready, the failure names the VM and step, and the exit code is 1
- **Test:** test/e2e/TestDistroLifecycle (checkFirstUp)

### Requirement: up creates what is missing, starts what is stopped
`vmherd up` SHALL create VMs that don't exist, SHALL start, resume or wake VMs that exist but are not running (forcing a crashed VM off first, waiting for one that is shutting down), and SHALL leave running VMs as they are. New VMs SHALL be created in parallel, at most four at a time, after their images are downloaded and verified.

#### Scenario: First up
- **WHEN** the user runs `vmherd up` in a project with no VMs
- **THEN** every VM is created, boots, answers SSH, finishes first-boot provisioning, and up lists each ready VM with its ssh command line
- **Test:** test/e2e/TestDistroLifecycle (checkFirstUp, checkStatusShowsAddresses)

#### Scenario: up after down
- **WHEN** the VMs are shut off and the user runs `vmherd up`
- **THEN** they start with their disks unchanged and first-boot provisioning does not run again
- **Test:** test/e2e/TestDistroLifecycle (checkRestartDownUp)

#### Scenario: up while running
- **WHEN** a VM is already running
- **THEN** up prints "<vm> is already running." and does not restart it
- **Test:** none yet — cmd/ code, needs M2.3

#### Scenario: Libvirt states
- **WHEN** libvirt reports a domain as running, paused, shut off, crashed, pmsuspended, in shutdown or dying
- **THEN** vmherd maps it to its own state and refuses unknown state strings with an error
- **Test:** internal/vm/TestParseDomainStateKnownStates, internal/vm/TestParseDomainStateRejectsUnknownState, internal/vm/TestIsPoweredOn, internal/vm/TestIsRunning, internal/vm/TestEnsureRunningRefusesMissingDomain

### Requirement: Ready means reachable and provisioned
After booting, up, restart and reload SHALL wait for each VM's address (DHCP VMs up to 120 s), then for SSH with exponential backoff (up to 180 s), then, for newly created VMs only, for cloud-init to finish (up to 30 min).

#### Scenario: SSH backoff
- **WHEN** vmherd waits for SSH
- **THEN** it retries after 1, 2, 4, 8 and 16 seconds, then every 16 seconds, until the total timeout
- **Test:** internal/ssh/TestExponentialBackoffConstants, internal/ssh/TestBackoffSequenceValues

#### Scenario: A VM never answers
- **WHEN** a VM does not answer SSH before the timeout
- **THEN** up prints "<vm> (<ip>)... timeout", reports the other VMs as ready, and exits 1
- **Test:** none yet — cmd/ code, needs M2.3

### Requirement: down stops gracefully
`vmherd down` SHALL stop each running VM's port forwarding, then send an ACPI shutdown to all of them in parallel, and SHALL force a VM off only after 60 s, saying so. VMs that are already off or don't exist SHALL be reported and skipped.

#### Scenario: Graceful stop
- **WHEN** the user runs `vmherd down` with running VMs
- **THEN** every VM is shut off and the disks are kept
- **Test:** test/e2e/TestDistroLifecycle (checkRestartDownUp)

#### Scenario: Guest ignores shutdown
- **WHEN** a guest does not power off within 60 s
- **THEN** it is forced off and down prints "<vm> did not shut down within 1m0s and was forced off."
- **Test:** none yet — cmd/ code, needs M2.3

### Requirement: restart keeps the disk
`vmherd restart` SHALL stop powered-on VMs gracefully and start them again with the same disk, and SHALL fail before stopping anything when a named VM doesn't exist.

#### Scenario: Restart
- **WHEN** the user runs `vmherd restart` on a provisioned VM
- **THEN** the VM is running again, answers SSH, and files written before are still there
- **Test:** test/e2e/TestDistroLifecycle (checkRestartDownUp)

#### Scenario: Restart a missing VM
- **WHEN** a VM in the Herdfile was never created
- **THEN** restart fails with "VM does not exist — run 'vmherd up <vm>' first" and stops nothing
- **Test:** none yet — cmd/ code, needs M2.3

### Requirement: reload rebuilds only when it can
`vmherd reload` SHALL download and verify every image the VMs need, and check the host identity for shared folders, before it changes any VM. It SHALL then destroy each VM (domain, disk, seed) and create it again from the current Herdfile, keeping the network and SSH key.

#### Scenario: Image unavailable
- **WHEN** the image URL of a VM to reload no longer works
- **THEN** reload fails before touching any VM and says the existing VMs and disks are untouched
- **Test:** cmd/vmherd/TestCheckReloadPrerequisitesStopsWhenImageUnavailable

#### Scenario: Reload after a Herdfile change
- **WHEN** the user changes a VM's memory and runs `vmherd reload <vm>`
- **THEN** the VM is recreated with the new memory and runs first-boot provisioning again
- **Test:** none yet — cmd/ code, needs M2.3; an e2e phase is planned

### Requirement: destroy removes only what vmherd created
`vmherd destroy` SHALL stop port forwarding and remove each VM's domain, disk, seed ISO and state, attempting every step even when one fails. Without VM names it SHALL also remove the project's NAT network (unless other VMs are still attached, which it names) and the project state directory. It SHALL never remove a host bridge.

#### Scenario: Destroy everything
- **WHEN** the user runs `vmherd destroy` in a project
- **THEN** no domain, disk, seed ISO or network with the project's prefix remains
- **Test:** test/e2e/TestDistroLifecycle (checkDestroyLeavesNothing)

#### Scenario: Network still in use
- **WHEN** another VM is attached to the project's network
- **THEN** destroy keeps the network, names the attached VMs and prints the virsh commands to remove it later
- **Test:** internal/network/TestDomainUsesNetwork; the destroy path itself: none yet — cmd/ code, needs M2.3

#### Scenario: VM never created
- **WHEN** a VM in the Herdfile doesn't exist
- **THEN** destroy prints "<vm> does not exist, skipping." and continues
- **Test:** none yet — cmd/ code, needs M2.3

### Requirement: status is deterministic
`vmherd status` SHALL print one row per Herdfile VM, sorted by name, with columns NAME, IMAGE, IP and STATE. The IP column SHALL show the static address, or for DHCP VMs the current lease, "dhcp (no lease yet)" while booting, or "dhcp" when not running.

#### Scenario: Static and DHCP VMs
- **WHEN** one running VM has a static IP and another uses DHCP
- **THEN** status shows the static address, a leased address inside the project subnet, and `running` for both
- **Test:** test/e2e/TestDistroLifecycle (checkStatusShowsAddresses)
