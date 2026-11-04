package vm

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// TestDetectQEMUUser verifies that detectQEMUUser always returns a non-empty user string,
// whether parsed from config or falling back to a default.
func TestDetectQEMUUser(t *testing.T) {
	user := detectQEMUUser()
	if user == "" {
		t.Error("expected non-empty QEMU user")
	}
	t.Logf("detected QEMU user: %s", user)
}

// TestParseQEMUUserFromConfig uses table-driven cases to verify that the config
// parser correctly extracts the user directive from qemu.conf content.
func TestParseQEMUUserFromConfig(t *testing.T) {
	testCases := []struct {
		description    string
		configContent  string
		expectedResult string
	}{
		{
			description:    "standard libvirt-qemu user",
			configContent:  `user = "libvirt-qemu"`,
			expectedResult: "libvirt-qemu",
		},
		{
			description:    "qemu user on Fedora/RHEL",
			configContent:  `user = "qemu"`,
			expectedResult: "qemu",
		},
		{
			description:    "commented out directive is ignored",
			configContent:  `#user = "root"`,
			expectedResult: "",
		},
		{
			description:    "empty content returns empty string",
			configContent:  "",
			expectedResult: "",
		},
		{
			description:    "numeric UID format",
			configContent:  `user = "+107"`,
			expectedResult: "+107",
		},
		{
			description:    "user directive among other lines",
			configContent:  "# This is a comment\ngroup = \"libvirt\"\nuser = \"qemu\"\n# end",
			expectedResult: "qemu",
		},
		{
			description:    "single-quoted value",
			configContent:  `user = 'nobody'`,
			expectedResult: "nobody",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.description, func(t *testing.T) {
			result := parseQEMUUserFromConfig(testCase.configContent)
			if result != testCase.expectedResult {
				t.Errorf("parseQEMUUserFromConfig(%q) = %q, want %q",
					testCase.configContent, result, testCase.expectedResult)
			}
		})
	}
}

func TestParentDirsTopDown(t *testing.T) {
	testCases := []struct {
		absolutePath string   // input path
		wantParents  []string // expected ancestors, "/" first
	}{
		{absolutePath: "/home/bob/proj", wantParents: []string{"/", "/home", "/home/bob"}},
		{absolutePath: "/home/bob/proj/", wantParents: []string{"/", "/home", "/home/bob"}},
		{absolutePath: "/srv", wantParents: []string{"/"}},
		{absolutePath: "/", wantParents: []string{"/"}},
	}
	for _, testCase := range testCases {
		t.Run(testCase.absolutePath, func(t *testing.T) {
			gotParents := parentDirsTopDown(testCase.absolutePath)
			if !slices.Equal(gotParents, testCase.wantParents) {
				t.Errorf("parentDirsTopDown(%q) = %v, want %v", testCase.absolutePath, gotParents, testCase.wantParents)
			}
		})
	}
}

func TestGrantParentTraversalGrantsOnlyBlockedDirsTopDown(t *testing.T) {
	// /home/bob is 0750 and /home/bob/work is 0700: both block QEMU.
	blockedDirs := map[string]bool{"/home/bob": true, "/home/bob/work": true}
	var probedDirs, grantedDirs []string
	canTraverse := func(dirPath string) bool {
		probedDirs = append(probedDirs, dirPath)
		return !blockedDirs[dirPath]
	}
	grantTraverse := func(dirPath string) error {
		grantedDirs = append(grantedDirs, dirPath)
		delete(blockedDirs, dirPath)
		return nil
	}

	if err := grantParentTraversal("/home/bob/work/proj", canTraverse, grantTraverse); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := []string{"/home/bob", "/home/bob/work"}; !slices.Equal(grantedDirs, want) {
		t.Errorf("granted %v, want %v", grantedDirs, want)
	}
	if want := []string{"/", "/home", "/home/bob", "/home/bob/work"}; !slices.Equal(probedDirs, want) {
		t.Errorf("probed %v, want %v (top-down, shared folder itself excluded)", probedDirs, want)
	}
}

func TestGrantParentTraversalNoBlockedDirs(t *testing.T) {
	grantTraverse := func(dirPath string) error {
		t.Errorf("unexpected grant on %s", dirPath)
		return nil
	}
	if err := grantParentTraversal("/srv/share", func(string) bool { return true }, grantTraverse); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGrantParentTraversalStopsOnGrantError(t *testing.T) {
	grantError := errors.New("run 'sudo setfacl -m u:libvirt-qemu:x /home/bob' manually")
	var grantedDirs []string
	grantTraverse := func(dirPath string) error {
		grantedDirs = append(grantedDirs, dirPath)
		return grantError
	}
	canTraverse := func(dirPath string) bool { return dirPath == "/" }

	err := grantParentTraversal("/home/bob/proj", canTraverse, grantTraverse)
	if !errors.Is(err, grantError) {
		t.Fatalf("expected grant error, got %v", err)
	}
	if want := []string{"/home"}; !slices.Equal(grantedDirs, want) {
		t.Errorf("granted %v, want %v (must stop at first failure)", grantedDirs, want)
	}
}

func TestNewQEMUIdentity(t *testing.T) {
	testCases := []struct {
		qemuUser     string       // value from qemu.conf
		wantIdentity qemuIdentity // expected forms for setfacl and sudo
	}{
		{qemuUser: "libvirt-qemu", wantIdentity: qemuIdentity{displayName: "libvirt-qemu", aclName: "libvirt-qemu", sudoName: "libvirt-qemu"}},
		{qemuUser: "+107", wantIdentity: qemuIdentity{displayName: "+107", aclName: "107", sudoName: "#107"}},
	}
	for _, testCase := range testCases {
		t.Run(testCase.qemuUser, func(t *testing.T) {
			if gotIdentity := newQEMUIdentity(testCase.qemuUser); gotIdentity != testCase.wantIdentity {
				t.Errorf("newQEMUIdentity(%q) = %+v, want %+v", testCase.qemuUser, gotIdentity, testCase.wantIdentity)
			}
		})
	}
}

// TestEnsureSharedFolderAccess runs the real setfacl/sudo path on a temp
// directory. Requires root and the acl package (run on the libvirt testbed).
func TestEnsureSharedFolderAccess(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("requires root to run setfacl with sudo")
	}

	tmpDir := filepath.Join(t.TempDir(), "shared")
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := ensureSharedFolderAccess(tmpDir); err != nil {
		t.Errorf("ensureSharedFolderAccess failed: %v", err)
	}
	info, err := os.Stat(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o007 != 0o005 {
		t.Errorf("permissions for other changed: got %v, want r-x unchanged", info.Mode().Perm())
	}
}
