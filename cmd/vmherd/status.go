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
			project, err := config.LoadProject()
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
				state := vm.State(domainName)

				_, _ = fmt.Fprintf(writer, "%s\t%s\t%s\t%s\n",
					vmConfig.Name,
					vmConfig.Image,
					vmConfig.Network.IP,
					state,
				)
			}

			_ = writer.Flush()
			return nil
		},
	}
}
