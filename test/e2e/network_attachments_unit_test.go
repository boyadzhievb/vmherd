package e2e

import (
	"slices"
	"testing"
)

// TestParseDomiflistNetworks checks that only interfaces of type "network"
// are returned, for running and shut-off domains alike.
func TestParseDomiflistNetworks(t *testing.T) {
	testCases := []struct {
		name             string   // what the case covers
		domiflistOutput  string   // virsh domiflist output
		expectedNetworks []string // networks the domain is attached to
	}{
		{
			name: "running domain on a network",
			domiflistOutput: ` Interface   Type      Source                   Model    MAC
-----------------------------------------------------------------------------
 vnet3       network   vmherd-terramino-multi   virtio   52:54:00:aa:bb:cc
`,
			expectedNetworks: []string{"vmherd-terramino-multi"},
		},
		{
			name: "shut-off domain has no interface name",
			domiflistOutput: ` Interface   Type      Source                   Model    MAC
-----------------------------------------------------------------------------
 -           network   vmherd-terramino-multi   virtio   52:54:00:aa:bb:cc
`,
			expectedNetworks: []string{"vmherd-terramino-multi"},
		},
		{
			name: "bridge interface is not a network",
			domiflistOutput: ` Interface   Type     Source   Model    MAC
-------------------------------------------------------
 vnet4       bridge   br1      virtio   52:54:00:aa:bb:dd
 vnet5       network  default  virtio   52:54:00:aa:bb:ee
`,
			expectedNetworks: []string{"default"},
		},
		{
			name: "no interfaces",
			domiflistOutput: ` Interface   Type   Source   Model   MAC
-------------------------------------------
`,
			expectedNetworks: nil,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			networks := parseDomiflistNetworks(testCase.domiflistOutput)
			if !slices.Equal(networks, testCase.expectedNetworks) {
				t.Errorf("parseDomiflistNetworks = %v, want %v", networks, testCase.expectedNetworks)
			}
		})
	}
}

// TestForeignDomainsOnNetwork checks that a network counts as in use only
// through domains outside the test project's prefix, sorted by name.
func TestForeignDomainsOnNetwork(t *testing.T) {
	const networkName = "vmherd-terramino-multi"
	const domainPrefix = "vmherd-acceptance-terramino-multi-"
	testCases := []struct {
		name             string              // what the case covers
		networksByDomain map[string][]string // domain → networks it is attached to
		expectedDomains  []string            // domains that block removing the network
	}{
		{
			name: "only the project's own domains",
			networksByDomain: map[string][]string{
				domainPrefix + "redis":   {networkName},
				domainPrefix + "backend": {networkName},
			},
			expectedDomains: nil,
		},
		{
			name: "another project's domains on the network",
			networksByDomain: map[string][]string{
				domainPrefix + "redis":                {networkName},
				"vmherd-terramino-multi-1a2b-redis":   {networkName},
				"vmherd-terramino-multi-1a2b-backend": {"other-network", networkName},
			},
			expectedDomains: []string{"vmherd-terramino-multi-1a2b-backend", "vmherd-terramino-multi-1a2b-redis"},
		},
		{
			name: "other domains on other networks",
			networksByDomain: map[string][]string{
				"vmherd-other-1a2b-ctrl": {"vmherd-other"},
				"plain-domain":           nil,
			},
			expectedDomains: nil,
		},
		{
			name:             "no domains at all",
			networksByDomain: map[string][]string{},
			expectedDomains:  nil,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			foreignDomains := foreignDomainsOnNetwork(networkName, domainPrefix, testCase.networksByDomain)
			if !slices.Equal(foreignDomains, testCase.expectedDomains) {
				t.Errorf("foreignDomainsOnNetwork = %v, want %v", foreignDomains, testCase.expectedDomains)
			}
		})
	}
}
