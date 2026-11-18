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
images:
  ubuntu:noble: https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-amd64.img
  debian:bookworm: https://cloud.debian.org/images/cloud/bookworm/latest/debian-12-generic-amd64.qcow2
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

// TestCheckSharedFolderHostPathsExist checks that up's and reload's share
// check reports a missing folder, a file, a dangling symlink (the share or a
// parent) with a fitting hint, every problem in Herdfile order, and passes
// when all folders exist or there are no shares.
func TestCheckSharedFolderHostPathsExist(t *testing.T) {
	baseDir := t.TempDir()
	presentDir := filepath.Join(baseDir, "present")
	missingDir := filepath.Join(baseDir, "terramino-go")
	otherMissingDir := filepath.Join(baseDir, "other-missing")
	spacedMissingDir := filepath.Join(baseDir, "it's my folder")
	regularFile := filepath.Join(baseDir, "a-file")
	missingLinkTarget := filepath.Join(baseDir, "gone")
	danglingLink := filepath.Join(baseDir, "link")
	danglingParentLink := filepath.Join(baseDir, "parent-link")
	if err := os.MkdirAll(presentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(regularFile, []byte("not a folder"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, linkPath := range []string{danglingLink, danglingParentLink} {
		if err := os.Symlink(missingLinkTarget, linkPath); err != nil {
			t.Fatal(err)
		}
	}
	shareOf := func(hostPath, resolvedHostPath string) SharedFolder {
		return SharedFolder{Host: hostPath, Guest: "/share", ResolvedHost: resolvedHostPath}
	}
	testCases := []struct {
		name              string   // description of the case
		vms               []VM     // the VMs to check
		wantErrSubstrings []string // fragments the error must contain, in this order (none = pass)
		unwantedFragments []string // fragments the error must not contain
	}{
		{
			name: "missing",
			vms:  []VM{{Name: "web", SharedFolders: []SharedFolder{shareOf("./terramino-go", missingDir)}}},
			wantErrSubstrings: []string{`VM "web"`, `"./terramino-go"`, missingDir, "does not exist",
				"mkdir -p '" + missingDir + "'", "remove the share from the Herdfile"},
		},
		{
			name: "missing with space and quote",
			vms:  []VM{{Name: "web", SharedFolders: []SharedFolder{shareOf("./it's my folder", spacedMissingDir)}}},
			wantErrSubstrings: []string{`VM "web"`, "does not exist",
				"mkdir -p '" + strings.ReplaceAll(spacedMissingDir, "'", `'\''`) + "'"},
		},
		{
			name: "dangling symlink",
			vms:  []VM{{Name: "web", SharedFolders: []SharedFolder{shareOf("./link", danglingLink)}}},
			wantErrSubstrings: []string{`VM "web"`, `"./link"`, danglingLink, "is a symlink to missing " + missingLinkTarget,
				"create the target or fix the link"},
			unwantedFragments: []string{"mkdir -p"},
		},
		{
			name: "dangling symlinked parent",
			vms: []VM{{Name: "web", SharedFolders: []SharedFolder{
				shareOf("./parent-link/src", filepath.Join(danglingParentLink, "src"))}}},
			wantErrSubstrings: []string{`VM "web"`, `"./parent-link/src"`, danglingParentLink,
				"is a symlink to missing " + missingLinkTarget, "create the target or fix the link"},
			unwantedFragments: []string{"mkdir -p"},
		},
		{
			name:              "file",
			vms:               []VM{{Name: "web", SharedFolders: []SharedFolder{shareOf("./a-file", regularFile)}}},
			wantErrSubstrings: []string{`VM "web"`, `"./a-file"`, regularFile, "is not a directory"},
		},
		{
			name: "two VMs",
			vms: []VM{
				{Name: "first", SharedFolders: []SharedFolder{shareOf("./terramino-go", missingDir)}},
				{Name: "second", SharedFolders: []SharedFolder{shareOf("./present", presentDir), shareOf("./other-missing", otherMissingDir)}},
			},
			wantErrSubstrings: []string{`VM "first"`, "mkdir -p '" + missingDir + "'", `VM "second"`, "mkdir -p '" + otherMissingDir + "'"},
		},
		{
			name: "present",
			vms:  []VM{{Name: "web", SharedFolders: []SharedFolder{shareOf(".", baseDir), shareOf("./present", presentDir)}}},
		},
		{
			name: "no shares",
			vms:  []VM{{Name: "web", SharedFolders: []SharedFolder{}}, {Name: "db"}},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			err := CheckSharedFolderHostPathsExist(testCase.vms)
			if len(testCase.wantErrSubstrings) == 0 {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected an error containing %q, got nil", testCase.wantErrSubstrings)
			}
			remainingMessage := err.Error()
			for _, wantSubstring := range testCase.wantErrSubstrings {
				position := strings.Index(remainingMessage, wantSubstring)
				if position < 0 {
					t.Fatalf("error should contain %q (in this order), got: %v", wantSubstring, err)
				}
				remainingMessage = remainingMessage[position+len(wantSubstring):]
			}
			for _, unwantedFragment := range testCase.unwantedFragments {
				if strings.Contains(err.Error(), unwantedFragment) {
					t.Errorf("error should not contain %q, got: %v", unwantedFragment, err)
				}
			}
		})
	}
}

// TestLoadAcceptsMissingShareFolder checks that loading still accepts a share
// whose host folder doesn't exist, so status, destroy, down and ssh keep
// working after the folder is deleted; only up, reload and restart refuse it.
func TestLoadAcceptsMissingShareFolder(t *testing.T) {
	fixture := newSharedPathFixture(t)
	herdfileContent := `
images:
  ubuntu:noble: https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-amd64.img
vms:
  - name: web
    image: ubuntu:noble
    shared_folders:
      - host: ./terramino-go
        guest: /app
network:
  name: test-net
  subnet: 192.168.122.0/24
`
	if err := os.WriteFile(filepath.Join(fixture.projectDir, HerdfileName), []byte(herdfileContent), 0o644); err != nil {
		t.Fatal(err)
	}
	missingFolder := filepath.Join(fixture.projectDir, "terramino-go")
	if _, err := os.Stat(missingFolder); !os.IsNotExist(err) {
		t.Fatalf("test setup: %s should not exist, stat error: %v", missingFolder, err)
	}
	herdfile, err := Load(fixture.projectDir)
	if err != nil {
		t.Fatalf("a missing share folder must not fail loading (status, destroy, down and ssh rely on it): %v", err)
	}
	if got := herdfile.VMs[0].SharedFolders[0].ResolvedHost; got != missingFolder {
		t.Errorf("ResolvedHost = %q, want %q", got, missingFolder)
	}
}
