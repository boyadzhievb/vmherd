package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// loadHerdfileText writes herdfileText as the Herdfile of a temp project and
// returns Load's error (nil if it loaded).
func loadHerdfileText(t *testing.T, herdfileText string) error {
	t.Helper()
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, HerdfileName), []byte(herdfileText), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(projectDir)
	return err
}

// assertErrorMentions fails unless err is set and contains every fragment.
func assertErrorMentions(t *testing.T, err error, fragments ...string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected an error mentioning %q, got none", fragments)
	}
	for _, fragment := range fragments {
		if !strings.Contains(err.Error(), fragment) {
			t.Errorf("error does not mention %q:\n%v", fragment, err)
		}
	}
}

// TestLoadReportsAllValidationProblems is M1.9's "done when": three distinct
// problems in different places are all reported, with a count.
func TestLoadReportsAllValidationProblems(t *testing.T) {
	err := loadHerdfileText(t, `
images:
  ubuntu:noble: https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-amd64.img
vms:
  - name: web_1
    image: ubuntu:noble
    disk: 20GB
    shared_folders: []
ssh:
  user: Root
`)
	assertErrorMentions(t, err,
		"Herdfile has 3 problems:",
		`VM name "web_1" is invalid`,
		`disk size "20GB" is invalid`,
		`ssh.user "Root" is invalid`)
	if lineCount := strings.Count(err.Error(), "\n  - "); lineCount != 3 {
		t.Errorf("expected 3 listed problems, got %d:\n%v", lineCount, err)
	}
}

// TestLoadReportsUnknownKeysWithLineNumbers checks strict decoding: every
// unknown key is reported with its line, location and a suggestion or the
// known keys there.
func TestLoadReportsUnknownKeysWithLineNumbers(t *testing.T) {
	err := loadHerdfileText(t, `vms:
  - name: web
    image: ubuntu:noble
    memroy: 2048
    network:
      ipp: 192.168.122.10
netwrok:
  name: web-net
ssh:
  usr: vmherd
  favourite_colour: blue
`)
	assertErrorMentions(t, err,
		"Herdfile has 5 problems:",
		"line 4: unknown key 'memroy' in a VM (under 'vms:') — did you mean 'memory'?",
		"line 6: unknown key 'ipp' in a VM's 'network:' — did you mean 'ip'?",
		"line 7: unknown key 'netwrok' at the top level — did you mean 'network'?",
		"line 10: unknown key 'usr' in 'ssh:' — did you mean 'user'?",
		"line 11: unknown key 'favourite_colour' in 'ssh:' — known keys there: password, user")
}

// TestLoadReportsWronglyTypedValues checks that type mismatches name the line,
// the value and what the key needs.
func TestLoadReportsWronglyTypedValues(t *testing.T) {
	err := loadHerdfileText(t, `vms:
  - name: web
    image: ubuntu:noble
    cpus: two
    console_log: maybe
    ports: 8080
`)
	assertErrorMentions(t, err,
		"Herdfile has 3 problems:",
		"line 4: 'two' is text, but this key needs a whole number",
		"line 5: 'maybe' is text, but this key needs true or false",
		"line 6: '8080' is a number, but this key needs a list (lines starting with '- ')")
}

func TestLoadRejectsEmptyHerdfile(t *testing.T) {
	for _, emptyText := range []string{"", "\n# only a comment\n"} {
		assertErrorMentions(t, loadHerdfileText(t, emptyText), "Herdfile is empty", "vmherd init")
	}
}

func TestLoadReportsYAMLSyntaxError(t *testing.T) {
	assertErrorMentions(t, loadHerdfileText(t, "vms:\n  - name: web\n   image: ubuntu:noble\n"),
		"Herdfile: invalid YAML syntax:", "line ")
}

// TestLoadReportsBadSubnetOnce checks that an invalid subnet is one problem,
// not repeated as "outside the subnet" for every VM with an ip.
func TestLoadReportsBadSubnetOnce(t *testing.T) {
	err := loadHerdfileText(t, `
images:
  ubuntu:noble: https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-amd64.img
vms:
  - name: web
    image: ubuntu:noble
    shared_folders: []
    network:
      ip: 10.0.0.10
  - name: db
    image: ubuntu:noble
    shared_folders: []
    network:
      ip: 10.0.0.11
network:
  subnet: 10.0.0.0/16
`)
	assertErrorMentions(t, err, "Herdfile: only /24 subnets are supported")
	if strings.Contains(err.Error(), "outside the network subnet") {
		t.Errorf("bad subnet was repeated per VM:\n%v", err)
	}
}

// TestLoadReportsUnnamedVMsOtherProblems checks that a VM without a name is
// called by its position and its other problems are still reported.
func TestLoadReportsUnnamedVMsOtherProblems(t *testing.T) {
	err := loadHerdfileText(t, `
images:
  ubuntu:noble: https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-amd64.img
vms:
  - name: web
    image: ubuntu:noble
    shared_folders: []
  - image: noble
    shared_folders: []
`)
	assertErrorMentions(t, err,
		"Herdfile has 2 problems:",
		"VM #2 under 'vms:' has no 'name'",
		`VM "#2": image must be 'distro:codename' format`)
}

// TestLoadReportsEveryBadPortAndShare checks that the per-VM helpers report
// every bad entry, not just the first.
func TestLoadReportsEveryBadPortAndShare(t *testing.T) {
	err := loadHerdfileText(t, `
images:
  ubuntu:noble: https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-amd64.img
vms:
  - name: web
    image: ubuntu:noble
    shared_folders:
      - host: .
        guest: vagrant
      - host: .
        guest: data
    ports:
      - host: 0
        guest: 80
      - host: 8443
        guest: 70000
        protocol: udp
`)
	assertErrorMentions(t, err,
		"Herdfile has 5 problems:",
		`guest path "vagrant" must be absolute`,
		`guest path "data" must be absolute`,
		"host port 0 is out of range",
		"guest port 70000 is out of range",
		`port 8443 uses protocol "udp"`)
}

func TestEditDistance(t *testing.T) {
	testCases := []struct {
		first, second string
		wantDistance  int
	}{
		{"memory", "memory", 0},
		{"memroy", "memory", 1}, // swapped neighbours
		{"mem", "memory", 3},
		{"ipp", "ip", 1},
		{"", "ssh", 3},
	}
	for _, testCase := range testCases {
		if gotDistance := editDistance(testCase.first, testCase.second); gotDistance != testCase.wantDistance {
			t.Errorf("editDistance(%q, %q) = %d, want %d", testCase.first, testCase.second, gotDistance, testCase.wantDistance)
		}
	}
}

func TestClosestKeyIgnoresUnrelatedKeys(t *testing.T) {
	knownKeys := []string{"cpus", "disk", "image", "memory", "name"}
	if suggestion := closestKey("favourite_colour", knownKeys); suggestion != "" {
		t.Errorf("closestKey() suggested %q for an unrelated key", suggestion)
	}
	if suggestion := closestKey("CPUS", knownKeys); suggestion != "cpus" {
		t.Errorf("closestKey(CPUS) = %q, want cpus", suggestion)
	}
}
