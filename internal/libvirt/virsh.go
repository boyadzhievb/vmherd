package libvirt

import (
	"os/exec"

	"github.com/boyadzhievb/vmherd/internal/ui"
)

// Virsh runs a virsh subcommand with sudo and streams output to the UI writers.
func Virsh(args ...string) error {
	cmd := exec.Command("sudo", append([]string{"virsh"}, args...)...)
	cmd.Stdout = ui.Stdout()
	cmd.Stderr = ui.Stderr()
	return cmd.Run()
}
