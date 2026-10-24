package portfwd

import (
	"net"
	"testing"
)

func TestCheckConflictAvailablePort(t *testing.T) {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("cannot find a free port: %v", err)
	}
	freePort := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()

	if err := CheckConflict(freePort, "tcp"); err != nil {
		t.Errorf("expected no conflict on free port %d, got: %v", freePort, err)
	}
}

func TestCheckConflictBoundTCPPort(t *testing.T) {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("cannot listen: %v", err)
	}
	defer func() { _ = listener.Close() }()

	boundPort := listener.Addr().(*net.TCPAddr).Port
	if err := CheckConflict(boundPort, "tcp"); err == nil {
		t.Errorf("expected conflict on bound TCP port %d", boundPort)
	}
}

func TestCheckConflictBoundUDPPort(t *testing.T) {
	packetConn, err := net.ListenPacket("udp", ":0")
	if err != nil {
		t.Fatalf("cannot listen: %v", err)
	}
	defer func() { _ = packetConn.Close() }()

	boundPort := packetConn.LocalAddr().(*net.UDPAddr).Port
	if err := CheckConflict(boundPort, "udp"); err == nil {
		t.Errorf("expected conflict on bound UDP port %d", boundPort)
	}
}

func TestCheckConflictInvalidPort(t *testing.T) {
	if err := CheckConflict(0, "tcp"); err == nil {
		t.Error("expected error for port 0")
	}
	if err := CheckConflict(70000, "tcp"); err == nil {
		t.Error("expected error for port 70000")
	}
}

func TestCheckConflictInvalidProtocol(t *testing.T) {
	if err := CheckConflict(8080, "sctp"); err == nil {
		t.Error("expected error for unsupported protocol")
	}
}

func TestCheckConflictDefaultProtocol(t *testing.T) {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("cannot find a free port: %v", err)
	}
	freePort := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()

	if err := CheckConflict(freePort, ""); err != nil {
		t.Errorf("expected no conflict with empty (default tcp) protocol on free port: %v", err)
	}
}

func TestNormalizeProtocol(t *testing.T) {
	rule := normalizeProtocol(Rule{HostPort: 80, GuestPort: 80, GuestIP: "10.0.0.1"})
	if rule.Protocol != "tcp" {
		t.Errorf("expected default protocol 'tcp', got %q", rule.Protocol)
	}

	ruleWithProto := normalizeProtocol(Rule{HostPort: 53, GuestPort: 53, GuestIP: "10.0.0.1", Protocol: "udp"})
	if ruleWithProto.Protocol != "udp" {
		t.Errorf("expected protocol 'udp', got %q", ruleWithProto.Protocol)
	}
}
