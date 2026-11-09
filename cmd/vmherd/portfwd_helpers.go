package main

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/boyadzhievb/vmherd/internal/config"
	"github.com/boyadzhievb/vmherd/internal/portfwd"
	"github.com/boyadzhievb/vmherd/internal/ssh"
)

// startPortForwardingForVMs starts (or keeps, or updates) the SSH port-forward
// tunnel of each VM. Call it after SSH is ready. Every VM is attempted; the
// failures are returned joined.
func startPortForwardingForVMs(project *config.Project, vms []config.VM) error {
	var portForwardErrors []error
	for _, vmConfig := range vms {
		if vmConfig.Network.IP == "" {
			continue
		}
		tunnel := portForwardTunnel(project, vmConfig)
		if err := portfwd.RemoveLegacyRules(tunnel.GuestIP, tunnel.Forwards); err != nil {
			portForwardErrors = append(portForwardErrors, fmt.Errorf("%s: %w", vmConfig.Name, err))
		}
		if err := portfwd.Start(tunnel); err != nil {
			portForwardErrors = append(portForwardErrors, fmt.Errorf("%s: port forwarding failed: %w", vmConfig.Name, err))
		}
	}
	return errors.Join(portForwardErrors...)
}

// stopPortForwarding stops a VM's SSH port-forward tunnel, if one runs, and
// removes iptables rules vmherd v0.1.x left for it (v0.1.x VMs always have a
// static ip, so the Herdfile value is the right one here). Both steps are
// always attempted; errors don't name the VM, callers add it.
func stopPortForwarding(project *config.Project, vmConfig config.VM) error {
	var stopErrors []error
	if err := portfwd.Stop(vmStateDir(project, vmConfig)); err != nil {
		stopErrors = append(stopErrors, fmt.Errorf("could not stop port forwarding: %w", err))
	}
	tunnel := portForwardTunnel(project, vmConfig)
	if err := portfwd.RemoveLegacyRules(tunnel.GuestIP, tunnel.Forwards); err != nil {
		stopErrors = append(stopErrors, err)
	}
	return errors.Join(stopErrors...)
}

// portForwardTunnel describes the SSH tunnel carrying a VM's Herdfile ports.
func portForwardTunnel(project *config.Project, vmConfig config.VM) portfwd.Tunnel {
	forwards := make([]ssh.LocalForward, len(vmConfig.Ports))
	for index, portForward := range vmConfig.Ports {
		forwards[index] = ssh.LocalForward{HostIP: portForward.HostIP, HostPort: portForward.Host, GuestPort: portForward.Guest}
	}
	return portfwd.Tunnel{
		VMName:     vmConfig.Name,
		VMStateDir: vmStateDir(project, vmConfig),
		KeyPath:    project.SSHKeyPath(),
		User:       project.Herdfile.SSH.User,
		GuestIP:    vmConfig.Network.IP,
		Forwards:   forwards,
	}
}

// vmStateDir returns the per-VM state directory (cloud-init seed, tunnel record).
func vmStateDir(project *config.Project, vmConfig config.VM) string {
	return filepath.Join(project.StateDir, vmConfig.Name)
}
