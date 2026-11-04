package vm

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/boyadzhievb/vmherd/internal/ui"
)

// qemuFolderACL is the ACL entry granting the QEMU user full access to a shared folder.
const qemuFolderACL = "rwx"

// qemuTraverseACL is the ACL entry granting the QEMU user traverse-only access
// (no listing, no reading) to a parent of a shared folder.
const qemuTraverseACL = "x"

// qemuIdentity names the QEMU process user in the forms different tools expect.
type qemuIdentity struct {
	displayName string // name as written in qemu.conf, for messages
	aclName     string // name or numeric UID for setfacl (u:<aclName>:rwx)
	sudoName    string // name or #UID for 'sudo -u'
}

// newQEMUIdentity builds a qemuIdentity from a qemu.conf user value, which is
// either a user name or a numeric UID written as "+107".
func newQEMUIdentity(qemuUser string) qemuIdentity {
	if numericUID, isNumeric := strings.CutPrefix(qemuUser, "+"); isNumeric {
		return qemuIdentity{displayName: qemuUser, aclName: numericUID, sudoName: "#" + numericUID}
	}
	return qemuIdentity{displayName: qemuUser, aclName: qemuUser, sudoName: qemuUser}
}

// ensureSharedFolderAccess grants the QEMU process user rwx on a shared folder
// via ACLs and traverse-only access on any parent directory that blocks it.
// Never widens permissions for "other"; on failure returns the exact command to run.
func ensureSharedFolderAccess(hostPath string) error {
	qemuUser := newQEMUIdentity(detectQEMUUser())
	if _, err := exec.LookPath("setfacl"); err != nil {
		return fmt.Errorf("cannot grant QEMU access to shared folder %s: setfacl is not installed — "+
			"install the 'acl' package (apt install acl / dnf install acl), then re-run", hostPath)
	}
	folderACL := fmt.Sprintf("u:%s:%s", qemuUser.aclName, qemuFolderACL)
	defaultFolderACL := "d:" + folderACL
	if err := runSetfacl(hostPath, folderACL, defaultFolderACL); err != nil {
		return fmt.Errorf("granting QEMU user %s access to shared folder %s: %w — the filesystem may not support ACLs; "+
			"run 'sudo setfacl -m %s -m %s %s' manually or move the share to a filesystem that does",
			qemuUser.displayName, hostPath, err, folderACL, defaultFolderACL, hostPath)
	}
	if err := runAsQEMUUser(qemuUser, "test", "-x", "/"); err != nil {
		return fmt.Errorf("cannot run commands as QEMU user %s to check shared folder %s: %w — "+
			"check the 'user' setting in /etc/libvirt/qemu.conf", qemuUser.displayName, hostPath, err)
	}

	canTraverse := func(dirPath string) bool { return runAsQEMUUser(qemuUser, "test", "-x", dirPath) == nil }
	grantTraverse := func(dirPath string) error {
		traverseACL := fmt.Sprintf("u:%s:%s", qemuUser.aclName, qemuTraverseACL)
		if err := runSetfacl(dirPath, traverseACL); err != nil {
			return fmt.Errorf("granting QEMU user %s traverse access to %s: %w — run 'sudo setfacl -m %s %s' manually",
				qemuUser.displayName, dirPath, err, traverseACL, dirPath)
		}
		ui.Printf("Granted %s traverse-only access (x) to %s so QEMU can reach shared folder %s\n",
			qemuUser.displayName, dirPath, hostPath)
		return nil
	}
	if err := grantParentTraversal(hostPath, canTraverse, grantTraverse); err != nil {
		return err
	}

	if err := runAsQEMUUser(qemuUser, "test", "-r", hostPath, "-a", "-w", hostPath, "-a", "-x", hostPath); err != nil {
		return fmt.Errorf("QEMU user %s still cannot read and write shared folder %s after setting ACLs — "+
			"check with 'sudo -u %s test -w %s && echo ok' and 'getfacl %s'",
			qemuUser.displayName, hostPath, qemuUser.sudoName, hostPath, hostPath)
	}
	return nil
}

// parentDirsTopDown returns every ancestor of absolutePath from "/" down to
// its direct parent, excluding absolutePath itself.
func parentDirsTopDown(absolutePath string) []string {
	var parentDirs []string
	for currentDir := filepath.Dir(filepath.Clean(absolutePath)); ; currentDir = filepath.Dir(currentDir) {
		parentDirs = append(parentDirs, currentDir)
		if currentDir == filepath.Dir(currentDir) {
			break
		}
	}
	slices.Reverse(parentDirs)
	return parentDirs
}

// grantParentTraversal walks the parents of hostPath from "/" downwards and
// calls grantTraverse on each one canTraverse reports as blocked. Top-down
// order matters: a directory can only be probed once its parent is passable.
func grantParentTraversal(hostPath string, canTraverse func(string) bool, grantTraverse func(string) error) error {
	for _, parentDir := range parentDirsTopDown(hostPath) {
		if canTraverse(parentDir) {
			continue
		}
		if err := grantTraverse(parentDir); err != nil {
			return err
		}
	}
	return nil
}

// runSetfacl applies one or more ACL entries to targetPath with sudo and
// returns setfacl's own error output on failure.
func runSetfacl(targetPath string, aclEntries ...string) error {
	setfaclArgs := []string{"setfacl"}
	for _, aclEntry := range aclEntries {
		setfaclArgs = append(setfaclArgs, "-m", aclEntry)
	}
	setfaclArgs = append(setfaclArgs, targetPath)
	if output, err := exec.Command("sudo", setfaclArgs...).CombinedOutput(); err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

// runAsQEMUUser runs a command as the QEMU process user via 'sudo -u', so
// permission checks honour ACLs and group membership exactly as QEMU sees them.
func runAsQEMUUser(qemuUser qemuIdentity, command ...string) error {
	sudoArgs := append([]string{"-u", qemuUser.sudoName}, command...)
	output, err := exec.Command("sudo", sudoArgs...).CombinedOutput()
	if err == nil {
		return nil
	}
	if trimmedOutput := strings.TrimSpace(string(output)); trimmedOutput != "" {
		return fmt.Errorf("running %q as %s: %w: %s", strings.Join(command, " "), qemuUser.displayName, err, trimmedOutput)
	}
	return fmt.Errorf("running %q as %s: %w", strings.Join(command, " "), qemuUser.displayName, err)
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
