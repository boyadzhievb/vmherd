package main

import (
	"fmt"
	"slices"

	"github.com/boyadzhievb/vmherd/internal/cloudinit"
	"github.com/boyadzhievb/vmherd/internal/config"
	"github.com/boyadzhievb/vmherd/internal/ui"
	"github.com/boyadzhievb/vmherd/internal/vm"
)

// loadProjectWithDomains loads the project for commands that act on libvirt
// domains, and keeps the pre-M0.8 domain name for VMs this project created
// under it, so upgrading vmherd doesn't orphan existing VMs.
func loadProjectWithDomains() (*config.Project, error) {
	project, err := config.LoadProject()
	if err != nil {
		return nil, err
	}
	for _, vmConfig := range project.Herdfile.VMs {
		if err := adoptLegacyDomainName(project, vmConfig.Name); err != nil {
			return nil, err
		}
	}
	return project, nil
}

// adoptLegacyDomainName switches vmName to its old-style domain name when the
// new name doesn't exist and the old domain is this project's: its MAC matches
// the one saved in this project's state dir. Old names aren't unique across
// projects, so a domain with any other MAC belongs to someone else.
func adoptLegacyDomainName(project *config.Project, vmName string) error {
	savedMACAddress, err := cloudinit.ReadSavedMACAddress(project.StateDir, vmName)
	if err != nil {
		return fmt.Errorf("checking for an old-style domain of VM %s: %w", vmName, err)
	}
	if savedMACAddress == "" {
		return nil
	}

	legacyDomainName := project.LegacyDomainName(vmName)
	if legacyDomainName == project.DomainName(vmName) {
		return nil
	}
	currentNameState, err := vm.State(project.DomainName(vmName))
	if err != nil {
		return fmt.Errorf("looking up VM %s: %w", vmName, err)
	}
	if currentNameState != vm.StateNotCreated {
		return nil
	}
	legacyNameState, err := vm.State(legacyDomainName)
	if err != nil {
		return fmt.Errorf("checking for an old-style domain of VM %s: %w", vmName, err)
	}
	if legacyNameState == vm.StateNotCreated {
		return nil
	}

	domainMACAddresses, err := vm.MACAddresses(legacyDomainName)
	if err != nil {
		return fmt.Errorf("checking whether domain %s belongs to this project: %w", legacyDomainName, err)
	}
	if !slices.Contains(domainMACAddresses, savedMACAddress) {
		ui.Printf("Note: libvirt domain %s has VM %s's old-style name but was not created by this project (MAC differs); leaving it alone and using %s.\n",
			legacyDomainName, vmName, project.DomainName(vmName))
		return nil
	}

	project.UseLegacyDomainName(vmName)
	return nil
}
