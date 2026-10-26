package main

import (
	"fmt"
	"strings"

	"github.com/boyadzhievb/vmherd/internal/config"
	"github.com/boyadzhievb/vmherd/internal/ssh"
	"github.com/boyadzhievb/vmherd/internal/vm"
	"github.com/spf13/cobra"
)

// newSCPCmd creates the 'scp' command for copying files to and from VMs.
func newSCPCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "scp <source> <destination>",
		Short: "Copy files between local host and VMs (name:/path syntax)",
		Long: `Copy files using scp. Use name:/path to refer to a VM path.

Examples:
  vmherd scp ./local.txt ctrl:/tmp/remote.txt
  vmherd scp ctrl:/var/log/syslog ./syslog.txt`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			project, err := config.LoadProject()
			if err != nil {
				return err
			}

			vmByName := make(map[string]config.VM)
			for _, vm := range project.Herdfile.VMs {
				vmByName[vm.Name] = vm
			}

			scpArgs := make([]string, 2)
			for i, arg := range args {
				resolved, err := resolveScpPath(arg, vmByName, project.Herdfile.SSH.User)
				if err != nil {
					return err
				}
				scpArgs[i] = resolved
			}

			for _, vmConfig := range project.Herdfile.VMs {
				for _, arg := range args {
					colonIdx := strings.Index(arg, ":")
					if colonIdx > 0 && arg[:colonIdx] == vmConfig.Name {
						domainName := project.DomainName(vmConfig.Name)
						if vm.State(domainName) != vm.StateRunning {
							return fmt.Errorf("VM %s is not running — start it with 'vmherd up %s'", vmConfig.Name, vmConfig.Name)
						}
					}
				}
			}

			return ssh.CopyFiles(project.SSHKeyPath(), project.Herdfile.SSH.User, scpArgs)
		},
	}
}

// resolveScpPath converts "vmname:/path" to "user@ip:/path".
func resolveScpPath(path string, vmByName map[string]config.VM, sshUser string) (string, error) {
	colonIdx := strings.Index(path, ":")
	if colonIdx < 0 {
		return path, nil
	}

	prefix := path[:colonIdx]
	suffix := path[colonIdx:]

	vmConfig, ok := vmByName[prefix]
	if !ok {
		if strings.Contains(prefix, "/") || strings.Contains(prefix, ".") {
			return path, nil
		}
		available := make([]string, 0, len(vmByName))
		for name := range vmByName {
			available = append(available, name)
		}
		return "", fmt.Errorf("unknown VM %q — available: %s", prefix, strings.Join(available, ", "))
	}

	if vmConfig.Network.IP == "" {
		return "", fmt.Errorf("VM %q has no IP address configured", prefix)
	}

	return fmt.Sprintf("%s@%s%s", sshUser, vmConfig.Network.IP, suffix), nil
}
