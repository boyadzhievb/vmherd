package vm

import (
	"fmt"
	"time"

	"github.com/boyadzhievb/vmherd/internal/libvirt"
	"github.com/boyadzhievb/vmherd/internal/ui"
)

const (
	// GracefulShutdownTimeout is how long StopGracefully waits for the guest
	// to power off before forcing it off (Vagrant's graceful_halt_timeout).
	GracefulShutdownTimeout = 60 * time.Second
	// shutdownInitialPollInterval is the first wait between state checks while stopping.
	shutdownInitialPollInterval = 1 * time.Second
	// shutdownMaxPollInterval caps the backoff between state checks while stopping.
	shutdownMaxPollInterval = 4 * time.Second
)

// StopOutcome says how StopGracefully brought a VM to power-off.
type StopOutcome string

const (
	// StopAlreadyOff means the VM was already powered off or not created.
	StopAlreadyOff StopOutcome = "already off"
	// StopGraceful means the guest shut down by itself.
	StopGraceful StopOutcome = "shut down"
	// StopForcedAfterTimeout means the guest did not power off within the
	// timeout (or refused the request) and was forced off.
	StopForcedAfterTimeout StopOutcome = "forced off after timeout"
	// StopForcedNoGuest means the guest could not handle a shutdown request
	// (crashed or sleeping) and was forced off right away.
	StopForcedNoGuest StopOutcome = "forced off"
)

// EnsureRunning brings an existing domain from state to running: start a
// stopped VM, resume a paused one, wake a sleeping one, wait for a shutdown
// in progress to finish and start again, or force a crashed one off and start it.
func EnsureRunning(domainName string, state VMState) error {
	switch state {
	case StateRunning, StateIdle:
		return nil
	case StateStopped:
		return Start(domainName)
	case StatePaused:
		return Resume(domainName)
	case StatePMSuspended:
		return WakeUp(domainName)
	case StateShuttingDown, StateDying:
		if _, err := waitForPowerOff(domainName, GracefulShutdownTimeout); err != nil {
			return err
		}
		return Start(domainName)
	case StateCrashed:
		if err := forceStopIfActive(domainName); err != nil {
			return err
		}
		return Start(domainName)
	default:
		return fmt.Errorf("cannot start VM %s: it is in state %q — run 'vmherd up' to create it", domainName, state)
	}
}

// StopGracefully brings the domain to power-off. A running guest gets an ACPI
// shutdown request and up to timeout to power off (checked with backoff); a
// paused guest is resumed first. Crashed and sleeping guests are forced off.
func StopGracefully(domainName string, timeout time.Duration) (StopOutcome, error) {
	state, err := State(domainName)
	if err != nil {
		return "", err
	}
	switch state {
	case StateStopped, StateNotCreated:
		return StopAlreadyOff, nil
	case StateCrashed, StatePMSuspended:
		return StopForcedNoGuest, forceStopIfActive(domainName)
	case StatePaused:
		// A paused guest's CPUs can't handle the ACPI request.
		if err := Resume(domainName); err != nil {
			return "", err
		}
		return requestShutdownAndWait(domainName, timeout)
	case StateShuttingDown, StateDying:
		return waitForPowerOff(domainName, timeout)
	default:
		return requestShutdownAndWait(domainName, timeout)
	}
}

// requestShutdownAndWait sends the ACPI shutdown request and waits for the
// guest to power off, forcing it off if the request fails.
func requestShutdownAndWait(domainName string, timeout time.Duration) (StopOutcome, error) {
	if err := Shutdown(domainName); err != nil {
		ui.Printf("Warning: %v — forcing it off\n", err)
		return StopForcedAfterTimeout, forceStopIfActive(domainName)
	}
	return waitForPowerOff(domainName, timeout)
}

// waitForPowerOff polls the domain state with backoff until the guest is off,
// and forces it off once timeout passes.
func waitForPowerOff(domainName string, timeout time.Duration) (StopOutcome, error) {
	deadline := time.Now().Add(timeout)
	pollInterval := shutdownInitialPollInterval
	for {
		state, err := State(domainName)
		if err != nil {
			return "", err
		}
		if !isPoweredOn(state) {
			return StopGraceful, forceStopIfActive(domainName)
		}
		if !time.Now().Before(deadline) {
			return StopForcedAfterTimeout, forceStopIfActive(domainName)
		}
		time.Sleep(min(pollInterval, time.Until(deadline)))
		pollInterval = min(pollInterval*2, shutdownMaxPollInterval)
	}
}

// forceStopIfActive powers the domain off if it still has a QEMU process.
// It checks 'virsh list' rather than the state, because a crashed domain can
// be either active or inactive.
func forceStopIfActive(domainName string) error {
	active, err := isDomainListed(domainName, activeDomainsOnly)
	if err != nil {
		return fmt.Errorf("checking whether VM %s is still active: %w", domainName, err)
	}
	if !active {
		return nil
	}
	return ForceStop(domainName)
}

// ForceStop immediately powers off a VM ('virsh destroy'; the disk is kept).
func ForceStop(domainName string) error {
	if err := libvirt.Virsh("destroy", domainName); err != nil {
		return fmt.Errorf("cannot force-stop VM %s: %w — %s", domainName, err, libvirtUnreachableHint)
	}
	return nil
}

// Start boots a stopped VM.
func Start(domainName string) error {
	if err := libvirt.Virsh("start", domainName); err != nil {
		return fmt.Errorf("cannot start VM %s: %w", domainName, err)
	}
	return nil
}

// Shutdown sends a graceful shutdown signal to a VM.
func Shutdown(domainName string) error {
	if err := libvirt.Virsh("shutdown", domainName); err != nil {
		return fmt.Errorf("cannot shut down VM %s: %w", domainName, err)
	}
	return nil
}

// Resume continues a paused VM's CPUs ('virsh resume').
func Resume(domainName string) error {
	if err := libvirt.Virsh("resume", domainName); err != nil {
		return fmt.Errorf("cannot resume paused VM %s: %w — check it with 'sudo virsh dominfo %s'", domainName, err, domainName)
	}
	return nil
}

// WakeUp wakes a guest that suspended itself to RAM or disk ('virsh dompmwakeup').
func WakeUp(domainName string) error {
	if err := libvirt.Virsh("dompmwakeup", domainName); err != nil {
		return fmt.Errorf("cannot wake suspended VM %s: %w — 'vmherd restart' forces it off and starts it again", domainName, err)
	}
	return nil
}

// Destroy powers off the domain if it is active and undefines it. It does not
// remove the disk or seed ISO — call RemoveDisk and cloudinit.RemoveSeed
// separately, because these files are created outside libvirt's storage pool
// and virsh --remove-all-storage cannot manage them.
func Destroy(domainName string) error {
	if err := forceStopIfActive(domainName); err != nil {
		return fmt.Errorf("cannot destroy VM %s: %w", domainName, err)
	}
	if err := libvirt.Virsh("undefine", domainName); err != nil {
		return fmt.Errorf("cannot destroy VM %s: %w", domainName, err)
	}
	return nil
}
