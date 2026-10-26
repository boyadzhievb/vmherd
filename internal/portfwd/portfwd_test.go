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

func TestExistingNATRuleConflictsExactMatch(t *testing.T) {
	iptablesOutput := `Chain PREROUTING (policy ACCEPT)
target     prot opt source               destination
DNAT       tcp  --  0.0.0.0/0            0.0.0.0/0            tcp dpt:8080 to:192.168.122.10:80`

	if !existingNATRuleConflicts(iptablesOutput, 8080, "tcp") {
		t.Error("expected conflict for port 8080/tcp")
	}
}

func TestExistingNATRuleConflictsNoFalseMatchShorterPort(t *testing.T) {
	iptablesOutput := `Chain PREROUTING (policy ACCEPT)
target     prot opt source               destination
DNAT       tcp  --  0.0.0.0/0            0.0.0.0/0            tcp dpt:8080 to:192.168.122.10:80`

	if existingNATRuleConflicts(iptablesOutput, 80, "tcp") {
		t.Error("should not match port 80 when only 8080 exists")
	}
}

func TestExistingNATRuleConflictsNoFalseMatchLongerPort(t *testing.T) {
	iptablesOutput := `Chain PREROUTING (policy ACCEPT)
target     prot opt source               destination
DNAT       tcp  --  0.0.0.0/0            0.0.0.0/0            tcp dpt:80 to:192.168.122.10:80`

	if existingNATRuleConflicts(iptablesOutput, 8080, "tcp") {
		t.Error("should not match port 8080 when only 80 exists")
	}
}

func TestExistingNATRuleConflictsEmptyOutput(t *testing.T) {
	if existingNATRuleConflicts("", 8080, "tcp") {
		t.Error("expected no conflict for empty output")
	}
}

func TestExistingNATRuleConflictsMultipleRulesOneMatching(t *testing.T) {
	iptablesOutput := `Chain PREROUTING (policy ACCEPT)
target     prot opt source               destination
DNAT       tcp  --  0.0.0.0/0            0.0.0.0/0            tcp dpt:3000 to:192.168.122.10:3000
DNAT       tcp  --  0.0.0.0/0            0.0.0.0/0            tcp dpt:8080 to:192.168.122.10:80
DNAT       udp  --  0.0.0.0/0            0.0.0.0/0            udp dpt:53 to:192.168.122.10:53`

	if !existingNATRuleConflicts(iptablesOutput, 8080, "tcp") {
		t.Error("expected conflict for port 8080/tcp among multiple rules")
	}
}

func TestExistingNATRuleConflictsProtocolMismatch(t *testing.T) {
	iptablesOutput := `Chain PREROUTING (policy ACCEPT)
target     prot opt source               destination
DNAT       udp  --  0.0.0.0/0            0.0.0.0/0            udp dpt:8080 to:192.168.122.10:8080`

	if existingNATRuleConflicts(iptablesOutput, 8080, "tcp") {
		t.Error("should not match udp rule when looking for tcp")
	}
}

func TestExistingNATRuleConflictsUDPMatch(t *testing.T) {
	iptablesOutput := `Chain PREROUTING (policy ACCEPT)
target     prot opt source               destination
DNAT       udp  --  0.0.0.0/0            0.0.0.0/0            udp dpt:53 to:192.168.122.10:53`

	if !existingNATRuleConflicts(iptablesOutput, 53, "udp") {
		t.Error("expected conflict for port 53/udp")
	}
}

func TestExistingNATRuleConflictsPortAtEndOfLine(t *testing.T) {
	iptablesOutput := `Chain PREROUTING (policy ACCEPT)
target     prot opt source               destination
DNAT       tcp  --  0.0.0.0/0            0.0.0.0/0            tcp dpt:443`

	if !existingNATRuleConflicts(iptablesOutput, 443, "tcp") {
		t.Error("expected conflict for port 443/tcp at end of line")
	}
}

func TestExistingNATRuleConflictsHeaderOnly(t *testing.T) {
	iptablesOutput := `Chain PREROUTING (policy ACCEPT)
target     prot opt source               destination`

	if existingNATRuleConflicts(iptablesOutput, 8080, "tcp") {
		t.Error("expected no conflict for header-only output")
	}
}

func TestExistingNATRuleConflictsNumberedOutput(t *testing.T) {
	iptablesOutput := `Chain PREROUTING (policy ACCEPT)
num  target     prot opt source               destination
1    DNAT       tcp  --  0.0.0.0/0            0.0.0.0/0            tcp dpt:8080 to:192.168.122.10:80`

	if !existingNATRuleConflicts(iptablesOutput, 8080, "tcp") {
		t.Error("expected conflict for port 8080/tcp in numbered output")
	}
}
