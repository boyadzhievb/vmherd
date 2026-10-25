package vm

import (
	"os"
	"path/filepath"
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

// TestEnsureSharedFolderAccess verifies that ACL/chmod setup succeeds for a temp directory.
// Requires root to run setfacl/chmod with sudo.
func TestEnsureSharedFolderAccess(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("requires root to test setfacl/chmod with sudo")
	}

	tmpDir := filepath.Join(t.TempDir(), "shared")
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := ensureSharedFolderAccess(tmpDir); err != nil {
		t.Errorf("ensureSharedFolderAccess failed: %v", err)
	}
}
