package vm

import (
	"regexp"
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
