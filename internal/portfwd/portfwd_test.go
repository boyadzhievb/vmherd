package portfwd

import (
	"testing"
)

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
