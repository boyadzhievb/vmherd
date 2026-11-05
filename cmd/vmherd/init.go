package main

import (
	"fmt"
	"os"

	"github.com/boyadzhievb/vmherd/internal/config"
	"github.com/boyadzhievb/vmherd/internal/network"
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

			exampleContent, err := buildExampleHerdfile()
			if err != nil {
				return err
			}
			if err := os.WriteFile(config.HerdfileName, []byte(exampleContent), 0o644); err != nil {
				return fmt.Errorf("cannot write %s: %w", config.HerdfileName, err)
			}

			ui.Printf("Created %s — edit it, then run 'vmherd up'\n", config.HerdfileName)
			return nil
		},
	}
}

// fallbackExampleSubnetBase is used by 'vmherd init' when libvirt can't be
// queried for a free subnet; init warns so the user can check it.
const fallbackExampleSubnetBase = "192.168.150"

// buildExampleHerdfile renders the example Herdfile with this project's own
// network name and the first subnet no libvirt network or host route uses.
func buildExampleHerdfile() (string, error) {
	projectDir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("cannot determine working directory: %w", err)
	}
	networkName, err := config.DefaultNetworkName(projectDir)
	if err != nil {
		return "", fmt.Errorf("choosing a network name for %s: %w", projectDir, err)
	}

	subnetBase, err := network.FindFreeSubnet()
	if err != nil {
		ui.Printf("Warning: %v; using %s.0/24 — check it doesn't clash with your networks ('virsh net-list --all', 'ip route')\n",
			err, fallbackExampleSubnetBase)
		subnetBase = fallbackExampleSubnetBase
	}
	ui.Printf("Using network %s with subnet %s.0/24\n", networkName, subnetBase)

	return config.ExampleHerdfile(networkName, subnetBase)
}
