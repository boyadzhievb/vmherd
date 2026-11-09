package vm

import (
	"regexp"
	"slices"
	"strings"
	"testing"
)

// qemuMACPattern matches a valid QEMU MAC address with the 52:54:00 OUI prefix
// followed by three random hex octets.
var qemuMACPattern = regexp.MustCompile(`^52:54:00:[0-9a-f]{2}:[0-9a-f]{2}:[0-9a-f]{2}$`)

// TestGenerateMACAddress verifies that GenerateMACAddress produces a valid QEMU MAC
// with the correct prefix, format, and uniqueness across successive calls.
func TestGenerateMACAddress(t *testing.T) {
	firstMAC, err := GenerateMACAddress()
	if err != nil {
		t.Fatalf("GenerateMACAddress() returned unexpected error: %v", err)
	}

	// Verify the QEMU OUI prefix.
	const expectedPrefix = "52:54:00:"
	if !strings.HasPrefix(firstMAC, expectedPrefix) {
		t.Errorf("MAC address %q does not start with QEMU OUI prefix %q", firstMAC, expectedPrefix)
	}

	// Verify the format is six colon-separated hex octets.
	octets := strings.Split(firstMAC, ":")
	const expectedOctetCount = 6
	if len(octets) != expectedOctetCount {
		t.Errorf("MAC address %q has %d octets, want %d", firstMAC, len(octets), expectedOctetCount)
	}

	if !qemuMACPattern.MatchString(firstMAC) {
		t.Errorf("MAC address %q does not match expected pattern %s", firstMAC, qemuMACPattern.String())
	}

	// Verify two successive calls produce different addresses.
	secondMAC, err := GenerateMACAddress()
	if err != nil {
		t.Fatalf("second GenerateMACAddress() returned unexpected error: %v", err)
	}

	if firstMAC == secondMAC {
		t.Errorf("two successive calls returned the same MAC address: %s", firstMAC)
	}
}

// TestGenerateMACAddressFormat generates multiple MAC addresses and confirms every one
// matches the expected QEMU OUI pattern.
func TestGenerateMACAddressFormat(t *testing.T) {
	const generationCount = 10

	for iteration := 0; iteration < generationCount; iteration++ {
		macAddress, err := GenerateMACAddress()
		if err != nil {
			t.Fatalf("iteration %d: GenerateMACAddress() returned unexpected error: %v", iteration, err)
		}

		if !qemuMACPattern.MatchString(macAddress) {
			t.Errorf("iteration %d: MAC address %q does not match pattern %s",
				iteration, macAddress, qemuMACPattern.String())
		}
	}
}

func TestParseDomainInterfaceMACs(t *testing.T) {
	domiflistOutput := ` Interface   Type      Source           Model    MAC
-----------------------------------------------------------------
 vnet3       network   vmherd-default   virtio   52:54:00:AB:cd:01
 -           network   other            e1000    52:54:00:12:34:56

`
	macAddresses := parseDomainInterfaceMACs(domiflistOutput)
	expected := []string{"52:54:00:ab:cd:01", "52:54:00:12:34:56"}
	if !slices.Equal(macAddresses, expected) {
		t.Errorf("expected %v, got %v", expected, macAddresses)
	}
}

func TestParseDomainInterfaceMACsEmpty(t *testing.T) {
	domiflistOutput := " Interface   Type   Source   Model   MAC\n-------------------------------------------\n\n"
	if macAddresses := parseDomainInterfaceMACs(domiflistOutput); len(macAddresses) != 0 {
		t.Errorf("expected no MACs, got %v", macAddresses)
	}
}

func TestParseLeaseIPv4(t *testing.T) {
	domifaddrOutput := ` Name       MAC address          Protocol     Address
-------------------------------------------------------------------------------
 vnet7      52:54:00:ab:cd:01    ipv6         fd00::5/64
 vnet7      52:54:00:ab:cd:01    ipv4         192.168.150.123/24

`
	if got := parseLeaseIPv4(domifaddrOutput); got != "192.168.150.123" {
		t.Errorf("parseLeaseIPv4() = %q, want 192.168.150.123", got)
	}
	noLease := " Name       MAC address          Protocol     Address\n-------------------------------------------------------\n\n"
	if got := parseLeaseIPv4(noLease); got != "" {
		t.Errorf("parseLeaseIPv4() without a lease = %q, want empty", got)
	}
}
