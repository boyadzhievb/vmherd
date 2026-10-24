package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/boyadzhievb/vmherd/internal/cloudinit"
	"github.com/boyadzhievb/vmherd/internal/config"
	"github.com/boyadzhievb/vmherd/internal/image"
	"github.com/boyadzhievb/vmherd/internal/network"
	"github.com/boyadzhievb/vmherd/internal/ssh"
	"github.com/boyadzhievb/vmherd/internal/ui"
	"github.com/boyadzhievb/vmherd/internal/vm"
	"github.com/spf13/cobra"
)

func newUpCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "up [name...]",
		Short: "Create and start VMs (all or named)",
		RunE: func(cmd *cobra.Command, args []string) error {
			project, err := config.LoadProject()
			if err != nil {
				return err
			}

			vms, err := config.FilterVMs(project.Herdfile, args)
			if err != nil {
				return err
			}

			subnetBase, err := project.SubnetBase()
			if err != nil {
				return err
			}

			gatewayIP, err := project.GatewayIP()
			if err != nil {
				return err
			}

			dnsServer := vm.ResolveDNS(project.Herdfile.Network.DNS, gatewayIP)

			keyPath, err := ssh.EnsureKeyPair(project.StateDir, "vmherd-"+project.Name)
			if err != nil {
				return err
			}

			pubKey, err := ssh.ReadPublicKey(keyPath)
			if err != nil {
				return err
			}

			if err := network.Ensure(project.StateDir, project.Herdfile.Network.Name, subnetBase); err != nil {
				return err
			}

			for _, vmConfig := range vms {
				domainName := project.DomainName(vmConfig.Name)
				state := vm.State(domainName)

				if state == "running" {
					ui.Printf("%s is already running.\n", vmConfig.Name)
					continue
				}

				if state == "shut off" {
					ui.Printf("Starting %s...\n", vmConfig.Name)
					if err := vm.Start(domainName); err != nil {
						return err
					}
					continue
				}

				entry, err := image.Resolve(vmConfig.Image)
				if err != nil {
					return err
				}

				imagePath, err := image.EnsureCached(entry)
				if err != nil {
					return err
				}

				ui.Printf("Creating %s (%s, %dMB, %d CPUs)...\n",
					vmConfig.Name, vmConfig.Image, vmConfig.Memory, vmConfig.CPUs)

				ciFiles, err := cloudinit.GenerateFiles(
					project.StateDir, vmConfig,
					project.Herdfile.SSH.User, strings.TrimSpace(pubKey),
					subnetBase, dnsServer,
				)
				if err != nil {
					return err
				}

				if err := vm.Create(vm.CreateOptions{
					DomainName:         domainName,
					DiskPath:           project.DiskPath(vmConfig.Name),
					CloudInitUserData:  ciFiles.UserData,
					CloudInitMetaData:  ciFiles.MetaData,
					CloudInitNetConfig: ciFiles.NetworkConfig,
					NetworkName:        project.Herdfile.Network.Name,
					Memory:             vmConfig.Memory,
					CPUs:               vmConfig.CPUs,
					OSVariant:          entry.OSVariant,
					ImagePath:          imagePath,
					DiskSize:           vmConfig.Disk,
				}); err != nil {
					return err
				}

				ui.Printf("%s created.\n", vmConfig.Name)
			}

			ui.Println("\nWaiting for SSH on all VMs...")
			for _, vmConfig := range vms {
				if vmConfig.Network.IP == "" {
					continue
				}
				ui.Printf("  %s (%s)...", vmConfig.Name, vmConfig.Network.IP)
				if err := ssh.WaitForSSH(keyPath, project.Herdfile.SSH.User, vmConfig.Network.IP, vmConfig.Name); err != nil {
					ui.Println(" timeout")
					fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
					continue
				}
				ui.Println(" ready")
			}

			ui.Println("\nAll VMs ready:")
			for _, vmConfig := range vms {
				if vmConfig.Network.IP != "" {
					ui.Printf("  %-20s %s   ssh -i %s %s@%s\n",
						vmConfig.Name, vmConfig.Network.IP, keyPath,
						project.Herdfile.SSH.User, vmConfig.Network.IP)
				}
			}

			return nil
		},
	}
}
