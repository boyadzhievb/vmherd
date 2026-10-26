package main

import (
	"fmt"
	"os"

	"github.com/boyadzhievb/vmherd/internal/config"
	"github.com/boyadzhievb/vmherd/internal/ui"
	"github.com/spf13/cobra"
)

// newInitCmd creates the 'init' command that generates an example Herdfile.
func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Create an example Herdfile in the current directory",
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := os.Stat(config.HerdfileName); err == nil {
				return fmt.Errorf("%s already exists in this directory", config.HerdfileName)
			}

			if err := os.WriteFile(config.HerdfileName, []byte(config.ExampleHerdfile()), 0o644); err != nil {
				return fmt.Errorf("cannot write %s: %w", config.HerdfileName, err)
			}

			ui.Printf("Created %s — edit it, then run 'vmherd up'\n", config.HerdfileName)
			return nil
		},
	}
}
