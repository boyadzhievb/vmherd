package main

import (
	"path/filepath"

	"github.com/boyadzhievb/vmherd/internal/config"
	"github.com/boyadzhievb/vmherd/internal/portfwd"
	"github.com/boyadzhievb/vmherd/internal/ssh"
	"github.com/boyadzhievb/vmherd/internal/ui"
)

// startPortForwardingForVMs starts (or keeps, or updates) the SSH port-forward
// tunnel of each VM. Call it after SSH is ready. Failures are warnings.
func startPortForwardingForVMs(project *config.Project, vms []config.VM) {
	for _, vmConfig := range vms {
		if vmConfig.Network.IP == "" {
			continue
		}
		if err := portfwd.Start(portForwardTunnel(project, vmConfig)); err != nil {
			ui.Printf("Warning: port forwarding for %s failed: %v\n", vmConfig.Name, err)
		}
	}
}

// stopPortForwarding stops a VM's SSH port-forward tunnel, if one runs.
func stopPortForwarding(project *config.Project, vmConfig config.VM) {
	if err := portfwd.Stop(vmStateDir(project, vmConfig)); err != nil {
		ui.Printf("Warning: could not stop port forwarding for %s: %v\n", vmConfig.Name, err)
	}
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
