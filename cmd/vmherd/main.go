package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var version = "dev"

func main() {
	rootCmd := &cobra.Command{
		Use:   "vmherd",
		Short: "Lightweight VM manager for Linux using libvirt/KVM and cloud images",
		Long: `vmherd manages local libvirt/KVM virtual machines using official cloud images
from distro vendors (Ubuntu, Debian, Fedora, Arch, etc.) with cloud-init provisioning.

Define your VMs in a Herdfile, then use 'vmherd up' to bring them to life.`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	rootCmd.AddCommand(
		newInitCmd(),
		newUpCmd(),
		newDownCmd(),
		newDestroyCmd(),
		newStatusCmd(),
		newSSHCmd(),
		newSCPCmd(),
		newListImagesCmd(),
		newPullCmd(),
		newVersionCmd(),
	)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
