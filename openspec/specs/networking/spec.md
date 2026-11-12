# networking Specification

## Purpose

A project's VMs share one network: by default a vmherd-managed libvirt NAT network with its own /24, or an existing host bridge on the LAN. VMs get a static address through cloud-init or one from DHCP, and DNS comes from the host, never from a hardcoded server.

## Requirements

### Requirement: Managed NAT network
For `type: nat` (the default), `up` and `reload` SHALL create the project's libvirt network if missing (gateway `.1`, DHCP range `.100`–`.200`, autostart), SHALL start it if it is stopped, and SHALL refuse a subnet that overlaps another libvirt network. Only /24 subnets SHALL be accepted.

#### Scenario: Overlapping subnet
- **WHEN** the project's subnet overlaps an existing libvirt network's subnet
- **THEN** up fails before creating anything, names the other network, and suggests a free subnet
- **Test:** overlap detection: internal/network/TestSubnetsOverlapIdentical, internal/network/TestSubnetsOverlapSamePrefix, internal/network/TestSubnetsOverlapDisjoint, internal/network/TestSubnetsOverlapWideContainsNarrow, internal/network/TestSubnetsOverlapNarrowInsideWide, internal/network/TestSubnetsOverlapCompletelyDisjoint, internal/network/TestSubnetsOverlapInvalidCIDR; the error message itself: none yet — needs libvirt (M2.3)

#### Scenario: Not a /24
- **WHEN** the subnet is `10.0.0.0/16`
- **THEN** loading the Herdfile fails: only /24 subnets are supported
- **Test:** internal/config/TestValidateSubnetNon24Rejected

#### Scenario: Gateway address
- **WHEN** the subnet is `192.168.150.0/24`
- **THEN** the network's gateway is 192.168.150.1
- **Test:** internal/config/TestGatewayIP, internal/config/TestGatewayIPInvalidCIDR, internal/config/TestSubnetBase, internal/config/TestSubnetBaseCustom, internal/config/TestSubnetBaseInvalidCIDR

#### Scenario: Network names
- **WHEN** a network or Herdfile network name has characters other than letters, digits and hyphens
- **THEN** loading fails with the naming rule
- **Test:** internal/config/TestValidateNetworkNameSpecialChars

#### Scenario: Long network names
- **WHEN** the network name is longer than the 15-character Linux bridge limit
- **THEN** the bridge name is a prefix plus a hash of the network name, so two long names with the same prefix don't collide
- **Test:** internal/network/TestBridgeNameForNetworkShort, internal/network/TestBridgeNameForNetworkLong, internal/network/TestBridgeNameForNetworkSharedPrefixDoesNotCollide

#### Scenario: Missing network is harmless to query
- **WHEN** vmherd checks or destroys a network that doesn't exist
- **THEN** it reports it as absent and destroying it is a no-op
- **Test:** internal/network/TestNetworkExistsReturnsFalseForMissing, internal/network/TestNetworkIsActiveReturnsFalseForMissing, internal/network/TestNetworkDestroyNoopForMissing (integration tag)

### Requirement: init picks a free subnet
`vmherd init` SHALL write a project-unique network name `vmherd-<project>-<hash>` and the first /24 in 192.168.150.0–192.168.249.0 that no libvirt network, host interface or host route uses (the default route is ignored). When libvirt can't be queried it SHALL warn and use 192.168.150.0/24.

#### Scenario: Taken subnets are skipped
- **WHEN** 192.168.150.0/24 and 192.168.151.0/24 are in use
- **THEN** init writes 192.168.152.0/24; when every /24 in the range is taken it asks the user to set `subnet` by hand
- **Test:** internal/network/TestFirstFreeSubnet, internal/network/TestParseRouteTableCIDRs, internal/network/TestIPAndMaskToCIDRWithNetmask, internal/network/TestIPAndMaskToCIDRWithPrefix, internal/network/TestIPAndMaskToCIDRNoMask, internal/network/TestIPAndMaskToCIDRInvalidAddress, internal/network/TestIPAndMaskToCIDRInvalidNetmask, internal/network/TestIPAndMaskToCIDRNetworkAddress

#### Scenario: Project-unique network name
- **WHEN** init runs in two directories with the same name
- **THEN** each gets its own network name
- **Test:** internal/config/TestDefaultNetworkNameIsProjectUnique

### Requirement: Static or DHCP address
A VM with `network.ip` SHALL get that address through cloud-init network-config; a VM without it SHALL use DHCP, with DNS still taken from the host. On a NAT network a static address SHALL be inside the subnet, outside the DHCP range `.100`–`.200`, and unique in the project. `network.gateway` without `network.ip` SHALL be refused.

#### Scenario: Static address
- **WHEN** a VM has `ip: 192.168.150.10`
- **THEN** its network-config sets that address with the subnet's .1 as gateway and boots reachable at it
- **Test:** internal/cloudinit/TestGenerateFilesNetworkConfigContent, internal/cloudinit/TestGenerateFilesCustomGateway, test/e2e/TestDistroLifecycle (checkStatusShowsAddresses)

#### Scenario: DHCP address
- **WHEN** a VM has no `ip`
- **THEN** its network-config uses DHCP without the DHCP server's DNS, and status shows a leased address in the subnet
- **Test:** internal/cloudinit/TestGenerateFilesNetworkConfigDHCP, internal/vm/TestParseLeaseIPv4, test/e2e/TestDistroLifecycle (checkStatusShowsAddresses)

#### Scenario: Invalid static addresses
- **WHEN** an `ip` is not IPv4, is outside the subnet, is in `.100`–`.200`, or is used by another VM
- **THEN** loading fails naming the VM and the address
- **Test:** internal/config/TestValidateIPAddressInvalid, internal/config/TestValidateIPOutsideSubnet, internal/config/TestValidateIPInDHCPPoolRejected, internal/config/TestValidateIPOutsideDHCPPoolAccepted, internal/config/TestValidateIPAddressDuplicate, internal/config/TestVMWithoutIPIsValid

#### Scenario: Gateway without address
- **WHEN** a VM sets `network.gateway` but no `network.ip`
- **THEN** loading fails: DHCP provides the gateway
- **Test:** internal/config/TestGatewayWithoutIPIsRefused

#### Scenario: MAC address per VM
- **WHEN** a VM is created
- **THEN** it gets a new random MAC with QEMU's `52:54:00` prefix, saved in its network-config so vmherd can recognise the domain later (a MAC that stays the same across reload is planned in M5.2)
- **Test:** internal/vm/TestGenerateMACAddress, internal/vm/TestGenerateMACAddressFormat, internal/vm/TestParseDomainInterfaceMACs, internal/vm/TestParseDomainInterfaceMACsEmpty

### Requirement: DNS from the host
With `dns: auto` (the default), VMs SHALL use the first non-loopback nameserver in the host's `/etc/resolv.conf`, falling back to the network gateway. An explicit `dns:` SHALL be an IP address. vmherd SHALL never use a hardcoded public DNS server.

#### Scenario: Auto DNS
- **WHEN** the host's resolv.conf lists `127.0.0.53` then `192.168.1.1`
- **THEN** VMs use 192.168.1.1
- **Test:** internal/vm/TestResolveDNSAutoReadsResolvConf, internal/vm/TestResolveDNSParsesNameserver, internal/vm/TestIsLoopback, internal/vm/TestResolveDNSEmptyFallsBackToAuto

#### Scenario: No usable host nameserver
- **WHEN** resolv.conf has only loopback nameservers or can't be read
- **THEN** VMs use the network gateway
- **Test:** internal/vm/TestResolveDNSFallsBackToGateway

#### Scenario: Explicit DNS
- **WHEN** the Herdfile sets `dns: 192.168.1.1`
- **THEN** VMs use it, and a host name instead of an IP is refused
- **Test:** internal/vm/TestResolveDNSExplicitValue, internal/config/TestValidateFieldRules

### Requirement: Bridge mode
With `network: {type: bridge, bridge: <device>, gateway: <router>}` vmherd SHALL attach VMs to the existing host bridge, create no libvirt network and run no DHCP; `subnet` SHALL default to the gateway's /24, the `.100`–`.200` reservation SHALL not apply, and DHCP addresses SHALL be read from the host ARP table. `up` and `reload` SHALL fail when the bridge doesn't exist; `destroy` SHALL never remove it.

#### Scenario: Valid bridge network
- **WHEN** the Herdfile sets type bridge, bridge `br1` and gateway 192.168.100.1
- **THEN** it loads with subnet 192.168.100.0/24 and accepts static IPs in .100–.200 and DHCP VMs
- **Test:** internal/config/TestBridgeNetworkValid, internal/config/TestBridgeNetworkAllowsDHCPPoolAddresses, internal/config/TestBridgeNetworkAllowsDHCPVMs

#### Scenario: Incomplete bridge network
- **WHEN** bridge mode lacks `bridge` or `gateway`, or `bridge` is set without `type: bridge`
- **THEN** loading fails saying which field to add
- **Test:** internal/config/TestBridgeNetworkProblems

#### Scenario: Bridge missing on the host
- **WHEN** the named bridge does not exist under `/sys/class/net`
- **THEN** up fails naming the bridge before creating any VM
- **Test:** none yet — cmd/ code, needs M2.3
