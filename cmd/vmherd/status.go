package main

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/boyadzhievb/vmherd/internal/config"
	"github.com/boyadzhievb/vmherd/internal/vm"
	"github.com/spf13/cobra"
)

// newStatusCmd creates the 'status' command that shows VM states.
func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the state of all VMs defined in the Herdfile",
		RunE: func(cmd *cobra.Command, args []string) error {
			project, err := loadProjectWithDomains()
			if err != nil {
				return err
			}

			writer := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
			_, _ = fmt.Fprintln(writer, "NAME\tIMAGE\tIP\tSTATE")
			_, _ = fmt.Fprintln(writer, "----\t-----\t--\t-----")

			sortedVMs := slices.Clone(project.Herdfile.VMs)
			slices.SortFunc(sortedVMs, func(a, b config.VM) int {
				return strings.Compare(a.Name, b.Name)
			})

			for _, vmConfig := range sortedVMs {
				domainName := project.DomainName(vmConfig.Name)
				state, err := vm.State(domainName)
				if err != nil {
					return err
				}

				_, _ = fmt.Fprintf(writer, "%s\t%s\t%s\t%s\n",
					vmConfig.Name,
					vmConfig.Image,
					statusAddress(domainName, vmConfig, state),
					state,
				)
			}

			_ = writer.Flush()
			return nil
		},
	}
}

// statusAddress returns the IP column for 'vmherd status': the static ip, a
// running DHCP VM's lease, "dhcp (no lease yet)" while it boots, or "dhcp".
func statusAddress(domainName string, vmConfig config.VM, state vm.VMState) string {
	if !usesDHCP(vmConfig) {
		return vmConfig.Network.IP
	}
	if !state.IsRunning() {
		return "dhcp"
	}
	leasedAddress, err := vm.LeaseAddress(domainName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
		return "dhcp (unknown)"
	}
	if leasedAddress == "" {
		return "dhcp (no lease yet)"
	}
	return leasedAddress
}
