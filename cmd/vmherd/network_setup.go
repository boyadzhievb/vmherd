package main

import (
	"github.com/boyadzhievb/vmherd/internal/config"
	"github.com/boyadzhievb/vmherd/internal/network"
)

// ensureProjectNetwork makes the project's network available before VMs are
// created or started: the vmherd-managed libvirt NAT network is created and
// started; a host bridge is only checked, since vmherd never creates bridges.
func ensureProjectNetwork(project *config.Project) error {
	if project.Herdfile.Network.IsBridge() {
		return network.EnsureBridgeExists(project.Herdfile.Network.Bridge)
	}
	subnetBase, err := project.SubnetBase()
	if err != nil {
		return err
	}
	return network.Ensure(project.StateDir, project.Herdfile.Network.Name, subnetBase)
}
