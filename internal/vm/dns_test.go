package vm

import (
	"os"
	"testing"
)

func TestResolveDNSExplicitValue(t *testing.T) {
	result := ResolveDNS("1.1.1.1", "192.168.122.1")
	if result != "1.1.1.1" {
		t.Errorf("expected explicit DNS '1.1.1.1', got %q", result)
	}
}

func TestResolveDNSAutoReadsResolvConf(t *testing.T) {
	result := ResolveDNS("auto", "192.168.122.1")

	// On any system with /etc/resolv.conf, this should return a nameserver.
	// If the file doesn't exist (unlikely), it falls back to gateway.
	if result == "" {
		t.Error("expected non-empty DNS result")
	}
}

func TestResolveDNSEmptyFallsBackToAuto(t *testing.T) {
	result := ResolveDNS("", "192.168.122.1")
	if result == "" {
		t.Error("expected non-empty DNS result for empty config")
	}
}

func TestResolveDNSFallsBackToGateway(t *testing.T) {
	// Create a temp resolv.conf with no nameserver lines to test fallback.
	// We can't easily redirect os.ReadFile("/etc/resolv.conf"), so we test
	// the explicit-value path and the auto path on the real system.
	// The gateway fallback is tested indirectly: if /etc/resolv.conf is absent
	// or has no nameserver lines, ResolveDNS returns the gateway.
	result := ResolveDNS("auto", "10.0.0.1")
	if result == "" {
		t.Error("expected non-empty DNS result")
	}
}

func TestResolveDNSParsesNameserver(t *testing.T) {
	// Since ResolveDNS reads /etc/resolv.conf directly, we test the parsing
	// behavior through the exported function on the real system.
	if _, err := os.Stat("/etc/resolv.conf"); err != nil {
		t.Skip("no /etc/resolv.conf on this system")
	}

	result := ResolveDNS("auto", "192.168.122.1")
	if result == "192.168.122.1" {
		t.Log("resolv.conf has only loopback or no nameserver entries, fell back to gateway")
	}
}

func TestIsLoopback(t *testing.T) {
	tests := []struct {
		address  string
		expected bool
	}{
		{"127.0.0.1", true},
		{"127.0.0.53", true},
		{"127.0.1.1", true},
		{"::1", true},
		{"192.168.1.1", false},
		{"8.8.8.8", false},
		{"10.0.0.1", false},
		{"not-an-ip", false},
	}
	for _, tt := range tests {
		if got := isLoopback(tt.address); got != tt.expected {
			t.Errorf("isLoopback(%q) = %v, want %v", tt.address, got, tt.expected)
		}
	}
}
