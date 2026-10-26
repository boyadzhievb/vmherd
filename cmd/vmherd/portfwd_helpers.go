package main

import (
	"github.com/boyadzhievb/vmherd/internal/config"
	"github.com/boyadzhievb/vmherd/internal/portfwd"
	"github.com/boyadzhievb/vmherd/internal/ui"
)

// hasPortForwards returns true if any VM in the list has port forward rules configured.
func hasPortForwards(vms []config.VM) bool {
	for _, vmConfig := range vms {
		if len(vmConfig.Ports) > 0 && vmConfig.Network.IP != "" {
			return true
		}
	}
	return false
}

// setupPortForwarding removes any stale rules then installs iptables DNAT rules for a VM's port forwards.
func setupPortForwarding(vmConfig config.VM) {
	if len(vmConfig.Ports) == 0 || vmConfig.Network.IP == "" {
		return
	}
	rules := buildPortForwardRules(vmConfig)
	portfwd.Teardown(rules)
	if err := portfwd.Setup(rules); err != nil {
		ui.Printf("Warning: port forwarding setup failed for %s: %v\n", vmConfig.Name, err)
	}
}

// teardownPortForwarding removes iptables DNAT rules for a VM's port forwards.
func teardownPortForwarding(vmConfig config.VM) {
	if len(vmConfig.Ports) == 0 || vmConfig.Network.IP == "" {
		return
	}
	portfwd.Teardown(buildPortForwardRules(vmConfig))
}

// buildPortForwardRules converts Herdfile port forward config into portfwd.Rule slice.
func buildPortForwardRules(vmConfig config.VM) []portfwd.Rule {
	rules := make([]portfwd.Rule, len(vmConfig.Ports))
	for i, pf := range vmConfig.Ports {
		rules[i] = portfwd.Rule{
			HostPort:  pf.Host,
			GuestIP:   vmConfig.Network.IP,
			GuestPort: pf.Guest,
			Protocol:  pf.Protocol,
		}
	}
	return rules
}
