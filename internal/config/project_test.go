package config

import (
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
	project := &Project{Name: "myproject"}

	domain := project.DomainName("ctrl")
	if domain != "vmherd-myproject-ctrl" {
		t.Errorf("expected vmherd-myproject-ctrl, got %s", domain)
	}
}

func TestDiskPath(t *testing.T) {
	project := &Project{Name: "myproject"}

	diskPath := project.DiskPath("ctrl")
	expected := "/var/lib/libvirt/images/vmherd-myproject-ctrl.qcow2"
	if diskPath != expected {
		t.Errorf("expected %s, got %s", expected, diskPath)
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
