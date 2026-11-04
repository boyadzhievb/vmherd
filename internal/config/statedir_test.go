package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeTestFile creates a file (and its parent directories) with fixed content.
func writeTestFile(t *testing.T, filePath string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filePath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filePath, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// populateVMHerdState fills stateDir with the files vmherd normally creates.
func populateVMHerdState(t *testing.T, stateDir string, vmNames []string) {
	t.Helper()
	writeTestFile(t, filepath.Join(stateDir, "id_ed25519"))
	writeTestFile(t, filepath.Join(stateDir, "id_ed25519.pub"))
	writeTestFile(t, filepath.Join(stateDir, networkStateSubdir, "vmherd-default.xml"))
	for _, vmName := range vmNames {
		writeTestFile(t, filepath.Join(stateDir, vmName, "user-data"))
	}
}

// assertExists fails the test if path does not exist.
func assertExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); err != nil {
		t.Errorf("expected %s to exist: %v", path, err)
	}
}

func TestRemoveProjectStateDirRefusesUnmarkedDir(t *testing.T) {
	stateDir := t.TempDir()
	vmNames := []string{"ctrl"}
	populateVMHerdState(t, stateDir, vmNames)
	userFile := filepath.Join(stateDir, "Documents", "thesis.txt")
	writeTestFile(t, userFile)

	err := RemoveProjectStateDir(stateDir, vmNames)
	if err == nil || !strings.Contains(err.Error(), "refusing") {
		t.Fatalf("expected refusal error, got %v", err)
	}
	assertExists(t, userFile)
	assertExists(t, filepath.Join(stateDir, "id_ed25519"))
	assertExists(t, filepath.Join(stateDir, "ctrl", "user-data"))
}

func TestRemoveVMStateDirRefusesUnmarkedDir(t *testing.T) {
	stateDir := t.TempDir()
	vmDir := filepath.Join(stateDir, "ctrl")
	writeTestFile(t, filepath.Join(vmDir, "user-data"))

	if err := RemoveVMStateDir(stateDir, "ctrl"); err == nil {
		t.Fatal("expected refusal error for unmarked directory")
	}
	assertExists(t, vmDir)
}

func TestRemoveVMStateDirRejectsInvalidName(t *testing.T) {
	stateDir := t.TempDir()
	if err := ensureStateDirMarker(stateDir); err != nil {
		t.Fatal(err)
	}
	for _, vmName := range []string{"..", "../x", "", "a/b"} {
		if err := RemoveVMStateDir(stateDir, vmName); err == nil {
			t.Errorf("expected error for VM name %q", vmName)
		}
	}
	assertExists(t, stateDir)
}

func TestRemoveProjectStateDirRemovesMarkedDir(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "state")
	vmNames := []string{"ctrl", "worker-1"}
	populateVMHerdState(t, stateDir, vmNames)
	if err := ensureStateDirMarker(stateDir); err != nil {
		t.Fatal(err)
	}

	if err := RemoveProjectStateDir(stateDir, vmNames); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Lstat(stateDir); !os.IsNotExist(err) {
		t.Fatalf("expected %s to be removed, stat error: %v", stateDir, err)
	}
}

func TestRemoveProjectStateDirKeepsUnknownFiles(t *testing.T) {
	stateDir := t.TempDir()
	vmNames := []string{"ctrl"}
	populateVMHerdState(t, stateDir, vmNames)
	if err := ensureStateDirMarker(stateDir); err != nil {
		t.Fatal(err)
	}
	userFile := filepath.Join(stateDir, "notes.txt")
	writeTestFile(t, userFile)

	err := RemoveProjectStateDir(stateDir, vmNames)
	if err == nil || !strings.Contains(err.Error(), "left in place") {
		t.Fatalf("expected left-in-place error, got %v", err)
	}
	assertExists(t, userFile)
	if _, err := os.Lstat(filepath.Join(stateDir, "id_ed25519")); !os.IsNotExist(err) {
		t.Errorf("expected vmherd key to be removed, stat error: %v", err)
	}
}

func TestRemoveProjectStateDirReportsRemovalErrors(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	stateDir := t.TempDir()
	vmNames := []string{"ctrl"}
	populateVMHerdState(t, stateDir, vmNames)
	if err := ensureStateDirMarker(stateDir); err != nil {
		t.Fatal(err)
	}
	vmDir := filepath.Join(stateDir, "ctrl")
	if err := os.Chmod(vmDir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(vmDir, 0o700); err != nil {
			t.Errorf("restoring permissions on %s: %v", vmDir, err)
		}
	})

	err := RemoveProjectStateDir(stateDir, vmNames)
	if err == nil || !strings.Contains(err.Error(), "removing VM state") {
		t.Fatalf("expected VM state removal error, got %v", err)
	}
	assertExists(t, filepath.Join(stateDir, stateDirMarkerFile))
}

func TestResolveStateDirRefusesForeignEnvDir(t *testing.T) {
	homeLikeDir := t.TempDir()
	writeTestFile(t, filepath.Join(homeLikeDir, ".bashrc"))
	t.Setenv("VMHERD_STATE_DIR", homeLikeDir)

	_, err := resolveStateDir(t.TempDir(), []string{"ctrl"})
	if err == nil || !strings.Contains(err.Error(), ".bashrc") {
		t.Fatalf("expected error naming the foreign file, got %v", err)
	}
	if _, err := os.Lstat(filepath.Join(homeLikeDir, stateDirMarkerFile)); !os.IsNotExist(err) {
		t.Errorf("marker must not be written into a foreign directory, stat error: %v", err)
	}
}

func TestResolveStateDirAdoptsLegacyEnvDir(t *testing.T) {
	legacyDir := t.TempDir()
	vmNames := []string{"ctrl"}
	populateVMHerdState(t, legacyDir, vmNames)
	t.Setenv("VMHERD_STATE_DIR", legacyDir)

	stateDir, err := resolveStateDir(t.TempDir(), vmNames)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stateDir != legacyDir {
		t.Errorf("got state dir %s, want %s", stateDir, legacyDir)
	}
	assertExists(t, filepath.Join(legacyDir, stateDirMarkerFile))
}

func TestResolveStateDirMarksNewEnvDir(t *testing.T) {
	newDir := filepath.Join(t.TempDir(), "fresh")
	t.Setenv("VMHERD_STATE_DIR", newDir)

	if _, err := resolveStateDir(t.TempDir(), nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertExists(t, filepath.Join(newDir, stateDirMarkerFile))
}

func TestResolveStateDirMarksGlobalDir(t *testing.T) {
	t.Setenv("VMHERD_STATE_DIR", "")
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	stateDir, err := resolveStateDir(t.TempDir(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertExists(t, filepath.Join(stateDir, stateDirMarkerFile))
}
