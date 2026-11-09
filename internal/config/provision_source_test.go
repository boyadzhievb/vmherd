package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCheckProvisionSourceInsideProject covers M1.11: a sibling directory
// whose name starts with the project's name, plain "..", and symlinks that
// point outside or elsewhere inside the project.
func TestCheckProvisionSourceInsideProject(t *testing.T) {
	parentDir := t.TempDir()
	projectDir := filepath.Join(parentDir, "proj")
	siblingDir := filepath.Join(parentDir, "proj-evil")
	for _, directory := range []string{filepath.Join(projectDir, "scripts"), siblingDir} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, filePath := range []string{
		filepath.Join(projectDir, "scripts", "setup.sh"),
		filepath.Join(siblingDir, "secret"),
		filepath.Join(parentDir, "outside.txt"),
	} {
		if err := os.WriteFile(filePath, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	symlinks := map[string]string{
		"link-outside":     filepath.Join(siblingDir, "secret"),
		"link-inside":      filepath.Join(projectDir, "scripts", "setup.sh"),
		"link-to-parent":   parentDir,
		"scripts/relative": "../../outside.txt",
	}
	for linkName, target := range symlinks {
		if err := os.Symlink(target, filepath.Join(projectDir, linkName)); err != nil {
			t.Fatal(err)
		}
	}

	testCases := []struct {
		source      string
		wantProblem string // "" means accepted
	}{
		{"scripts/setup.sh", ""},
		{"./scripts/../scripts/setup.sh", ""},
		{"missing.sh", ""}, // reported by up/provision, not at load time
		{"link-inside", ""},
		{"../proj-evil/secret", "escapes project directory"},
		{"../outside.txt", "escapes project directory"},
		{"scripts/../../outside.txt", "escapes project directory"},
		{".", "is the project directory itself"},
		{"../proj", "is the project directory itself"},
		{"link-outside", "is a symlink to"},
		{"link-to-parent/outside.txt", "is a symlink to"},
		{"scripts/relative", "is a symlink to"},
		{"/etc/passwd", "must be a relative path"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.source, func(t *testing.T) {
			err := checkProvisionSourceInsideProject(testCase.source, projectDir)
			if testCase.wantProblem == "" {
				if err != nil {
					t.Errorf("source %q refused: %v", testCase.source, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), testCase.wantProblem) {
				t.Errorf("source %q: error %v, want one mentioning %q", testCase.source, err, testCase.wantProblem)
			}
		})
	}
}
