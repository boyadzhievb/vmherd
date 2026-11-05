package network

import (
	"net"
	"slices"
	"strings"
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

func TestBridgeNameForNetworkShort(t *testing.T) {
	for _, networkName := range []string{"virbr0", "vmherd-exactly", "vmherd-15-chars"} {
		if bridgeName := bridgeNameForNetwork(networkName); bridgeName != networkName {
			t.Errorf("bridgeNameForNetwork(%q) = %q, want the name unchanged", networkName, bridgeName)
		}
	}
}

func TestBridgeNameForNetworkLong(t *testing.T) {
	bridgeName := bridgeNameForNetwork("vmherd-web-8d7c1111")
	if len(bridgeName) != maxBridgeNameLength {
		t.Errorf("bridge name %q has %d chars, want %d", bridgeName, len(bridgeName), maxBridgeNameLength)
	}
	if !strings.HasPrefix(bridgeName, "vmherd-") {
		t.Errorf("bridge name %q should keep the vmherd- prefix", bridgeName)
	}
	if bridgeNameForNetwork("vmherd-web-8d7c1111") != bridgeName {
		t.Error("bridge name is not deterministic")
	}
}

func TestBridgeNameForNetworkSharedPrefixDoesNotCollide(t *testing.T) {
	firstBridge := bridgeNameForNetwork("vmherd-web-8d7c1111")
	secondBridge := bridgeNameForNetwork("vmherd-web-5f3b2c5e")
	if firstBridge == secondBridge {
		t.Errorf("networks with a shared 15-char prefix both got bridge %q", firstBridge)
	}
}

func TestFirstFreeSubnet(t *testing.T) {
	testCases := []struct {
		usedCIDRs    []string // networks already in use
		wantBase     string   // expected free subnet base
		wantNoSubnet bool     // expect an error because the pool is full
	}{
		{nil, "192.168.150", false},
		{[]string{"192.168.122.0/24", "192.168.100.49/24"}, "192.168.150", false},
		{[]string{"192.168.150.0/24", "192.168.151.0/24"}, "192.168.152", false},
		{[]string{"192.168.150.1/32"}, "192.168.151", false},
		{[]string{"192.168.144.0/20"}, "192.168.160", false},
		{[]string{"192.168.0.0/16"}, "", true},
	}
	for _, testCase := range testCases {
		gotBase, err := firstFreeSubnet(testCase.usedCIDRs)
		if testCase.wantNoSubnet {
			if err == nil || !strings.Contains(err.Error(), "set 'subnet'") {
				t.Errorf("firstFreeSubnet(%v): expected an actionable error, got %q, %v", testCase.usedCIDRs, gotBase, err)
			}
			continue
		}
		if err != nil || gotBase != testCase.wantBase {
			t.Errorf("firstFreeSubnet(%v) = %q, %v; want %q", testCase.usedCIDRs, gotBase, err, testCase.wantBase)
		}
	}
}

func TestParseRouteTableCIDRs(t *testing.T) {
	routeTable := `Iface	Destination	Gateway 	Flags	RefCnt	Use	Metric	Mask		MTU	Window	IRTT
eth0	00000000	0164A8C0	0003	0	0	0	00000000	0	0	0
eth0	0064A8C0	00000000	0001	0	0	0	00FFFFFF	0	0	0
tun0	0096A8C0	00000000	0001	0	0	0	00FFFFFF	0	0	0
docker0	000011AC	00000000	0001	0	0	0	0000FFFF	0	0	0
`
	gotCIDRs := parseRouteTableCIDRs(routeTable)
	wantCIDRs := []string{"192.168.100.0/24", "192.168.150.0/24", "172.17.0.0/16"}
	if !slices.Equal(gotCIDRs, wantCIDRs) {
		t.Errorf("parseRouteTableCIDRs() = %v, want %v (default route skipped)", gotCIDRs, wantCIDRs)
	}
}

func TestDomainUsesNetwork(t *testing.T) {
	domiflistOutput := ` Interface   Type      Source           Model    MAC
-----------------------------------------------------------------
 vnet3       network   vmherd-default   virtio   52:54:00:ab:cd:01
 vnet4       bridge    br0              virtio   52:54:00:ab:cd:02
`
	if !domainUsesNetwork(domiflistOutput, "vmherd-default") {
		t.Error("expected the domain to use vmherd-default")
	}
	if domainUsesNetwork(domiflistOutput, "br0") {
		t.Error("a bridge interface is not a libvirt network")
	}
	if domainUsesNetwork(domiflistOutput, "vmherd-other") {
		t.Error("the domain does not use vmherd-other")
	}
}
