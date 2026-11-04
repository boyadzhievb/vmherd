package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// stateDirMarkerFile marks a directory as a vmherd state directory. Removal
// helpers refuse to touch any directory that does not contain it.
const stateDirMarkerFile = ".vmherd-state"

// stateDirMarkerContent is written into the marker file to explain its purpose.
const stateDirMarkerContent = "This directory is managed by vmherd. 'vmherd destroy' removes it.\n"

// stateDirPermissions is the mode used when vmherd creates a state directory.
const stateDirPermissions = 0o700

// networkStateSubdir holds generated libvirt network XML inside a state directory.
const networkStateSubdir = "network"

// maxForeignEntriesShown caps how many unexpected entries an error message lists.
const maxForeignEntriesShown = 5

// knownStateFiles lists the regular files vmherd writes at the top level of a
// state directory. Read-only lookup table; never modified at runtime.
var knownStateFiles = map[string]bool{
	"id_ed25519":       true,
	"id_ed25519.pub":   true,
	stateDirMarkerFile: true,
}

// ensureStateDirMarker creates stateDir if needed and writes the marker file
// into it unless the marker already exists.
func ensureStateDirMarker(stateDir string) error {
	if err := os.MkdirAll(stateDir, stateDirPermissions); err != nil {
		return fmt.Errorf("creating state directory %s: %w", stateDir, err)
	}
	markerPath := filepath.Join(stateDir, stateDirMarkerFile)
	if _, err := os.Lstat(markerPath); err == nil {
		return nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("checking state marker %s: %w", markerPath, err)
	}
	if err := os.WriteFile(markerPath, []byte(stateDirMarkerContent), 0o600); err != nil {
		return fmt.Errorf("writing state marker %s: %w", markerPath, err)
	}
	return nil
}

// hasStateDirMarker reports whether stateDir contains the vmherd marker file.
func hasStateDirMarker(stateDir string) (bool, error) {
	markerPath := filepath.Join(stateDir, stateDirMarkerFile)
	info, err := os.Lstat(markerPath)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("checking state marker %s: %w", markerPath, err)
	}
	return info.Mode().IsRegular(), nil
}

// adoptUserStateDir prepares a user-chosen state directory (VMHERD_STATE_DIR).
// A new, empty, already-marked, or vmherd-only directory gets the marker;
// a directory holding anything else is refused so vmherd never claims it.
func adoptUserStateDir(stateDir string, vmNames []string) error {
	marked, err := hasStateDirMarker(stateDir)
	if err != nil {
		return err
	}
	if !marked {
		if err := checkOnlyVMHerdEntries(stateDir, vmNames); err != nil {
			return err
		}
	}
	return ensureStateDirMarker(stateDir)
}

// checkOnlyVMHerdEntries returns an error if stateDir contains any entry that
// vmherd does not create. A missing directory counts as empty.
func checkOnlyVMHerdEntries(stateDir string, vmNames []string) error {
	entries, err := os.ReadDir(stateDir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading state directory %s: %w", stateDir, err)
	}

	allowedSubdirs := map[string]bool{networkStateSubdir: true}
	for _, vmName := range vmNames {
		allowedSubdirs[vmName] = true
	}

	var foreignEntries []string
	for _, entry := range entries {
		entryName := entry.Name()
		if entry.IsDir() && allowedSubdirs[entryName] {
			continue
		}
		if entry.Type().IsRegular() && knownStateFiles[entryName] {
			continue
		}
		foreignEntries = append(foreignEntries, entryName)
	}
	if len(foreignEntries) == 0 {
		return nil
	}

	shownEntries := foreignEntries
	if len(shownEntries) > maxForeignEntriesShown {
		shownEntries = append(shownEntries[:maxForeignEntriesShown:maxForeignEntriesShown], "...")
	}
	return fmt.Errorf("VMHERD_STATE_DIR %s contains files not created by vmherd (%s): "+
		"point VMHERD_STATE_DIR at an empty or dedicated directory",
		stateDir, strings.Join(shownEntries, ", "))
}

// requireStateDirMarker returns an error unless stateDir carries the marker.
// Every removal helper calls this first.
func requireStateDirMarker(stateDir string) error {
	marked, err := hasStateDirMarker(stateDir)
	if err != nil {
		return err
	}
	if !marked {
		return fmt.Errorf("refusing to remove anything in %s: it has no %s marker, so it may not be a vmherd state directory; "+
			"remove it by hand if you are sure", stateDir, stateDirMarkerFile)
	}
	return nil
}

// removeVMSubdir deletes <stateDir>/<vmName> after checking the name cannot
// escape the state directory. A missing directory is not an error.
func removeVMSubdir(stateDir, vmName string) error {
	if !resourceNamePattern.MatchString(vmName) {
		return fmt.Errorf("refusing to remove state for VM %q: not a valid VM name", vmName)
	}
	vmStateDir := filepath.Join(stateDir, vmName)
	if err := os.RemoveAll(vmStateDir); err != nil {
		return fmt.Errorf("removing VM state %s: %w", vmStateDir, err)
	}
	return nil
}

// RemoveVMStateDir deletes the per-VM state (cloud-init seed, console log)
// for vmName, but only inside a marked vmherd state directory.
func RemoveVMStateDir(stateDir, vmName string) error {
	if err := requireStateDirMarker(stateDir); err != nil {
		return err
	}
	return removeVMSubdir(stateDir, vmName)
}

// RemoveProjectStateDir deletes only what vmherd created in a marked state
// directory (VM subdirs, keys, network XML, marker), then the directory itself
// if it is empty. Unknown files are left in place and reported.
func RemoveProjectStateDir(stateDir string, vmNames []string) error {
	if err := requireStateDirMarker(stateDir); err != nil {
		return err
	}

	var removalErrors []error
	for _, vmName := range vmNames {
		if err := removeVMSubdir(stateDir, vmName); err != nil {
			removalErrors = append(removalErrors, err)
		}
	}
	networkDir := filepath.Join(stateDir, networkStateSubdir)
	if err := os.RemoveAll(networkDir); err != nil {
		removalErrors = append(removalErrors, fmt.Errorf("removing network state %s: %w", networkDir, err))
	}
	for fileName := range knownStateFiles {
		if fileName == stateDirMarkerFile {
			continue
		}
		filePath := filepath.Join(stateDir, fileName)
		if err := os.Remove(filePath); err != nil && !errors.Is(err, fs.ErrNotExist) {
			removalErrors = append(removalErrors, fmt.Errorf("removing %s: %w", filePath, err))
		}
	}
	if len(removalErrors) > 0 {
		// Keep the marker so a retry of 'vmherd destroy' can finish the job.
		return errors.Join(removalErrors...)
	}

	markerPath := filepath.Join(stateDir, stateDirMarkerFile)
	if err := os.Remove(markerPath); err != nil {
		return fmt.Errorf("removing state marker %s: %w", markerPath, err)
	}
	if err := os.Remove(stateDir); err != nil {
		return fmt.Errorf("state directory %s still contains files not created by vmherd and was left in place: %w", stateDir, err)
	}
	return nil
}
