package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeHerdfile writes content as the directory's Herdfile and loads it.
func loadHerdfileString(t *testing.T, content string) (*Herdfile, error) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, HerdfileName), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return Load(dir)
}

// bridgeHerdfile returns a valid bridge-mode Herdfile with the given network section.
func bridgeHerdfile(networkSection string) string {
	return `
images:
  ubuntu:noble: https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-amd64.img
vms:
  - name: lan
    image: ubuntu:noble
    network:
      ip: 192.168.100.50
` + networkSection
}

// TestBridgeNetworkValid verifies a bridge Herdfile loads, derives the subnet
// from the gateway and reports bridge mode.
func TestBridgeNetworkValid(t *testing.T) {
	herdfile, err := loadHerdfileString(t, bridgeHerdfile(`
network:
  type: bridge
  bridge: br1
  gateway: 192.168.100.1
`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !herdfile.Network.IsBridge() {
		t.Error("expected IsBridge() to be true")
	}
	if herdfile.Network.Subnet != "192.168.100.0/24" {
		t.Errorf("expected derived subnet 192.168.100.0/24, got %q", herdfile.Network.Subnet)
	}
}

// TestBridgeNetworkAllowsDHCPPoolAddresses verifies that bridge-mode VM IPs
// inside .100-.200 pass: that range is only reserved on vmherd's NAT network.
func TestBridgeNetworkAllowsDHCPPoolAddresses(t *testing.T) {
	_, err := loadHerdfileString(t, `
images:
  ubuntu:noble: https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-amd64.img
vms:
  - name: lan
    image: ubuntu:noble
    network:
      ip: 192.168.100.150
network:
  type: bridge
  bridge: br1
  gateway: 192.168.100.1
`)
	if err != nil {
		t.Fatalf("expected .150 to be allowed in bridge mode, got: %v", err)
	}
}

// TestBridgeNetworkAllowsDHCPVMs verifies a bridge-mode VM without an ip
// (LAN DHCP) is accepted.
func TestBridgeNetworkAllowsDHCPVMs(t *testing.T) {
	_, err := loadHerdfileString(t, `
images:
  ubuntu:noble: https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-amd64.img
vms:
  - name: lan
    image: ubuntu:noble
network:
  type: bridge
  bridge: br1
  gateway: 192.168.100.1
`)
	if err != nil {
		t.Fatalf("expected DHCP VM to be allowed in bridge mode, got: %v", err)
	}
}

// TestBridgeNetworkProblems checks the bridge-mode validation errors.
func TestBridgeNetworkProblems(t *testing.T) {
	testCases := []struct {
		name           string
		networkSection string
		wantProblem    string
	}{
		{
			name:           "missing bridge device",
			networkSection: "network:\n  type: bridge\n  gateway: 192.168.100.1\n",
			wantProblem:    "network.bridge is required",
		},
		{
			name:           "missing gateway",
			networkSection: "network:\n  type: bridge\n  bridge: br1\n",
			wantProblem:    "network.gateway is required",
		},
		{
			name:           "invalid gateway",
			networkSection: "network:\n  type: bridge\n  bridge: br1\n  gateway: not-an-ip\n",
			wantProblem:    "not an IPv4 address",
		},
		{
			name:           "gateway outside subnet",
			networkSection: "network:\n  type: bridge\n  bridge: br1\n  gateway: 10.0.0.1\n  subnet: 192.168.100.0/24\n",
			wantProblem:    "outside the network subnet",
		},
		{
			name:           "bridge name too long",
			networkSection: "network:\n  type: bridge\n  bridge: averylongbridgename\n  gateway: 192.168.100.1\n",
			wantProblem:    "longer than 15 characters",
		},
		{
			name:           "unknown type",
			networkSection: "network:\n  type: mesh\n  bridge: br1\n  gateway: 192.168.100.1\n",
			wantProblem:    "network.type \"mesh\" is invalid",
		},
		{
			name:           "bridge fields without bridge type",
			networkSection: "network:\n  bridge: br1\n  gateway: 192.168.100.1\n",
			wantProblem:    "network.bridge \"br1\" is set but network.type is not 'bridge'",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := loadHerdfileString(t, bridgeHerdfile(testCase.networkSection))
			if err == nil {
				t.Fatalf("expected an error containing %q, got none", testCase.wantProblem)
			}
			if !strings.Contains(err.Error(), testCase.wantProblem) {
				t.Errorf("expected error containing %q, got: %v", testCase.wantProblem, err)
			}
		})
	}
}
