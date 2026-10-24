package config

import "testing"

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
