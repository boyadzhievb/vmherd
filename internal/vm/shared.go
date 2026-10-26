package vm

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/boyadzhievb/vmherd/internal/ui"
)

// ensureSharedFolderAccess grants the QEMU process user write access to a shared
// folder's host directory. Without this, 9p with accessmode=mapped fails because
// QEMU (running as libvirt-qemu/qemu) cannot create files in user-owned directories.
func ensureSharedFolderAccess(hostPath string) error {
	qemuUser := detectQEMUUser()

	aclArgs := fmt.Sprintf("u:%s:rwx", qemuUser)
	defaultACLArgs := fmt.Sprintf("d:u:%s:rwx", qemuUser)
	setfacl := exec.Command("sudo", "setfacl", "-m", aclArgs, "-m", defaultACLArgs, hostPath)
	if err := setfacl.Run(); err == nil {
		return nil
	}

	ui.Printf("Warning: setfacl unavailable for %s, falling back to chmod o+rwx — install the acl package for tighter permissions\n", hostPath)

	chmod := exec.Command("sudo", "chmod", "o+rwx", hostPath)
	if err := chmod.Run(); err != nil {
		return fmt.Errorf("cannot grant QEMU write access to shared folder %s: setfacl and chmod both failed — "+
			"run 'sudo setfacl -m u:%s:rwx %s' or 'sudo chmod o+rwx %s' manually",
			hostPath, qemuUser, hostPath, hostPath)
	}
	return nil
}

// parseQEMUUserFromConfig extracts the QEMU user from the content of qemu.conf.
// Returns an empty string if no uncommented user directive is found.
func parseQEMUUserFromConfig(content string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") || !strings.HasPrefix(line, "user") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			user := strings.TrimSpace(parts[1])
			user = strings.Trim(user, `"'`)
			if user != "" {
				return user
			}
		}
	}
	return ""
}

// detectQEMUUser returns the system user that runs QEMU processes.
// Checks /etc/libvirt/qemu.conf first, then falls back to probing common user names.
func detectQEMUUser() string {
	data, err := os.ReadFile("/etc/libvirt/qemu.conf")
	if err == nil {
		if user := parseQEMUUserFromConfig(string(data)); user != "" {
			return user
		}
	}

	for _, candidate := range []string{"libvirt-qemu", "qemu"} {
		if exec.Command("id", "-u", candidate).Run() == nil {
			return candidate
		}
	}

	return "libvirt-qemu"
}
