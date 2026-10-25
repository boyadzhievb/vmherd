package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/boyadzhievb/vmherd/internal/cloudinit"
	"github.com/boyadzhievb/vmherd/internal/config"
	"github.com/boyadzhievb/vmherd/internal/image"
	"github.com/boyadzhievb/vmherd/internal/network"
	"github.com/boyadzhievb/vmherd/internal/ssh"
	"github.com/boyadzhievb/vmherd/internal/ui"
	"github.com/boyadzhievb/vmherd/internal/vm"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

// maxParallelVMCreations limits how many VMs are created concurrently via virt-install.
const maxParallelVMCreations = 4

// vmCreateResult tracks the outcome of a single VM creation attempt.
type vmCreateResult struct {
	vmConfig config.VM
	err      error
}

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

			activeVMs, err := bringUpVMs(project, vms, strings.TrimSpace(pubKey), subnetBase, dnsServer)
			if err != nil {
				return err
			}

			waitForSSHInParallel(activeVMs, keyPath, project.Herdfile.SSH.User)

			ui.Println("\nAll VMs ready:")
			for _, vmConfig := range activeVMs {
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

// bringUpVMs handles running/stopped VMs immediately, then creates missing VMs
// in parallel. Returns the list of VMs that are now active.
func bringUpVMs(project *config.Project, vms []config.VM, pubKey, subnetBase, dnsServer string) ([]config.VM, error) {
	var activeVMs []config.VM
	var vmsToCreate []config.VM

	for _, vmConfig := range vms {
		domainName := project.DomainName(vmConfig.Name)
		state := vm.State(domainName)

		if state == "running" {
			ui.Printf("%s is already running.\n", vmConfig.Name)
			setupPortForwarding(vmConfig)
			activeVMs = append(activeVMs, vmConfig)
			continue
		}

		if state == "shut off" {
			ui.Printf("Starting %s...\n", vmConfig.Name)
			if err := vm.Start(domainName); err != nil {
				return nil, err
			}
			setupPortForwarding(vmConfig)
			activeVMs = append(activeVMs, vmConfig)
			continue
		}

		vmsToCreate = append(vmsToCreate, vmConfig)
	}

	if len(vmsToCreate) == 0 {
		return activeVMs, nil
	}

	resolvedImages, err := resolveAndCacheImages(vmsToCreate)
	if err != nil {
		return nil, err
	}

	results := createVMsInParallel(project, vmsToCreate, resolvedImages, pubKey, subnetBase, dnsServer)

	failCount := 0
	for _, result := range results {
		if result.err != nil {
			fmt.Fprintf(os.Stderr, "Error creating %s: %v\n", result.vmConfig.Name, result.err)
			failCount++
		} else {
			setupPortForwarding(result.vmConfig)
			activeVMs = append(activeVMs, result.vmConfig)
		}
	}

	if failCount == len(vmsToCreate) {
		return nil, fmt.Errorf("all %d VM creations failed — see errors above", failCount)
	}
	if failCount > 0 {
		fmt.Fprintf(os.Stderr, "%d of %d VM creations failed\n", failCount, len(vmsToCreate))
	}

	return activeVMs, nil
}

// resolvedImage holds a pre-resolved and pre-downloaded image entry.
type resolvedImage struct {
	entry     *image.Entry
	cachePath string
}

// resolveAndCacheImages downloads all unique images needed by the VMs to create.
// Runs serially to avoid concurrent download races on the same image file.
func resolveAndCacheImages(vmsToCreate []config.VM) (map[string]resolvedImage, error) {
	resolved := make(map[string]resolvedImage)
	for _, vmConfig := range vmsToCreate {
		if _, ok := resolved[vmConfig.Image]; ok {
			continue
		}
		entry, err := image.Resolve(vmConfig.Image)
		if err != nil {
			return nil, err
		}
		cachePath, err := image.EnsureCached(entry)
		if err != nil {
			return nil, err
		}
		resolved[vmConfig.Image] = resolvedImage{entry: entry, cachePath: cachePath}
	}
	return resolved, nil
}

// createVMsInParallel creates multiple VMs concurrently with bounded concurrency.
// All VMs are attempted regardless of individual failures.
func createVMsInParallel(
	project *config.Project,
	vmsToCreate []config.VM,
	resolvedImages map[string]resolvedImage,
	pubKey, subnetBase, dnsServer string,
) []vmCreateResult {
	results := make([]vmCreateResult, len(vmsToCreate))
	group := new(errgroup.Group)
	group.SetLimit(maxParallelVMCreations)

	for i, vmConfig := range vmsToCreate {
		results[i].vmConfig = vmConfig

		group.Go(func() error {
			results[i].err = createSingleVM(
				project, vmConfig, resolvedImages[vmConfig.Image],
				pubKey, subnetBase, dnsServer,
			)
			return nil
		})
	}

	_ = group.Wait()
	return results
}

// createSingleVM generates cloud-init config and runs virt-install for one VM.
func createSingleVM(
	project *config.Project,
	vmConfig config.VM,
	img resolvedImage,
	pubKey, subnetBase, dnsServer string,
) error {
	domainName := project.DomainName(vmConfig.Name)

	ui.Printf("Creating %s (%s, %dMB, %d CPUs)...\n",
		vmConfig.Name, vmConfig.Image, vmConfig.Memory, vmConfig.CPUs)

	sharedMounts := buildSharedMounts(project.Dir, vmConfig)

	macAddress, err := vm.GenerateMACAddress()
	if err != nil {
		return err
	}

	ciFiles, err := cloudinit.GenerateFiles(
		project.StateDir, vmConfig,
		project.Herdfile.SSH.User, project.Herdfile.SSH.Password, pubKey,
		subnetBase, dnsServer, macAddress, buildCloudInitMounts(sharedMounts),
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
		MACAddress:         macAddress,
		Memory:             vmConfig.Memory,
		CPUs:               vmConfig.CPUs,
		OSVariant:          img.entry.OSVariant,
		ImagePath:          img.cachePath,
		DiskSize:           vmConfig.Disk,
		SharedFolders:      sharedMounts,
	}); err != nil {
		return err
	}

	ui.Printf("%s created.\n", vmConfig.Name)
	return nil
}

// waitForSSHInParallel waits for SSH readiness on all VMs concurrently
// and prints results after all checks complete.
func waitForSSHInParallel(vms []config.VM, keyPath, sshUser string) {
	var vmsWithIP []config.VM
	for _, vmConfig := range vms {
		if vmConfig.Network.IP != "" {
			vmsWithIP = append(vmsWithIP, vmConfig)
		}
	}
	if len(vmsWithIP) == 0 {
		return
	}

	ui.Println("\nWaiting for SSH on all VMs...")

	type sshResult struct {
		vmConfig config.VM
		err      error
	}

	results := make([]sshResult, len(vmsWithIP))
	group := new(errgroup.Group)

	for i, vmConfig := range vmsWithIP {
		results[i].vmConfig = vmConfig

		group.Go(func() error {
			results[i].err = ssh.WaitForSSH(keyPath, sshUser, vmConfig.Network.IP, vmConfig.Name)
			return nil
		})
	}

	_ = group.Wait()

	for _, result := range results {
		if result.err != nil {
			ui.Printf("  %s (%s)... timeout\n", result.vmConfig.Name, result.vmConfig.Network.IP)
			fmt.Fprintf(os.Stderr, "Warning: %v\n", result.err)
		} else {
			ui.Printf("  %s (%s)... ready\n", result.vmConfig.Name, result.vmConfig.Network.IP)
		}
	}
}

// buildSharedMounts converts Herdfile shared folder config into vm.SharedFolderMount
// structs with resolved absolute host paths and generated mount tags.
func buildSharedMounts(projectDir string, vmConfig config.VM) []vm.SharedFolderMount {
	var mounts []vm.SharedFolderMount
	for i, sf := range vmConfig.SharedFolders {
		hostPath := sf.Host
		if !filepath.IsAbs(hostPath) {
			hostPath = filepath.Join(projectDir, hostPath)
		}
		mountTag := fmt.Sprintf("vmherd_share_%d", i)
		mounts = append(mounts, vm.SharedFolderMount{
			HostPath:  hostPath,
			GuestPath: sf.Guest,
			MountTag:  mountTag,
		})
	}
	return mounts
}

// buildCloudInitMounts converts vm.SharedFolderMount into cloudinit.SharedMount
// for cloud-init 9p mount configuration.
func buildCloudInitMounts(mounts []vm.SharedFolderMount) []cloudinit.SharedMount {
	var ciMounts []cloudinit.SharedMount
	for _, m := range mounts {
		ciMounts = append(ciMounts, cloudinit.SharedMount{
			MountTag:  m.MountTag,
			GuestPath: m.GuestPath,
		})
	}
	return ciMounts
}
