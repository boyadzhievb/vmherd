package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sharedPathFixture holds the directories used by the shared-folder policy tests.
type sharedPathFixture struct {
	homeDir    string // fake $HOME
	projectDir string // project directory inside $HOME
	outsideDir string // existing directory outside both $HOME and the project
}

// newSharedPathFixture creates a fake $HOME with a project inside it, an
// outside directory, and symlinks in the project pointing at /etc and outside.
func newSharedPathFixture(t *testing.T) sharedPathFixture {
	t.Helper()
	baseDir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	fixture := sharedPathFixture{
		homeDir:    filepath.Join(baseDir, "home"),
		projectDir: filepath.Join(baseDir, "home", "proj"),
		outsideDir: filepath.Join(baseDir, "outside"),
	}
	for _, dir := range []string{fixture.projectDir, filepath.Join(fixture.projectDir, "src"),
		filepath.Join(fixture.homeDir, "data"), fixture.outsideDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink("/etc", filepath.Join(fixture.projectDir, "etc-link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(fixture.outsideDir, filepath.Join(fixture.projectDir, "outside-link")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", fixture.homeDir)
	return fixture
}

func TestResolveSharedFolderHostPath(t *testing.T) {
	fixture := newSharedPathFixture(t)
	testCases := []struct {
		name             string // description of the case
		hostPath         string // value of shared_folders[].host
		allowOutsideHome bool   // value of shared_folders[].allow_outside_home
		wantPath         string // expected resolved path when accepted
		wantErrSubstring string // expected error fragment when rejected ("" = accept)
	}{
		{name: "project dir", hostPath: ".", wantPath: fixture.projectDir},
		{name: "project subdir", hostPath: "./src", wantPath: filepath.Join(fixture.projectDir, "src")},
		{name: "missing project subdir", hostPath: "./not-yet", wantPath: filepath.Join(fixture.projectDir, "not-yet")},
		{name: "home subdir", hostPath: filepath.Join(fixture.homeDir, "data"), wantPath: filepath.Join(fixture.homeDir, "data")},
		{name: "filesystem root", hostPath: "/", wantErrSubstring: "must not be shared as a whole"},
		{name: "relative escape to root", hostPath: strings.Repeat("../", 40), wantErrSubstring: "must not be shared as a whole"},
		{name: "home itself", hostPath: fixture.homeDir, wantErrSubstring: "must not be shared as a whole"},
		{name: "home itself via ..", hostPath: "..", wantErrSubstring: "must not be shared as a whole"},
		{name: "home itself even with opt-out", hostPath: fixture.homeDir, allowOutsideHome: true, wantErrSubstring: "must not be shared as a whole"},
		{name: "/home", hostPath: "/home", allowOutsideHome: true, wantErrSubstring: "must not be shared as a whole"},
		{name: "/etc", hostPath: "/etc", allowOutsideHome: true, wantErrSubstring: "forbidden system directory"},
		{name: "/dev", hostPath: "/dev/shm", allowOutsideHome: true, wantErrSubstring: "forbidden system directory"},
		{name: "/var/lib", hostPath: "/var/lib/libvirt", allowOutsideHome: true, wantErrSubstring: "forbidden system directory"},
		{name: "symlink to /etc", hostPath: "./etc-link", allowOutsideHome: true, wantErrSubstring: "forbidden system directory"},
		{name: "/var/x", hostPath: "/var/x", wantErrSubstring: "allow_outside_home"},
		{name: "outside dir", hostPath: fixture.outsideDir, wantErrSubstring: "allow_outside_home"},
		{name: "symlink to outside dir", hostPath: "./outside-link", wantErrSubstring: "allow_outside_home"},
		{name: "outside dir with opt-out", hostPath: fixture.outsideDir, allowOutsideHome: true, wantPath: fixture.outsideDir},
		{name: "symlink to outside dir with opt-out", hostPath: "./outside-link", allowOutsideHome: true, wantPath: fixture.outsideDir},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			resolvedPath, err := resolveSharedFolderHostPath(testCase.hostPath, fixture.projectDir, testCase.allowOutsideHome)
			if testCase.wantErrSubstring != "" {
				if err == nil || !strings.Contains(err.Error(), testCase.wantErrSubstring) {
					t.Fatalf("expected error containing %q, got path %q, err %v", testCase.wantErrSubstring, resolvedPath, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if resolvedPath != testCase.wantPath {
				t.Errorf("got %q, want %q", resolvedPath, testCase.wantPath)
			}
		})
	}
}

func TestResolveSharedFolderHostPathRejectsProjectAtHome(t *testing.T) {
	fixture := newSharedPathFixture(t)
	if _, err := resolveSharedFolderHostPath(".", fixture.homeDir, false); err == nil {
		t.Fatal("expected error when the project directory is $HOME itself")
	}
}

func TestLoadRecordsResolvedSharedFolderHost(t *testing.T) {
	fixture := newSharedPathFixture(t)
	herdfileContent := `
vms:
  - name: web
    image: ubuntu:noble
    shared_folders:
      - host: ./src
        guest: /src
      - host: ` + fixture.outsideDir + `
        guest: /outside
        allow_outside_home: true
network:
  name: test-net
  subnet: 192.168.122.0/24
`
	if err := os.WriteFile(filepath.Join(fixture.projectDir, HerdfileName), []byte(herdfileContent), 0o644); err != nil {
		t.Fatal(err)
	}
	herdfile, err := Load(fixture.projectDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sharedFolders := herdfile.VMs[0].SharedFolders
	if got, want := sharedFolders[0].ResolvedHost, filepath.Join(fixture.projectDir, "src"); got != want {
		t.Errorf("ResolvedHost[0] = %q, want %q", got, want)
	}
	if got, want := sharedFolders[1].ResolvedHost, fixture.outsideDir; got != want {
		t.Errorf("ResolvedHost[1] = %q, want %q", got, want)
	}
}
