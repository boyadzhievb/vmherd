package main

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/boyadzhievb/vmherd/internal/config"
	"github.com/boyadzhievb/vmherd/internal/vm"
	"github.com/spf13/cobra"
)

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

			for _, vmConfig := range project.Herdfile.VMs {
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
