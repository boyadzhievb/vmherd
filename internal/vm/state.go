package vm

import (
	"errors"
	"fmt"
	"os/exec"
	"slices"
	"strings"
)

// VMState is the lifecycle state of a VM: one of the states 'virsh domstate'
// prints, or StateNotCreated when libvirt has no such domain.
type VMState string

const (
	// StateRunning indicates the guest is powered on and running.
	StateRunning VMState = "running"
	// StateIdle indicates the guest is powered on but blocked on a resource
	// (virsh "idle"); vmherd treats it like running.
	StateIdle VMState = "idle"
	// StatePaused indicates the guest's CPUs are suspended ('virsh suspend').
	StatePaused VMState = "paused"
	// StateShuttingDown indicates the guest is handling a shutdown request.
	StateShuttingDown VMState = "in shutdown"
	// StateStopped indicates the VM exists but is powered off.
	StateStopped VMState = "shut off"
	// StateCrashed indicates the guest crashed. The domain may still be
	// active (on_crash=preserve) or already inactive.
	StateCrashed VMState = "crashed"
	// StateDying indicates libvirt is tearing the domain down.
	StateDying VMState = "dying"
	// StatePMSuspended indicates the guest suspended itself (S3/S4 sleep).
	StatePMSuspended VMState = "pmsuspended"
	// StateNotCreated indicates libvirt has no domain with this name.
	StateNotCreated VMState = "not created"
)

// libvirtUnreachableHint is appended to errors from virsh queries that failed
// for a reason other than a missing domain.
const libvirtUnreachableHint = "check that libvirtd is running ('sudo systemctl status libvirtd') and that 'sudo virsh list' works"

// IsRunning reports whether the guest is up and can be reached over SSH.
func (state VMState) IsRunning() bool {
	return state == StateRunning || state == StateIdle
}

// isPoweredOn reports whether the guest has not reached power-off yet. A
// crashed domain counts as off: whether it is still active is checked
// separately by forceStopIfActive.
func isPoweredOn(state VMState) bool {
	switch state {
	case StateRunning, StateIdle, StatePaused, StateShuttingDown, StateDying, StatePMSuspended:
		return true
	default:
		return false
	}
}

// State returns the libvirt state of a domain, or StateNotCreated when
// libvirt has no domain with that name. Any other virsh failure (libvirtd
// down, sudo refused) is returned as an error, never as "not created".
func State(domainName string) (VMState, error) {
	domstateOutput, domstateErr := exec.Command("sudo", "virsh", "domstate", domainName).Output()
	if domstateErr == nil {
		return parseDomainState(domainName, string(domstateOutput))
	}

	// domstate failed: tell "no such domain" from other failures by listing
	// all domains, which does not depend on virsh's (translated) messages.
	defined, listErr := isDomainListed(domainName, includeInactiveDomains)
	if listErr != nil {
		return "", fmt.Errorf("cannot query state of VM %s: %w", domainName, listErr)
	}
	if !defined {
		return StateNotCreated, nil
	}
	return "", fmt.Errorf("cannot query state of VM %s: 'virsh domstate' failed%s (%w) — %s",
		domainName, virshStderrDetail(domstateErr), domstateErr, libvirtUnreachableHint)
}

// parseDomainState maps 'virsh domstate' output to a VMState. Output vmherd
// doesn't know is an error, so a new libvirt state is never mistaken for one
// that commands handle.
func parseDomainState(domainName, domstateOutput string) (VMState, error) {
	state := VMState(strings.TrimSpace(domstateOutput))
	switch state {
	case StateRunning, StateIdle, StatePaused, StateShuttingDown, StateStopped,
		StateCrashed, StateDying, StatePMSuspended:
		return state, nil
	default:
		return "", fmt.Errorf("VM %s is in state %q, which vmherd does not know — check it with 'sudo virsh dominfo %s' and report it at the vmherd issue tracker",
			domainName, state, domainName)
	}
}

// domainListScope selects which domains 'virsh list' prints.
type domainListScope bool

const (
	// includeInactiveDomains lists every defined domain ('virsh list --all').
	includeInactiveDomains domainListScope = true
	// activeDomainsOnly lists only domains with a running QEMU process.
	activeDomainsOnly domainListScope = false
)

// isDomainListed reports whether 'virsh list --name' (with --all for
// includeInactiveDomains) contains domainName.
func isDomainListed(domainName string, scope domainListScope) (bool, error) {
	listArguments := []string{"virsh", "list", "--name"}
	if scope == includeInactiveDomains {
		listArguments = append(listArguments, "--all")
	}
	listOutput, err := exec.Command("sudo", listArguments...).Output()
	if err != nil {
		return false, fmt.Errorf("'virsh list' failed%s (%w) — %s", virshStderrDetail(err), err, libvirtUnreachableHint)
	}
	return slices.Contains(parseDomainNames(string(listOutput)), domainName), nil
}

// parseDomainNames returns the non-empty lines of 'virsh list --name' output.
func parseDomainNames(listOutput string) []string {
	var domainNames []string
	for _, line := range strings.Split(listOutput, "\n") {
		if domainName := strings.TrimSpace(line); domainName != "" {
			domainNames = append(domainNames, domainName)
		}
	}
	return domainNames
}

// virshStderrDetail returns ": <stderr>" for a failed exec.Cmd.Output call
// whose stderr was captured, or "" when there is none.
func virshStderrDetail(err error) string {
	var exitError *exec.ExitError
	if !errors.As(err, &exitError) {
		return ""
	}
	stderrText := strings.Join(strings.Fields(string(exitError.Stderr)), " ")
	if stderrText == "" {
		return ""
	}
	return ": " + stderrText
}
