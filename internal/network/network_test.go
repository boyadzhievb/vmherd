package network

import (
	"net"
	"testing"
)

func TestSubnetsOverlapIdentical(t *testing.T) {
	if !subnetsOverlap("192.168.122.0/24", "192.168.122.0/24") {
		t.Error("identical subnets should overlap")
	}
}

func TestSubnetsOverlapSamePrefix(t *testing.T) {
	if !subnetsOverlap("10.0.1.0/24", "10.0.1.0/24") {
		t.Error("same /24 subnets should overlap")
	}
}

func TestSubnetsOverlapDisjoint(t *testing.T) {
	if subnetsOverlap("192.168.122.0/24", "192.168.123.0/24") {
		t.Error("adjacent /24 subnets should not overlap")
	}
}

func TestSubnetsOverlapWideContainsNarrow(t *testing.T) {
	if !subnetsOverlap("10.0.0.0/16", "10.0.1.0/24") {
		t.Error("/16 containing a /24 should overlap")
	}
}

func TestSubnetsOverlapNarrowInsideWide(t *testing.T) {
	if !subnetsOverlap("10.0.1.0/24", "10.0.0.0/16") {
		t.Error("/24 inside a /16 should overlap (reversed argument order)")
	}
}

func TestSubnetsOverlapCompletelyDisjoint(t *testing.T) {
	if subnetsOverlap("10.0.0.0/8", "172.16.0.0/12") {
		t.Error("10.x and 172.16.x should not overlap")
	}
}

func TestSubnetsOverlapInvalidCIDR(t *testing.T) {
	if subnetsOverlap("not-a-cidr", "192.168.1.0/24") {
		t.Error("invalid CIDR should return false, not overlap")
	}
	if subnetsOverlap("192.168.1.0/24", "garbage") {
		t.Error("invalid CIDR should return false, not overlap")
	}
}

func TestIPAndMaskToCIDRWithNetmask(t *testing.T) {
	elem := networkIPXML{Address: "192.168.122.1", Netmask: "255.255.255.0"}
	cidr := ipAndMaskToCIDR(elem)
	if cidr != "192.168.122.0/24" {
		t.Errorf("expected 192.168.122.0/24, got %s", cidr)
	}
}

func TestIPAndMaskToCIDRWithPrefix(t *testing.T) {
	elem := networkIPXML{Address: "10.0.0.1", Prefix: "16"}
	cidr := ipAndMaskToCIDR(elem)
	if cidr != "10.0.0.0/16" {
		t.Errorf("expected 10.0.0.0/16, got %s", cidr)
	}
}

func TestIPAndMaskToCIDRNoMask(t *testing.T) {
	elem := networkIPXML{Address: "10.0.0.1"}
	cidr := ipAndMaskToCIDR(elem)
	if cidr != "" {
		t.Errorf("expected empty string when no netmask or prefix, got %s", cidr)
	}
}

func TestIPAndMaskToCIDRInvalidAddress(t *testing.T) {
	elem := networkIPXML{Address: "not-an-ip", Netmask: "255.255.255.0"}
	cidr := ipAndMaskToCIDR(elem)
	if cidr != "" {
		t.Errorf("expected empty string for invalid address, got %s", cidr)
	}
}

func TestIPAndMaskToCIDRInvalidNetmask(t *testing.T) {
	elem := networkIPXML{Address: "10.0.0.1", Netmask: "not-a-mask"}
	cidr := ipAndMaskToCIDR(elem)
	if cidr != "" {
		t.Errorf("expected empty string for invalid netmask, got %s", cidr)
	}
}

func TestIPAndMaskToCIDRNetworkAddress(t *testing.T) {
	elem := networkIPXML{Address: "172.16.5.1", Netmask: "255.255.0.0"}
	cidr := ipAndMaskToCIDR(elem)

	_, expectedNet, _ := net.ParseCIDR("172.16.0.0/16")
	_, gotNet, err := net.ParseCIDR(cidr)
	if err != nil {
		t.Fatalf("result %q is not valid CIDR: %v", cidr, err)
	}
	if expectedNet.String() != gotNet.String() {
		t.Errorf("expected network %s, got %s", expectedNet, gotNet)
	}
}

func TestTruncateBridgeNameShort(t *testing.T) {
	name := truncateBridgeName("virbr0")
	if name != "virbr0" {
		t.Errorf("expected virbr0, got %s", name)
	}
}

func TestTruncateBridgeNameExact15(t *testing.T) {
	name := truncateBridgeName("vmherd-exactly")
	if name != "vmherd-exactly" || len(name) != 14 {
		t.Errorf("expected vmherd-exactly (14 chars), got %s (%d chars)", name, len(name))
	}
}

func TestTruncateBridgeNameLong(t *testing.T) {
	name := truncateBridgeName("vmherd-my-very-long-network-name")
	if len(name) != maxBridgeNameLength {
		t.Errorf("expected length %d, got %d", maxBridgeNameLength, len(name))
	}
	if name != "vmherd-my-very-" {
		t.Errorf("expected vmherd-my-very-, got %s", name)
	}
}

func TestTruncateBridgeNameCollisionScenario(t *testing.T) {
	nameA := truncateBridgeName("vmherd-projectA-longname")
	nameB := truncateBridgeName("vmherd-projectA-othername")
	if nameA != nameB {
		t.Errorf("expected collision: both should truncate to same name, got %q and %q", nameA, nameB)
	}
}

func TestTruncateBridgeNameNoCollisionDifferentPrefix(t *testing.T) {
	nameA := truncateBridgeName("vmherd-alpha-network")
	nameB := truncateBridgeName("vmherd-beta-network")
	if nameA == nameB {
		t.Errorf("different prefixes should not collide, both got %q", nameA)
	}
}
