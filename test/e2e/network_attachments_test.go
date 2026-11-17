package e2e

// This file has no build tag: the decision whether a leftover network may be
// removed is pure, so it is unit-tested on every 'go test ./...'. The
// integration tests gather the input with virsh (see removeLeftovers).

import (
	"slices"
	"strings"
)

// domiflistHeaderLines is how many lines 'virsh domiflist' prints before the
// first interface: the column titles and a dashed rule.
const domiflistHeaderLines = 2

// domiflistNetworkType is the interface type of a NIC attached to a libvirt
// network (as opposed to "bridge", whose source is a host bridge).
const domiflistNetworkType = "network"

// parseDomiflistNetworks returns the libvirt networks a domain's interfaces
// are attached to, from 'virsh domiflist <domain>' output (columns:
// Interface, Type, Source, Model, MAC).
func parseDomiflistNetworks(domiflistOutput string) []string {
	var networkNames []string
	lines := strings.Split(strings.TrimSpace(domiflistOutput), "\n")
	for lineIndex, line := range lines {
		columns := strings.Fields(line)
		if lineIndex < domiflistHeaderLines || len(columns) < 3 {
			continue
		}
		if columns[1] == domiflistNetworkType {
			networkNames = append(networkNames, columns[2])
		}
	}
	return networkNames
}

// foreignDomainsOnNetwork lists, sorted, the domains attached to networkName
// whose name does not start with domainPrefix: domains of another project,
// which removing the network would cut off.
func foreignDomainsOnNetwork(networkName, domainPrefix string, networksByDomain map[string][]string) []string {
	var foreignDomains []string
	for domainName, networkNames := range networksByDomain {
		if strings.HasPrefix(domainName, domainPrefix) {
			continue
		}
		if slices.Contains(networkNames, networkName) {
			foreignDomains = append(foreignDomains, domainName)
		}
	}
	slices.Sort(foreignDomains)
	return foreignDomains
}
