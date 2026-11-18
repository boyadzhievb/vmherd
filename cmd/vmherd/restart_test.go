package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/boyadzhievb/vmherd/internal/config"
)

// TestCheckRestartPrerequisitesStopsWhenShareFolderMissing checks that
// restart's pre-check names a missing share folder and says no VM was
// stopped, so a running VM isn't stopped only to fail to start again.
func TestCheckRestartPrerequisitesStopsWhenShareFolderMissing(t *testing.T) {
	missingFolder := filepath.Join(t.TempDir(), "terramino-go")
	vmsToRestart := []config.VM{{
		Name:          "ctrl",
		Image:         "ubuntu:noble",
		SharedFolders: []config.SharedFolder{{Host: "./terramino-go", Guest: "/app", ResolvedHost: missingFolder}},
	}}

	err := checkRestartPrerequisites(vmsToRestart)
	if err == nil {
		t.Fatal("expected an error for a missing share folder, got nil")
	}
	for _, wantFragment := range []string{`VM "ctrl"`, "./terramino-go", missingFolder, "restart stopped before stopping any VM"} {
		if !strings.Contains(err.Error(), wantFragment) {
			t.Errorf("error should contain %q, got: %v", wantFragment, err)
		}
	}
}

// TestCheckRestartPrerequisitesPassesWithFoldersPresent checks that restart's
// pre-check passes when every share folder exists or there are no shares.
func TestCheckRestartPrerequisitesPassesWithFoldersPresent(t *testing.T) {
	vmsToRestart := []config.VM{
		{Name: "ctrl", SharedFolders: []config.SharedFolder{{Host: ".", Guest: "/vagrant", ResolvedHost: t.TempDir()}}},
		{Name: "worker", SharedFolders: []config.SharedFolder{}},
	}
	if err := checkRestartPrerequisites(vmsToRestart); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
