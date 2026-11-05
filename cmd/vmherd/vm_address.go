package main

import (
	"fmt"
	"slices"
	"time"

	"github.com/boyadzhievb/vmherd/internal/config"
	"github.com/boyadzhievb/vmherd/internal/ui"
	"github.com/boyadzhievb/vmherd/internal/vm"
	"golang.org/x/sync/errgroup"
)

const (
	// leaseWaitTimeout is how long up, reload and restart wait for a DHCP VM to get a lease.
	leaseWaitTimeout = 120 * time.Second
	// leaseInitialPollInterval is the first wait between DHCP lease checks.
	leaseInitialPollInterval = 1 * time.Second
	// leaseMaxPollInterval caps the exponential backoff between DHCP lease checks.
	leaseMaxPollInterval = 8 * time.Second
)

// usesDHCP reports whether a VM gets its address from the libvirt network's
// DHCP server, which is the case when the Herdfile gives it no network.ip.
func usesDHCP(vmConfig config.VM) bool {
	return vmConfig.Network.IP == ""
}

// withCurrentAddress returns a copy of vmConfig whose Network.IP is the
// address to connect to: the static ip, or the VM's current DHCP lease.
// A DHCP VM without a lease yet is an error with a recovery hint.
func withCurrentAddress(project *config.Project, vmConfig config.VM) (config.VM, error) {
	if !usesDHCP(vmConfig) {
		return vmConfig, nil
	}
	leasedAddress, err := vm.LeaseAddress(project.DomainName(vmConfig.Name))
	if err != nil {
		return vmConfig, fmt.Errorf("finding the address of VM %s: %w", vmConfig.Name, err)
	}
	if leasedAddress == "" {
		return vmConfig, fmt.Errorf("VM %s gets its address from DHCP and has no lease yet — it may still be booting; wait a moment or run 'vmherd up %s'",
			vmConfig.Name, vmConfig.Name)
	}
	vmConfig.Network.IP = leasedAddress
	return vmConfig, nil
}

// waitForAddresses returns copies of vms with Network.IP set to the address
// to connect to, waiting in parallel (each VM with its own deadline) for the
// DHCP leases of booting VMs. A DHCP VM that gets no lease keeps no address,
// so later SSH steps skip it, and a warning names it.
func waitForAddresses(project *config.Project, vms []config.VM) []config.VM {
	resolvedVMs := slices.Clone(vms)
	leaseErrors := make([]error, len(vms))
	group := new(errgroup.Group)
	waitingForLease := false
	for index, vmConfig := range vms {
		if !usesDHCP(vmConfig) {
			continue
		}
		waitingForLease = true
		group.Go(func() error {
			resolvedVMs[index].Network.IP, leaseErrors[index] = waitForLease(project.DomainName(vmConfig.Name))
			return nil
		})
	}
	if !waitingForLease {
		return resolvedVMs
	}

	ui.Println("\nWaiting for DHCP leases...")
	if err := group.Wait(); err != nil {
		ui.Printf("Warning: waiting for DHCP leases: %v\n", err)
	}
	for index, vmConfig := range vms {
		if !usesDHCP(vmConfig) {
			continue
		}
		if leaseErrors[index] != nil {
			ui.Printf("  %s... no lease\n", vmConfig.Name)
			ui.Printf("Warning: VM %s: %v\n", vmConfig.Name, leaseErrors[index])
			continue
		}
		ui.Printf("  %s... %s\n", vmConfig.Name, resolvedVMs[index].Network.IP)
	}
	return resolvedVMs
}

// waitForLease polls the domain's DHCP lease with exponential backoff for up
// to leaseWaitTimeout. Lookup errors are retried (libvirt may be busy) and
// the last one is reported if no lease appears.
func waitForLease(domainName string) (string, error) {
	deadline := time.Now().Add(leaseWaitTimeout)
	pollInterval := leaseInitialPollInterval
	var lastLookupErr error
	for {
		leasedAddress, err := vm.LeaseAddress(domainName)
		if err == nil && leasedAddress != "" {
			return leasedAddress, nil
		}
		lastLookupErr = err
		if time.Now().Add(pollInterval).After(deadline) {
			break
		}
		time.Sleep(pollInterval)
		pollInterval = min(pollInterval*2, leaseMaxPollInterval)
	}
	if lastLookupErr != nil {
		return "", fmt.Errorf("no DHCP lease within %s: %w", leaseWaitTimeout, lastLookupErr)
	}
	return "", fmt.Errorf("no DHCP lease within %s — check the VM's console ('virsh console %s') for network errors", leaseWaitTimeout, domainName)
}
