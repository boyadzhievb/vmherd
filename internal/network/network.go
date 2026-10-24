package network

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/boyadzhievb/vmherd/internal/ui"
)

// Ensure creates the libvirt network if it doesn't exist, starts it, and enables autostart.
func Ensure(stateDir, networkName, subnetBase string) error {
	if exists(networkName) {
		if !isActive(networkName) {
			if err := virsh("net-start", networkName); err != nil {
				return fmt.Errorf("cannot start network %s: %w", networkName, err)
			}
		}
		_ = virsh("net-autostart", networkName)
		return nil
	}

	ui.Printf("Creating libvirt network: %s (%s.0/24)\n", networkName, subnetBase)

	bridgeName := truncateBridgeName(networkName)

	netXML := fmt.Sprintf(`<network>
  <name>%s</name>
  <forward mode='nat'/>
  <bridge name='%s' stp='on' delay='0'/>
  <ip address='%s.1' netmask='255.255.255.0'>
    <dhcp>
      <range start='%s.100' end='%s.200'/>
    </dhcp>
  </ip>
</network>`, networkName, bridgeName, subnetBase, subnetBase, subnetBase)

	xmlDir := filepath.Join(stateDir, "network")
	if err := os.MkdirAll(xmlDir, 0o755); err != nil {
		return fmt.Errorf("cannot create state directory: %w", err)
	}

	xmlPath := filepath.Join(xmlDir, networkName+".xml")
	if err := os.WriteFile(xmlPath, []byte(netXML), 0o644); err != nil {
		return fmt.Errorf("cannot write network XML: %w", err)
	}

	if err := virsh("net-define", xmlPath); err != nil {
		if !exists(networkName) {
			return fmt.Errorf("cannot define network %s: %w — is libvirtd running?", networkName, err)
		}
	}
	if err := virsh("net-start", networkName); err != nil {
		if !isActive(networkName) {
			return fmt.Errorf("cannot start network %s: %w", networkName, err)
		}
	}
	_ = virsh("net-autostart", networkName)

	ui.Printf("Network %s created.\n", networkName)
	return nil
}

// Destroy removes a libvirt network.
func Destroy(networkName string) error {
	if !exists(networkName) {
		return nil
	}
	_ = virsh("net-destroy", networkName)
	if err := virsh("net-undefine", networkName); err != nil {
		return fmt.Errorf("cannot remove network %s: %w", networkName, err)
	}
	ui.Printf("Network %s removed.\n", networkName)
	return nil
}

func exists(networkName string) bool {
	out, err := exec.Command("sudo", "virsh", "net-list", "--all", "--name").Output()
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.TrimSpace(line) == networkName {
			return true
		}
	}
	return false
}

func isActive(networkName string) bool {
	out, err := exec.Command("sudo", "virsh", "net-list", "--name").Output()
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.TrimSpace(line) == networkName {
			return true
		}
	}
	return false
}

func virsh(args ...string) error {
	cmd := exec.Command("sudo", append([]string{"virsh"}, args...)...)
	cmd.Stdout = ui.Stdout()
	cmd.Stderr = ui.Stderr()
	return cmd.Run()
}

const maxBridgeNameLength = 15

// truncateBridgeName ensures the bridge name fits within the Linux 15-char limit.
func truncateBridgeName(networkName string) string {
	if len(networkName) <= maxBridgeNameLength {
		return networkName
	}
	return networkName[:maxBridgeNameLength]
}
