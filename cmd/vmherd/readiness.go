package main

import (
	"errors"
	"fmt"

	"github.com/boyadzhievb/vmherd/internal/config"
	"github.com/boyadzhievb/vmherd/internal/ssh"
	"github.com/boyadzhievb/vmherd/internal/ui"
	"golang.org/x/sync/errgroup"
)

// waitForVMsReady waits for each started VM's address and SSH, then (for the
// VMs in firstBootVMNames only) for first-boot provisioning, and starts port
// forwarding for every VM that answered SSH. It returns the VMs that are ready
// and the joined errors of every VM that didn't get there.
func waitForVMsReady(project *config.Project, vms []config.VM, keyPath string, firstBootVMNames map[string]bool) ([]config.VM, error) {
	addressedVMs, leaseErr := waitForAddresses(project, vms)
	reachableVMs, sshErr := waitForSSHInParallel(addressedVMs, keyPath, project.Herdfile.SSH.User)

	var firstBootVMs, readyVMs []config.VM
	for _, vmConfig := range reachableVMs {
		if firstBootVMNames[vmConfig.Name] {
			firstBootVMs = append(firstBootVMs, vmConfig)
		} else {
			readyVMs = append(readyVMs, vmConfig)
		}
	}
	provisionedVMs, provisionErr := waitForFirstBootProvisioning(firstBootVMs, keyPath, project.Herdfile.SSH.User)
	readyVMs = append(readyVMs, provisionedVMs...)

	// A VM whose provisioning failed still runs, so its ports are forwarded.
	portForwardErr := startPortForwardingForVMs(project, reachableVMs)
	return readyVMs, errors.Join(leaseErr, sshErr, provisionErr, portForwardErr)
}

// waitForSSHInParallel waits for SSH on all VMs concurrently and prints one
// line per VM once all checks complete. It returns the VMs that answered and
// the joined errors of those that timed out.
func waitForSSHInParallel(vms []config.VM, keyPath, sshUser string) ([]config.VM, error) {
	if len(vms) == 0 {
		return nil, nil
	}
	ui.Println("\nWaiting for SSH on all VMs...")

	sshErrors := make([]error, len(vms))
	group := new(errgroup.Group)
	for index, vmConfig := range vms {
		group.Go(func() error {
			sshErrors[index] = ssh.WaitForSSH(keyPath, sshUser, vmConfig.Network.IP, vmConfig.Name)
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil, fmt.Errorf("waiting for SSH: %w", err)
	}

	var readyVMs []config.VM
	for index, vmConfig := range vms {
		if sshErrors[index] != nil {
			ui.Printf("  %s (%s)... timeout\n", vmConfig.Name, vmConfig.Network.IP)
			sshErrors[index] = fmt.Errorf("%s: %w", vmConfig.Name, sshErrors[index])
			continue
		}
		ui.Printf("  %s (%s)... ready\n", vmConfig.Name, vmConfig.Network.IP)
		readyVMs = append(readyVMs, vmConfig)
	}
	return readyVMs, errors.Join(sshErrors...)
}

// incompleteCommandError turns the joined per-VM errors of a command that
// still processed every VM into its final error (exit code 1), one failure
// per line. It returns nil when nothing failed.
func incompleteCommandError(commandName string, vmFailures error) error {
	if vmFailures == nil {
		return nil
	}
	return fmt.Errorf("'vmherd %s' did not complete for every VM:\n%w", commandName, vmFailures)
}
