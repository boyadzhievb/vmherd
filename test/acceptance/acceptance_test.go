// Package acceptance holds the example projects vmherd ships for users to
// copy (test/acceptance/<scenario>/). These tests need no libvirt and run on
// every pull request; test/e2e TestAcceptanceScenarios boots the scenarios.
package acceptance

import (
	"path/filepath"
	"testing"

	"github.com/boyadzhievb/vmherd/internal/config"
)

// libvirtDefaultSubnet is the subnet of libvirt's stock 'default' network,
// which most hosts already have; a scenario on it can't create its network.
const libvirtDefaultSubnet = "192.168.122.0/24"

// scenarioDirectories lists every scenario, relative to this package.
var scenarioDirectories = []string{"terramino-single", "terramino-multi"}

// loadScenarioHerdfile loads a scenario's Herdfile from its directory, as
// 'vmherd' does when run there, and fails the test on any problem.
func loadScenarioHerdfile(t *testing.T, scenarioDirectory string) *config.Herdfile {
	t.Helper()
	absoluteDirectory, err := filepath.Abs(scenarioDirectory)
	if err != nil {
		t.Fatalf("resolving %s: %v", scenarioDirectory, err)
	}
	herdfile, err := config.Load(absoluteDirectory)
	if err != nil {
		t.Fatalf("scenario %s: %v", scenarioDirectory, err)
	}
	return herdfile
}

// TestScenarioHerdfilesLoad checks every scenario's Herdfile loads without
// problems, so a broken example fails here instead of in the nightly run.
func TestScenarioHerdfilesLoad(t *testing.T) {
	for _, scenarioDirectory := range scenarioDirectories {
		t.Run(scenarioDirectory, func(t *testing.T) {
			loadScenarioHerdfile(t, scenarioDirectory)
		})
	}
}

// TestScenarioNetworksAreDistinct checks no two scenarios share a network
// name or subnet and none uses libvirt's default subnet, so each scenario
// creates its own network wherever it runs.
func TestScenarioNetworksAreDistinct(t *testing.T) {
	scenarioByNetworkName := make(map[string]string)
	scenarioBySubnet := make(map[string]string)
	for _, scenarioDirectory := range scenarioDirectories {
		herdfile := loadScenarioHerdfile(t, scenarioDirectory)
		networkName, subnet := herdfile.Network.Name, herdfile.Network.Subnet
		if subnet == libvirtDefaultSubnet {
			t.Errorf("scenario %s uses %s, libvirt's default network subnet", scenarioDirectory, subnet)
		}
		if otherScenario, taken := scenarioByNetworkName[networkName]; taken {
			t.Errorf("scenarios %s and %s use the same network name %s", otherScenario, scenarioDirectory, networkName)
		}
		if otherScenario, taken := scenarioBySubnet[subnet]; taken {
			t.Errorf("scenarios %s and %s use the same subnet %s", otherScenario, scenarioDirectory, subnet)
		}
		scenarioByNetworkName[networkName] = scenarioDirectory
		scenarioBySubnet[subnet] = scenarioDirectory
	}
}
