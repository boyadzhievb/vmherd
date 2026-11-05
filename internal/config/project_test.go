package config

import (
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestSubnetBase(t *testing.T) {
	project := &Project{
		Herdfile: &Herdfile{
			Network: NetworkConfig{Subnet: "192.168.122.0/24"},
		},
	}

	base, err := project.SubnetBase()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if base != "192.168.122" {
		t.Errorf("expected 192.168.122, got %s", base)
	}
}

func TestSubnetBaseCustom(t *testing.T) {
	project := &Project{
		Herdfile: &Herdfile{
			Network: NetworkConfig{Subnet: "10.0.1.0/24"},
		},
	}

	base, err := project.SubnetBase()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if base != "10.0.1" {
		t.Errorf("expected 10.0.1, got %s", base)
	}
}

func TestGatewayIP(t *testing.T) {
	project := &Project{
		Herdfile: &Herdfile{
			Network: NetworkConfig{Subnet: "192.168.122.0/24"},
		},
	}

	gw, err := project.GatewayIP()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gw != "192.168.122.1" {
		t.Errorf("expected 192.168.122.1, got %s", gw)
	}
}

func TestDomainName(t *testing.T) {
	project := &Project{Name: "myproject", PathHash: "0123456789abcdef"}

	domain := project.DomainName("ctrl")
	if domain != "vmherd-myproject-01234567-ctrl" {
		t.Errorf("expected vmherd-myproject-01234567-ctrl, got %s", domain)
	}
}

func TestDiskPath(t *testing.T) {
	project := &Project{Name: "myproject", PathHash: "0123456789abcdef"}

	diskPath := project.DiskPath("ctrl")
	expected := "/var/lib/libvirt/images/vmherd-myproject-01234567-ctrl.qcow2"
	if diskPath != expected {
		t.Errorf("expected %s, got %s", expected, diskPath)
	}
}

func TestDomainNameDiffersForSameBasename(t *testing.T) {
	firstHash, err := projectPathHash("/home/user/a/web")
	if err != nil {
		t.Fatalf("hashing first path: %v", err)
	}
	secondHash, err := projectPathHash("/home/user/b/web")
	if err != nil {
		t.Fatalf("hashing second path: %v", err)
	}
	firstProject := &Project{Name: "web", PathHash: firstHash}
	secondProject := &Project{Name: "web", PathHash: secondHash}

	if firstProject.DomainName("ctrl") == secondProject.DomainName("ctrl") {
		t.Errorf("projects in different directories share domain name %s", firstProject.DomainName("ctrl"))
	}
	if firstProject.DiskPath("ctrl") == secondProject.DiskPath("ctrl") {
		t.Errorf("projects in different directories share disk path %s", firstProject.DiskPath("ctrl"))
	}
}

func TestDomainNameHashMatchesStateDir(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/state")
	projectDir := "/home/user/web"
	pathHash, err := projectPathHash(projectDir)
	if err != nil {
		t.Fatalf("hashing path: %v", err)
	}
	stateDir, err := globalStateDir(projectDir)
	if err != nil {
		t.Fatalf("resolving state dir: %v", err)
	}
	if stateDir != "/state/vmherd/web-"+pathHash {
		t.Errorf("state dir %s does not use path hash %s", stateDir, pathHash)
	}
	domain := (&Project{Name: "web", PathHash: pathHash}).DomainName("ctrl")
	if domain != "vmherd-web-"+pathHash[:domainHashLength]+"-ctrl" {
		t.Errorf("domain %s does not use path hash prefix %s", domain, pathHash[:domainHashLength])
	}
}

func TestDomainNameUnsafeProjectNames(t *testing.T) {
	validDomainName := regexp.MustCompile(`^vmherd-[a-z0-9]+(-[a-z0-9]+)*-[0-9a-f]{8}-ctrl$`)
	testCases := []struct {
		projectName  string // directory basename
		expectedPart string // sanitized project part of the domain name
	}{
		{"web", "web"},
		{"My Project", "my-project"},
		{"evil,format=raw", "evil-format-raw"},
		{"a,,  b", "a-b"},
		{"--edge--", "edge"},
		{"proj_v2.0", "proj-v2-0"},
		{"日本語", fallbackProjectNamePart},
		{",", fallbackProjectNamePart},
		{strings.Repeat("x", 100), strings.Repeat("x", maxProjectNamePartLength)},
		{strings.Repeat("x", maxProjectNamePartLength-1) + "-y", strings.Repeat("x", maxProjectNamePartLength-1)},
	}
	for _, testCase := range testCases {
		project := &Project{Name: testCase.projectName, PathHash: "0123456789abcdef"}
		domain := project.DomainName("ctrl")
		expectedDomain := "vmherd-" + testCase.expectedPart + "-01234567-ctrl"
		if domain != expectedDomain {
			t.Errorf("project %q: expected %s, got %s", testCase.projectName, expectedDomain, domain)
		}
		if !validDomainName.MatchString(domain) {
			t.Errorf("project %q: domain %s contains unsafe characters", testCase.projectName, domain)
		}
	}
}

func TestLegacyDomainNameOverride(t *testing.T) {
	project := &Project{Name: "web", PathHash: "0123456789abcdef"}
	newName := project.DomainName("ctrl")

	project.UseLegacyDomainName("ctrl")
	if project.DomainName("ctrl") != "vmherd-web-ctrl" {
		t.Errorf("expected legacy name vmherd-web-ctrl, got %s", project.DomainName("ctrl"))
	}
	if project.DiskPath("ctrl") != "/var/lib/libvirt/images/vmherd-web-ctrl.qcow2" {
		t.Errorf("expected legacy disk path, got %s", project.DiskPath("ctrl"))
	}
	if project.DomainName("worker") != "vmherd-web-01234567-worker" {
		t.Errorf("override leaked to another VM: %s", project.DomainName("worker"))
	}

	project.DropLegacyDomainName("ctrl")
	if project.DomainName("ctrl") != newName {
		t.Errorf("expected %s after dropping override, got %s", newName, project.DomainName("ctrl"))
	}
}

// TestSubnetBaseInvalidCIDR verifies that SubnetBase returns an error for invalid CIDR notation.
func TestSubnetBaseInvalidCIDR(t *testing.T) {
	project := &Project{
		Herdfile: &Herdfile{
			Network: NetworkConfig{Subnet: "not-a-cidr"},
		},
	}

	_, err := project.SubnetBase()
	if err == nil {
		t.Fatal("expected error for invalid CIDR subnet")
	}
}

// TestGatewayIPInvalidCIDR verifies that GatewayIP returns an error for invalid CIDR notation.
func TestGatewayIPInvalidCIDR(t *testing.T) {
	project := &Project{
		Herdfile: &Herdfile{
			Network: NetworkConfig{Subnet: "not-a-cidr"},
		},
	}

	_, err := project.GatewayIP()
	if err == nil {
		t.Fatal("expected error for invalid CIDR subnet")
	}
}

// TestExampleHerdfileIsValidYAML verifies that ExampleHerdfile produces parseable YAML with at least one VM.
func TestExampleHerdfileIsValidYAML(t *testing.T) {
	exampleContent := ExampleHerdfile()

	var herdfile Herdfile
	if err := yaml.Unmarshal([]byte(exampleContent), &herdfile); err != nil {
		t.Fatalf("ExampleHerdfile produced invalid YAML: %v", err)
	}

	if len(herdfile.VMs) == 0 {
		t.Fatal("ExampleHerdfile must define at least one VM")
	}
}
