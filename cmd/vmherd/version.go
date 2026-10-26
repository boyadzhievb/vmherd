package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

// newVersionCmd creates the 'version' command that shows the build version.
func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show vmherd version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("vmherd %s\n", version)
		},
	}
}
