package cloudinit

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/boyadzhievb/vmherd/internal/ui"
)

// seedVolumeLabel is the volume label for the cloud-init NoCloud seed ISO.
const seedVolumeLabel = "cidata"

// BuildAndInstallSeed reads the three cloud-init files from ciFiles, packages
// them as a cidata ISO 9660 image, and installs it to seedPath via sudo.
func BuildAndInstallSeed(ciFiles *Files, seedPath string) error {
	userData, err := os.ReadFile(ciFiles.UserData)
	if err != nil {
		return fmt.Errorf("reading user-data for seed ISO: %w", err)
	}
	metaData, err := os.ReadFile(ciFiles.MetaData)
	if err != nil {
		return fmt.Errorf("reading meta-data for seed ISO: %w", err)
	}
	networkConfig, err := os.ReadFile(ciFiles.NetworkConfig)
	if err != nil {
		return fmt.Errorf("reading network-config for seed ISO: %w", err)
	}

	isoBytes, err := buildISO9660(seedVolumeLabel, []isoFile{
		{name: "meta-data", content: metaData},
		{name: "network-config", content: networkConfig},
		{name: "user-data", content: userData},
	})
	if err != nil {
		return fmt.Errorf("building seed ISO: %w", err)
	}

	tmpFile, err := os.CreateTemp("", "vmherd-seed-*.iso")
	if err != nil {
		return fmt.Errorf("creating temp file for seed ISO: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer func() {
		if removeErr := os.Remove(tmpPath); removeErr != nil && !os.IsNotExist(removeErr) {
			ui.Printf("Warning: could not remove temp seed file %s: %v\n", tmpPath, removeErr)
		}
	}()

	if _, err := tmpFile.Write(isoBytes); err != nil {
		if closeErr := tmpFile.Close(); closeErr != nil {
			ui.Printf("Warning: could not close temp seed file %s: %v\n", tmpPath, closeErr)
		}
		return fmt.Errorf("writing seed ISO to temp file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("closing seed ISO temp file: %w", err)
	}

	installCmd := exec.Command("sudo", "install", "-m", "0640", tmpPath, seedPath)
	installCmd.Stdout = ui.Stdout()
	installCmd.Stderr = ui.Stderr()
	if err := installCmd.Run(); err != nil {
		return fmt.Errorf("installing seed ISO to %s: %w", seedPath, err)
	}

	return nil
}

// RemoveSeed removes the seed ISO at seedPath via sudo. It is not an error if
// the file does not exist (idempotent for destroy/cleanup).
func RemoveSeed(seedPath string) error {
	cmd := exec.Command("sudo", "rm", "-f", seedPath)
	cmd.Stdout = ui.Stdout()
	cmd.Stderr = ui.Stderr()
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("removing seed ISO %s: %w", seedPath, err)
	}
	return nil
}
