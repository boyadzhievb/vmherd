package main

import (
	"errors"
	"fmt"

	"github.com/boyadzhievb/vmherd/internal/config"
	"github.com/boyadzhievb/vmherd/internal/ui"
	"github.com/boyadzhievb/vmherd/internal/vm"
	"golang.org/x/sync/errgroup"
)

// stopVMsGracefully stops running VMs in parallel: port forwarding first, then
// an ACPI shutdown that is forced only after vm.GracefulShutdownTimeout.
// It returns the joined errors of VMs that could not be stopped.
func stopVMsGracefully(project *config.Project, vms []config.VM) error {
	stopErrors := make([]error, len(vms))
	group := new(errgroup.Group)
	for index, vmConfig := range vms {
		ui.Printf("Stopping %s...\n", vmConfig.Name)
		stopPortForwarding(project, vmConfig)
		group.Go(func() error {
			forcedOff, err := vm.StopGracefully(project.DomainName(vmConfig.Name), vm.GracefulShutdownTimeout)
			if err != nil {
				stopErrors[index] = fmt.Errorf("stopping %s: %w", vmConfig.Name, err)
				return nil
			}
			if forcedOff {
				ui.Printf("%s did not shut down within %s and was forced off.\n", vmConfig.Name, vm.GracefulShutdownTimeout)
			}
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return fmt.Errorf("stopping VMs: %w", err)
	}
	return errors.Join(stopErrors...)
}
