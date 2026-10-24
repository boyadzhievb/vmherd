package main

import (
	"fmt"
	"os"

	"github.com/boyadzhievb/vmherd/internal/ui"
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
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			q, _ := cmd.Flags().GetBool("quiet")
			ui.SetQuiet(q)
		},
	}

	rootCmd.PersistentFlags().BoolP("quiet", "q", false, "suppress status output and progress bars")

	rootCmd.AddCommand(
		newInitCmd(),
		newUpCmd(),
		newDownCmd(),
		newReloadCmd(),
		newDestroyCmd(),
		newProvisionCmd(),
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
